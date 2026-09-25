package senshi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wraient/curd/internal/curdhost"
	"github.com/wraient/curd/internal/providers"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func withSenshiTestClient(t *testing.T, client *http.Client) {
	t.Helper()
	previous := curdhost.HTTPClient
	t.Cleanup(func() {
		curdhost.HTTPClient = previous
	})
	curdhost.HTTPClient = func() *http.Client { return client }
}

func jsonResponse(req *http.Request, statusCode int, payload any) *http.Response {
	body, _ := json.Marshal(payload)
	resp := &http.Response{
		StatusCode: statusCode,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(string(body))),
		Request:    req,
	}
	resp.Header.Set("Content-Type", "application/json")
	return resp
}

func TestSearchAnimeParsesResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/anime/filter" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode search request: %v", err)
		}
		if request["sortBy"] != "score_desc" || request["languagePreference"] != "EN" || request["limit"] != float64(30) {
			t.Fatalf("incomplete search request: %#v", request)
		}
		_ = json.NewEncoder(w).Encode(filterResponse{
			Data: []animeItem{{
				ID:           52991,
				PublicID:     "4medy",
				Title:        "Sousou no Frieren",
				TitleEnglish: "Frieren: Beyond Journey's End",
				Type:         "TV",
				AniEpisodes:  "28",
				AniYear:      2023,
				AnimePicture: "/images/frieren.webp",
			}},
			Total: 1,
		})
	}))
	defer server.Close()

	withSenshiTestClient(t, server.Client())
	originalBase := baseURL
	t.Cleanup(func() { baseURL = originalBase })
	baseURL = server.URL

	options, err := searchAnime("frieren", "sub")
	if err != nil {
		t.Fatalf("searchAnime: %v", err)
	}
	if len(options) != 1 {
		t.Fatalf("expected 1 option, got %d", len(options))
	}
	if options[0].Key != "52991" {
		t.Fatalf("unexpected key %q", options[0].Key)
	}
	if !strings.Contains(options[0].Thumbnail, "/images/frieren.webp") {
		t.Fatalf("unexpected thumbnail %q", options[0].Thumbnail)
	}
	item, ok := options[0].ExtraData.(SearchItem)
	if !ok || item.Episodes != 28 {
		t.Fatalf("unexpected extra data %+v", options[0].ExtraData)
	}
}

func TestEpisodesListParsesEpisodeIDs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/episodes/52991" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode([]episodeItem{
			{ID: 1, EpID: 1, MalID: 52991},
			{ID: 3, EpID: 3, MalID: 52991},
		})
	}))
	defer server.Close()

	withSenshiTestClient(t, server.Client())
	originalBase := baseURL
	t.Cleanup(func() { baseURL = originalBase })
	baseURL = server.URL

	episodes, err := episodesList("52991", "sub")
	if err != nil {
		t.Fatalf("episodesList: %v", err)
	}
	if len(episodes) != 2 || episodes[0] != "1" || episodes[1] != "3" {
		t.Fatalf("unexpected episodes %#v", episodes)
	}
}

func TestGetEpisodeStreamsForModeResolvesVidcloudThroughProxy(t *testing.T) {
	var server *httptest.Server
	var sawVideoHeaders bool
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/episode-embeds/62435/1":
			_ = json.NewEncoder(w).Encode([]embedItem{{RemoteSourceID: intPtr(42), Status: "HardSub"}})
		case "/_v1/sources":
			if r.URL.Query().Get("id") != "42" {
				t.Fatalf("unexpected source id %q", r.URL.Query().Get("id"))
			}
			_ = json.NewEncoder(w).Encode([]vidcloudEntry{{
				Source: &vidcloudSource{Src: server.URL + "/master.txt"},
				Tracks: []vidcloudTrack{{URL: server.URL + "/english.vtt", Label: "ENG", Default: true}},
			}})
		case "/master.txt":
			sawVideoHeaders = r.Header.Get("Origin") == baseURL && r.Header.Get("Referer") == baseURL+"/"
			_, _ = io.WriteString(w, "#EXTM3U\n#EXT-X-MEDIA:TYPE=AUDIO,DEFAULT=NO,URI=\"audio/0_ja.txt\"\n#EXT-X-MEDIA:TYPE=AUDIO,DEFAULT=NO,URI=\"audio/1_en.txt\"\n#EXT-X-STREAM-INF:BANDWIDTH=1\nvideo/main.txt\n")
		case "/english.vtt":
			_, _ = io.WriteString(w, "WEBVTT\n")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	withSenshiTestClient(t, server.Client())
	originalBase := baseURL
	originalVidcloudBase := vidcloudSourcesBaseURL
	resetSenshiProxyForTest()
	t.Cleanup(func() {
		baseURL = originalBase
		vidcloudSourcesBaseURL = originalVidcloudBase
		resetSenshiProxyForTest()
	})
	baseURL = server.URL
	vidcloudSourcesBaseURL = server.URL

	links, hints, err := getEpisodeStreamsForMode("62435", providers.PlaybackConfig{SubOrDub: "sub"}, 1)
	if err != nil {
		t.Fatalf("getEpisodeStreamsForMode: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("unexpected links %#v", links)
	}
	resp, err := http.Get(links[0])
	if err != nil {
		t.Fatalf("fetch proxied manifest: %v", err)
	}
	manifest, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if !sawVideoHeaders {
		t.Fatal("proxied playlist request did not include Senshi Origin and Referer")
	}
	if strings.Contains(string(manifest), "1_en") || !strings.Contains(string(manifest), "0_ja") {
		t.Fatalf("unexpected filtered manifest:\n%s", manifest)
	}
	if !strings.Contains(string(manifest), "127.0.0.1") {
		t.Fatalf("nested playlist was not proxied:\n%s", manifest)
	}
	if !strings.Contains(hints[links[0]].Subtitle, "127.0.0.1") {
		t.Fatalf("subtitle was not proxied: %q", hints[links[0]].Subtitle)
	}
}

func intPtr(value int) *int {
	return &value
}
