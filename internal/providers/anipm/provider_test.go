package anipm

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/wraient/curd/internal/curdhost"
	"github.com/wraient/curd/internal/providers"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func withAnipmTestClient(t *testing.T, client *http.Client) {
	t.Helper()
	previous := curdhost.HTTPClient
	t.Cleanup(func() { curdhost.HTTPClient = previous })
	curdhost.HTTPClient = func() *http.Client { return client }
}

type fixture struct {
	mu          sync.Mutex
	lastChannel string
	server      *httptest.Server
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/anime/search", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"items":[{"id":6351,"routeId":"frieren-6351-rblfpv","slug":"frieren-c6fbj","title":"Frieren: Beyond Journey's End","year":2023,"type":"TV","episodeCount":28,"subCount":28,"dubCount":28,"malId":52991,"poster":"/api/anime/cover?key=abc"}]}`)
	})
	mux.HandleFunc("/api/anime/series/6351", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"id":6351,"title":"Frieren","episodes":[{"number":1,"sub":true,"dub":true},{"number":2,"sub":true,"dub":false}]}`)
	})
	mux.HandleFunc("/api/anime/playback-bootstrap/settlar/6351", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"episode":1,"effectiveLanguage":"sub","settlarSelection":"sel123","availability":{"sub":true,"dub":true},"anipmPackages":{"episodes":{"1":{"sub":true,"dub":true,"subhard":true},"2":{"sub":true,"dub":false,"subhard":false}}}}`)
	})
	mux.HandleFunc("/api/anime/settlar/session", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.lastChannel = r.URL.Query().Get("channel")
		f.mu.Unlock()
		_, _ = io.WriteString(w, `{"embedUrl":"`+f.server.URL+`/embed/v1?t=tok123"}`)
	})
	mux.HandleFunc("/embed/v1", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "session", Value: "abc", Path: "/"})
		_, _ = io.WriteString(w, `<html>player</html>`)
	})
	mux.HandleFunc("/api/embed/session", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"kind":"hls","source":"`+f.server.URL+`/master.m3u8","audioLang":"ja","subtitles":[{"url":"`+f.server.URL+`/en.vtt","assUrl":"`+f.server.URL+`/en.ass","label":"English","srclang":"en","default":true}]}`)
	})
	mux.HandleFunc("/master.m3u8", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1,RESOLUTION=1280x720\n/media.m3u8\n")
	})
	mux.HandleFunc("/media.m3u8", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "#EXTM3U\n#EXTINF:10.0,\n/seg1.ts\n")
	})
	mux.HandleFunc("/seg1.ts", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte{0x47, 0x40, 0x00, 0x10, 0x02, 0x42, 0xf0, 0x25})
	})
	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)

	withAnipmTestClient(t, f.server.Client())
	originalBase, originalEmbed := baseURL, embedBaseURL
	t.Cleanup(func() { baseURL, embedBaseURL = originalBase, originalEmbed })
	baseURL = f.server.URL
	embedBaseURL = f.server.URL
	return f
}

func (f *fixture) channel() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastChannel
}

func TestSearchAnimeParsesResults(t *testing.T) {
	newFixture(t)

	options, err := searchAnime("frieren", "sub")
	if err != nil {
		t.Fatalf("searchAnime: %v", err)
	}
	if len(options) != 1 || options[0].Key != "frieren-6351-rblfpv" {
		t.Fatalf("unexpected options %#v", options)
	}
	item, ok := options[0].ExtraData.(SearchItem)
	if !ok || item.MalID != 52991 || item.Episodes != 28 {
		t.Fatalf("unexpected extra data %+v", options[0].ExtraData)
	}
}

func TestEpisodesListFiltersByMode(t *testing.T) {
	newFixture(t)

	sub, err := episodesList("6351", "sub")
	if err != nil || strings.Join(sub, ",") != "1,2" {
		t.Fatalf("sub episodes = %v err=%v", sub, err)
	}
	dub, err := episodesList("6351", "dub")
	if err != nil || strings.Join(dub, ",") != "1" {
		t.Fatalf("dub episodes = %v err=%v", dub, err)
	}
}

func TestGetEpisodeStreamsSoftSub(t *testing.T) {
	f := newFixture(t)

	links, hints, err := getEpisodeStreamsForMode("6351", providers.PlaybackConfig{SubOrDub: "sub", SubStyle: "soft"}, 1)
	if err != nil {
		t.Fatalf("getEpisodeStreamsForMode: %v", err)
	}
	if len(links) != 1 || !strings.HasSuffix(links[0], "/master.m3u8") {
		t.Fatalf("unexpected links %#v", links)
	}
	if f.channel() != "sub" {
		t.Fatalf("expected soft sub channel, got %q", f.channel())
	}
	if !strings.HasSuffix(hints[links[0]].Subtitle, "/en.ass") {
		t.Fatalf("expected ASS subtitle hint, got %q", hints[links[0]].Subtitle)
	}
}

func TestGetEpisodeStreamsHardSubUsesSubhardChannel(t *testing.T) {
	f := newFixture(t)

	_, _, err := getEpisodeStreamsForMode("6351", providers.PlaybackConfig{SubOrDub: "sub", SubStyle: "hard"}, 1)
	if err != nil {
		t.Fatalf("getEpisodeStreamsForMode hard: %v", err)
	}
	if f.channel() != "subhard" {
		t.Fatalf("expected subhard channel, got %q", f.channel())
	}
}

func TestGetEpisodeStreamsDub(t *testing.T) {
	f := newFixture(t)

	links, hints, err := getEpisodeStreamsForMode("6351", providers.PlaybackConfig{SubOrDub: "dub"}, 1)
	if err != nil {
		t.Fatalf("getEpisodeStreamsForMode dub: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("unexpected links %#v", links)
	}
	if f.channel() != "dub" {
		t.Fatalf("expected dub channel, got %q", f.channel())
	}
	if hints[links[0]].Subtitle != "" {
		t.Fatalf("expected no subtitle hint for dub, got %q", hints[links[0]].Subtitle)
	}
}

func TestGetEpisodeStreamsRejectsBadInput(t *testing.T) {
	newFixture(t)

	for _, epNo := range []int{0, -1} {
		if _, _, err := getEpisodeStreamsForMode("6351", providers.PlaybackConfig{SubOrDub: "sub"}, epNo); err == nil {
			t.Fatalf("ep %d must error", epNo)
		}
	}
	if _, _, err := getEpisodeStreamsForMode("", providers.PlaybackConfig{SubOrDub: "sub"}, 1); err == nil {
		t.Fatal("empty show id must error")
	}
	if _, _, err := getEpisodeStreamsForMode("no-such-show", providers.PlaybackConfig{SubOrDub: "sub"}, 1); err == nil {
		t.Fatal("unknown show id must error")
	}
}
