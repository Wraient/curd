package anipub

import (
	"fmt"
	"strconv"
	"strings"
)

func episodesList(showID, mode string) ([]string, error) {
	showID, err := parseShowID(showID)
	if err != nil {
		return nil, err
	}
	_ = mode

	var details detailsResponse
	if err := fetchJSON(detailsURL(showID), baseURL+"/", &details); err != nil {
		return nil, err
	}

	count := len(details.Local.Ep) + 1
	if count <= 0 {
		return nil, fmt.Errorf("no episodes found for show %s", showID)
	}

	episodes := make([]string, 0, count)
	for epNo := 1; epNo <= count; epNo++ {
		episodes = append(episodes, strconv.Itoa(epNo))
	}
	return episodes, nil
}

func episodeVideoLink(details detailsResponse, epNo int) (string, error) {
	if epNo <= 0 {
		return "", fmt.Errorf("invalid episode number %d", epNo)
	}
	if epNo == 1 {
		return normalizeEpisodeLink(details.Local.Link), nil
	}
	idx := epNo - 2
	if idx < 0 || idx >= len(details.Local.Ep) {
		return "", fmt.Errorf("episode %d not found", epNo)
	}
	return normalizeEpisodeLink(details.Local.Ep[idx].Link), nil
}

func normalizeEpisodeLink(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "src=")
	return strings.TrimSpace(raw)
}

func parseShowID(showID string) (string, error) {
	showID = strings.TrimSpace(showID)
	if showID == "" {
		return "", fmt.Errorf("empty show id")
	}
	if _, err := strconv.Atoi(showID); err != nil {
		return "", fmt.Errorf("invalid anipub show id %q", showID)
	}
	return showID, nil
}
