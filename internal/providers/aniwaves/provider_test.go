package aniwaves

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wraient/curd/internal/curdhost"
	"github.com/wraient/curd/internal/providers"
)

func withAniwavesTestClient(t *testing.T, client *http.Client) {
	t.Helper()
	previous := curdhost.HTTPClient
	t.Cleanup(func() { curdhost.HTTPClient = previous })
	curdhost.HTTPClient = func() *http.Client { return client }
}

func newAniwavesFixture(t *testing.T) *httptest.Server {
	t.Helper()
	var server *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/ajax/anime/search", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		_, _ = io.WriteString(w, `{"status":200,"result":{"html":"<div class=\"scaff items\"><a class=\"item\" href=\"/watch/sousou-no-frieren-74575\"><div class=\"poster\"><span><img src=\"https://cdn.example/frieren.jpg\"/></span></div><div class=\"info\"><div class=\"name d-title\" data-jp=\"Sousou no Frieren\">Frieren: Beyond Journey&#039;s End</div><div class=\"meta\"><span class=\"dot\">TV</span></div></div></a></div>"}}`)
	})
	mux.HandleFunc("/ajax/episode/list/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"status":200,"result":"<li><a href=\"/watch/123/ep-1\" data-num=\"1\" data-sub=\"1\" data-dub=\"1\">1</a></li><li><a href=\"/watch/123/ep-2\" data-num=\"2\" data-sub=\"1\" data-dub=\"0\">2</a></li>"}`)
	})
	mux.HandleFunc("/ajax/server/list", func(w http.ResponseWriter, r *http.Request) {
		// Episode 2 has no dub server in this fixture.
		dub := `<div class=\"type\" data-type=\"dub\"><ul><li data-link-id=\"f6e5d4c3b2\"></li></ul></div>`
		if strings.Contains(r.URL.RawQuery, "eps=2") {
			dub = ""
		}
		_, _ = io.WriteString(w, `{"status":200,"result":"<div class=\"servers\"><div class=\"type\" data-type=\"sub\"><ul><li data-link-id=\"a1b2c3d4e5\"></li></ul></div>`+dub+`</div>"}`)
	})
	mux.HandleFunc("/ajax/sources", func(w http.ResponseWriter, r *http.Request) {
		// Sub and dub server links resolve to distinct embeds so tests can
		// prove the requested section actually drove the lookup.
		embed := "/embed-1/abc123?asi=0"
		if r.URL.Query().Get("id") == "f6e5d4c3b2" {
			embed = "/embed-1/dub456?asi=0"
		}
		_, _ = io.WriteString(w, `{"status":200,"result":{"url":"`+server.URL+embed+`","server":4}}`)
	})
	mux.HandleFunc("/embed-1/abc123", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "vh_session", Value: "sess", Path: "/"})
		_, _ = io.WriteString(w, `<html><body data-id="abc123def456"></body></html>`)
	})
	mux.HandleFunc("/embed-1/dub456", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "vh_session", Value: "sess", Path: "/"})
		_, _ = io.WriteString(w, `<html><body data-id="dub456data789"></body></html>`)
	})
	mux.HandleFunc("/embed-1/getSources", func(w http.ResponseWriter, r *http.Request) {
		playlist := ""
		switch r.URL.Query().Get("id") {
		case "abc123def456":
			playlist = "/master.m3u8"
		case "dub456data789":
			playlist = "/dub.m3u8"
		default:
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if _, err := r.Cookie("vh_session"); err != nil {
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `{"error":"missing session"}`)
			return
		}
		_, _ = io.WriteString(w, `{"sources":"`+server.URL+playlist+`","tracks":[]}`)
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

	withAniwavesTestClient(t, server.Client())
	originalBase := baseURL
	t.Cleanup(func() { baseURL = originalBase })
	baseURL = server.URL
	return server
}

func TestSearchAnimeParsesResults(t *testing.T) {
	newAniwavesFixture(t)

	options, err := searchAnime("frieren", "sub")
	if err != nil {
		t.Fatalf("searchAnime: %v", err)
	}
	if len(options) != 1 || options[0].Key != "sousou-no-frieren-74575" {
		t.Fatalf("unexpected options %#v", options)
	}
	item, ok := options[0].ExtraData.(SearchItem)
	if !ok || item.JapaneseTitle != "Sousou no Frieren" || item.Type != "TV" {
		t.Fatalf("unexpected extra data %+v", options[0].ExtraData)
	}
	if !strings.Contains(options[0].Label, "TV") {
		t.Fatalf("label should carry the type: %q", options[0].Label)
	}
}

func TestEpisodesListFiltersByMode(t *testing.T) {
	newAniwavesFixture(t)

	sub, err := episodesList("sousou-no-frieren-74575", "sub")
	if err != nil || strings.Join(sub, ",") != "1,2" {
		t.Fatalf("sub episodes = %v err=%v", sub, err)
	}
	dub, err := episodesList("sousou-no-frieren-74575", "dub")
	if err != nil || strings.Join(dub, ",") != "1" {
		t.Fatalf("dub episodes = %v err=%v", dub, err)
	}
	if _, err := episodesList("no-id-here", "sub"); err == nil {
		t.Fatal("non-numeric show id must error")
	}
}

func TestGetEpisodeStreamsSubAndDub(t *testing.T) {
	newAniwavesFixture(t)

	links, hints, err := getEpisodeStreamsForMode("sousou-no-frieren-74575", providers.PlaybackConfig{SubOrDub: "sub", SubStyle: "soft"}, 1)
	if err != nil {
		t.Fatalf("getEpisodeStreamsForMode sub: %v", err)
	}
	if len(links) != 1 || !strings.HasSuffix(links[0], "/master.m3u8") {
		t.Fatalf("unexpected links %#v", links)
	}
	if !strings.Contains(hints[links[0]].Referrer, "/embed-1/abc123") {
		t.Fatalf("referrer must be the embed page, got %q", hints[links[0]].Referrer)
	}

	dubLinks, _, err := getEpisodeStreamsForMode("sousou-no-frieren-74575", providers.PlaybackConfig{SubOrDub: "dub"}, 2)
	if err == nil || len(dubLinks) != 0 {
		t.Fatalf("ep2 has no dub server, expected error got %#v %v", dubLinks, err)
	}
}

func TestGetEpisodeStreamsDubUsesDubSection(t *testing.T) {
	newAniwavesFixture(t)

	// Episode 1 lists both sections; the dub lookup must resolve the dub
	// embed, proving section attribution instead of falling back to sub.
	links, hints, err := getEpisodeStreamsForMode("sousou-no-frieren-74575", providers.PlaybackConfig{SubOrDub: "dub"}, 1)
	if err != nil {
		t.Fatalf("getEpisodeStreamsForMode dub ep1: %v", err)
	}
	if len(links) != 1 || !strings.HasSuffix(links[0], "/dub.m3u8") {
		t.Fatalf("dub must resolve the dub stream, got %#v", links)
	}
	if !strings.Contains(hints[links[0]].Referrer, "/embed-1/dub456") {
		t.Fatalf("referrer must be the dub embed page, got %q", hints[links[0]].Referrer)
	}
}

func TestSearchAnimeDoesNotShiftFieldsAcrossResults(t *testing.T) {
	blocks := itemBlockRE.FindAllString(`<a class="item" href="/watch/first-1"><div class="poster"><img src="https://cdn.example/1.jpg"/></div><div class="info"><div class="name d-title" data-jp="First JP">First</div><div class="meta"><span class="dot">TV</span></div></div></a><a class="item" href="/watch/second-2"><div class="info"><div class="name d-title">Second</div></div></a>`, -1)
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}
	_, _, jp1, typ1, thumb1 := parseSearchItem(blocks[0])
	path2, title2, jp2, typ2, thumb2 := parseSearchItem(blocks[1])
	if jp1 != "First JP" || typ1 != "TV" || thumb1 != "https://cdn.example/1.jpg" {
		t.Fatalf("first block misparsed: %q %q %q", jp1, typ1, thumb1)
	}
	if path2 != "/watch/second-2" || title2 != "Second" || jp2 != "" || typ2 != "" || thumb2 != "" {
		t.Fatalf("missing fields leaked into second block: %q %q %q %q %q", path2, title2, jp2, typ2, thumb2)
	}
}

func TestParseSearchItemToleratesMissingFields(t *testing.T) {
	path, title, jp, typ, thumb := parseSearchItem(`<a class="item" href="/watch/no-meta-show-1"><div class="info"><div class="name d-title">No Meta Show</div></div></a>`)
	if path != "/watch/no-meta-show-1" || title != "No Meta Show" || jp != "" || typ != "" || thumb != "" {
		t.Fatalf("missing-field block misparsed: %q %q %q %q %q", path, title, jp, typ, thumb)
	}
}

func TestParseEpisodeTagToleratesMissingFlags(t *testing.T) {
	n, sub, dub, ok := parseEpisodeTag(`<a href="/watch/1/ep-3" data-num="3"></a>`)
	if !ok || n != 3 || sub || dub {
		t.Fatalf("got %d %v %v %v", n, sub, dub, ok)
	}
	if _, _, _, ok := parseEpisodeTag(`<a class="x" href="/watch/1"></a>`); ok {
		t.Fatal("tag without data-num must not parse")
	}
}

func TestGetEpisodeStreamsRejectsBadInput(t *testing.T) {
	newAniwavesFixture(t)

	for _, epNo := range []int{0, -1} {
		if _, _, err := getEpisodeStreamsForMode("sousou-no-frieren-74575", providers.PlaybackConfig{SubOrDub: "sub"}, epNo); err == nil {
			t.Fatalf("ep %d must error", epNo)
		}
	}
	if _, _, err := getEpisodeStreamsForMode("", providers.PlaybackConfig{SubOrDub: "sub"}, 1); err == nil {
		t.Fatal("empty show id must error")
	}
}
