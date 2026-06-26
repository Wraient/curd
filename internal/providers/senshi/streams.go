package senshi

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/wraient/curd/internal/providers"
)

func getEpisodeStreamsForMode(malIDStr string, config providers.PlaybackConfig, epNo int) ([]string, map[string]providers.StreamPlaybackHint, error) {
	malID, err := parseMalID(malIDStr)
	if err != nil {
		return nil, nil, err
	}
	if epNo <= 0 {
		return nil, nil, fmt.Errorf("invalid episode number %d", epNo)
	}

	mode := providers.NormalizeTranslationType(config.SubOrDub)
	wantStatus := "HardSub"
	if mode == "dub" {
		wantStatus = "Dub"
	}

	var embeds []embedItem
	url := fmt.Sprintf("%s/episode-embeds/%d/%d", baseURL, malID, epNo)
	if err := fetchJSON(http.MethodGet, url, nil, &embeds); err != nil {
		return nil, nil, err
	}
	if len(embeds) == 0 {
		return nil, nil, fmt.Errorf("no streams found for episode %d", epNo)
	}

	for _, item := range embeds {
		if !strings.EqualFold(strings.TrimSpace(item.Status), wantStatus) {
			continue
		}
		streamURL := strings.TrimSpace(item.URL)
		if streamURL == "" {
			continue
		}
		hints := map[string]providers.StreamPlaybackHint{
			streamURL: {
				Referrer: baseURL + "/",
			},
		}
		return []string{streamURL}, hints, nil
	}

	return nil, nil, fmt.Errorf("no %s streams found for episode %d", mode, epNo)
}
