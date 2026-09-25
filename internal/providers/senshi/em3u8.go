package senshi

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/wraient/curd/internal/curdhost"
)

const senshiEncryptedPlaylistPrefix = "EM3U8v1:"

var (
	senshiFallbackPlaylistKey, _ = hex.DecodeString("6EE2721327ED469BB6D93AB9B7A838045190B5BA85D9CEA3B1E17805F7B4AEF6")
	senshiPlaylistKey            = append([]byte(nil), senshiFallbackPlaylistKey...)
	senshiPlaylistKeyMu          sync.Mutex
	senshiProxyMu                sync.Mutex
	senshiProxyServer            *senshiProxy
	senshiIndexScriptRE          = regexp.MustCompile(`assets/(index-[A-Za-z0-9_-]+)\.js`)
	senshiWatchScriptRE          = regexp.MustCompile(`WatchPage-([A-Za-z0-9_-]+)\.js`)
	senshiKeyArrayRE             = regexp.MustCompile(`Uint8Array\.from\(\[((?:\d{1,3},){31}\d{1,3})\]\)`)
	senshiURIAttributeRE         = regexp.MustCompile(`URI="([^"]+)"`)
)

type senshiProxySession struct {
	audioRendition string
}

type senshiProxy struct {
	baseURL  string
	server   *http.Server
	sessions map[string]senshiProxySession
}

func registerSenshiStream(masterURL, audioRendition, subtitleURL string) (string, string, error) {
	proxy, err := getSenshiProxy()
	if err != nil {
		return "", "", err
	}
	id, err := randomSenshiSessionID()
	if err != nil {
		return "", "", err
	}
	senshiProxyMu.Lock()
	proxy.sessions[id] = senshiProxySession{audioRendition: audioRendition}
	senshiProxyMu.Unlock()

	streamURL := proxy.proxyURL(id, masterURL)
	proxiedSubtitle := ""
	if strings.TrimSpace(subtitleURL) != "" {
		proxiedSubtitle = proxy.proxyURL(id, subtitleURL)
	}
	return streamURL, proxiedSubtitle, nil
}

func getSenshiProxy() (*senshiProxy, error) {
	senshiProxyMu.Lock()
	defer senshiProxyMu.Unlock()
	if senshiProxyServer != nil {
		return senshiProxyServer, nil
	}

	proxy := &senshiProxy{sessions: make(map[string]senshiProxySession)}
	mux := http.NewServeMux()
	mux.HandleFunc("/stream/", proxy.handle)
	server := &http.Server{Handler: mux}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("start Senshi proxy: %w", err)
	}
	proxy.baseURL = "http://" + listener.Addr().String()
	proxy.server = server
	go func() { _ = server.Serve(listener) }()
	senshiProxyServer = proxy
	return proxy, nil
}

func (p *senshiProxy) proxyURL(sessionID, upstream string) string {
	return p.baseURL + "/stream/" + sessionID + "?url=" + url.QueryEscape(upstream)
}

func (p *senshiProxy) handle(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/stream/")
	sessionID := strings.Trim(path, "/")
	senshiProxyMu.Lock()
	session, ok := p.sessions[sessionID]
	senshiProxyMu.Unlock()
	if !ok {
		http.NotFound(w, r)
		return
	}

	upstream := strings.TrimSpace(r.URL.Query().Get("url"))
	parsed, err := url.Parse(upstream)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		http.Error(w, "invalid upstream URL", http.StatusBadRequest)
		return
	}

	body, contentType, status, err := fetchSenshiResource(upstream)
	if err != nil {
		http.Error(w, err.Error(), status)
		return
	}
	if isSenshiPlaylistURL(upstream) {
		playlist, err := decryptSenshiPlaylist(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		playlist = filterSenshiAudioRenditions(playlist, session.audioRendition)
		playlist = p.rewritePlaylist(sessionID, upstream, playlist)
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		_, _ = io.WriteString(w, playlist)
		return
	}
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	if strings.HasSuffix(strings.ToLower(parsed.Path), ".jpg") {
		w.Header().Set("Content-Type", "video/mp2t")
	}
	_, _ = w.Write(body)
}

func fetchSenshiResource(rawURL string) ([]byte, string, int, error) {
	req, err := newRequest(http.MethodGet, rawURL)
	if err != nil {
		return nil, "", http.StatusBadRequest, err
	}
	req.Header.Set("Origin", baseURL)
	resp, err := curdhost.HTTPClient().Do(req)
	if err != nil {
		return nil, "", http.StatusBadGateway, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", http.StatusBadGateway, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, "", http.StatusBadGateway, fmt.Errorf("Senshi upstream returned status %d", resp.StatusCode)
	}
	return body, resp.Header.Get("Content-Type"), http.StatusBadGateway, nil
}

func decryptSenshiPlaylist(body []byte) (string, error) {
	text := strings.TrimSpace(string(body))
	if !strings.HasPrefix(text, senshiEncryptedPlaylistPrefix) {
		return string(body), nil
	}

	senshiPlaylistKeyMu.Lock()
	key := append([]byte(nil), senshiPlaylistKey...)
	senshiPlaylistKeyMu.Unlock()
	decrypted, err := decryptSenshiPlaylistWithKey(text, key)
	if err == nil {
		return decrypted, nil
	}
	fresh, refreshErr := refreshSenshiPlaylistKey()
	if refreshErr != nil {
		return "", fmt.Errorf("decrypt Senshi playlist: %w", err)
	}
	decrypted, err = decryptSenshiPlaylistWithKey(text, fresh)
	if err != nil {
		return "", fmt.Errorf("decrypt Senshi playlist with refreshed key: %w", err)
	}
	return decrypted, nil
}

func decryptSenshiPlaylistWithKey(body string, key []byte) (string, error) {
	data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(strings.TrimPrefix(body, senshiEncryptedPlaylistPrefix)))
	if err != nil {
		return "", err
	}
	if len(data) < 12+16 {
		return "", fmt.Errorf("truncated encrypted playlist")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	plain, err := gcm.Open(nil, data[:12], data[12:], nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func refreshSenshiPlaylistKey() ([]byte, error) {
	home, err := fetchSenshiText(baseURL + "/")
	if err != nil {
		return nil, err
	}
	indexMatch := senshiIndexScriptRE.FindStringSubmatch(home)
	if len(indexMatch) != 2 {
		return nil, fmt.Errorf("Senshi index script not found")
	}
	indexJS, err := fetchSenshiText(baseURL + "/assets/" + indexMatch[1] + ".js")
	if err != nil {
		return nil, err
	}
	watchMatch := senshiWatchScriptRE.FindStringSubmatch(indexJS)
	if len(watchMatch) != 2 {
		return nil, fmt.Errorf("Senshi watch script not found")
	}
	watchJS, err := fetchSenshiText(baseURL + "/assets/WatchPage-" + watchMatch[1] + ".js")
	if err != nil {
		return nil, err
	}
	matches := senshiKeyArrayRE.FindAllStringSubmatch(watchJS, -1)
	if len(matches) != 2 {
		return nil, fmt.Errorf("expected two Senshi playlist key arrays, got %d", len(matches))
	}
	a, err := parseSenshiKeyArray(matches[0][1])
	if err != nil {
		return nil, err
	}
	b, err := parseSenshiKeyArray(matches[1][1])
	if err != nil {
		return nil, err
	}
	key := make([]byte, 32)
	for i := range key {
		key[i] = a[i] ^ b[i]
	}
	senshiPlaylistKeyMu.Lock()
	senshiPlaylistKey = append([]byte(nil), key...)
	senshiPlaylistKeyMu.Unlock()
	return key, nil
}

func parseSenshiKeyArray(csv string) ([]byte, error) {
	parts := strings.Split(csv, ",")
	if len(parts) != 32 {
		return nil, fmt.Errorf("invalid Senshi key array length %d", len(parts))
	}
	key := make([]byte, len(parts))
	for i, part := range parts {
		value, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || value < 0 || value > 255 {
			return nil, fmt.Errorf("invalid Senshi key byte %q", part)
		}
		key[i] = byte(value)
	}
	return key, nil
}

func fetchSenshiText(rawURL string) (string, error) {
	body, _, _, err := fetchSenshiResource(rawURL)
	return string(body), err
}

func (p *senshiProxy) rewritePlaylist(sessionID, parentURL, playlist string) string {
	parent, err := url.Parse(parentURL)
	if err != nil {
		return playlist
	}
	lines := strings.Split(strings.ReplaceAll(playlist, "\r\n", "\n"), "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			lines[i] = senshiURIAttributeRE.ReplaceAllStringFunc(line, func(attribute string) string {
				match := senshiURIAttributeRE.FindStringSubmatch(attribute)
				if len(match) != 2 {
					return attribute
				}
				resolved := resolveSenshiURL(parent, match[1])
				return `URI="` + p.proxyURL(sessionID, resolved) + `"`
			})
			continue
		}
		resolved := resolveSenshiURL(parent, trimmed)
		lines[i] = p.proxyURL(sessionID, resolved)
	}
	return strings.Join(lines, "\n")
}

func resolveSenshiURL(parent *url.URL, ref string) string {
	parsed, err := url.Parse(strings.TrimSpace(ref))
	if err != nil {
		return ref
	}
	return parent.ResolveReference(parsed).String()
}

func isSenshiPlaylistURL(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	path := strings.ToLower(parsed.Path)
	return strings.HasSuffix(path, ".m3u8") || strings.HasSuffix(path, ".txt")
}

func filterSenshiAudioRenditions(manifest, pattern string) string {
	if pattern == "" {
		return manifest
	}
	lines := strings.Split(manifest, "\n")
	audioCount, matchingCount := 0, 0
	for _, line := range lines {
		if strings.Contains(line, "#EXT-X-MEDIA") && strings.Contains(line, "TYPE=AUDIO") {
			audioCount++
			if strings.Contains(line, pattern) {
				matchingCount++
			}
		}
	}
	if audioCount == 0 || matchingCount == 0 || matchingCount == audioCount {
		return manifest
	}
	for i, line := range lines {
		if !strings.Contains(line, "#EXT-X-MEDIA") || !strings.Contains(line, "TYPE=AUDIO") {
			continue
		}
		if strings.Contains(line, pattern) {
			lines[i] = strings.Replace(line, "DEFAULT=NO", "DEFAULT=YES", 1)
		} else {
			lines[i] = ""
		}
	}
	return strings.Join(lines, "\n")
}

func randomSenshiSessionID() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func resetSenshiProxyForTest() {
	senshiProxyMu.Lock()
	defer senshiProxyMu.Unlock()
	if senshiProxyServer != nil {
		_ = senshiProxyServer.server.Close()
	}
	senshiProxyServer = nil
}
