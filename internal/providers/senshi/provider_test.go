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
		_ = json.NewEncoder(w).Encode(filterResponse{
			Data: []animeItem{{
				ID:           52991,
				PublicID:     "4medy",
				Title:        "Sousou no Frieren",
				TitleEnglish: "Frieren: Beyond Journey's End",
				Type:         "TV",
				AniEpisodes:  "28",
				AniYear:      2023,
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
	if !strings.Contains(options[0].Thumbnail, "/posters/52991.webp") {
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

func TestGetEpisodeStreamsForModeSelectsHardSub(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/episode-embeds/52991/1" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode([]embedItem{
			{URL: "https://cdn.example/sub.m3u8", Status: "HardSub"},
			{URL: "https://cdn.example/dub.m3u8", Status: "Dub"},
		})
	}))
	defer server.Close()

	withSenshiTestClient(t, server.Client())
	originalBase := baseURL
	t.Cleanup(func() { baseURL = originalBase })
	baseURL = server.URL

	links, hints, err := getEpisodeStreamsForMode("52991", providers.PlaybackConfig{SubOrDub: "sub"}, 1)
	if err != nil {
		t.Fatalf("getEpisodeStreamsForMode: %v", err)
	}
	if len(links) != 1 || links[0] != "https://cdn.example/sub.m3u8" {
		t.Fatalf("unexpected links %#v", links)
	}
	if hints[links[0]].Referrer != baseURL+"/" {
		t.Fatalf("unexpected referrer %q", hints[links[0]].Referrer)
	}
}
