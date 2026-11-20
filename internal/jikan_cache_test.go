package internal

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// Jikan (api.jikan.moe) hangs and rate-limits hard, while GetEpisodeData
// blocks every episode start and FetchFillerEpisodes pages per anime. These
// pin the disk cache + bounded retries that keep playback off the network
// after the first successful fetch.

func withJikanTestConfig(t *testing.T, storage string) {
	t.Helper()
	old := GetGlobalConfig()
	SetGlobalConfig(&CurdConfig{StoragePath: storage})
	t.Cleanup(func() { SetGlobalConfig(old) })
}

func withJikanBaseURL(t *testing.T, url string) {
	t.Helper()
	old := jikanBaseURL
	jikanBaseURL = url
	t.Cleanup(func() { jikanBaseURL = old })
}

func seedJikanCache(t *testing.T, dir string, c jikanCacheFile) {
	t.Helper()
	raw, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, jikanCacheFileName), raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func freshEntry() jikanEpisodeCacheEntry {
	return jikanEpisodeCacheEntry{
		TitleEnglish: "JoJo's Bizarre Adventure",
		IsFiller:     true,
		IsRecap:      false,
		FetchedAt:    time.Now(),
	}
}

// A fresh cached episode must be applied with zero network traffic: point
// the base URL at an unroutable address so any request would fail loudly.
func TestJikanEpisodeCacheHitAvoidsNetwork(t *testing.T) {
	dir := t.TempDir()
	withJikanTestConfig(t, dir)
	withJikanBaseURL(t, "http://127.0.0.1:1")
	seedJikanCache(t, dir, jikanCacheFile{
		Episodes: map[string]jikanEpisodeCacheEntry{
			"11757:5": freshEntry(),
		},
	})

	anime := &Anime{}
	start := time.Now()
	if err := GetEpisodeData(11757, 5, anime); err != nil {
		t.Fatalf("cached lookup must not error: %v", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatal("cached lookup hit the network")
	}
	if anime.Ep.Title.English != "JoJo's Bizarre Adventure" || !anime.Ep.IsFiller {
		t.Fatalf("cached values not applied: %+v", anime.Ep)
	}
}

// An expired entry must be refetched and the fresh result stored.
func TestJikanEpisodeCacheExpiredRefetches(t *testing.T) {
	var calls int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"title":"Refetched","filler":false,"recap":true}}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	withJikanTestConfig(t, dir)
	withJikanBaseURL(t, srv.URL)
	stale := freshEntry()
	stale.FetchedAt = time.Now().Add(-30 * 24 * time.Hour)
	seedJikanCache(t, dir, jikanCacheFile{Episodes: map[string]jikanEpisodeCacheEntry{"1:1": stale}})

	anime := &Anime{}
	if err := GetEpisodeData(1, 1, anime); err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt64(&calls) != 1 {
		t.Fatalf("expected one refetch, got %d", calls)
	}
	if anime.Ep.Title.English != "Refetched" || !anime.Ep.IsRecap || anime.Ep.IsFiller {
		t.Fatalf("fresh values not applied: %+v", anime.Ep)
	}
	stored := loadJikanCacheFile(filepath.Join(dir, jikanCacheFileName))
	if got, ok := stored.Episodes["1:1"]; !ok || !jikanEntryFresh(got.FetchedAt, time.Now()) {
		t.Fatal("refetch was not stored fresh")
	}
}

// When Jikan is unreachable, a stale entry still beats zero-value defaults.
func TestJikanStaleFallbackOnNetworkFailure(t *testing.T) {
	dir := t.TempDir()
	withJikanTestConfig(t, dir)
	withJikanBaseURL(t, "http://127.0.0.1:1")
	stale := freshEntry()
	stale.FetchedAt = time.Now().Add(-30 * 24 * time.Hour)
	seedJikanCache(t, dir, jikanCacheFile{Episodes: map[string]jikanEpisodeCacheEntry{"2:3": stale}})

	anime := &Anime{}
	if err := GetEpisodeData(2, 3, anime); err != nil {
		t.Fatalf("stale fallback must not error: %v", err)
	}
	if !anime.Ep.IsFiller {
		t.Fatal("stale filler flag not applied")
	}
}

// A failed lookup suppresses network retries for an hour: the second call
// during an outage must be instant and still resolve to defaults.
func TestJikanNegativeCacheShortCircuits(t *testing.T) {
	dir := t.TempDir()
	withJikanTestConfig(t, dir)
	withJikanBaseURL(t, "http://127.0.0.1:1")

	anime := &Anime{}
	if err := GetEpisodeData(77, 1, anime); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	again := &Anime{}
	if err := GetEpisodeData(77, 1, again); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatal("negative cache did not short-circuit the retry")
	}
	stored := loadJikanCacheFile(filepath.Join(dir, jikanCacheFileName))
	if _, ok := stored.Negative[jikanNegativeKey("ep", "77:1")]; !ok {
		t.Fatal("failure was not recorded")
	}
}

// An expired negative entry re-probes the network instead of suppressing.
func TestJikanNegativeCacheExpiredReprobes(t *testing.T) {
	var calls int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"title":"Back","filler":false,"recap":false}}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	withJikanTestConfig(t, dir)
	withJikanBaseURL(t, srv.URL)
	past := time.Now().Add(-2 * time.Hour)
	seedJikanCache(t, dir, jikanCacheFile{
		Negative: map[string]time.Time{jikanNegativeKey("ep", "5:5"): past},
	})

	if err := GetEpisodeData(5, 5, &Anime{}); err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt64(&calls) != 1 {
		t.Fatalf("expected a re-probe after expiry, got %d calls", calls)
	}
}

// Corrupt cache files must never break lookups.
func TestJikanCorruptCacheTolerated(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, jikanCacheFileName), []byte("{oops"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := loadJikanCacheFile(filepath.Join(dir, jikanCacheFileName))
	if len(c.Episodes) != 0 || len(c.Filler) != 0 {
		t.Fatal("corrupt cache must load empty")
	}
}

// A fresh cached filler list must be returned with zero network traffic.
func TestFillerCacheHitAvoidsNetwork(t *testing.T) {
	dir := t.TempDir()
	withJikanTestConfig(t, dir)
	withJikanBaseURL(t, "http://127.0.0.1:1")
	seedJikanCache(t, dir, jikanCacheFile{
		Filler: map[string]jikanFillerCacheEntry{
			"11757": {Episodes: []int{4, 9}, FetchedAt: time.Now()},
		},
	})
	got, err := FetchFillerEpisodes(11757)
	if err != nil {
		t.Fatalf("cached filler must not error: %v", err)
	}
	if len(got) != 2 || got[0] != 4 || got[1] != 9 {
		t.Fatalf("wrong cached filler: %v", got)
	}
}

// Persistent 429s must fail bounded instead of retrying forever (the old
// loop never gave up, cementing the rate limit it was hitting).
func TestFiller429RetryBounded(t *testing.T) {
	var calls int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&calls, 1)
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	oldMax, oldDelay := jikanFillerMaxRetries, jikanRetryDelay
	jikanFillerMaxRetries, jikanRetryDelay = 2, time.Millisecond
	defer func() { jikanFillerMaxRetries, jikanRetryDelay = oldMax, oldDelay }()

	dir := t.TempDir()
	withJikanTestConfig(t, dir)
	withJikanBaseURL(t, srv.URL)
	start := time.Now()
	_, err := FetchFillerEpisodes(999001)
	if err == nil {
		t.Fatal("expected rate-limit error")
	}
	if n := atomic.LoadInt64(&calls); n != 3 {
		t.Fatalf("expected 3 attempts (1 + 2 retries), got %d", n)
	}
	if time.Since(start) > 30*time.Second {
		t.Fatal("retry loop took too long")
	}
}
