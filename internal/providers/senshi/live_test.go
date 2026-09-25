package senshi

import (
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/wraient/curd/internal/curdhost"
	"github.com/wraient/curd/internal/providers"
)

// Run with: CURD_LIVE_SENSHI=1 go test ./internal/providers/senshi -run TestLiveSenshiPlaybackResolution -v
func TestLiveSenshiPlaybackResolution(t *testing.T) {
	if os.Getenv("CURD_LIVE_SENSHI") == "" {
		t.Skip("set CURD_LIVE_SENSHI=1 to run the live Senshi flow")
	}
	previousClient := curdhost.HTTPClient
	curdhost.HTTPClient = func() *http.Client { return http.DefaultClient }
	t.Cleanup(func() {
		curdhost.HTTPClient = previousClient
		resetSenshiProxyForTest()
	})

	results, err := searchAnime("frieren", "sub")
	if err != nil || len(results) == 0 {
		t.Fatalf("search Senshi: results=%d err=%v", len(results), err)
	}
	episodes, err := episodesList(results[0].Key, "sub")
	if err != nil || len(episodes) == 0 {
		t.Fatalf("list Senshi episodes: episodes=%d err=%v", len(episodes), err)
	}
	links, _, err := getEpisodeStreamsForMode(results[0].Key, providers.PlaybackConfig{SubOrDub: "sub"}, 1)
	if err != nil || len(links) == 0 {
		t.Fatalf("resolve Senshi stream: links=%d err=%v", len(links), err)
	}
	resp, err := http.Get(links[0])
	if err != nil {
		t.Fatalf("fetch proxied Senshi master: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read proxied Senshi master: %v", err)
	}
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(strings.TrimSpace(string(body)), "#EXTM3U") {
		t.Fatalf("proxied Senshi master status=%d body=%q", resp.StatusCode, body)
	}
	var variantURL string
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			variantURL = line
			break
		}
	}
	if variantURL == "" {
		t.Fatal("proxied Senshi master did not contain a variant URL")
	}
	variantResp, err := http.Get(variantURL)
	if err != nil {
		t.Fatalf("fetch proxied Senshi variant: %v", err)
	}
	defer variantResp.Body.Close()
	variantBody, err := io.ReadAll(variantResp.Body)
	if err != nil {
		t.Fatalf("read proxied Senshi variant: %v", err)
	}
	if variantResp.StatusCode != http.StatusOK || !strings.HasPrefix(strings.TrimSpace(string(variantBody)), "#EXTM3U") {
		t.Fatalf("proxied Senshi variant status=%d body=%q", variantResp.StatusCode, variantBody)
	}
	var segmentURL string
	for _, line := range strings.Split(string(variantBody), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			segmentURL = line
			break
		}
	}
	if segmentURL == "" {
		t.Fatal("proxied Senshi variant did not contain a segment URL")
	}
	segmentReq, err := http.NewRequest(http.MethodGet, segmentURL, nil)
	if err != nil {
		t.Fatalf("build Senshi segment request: %v", err)
	}
	segmentReq.Header.Set("Referer", baseURL+"/")
	segmentResp, err := http.DefaultClient.Do(segmentReq)
	if err != nil {
		t.Fatalf("fetch Senshi segment with MPV-compatible headers: %v", err)
	}
	defer segmentResp.Body.Close()
	if segmentResp.StatusCode != http.StatusOK {
		t.Fatalf("Senshi segment status=%d", segmentResp.StatusCode)
	}
}
