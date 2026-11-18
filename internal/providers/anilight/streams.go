package anilight

import (
	"fmt"
	"strings"

	"github.com/wraient/curd/internal/providers"
	"github.com/wraient/curd/internal/providers/substyle"
)

func getEpisodeStreamsForMode(showID string, config providers.PlaybackConfig, epNo int) ([]string, map[string]providers.StreamPlaybackHint, error) {
	showID = strings.TrimSpace(showID)
	if showID == "" {
		return nil, nil, fmt.Errorf("empty show id")
	}
	if epNo <= 0 {
		return nil, nil, fmt.Errorf("invalid episode number %d", epNo)
	}

	mode := providers.NormalizeTranslationType(config.SubOrDub)
	lang := "dub"
	if mode == "sub" {
		lang = "sub"
		// anilight serves sub and dub mp4s; there is no hardsub channel, so
		// an explicit hard preference degrades to soft via the shared
		// substyle contract.
		if _, err := substyle.Choose(true, false, config.SubStyle); err != nil {
			return nil, nil, err
		}
	}

	token, err := sessionTokenForSources()
	if err != nil {
		return nil, nil, err
	}

	body, err := fetchSources(showID, epNo, lang, token)
	if err != nil {
		// The cached token was rejected server-side (rotated or expired
		// early). Handshake once more and retry the lookup a single time
		// instead of failing until the cache entry lapses.
		if !isStaleSessionError(err) {
			return nil, nil, err
		}
		invalidateSessionToken()
		if token, err = sessionTokenForSources(); err != nil {
			return nil, nil, err
		}
		if body, err = fetchSources(showID, epNo, lang, token); err != nil {
			return nil, nil, err
		}
	}
	var payload sourcesResponse
	if err := decodeJSON(body, &payload); err != nil {
		return nil, nil, err
	}
	if staleErr := strings.TrimSpace(payload.Error); staleErr != "" {
		if !isStaleSessionMessage(staleErr) {
			return nil, nil, fmt.Errorf("anilight stream unavailable for episode %d: %s", epNo, staleErr)
		}
		invalidateSessionToken()
		if token, err = sessionTokenForSources(); err != nil {
			return nil, nil, err
		}
		if body, err = fetchSources(showID, epNo, lang, token); err != nil {
			return nil, nil, err
		}
		payload = sourcesResponse{}
		if err := decodeJSON(body, &payload); err != nil {
			return nil, nil, err
		}
		if staleErr = strings.TrimSpace(payload.Error); staleErr != "" {
			return nil, nil, fmt.Errorf("anilight stream unavailable for episode %d: %s", epNo, staleErr)
		}
	}
	if len(payload.Sources) == 0 {
		return nil, nil, fmt.Errorf("anilight returned no streams for episode %d", epNo)
	}
	streamURL := resolveCDNURL(strings.TrimSpace(payload.Sources[0].URL))
	if streamURL == "" {
		return nil, nil, fmt.Errorf("anilight returned an empty stream url for episode %d", epNo)
	}
	if err := probeStreamHead(streamURL); err != nil {
		return nil, nil, fmt.Errorf("anilight stream rejected: %w", err)
	}

	subtitle := ""
	if mode == "sub" {
		subtitle = pickEnglishSubtitle(payload.Tracks)
	}
	hints := map[string]providers.StreamPlaybackHint{
		streamURL: {
			Referrer: referrer,
			Subtitle: subtitle,
		},
	}
	return []string{streamURL}, hints, nil
}

// fetchSources runs one authenticated sources lookup.
func fetchSources(showID string, epNo int, lang, token string) ([]byte, error) {
	req, err := newRequest(sourcesURL(showID, epNo, lang), referrer)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-source-session", token)
	return doRequest(req)
}

// isStaleSessionError reports whether a failed lookup looks like a rejected
// session token rather than a genuinely missing episode: the real API
// answers unknown/expired tokens with 401 or a token-complaint body.
func isStaleSessionError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "status 401") || isStaleSessionMessage(msg)
}

// isStaleSessionMessage reports whether a sources error string complains
// about the session token instead of the requested content.
func isStaleSessionMessage(msg string) bool {
	msg = strings.ToLower(strings.TrimSpace(msg))
	return strings.Contains(msg, "session") || strings.Contains(msg, "token")
}

// resolveCDNURL maps a source token to its playable URL the way the web
// player does: relative tokens resolve under the satella /e/ namespace,
// absolute URLs pass through untouched.
func resolveCDNURL(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	if strings.HasPrefix(token, "http://") || strings.HasPrefix(token, "https://") {
		return token
	}
	token = strings.TrimPrefix(token, "/")
	token = strings.TrimPrefix(token, "e/")
	return strings.TrimSuffix(streamCDN, "/") + "/e/" + token
}

// pickEnglishSubtitle prefers the default English caption track and
// resolves its CDN path the way the web player does.
func pickEnglishSubtitle(tracks []sourceTrack) string {
	var fallback string
	for _, track := range tracks {
		lang := strings.ToLower(strings.TrimSpace(track.Lang))
		label := strings.ToLower(strings.TrimSpace(track.Label))
		if lang != "en" && lang != "english" && !strings.Contains(label, "english") {
			continue
		}
		candidate := resolveCDNURL(track.URL)
		if candidate == "" {
			continue
		}
		if strings.Contains(label, "default") || track.ID == "English" {
			return candidate
		}
		if fallback == "" {
			fallback = candidate
		}
	}
	return fallback
}
