package anipub

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

// detailsCacheTTL bounds how long a fetched details document is reused.
// Search -> EpisodesList -> GetEpisodeStreams hits the same details endpoint
// twice within seconds; the cache drops that redundant round trip. Errors are
// never cached, and the key includes baseURL so fixture servers in tests never
// share entries with each other or with live traffic.
const detailsCacheTTL = 60 * time.Second

// maxDetailsCacheEntries bounds the cache; expired entries are dropped on
// store and the whole map is shed if it still exceeds the cap, so a long
// session cannot accumulate one entry per show forever.
const maxDetailsCacheEntries = 256

var (
	detailsCacheMu sync.Mutex
	detailsCache   = map[string]cachedDetails{}
)

type cachedDetails struct {
	details   detailsResponse
	expiresAt time.Time
}

func fetchDetails(showID string) (detailsResponse, error) {
	key := baseURL + "\x00" + showID
	detailsCacheMu.Lock()
	if entry, ok := detailsCache[key]; ok && time.Now().Before(entry.expiresAt) {
		details := entry.details
		detailsCacheMu.Unlock()
		return details, nil
	}
	detailsCacheMu.Unlock()

	var details detailsResponse
	if err := fetchJSON(detailsURL(showID), baseURL+"/", &details); err != nil {
		return detailsResponse{}, err
	}

	now := time.Now()
	detailsCacheMu.Lock()
	defer detailsCacheMu.Unlock()
	for k, entry := range detailsCache {
		if !now.Before(entry.expiresAt) {
			delete(detailsCache, k)
		}
	}
	if len(detailsCache) >= maxDetailsCacheEntries {
		detailsCache = map[string]cachedDetails{}
	}
	detailsCache[key] = cachedDetails{details: details, expiresAt: now.Add(detailsCacheTTL)}
	return details, nil
}

func episodesList(showID, mode string) ([]string, error) {
	showID, err := parseShowID(showID)
	if err != nil {
		return nil, err
	}
	_ = mode

	details, err := fetchDetails(showID)
	if err != nil {
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
