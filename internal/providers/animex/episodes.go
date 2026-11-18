package animex

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/wraient/curd/internal/providers"
)

func episodesList(showID, mode string) ([]string, error) {
	showID = strings.TrimSpace(showID)
	if showID == "" {
		return nil, fmt.Errorf("empty show id")
	}

	var episodes []ppEpisode
	if err := fetchJSON(restURL("/episodes", map[string]string{"id": showID}), referrer, &episodes); err != nil {
		return nil, err
	}
	wantDub := providers.NormalizeTranslationType(mode) == "dub"
	result := make([]string, 0, len(episodes))
	for _, episode := range episodes {
		if episode.Number <= 0 {
			continue
		}
		if wantDub && !episode.HasDub {
			continue
		}
		if !wantDub && !episode.HasSub {
			continue
		}
		result = append(result, strconv.Itoa(episode.Number))
	}
	if len(result) == 0 {
		if len(episodes) > 0 {
			// The show exists but has no episodes in this mode; report the
			// full list so total-episode detection still works. Stream
			// resolution will surface the per-mode error.
			for _, episode := range episodes {
				if episode.Number > 0 {
					result = append(result, strconv.Itoa(episode.Number))
				}
			}
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("no episodes found for show %s", showID)
	}
	return result, nil
}
