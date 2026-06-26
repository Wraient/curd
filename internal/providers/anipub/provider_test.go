package anipub

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

func withAnipubTestClient(t *testing.T, client *http.Client) {
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

func textResponse(req *http.Request, statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}
}

func TestDecodeSearchResultsSupportsSingleObjectAndNotFound(t *testing.T) {
	t.Run("array", func(t *testing.T) {
		results, err := decodeSearchResults([]byte(`[{"Name":"Frieren","Id":2454}]`))
		if err != nil || len(results) != 1 || results[0].ID != 2454 {
			t.Fatalf("array decode: %#v err=%v", results, err)
		}
	})
	t.Run("single object", func(t *testing.T) {
		results, err := decodeSearchResults([]byte(`{"Name":"Delicious in Dungeon","Id":2475,"finder":"delicious-in-dungeon"}`))
		if err != nil || len(results) != 1 || results[0].ID != 2475 {
			t.Fatalf("single decode: %#v err=%v", results, err)
		}
	})
	t.Run("not found", func(t *testing.T) {
		results, err := decodeSearchResults([]byte(`{"found":false}`))
		if err != nil || len(results) != 0 {
			t.Fatalf("not found decode: %#v err=%v", results, err)
		}
	})
}

func TestSearchAnimeParsesResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/search/frieren":
			_ = json.NewEncoder(w).Encode([]searchResult{{
				Name:   "Frieren: Beyond Journey's End",
				ID:     2454,
				Image:  "https://cdn.example/frieren.jpg",
				Finder: "frieren-beyond-journeys-end",
			}})
		case r.URL.Path == "/api/info/2454":
			_ = json.NewEncoder(w).Encode(infoResponse{
				ID:      2454,
				Name:    "Frieren: Beyond Journey's End",
				MALID:   "52991",
				EpCount: 28,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	withAnipubTestClient(t, server.Client())
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
	if options[0].Key != "2454" {
		t.Fatalf("unexpected key %q", options[0].Key)
	}
	item, ok := options[0].ExtraData.(SearchItem)
	if !ok || item.MalID != 52991 || item.Episodes != 28 {
		t.Fatalf("unexpected extra data %+v", options[0].ExtraData)
	}
}

func TestEpisodesListCountsLocalEpisodes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/api/details/2454" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(detailsResponse{
			Local: localDetails{
				Link: "src=https://anipub.xyz/video/107257/sub",
				Ep: []episodeLink{
					{Link: "src=https://anipub.xyz/video/107259/sub"},
					{Link: "src=https://anipub.xyz/video/107260/sub"},
				},
			},
		})
	}))
	defer server.Close()

	withAnipubTestClient(t, server.Client())
	originalBase := baseURL
	t.Cleanup(func() { baseURL = originalBase })
	baseURL = server.URL

	episodes, err := episodesList("2454", "sub")
	if err != nil {
		t.Fatalf("episodesList: %v", err)
	}
	if len(episodes) != 3 || episodes[0] != "1" || episodes[2] != "3" {
		t.Fatalf("unexpected episodes %#v", episodes)
	}
}

func TestGetEpisodeStreamsForModeResolvesMegaplay(t *testing.T) {
	megaplay := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/stream/s-2/107257/sub":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<div id="player" data-id="13461" data-realid="107257"></div>`))
		case "/stream/getSources":
			_ = json.NewEncoder(w).Encode(megaplaySourcesResponse{
				Sources: struct {
					File string `json:"file"`
				}{File: "https://cdn.example/master.m3u8"},
				Tracks: []struct {
					File    string `json:"file"`
					Label   string `json:"label"`
					Kind    string `json:"kind"`
					Default bool   `json:"default"`
				}{
					{File: "https://cdn.example/eng.vtt", Label: "English", Kind: "captions", Default: true},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer megaplay.Close()

	anipubServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/api/details/2454" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(detailsResponse{
			Local: localDetails{
				Link: "src=https://anipub.xyz/video/107257/sub",
			},
		})
	}))
	defer anipubServer.Close()

	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if strings.HasPrefix(req.URL.String(), megaplay.URL) {
			return http.DefaultTransport.RoundTrip(req)
		}
		if strings.HasPrefix(req.URL.String(), anipubServer.URL) {
			return http.DefaultTransport.RoundTrip(req)
		}
		return textResponse(req, http.StatusNotFound, ""), nil
	})}
	withAnipubTestClient(t, client)
	originalBase := baseURL
	originalMega := megaplayBaseURL
	t.Cleanup(func() {
		baseURL = originalBase
		megaplayBaseURL = originalMega
	})
	baseURL = anipubServer.URL
	megaplayBaseURL = megaplay.URL

	links, hints, err := getEpisodeStreamsForMode("2454", providers.PlaybackConfig{SubOrDub: "sub"}, 1)
	if err != nil {
		t.Fatalf("getEpisodeStreamsForMode: %v", err)
	}
	if len(links) != 1 || links[0] != "https://cdn.example/master.m3u8" {
		t.Fatalf("unexpected links %#v", links)
	}
	if hints[links[0]].Referrer != megaplay.URL+"/" {
		t.Fatalf("unexpected referrer %q", hints[links[0]].Referrer)
	}
	if hints[links[0]].Subtitle != "https://cdn.example/eng.vtt" {
		t.Fatalf("unexpected subtitle %q", hints[links[0]].Subtitle)
	}
}

func TestResolveMegaplayStreamUsesDubMode(t *testing.T) {
	megaplay := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/stream/s-2/42/dub":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<div data-id="99"></div>`))
		case "/stream/getSources":
			_ = json.NewEncoder(w).Encode(megaplaySourcesResponse{
				Sources: struct {
					File string `json:"file"`
				}{File: "https://cdn.example/dub.m3u8"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer megaplay.Close()

	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if strings.HasPrefix(req.URL.String(), megaplay.URL) {
			return http.DefaultTransport.RoundTrip(req)
		}
		return textResponse(req, http.StatusNotFound, ""), nil
	})}
	withAnipubTestClient(t, client)
	originalMega := megaplayBaseURL
	t.Cleanup(func() { megaplayBaseURL = originalMega })
	megaplayBaseURL = megaplay.URL

	streamURL, subtitle, err := resolveMegaplayStream("https://anipub.xyz/video/42/sub", "dub")
	if err != nil {
		t.Fatalf("resolveMegaplayStream: %v", err)
	}
	if streamURL != "https://cdn.example/dub.m3u8" {
		t.Fatalf("unexpected stream %q", streamURL)
	}
	if subtitle != "" {
		t.Fatalf("expected no subtitle for dub, got %q", subtitle)
	}
}
