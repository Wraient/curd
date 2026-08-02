package anipub

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/wraient/curd/internal/curdhost"
)

// decoySegmentMarkers identify ad/decoy segments that the megap.kotocdn.site
// CDN injects into resolved HLS playlists. These segments are 1x1 PNGs (or
// 302 redirects to them) served from ByteDance ad infrastructure; mpv can
// never decode them, so playback never starts and the player sits on its idle
// "Drop files or URLs to play here." screen.
var decoySegmentMarkers = []string{
	"ibyteimg.com",
	"byteimg.com",
	"ad-site-i18n",
}

// maxPlaylistBytes bounds how much of a manifest we download during validation.
const maxPlaylistBytes = 2 << 20

// isMegaplayCDNHost reports whether the stream is hosted by the megaplay CDN,
// which is the only CDN this validation is scoped to.
func isMegaplayCDNHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	return host == "kotocdn.site" || host == "megap.kotocdn.site" || strings.HasSuffix(host, ".kotocdn.site")
}

// validateResolvedStream inspects a resolved anipub stream URL before it is
// handed to the media player. When the megaplay CDN is serving an ad-injected
// decoy playlist (or a fully decoy one), an error is returned so the caller
// can fall back to another provider instead of opening an idle mpv window.
func validateResolvedStream(rawURL string) error {
	streamURL, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || streamURL.Scheme == "" || streamURL.Host == "" {
		return fmt.Errorf("invalid anipub stream url %q", rawURL)
	}
	if !isMegaplayCDNHost(streamURL.Host) {
		return nil
	}

	v := &hlsStreamValidator{
		client:   curdhost.HTTPClient(),
		referrer: megaplayBaseURL + "/",
	}

	masterBody, err := v.fetch(streamURL.String())
	if err != nil {
		return fmt.Errorf("anipub stream manifest fetch failed: %w", err)
	}
	if !strings.Contains(masterBody, "#EXTM3U") {
		return fmt.Errorf("anipub stream manifest %q is not an HLS playlist", rawURL)
	}

	mediaURL, err := selectMediaPlaylistURL(streamURL, masterBody)
	if err != nil {
		return err
	}
	mediaBody, err := v.fetch(mediaURL)
	if err != nil {
		return fmt.Errorf("anipub stream media playlist fetch failed: %w", err)
	}

	segments := parsePlaylistSegments(mediaBody)
	if len(segments) == 0 {
		return fmt.Errorf("anipub stream %q has no media segments", rawURL)
	}

	decoyCount := 0
	for _, segment := range segments {
		if isDecoySegmentURI(segment) {
			decoyCount++
		}
	}
	if decoyCount == len(segments) {
		return fmt.Errorf("anipub stream %q is an ad-injected decoy: all %d segments are ad/decoy content", rawURL, len(segments))
	}
	if decoyRatio := float64(decoyCount) / float64(len(segments)); decoyRatio >= 0.5 {
		return fmt.Errorf("anipub stream %q is an ad-injected decoy: %d/%d segments are ad/decoy content", rawURL, decoyCount, len(segments))
	}

	// The first non-decoy segment may still redirect to ad/decoy content, so
	// probe its magic bytes before trusting the playlist.
	for _, segment := range segments {
		if isDecoySegmentURI(segment) {
			continue
		}
		if data := v.fetchRange(segment, 0, 15); looksLikeDecoySegment(data) {
			return fmt.Errorf("anipub stream %q first media segment is not video content", rawURL)
		}
		break
	}

	return nil
}

// hlsStreamValidator fetches manifests and probe bytes for a stream.
type hlsStreamValidator struct {
	client   *http.Client
	referrer string
}

func (v *hlsStreamValidator) fetch(rawURL string) (string, error) {
	if v.client == nil {
		return "", fmt.Errorf("http client not configured")
	}
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Referer", v.referrer)
	req.Header.Set("Accept", "application/vnd.apple.mpegurl, application/x-mpegURL, */*")

	resp, err := v.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if !curdhost.HTTPStatusOK(resp.StatusCode) {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", curdhost.HTTPStatusError("megaplay hls manifest", resp.StatusCode, body)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxPlaylistBytes))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// fetchRange requests the first few bytes of a segment so its magic bytes can
// be inspected. Errors are swallowed: an unavailable probe must not reject a
// stream that mpv could still play.
func (v *hlsStreamValidator) fetchRange(rawURL string, start, end int) []byte {
	if v.client == nil {
		return nil
	}
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Referer", v.referrer)

	resp, err := v.client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if !curdhost.HTTPStatusOK(resp.StatusCode) {
		return nil
	}

	buf := make([]byte, end-start+1)
	n, _ := io.ReadFull(resp.Body, buf)
	return buf[:n]
}

// selectMediaPlaylistURL picks the highest-bandwidth variant from a master
// playlist, or returns the master URL itself when it is a media playlist.
func selectMediaPlaylistURL(masterURL *url.URL, body string) (string, error) {
	if !strings.Contains(body, "#EXT-X-STREAM-INF") {
		return masterURL.String(), nil
	}

	bestBandwidth := -1
	bestURI := ""
	lines := strings.Split(body, "\n")
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, "#EXT-X-STREAM-INF") {
			continue
		}
		bandwidth := parseBandwidth(line)
		if bandwidth <= bestBandwidth {
			continue
		}
		for j := i + 1; j < len(lines); j++ {
			next := strings.TrimSpace(lines[j])
			if next == "" || strings.HasPrefix(next, "#") {
				continue
			}
			bestBandwidth = bandwidth
			bestURI = next
			break
		}
	}
	if bestURI == "" {
		return "", fmt.Errorf("no variant playlist found in master playlist")
	}
	ref, err := url.Parse(bestURI)
	if err != nil {
		return "", fmt.Errorf("invalid variant playlist uri %q: %w", bestURI, err)
	}
	resolved := masterURL.ResolveReference(ref)
	return resolved.String(), nil
}

func parseBandwidth(infLine string) int {
	upper := strings.ToUpper(infLine)
	idx := strings.Index(upper, "BANDWIDTH=")
	if idx < 0 {
		return 0
	}
	rest := upper[idx+len("BANDWIDTH="):]
	if comma := strings.IndexByte(rest, ','); comma >= 0 {
		rest = rest[:comma]
	}
	value, err := strconv.Atoi(strings.TrimSpace(rest))
	if err != nil {
		return 0
	}
	return value
}

// parsePlaylistSegments extracts media segment URIs from a playlist body.
func parsePlaylistSegments(body string) []string {
	var segments []string
	for _, raw := range strings.Split(body, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		segments = append(segments, line)
	}
	return segments
}

func isDecoySegmentURI(raw string) bool {
	lower := strings.ToLower(raw)
	for _, marker := range decoySegmentMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// looksLikeDecoySegment reports whether probe bytes look like an image, an
// HTML error page, or other non-video content instead of an HLS media segment.
func looksLikeDecoySegment(data []byte) bool {
	if len(data) < 4 {
		return false
	}
	if data[0] == 0x89 && data[1] == 'P' && data[2] == 'N' && data[3] == 'G' {
		return true
	}
	if data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
		return true
	}
	if string(data[:4]) == "GIF8" {
		return true
	}
	if len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
		return true
	}
	lower := strings.ToLower(string(data))
	if strings.HasPrefix(lower, "<") || strings.Contains(lower, "<html") || strings.Contains(lower, "<!doctype") {
		return true
	}
	return false
}
