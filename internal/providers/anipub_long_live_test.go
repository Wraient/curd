package providers_test

import (
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/wraient/curd/internal/curdhost"
)

// Run AniPub edge cases in isolation with generous pacing to avoid rate limits:
//
//	CURD_LIVE_ANIPUB=1 go test ./internal/providers -run TestLiveAnipubLong -v -timeout 60m
//
// Optional: CURD_LIVE_ANIPUB_DELAY_MS=2500 (default 2000)
func initLiveAnipub(tb testing.TB) {
	tb.Helper()
	if os.Getenv("CURD_LIVE_ANIPUB") == "" {
		tb.Skip("set CURD_LIVE_ANIPUB=1 to run isolated long AniPub live tests")
	}
	if curdhost.HTTPClient == nil {
		curdhost.HTTPClient = func() *http.Client { return http.DefaultClient }
	}
}

func anipubLiveDelay() time.Duration {
	ms := 2000
	if raw := os.Getenv("CURD_LIVE_ANIPUB_DELAY_MS"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			ms = parsed
		}
	}
	return time.Duration(ms) * time.Millisecond
}

func TestLiveAnipubLong(t *testing.T) {
	initLiveAnipub(t)

	delay := anipubLiveDelay()
	t.Logf("anipub long run: %d cases, %v delay between requests", len(edgeAnimeCases), delay)

	var anipub edgeProvider
	for _, p := range edgeProviders() {
		if p.name == "anipub" {
			anipub = p
			break
		}
	}
	if anipub.name == "" {
		t.Fatal("anipub provider not found")
	}

	type counts struct{ search, match, episodes, stream, total int }
	var c counts

	for i, tc := range edgeAnimeCases {
		res := runEdgeCase(anipub, tc)
		c.total++
		if res.searchOK {
			c.search++
		}
		if res.matchOK {
			c.match++
		}
		if res.episodesOK {
			c.episodes++
		}
		if res.streamOK {
			c.stream++
		}
		t.Logf("[%d/%d] %s", i+1, len(edgeAnimeCases), edgeStatusLine(res))
		if i < len(edgeAnimeCases)-1 {
			time.Sleep(delay)
		}
	}

	t.Logf("anipub long: search %d/%d match %d/%d episodes %d/%d stream %d/%d",
		c.search, c.total, c.match, c.total, c.episodes, c.total, c.stream, c.total)
}
