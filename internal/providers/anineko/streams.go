package anineko

import (
	"fmt"
	"strings"

	"github.com/wraient/curd/internal/providers"
)

func getEpisodeStreamsForMode(slug string, config providers.PlaybackConfig, epNo int) ([]string, map[string]providers.StreamPlaybackHint, error) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return nil, nil, fmt.Errorf("empty show id")
	}
	if epNo <= 0 {
		return nil, nil, fmt.Errorf("invalid episode number %d", epNo)
	}

	mode := providers.NormalizeTranslationType(config.SubOrDub)
	html, err := fetchString(watchURL(slug, epNo), baseURL+"/")
	if err != nil {
		return nil, nil, err
	}

	groups := extractLangEmbedURLs(html)
	embedURLs, err := embedURLsForMode(groups, mode, config.SubStyle)
	if err != nil {
		return nil, nil, err
	}
	if len(embedURLs) == 0 {
		return nil, nil, fmt.Errorf("no %s streams found for episode %d", mode, epNo)
	}

	bibiembURLs, vibeplayerURLs, _ := partitionByHost(embedURLs)

	for _, embedURL := range bibiembURLs {
		stream, err := resolveBibiemb(embedURL)
		if err != nil {
			continue
		}
		return singleStreamResult(stream)
	}

	for _, embedURL := range vibeplayerURLs {
		stream, err := resolveVibeplayer(embedURL)
		if err != nil {
			continue
		}
		return singleStreamResult(stream)
	}

	return nil, nil, fmt.Errorf("no playable streams resolved for episode %d", epNo)
}

func singleStreamResult(stream resolvedStream) ([]string, map[string]providers.StreamPlaybackHint, error) {
	if strings.TrimSpace(stream.URL) == "" {
		return nil, nil, fmt.Errorf("empty stream url")
	}
	hints := map[string]providers.StreamPlaybackHint{
		stream.URL: {
			Referrer: stream.Referrer,
			Subtitle: stream.Subtitle,
		},
	}
	return []string{stream.URL}, hints, nil
}
