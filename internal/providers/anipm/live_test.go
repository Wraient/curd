package anipm

import (
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/wraient/curd/internal/curdhost"
	"github.com/wraient/curd/internal/providers"
)

// TestLiveAnipmProviderFlow runs search -> episodes -> sub(soft) ->
// sub(hard) -> dub against the real ani.pm / Settlar services and probes
// the resolved master playlist down to a media segment.
//
//	CURD_LIVE_ANIPM=1 go test ./internal/providers/anipm -run TestLiveAnipmProviderFlow -v -count=1 -timeout 10m
func TestLiveAnipmProviderFlow(t *testing.T) {
	if os.Getenv("CURD_LIVE_ANIPM") == "" {
		t.Skip("set CURD_LIVE_ANIPM=1 to run the live ani.pm flow")
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
		title := strings.ReplaceAll(result.Title, "’", "'")
		if strings.EqualFold(title, "Frieren: Beyond Journey's End") {
			showID = result.Key
			break
		}
	}
	if showID == "" {
		t.Fatalf("Frieren not found in %#v", results)
	}

	for _, mode := range []string{"sub", "dub"} {
		episodes, err := provider.EpisodesList(showID, mode)
		if err != nil || len(episodes) == 0 {
			t.Fatalf("episodes %s: n=%d err=%v", mode, len(episodes), err)
		}
		t.Logf("%s episodes: %d", mode, len(episodes))
	}

	cases := []struct {
		name    string
		config  providers.PlaybackConfig
		wantSub bool
	}{
		{name: "sub_soft", config: providers.PlaybackConfig{SubOrDub: "sub", SubStyle: "soft"}, wantSub: true},
		{name: "sub_hard", config: providers.PlaybackConfig{SubOrDub: "sub", SubStyle: "hard"}},
		{name: "dub", config: providers.PlaybackConfig{SubOrDub: "dub"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			links, hints, err := provider.GetEpisodeURLForModeWithHints(tc.config, showID, 1, tc.config.SubOrDub)
			if err != nil {
				t.Fatalf("streams: %v", err)
			}
			if len(links) == 0 {
				t.Fatal("no stream links")
			}
			hint := hints[links[0]]
			if tc.wantSub && hint.Subtitle == "" {
				t.Fatal("expected an external subtitle for soft sub")
			}
			probeLivePlaylist(t, links[0], hint.Referrer)
			t.Logf("stream ok subtitle=%q", hint.Subtitle)
		})
	}
}

func probeLivePlaylist(t *testing.T, masterURL, referer string) {
	t.Helper()
	fetch := func(rawURL string) string {
		req, err := http.NewRequest(http.MethodGet, rawURL, nil)
		if err != nil {
			t.Fatalf("probe url: %v", err)
		}
		// media.settlar.io rejects the Go default UA; mpv's UA is accepted.
		req.Header.Set("User-Agent", userAgent)
		if referer != "" {
			req.Header.Set("Referer", referer)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("probe fetch %q: %v", rawURL, err)
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if err != nil {
			t.Fatalf("probe read: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("probe %q status %d", rawURL, resp.StatusCode)
		}
		return string(body)
	}

	master := fetch(masterURL)
	if !strings.Contains(master, "#EXTM3U") {
		t.Fatalf("master is not HLS: %.120q", master)
	}
	mediaURL := firstPlaylistLine(t, master, masterURL)
	media := fetch(mediaURL)
	segment := firstPlaylistLine(t, media, mediaURL)
	req, err := http.NewRequest(http.MethodGet, segment, nil)
	if err != nil {
		t.Fatalf("segment url: %v", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Referer", referer)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("segment fetch: %v", err)
	}
	defer resp.Body.Close()
	head := make([]byte, 1)
	if _, err := io.ReadFull(resp.Body, head); err != nil {
		t.Fatalf("segment read: %v", err)
	}
	if head[0] != 0x47 {
		t.Fatalf("segment does not start with MPEG-TS sync byte: %#x", head)
	}
}

// firstPlaylistLine returns the first non-comment playlist entry resolved
// against the playlist URL it came from.
func firstPlaylistLine(t *testing.T, body, playlistURL string) string {
	t.Helper()
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "http://") || strings.HasPrefix(line, "https://") {
			return line
		}
		base := playlistURL[:strings.LastIndex(playlistURL, "/")+1]
		return base + line
	}
	t.Fatalf("no playlist entry in %.200q", body)
	return ""
}
