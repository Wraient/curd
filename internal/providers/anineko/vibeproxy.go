package anineko

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

var pngIENDMarker = []byte{0x49, 0x45, 0x4E, 0x44, 0xAE, 0x42, 0x60, 0x82}

var (
	vibeProxyMu     sync.Mutex
	vibeProxyServer *vibeProxy
)

type vibeSession struct {
	masterURL string
	referer   string
	variants  map[string][]vibeSegment
}

type vibeSegment struct {
	duration float64
	url      string
}

type vibeProxy struct {
	baseURL  string
	server   *http.Server
	sessions map[string]*vibeSession
}

func registerVibeStream(masterURL, referer string) (string, error) {
	proxy, err := getVibeProxy()
	if err != nil {
		return "", err
	}
	return proxy.register(masterURL, referer)
}

func getVibeProxy() (*vibeProxy, error) {
	vibeProxyMu.Lock()
	defer vibeProxyMu.Unlock()

	if vibeProxyServer != nil {
		return vibeProxyServer, nil
	}

	proxy := &vibeProxy{sessions: map[string]*vibeSession{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/stream/", proxy.handle)
	server := &http.Server{Handler: mux}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("start vibe proxy listener: %w", err)
	}
	proxy.server = server
	proxy.baseURL = "http://" + listener.Addr().String()

	go func() {
		_ = server.Serve(listener)
	}()

	vibeProxyServer = proxy
	return proxy, nil
}

func (p *vibeProxy) register(masterURL, referer string) (string, error) {
	id, err := randomSessionID()
	if err != nil {
		return "", err
	}
	vibeProxyMu.Lock()
	p.sessions[id] = &vibeSession{
		masterURL: masterURL,
		referer:   referer,
		variants:  map[string][]vibeSegment{},
	}
	vibeProxyMu.Unlock()
	return p.baseURL + "/stream/" + id + "/master.m3u8", nil
}

func (p *vibeProxy) handle(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/stream/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		http.NotFound(w, r)
		return
	}
	sessionID := parts[0]

	vibeProxyMu.Lock()
	session, ok := p.sessions[sessionID]
	vibeProxyMu.Unlock()
	if !ok {
		http.NotFound(w, r)
		return
	}

	switch {
	case len(parts) == 2 && parts[1] == "master.m3u8":
		p.serveMaster(w, sessionID, session, parts[1])
	case len(parts) == 2 && strings.HasSuffix(parts[1], ".m3u8"):
		p.serveVariant(w, sessionID, session, parts[1])
	case len(parts) == 4 && parts[2] == "seg":
		p.serveSegment(w, r, session, parts[1], parts[3])
	default:
		http.NotFound(w, r)
	}
}

func (p *vibeProxy) serveMaster(w http.ResponseWriter, sessionID string, session *vibeSession, _ string) {
	body, err := fetchString(session.masterURL, session.referer)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	rewritten := rewritePlaylistLines(body, func(line string) string {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			return line
		}
		return p.baseURL + "/stream/" + sessionID + "/" + line
	})
	writePlaylist(w, rewritten)
}

func (p *vibeProxy) serveVariant(w http.ResponseWriter, sessionID string, session *vibeSession, variantName string) {
	variantURL := resolvePlaylistURL(session.masterURL, variantName)
	body, err := fetchString(variantURL, session.referer)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	segments := make([]vibeSegment, 0)
	var currentDuration float64
	rewritten := rewritePlaylistLines(body, func(line string) string {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#EXTINF:") {
			currentDuration = parseExtInfDuration(line)
			return line
		}
		if line == "" || strings.HasPrefix(line, "#") {
			return line
		}
		segmentURL := resolvePlaylistURL(variantURL, line)
		index := len(segments)
		segments = append(segments, vibeSegment{duration: currentDuration, url: segmentURL})
		return fmt.Sprintf("%s/stream/%s/%s/seg/%d", p.baseURL, sessionID, variantName, index)
	})

	vibeProxyMu.Lock()
	session.variants[variantName] = segments
	vibeProxyMu.Unlock()

	writePlaylist(w, rewritten)
}

func (p *vibeProxy) serveSegment(w http.ResponseWriter, r *http.Request, session *vibeSession, variantName, indexText string) {
	index, err := strconv.Atoi(indexText)
	if err != nil || index < 0 {
		http.NotFound(w, r)
		return
	}

	vibeProxyMu.Lock()
	segments := session.variants[variantName]
	vibeProxyMu.Unlock()
	if index >= len(segments) {
		http.NotFound(w, r)
		return
	}

	data, err := fetchBytes(segments[index].url, session.referer)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	body := stripPNGWrapper(data)
	w.Header().Set("Content-Type", "video/mp2t")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	_, _ = w.Write(body)
}

func rewritePlaylistLines(playlist string, mapLine func(string) string) string {
	lines := strings.Split(strings.ReplaceAll(playlist, "\r\n", "\n"), "\n")
	for i, line := range lines {
		lines[i] = mapLine(line)
	}
	return strings.Join(lines, "\n")
}

func parseExtInfDuration(line string) float64 {
	line = strings.TrimPrefix(strings.TrimSpace(line), "#EXTINF:")
	if idx := strings.Index(line, ","); idx >= 0 {
		line = line[:idx]
	}
	value, err := strconv.ParseFloat(strings.TrimSpace(line), 64)
	if err != nil {
		return 0
	}
	return value
}

func stripPNGWrapper(data []byte) []byte {
	idx := bytes.Index(data, pngIENDMarker)
	if idx < 0 {
		return data
	}
	return data[idx+len(pngIENDMarker):]
}

func writePlaylist(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	_, _ = io.WriteString(w, body)
}

func randomSessionID() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func resetVibeProxyForTest() {
	vibeProxyMu.Lock()
	defer vibeProxyMu.Unlock()
	if vibeProxyServer != nil && vibeProxyServer.server != nil {
		_ = vibeProxyServer.server.Close()
	}
	vibeProxyServer = nil
}
