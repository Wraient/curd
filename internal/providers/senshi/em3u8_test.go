package senshi

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wraient/curd/internal/curdhost"
)

func TestDecryptSenshiPlaylist(t *testing.T) {
	plain := "#EXTM3U\nsegment.jpg\n"
	block, err := aes.NewCipher(senshiFallbackPlaylistKey)
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	nonce := []byte("123456789012")
	payload := append(append([]byte(nil), nonce...), gcm.Seal(nil, nonce, []byte(plain), nil)...)
	encrypted := senshiEncryptedPlaylistPrefix + base64.StdEncoding.EncodeToString(payload)

	senshiPlaylistKeyMu.Lock()
	senshiPlaylistKey = append([]byte(nil), senshiFallbackPlaylistKey...)
	senshiPlaylistKeyMu.Unlock()
	got, err := decryptSenshiPlaylist([]byte(encrypted))
	if err != nil {
		t.Fatalf("decryptSenshiPlaylist: %v", err)
	}
	if got != plain {
		t.Fatalf("got %q, want %q", got, plain)
	}
}

func TestSenshiProxyRewritesNestedPlaylistsAndSegments(t *testing.T) {
	proxy := &senshiProxy{baseURL: "http://127.0.0.1:1234"}
	manifest := "#EXTM3U\n#EXT-X-MEDIA:TYPE=AUDIO,URI=\"audio/0_ja.txt\"\nvideo/main.m3u8\nsegments/one.jpg?sig=x\n"
	got := proxy.rewritePlaylist("session", "https://cdn.example/path/master.txt", manifest)

	if !strings.Contains(got, "http://127.0.0.1:1234/stream/session?url=") {
		t.Fatalf("nested playlists were not proxied:\n%s", got)
	}
	if !strings.Contains(got, "segments%2Fone.jpg") {
		t.Fatalf("segment URL was not proxied:\n%s", got)
	}
}

func TestFilterSenshiAudioRenditions(t *testing.T) {
	manifest := "#EXTM3U\n#EXT-X-MEDIA:TYPE=AUDIO,DEFAULT=NO,URI=\"audio/0_ja.txt\"\n#EXT-X-MEDIA:TYPE=AUDIO,DEFAULT=NO,URI=\"audio/1_en.txt\"\n"
	got := filterSenshiAudioRenditions(manifest, "1_en")
	if strings.Contains(got, "0_ja") || !strings.Contains(got, "1_en") || !strings.Contains(got, "DEFAULT=YES") {
		t.Fatalf("unexpected filtered manifest:\n%s", got)
	}
}

func TestRefreshSenshiPlaylistKeyFromSiteBundles(t *testing.T) {
	first := make([]string, 32)
	second := make([]string, 32)
	for i := range first {
		first[i] = fmt.Sprintf("%d", i)
		second[i] = fmt.Sprintf("%d", 255-i)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			_, _ = io.WriteString(w, `<script src="/assets/index-example.js"></script>`)
		case "/assets/index-example.js":
			_, _ = io.WriteString(w, `import("./WatchPage-example.js")`)
		case "/assets/WatchPage-example.js":
			_, _ = io.WriteString(w, "Uint8Array.from(["+strings.Join(first, ",")+"]);Uint8Array.from(["+strings.Join(second, ",")+"])")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	previousBase := baseURL
	previousClient := curdhost.HTTPClient
	baseURL = server.URL
	curdhost.HTTPClient = func() *http.Client { return server.Client() }
	t.Cleanup(func() {
		baseURL = previousBase
		curdhost.HTTPClient = previousClient
		senshiPlaylistKeyMu.Lock()
		senshiPlaylistKey = append([]byte(nil), senshiFallbackPlaylistKey...)
		senshiPlaylistKeyMu.Unlock()
	})

	key, err := refreshSenshiPlaylistKey()
	if err != nil {
		t.Fatalf("refreshSenshiPlaylistKey: %v", err)
	}
	if len(key) != 32 {
		t.Fatalf("key length = %d", len(key))
	}
	for i, value := range key {
		if value != 255 {
			t.Fatalf("key[%d] = %d, want 255", i, value)
		}
	}
}
