package anikoto

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/wraient/curd/internal/curdhost"
)

func TestAnikotoProviderFlow(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/filter":
			if r.URL.Query().Get("keyword") != "frieren" || r.URL.Query().Get("vrf") == "" {
				t.Fatalf("unexpected search query: %s", r.URL.RawQuery)
			}
			html := `<div class="item"><a class="name" href="SERVER_URL/watch/not-a-result/ep-1">Not a search result</a></div><div class="ani items"><div class="item"><div class="ani poster tip" data-tip="6351"><a href="SERVER_URL/watch/frieren-test-abc/ep-1"><img src="SERVER_URL/poster.jpg"></a></div><div class="info"><a class="name d-title" href="SERVER_URL/watch/frieren-test-abc/ep-1" data-jp="Sousou no Frieren">Frieren: Beyond Journey's End</a></div></div></div>`
			_, _ = io.WriteString(w, strings.ReplaceAll(html, "SERVER_URL", server.URL))
		case r.URL.Path == "/watch/fallback-id/ep-1":
			_, _ = io.WriteString(w, `<div id="watch-main" data-id="6351"></div>`)
		case r.URL.Path == "/ajax/episode/list/6351":
			if r.URL.Query().Get("vrf") == "" {
				t.Fatal("episode VRF is empty")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"status": 200, "result": `
				<div class="episodes"><ul>
					<li><a href="#" data-num="1" data-sub="1" data-dub="1" data-ids="episode-token-1" data-mal="52991" data-slug="1" data-timestamp="1729242913">Episode 1</a></li>
					<li><a href="#" data-num="2" data-sub="1" data-dub="0" data-ids="episode-token-2" data-mal="52991" data-slug="2" data-timestamp="1729242913">Episode 2</a></li>
				</ul></div>`})
		case r.URL.Path == "/ajax/server/list":
			if r.URL.Query().Get("servers") != "episode-token-1" {
				t.Fatalf("unexpected server token %q", r.URL.Query().Get("servers"))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"status": 200, "result": `
				<div class="servers">
					<div class="type" data-type="sub"><label>SUB</label><ul>
						<li data-link-id="sub-link">Vidstream-2</li>
						<li data-link-id="sub-backup">HD-1</li>
					</ul></div>
					<div class="type" data-type="dub"><label>DUB</label><ul>
						<li data-link-id="dub-link">HD-1</li>
					</ul></div>
				</div>`})
		case r.URL.Path == "/ajax/server":
			serverPath := ""
			switch r.URL.Query().Get("get") {
			case "sub-link":
				serverPath = "/stream/s-2/107257/sub"
			case "dub-link":
				serverPath = "/stream/s-2/107258/dub"
			default:
				http.Error(w, "unsupported test server", http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"status": 200, "result": map[string]any{"url": server.URL + serverPath}})
		case strings.HasPrefix(r.URL.Path, "/stream/s-2/") && (strings.HasSuffix(r.URL.Path, "/sub") || strings.HasSuffix(r.URL.Path, "/dub")):
			if r.Header.Get("Referer") != server.URL+"/" {
				t.Fatalf("unexpected embed referrer %q", r.Header.Get("Referer"))
			}
			_, _ = io.WriteString(w, `<div data-id="13461"></div>`)
		case r.URL.Path == "/stream/getSources":
			if r.URL.Query().Get("id") != "13461" || !strings.HasPrefix(r.Header.Get("Referer"), server.URL+"/stream/s-2/") {
				t.Fatalf("unexpected getSources request: id=%q referer=%q", r.URL.Query().Get("id"), r.Header.Get("Referer"))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"enc": encryptMegaPlayFixture(t, `{"file":"`+server.URL+`/cdn/0123456789abcdef0123456789abcdef/fedcba9876543210fedcba9876543210/master.m3u8"}`),
				"tracks": []map[string]any{
					{"file": server.URL + "/sub/eng.vtt", "label": "English", "kind": "captions", "default": true},
					{"file": server.URL + "/sub/forced.vtt", "label": "Forced", "kind": "captions"},
				},
			})
		case strings.HasPrefix(r.URL.Path, "/cdn/") && strings.HasSuffix(r.URL.Path, "/master.m3u8"):
			if r.Header.Get("Referer") != server.URL+"/" {
				t.Fatalf("signed master requires referrer, got %q", r.Header.Get("Referer"))
			}
			_, _ = io.WriteString(w, "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1\nindex.m3u8\n")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	previousBase := baseURL
	previousClient := curdhost.HTTPClient
	baseURL = server.URL
	curdhost.HTTPClient = func() *http.Client { return server.Client() }
	t.Cleanup(func() {
		baseURL = previousBase
		curdhost.HTTPClient = previousClient
	})

	animeID, err := resolveAnimeID("fallback-id")
	if err != nil || animeID != 6351 {
		t.Fatalf("resolveAnimeID() = %d, %v", animeID, err)
	}

	searchResults, err := searchAnime("frieren", "sub")
	if err != nil {
		t.Fatalf("searchAnime: %v", err)
	}
	if len(searchResults) != 1 || searchResults[0].Key != "frieren-test-abc#6351" {
		t.Fatalf("unexpected search results: %#v", searchResults)
	}

	episodes, err := episodesList(searchResults[0].Key, "sub")
	if err != nil {
		t.Fatalf("episodesList: %v", err)
	}
	if len(episodes) != 2 || episodes[0] != "1" || episodes[1] != "2" {
		t.Fatalf("unexpected episodes: %#v", episodes)
	}
	dubEpisodes, err := episodesList(searchResults[0].Key, "dub")
	if err != nil {
		t.Fatalf("episodesList(dub): %v", err)
	}
	if len(dubEpisodes) != 1 || dubEpisodes[0] != "1" {
		t.Fatalf("unexpected dub episodes: %#v", dubEpisodes)
	}

	links, hints, err := getEpisodeStreamsForMode(searchResults[0].Key, 1, "sub")
	if err != nil {
		t.Fatalf("getEpisodeStreamsForMode: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("unexpected stream links: %#v", links)
	}
	if !regexp.MustCompile(`[?&]token=`).MatchString(links[0]) {
		t.Fatalf("MegaPlay URL was not signed: %s", links[0])
	}
	if hints[links[0]].Referrer != server.URL+"/" {
		t.Fatalf("unexpected playback referrer %q", hints[links[0]].Referrer)
	}
	if hints[links[0]].Subtitle != server.URL+"/sub/eng.vtt" {
		t.Fatalf("unexpected subtitle %q", hints[links[0]].Subtitle)
	}

	req, err := http.NewRequest(http.MethodGet, links[0], nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Referer", hints[links[0]].Referrer)
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("fetch signed master: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(string(body), "#EXTM3U") {
		t.Fatalf("unexpected signed master status=%d body=%q", resp.StatusCode, body)
	}

	dubLinks, dubHints, err := getEpisodeStreamsForMode(searchResults[0].Key, 1, "dub")
	if err != nil {
		t.Fatalf("resolve dub stream: %v", err)
	}
	if len(dubLinks) != 1 || !strings.Contains(dubLinks[0], "token=") {
		t.Fatalf("unexpected dub links: %#v", dubLinks)
	}
	if dubHints[dubLinks[0]].Subtitle != "" {
		t.Fatalf("unexpected dub subtitle %q", dubHints[dubLinks[0]].Subtitle)
	}
}

func TestEncryptVRF(t *testing.T) {
	if got := encryptVRF("6351"); got != "cE9mNXJ3M0h3b0VNODVnRA%3D%3D" {
		t.Fatalf("encryptVRF() = %q", got)
	}
}

func TestResolveMegaPlaySourcePreservesPlainSource(t *testing.T) {
	raw := "https://cdn.example/0123456789abcdef0123456789abcdef/fedcba9876543210fedcba9876543210/master.m3u8"
	got, err := resolveMegaPlaySource(megaPlaySourcesResponse{Sources: flexibleString(raw)})
	if err != nil {
		t.Fatalf("resolveMegaPlaySource: %v", err)
	}
	if got != raw {
		t.Fatalf("plain source changed: %s", got)
	}
}

func TestParseAnikotoServerCandidatesFiltersMode(t *testing.T) {
	html := `<div class="servers"><div class="type" data-type="sub"><ul><li data-link-id="sub-link">Vidstream-2</li></ul></div><div class="type" data-type="dub"><ul><li data-link-id="dub-link">HD-1</li></ul></div></div>`
	sub := parseServerCandidates(html, "sub")
	dub := parseServerCandidates(html, "dub")
	if len(sub) != 1 || sub[0].ID != "sub-link" {
		t.Fatalf("unexpected sub candidates: %#v", sub)
	}
	if len(dub) != 1 || dub[0].ID != "dub-link" {
		t.Fatalf("unexpected dub candidates: %#v", dub)
	}
}

func encryptMegaPlayFixture(t *testing.T, plaintext string) string {
	t.Helper()
	key := make([]byte, 32)
	copy(key, []byte("i?LMTAx0Q6,:}50U"))
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	iv := []byte("W0;27ToaUpl_P%'c")
	padded := pkcs7Pad([]byte(plaintext), 16)
	ciphertext := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(ciphertext, padded)
	return base64.RawURLEncoding.EncodeToString(ciphertext)
}

func pkcs7Pad(data []byte, size int) []byte {
	padding := size - len(data)%size
	return append(data, bytes.Repeat([]byte{byte(padding)}, padding)...)
}
