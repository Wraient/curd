package anipm

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/wraient/curd/internal/providers"
)

func episodesList(showID, mode string) ([]string, error) {
	if _, err := parseShowID(showID); err != nil {
		return nil, err
	}

	series, err := fetchSeries(showID)
	if err != nil {
		return nil, err
	}

	wantDub := providers.NormalizeTranslationType(mode) == "dub"
	seen := map[int]struct{}{}
	for _, ep := range series.Episodes {
		if ep.Number <= 0 {
			continue
		}
		if wantDub && !ep.Dub {
			continue
		}
		if !wantDub && !ep.Sub {
			continue
		}
		seen[ep.Number] = struct{}{}
	}
	if len(seen) == 0 {
		if len(series.Episodes) > 0 {
			// The series exists but has no episodes in this mode; report the
			// full list so total-episode detection still works. Stream
			// resolution will surface the per-mode error.
			for _, ep := range series.Episodes {
				if ep.Number > 0 {
					seen[ep.Number] = struct{}{}
				}
			}
		}
	}
	if len(seen) == 0 {
		return nil, fmt.Errorf("no episodes found for show %s", showID)
	}

	nums := make([]int, 0, len(seen))
	for n := range seen {
		nums = append(nums, n)
	}
	sort.Ints(nums)
	result := make([]string, 0, len(nums))
	for _, n := range nums {
		result = append(result, strconv.Itoa(n))
	}
	return result, nil
}

func fetchSeries(showID string) (*seriesResponse, error) {
	var series seriesResponse
	if err := fetchJSON(seriesURL(showID), baseURL+"/", &series); err == nil {
		return &series, nil
	}
	// AniList-sourced catalog entries live under the ani endpoint; they
	// carry episode metadata but no Settlar playback (streams error out
	// gracefully at the bootstrap step).
	var ani seriesResponse
	if err := fetchJSON(aniURL(showID), baseURL+"/", &ani); err != nil {
		return nil, err
	}
	return &ani, nil
}

// parseShowID accepts either a numeric ani.pm id or a search routeId
// (slug-<id>-<suffix>); both address the same series/playback endpoints.
func parseShowID(showID string) (string, error) {
	showID = strings.TrimSpace(showID)
	if showID == "" {
		return "", fmt.Errorf("empty show id")
	}
	return showID, nil
}
