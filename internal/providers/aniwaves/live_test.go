package aniwaves

import (
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/wraient/curd/internal/curdhost"
	"github.com/wraient/curd/internal/providers"
)

// TestLiveAniwavesProviderFlow runs search -> episodes -> sub -> dub against
// the real aniwaves ajax API and vhscdn embeds, and probes the resolved HLS
// playlist.
//
//	CURD_LIVE_ANIWAVES=1 go test ./internal/providers/aniwaves -run TestLiveAniwavesProviderFlow -v -count=1 -timeout 10m
func TestLiveAniwavesProviderFlow(t *testing.T) {
	if os.Getenv("CURD_LIVE_ANIWAVES") == "" {
		t.Skip("set CURD_LIVE_ANIWAVES=1 to run the live aniwaves flow")
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
		links, hints, err := provider.GetEpisodeURLForModeWithHints(providers.PlaybackConfig{SubOrDub: mode, SubStyle: "soft"}, showID, 1, mode)
		if err != nil {
			t.Fatalf("streams %s: %v", mode, err)
		}
		if len(links) == 0 {
			t.Fatalf("no %s stream links", mode)
		}
		probeLivePlaylist(t, links[0], hints[links[0]].Referrer)
		t.Logf("stream %s ok", mode)
	}
}

func probeLivePlaylist(t *testing.T, masterURL, referer string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, masterURL, nil)
	if err != nil {
		t.Fatalf("probe url: %v", err)
	}
	req.Header.Set("User-Agent", userAgent)
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("probe fetch: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		t.Fatalf("probe read: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("probe status %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), "#EXTM3U") {
		t.Fatalf("master is not HLS: %.120q", body)
	}
}
