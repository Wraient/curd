package animex

import (
	"strconv"
	"sync"
	"time"
)

// Short-lived cache for mode-independent catalog responses.
//
// Both /episodes and /servers describe every audio language at once and are
// filtered locally per mode (see episodesList, getEpisodeStreamsForMode), so
// the sub and dub lookups for one title fetch byte-identical bodies. This
// cache deduplicates those redundant fetches within its TTL.
//
// Deliberately NOT cached:
//   - /sources: signed, short-lived stream URLs. Every playback re-resolves
//     so mpv never receives a pre-prompt URL that expires mid-episode.
//   - search: query-dependent and caller-driven; nothing redundant in it.
//
// Only successful fetches are stored; errors always re-fetch on the next
// call. The mutex is never held across network I/O: check under lock,
// release, fetch, re-lock to store. Cache keys include restBase so httptest
// fixtures (which swap restBase per server) never share entries.
const (
	catalogCacheTTL = 120 * time.Second
	catalogCacheMax = 256
)

type episodesCacheEntry struct {
	episodes []ppEpisode
	expires  time.Time
}

type serversCacheEntry struct {
	servers ppServers
	expires time.Time
}

var (
	episodesMu    sync.RWMutex
	episodesCache = make(map[string]episodesCacheEntry)

	serversMu    sync.RWMutex
	serversCache = make(map[string]serversCacheEntry)
)

func episodesCacheKey(showID string) string {
	return restBase + "\x00" + showID
}

func serversCacheKey(showID string, epNo int) string {
	return restBase + "\x00" + showID + "\x00" + strconv.Itoa(epNo)
}

func pruneEpisodesCacheLocked(now time.Time) {
	for key, entry := range episodesCache {
		if !now.Before(entry.expires) {
			delete(episodesCache, key)
		}
	}
	if len(episodesCache) >= catalogCacheMax {
		episodesCache = make(map[string]episodesCacheEntry)
	}
}

func pruneServersCacheLocked(now time.Time) {
	for key, entry := range serversCache {
		if !now.Before(entry.expires) {
			delete(serversCache, key)
		}
	}
	if len(serversCache) >= catalogCacheMax {
		serversCache = make(map[string]serversCacheEntry)
	}
}

// cachedEpisodes returns the show's full episode catalog, fetching once per
// TTL. Filtering by mode stays with the caller, unchanged.
func cachedEpisodes(showID string) ([]ppEpisode, error) {
	key := episodesCacheKey(showID)
	now := time.Now()
	episodesMu.RLock()
	entry, ok := episodesCache[key]
	episodesMu.RUnlock()
	if ok && now.Before(entry.expires) {
		return entry.episodes, nil
	}

	var episodes []ppEpisode
	if err := fetchJSON(restURL("/episodes", map[string]string{"id": showID}), referrer, &episodes); err != nil {
		return nil, err
	}

	now = time.Now()
	episodesMu.Lock()
	pruneEpisodesCacheLocked(now)
	episodesCache[key] = episodesCacheEntry{episodes: episodes, expires: now.Add(catalogCacheTTL)}
	episodesMu.Unlock()
	return episodes, nil
}

// cachedServers returns the episode's sub/dub provider lists, fetching once
// per TTL. Mode selection stays with the caller, unchanged.
func cachedServers(showID string, epNo int) (ppServers, error) {
	key := serversCacheKey(showID, epNo)
	now := time.Now()
	serversMu.RLock()
	entry, ok := serversCache[key]
	serversMu.RUnlock()
	if ok && now.Before(entry.expires) {
		return entry.servers, nil
	}

	var servers ppServers
	if err := fetchJSON(restURL("/servers", map[string]string{"id": showID, "epNum": strconv.Itoa(epNo)}), referrer, &servers); err != nil {
		return ppServers{}, err
	}

	now = time.Now()
	serversMu.Lock()
	pruneServersCacheLocked(now)
	serversCache[key] = serversCacheEntry{servers: servers, expires: now.Add(catalogCacheTTL)}
	serversMu.Unlock()
	return servers, nil
}
