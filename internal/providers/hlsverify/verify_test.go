package hlsverify

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/wraient/curd/internal/curdhost"
)

func useClient(t *testing.T, client *http.Client) {
	t.Helper()
	previous := curdhost.HTTPClient
	curdhost.HTTPClient = func() *http.Client { return client }
	t.Cleanup(func() { curdhost.HTTPClient = previous })
}

func newValidatorServer(t *testing.T, serve func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(serve))
	t.Cleanup(server.Close)
	useClient(t, server.Client())
	return server
}

func TestValidateRejectsLeadingDecoySegment(t *testing.T) {
	var server *httptest.Server
	server = newValidatorServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/master.m3u8"):
			fmt.Fprintf(w, "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1000000\n%s/media.m3u8\n", server.URL)
		case strings.HasSuffix(r.URL.Path, "/media.m3u8"):
			fmt.Fprintf(w, "#EXTM3U\n#EXTINF:4.0,\nhttps://p16-ad-site-sign-sg.tiktokcdn.com/ad-site-i18n-sg/ad.image\n#EXTINF:4.0,\n%s/seg-1.jpg\n#EXT-X-ENDLIST\n", server.URL)
		default:
			w.Write([]byte{0x47, 0x40, 0x11, 0x10})
		}
	})

	err := Validate(server.URL+"/master.m3u8", Options{})
	if err == nil || !strings.Contains(err.Error(), "begins with an ad/decoy segment") {
		t.Fatalf("expected leading-decoy rejection, got %v", err)
	}
}

func TestValidateRejectsMajorityDecoy(t *testing.T) {
	var server *httptest.Server
	server = newValidatorServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/master.m3u8"):
			fmt.Fprintf(w, "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1000000\n%s/media.m3u8\n", server.URL)
		default:
			fmt.Fprintf(w, "#EXTM3U\n#EXTINF:4.0,\n%s/seg-1.jpg\n#EXTINF:4.0,\nhttps://x.byteimg.com/a\n#EXTINF:4.0,\nhttps://x.ibyteimg.com/b\n#EXT-X-ENDLIST\n", server.URL)
		}
	})

	err := Validate(server.URL+"/master.m3u8", Options{})
	if err == nil || !strings.Contains(err.Error(), "ad-injected decoy") {
		t.Fatalf("expected majority-decoy rejection, got %v", err)
	}
}

func TestValidateAcceptsCleanStream(t *testing.T) {
	var server *httptest.Server
	server = newValidatorServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/master.m3u8"):
			fmt.Fprintf(w, "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1000000\n%s/media.m3u8\n", server.URL)
		case strings.HasSuffix(r.URL.Path, "/media.m3u8"):
			fmt.Fprintf(w, "#EXTM3U\n#EXTINF:4.0,\n%s/seg-1.jpg\n#EXT-X-ENDLIST\n", server.URL)
		default:
			w.Write([]byte{0x47, 0x40, 0x11, 0x10})
		}
	})

	if err := Validate(server.URL+"/master.m3u8", Options{}); err != nil {
		t.Fatalf("expected clean stream to validate, got %v", err)
	}
}

func TestValidateRejectsImageFirstSegment(t *testing.T) {
	var server *httptest.Server
	server = newValidatorServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/master.m3u8"):
			fmt.Fprintf(w, "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1000000\n%s/media.m3u8\n", server.URL)
		case strings.HasSuffix(r.URL.Path, "/media.m3u8"):
			fmt.Fprintf(w, "#EXTM3U\n#EXTINF:4.0,\n%s/seg-1.jpg\n#EXT-X-ENDLIST\n", server.URL)
		default:
			w.Write([]byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A})
		}
	})

	err := Validate(server.URL+"/master.m3u8", Options{})
	if err == nil || !strings.Contains(err.Error(), "not video content") {
		t.Fatalf("expected non-video rejection, got %v", err)
	}
}

func TestSelectMediaPlaylistURL(t *testing.T) {
	master, _ := url.Parse("https://cdn.example/anime/master.m3u8")
	body := "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=800000\nlow.m3u8\n#EXT-X-STREAM-INF:BANDWIDTH=2000000\nhigh.m3u8\n"
	got, err := SelectMediaPlaylistURL(master, body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "https://cdn.example/anime/high.m3u8" {
		t.Fatalf("got %q, want highest-bandwidth variant", got)
	}
}

func TestLooksLikeDecoySegment(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want bool
	}{
		{"png", []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, true},
		{"jpeg", []byte{0xFF, 0xD8, 0xFF, 0xE0}, true},
		{"webp", append([]byte("RIFF\x00\x00\x00\x00WEBP"), 0x00), true},
		{"html", []byte("<html><body>ad</body></html>"), true},
		{"mpegts", []byte{0x47, 0x40, 0x11, 0x10}, false},
		{"short", []byte{0x47}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := LooksLikeDecoySegment(tc.data); got != tc.want {
				t.Fatalf("LooksLikeDecoySegment = %v, want %v", got, tc.want)
			}
		})
	}
}
