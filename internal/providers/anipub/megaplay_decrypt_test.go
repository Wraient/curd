package anipub

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testMegaplayEnc = "wdeBruh3qqn_i5wUNnyaPcXqidp1UWP84FfPHzGyKXAz4mAVkH6j3DueswO2yXLWn8H-XMHNvbAo5Gsg7zIcFBuQI_zsUvMGI1gKwQsPTSHQHiF55R4BopgEQ-7jebQQ4C0Gu7YhaMucopp6d3Q8yAY9b5GdsSvPGq6CUn7SHyc"

func TestDecryptMegaplayEnc(t *testing.T) {
	got, err := decryptMegaplayEnc(testMegaplayEnc)
	if err != nil {
		t.Fatalf("decryptMegaplayEnc: %v", err)
	}
	want := "https://fetch.nexabloom.top/anime/bb6d2babd7797d94d8f4a8600bc9b44e/b7d51fb7e838ee9b60dcdb34b953bc07/master.m3u8"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestDecryptMegaplayEncRejectsGarbage(t *testing.T) {
	for _, enc := range []string{"", "!!!", "aGVsbG8="} {
		if _, err := decryptMegaplayEnc(enc); err == nil {
			t.Fatalf("expected error for %q", enc)
		}
	}
}

func TestResolveMegaplayStreamFallsBackToEnc(t *testing.T) {
	megaplay := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/stream/s-2/42/sub":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<div data-id="99"></div>`))
		case "/stream/getSources":
			_ = json.NewEncoder(w).Encode(megaplaySourcesResponse{Enc: testMegaplayEnc})
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

	streamURL, _, err := resolveMegaplayStream("https://anipub.xyz/video/42/sub", "sub")
	if err != nil {
		t.Fatalf("resolveMegaplayStream: %v", err)
	}
	if !strings.HasSuffix(streamURL, "/master.m3u8") {
		t.Fatalf("unexpected stream %q", streamURL)
	}
}
