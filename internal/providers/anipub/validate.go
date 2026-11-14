package anipub

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/wraient/curd/internal/providers/hlsverify"
)

// decoySegmentMarkers identify ad/decoy segments that the megap.kotocdn.site
// CDN injects into resolved HLS playlists. These segments are 1x1 PNGs (or
// 302 redirects to them) served from ByteDance ad infrastructure; mpv can
// never decode them, so playback never starts and the player sits on its idle
// "Drop files or URLs to play here." screen.
var decoySegmentMarkers = hlsverify.DefaultDecoyMarkers

// isMegaplayCDNHost reports whether the stream is hosted by the megaplay CDN,
// which is the only CDN this validation is scoped to.
func isMegaplayCDNHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	return host == "kotocdn.site" || host == "megap.kotocdn.site" || strings.HasSuffix(host, ".kotocdn.site")
}

// validateResolvedStream inspects a resolved anipub stream URL before it is
// handed to the media player. When the megaplay CDN is serving an ad-injected
// decoy playlist, an error is returned so the caller can fall back to another
// provider instead of opening an idle mpv window.
func validateResolvedStream(rawURL string) error {
	streamURL, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || streamURL.Scheme == "" || streamURL.Host == "" {
		return fmt.Errorf("invalid anipub stream url %q", rawURL)
	}
	if !isMegaplayCDNHost(streamURL.Host) {
		return nil
	}
	return hlsverify.Validate(rawURL, hlsverify.Options{
		Referer:      megaplayBaseURL + "/",
		UserAgent:    userAgent,
		DecoyMarkers: decoySegmentMarkers,
	})
}

// selectMediaPlaylistURL picks the highest-bandwidth variant from a master
// playlist, or returns the master URL itself when it is a media playlist.
func selectMediaPlaylistURL(masterURL *url.URL, body string) (string, error) {
	return hlsverify.SelectMediaPlaylistURL(masterURL, body)
}

// parsePlaylistSegments extracts media segment URIs from a playlist body.
func parsePlaylistSegments(body string) []string {
	return hlsverify.ParsePlaylistSegments(body)
}

func isDecoySegmentURI(raw string) bool {
	return isDecoySegmentURIMarkers(raw, decoySegmentMarkers)
}

func isDecoySegmentURIMarkers(raw string, markers []string) bool {
	lower := strings.ToLower(raw)
	for _, marker := range markers {
		if strings.Contains(lower, strings.ToLower(marker)) {
			return true
		}
	}
	return false
}

// looksLikeDecoySegment reports whether probe bytes look like an image, an
// HTML error page, or other non-video content instead of an HLS media segment.
func looksLikeDecoySegment(data []byte) bool {
	return hlsverify.LooksLikeDecoySegment(data)
}
