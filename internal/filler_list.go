package internal

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type AnimeFillerListEpisode struct {
	EpisodeID int  `json:"mal_id"`
	IsFiller  bool `json:"filler"`
}

type EpisodesResponse struct {
	Data       []AnimeFillerListEpisode `json:"data"`
	Pagination struct {
		HasNextPage bool `json:"has_next_page"`
	} `json:"pagination"`
}

// jikanFillerMaxRetries bounds 429 retries: the old code retried forever,
// hammering Jikan into a permanent rate limit. jikanRetryDelay is the
// fallback wait; Retry-After is honored when the server sends one.
var jikanFillerMaxRetries = 5
var jikanRetryDelay = 2 * time.Second

func jikanRetryAfter(resp *http.Response) time.Duration {
	if resp == nil {
		return jikanRetryDelay
	}
	if raw := strings.TrimSpace(resp.Header.Get("Retry-After")); raw != "" {
		if secs, err := strconv.Atoi(raw); err == nil && secs > 0 {
			if secs > 30 {
				secs = 30
			}
			return time.Duration(secs) * time.Second
		}
	}
	return jikanRetryDelay
}

func FetchFillerEpisodes(malID int) ([]int, error) {
	now := time.Now()
	path := jikanCachePath()
	key := jikanFillerCacheKey(malID)
	nkey := jikanNegativeKey("fl", key)
	jikanCacheMu.Lock()
	cached := loadJikanCacheFile(path)
	fillerEntry, haveCached := cached.Filler[key]
	negHit := jikanNegativeHit(cached, nkey, now)
	jikanCacheMu.Unlock()
	if haveCached && jikanEntryFresh(fillerEntry.FetchedAt, now) {
		return append([]int(nil), fillerEntry.Episodes...), nil
	}
	if negHit || jikanHostDown(path, now) {
		// Failed recently: skip the paginated hammering. Stale list (if
		// any) still applies; otherwise report no filler data, instantly.
		if haveCached {
			return append([]int(nil), fillerEntry.Episodes...), nil
		}
		Log(fmt.Sprintf("Jikan filler list for %d failed recently, skipping re-fetch", malID))
		return nil, nil
	}

	baseURL := fmt.Sprintf("%s/anime/%d/episodes", jikanBaseURL, malID)
	var fillerEpisodes []int
	page := 1
	retries := 0
	rateLimiter := time.NewTicker(334 * time.Millisecond) // ~3 requests per second
	defer rateLimiter.Stop()

	for {
		<-rateLimiter.C // Wait for rate limit
		url := fmt.Sprintf("%s?page=%d", baseURL, page)
		resp, err := jikanHTTPClient().Get(url)
		if err != nil {
			markJikanHostDown(path, time.Now())
			jikanCacheMu.Lock()
			cached = loadJikanCacheFile(path)
			noteJikanNegative(&cached, nkey, time.Now())
			saveJikanCacheFile(path, cached)
			jikanCacheMu.Unlock()
			if haveCached {
				return append([]int(nil), fillerEntry.Episodes...), nil
			}
			return nil, fmt.Errorf("error fetching episodes: %v", err)
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			resp.Body.Close()
			if retries >= jikanFillerMaxRetries {
				// Rate limits are transient: no hour-long negative entry
				// (it would disable filler skip for the show).
				if haveCached {
					return append([]int(nil), fillerEntry.Episodes...), nil
				}
				return nil, fmt.Errorf("rate limited fetching filler list for %d after %d retries", malID, retries)
			}
			retries++
			time.Sleep(jikanRetryAfter(resp))
			continue
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			if resp.StatusCode >= 500 {
				markJikanHostDown(path, time.Now())
			}
			jikanCacheMu.Lock()
			cached = loadJikanCacheFile(path)
			noteJikanNegative(&cached, nkey, time.Now())
			saveJikanCacheFile(path, cached)
			jikanCacheMu.Unlock()
			if haveCached {
				return append([]int(nil), fillerEntry.Episodes...), nil
			}
			return nil, fmt.Errorf("received non-200 response: %d", resp.StatusCode)
		}

		var episodesResp EpisodesResponse
		if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&episodesResp); err != nil {
			resp.Body.Close()
			// Usually a body read timeout on a stalling Jikan: treat like any
			// other outage so the next launch doesn't re-pay it.
			markJikanHostDown(path, time.Now())
			jikanCacheMu.Lock()
			cached = loadJikanCacheFile(path)
			noteJikanNegative(&cached, nkey, time.Now())
			saveJikanCacheFile(path, cached)
			jikanCacheMu.Unlock()
			if haveCached {
				return append([]int(nil), fillerEntry.Episodes...), nil
			}
			return nil, fmt.Errorf("error decoding response: %v", err)
		}
		resp.Body.Close()
		markJikanHostUp(path)

		for _, episode := range episodesResp.Data {
			if episode.IsFiller {
				fillerEpisodes = append(fillerEpisodes, episode.EpisodeID)
			}
		}

		if !episodesResp.Pagination.HasNextPage {
			break
		}
		page++
	}

	jikanCacheMu.Lock()
	cached = loadJikanCacheFile(path)
	cached.Filler[key] = jikanFillerCacheEntry{Episodes: fillerEpisodes, FetchedAt: time.Now()}
	delete(cached.Negative, nkey)
	saveJikanCacheFile(path, cached)
	jikanCacheMu.Unlock()

	return fillerEpisodes, nil
}

// IsEpisodeFiller checks if a given episode number is in the filler episodes list
func IsEpisodeFiller(fillerEpisodes []int, episodeNumber int) bool {
	for _, fillerEp := range fillerEpisodes {
		if fillerEp == episodeNumber {
			return true
		}
	}
	return false
}

// GetNextCanonEpisode returns the next non-filler episode number after the current episode
func GetNextCanonEpisode(fillerEpisodes []int, currentEpisode int) int {
	nextEpisode := currentEpisode + 1

	// Keep incrementing episode number until we find a non-filler episode
	for IsEpisodeFiller(fillerEpisodes, nextEpisode) {
		nextEpisode++
	}

	return nextEpisode
}
