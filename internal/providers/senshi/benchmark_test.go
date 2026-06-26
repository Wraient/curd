package senshi

import (
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/wraient/curd/internal/curdhost"
	"github.com/wraient/curd/internal/providers"
)

// Benchmark live senshi vs anineko APIs. Run with:
//
//	CURD_LIVE_BENCH=1 go test ./internal/providers/senshi -run '^$' -bench Benchmark -benchtime=3x
func BenchmarkSenshiSearch(b *testing.B) {
	if os.Getenv("CURD_LIVE_BENCH") == "" {
		b.Skip("set CURD_LIVE_BENCH=1 to run live benchmarks")
	}
	if curdhost.HTTPClient == nil {
		curdhost.HTTPClient = func() *http.Client { return http.DefaultClient }
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := searchAnime("frieren", "sub"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSenshiEpisodeStream(b *testing.B) {
	if os.Getenv("CURD_LIVE_BENCH") == "" {
		b.Skip("set CURD_LIVE_BENCH=1 to run live benchmarks")
	}
	if curdhost.HTTPClient == nil {
		curdhost.HTTPClient = func() *http.Client { return http.DefaultClient }
	}
	const malID = "52991"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := episodesList(malID, "sub"); err != nil {
			b.Fatal(err)
		}
		if _, _, err := getEpisodeStreamsForMode(malID, providers.PlaybackConfig{SubOrDub: "sub"}, 1); err != nil {
			b.Fatal(err)
		}
	}
}

func TestLiveBenchmarkSenshiFasterThanAninekoSearch(t *testing.T) {
	if os.Getenv("CURD_LIVE_BENCH") == "" {
		t.Skip("set CURD_LIVE_BENCH=1 to run live benchmarks")
	}
	if curdhost.HTTPClient == nil {
		curdhost.HTTPClient = func() *http.Client { return http.DefaultClient }
	}

	senshiStart := time.Now()
	if _, err := searchAnime("frieren", "sub"); err != nil {
		t.Fatalf("senshi search: %v", err)
	}
	senshiSearch := time.Since(senshiStart)

	aninekoStart := time.Now()
	req, err := http.NewRequest(http.MethodGet, "https://anineko.to/ajax/search?q=frieren", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Referer", "https://anineko.to/")
	resp, err := curdhost.HTTPClient().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if !curdhost.HTTPStatusOK(resp.StatusCode) {
		t.Fatalf("anineko search status %d", resp.StatusCode)
	}
	aninekoSearch := time.Since(aninekoStart)

	t.Logf("search latency senshi=%s anineko=%s", senshiSearch, aninekoSearch)
	if senshiSearch > aninekoSearch*2 {
		t.Fatalf("expected senshi search to be competitive, senshi=%s anineko=%s", senshiSearch, aninekoSearch)
	}
}
