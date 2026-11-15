package anipm

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/wraient/curd/internal/providers"
	"github.com/wraient/curd/internal/providers/hlsverify"
	"github.com/wraient/curd/internal/providers/substyle"
)

func getEpisodeStreamsForMode(showID string, config providers.PlaybackConfig, epNo int) ([]string, map[string]providers.StreamPlaybackHint, error) {
	showID, err := parseShowID(showID)
	if err != nil {
		return nil, nil, err
	}
	if epNo <= 0 {
		return nil, nil, fmt.Errorf("invalid episode number %d", epNo)
	}

	mode := providers.NormalizeTranslationType(config.SubOrDub)

	var bootstrap bootstrapResponse
	if err := fetchJSON(bootstrapURL(showID, epNo, mode), baseURL+"/", &bootstrap); err != nil {
		return nil, nil, err
	}
	if bootstrap.Error != "" {
		return nil, nil, fmt.Errorf("anipm playback unavailable for episode %d: %s", epNo, bootstrap.Error)
	}
	if bootstrap.SettlarSelection == "" {
		return nil, nil, fmt.Errorf("anipm returned no stream selection for episode %d", epNo)
	}
	if mode == "dub" && !bootstrap.Availability.Dub {
		return nil, nil, fmt.Errorf("anipm has no dub stream for episode %d", epNo)
	}
	if mode == "sub" && !bootstrap.Availability.Sub {
		return nil, nil, fmt.Errorf("anipm has no sub stream for episode %d", epNo)
	}

	channel := "dub"
	if mode == "sub" {
		channel = "sub"
		hasHard := false
		if pkg, ok := bootstrap.Packages.Episodes[strconv.Itoa(epNo)]; ok {
			hasHard = pkg.SubHard
		}
		style, err := substyle.Choose(true, hasHard, config.SubStyle)
		if err != nil {
			return nil, nil, err
		}
		if style == "hard" {
			channel = "subhard"
		}
	}

	var session sessionResponse
	if err := fetchJSON(settlarSessionURL(bootstrap.SettlarSelection, epNo, channel), baseURL+"/", &session); err != nil {
		return nil, nil, err
	}
	if session.Error != "" {
		return nil, nil, fmt.Errorf("anipm stream session failed for episode %d: %s", epNo, session.Error)
	}
	embedURL := strings.TrimSpace(session.EmbedURL)
	if embedURL == "" {
		return nil, nil, fmt.Errorf("anipm returned no embed url for episode %d", epNo)
	}

	embed, err := fetchEmbedSession(embedURL)
	if err != nil {
		return nil, nil, err
	}
	if embed.Error != "" {
		return nil, nil, fmt.Errorf("anipm embed session failed for episode %d: %s", epNo, embed.Error)
	}
	if !strings.EqualFold(embed.Kind, "hls") || strings.TrimSpace(embed.Source) == "" {
		return nil, nil, fmt.Errorf("anipm returned no playable stream for episode %d", epNo)
	}
	streamURL := strings.TrimSpace(embed.Source)

	if err := hlsverify.Validate(streamURL, hlsverify.Options{
		Referer:   embedBaseURL + "/",
		UserAgent: userAgent,
	}); err != nil {
		return nil, nil, fmt.Errorf("anipm stream rejected: %w", err)
	}

	subtitle := ""
	if channel == "sub" {
		subtitle = pickEnglishSubtitle(embed.Subtitles)
	}

	hints := map[string]providers.StreamPlaybackHint{
		streamURL: {
			Referrer: baseURL + "/",
			Subtitle: subtitle,
		},
	}
	return []string{streamURL}, hints, nil
}

// fetchEmbedSession exchanges the one-time embed URL for the actual HLS
// source. The embed page sets session cookies that the session API needs.
func fetchEmbedSession(embedURL string) (*embedSessionResponse, error) {
	parsed, err := url.Parse(embedURL)
	if err != nil {
		return nil, fmt.Errorf("parse anipm embed url: %w", err)
	}
	token := parsed.Query().Get("t")
	if token == "" {
		return nil, fmt.Errorf("anipm embed url has no session token")
	}

	_, cookies, err := fetch(embedURL, baseURL+"/", nil)
	if err != nil {
		return nil, err
	}

	sessionURL := fmt.Sprintf("%s/api/embed/session?t=%s", embedBaseURL, url.QueryEscape(token))
	body, _, err := fetch(sessionURL, embedURL, cookies)
	if err != nil {
		return nil, err
	}
	var embed embedSessionResponse
	if err := json.Unmarshal(body, &embed); err != nil {
		return nil, fmt.Errorf("parse anipm embed session: %w", err)
	}
	return &embed, nil
}

// pickEnglishSubtitle prefers the default English track and its ASS
// (typeset) variant when the embed offers one.
func pickEnglishSubtitle(tracks []embedSubtitle) string {
	var fallback string
	for _, track := range tracks {
		lang := strings.ToLower(strings.TrimSpace(track.SrcLang))
		label := strings.ToLower(strings.TrimSpace(track.Label))
		if lang != "en" && lang != "eng" && !strings.Contains(label, "english") {
			continue
		}
		candidate := strings.TrimSpace(track.AssURL)
		if candidate == "" {
			candidate = strings.TrimSpace(track.URL)
		}
		if candidate == "" {
			continue
		}
		if track.Default || track.AutoSelect {
			return candidate
		}
		if fallback == "" {
			fallback = candidate
		}
	}
	return fallback
}
