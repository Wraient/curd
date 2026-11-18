package anilight

import (
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/wraient/curd/internal/curdhost"
	"github.com/wraient/curd/internal/providers"
)

// TestLiveAnilightProviderFlow runs search -> episodes -> sub -> dub
// against the real anilight API and probes the resolved mp4 down to a few
// bytes.
//
//	CURD_LIVE_ANILIGHT=1 go test ./internal/providers/anilight -run TestLiveAnilightProviderFlow -v -count=1 -timeout 10m
func TestLiveAnilightProviderFlow(t *testing.T) {
	if os.Getenv("CURD_LIVE_ANILIGHT") == "" {
		t.Skip("set CURD_LIVE_ANILIGHT=1 to run the live anilight flow")
	}
	previous := curdhost.HTTPClient
	curdhost.HTTPClient = func() *http.Client { return http.DefaultClient }
	t.Cleanup(func() { curdhost.HTTPClient = previous })

	provider := &Provider{}

	results, err := provider.SearchAnime("frieren", "sub")
	if err != nil || len(results) == 0 {
		t.Fatalf("search: results=%d err=%v", len(results), err)
	}
	var showID string
	for _, result := range results {
		if strings.Contains(strings.ToLower(result.Title), "frieren") {
			showID = result.Key
			break
		}
	}
	if showID == "" {
		t.Fatalf("Frieren not found in %#v", results)
	}

	episodes, err := provider.EpisodesList(showID, "sub")
	if err != nil || len(episodes) == 0 {
		t.Fatalf("episodes: n=%d err=%v", len(episodes), err)
	}
	t.Logf("sub episodes: %d", len(episodes))

	for _, mode := range []string{"sub", "dub"} {
		style := "soft"
		if mode == "dub" {
			style = ""
		}
		links, hints, err := provider.GetEpisodeURLForModeWithHints(providers.PlaybackConfig{SubOrDub: mode, SubStyle: style}, showID, 1, mode)
		if err != nil {
			t.Fatalf("streams %s: %v", mode, err)
		}
		if len(links) == 0 {
			t.Fatalf("no %s stream links", mode)
		}
		probeLiveMedia(t, links[0], hints[links[0]].Referrer)
		t.Logf("stream %s ok hasSubtitle=%v", mode, hints[links[0]].Subtitle != "")
	}
}

func probeLiveMedia(t *testing.T, mediaURL, referer string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, mediaURL, nil)
	if err != nil {
		t.Fatalf("probe url: %v", err)
	}
	req.Header.Set("User-Agent", userAgent)
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	req.Header.Set("Range", "bytes=0-16383")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("probe fetch: %v", err)
	}
	defer resp.Body.Close()
	head := make([]byte, 8)
	if _, err := io.ReadFull(resp.Body, head); err != nil {
		t.Fatalf("probe read: %v", err)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		t.Fatalf("probe status %d", resp.StatusCode)
	}
}
