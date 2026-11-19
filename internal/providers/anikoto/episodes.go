package anikoto

import (
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wraient/curd/internal/providers"
)

// episodeMetadataCache memoizes the episode list per show so the
// EpisodesList -> GetEpisodeURL flow does not fetch the same list twice.
// Entries live only for episodeMetadataCacheTTL; matching and episode
// availability are always computed from the cached payload, never skipped.
const episodeMetadataCacheTTL = 5 * time.Minute

// maxEpisodeMetadataEntries bounds the cache; expired entries are dropped
// on store and the whole map is shed if it still exceeds the cap, so a
// long session cannot accumulate one entry per show forever.
const maxEpisodeMetadataEntries = 256

var episodeMetadataCache = struct {
	sync.RWMutex
	entries map[string]episodeMetadataCacheEntry
}{entries: make(map[string]episodeMetadataCacheEntry)}

type episodeMetadataCacheEntry struct {
	expiresAt time.Time
	episodes  []episodeMetadata
}

// animeIDAliases remembers slug -> numeric ID resolutions so bare slugs
// skip the watch-page fetch on repeat lookups. Aliases share the episode
// TTL: a stale alias can only resolve to an ID whose episodes were cached
// together, and any miss falls through to a fresh resolution.
var (
	animeIDMu      sync.RWMutex
	animeIDAliases = map[string]animeIDAlias{}
)

type animeIDAlias struct {
	id        int
	expiresAt time.Time
}

func animeIDAliasKey(slug string) string {
	return baseURL + "\x00" + slug
}

func cachedAnimeID(slug string) (int, bool) {
	animeIDMu.RLock()
	alias, ok := animeIDAliases[animeIDAliasKey(slug)]
	animeIDMu.RUnlock()
	if !ok || time.Now().After(alias.expiresAt) {
		return 0, false
	}
	return alias.id, true
}

func storeAnimeID(slug string, id int) {
	if id <= 0 {
		return
	}
	now := time.Now()
	animeIDMu.Lock()
	defer animeIDMu.Unlock()
	for key, alias := range animeIDAliases {
		if !now.Before(alias.expiresAt) {
			delete(animeIDAliases, key)
		}
	}
	animeIDAliases[animeIDAliasKey(slug)] = animeIDAlias{id: id, expiresAt: now.Add(episodeMetadataCacheTTL)}
}

func cachedEpisodeMetadata(key string) ([]episodeMetadata, bool) {
	episodeMetadataCache.RLock()
	entry, ok := episodeMetadataCache.entries[key]
	episodeMetadataCache.RUnlock()
	if !ok || time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return entry.episodes, true
}

func storeEpisodeMetadata(key string, episodes []episodeMetadata) {
	now := time.Now()
	episodeMetadataCache.Lock()
	defer episodeMetadataCache.Unlock()
	for k, entry := range episodeMetadataCache.entries {
		if !now.Before(entry.expiresAt) {
			delete(episodeMetadataCache.entries, k)
		}
	}
	if len(episodeMetadataCache.entries) >= maxEpisodeMetadataEntries {
		episodeMetadataCache.entries = make(map[string]episodeMetadataCacheEntry)
	}
	episodeMetadataCache.entries[key] = episodeMetadataCacheEntry{
		expiresAt: now.Add(episodeMetadataCacheTTL),
		episodes:  episodes,
	}
}

func episodesList(showID, mode string) ([]string, error) {
	_, _, err := parseShowID(showID)
	if err != nil {
		return nil, err
	}
	mode = providers.NormalizeTranslationType(mode)
	episodes, err := fetchEpisodeMetadata(showID)
	if err != nil {
		return nil, err
	}
	numbers := make([]int, 0, len(episodes))
	seen := make(map[int]struct{})
	for _, episode := range episodes {
		if !episodeAvailableForMode(episode, mode) {
			continue
		}
		if _, exists := seen[episode.Number]; exists {
			continue
		}
		seen[episode.Number] = struct{}{}
		numbers = append(numbers, episode.Number)
	}
	sort.Ints(numbers)
	result := make([]string, 0, len(numbers))
	for _, number := range numbers {
		result = append(result, strconv.Itoa(number))
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("no Anikoto %s episodes found", mode)
	}
	return result, nil
}

func fetchEpisodeMetadata(showID string) ([]episodeMetadata, error) {
	slug, animeID, err := parseShowID(showID)
	if err != nil {
		return nil, err
	}
	if animeID == 0 {
		// A prior lookup may already have resolved this slug: reuse the
		// alias instead of paying for another watch-page fetch.
		if id, ok := cachedAnimeID(slug); ok {
			animeID = id
		} else {
			animeID, err = resolveAnimeID(slug)
			if err != nil {
				return nil, err
			}
			storeAnimeID(slug, animeID)
		}
	}
	// The key includes baseURL so offline fixture servers never share
	// entries with live lookups (or with each other across tests).
	cacheKey := baseURL + "\x00" + slug + "#" + strconv.Itoa(animeID)
	if episodes, ok := cachedEpisodeMetadata(cacheKey); ok {
		return episodes, nil
	}
	endpoint := fmt.Sprintf("%s/ajax/episode/list/%d?vrf=%s", baseURL, animeID, encryptVRF(strconv.Itoa(animeID)))
	referer := watchURL(slug, 0)
	raw, err := fetchResource(endpoint, referer, "application/json, text/javascript, */*; q=0.01", true)
	if err != nil {
		return nil, err
	}
	var response ajaxHTMLResponse
	if err := decodeAjaxResponse(raw, &response); err != nil {
		return nil, err
	}
	episodes := parseEpisodeMetadata(response.Result)
	if len(episodes) == 0 {
		return nil, fmt.Errorf("no Anikoto episodes found for %q", slug)
	}
	storeEpisodeMetadata(cacheKey, episodes)
	return episodes, nil
}

func parseEpisodeMetadata(body string) []episodeMetadata {
	episodes := make([]episodeMetadata, 0)
	seen := make(map[int]struct{})
	for _, tag := range htmlTagPattern.FindAllString(body, -1) {
		if !strings.HasPrefix(strings.ToLower(tag), "<a") {
			continue
		}
		attrs := parseHTMLAttrs(tag)
		number, err := strconv.Atoi(attrs["data-num"])
		if err != nil || number <= 0 || strings.TrimSpace(attrs["data-ids"]) == "" {
			continue
		}
		if _, exists := seen[number]; exists {
			continue
		}
		seen[number] = struct{}{}
		episodes = append(episodes, episodeMetadata{
			Number: number,
			IDs:    strings.TrimSpace(attrs["data-ids"]),
			Sub:    attrs["data-sub"] == "1",
			Dub:    attrs["data-dub"] == "1",
		})
	}
	return episodes
}

func parseShowID(showID string) (string, int, error) {
	parts := strings.SplitN(strings.TrimSpace(showID), "#", 2)
	slug := strings.TrimSpace(parts[0])
	if slug == "" || strings.ContainsAny(slug, "/?#") {
		return "", 0, fmt.Errorf("invalid Anikoto show id %q", showID)
	}
	animeID := 0
	if len(parts) == 2 {
		parsed, err := strconv.Atoi(parts[1])
		if err != nil || parsed <= 0 {
			return "", 0, fmt.Errorf("invalid Anikoto anime id %q", parts[1])
		}
		animeID = parsed
	}
	return slug, animeID, nil
}

func resolveAnimeID(slug string) (int, error) {
	body, err := fetchResource(watchURL(slug, 1), baseURL+"/", "text/html,application/xhtml+xml", false)
	if err != nil {
		return 0, err
	}
	for _, tag := range htmlTagPattern.FindAllString(string(body), -1) {
		attrs := parseHTMLAttrs(tag)
		if attrs["id"] == "watch-main" {
			animeID, err := strconv.Atoi(attrs["data-id"])
			if err == nil && animeID > 0 {
				return animeID, nil
			}
		}
	}
	return 0, fmt.Errorf("Anikoto anime ID not found for %q", slug)
}

func episodeForNumber(showID string, number int) (episodeMetadata, error) {
	episodes, err := fetchEpisodeMetadata(showID)
	if err != nil {
		return episodeMetadata{}, err
	}
	for _, episode := range episodes {
		if episode.Number == number {
			return episode, nil
		}
	}
	return episodeMetadata{}, fmt.Errorf("Anikoto episode %d not found", number)
}

func episodeAvailableForMode(episode episodeMetadata, mode string) bool {
	if mode == "dub" {
		return episode.Dub
	}
	return episode.Sub
}

func watchURL(slug string, episode int) string {
	path := fmt.Sprintf("%s/watch/%s", strings.TrimRight(baseURL, "/"), url.PathEscape(slug))
	if episode > 0 {
		path += fmt.Sprintf("/ep-%d", episode)
	}
	return path
}
