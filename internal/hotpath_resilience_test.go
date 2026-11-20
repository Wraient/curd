package internal

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// These pin the rule: optional metadata services (Jikan, AniList ID lookups,
// cover art, tracker writes) never gate playback or the picker, and never
// lose or reorder user data. Each test uses a server that hangs or fails the
// way the real upstreams do.

// hangingServer blocks every request until the test ends.
func hangingServer(t *testing.T, hits *int64) *httptest.Server {
	t.Helper()
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(hits, 1)
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(func() { close(release); srv.Close() })
	return srv
}

// drainJikanLookups registers a cleanup (runs first, LIFO) that waits for
// background lookups started by a time-boxed call, so restoring test
// globals never races a request still in flight.
func drainJikanLookups(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			jikanInflightMu.Lock()
			n := len(jikanInflight)
			jikanInflightMu.Unlock()
			if n == 0 {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Error("background Jikan lookup never finished")
	})
}

func withJikanTimeouts(t *testing.T, request, budget time.Duration) {
	t.Helper()
	oldReq, oldBudget := jikanRequestTimeout, jikanPrePlayBudget
	jikanRequestTimeout, jikanPrePlayBudget = request, budget
	t.Cleanup(func() { jikanRequestTimeout, jikanPrePlayBudget = oldReq, oldBudget })
}

// The JoJo bug: Jikan hangs, and episode start waited the full timeout.
// The pre-play check must return within its budget, playing with defaults.
func TestPrePlayEpisodeDataRespectsBudget(t *testing.T) {
	var hits int64
	srv := hangingServer(t, &hits)
	dir := t.TempDir()
	withJikanTestConfig(t, dir)
	withJikanBaseURL(t, srv.URL)
	withJikanTimeouts(t, 600*time.Millisecond, 300*time.Millisecond)
	drainJikanLookups(t)

	anime := &Anime{}
	anime.Ep.IsFiller = true // stale flag from a previous episode
	start := time.Now()
	applied := GetEpisodeDataWithin(14719, 5, anime, jikanPrePlayBudget)
	elapsed := time.Since(start)
	if applied {
		t.Fatal("hanging Jikan must not report applied data")
	}
	if elapsed > time.Second {
		t.Fatalf("pre-play check took %s, budget 300ms", elapsed)
	}
	if anime.Ep.IsFiller || anime.Ep.IsRecap {
		t.Fatal("timed-out check must reset filler/recap to defaults, not keep stale flags")
	}
}

// A second lookup for the same episode while the first is in flight must
// join it, not fire a second request (pre-play + in-episode checks).
func TestJikanEpisodeLookupDeduplicates(t *testing.T) {
	var hits int64
	gate := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
		<-gate
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"title":"Ep","filler":true,"recap":false}}`))
	}))
	defer srv.Close()
	dir := t.TempDir()
	withJikanTestConfig(t, dir)
	withJikanBaseURL(t, srv.URL)

	var wg sync.WaitGroup
	results := make([]bool, 4)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, results[i] = lookupJikanEpisode(9, 2)
		}(i)
	}
	time.Sleep(150 * time.Millisecond)
	close(gate)
	wg.Wait()
	if got := atomic.LoadInt64(&hits); got != 1 {
		t.Fatalf("expected 1 upstream request, got %d", got)
	}
	for i, ok := range results {
		if !ok {
			t.Fatalf("caller %d did not get the shared result", i)
		}
	}
}

// A timed-out pre-play check keeps fetching in the background; the late
// answer must land in the cache so the next start is instant.
func TestPrePlayLateAnswerStillCached(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(400 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"title":"Late","filler":true,"recap":false}}`))
	}))
	defer srv.Close()
	dir := t.TempDir()
	withJikanTestConfig(t, dir)
	withJikanBaseURL(t, srv.URL)

	drainJikanLookups(t)
	if GetEpisodeDataWithin(3, 4, &Anime{}, 50*time.Millisecond) {
		t.Fatal("should have timed out")
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		c := loadJikanCacheFile(filepath.Join(dir, jikanCacheFileName))
		if e, ok := c.Episodes["3:4"]; ok && e.IsFiller {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("late Jikan answer never reached the cache")
}

// One Jikan timeout opens the breaker: every other Jikan call (other
// episodes, filler list, pictures for matching) fails instantly with zero
// requests — and the state survives a relaunch.
func TestJikanHostBreakerShortCircuitsAllCalls(t *testing.T) {
	var hits int64
	srv := hangingServer(t, &hits)
	dir := t.TempDir()
	withJikanTestConfig(t, dir)
	withJikanBaseURL(t, srv.URL)
	withJikanTimeouts(t, 200*time.Millisecond, 2*time.Second)

	if _, ok := lookupJikanEpisode(14719, 5); ok {
		t.Fatal("hanging server cannot yield data")
	}
	if atomic.LoadInt64(&hits) != 1 {
		t.Fatalf("expected the first probe to hit once, got %d", hits)
	}

	start := time.Now()
	_, _ = lookupJikanEpisode(14719, 6)
	_, _ = FetchFillerEpisodes(14719)
	if _, err := FetchJikanPictures(14719); err == nil {
		t.Fatal("pictures should fail while Jikan is down")
	}
	if _, err := FetchJikanAnimeData(14719); err == nil {
		t.Fatal("anime data should fail while Jikan is down")
	}
	if d := time.Since(start); d > 100*time.Millisecond {
		t.Fatalf("breaker-open calls took %s", d)
	}
	if got := atomic.LoadInt64(&hits); got != 1 {
		t.Fatalf("breaker open but %d requests reached Jikan", got)
	}

	// Simulate a relaunch: in-memory state gone, persisted state remains.
	jikanHostState.mu.Lock()
	jikanHostState.loaded = false
	jikanHostState.mu.Unlock()
	if !jikanHostDown(filepath.Join(dir, jikanCacheFileName), time.Now()) {
		t.Fatal("outage state not persisted across launches")
	}
}

// The breaker closes after its TTL and on the first success.
func TestJikanHostBreakerRecovers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"title":"Back","filler":false,"recap":false}}`))
	}))
	defer srv.Close()
	dir := t.TempDir()
	withJikanTestConfig(t, dir)
	withJikanBaseURL(t, srv.URL)
	path := filepath.Join(dir, jikanCacheFileName)

	markJikanHostDown(path, time.Now().Add(-jikanHostDownTTL-time.Minute))
	if jikanHostDown(path, time.Now()) {
		t.Fatal("expired outage must not block")
	}
	if _, ok := lookupJikanEpisode(5, 1); !ok {
		t.Fatal("recovered Jikan should serve data")
	}
	if _, stillDown := loadJikanCacheFile(path).Negative[jikanHostNegativeKey]; stillDown {
		t.Fatal("success must clear the persisted outage")
	}
}

// 5xx (Cloudflare 502/503/522 in front of Jikan) opens the breaker too;
// a 404 (episode not listed) must not.
func TestJikanBreakerOnlyOpensForOutages(t *testing.T) {
	status := int64(404)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(int(atomic.LoadInt64(&status)))
	}))
	defer srv.Close()
	dir := t.TempDir()
	withJikanTestConfig(t, dir)
	withJikanBaseURL(t, srv.URL)
	path := filepath.Join(dir, jikanCacheFileName)

	_, _ = makeGetRequest(srv.URL+"/x", nil)
	if jikanHostDown(path, time.Now()) {
		t.Fatal("404 must not open the outage breaker")
	}
	atomic.StoreInt64(&status, 429)
	_, _ = makeGetRequest(srv.URL+"/x", nil)
	if jikanHostDown(path, time.Now()) {
		t.Fatal("429 must not open the outage breaker")
	}
	atomic.StoreInt64(&status, 503)
	_, _ = makeGetRequest(srv.URL+"/x", nil)
	if !jikanHostDown(path, time.Now()) {
		t.Fatal("503 must open the outage breaker")
	}
}

func withProgressUpdater(t *testing.T, fn func(token string, mediaID, progress int) error) {
	t.Helper()
	if !FlushProgressQueue(5 * time.Second) {
		t.Fatal("progress queue busy from a previous test")
	}
	old := progressUpdater
	oldRemote := remoteProgressFor
	progressUpdater = fn
	remoteProgressFor = func(int) (int, bool, bool) { return 0, false, false }
	t.Cleanup(func() {
		FlushProgressQueue(5 * time.Second)
		progressUpdater = old
		remoteProgressFor = oldRemote
	})
}

// Writes run in submission order, one at a time: ep 5 can never land
// after ep 6 and leave the tracker behind.
func TestProgressQueuePreservesOrder(t *testing.T) {
	var mu sync.Mutex
	var got []int
	var inflight, maxInflight int64
	withProgressUpdater(t, func(_ string, _ int, p int) error {
		n := atomic.AddInt64(&inflight, 1)
		for {
			m := atomic.LoadInt64(&maxInflight)
			if n <= m || atomic.CompareAndSwapInt64(&maxInflight, m, n) {
				break
			}
		}
		time.Sleep(time.Duration(10-p) * 3 * time.Millisecond) // early eps slower
		mu.Lock()
		got = append(got, p)
		mu.Unlock()
		atomic.AddInt64(&inflight, -1)
		return nil
	})
	for p := 1; p <= 8; p++ {
		QueueAnimeProgress("tok", 42, p, nil)
	}
	if !FlushProgressQueue(5 * time.Second) {
		t.Fatal("queue did not drain")
	}
	for i, p := range got {
		if p != i+1 {
			t.Fatalf("writes reordered: %v", got)
		}
	}
	if len(got) != 8 || maxInflight != 1 {
		t.Fatalf("got %v with max concurrency %d", got, maxInflight)
	}
}

// The lost-write bug: exit used to kill in-flight writes. Flush must wait
// for them; a stuck tracker must not hang exit past the timeout.
func TestProgressQueueFlushWaitsThenGivesUp(t *testing.T) {
	var done int64
	withProgressUpdater(t, func(string, int, int) error {
		time.Sleep(100 * time.Millisecond)
		atomic.AddInt64(&done, 1)
		return nil
	})
	QueueAnimeProgress("tok", 1, 3, nil)
	if PendingProgressWrites() == 0 {
		t.Fatal("pending count missed the queued write")
	}
	if !FlushProgressQueue(2 * time.Second) {
		t.Fatal("flush gave up on a healthy write")
	}
	if atomic.LoadInt64(&done) != 1 {
		t.Fatal("flush returned before the write finished")
	}

	block := make(chan struct{})
	progressUpdater = func(string, int, int) error { <-block; return nil }
	QueueAnimeProgress("tok", 1, 4, nil)
	start := time.Now()
	if FlushProgressQueue(150 * time.Millisecond) {
		t.Fatal("flush claimed success on a stuck tracker")
	}
	if time.Since(start) > time.Second {
		t.Fatal("flush overran its timeout")
	}
	close(block)
}

// A synchronous (ordered) write waits behind queued ones and returns its
// own error; a panicking updater must not kill the worker.
func TestProgressOrderedWriteAndPanicSafety(t *testing.T) {
	var mu sync.Mutex
	var got []int
	withProgressUpdater(t, func(_ string, _ int, p int) error {
		if p == 2 {
			panic("tracker client bug")
		}
		time.Sleep(20 * time.Millisecond)
		mu.Lock()
		got = append(got, p)
		mu.Unlock()
		if p == 3 {
			return errors.New("tracker said no")
		}
		return nil
	})
	QueueAnimeProgress("tok", 7, 1, nil)
	QueueAnimeProgress("tok", 7, 2, nil)
	err := UpdateAnimeProgressOrdered("tok", 7, 3, 2*time.Second)
	if err == nil || err.Error() != "tracker said no" {
		t.Fatalf("ordered write lost its result: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if fmt.Sprint(got) != "[1 3]" {
		t.Fatalf("ordered write overtook queued writes or worker died: %v", got)
	}
}

// AniList answers 429 with Retry-After: 60; four obeyed retries stalled
// callers for minutes.
func TestAniListRetryDelayCapped(t *testing.T) {
	resp := &http.Response{Header: http.Header{"Retry-After": []string{"60"}}}
	if d := aniListRetryDelay(resp, 0); d > aniListMaxRetryDelay {
		t.Fatalf("Retry-After not capped: %s", d)
	}
	if d := aniListRetryDelay(nil, 10); d > aniListMaxRetryDelay {
		t.Fatalf("linear backoff not capped: %s", d)
	}
}

type failTransport struct{ t *testing.T }

func (f failTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	f.t.Errorf("unexpected network request to %s", r.URL)
	return nil, errors.New("network disabled in test")
}

// MAL ID + cover were re-fetched from AniList before every episode; a
// memoized show must not touch the network.
func TestAniListMediaLookupsMemoized(t *testing.T) {
	old := sharedHTTPClient
	sharedHTTPClient = &http.Client{Transport: failTransport{t}}
	t.Cleanup(func() { sharedHTTPClient = old })

	memoAniListMedia(999001, 555, "https://img/x.jpg")
	mal, err := GetAnimeMalID(999001)
	if err != nil || mal != 555 {
		t.Fatalf("GetAnimeMalID = %d, %v", mal, err)
	}
	mal, cover, err := GetAnimeIDAndImage(999001)
	if err != nil || mal != 555 || cover != "https://img/x.jpg" {
		t.Fatalf("GetAnimeIDAndImage = %d %q %v", mal, cover, err)
	}
	if _, err := GetAnimeMalID(0); err == nil {
		t.Fatal("invalid ID must fail fast without a request")
	}
}

// The picker used to download every cover sequentially before opening and
// dropped shows whose cover failed. Now: bounded wait, partial results.
func TestCoverCacheWithinBudget(t *testing.T) {
	// Not t.TempDir: the hanging download legitimately finishes in the
	// background after the budget, and may write after the test returns.
	home, err := os.MkdirTemp("", "curd-cover-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		time.Sleep(300 * time.Millisecond)
		_ = os.RemoveAll(home)
	})
	t.Setenv("HOME", home)
	var hangHits int64
	hang := hangingServer(t, &hangHits)
	fast := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("JPEGDATA" + r.URL.Path))
	}))
	defer fast.Close()

	urls := []string{fast.URL + "/a.jpg", hang.URL + "/slow.jpg", fast.URL + "/b.jpg", "", fast.URL + "/a.jpg"}
	start := time.Now()
	got := cacheImagesWithin(urls, 400*time.Millisecond)
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("cover wait took %s", d)
	}
	for _, u := range []string{fast.URL + "/a.jpg", fast.URL + "/b.jpg"} {
		path := got[u]
		if path == "" {
			t.Fatalf("fast cover %s missing", u)
		}
		raw, err := os.ReadFile(path)
		if err != nil || string(raw) != "JPEGDATA"+u[len(fast.URL):] {
			t.Fatalf("cover %s content %q err %v", u, raw, err)
		}
	}
	if got[hang.URL+"/slow.jpg"] != "" {
		t.Fatal("hanging cover cannot be ready")
	}
}

// Concurrent downloads of one cover (pre-download + menu) must not
// interleave bytes in the cache file.
func TestCoverConcurrentDownloadsAtomic(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	payload := make([]byte, 256<<10)
	for i := range payload {
		payload[i] = byte(i % 251)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for off := 0; off < len(payload); off += 8 << 10 {
			_, _ = w.Write(payload[off : off+8<<10])
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
	}))
	defer srv.Close()

	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = downloadToCache(srv.URL + "/c.jpg") }()
	}
	wg.Wait()
	path, err := downloadToCache(srv.URL + "/c.jpg")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if len(raw) != len(payload) {
		t.Fatalf("cover corrupted: %d bytes, want %d", len(raw), len(payload))
	}
	for i := range raw {
		if raw[i] != payload[i] {
			t.Fatalf("cover corrupted at byte %d", i)
		}
	}
	leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(path), "*.part"))
	if len(leftovers) != 0 {
		t.Fatalf("temp files left behind: %v", leftovers)
	}
}

// Audit finding: progress writes (mutations) run off the playback path, so
// they must honor AniList's full Retry-After — capping them retried inside
// the same rate-limit window and dropped the write. Reads stay capped.
func TestAniListMutationsHonorRetryAfter(t *testing.T) {
	resp := &http.Response{Header: http.Header{"Retry-After": []string{"60"}}}
	if d := aniListRetryDelayFor(resp, 0, true); d != 60*time.Second {
		t.Fatalf("mutation back-off = %s, want 60s", d)
	}
	if d := aniListRetryDelayFor(resp, 0, false); d != aniListMaxRetryDelay {
		t.Fatalf("read back-off = %s, want cap %s", d, aniListMaxRetryDelay)
	}
	if !isGraphQLMutation("\n\tmutation ($mediaId: Int) {") || isGraphQLMutation("query { Viewer { id } }") {
		t.Fatal("mutation detection wrong")
	}
}

// Audit finding: an automatic completion write must never move the tracker
// backwards (list refreshed late / watched elsewhere); an explicit user
// choice may, and rewatch progress may restart low.
func TestProgressQueueMonotonicGuard(t *testing.T) {
	var mu sync.Mutex
	var wrote []int
	withProgressUpdater(t, func(_ string, _ int, p int) error {
		mu.Lock()
		wrote = append(wrote, p)
		mu.Unlock()
		return nil
	})
	repeating := false
	remoteProgressFor = func(int) (int, bool, bool) { return 10, repeating, true }

	QueueAnimeProgress("tok", 5, 7, nil)      // behind tracker: skipped
	QueueAnimeProgress("tok", 5, 11, nil)     // ahead: written
	QueueAnimeProgressExact("tok", 5, 3, nil) // explicit: written
	FlushProgressQueue(2 * time.Second)
	repeating = true
	QueueAnimeProgress("tok", 5, 2, nil) // rewatch: written
	FlushProgressQueue(2 * time.Second)

	mu.Lock()
	defer mu.Unlock()
	if fmt.Sprint(wrote) != "[11 3 2]" {
		t.Fatalf("writes = %v, want [11 3 2]", wrote)
	}
}

// Audit finding: invalid jobs must report back instead of leaving an
// ordered write waiting out its full timeout.
func TestProgressInvalidJobReportsImmediately(t *testing.T) {
	withProgressUpdater(t, func(string, int, int) error { return nil })
	start := time.Now()
	if err := UpdateAnimeProgressOrdered("tok", 0, 3, 5*time.Second); err == nil {
		t.Fatal("invalid media id must error")
	}
	if time.Since(start) > time.Second {
		t.Fatal("invalid job waited for the timeout")
	}
}

// A rate-limited write is retried by the worker, not dropped.
func TestProgressRateLimitRetried(t *testing.T) {
	var calls int64
	withProgressUpdater(t, func(string, int, int) error {
		if atomic.AddInt64(&calls, 1) < 3 {
			return errors.New("failed with status 429: slow down")
		}
		return nil
	})
	old := progressRateLimitDelay
	progressRateLimitDelay = 10 * time.Millisecond
	t.Cleanup(func() { progressRateLimitDelay = old })
	if err := UpdateAnimeProgressOrdered("tok", 9, 4, 3*time.Second); err != nil {
		t.Fatalf("rate-limited write not retried to success: %v", err)
	}
	if atomic.LoadInt64(&calls) != 3 {
		t.Fatalf("calls = %d, want 3", calls)
	}
}

// Audit finding: a 429 is transient — it must not negative-cache the
// episode for an hour (hiding its recap flag).
func TestJikan429NotNegativeCached(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	dir := t.TempDir()
	withJikanTestConfig(t, dir)
	withJikanBaseURL(t, srv.URL)

	_, _ = lookupJikanEpisode(31, 2)
	c := loadJikanCacheFile(filepath.Join(dir, jikanCacheFileName))
	if _, neg := c.Negative[jikanNegativeKey("ep", "31:2")]; neg {
		t.Fatal("429 was negative-cached")
	}
}

// Audit finding: cache writes are atomic and prune expired negatives.
func TestJikanCacheSaveAtomicAndPruned(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, jikanCacheFileName)
	c := loadJikanCacheFile("")
	c.Negative["ep:old"] = time.Now().Add(-3 * time.Hour)
	c.Negative["ep:new"] = time.Now()
	saveJikanCacheFile(path, c)
	got := loadJikanCacheFile(path)
	if _, ok := got.Negative["ep:old"]; ok {
		t.Fatal("expired negative entry not pruned")
	}
	if _, ok := got.Negative["ep:new"]; !ok {
		t.Fatal("live negative entry lost")
	}
	if tmps, _ := filepath.Glob(filepath.Join(dir, "*.tmp")); len(tmps) != 0 {
		t.Fatalf("temp files left: %v", tmps)
	}
}

// Audit finding: once shutdown begins, entry points that would launch a
// new episode park instead of racing os.Exit.
func TestExitInProgressParksPlayback(t *testing.T) {
	if ExitInProgress() {
		t.Fatal("flag set before any exit")
	}
	processExiting.Store(true)
	t.Cleanup(func() { processExiting.Store(false) })
	returned := make(chan struct{})
	go func() {
		_, _ = StartVideo("http://example.invalid/x.m3u8", nil, "t", &Anime{})
		close(returned)
	}()
	select {
	case <-returned:
		t.Fatal("StartVideo launched during shutdown")
	case <-time.After(200 * time.Millisecond):
	}
}

// Re-audit finding A: a MAL 429 must be recognizable as a rate limit so
// the progress worker retries instead of dropping the write.
func TestMyAnimeList429IsRateLimitError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":"slow_down"}`))
	}))
	defer srv.Close()
	dir := t.TempDir()
	tokenJSON := fmt.Sprintf(`{"access_token":"a","refresh_token":"r","token_type":"Bearer","expires_at":%q}`, time.Now().Add(time.Hour).Format(time.RFC3339))
	cfg := &CurdConfig{StoragePath: dir, TrackingRemote: "myanimelist", MyAnimeListClientID: "x"}
	if err := os.WriteFile(myAnimeListTokenPath(cfg), []byte(tokenJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	err := myAnimeListRequest(cfg, http.MethodGet, srv.URL+"/x", nil, nil)
	if err == nil || !isRateLimitError(err) {
		t.Fatalf("MAL 429 not recognized as rate limit: %v", err)
	}
}
