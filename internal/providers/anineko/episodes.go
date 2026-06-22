package anineko

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var episodePathRE = regexp.MustCompile(`/ep-(\d+)`)

func episodesList(showID, mode string) ([]string, error) {
	slug := strings.TrimSpace(showID)
	if slug == "" {
		return nil, fmt.Errorf("empty show id")
	}

	html, err := fetchString(watchURL(slug, 0), baseURL+"/")
	if err != nil {
		return nil, err
	}

	seen := map[int]struct{}{}
	for _, match := range episodePathRE.FindAllStringSubmatch(html, -1) {
		if len(match) < 2 {
			continue
		}
		epNo, err := strconv.Atoi(match[1])
		if err != nil || epNo <= 0 {
			continue
		}
		seen[epNo] = struct{}{}
	}
	if len(seen) == 0 {
		return nil, fmt.Errorf("no episodes found for %q", slug)
	}

	episodes := make([]int, 0, len(seen))
	for epNo := range seen {
		episodes = append(episodes, epNo)
	}
	sort.Ints(episodes)

	result := make([]string, 0, len(episodes))
	for _, epNo := range episodes {
		result = append(result, strconv.Itoa(epNo))
	}
	return result, nil
}
