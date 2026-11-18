package animex

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wraient/curd/internal/curdhost"
	"github.com/wraient/curd/internal/providers"
)

func withAnimexTestClient(t *testing.T, client *http.Client) {
	t.Helper()
	previous := curdhost.HTTPClient
	t.Cleanup(func() { curdhost.HTTPClient = previous })
	curdhost.HTTPClient = func() *http.Client { return client }
}

func newAnimexFixture(t *testing.T) *httptest.Server {
	t.Helper()
	var server *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/graphql", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"data":{"catalogAnime":{"items":[{"id":"frieren-beyond-slug","anilistId":154587,"malId":52991,"titleRomaji":"Sousou no Frieren","titleEnglish":"Frieren: Beyond Journey's End","format":"TV","episodeCount":28,"seasonYear":2023,"coverImage":{"extraLarge":"https://cdn.example/frieren-xl.jpg","large":"https://cdn.example/frieren.jpg"}}]}}}`)
	})
	mux.HandleFunc("/rest/api/episodes", func(w http.ResponseWriter, r *http.Request) {
		// The show id is mandatory: the API has no show context otherwise.
		if r.URL.Query().Get("id") == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_, _ = io.WriteString(w, `[{"number":1,"hasSub":true,"hasDub":true},{"number":2,"hasSub":true,"hasDub":false}]`)
	})
	mux.HandleFunc("/rest/api/servers", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"subProviders":[{"id":"nero","default":true}],"dubProviders":[{"id":"nero","default":true}]}`)
	})
	mux.HandleFunc("/rest/api/sources", func(w http.ResponseWriter, r *http.Request) {
		// Server selection is mandatory: the query must name the show,
		// episode, type, and provider, mirroring the real API.
		query := r.URL.Query()
		if query.Get("id") == "" || query.Get("epNum") == "" || query.Get("type") == "" || query.Get("providerId") == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		// Sub and dub resolve to distinct streams so tests can prove the
		// requested language actually reached the lookup.
		playlist := "/master.m3u8"
		if query.Get("type") == "dub" {
			playlist = "/dub.m3u8"
		}
		_, _ = io.WriteString(w, `{"sources":[{"url":"`+server.URL+playlist+`","type":"application/vnd.apple.mpegurl"}],"tracks":[]}`)
	})
	mux.HandleFunc("/master.m3u8", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1,RESOLUTION=1280x720\n/media.m3u8\n")
	})
	mux.HandleFunc("/dub.m3u8", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1,RESOLUTION=1280x720\n/media.m3u8\n")
	})
	mux.HandleFunc("/media.m3u8", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "#EXTM3U\n#EXTINF:10.0,\n/seg1.ts\n")
	})
	mux.HandleFunc("/seg1.ts", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte{0x47, 0x40, 0x00, 0x10, 0x02, 0x42, 0xf0, 0x25})
	})
	server = httptest.NewServer(mux)
	t.Cleanup(server.Close)

	withAnimexTestClient(t, server.Client())
	originalGraphQL, originalRest := graphqlURL, restBase
	t.Cleanup(func() { graphqlURL, restBase = originalGraphQL, originalRest })
	graphqlURL = server.URL + "/graphql"
	restBase = server.URL + "/rest/api"
	return server
}

func TestSearchAnimeParsesResults(t *testing.T) {
	newAnimexFixture(t)

	options, err := searchAnime("frieren", "sub")
	if err != nil {
		t.Fatalf("searchAnime: %v", err)
	}
	if len(options) != 1 || options[0].Key != "frieren-beyond-slug" {
		t.Fatalf("unexpected options %#v", options)
	}
	item, ok := options[0].ExtraData.(SearchItem)
	if !ok || item.MalID != 52991 || item.Episodes != 28 || item.Type != "TV" {
		t.Fatalf("unexpected extra data %+v", options[0].ExtraData)
	}
	if options[0].Thumbnail != "https://cdn.example/frieren-xl.jpg" {
		t.Fatalf("expected largest cover variant, got %q", options[0].Thumbnail)
	}
}

func TestEpisodesListFiltersByMode(t *testing.T) {
	newAnimexFixture(t)

	sub, err := episodesList("frieren-beyond-slug", "sub")
	if err != nil || strings.Join(sub, ",") != "1,2" {
		t.Fatalf("sub episodes = %v err=%v", sub, err)
	}
	dub, err := episodesList("frieren-beyond-slug", "dub")
	if err != nil || strings.Join(dub, ",") != "1" {
		t.Fatalf("dub episodes = %v err=%v", dub, err)
	}
}

func TestGetEpisodeStreamsSubAndDub(t *testing.T) {
	newAnimexFixture(t)

	links, _, err := getEpisodeStreamsForMode("frieren-beyond-slug", providers.PlaybackConfig{SubOrDub: "sub", SubStyle: "soft"}, 1)
	if err != nil {
		t.Fatalf("getEpisodeStreamsForMode sub: %v", err)
	}
	if len(links) != 1 || !strings.HasSuffix(links[0], "/master.m3u8") {
		t.Fatalf("unexpected links %#v", links)
	}

	links, _, err = getEpisodeStreamsForMode("frieren-beyond-slug", providers.PlaybackConfig{SubOrDub: "dub"}, 1)
	if err != nil {
		t.Fatalf("getEpisodeStreamsForMode dub: %v", err)
	}
	if len(links) != 1 || !strings.HasSuffix(links[0], "/dub.m3u8") {
		t.Fatalf("dub must resolve the dub stream, got %#v", links)
	}
}

func TestGetEpisodeStreamsRejectsBadInput(t *testing.T) {
	newAnimexFixture(t)

	for _, epNo := range []int{0, -1} {
		if _, _, err := getEpisodeStreamsForMode("frieren-beyond-slug", providers.PlaybackConfig{SubOrDub: "sub"}, epNo); err == nil {
			t.Fatalf("ep %d must error", epNo)
		}
	}
	if _, _, err := getEpisodeStreamsForMode("", providers.PlaybackConfig{SubOrDub: "sub"}, 1); err == nil {
		t.Fatal("empty show id must error")
	}
}
