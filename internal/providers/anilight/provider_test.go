package anilight

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/wraient/curd/internal/curdhost"
	"github.com/wraient/curd/internal/providers"
)

func withAnilightTestClient(t *testing.T, client *http.Client) {
	t.Helper()
	previous := curdhost.HTTPClient
	t.Cleanup(func() { curdhost.HTTPClient = previous })
	curdhost.HTTPClient = func() *http.Client { return client }
}

type anilightFixture struct {
	server *httptest.Server
}

func newAnilightFixture(t *testing.T) *anilightFixture {
	t.Helper()
	f := &anilightFixture{}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/search", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"id":156,"slug":"sousou-no-frieren","anilistId":154587,"idMal":52991,"title":{"romaji":"Sousou no Frieren","english":"Frieren: Beyond Journey's End","native":"葬送のフリーレン"},"coverImage":{"large":"https://cdn.example/frieren.jpg"},"format":"TV","seasonYear":2023,"episodes":28,"subEpisodes":28,"dubEpisodes":28}]`))
	})
	mux.HandleFunc("/api/anime/sousou-no-frieren", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"slug":"sousou-no-frieren","episodes":28,"subEpisodes":28,"dubEpisodes":28}`))
	})
	mux.HandleFunc("/api/sources/verify", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"sessionToken":"tok123","expiresAt":9999999999999}`))
	})
	mux.HandleFunc("/api/sources", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-source-session") != "tok123" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		// The lookup is keyed on show, episode, and language: requests
		// missing any of them are rejected like the real API.
		query := r.URL.Query()
		if query.Get("slug") == "" || query.Get("episode") == "" || query.Get("language") == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		// Sub and dub resolve to distinct files so tests can prove the
		// requested language actually reached the lookup.
		video, tracks := "/e/vid123", `[{"id":"English","url":"/e/sub123","kind":"captions","lang":"English","label":"English","default":true}]`
		if query.Get("language") == "dub" {
			video, tracks = "/e/vid456", `[]`
		}
		_, _ = w.Write([]byte(`{"sources":[{"url":"` + video + `","quality":"1080p","type":"mp4"}],"tracks":` + tracks + `}`))
	})
	mux.HandleFunc("/e/vid123", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Accept-Ranges", "bytes")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte{0x00, 0x01})
	})
	mux.HandleFunc("/e/vid456", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Accept-Ranges", "bytes")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte{0x00, 0x01})
	})
	mux.HandleFunc("/e/sub123", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("WEBVTT\n\n00:00.000 --> 00:01.000\nHi\n"))
	})
	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)

	withAnilightTestClient(t, f.server.Client())
	originalBase, originalCDN := baseURL, streamCDN
	t.Cleanup(func() { baseURL, streamCDN = originalBase, originalCDN })
	baseURL = f.server.URL
	streamCDN = f.server.URL
	resetSessionForTest()
	return f
}

func TestSearchAnimeParsesResults(t *testing.T) {
	newAnilightFixture(t)

	options, err := searchAnime("frieren", "sub")
	if err != nil {
		t.Fatalf("searchAnime: %v", err)
	}
	if len(options) != 1 || options[0].Key != "sousou-no-frieren" {
		t.Fatalf("unexpected options %#v", options)
	}
	item, ok := options[0].ExtraData.(SearchItem)
	if !ok || item.MalID != 52991 || item.Episodes != 28 || item.Type != "TV" {
		t.Fatalf("unexpected extra data %+v", options[0].ExtraData)
	}
	if item.Romaji != "Sousou no Frieren" {
		t.Fatalf("expected romaji title, got %+v", item)
	}
}

func TestEpisodesListFiltersByMode(t *testing.T) {
	newAnilightFixture(t)

	sub, err := episodesList("sousou-no-frieren", "sub")
	if err != nil || len(sub) != 28 || sub[0] != "1" {
		t.Fatalf("sub episodes = %d err=%v", len(sub), err)
	}
	dub, err := episodesList("sousou-no-frieren", "dub")
	if err != nil || len(dub) != 28 {
		t.Fatalf("dub episodes = %d err=%v", len(dub), err)
	}
}

func TestGetEpisodeStreamsSoftSub(t *testing.T) {
	newAnilightFixture(t)

	links, hints, err := getEpisodeStreamsForMode("sousou-no-frieren", providers.PlaybackConfig{SubOrDub: "sub", SubStyle: "soft"}, 1)
	if err != nil {
		t.Fatalf("getEpisodeStreamsForMode: %v", err)
	}
	if len(links) != 1 || !strings.HasSuffix(links[0], "/e/vid123") {
		t.Fatalf("unexpected links %#v", links)
	}
	if !strings.HasSuffix(hints[links[0]].Subtitle, "/e/sub123") {
		t.Fatalf("expected English subtitle hint, got %q", hints[links[0]].Subtitle)
	}
}

func TestGetEpisodeStreamsHardSubFallsBackToSoft(t *testing.T) {
	newAnilightFixture(t)

	// No hardsub channel exists; the shared substyle contract degrades an
	// explicit hard preference to a soft stream instead of failing.
	links, _, err := getEpisodeStreamsForMode("sousou-no-frieren", providers.PlaybackConfig{SubOrDub: "sub", SubStyle: "hard"}, 1)
	if err != nil {
		t.Fatalf("hard preference must degrade to soft, got error: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("unexpected links %#v", links)
	}
}

func TestGetEpisodeStreamsDub(t *testing.T) {
	newAnilightFixture(t)

	links, hints, err := getEpisodeStreamsForMode("sousou-no-frieren", providers.PlaybackConfig{SubOrDub: "dub"}, 1)
	if err != nil {
		t.Fatalf("getEpisodeStreamsForMode dub: %v", err)
	}
	if len(links) != 1 || !strings.HasSuffix(links[0], "/e/vid456") {
		t.Fatalf("dub must resolve the dub file, got %#v", links)
	}
	if hints[links[0]].Subtitle != "" {
		t.Fatalf("dub must have no subtitle hint, got %q", hints[links[0]].Subtitle)
	}
}

func TestGetEpisodeStreamsRejectsBadInput(t *testing.T) {
	newAnilightFixture(t)

	for _, epNo := range []int{0, -1} {
		if _, _, err := getEpisodeStreamsForMode("sousou-no-frieren", providers.PlaybackConfig{SubOrDub: "sub"}, epNo); err == nil {
			t.Fatalf("ep %d must error", epNo)
		}
	}
	if _, _, err := getEpisodeStreamsForMode("", providers.PlaybackConfig{SubOrDub: "sub"}, 1); err == nil {
		t.Fatal("empty show id must error")
	}
}

func TestStaleSessionTokenTriggersRehandshake(t *testing.T) {
	newAnilightFixture(t)

	// Poison the cache with a token the fixture rejects: the lookup must
	// handshake again and succeed instead of failing until expiry.
	setSessionForTest("stale-token", 9999999999999)
	links, _, err := getEpisodeStreamsForMode("sousou-no-frieren", providers.PlaybackConfig{SubOrDub: "sub", SubStyle: "soft"}, 1)
	if err != nil {
		t.Fatalf("stale token must trigger a re-handshake, got: %v", err)
	}
	if len(links) != 1 || !strings.HasSuffix(links[0], "/e/vid123") {
		t.Fatalf("unexpected links %#v", links)
	}
}

func TestSessionTokenCachedAndThreadSafe(t *testing.T) {
	newAnilightFixture(t)

	const workers = 8
	var wg sync.WaitGroup
	tokens := make([]string, workers)
	errs := make([]error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			tokens[n], errs[n] = sessionTokenForSources()
		}(i)
	}
	wg.Wait()
	for i := range errs {
		if errs[i] != nil || tokens[i] != "tok123" {
			t.Fatalf("worker %d: token=%q err=%v", i, tokens[i], errs[i])
		}
	}
}
