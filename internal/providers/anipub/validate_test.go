package anipub

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestSelectMediaPlaylistURL(t *testing.T) {
	master := func() *url.URL { u, _ := url.Parse("https://megap.kotocdn.site/abc/master.m3u8"); return u }

	t.Run("media playlist with no variants returns master", func(t *testing.T) {
		got, err := selectMediaPlaylistURL(master(), "#EXTM3U\n#EXTINF:5,\nhttps://seg/1.ts\n")
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if got != "https://megap.kotocdn.site/abc/master.m3u8" {
			t.Fatalf("unexpected %q", got)
		}
	})

	t.Run("picks highest bandwidth relative variant", func(t *testing.T) {
		body := "#EXTM3U\n" +
			"#EXT-X-STREAM-INF:BANDWIDTH=500000\nlow.m3u8\n" +
			"#EXT-X-STREAM-INF:BANDWIDTH=2000000\nhigh.m3u8\n"
		got, err := selectMediaPlaylistURL(master(), body)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if got != "https://megap.kotocdn.site/abc/high.m3u8" {
			t.Fatalf("unexpected %q", got)
		}
	})

	t.Run("picks highest bandwidth absolute variant", func(t *testing.T) {
		body := "#EXTM3U\n" +
			"#EXT-X-STREAM-INF:BANDWIDTH=2000000\nhttps://cdn.example/high.m3u8\n" +
			"#EXT-X-STREAM-INF:BANDWIDTH=500000\nlow.m3u8\n"
		got, err := selectMediaPlaylistURL(master(), body)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if got != "https://cdn.example/high.m3u8" {
			t.Fatalf("unexpected %q", got)
		}
	})

	t.Run("no variant found errors", func(t *testing.T) {
		body := "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=500000\n"
		if _, err := selectMediaPlaylistURL(master(), body); err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestParsePlaylistSegmentsAndDecoyURIs(t *testing.T) {
	body := "#EXTM3U\n#EXTINF:5,\nhttps://megap.kotocdn.site/seg/1.ts\n#EXTINF:5,\n" +
		"https://p16-ad-sg.ibyteimg.com/obj/ad-site-i18n/decoy\n"
	segments := parsePlaylistSegments(body)
	if len(segments) != 2 {
		t.Fatalf("expected 2 segments, got %d", len(segments))
	}
	if isDecoySegmentURI(segments[0]) {
		t.Fatalf("kotocdn segment misclassified as decoy")
	}
	if !isDecoySegmentURI(segments[1]) {
		t.Fatalf("ibyteimg segment not classified as decoy")
	}
}

func TestLooksLikeDecoySegment(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want bool
	}{
		{"png", []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A}, true},
		{"jpeg", []byte{0xFF, 0xD8, 0xFF, 0xE0}, true},
		{"gif", []byte("GIF89a..."), true},
		{"webp", []byte("RIFFxxxxWEBP"), true},
		{"html", []byte("<html><body>error</body></html>"), true},
		{"doctype", []byte("<!DOCTYPE html>"), true},
		{"mpeg ts", []byte{0x47, 0x47, 0x00, 0x1B}, false},
		{"fmp4 ftyp", []byte{0x00, 0x00, 0x00, 0x18, 'f', 't', 'y', 'p'}, false},
		{"short bytes", []byte{0x47, 0x47}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := looksLikeDecoySegment(tc.data); got != tc.want {
				t.Fatalf("looksLikeDecoySegment(%v) = %v, want %v", tc.data, got, tc.want)
			}
		})
	}
}

func TestValidateResolvedStream(t *testing.T) {
	const masterURL = "https://megap.kotocdn.site/abc/master.m3u8"

	newClient := func(handler func(*http.Request) *http.Response) *http.Client {
		return &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if resp := handler(req); resp != nil {
				resp.Request = req
				return resp, nil
			}
			return textResponse(req, http.StatusNotFound, ""), nil
		})}
	}

	text := func(body string) string { return body }

	t.Run("non kotocdn host is skipped", func(t *testing.T) {
		client := newClient(func(req *http.Request) *http.Response {
			t.Fatalf("no requests expected, got %s", req.URL)
			return nil
		})
		withAnipubTestClient(t, client)
		if err := validateResolvedStream("https://cdn.example/master.m3u8"); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("clean stream passes", func(t *testing.T) {
		client := newClient(func(req *http.Request) *http.Response {
			switch {
			case strings.HasSuffix(req.URL.Path, "/master.m3u8"):
				return textResponse(req, http.StatusOK, text("#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1000\nindex.m3u8\n"))
			case strings.HasSuffix(req.URL.Path, "/index.m3u8"):
				return textResponse(req, http.StatusOK, text("#EXTM3U\n#EXTINF:5,\nhttps://megap.kotocdn.site/abc/seg/1.ts\n#EXTINF:5,\nhttps://megap.kotocdn.site/abc/seg/2.ts\n"))
			default:
				return textResponse(req, http.StatusOK, string([]byte{0x47, 0x47, 0x00, 0x1B}))
			}
		})
		withAnipubTestClient(t, client)
		if err := validateResolvedStream(masterURL); err != nil {
			t.Fatalf("expected clean stream to pass, got %v", err)
		}
	})

	t.Run("fully decoy stream is rejected", func(t *testing.T) {
		client := newClient(func(req *http.Request) *http.Response {
			switch {
			case strings.HasSuffix(req.URL.Path, "/master.m3u8"):
				return textResponse(req, http.StatusOK, text("#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1000\nindex.m3u8\n"))
			default:
				return textResponse(req, http.StatusOK, text("#EXTM3U\n#EXTINF:5,\nhttps://p16-ad-sg.ibyteimg.com/obj/ad-site-i18n/x\n#EXTINF:5,\nhttps://p16-ad-sg.ibyteimg.com/obj/ad-site-i18n/y\n"))
			}
		})
		withAnipubTestClient(t, client)
		if err := validateResolvedStream(masterURL); err == nil || !strings.Contains(err.Error(), "decoy") {
			t.Fatalf("expected decoy error, got %v", err)
		}
	})

	t.Run("majority decoy stream is rejected", func(t *testing.T) {
		client := newClient(func(req *http.Request) *http.Response {
			switch {
			case strings.HasSuffix(req.URL.Path, "/master.m3u8"):
				return textResponse(req, http.StatusOK, text("#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1000\nindex.m3u8\n"))
			default:
				return textResponse(req, http.StatusOK, text("#EXTM3U\n#EXTINF:5,\nhttps://megap.kotocdn.site/abc/seg/1.ts\n#EXTINF:5,\nhttps://p16-ad-sg.ibyteimg.com/obj/ad-site-i18n/a\n#EXTINF:5,\nhttps://p16-ad-sg.ibyteimg.com/obj/ad-site-i18n/b\n"))
			}
		})
		withAnipubTestClient(t, client)
		if err := validateResolvedStream(masterURL); err == nil || !strings.Contains(err.Error(), "decoy") {
			t.Fatalf("expected decoy error, got %v", err)
		}
	})

	t.Run("real segment redirecting to decoy is rejected", func(t *testing.T) {
		client := newClient(func(req *http.Request) *http.Response {
			switch {
			case strings.HasSuffix(req.URL.Path, "/master.m3u8"):
				return textResponse(req, http.StatusOK, text("#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1000\nindex.m3u8\n"))
			case strings.HasSuffix(req.URL.Path, "/index.m3u8"):
				return textResponse(req, http.StatusOK, text("#EXTM3U\n#EXTINF:5,\nhttps://megap.kotocdn.site/abc/seg/1.ts\n"))
			case strings.Contains(req.URL.Host, "ibyteimg.com"):
				return textResponse(req, http.StatusOK, string([]byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A}))
			default:
				resp := textResponse(req, http.StatusFound, "")
				resp.Header.Set("Location", "https://p16-ad-sg.ibyteimg.com/obj/ad-site-i18n/decoy")
				return resp
			}
		})
		withAnipubTestClient(t, client)
		if err := validateResolvedStream(masterURL); err == nil || !strings.Contains(err.Error(), "not video content") {
			t.Fatalf("expected probe rejection, got %v", err)
		}
	})

	t.Run("segment probe failure is tolerated", func(t *testing.T) {
		client := newClient(func(req *http.Request) *http.Response {
			switch {
			case strings.HasSuffix(req.URL.Path, "/master.m3u8"):
				return textResponse(req, http.StatusOK, text("#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1000\nindex.m3u8\n"))
			case strings.HasSuffix(req.URL.Path, "/index.m3u8"):
				return textResponse(req, http.StatusOK, text("#EXTM3U\n#EXTINF:5,\nhttps://megap.kotocdn.site/abc/seg/1.ts\n"))
			default:
				return textResponse(req, http.StatusInternalServerError, "")
			}
		})
		withAnipubTestClient(t, client)
		if err := validateResolvedStream(masterURL); err != nil {
			t.Fatalf("expected probe failure to be tolerated, got %v", err)
		}
	})

	t.Run("non 200 manifest is rejected", func(t *testing.T) {
		client := newClient(func(req *http.Request) *http.Response {
			return textResponse(req, http.StatusForbidden, "")
		})
		withAnipubTestClient(t, client)
		if err := validateResolvedStream(masterURL); err == nil {
			t.Fatal("expected manifest error")
		}
	})

	t.Run("non HLS manifest is rejected", func(t *testing.T) {
		client := newClient(func(req *http.Request) *http.Response {
			return textResponse(req, http.StatusOK, text("<html>challenge</html>"))
		})
		withAnipubTestClient(t, client)
		if err := validateResolvedStream(masterURL); err == nil {
			t.Fatal("expected non-hls error")
		}
	})

	t.Run("empty media playlist is rejected", func(t *testing.T) {
		client := newClient(func(req *http.Request) *http.Response {
			switch {
			case strings.HasSuffix(req.URL.Path, "/master.m3u8"):
				return textResponse(req, http.StatusOK, text("#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1000\nindex.m3u8\n"))
			default:
				return textResponse(req, http.StatusOK, text("#EXTM3U\n"))
			}
		})
		withAnipubTestClient(t, client)
		if err := validateResolvedStream(masterURL); err == nil {
			t.Fatal("expected empty playlist error")
		}
	})
}
