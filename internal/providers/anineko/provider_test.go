package anineko

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wraient/curd/internal/curdhost"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func testResponse(req *http.Request, statusCode int, body string, headers map[string]string) *http.Response {
	resp := &http.Response{
		StatusCode: statusCode,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}
	for key, value := range headers {
		resp.Header.Set(key, value)
	}
	return resp
}

func withAninekoTestClient(t *testing.T, client *http.Client) {
	t.Helper()
	previous := curdhost.HTTPClient
	t.Cleanup(func() {
		curdhost.HTTPClient = previous
		resetVibeProxyForTest()
		resetSubStyleForTest()
	})
	curdhost.HTTPClient = func() *http.Client { return client }
}

func TestSearchAnimeParsesResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ajax/search" {
			http.NotFound(w, r)
			return
		}
		_, _ = io.WriteString(w, `{"success":true,"results":[{"title":"One Piece","url":"/watch/one-piece","image":"/img/op.jpg","meta":"TV"}]}`)
	}))
	defer server.Close()

	withAninekoTestClient(t, server.Client())

	originalBase := baseURL
	t.Cleanup(func() { baseURL = originalBase })
	baseURL = server.URL

	options, err := searchAnime("one piece", "sub")
	if err != nil {
		t.Fatalf("searchAnime: %v", err)
	}
	if len(options) != 1 {
		t.Fatalf("expected 1 option, got %d", len(options))
	}
	if options[0].Key != "one-piece" {
		t.Fatalf("unexpected key %q", options[0].Key)
	}
	if options[0].Thumbnail != server.URL+"/img/op.jpg" {
		t.Fatalf("unexpected thumbnail %q", options[0].Thumbnail)
	}
}

func TestEpisodesListParsesEpisodeLinks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `<a href="/watch/frieren/ep-1">1</a><a href="/watch/frieren/ep-12">12</a>`)
	}))
	defer server.Close()

	withAninekoTestClient(t, server.Client())
	originalBase := baseURL
	t.Cleanup(func() { baseURL = originalBase })
	baseURL = server.URL

	episodes, err := episodesList("frieren", "sub")
	if err != nil {
		t.Fatalf("episodesList: %v", err)
	}
	if strings.Join(episodes, ",") != "1,12" {
		t.Fatalf("unexpected episodes %v", episodes)
	}
}

func TestExtractLangEmbedURLs(t *testing.T) {
	html := `
<div class="server-items" data-id="sub">
  <div data-video="https://bibiemb.xyz/aaa"></div>
  <div data-video="https://vibeplayer.site/bbb?sub=https://cdn.example/sub.vtt"></div>
</div>
<div class="server-items" data-id="dub">
  <div data-video="https://bibiemb.xyz/ccc"></div>
</div>`

	groups := extractLangEmbedURLs(html)
	if len(groups["sub"]) != 2 {
		t.Fatalf("expected 2 sub urls, got %d", len(groups["sub"]))
	}
	if len(groups["dub"]) != 1 {
		t.Fatalf("expected 1 dub url, got %d", len(groups["dub"]))
	}
	subURLs := embedURLsForSubStyle(groups, "soft")
	if len(subURLs) != 2 {
		t.Fatalf("expected 2 soft sub urls, got %d", len(subURLs))
	}
	if subtitleFromEmbedURL(subURLs[1]) != "https://cdn.example/sub.vtt" {
		t.Fatalf("unexpected subtitle %q", subtitleFromEmbedURL(subURLs[1]))
	}
}

func TestChooseSubStyleRespectsPreference(t *testing.T) {
	groups := map[string][]string{
		"hsub": {"https://vibeplayer.site/hard"},
		"sub":  {"https://vibeplayer.site/soft?sub=https://cdn.example/sub.vtt"},
		"dub":  {"https://vibeplayer.site/dub"},
	}

	style, err := chooseSubStyle(groups, "soft")
	if err != nil || style != "soft" {
		t.Fatalf("chooseSubStyle soft: style=%q err=%v", style, err)
	}

	style, err = chooseSubStyle(groups, "hard")
	if err != nil || style != "hard" {
		t.Fatalf("chooseSubStyle hard: style=%q err=%v", style, err)
	}
}

func TestChooseSubStylePromptSelectOnce(t *testing.T) {
	groups := map[string][]string{
		"hsub": {"https://vibeplayer.site/hard"},
		"sub":  {"https://vibeplayer.site/soft?sub=https://cdn.example/sub.vtt"},
	}

	prompts := 0
	previousPrompt := curdhost.PromptSelect
	previousOut := curdhost.Out
	previousPersist := curdhost.PersistSubStylePreference
	previousCurrent := curdhost.CurrentSubStyle
	curdhost.PromptSelect = func(options []curdhost.PromptOption) (curdhost.PromptOption, error) {
		prompts++
		return curdhost.PromptOption{Key: "hard", Label: "Hard sub"}, nil
	}
	curdhost.Out = func(string) {}
	saved := ""
	curdhost.PersistSubStylePreference = func(style string) error {
		saved = style
		return nil
	}
	curdhost.CurrentSubStyle = func() string { return saved }
	t.Cleanup(func() {
		curdhost.PromptSelect = previousPrompt
		curdhost.Out = previousOut
		curdhost.PersistSubStylePreference = previousPersist
		curdhost.CurrentSubStyle = previousCurrent
		resetSubStyleForTest()
	})

	for i := 0; i < 2; i++ {
		style, err := chooseSubStyle(groups, "ask")
		if err != nil || style != "hard" {
			t.Fatalf("iteration %d: style=%q err=%v", i, style, err)
		}
	}
	if prompts != 1 {
		t.Fatalf("expected 1 prompt, got %d", prompts)
	}
	if saved != "hard" {
		t.Fatalf("expected saved preference hard, got %q", saved)
	}
}

func TestResolveBibiembPicks1080pVariant(t *testing.T) {
	const hash = "agdeadbeef"

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/"+hash:
			_, _ = io.WriteString(w, `const src = "`+server.URL+`/`+hash+`/master.m3u8"`)
		case r.URL.Path == "/"+hash+"/master.m3u8":
			_, _ = io.WriteString(w, `#EXTM3U
#EXT-X-STREAM-INF:BANDWIDTH=1,NAME="720p"
`+server.URL+`/`+hash+`/720p/index.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=2,NAME="1080p"
`+server.URL+`/`+hash+`/1080p/index.m3u8`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	withAninekoTestClient(t, server.Client())

	stream, err := resolveBibiemb(server.URL + "/" + hash)
	if err != nil {
		t.Fatalf("resolveBibiemb: %v", err)
	}
	if !strings.HasSuffix(stream.URL, "/1080p/index.m3u8") {
		t.Fatalf("expected 1080p variant, got %q", stream.URL)
	}
}

func TestStripPNGWrapper(t *testing.T) {
	png := []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
		0x89, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x44, 0x41,
		0x54, 0x78, 0xda, 0x63, 0x64, 0xf8, 0xcf, 0x50,
		0x0f, 0x00, 0x03, 0x86, 0x01, 0x80, 0x5a, 0x34,
		0x7d, 0x6b, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45,
		0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
	}
	payload := []byte{0x47, 0x01, 0x02, 0x03}
	data := append(append([]byte{}, png...), payload...)
	if got := stripPNGWrapper(data); string(got) != string(payload) {
		t.Fatalf("unexpected stripped payload %v", got)
	}
}

func TestVibeProxyRewritesAndStripsSegments(t *testing.T) {
	const (
		id      = "abc123"
		variant = "1720.m3u8"
	)

	png := []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
		0x89, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x44, 0x41,
		0x54, 0x78, 0xda, 0x63, 0x64, 0xf8, 0xcf, 0x50,
		0x0f, 0x00, 0x03, 0x86, 0x01, 0x80, 0x5a, 0x34,
		0x7d, 0x6b, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45,
		0x4e, 0x44, 0xae, 0x42, 0x60, 0x82, 0x47, 0x11,
	}

	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/public/stream/" + id + "/master.m3u8":
			_, _ = io.WriteString(w, "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1,NAME=\"720p\"\n"+variant+"\n")
		case "/public/stream/" + id + "/" + variant:
			_, _ = io.WriteString(w, "#EXTM3U\n#EXTINF:10.0,\n/public/segment.bin\n")
		case "/public/segment.bin":
			_, _ = w.Write(png)
		default:
			http.NotFound(w, r)
		}
	}))
	defer remote.Close()

	withAninekoTestClient(t, remote.Client())
	resetVibeProxyForTest()

	masterURL := remote.URL + "/public/stream/" + id + "/master.m3u8"
	proxyURL, err := registerVibeStream(masterURL, remote.URL+"/"+id)
	if err != nil {
		t.Fatalf("registerVibeStream: %v", err)
	}

	resp, err := http.Get(proxyURL)
	if err != nil {
		t.Fatalf("get master: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), "/1720.m3u8") {
		t.Fatalf("master not rewritten: %s", body)
	}

	variantURL := ""
	for _, line := range strings.Split(strings.ReplaceAll(string(body), "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			variantURL = line
			break
		}
	}
	resp, err = http.Get(variantURL)
	if err != nil {
		t.Fatalf("get variant: %v", err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	segURL := ""
	for _, line := range strings.Split(strings.TrimSpace(string(body)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			segURL = line
		}
	}

	resp, err = http.Get(segURL)
	if err != nil {
		t.Fatalf("get segment: %v", err)
	}
	segBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if len(segBody) != 2 || segBody[0] != 0x47 {
		t.Fatalf("expected stripped mpeg-ts payload, got %v", segBody)
	}
}
