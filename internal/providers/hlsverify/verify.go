// Package hlsverify inspects resolved HLS stream URLs before they are handed
// to a media player. Ad-injected CDNs prepend 1x1 image decoys (or serve a
// fully decoy playlist) that mpv cannot decode, so playback never starts.
// It is shared by providers whose streams resolve to the same decoy-injecting
// MegaPlay-style CDNs.
package hlsverify

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/wraient/curd/internal/curdhost"
)

// DefaultDecoyMarkers identify ad/decoy segments served from ByteDance ad
// infrastructure. These are 1x1 PNG/JPEG images (or 302 redirects to them)
// prepended to or mixed into resolved HLS playlists.
var DefaultDecoyMarkers = []string{
	"ibyteimg.com",
	"byteimg.com",
	"ad-site-i18n",
	"tiktokcdn.com",
}

// maxPlaylistBytes bounds how much of a manifest we download during validation.
const maxPlaylistBytes = 2 << 20

const defaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

// Options configures stream validation.
type Options struct {
	// Referer is sent with every manifest and probe request.
	Referer string
	// UserAgent is sent with every request; a Firefox UA is used when empty.
	UserAgent string
	// DecoyMarkers overrides DefaultDecoyMarkers when non-empty.
	DecoyMarkers []string
}

func (o Options) markers() []string {
	if len(o.DecoyMarkers) > 0 {
		return o.DecoyMarkers
	}
	return DefaultDecoyMarkers
}

func (o Options) userAgent() string {
	if strings.TrimSpace(o.UserAgent) != "" {
		return o.UserAgent
	}
	return defaultUserAgent
}

// Validate inspects a resolved stream URL. It returns an error when the stream
// is an ad-injected decoy (leading, majority, or fully decoy) or when its first
// real media segment is not decodable video content.
func Validate(rawURL string, opts Options) error {
	streamURL, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || streamURL.Scheme == "" || streamURL.Host == "" {
		return fmt.Errorf("invalid stream url %q", rawURL)
	}

	v := &streamValidator{opts: opts}

	masterBody, err := v.fetch(streamURL.String())
	if err != nil {
		return fmt.Errorf("stream manifest fetch failed: %w", err)
	}
	if !strings.Contains(masterBody, "#EXTM3U") {
		return fmt.Errorf("stream manifest %q is not an HLS playlist", rawURL)
	}

	mediaURL, err := SelectMediaPlaylistURL(streamURL, masterBody)
	if err != nil {
		return err
	}
	mediaBody, err := v.fetch(mediaURL)
	if err != nil {
		return fmt.Errorf("stream media playlist fetch failed: %w", err)
	}

	segments := ParsePlaylistSegments(mediaBody)
	if len(segments) == 0 {
		return fmt.Errorf("stream %q has no media segments", rawURL)
	}

	markers := opts.markers()

	// A leading decoy is fatal: the player opens from the first segment and
	// cannot skip an undecodable image, so such a stream never starts.
	if isDecoySegmentURI(segments[0], markers) {
		return fmt.Errorf("stream %q begins with an ad/decoy segment", rawURL)
	}

	decoyCount := 0
	for _, segment := range segments {
		if isDecoySegmentURI(segment, markers) {
			decoyCount++
		}
	}
	if decoyCount == len(segments) {
		return fmt.Errorf("stream %q is an ad-injected decoy: all %d segments are ad/decoy content", rawURL, len(segments))
	}
	if decoyRatio := float64(decoyCount) / float64(len(segments)); decoyRatio >= 0.5 {
		return fmt.Errorf("stream %q is an ad-injected decoy: %d/%d segments are ad/decoy content", rawURL, decoyCount, len(segments))
	}

	// The first non-decoy segment may still redirect to ad/decoy content, so
	// probe its magic bytes before trusting the playlist.
	for _, segment := range segments {
		if isDecoySegmentURI(segment, markers) {
			continue
		}
		if data := v.fetchRange(segment, 0, 15); LooksLikeDecoySegment(data) {
			return fmt.Errorf("stream %q first media segment is not video content", rawURL)
		}
		break
	}

	return nil
}

// streamValidator fetches manifests and probe bytes for a stream.
type streamValidator struct {
	opts Options
}

func (v *streamValidator) fetch(rawURL string) (string, error) {
	client := curdhost.HTTPClient()
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", v.opts.userAgent())
	req.Header.Set("Referer", v.opts.Referer)
	req.Header.Set("Accept", "application/vnd.apple.mpegurl, application/x-mpegURL, */*")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if !curdhost.HTTPStatusOK(resp.StatusCode) {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", curdhost.HTTPStatusError("hls manifest", resp.StatusCode, body)
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
func (v *streamValidator) fetchRange(rawURL string, start, end int) []byte {
	client := curdhost.HTTPClient()
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
	req.Header.Set("User-Agent", v.opts.userAgent())
	req.Header.Set("Referer", v.opts.Referer)

	resp, err := client.Do(req)
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

// SelectMediaPlaylistURL picks the highest-bandwidth variant from a master
// playlist, or returns the master URL itself when it is a media playlist.
func SelectMediaPlaylistURL(masterURL *url.URL, body string) (string, error) {
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

// ParsePlaylistSegments extracts media segment URIs from a playlist body.
func ParsePlaylistSegments(body string) []string {
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

func isDecoySegmentURI(raw string, markers []string) bool {
	lower := strings.ToLower(raw)
	for _, marker := range markers {
		if strings.Contains(lower, strings.ToLower(marker)) {
			return true
		}
	}
	return false
}

// LooksLikeDecoySegment reports whether probe bytes look like an image, an
// HTML error page, or other non-video content instead of an HLS media segment.
func LooksLikeDecoySegment(data []byte) bool {
	if len(data) < 4 {
		return false
	}
	if bytes.HasPrefix(data, []byte{0x89, 'P', 'N', 'G'}) {
		return true
	}
	if bytes.HasPrefix(data, []byte{0xFF, 0xD8, 0xFF}) {
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
