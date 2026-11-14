package anikoto

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wraient/curd/internal/curdhost"
	"github.com/wraient/curd/internal/providers"
)

func cleanStreamServer(t *testing.T) *httptest.Server {
	t.Helper()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/master.m3u8"):
			fmt.Fprintf(w, "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1000000\n%s/media.m3u8\n", server.URL)
		case strings.HasSuffix(r.URL.Path, "/media.m3u8"):
			fmt.Fprintf(w, "#EXTM3U\n#EXTINF:4.0,\n%s/seg-1.jpg\n#EXT-X-ENDLIST\n", server.URL)
		default:
			w.Write([]byte{0x47, 0x40, 0x11, 0x10})
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func adInjectedStreamServer(t *testing.T) *httptest.Server {
	t.Helper()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/master.m3u8"):
			fmt.Fprintf(w, "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1000000\n%s/media.m3u8\n", server.URL)
		case strings.HasSuffix(r.URL.Path, "/media.m3u8"):
			fmt.Fprintf(w, "#EXTM3U\n#EXTINF:4.0,\nhttps://p16-ad-site-sign-sg.tiktokcdn.com/ad-site-i18n-sg/ad.image\n#EXTINF:4.0,\n%s/seg-1.jpg\n#EXT-X-ENDLIST\n", server.URL)
		default:
			w.Write([]byte{0x47, 0x40, 0x11, 0x10})
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func withAnikotoValidatorClient(t *testing.T, client *http.Client) {
	t.Helper()
	previous := curdhost.HTTPClient
	curdhost.HTTPClient = func() *http.Client { return client }
	t.Cleanup(func() { curdhost.HTTPClient = previous })
}

func TestPrioritizePlayableStreamsMovesCleanStreamsFirst(t *testing.T) {
	clean := cleanStreamServer(t)
	advertised := adInjectedStreamServer(t)
	withAnikotoValidatorClient(t, clean.Client())

	adLink := advertised.URL + "/master.m3u8"
	cleanLink := clean.URL + "/master.m3u8"
	links := []string{adLink, cleanLink}
	hints := map[string]providers.StreamPlaybackHint{
		adLink:    {Referrer: "https://megaplay.buzz/"},
		cleanLink: {Referrer: "https://megaplay.buzz/"},
	}

	got := prioritizePlayableStreams(links, hints)
	if len(got) != 2 {
		t.Fatalf("expected 2 links, got %d", len(got))
	}
	if got[0] != cleanLink {
		t.Fatalf("expected clean stream first, got %q", got[0])
	}
	if got[1] != adLink {
		t.Fatalf("expected ad stream last, got %q", got[1])
	}
}

func TestPrioritizePlayableStreamsKeepsOrderWhenAllValid(t *testing.T) {
	clean := cleanStreamServer(t)
	withAnikotoValidatorClient(t, clean.Client())

	first := clean.URL + "/master.m3u8"
	second := clean.URL + "/other/master.m3u8"
	links := []string{first, second}
	hints := map[string]providers.StreamPlaybackHint{
		first:  {Referrer: "https://megaplay.buzz/"},
		second: {Referrer: "https://megaplay.buzz/"},
	}

	got := prioritizePlayableStreams(links, hints)
	if got[0] != first || got[1] != second {
		t.Fatalf("expected order preserved, got %v", got)
	}
}

func TestPrioritizePlayableStreamsSkipsSingleLink(t *testing.T) {
	withAnikotoValidatorClient(t, nil)

	link := "https://unreachable.example/master.m3u8"
	got := prioritizePlayableStreams([]string{link}, map[string]providers.StreamPlaybackHint{
		link: {Referrer: "https://megaplay.buzz/"},
	})
	if len(got) != 1 || got[0] != link {
		t.Fatalf("expected single link unchanged, got %v", got)
	}
}
