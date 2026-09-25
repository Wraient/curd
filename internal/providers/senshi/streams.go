package senshi

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/wraient/curd/internal/providers"
)

var vidcloudSourcesBaseURL = "https://s.vidcloud.se"

func getEpisodeStreamsForMode(malIDStr string, config providers.PlaybackConfig, epNo int) ([]string, map[string]providers.StreamPlaybackHint, error) {
	malID, err := parseMalID(malIDStr)
	if err != nil {
		return nil, nil, err
	}
	if epNo <= 0 {
		return nil, nil, fmt.Errorf("invalid episode number %d", epNo)
	}

	mode := providers.NormalizeTranslationType(config.SubOrDub)
	var embeds []embedItem
	embedURL := fmt.Sprintf("%s/episode-embeds/%d/%d", baseURL, malID, epNo)
	if err := fetchJSON(http.MethodGet, embedURL, nil, &embeds); err != nil {
		return nil, nil, err
	}

	audioRendition := "0_ja"
	if mode == "dub" {
		audioRendition = "1_en"
	}

	links := make([]string, 0)
	hints := make(map[string]providers.StreamPlaybackHint)
	seen := make(map[string]struct{})
	var lastErr error
	for _, embed := range embeds {
		if !senshiEmbedMatchesMode(embed.Status, mode) || embed.RemoteSourceID == nil || *embed.RemoteSourceID <= 0 {
			continue
		}

		entries, err := fetchVidcloudEntries(*embed.RemoteSourceID)
		if err != nil {
			lastErr = err
			continue
		}
		for _, entry := range entries {
			if entry.Source == nil || strings.TrimSpace(entry.Source.Src) == "" {
				continue
			}
			sourceURL := strings.TrimSpace(entry.Source.Src)
			subtitle := pickVidcloudSubtitle(entry.Tracks, mode)
			if parent, parseErr := url.Parse(sourceURL); parseErr == nil && subtitle != "" {
				subtitle = resolveSenshiURL(parent, subtitle)
			}
			streamURL, subtitleURL, err := registerSenshiStream(sourceURL, audioRendition, subtitle)
			if err != nil {
				lastErr = err
				continue
			}
			if _, exists := seen[streamURL]; exists {
				continue
			}
			seen[streamURL] = struct{}{}
			links = append(links, streamURL)
			hints[streamURL] = providers.StreamPlaybackHint{Referrer: baseURL + "/", Subtitle: subtitleURL}
		}
	}

	if len(links) > 0 {
		return links, hints, nil
	}
	if lastErr != nil {
		return nil, nil, fmt.Errorf("resolve Senshi %s stream for episode %d: %w", mode, epNo, lastErr)
	}
	return nil, nil, fmt.Errorf("no %s streams found for episode %d", mode, epNo)
}

func senshiEmbedMatchesMode(status, mode string) bool {
	if mode == "dub" {
		return strings.EqualFold(strings.TrimSpace(status), "Dub")
	}
	return isSubEmbedStatus(status)
}

func fetchVidcloudEntries(sourceID int) ([]vidcloudEntry, error) {
	endpoint, err := url.Parse(vidcloudSourcesBaseURL + "/_v1/sources")
	if err != nil {
		return nil, err
	}
	query := endpoint.Query()
	query.Set("id", strconv.Itoa(sourceID))
	endpoint.RawQuery = query.Encode()

	var entries []vidcloudEntry
	if err := fetchJSON(http.MethodGet, endpoint.String(), nil, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func pickVidcloudSubtitle(tracks []vidcloudTrack, mode string) string {
	filtered := make([]senshiSubtitleTrack, 0, len(tracks))
	all := make([]senshiSubtitleTrack, 0, len(tracks))
	for _, track := range tracks {
		trackURL := strings.TrimSpace(track.URL)
		if trackURL == "" {
			trackURL = strings.TrimSpace(track.VTTURL)
		}
		if trackURL == "" || strings.EqualFold(strings.TrimSpace(track.Label), "chapter") {
			continue
		}
		candidate := senshiSubtitleTrack{Src: trackURL, Label: track.Label, Default: track.Default}
		all = append(all, candidate)
		isDub := strings.Contains(strings.ToLower(track.Label), "dub") || strings.Contains(strings.ToLower(trackURL), "ai_dub")
		if (mode == "dub") == isDub {
			filtered = append(filtered, candidate)
		}
	}
	if len(filtered) == 0 {
		filtered = all
	}
	return pickSenshiSubtitleTrack(filtered)
}
