package anikoto

import (
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/wraient/curd/internal/curdhost"
	"github.com/wraient/curd/internal/providers"
)

func TestLiveAnikotoProviderFlow(t *testing.T) {
	if os.Getenv("CURD_LIVE_ANIKOTO") == "" {
		t.Skip("set CURD_LIVE_ANIKOTO=1 to run the live Anikoto flow")
	}
	previousClient := curdhost.HTTPClient
	curdhost.HTTPClient = func() *http.Client { return http.DefaultClient }
	t.Cleanup(func() { curdhost.HTTPClient = previousClient })

	provider := &Provider{}
	results, err := provider.SearchAnime("frieren", "sub")
	if err != nil || len(results) == 0 {
		t.Fatalf("search Anikoto: results=%d err=%v", len(results), err)
	}
	selected := -1
	for index, result := range results {
		if strings.EqualFold(result.Title, "Frieren: Beyond Journey's End") {
			selected = index
			break
		}
	}
	if selected < 0 {
		t.Fatalf("main Frieren result not found in %#v", results)
	}
	episodes, err := provider.EpisodesList(results[selected].Key, "sub")
	if err != nil || len(episodes) == 0 {
		t.Fatalf("list Anikoto episodes: episodes=%d err=%v", len(episodes), err)
	}
	links, hints, err := provider.GetEpisodeURLForModeWithHints(providers.PlaybackConfig{SubOrDub: "sub"}, results[selected].Key, 1, "sub")
	if err != nil || len(links) == 0 {
		t.Fatalf("resolve Anikoto stream: links=%d err=%v", len(links), err)
	}

	link := links[0]
	hint := hints[link]
	req, err := http.NewRequest(http.MethodGet, link, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Referer", hint.Referrer)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("fetch Anikoto master: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		t.Fatalf("read Anikoto master: %v", err)
	}
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(strings.TrimSpace(string(body)), "#EXTM3U") {
		t.Fatalf("unexpected Anikoto master status=%d body=%q", resp.StatusCode, body)
	}

	fetch := func(rawURL, referer, byteRange string) ([]byte, int) {
		req, err := http.NewRequest(http.MethodGet, rawURL, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Referer", referer)
		if byteRange != "" {
			req.Header.Set("Range", byteRange)
		}
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("fetch Anikoto media %s: %v", rawURL, err)
		}
		defer response.Body.Close()
		payload, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes))
		if err != nil {
			t.Fatalf("read Anikoto media %s: %v", rawURL, err)
		}
		return payload, response.StatusCode
	}

	masterURL, err := url.Parse(link)
	if err != nil {
		t.Fatal(err)
	}
	variantPath := firstAnikotoMediaPath(string(body))
	if variantPath == "" {
		t.Fatal("Anikoto master did not contain a variant")
	}
	variantRef, err := url.Parse(variantPath)
	if err != nil {
		t.Fatal(err)
	}
	variantBody, status := fetch(masterURL.ResolveReference(variantRef).String(), hint.Referrer, "")
	if status != http.StatusOK || !strings.HasPrefix(strings.TrimSpace(string(variantBody)), "#EXTM3U") {
		t.Fatalf("unexpected Anikoto variant status=%d body=%q", status, variantBody)
	}
	segmentPath := firstAnikotoMediaPath(string(variantBody))
	if segmentPath == "" {
		t.Fatal("Anikoto variant did not contain a segment")
	}
	segmentRef, err := url.Parse(segmentPath)
	if err != nil {
		t.Fatal(err)
	}
	_, status = fetch(masterURL.ResolveReference(segmentRef).String(), hint.Referrer, "bytes=0-1")
	if status != http.StatusOK && status != http.StatusPartialContent {
		t.Fatalf("unexpected Anikoto segment status=%d", status)
	}
	if hint.Subtitle != "" {
		_, status = fetch(hint.Subtitle, hint.Referrer, "bytes=0-1")
		if status != http.StatusOK && status != http.StatusPartialContent {
			t.Fatalf("unexpected Anikoto subtitle status=%d", status)
		}
	}
}

func firstAnikotoMediaPath(manifest string) string {
	for _, line := range strings.Split(manifest, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			return line
		}
	}
	return ""
}
