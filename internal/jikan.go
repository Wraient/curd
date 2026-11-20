package internal

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// jikanBaseURL is a var so tests can point Jikan calls at a local server.
var jikanBaseURL = "https://api.jikan.moe/v4"

// jikanRequestTimeout bounds one Jikan request. Jikan is optional metadata
// (titles, filler/recap flags, matching hints): a healthy Jikan answers in
// well under a second, and when it hangs it hangs for the full timeout, so a
// short bound costs nothing when it's up and saves seconds when it's down.
var jikanRequestTimeout = 6 * time.Second

// jikanPrePlayBudget is the most an episode start waits for Jikan before
// playing anyway. A late answer still lands in the cache (and the in-episode
// skip check) — playback is never gated on it.
var jikanPrePlayBudget = 2 * time.Second

func jikanHTTPClient() *http.Client {
	base := sharedHTTPClient
	if base == nil {
		return &http.Client{Timeout: jikanRequestTimeout}
	}
	return &http.Client{Transport: base.Transport, Jar: base.Jar, Timeout: jikanRequestTimeout}
}

// GetEpisodeData fetches episode data for a given anime ID and episode number.
// Results are cached on disk (see jikan_cache.go): Jikan is slow and strictly
// rate limited, so a repeat lookup must not touch the network. When the
// network fails, a stale cached entry still beats the zero-value defaults.
// Never returns an error for upstream failure: callers must keep playing.
func GetEpisodeData(animeID int, episodeNo int, anime *Anime) error {
	if anime == nil {
		return nil
	}
	entry, ok := lookupJikanEpisode(animeID, episodeNo)
	if ok {
		applyJikanEpisodeEntry(anime, entry)
	} else {
		anime.Ep.IsFiller = false
		anime.Ep.IsRecap = false
	}
	return nil
}

// GetEpisodeDataWithin is GetEpisodeData with a wall-clock budget for the
// pre-play path. It reports whether data was applied. On timeout the anime
// is left with non-filler/non-recap defaults and the lookup keeps running in
// the background (deduplicated), so a later GetEpisodeData call joins it and
// the result still reaches the cache. The anime is only written from the
// calling goroutine, never from the background lookup.
func GetEpisodeDataWithin(animeID int, episodeNo int, anime *Anime, budget time.Duration) bool {
	if anime == nil {
		return false
	}
	type result struct {
		entry jikanEpisodeCacheEntry
		ok    bool
	}
	ch := make(chan result, 1)
	go func() {
		e, ok := lookupJikanEpisode(animeID, episodeNo)
		ch <- result{e, ok}
	}()
	timer := time.NewTimer(budget)
	defer timer.Stop()
	select {
	case r := <-ch:
		if r.ok {
			applyJikanEpisodeEntry(anime, r.entry)
			return true
		}
	case <-timer.C:
		Log(fmt.Sprintf("Jikan episode %d:%d exceeded %s pre-play budget; playing without it", animeID, episodeNo, budget))
	}
	anime.Ep.IsFiller = false
	anime.Ep.IsRecap = false
	return false
}

type jikanEpisodeCall struct {
	done  chan struct{}
	entry jikanEpisodeCacheEntry
	ok    bool
}

var (
	jikanInflightMu sync.Mutex
	jikanInflight   = map[string]*jikanEpisodeCall{}
)

// lookupJikanEpisode resolves one episode from cache or network. ok=false
// means no data at all (defaults apply). Concurrent callers for the same
// episode share one request: the pre-play check and the in-episode check
// must not each pay a Jikan timeout.
func lookupJikanEpisode(animeID, episodeNo int) (jikanEpisodeCacheEntry, bool) {
	if animeID <= 0 || episodeNo <= 0 {
		return jikanEpisodeCacheEntry{}, false
	}
	path := jikanCachePath()
	key := jikanEpisodeCacheKey(animeID, episodeNo)
	flightKey := path + "|" + key

	jikanInflightMu.Lock()
	if call, ok := jikanInflight[flightKey]; ok {
		jikanInflightMu.Unlock()
		<-call.done
		return call.entry, call.ok
	}
	call := &jikanEpisodeCall{done: make(chan struct{})}
	jikanInflight[flightKey] = call
	jikanInflightMu.Unlock()

	call.entry, call.ok = fetchJikanEpisode(path, key, animeID, episodeNo)

	jikanInflightMu.Lock()
	delete(jikanInflight, flightKey)
	jikanInflightMu.Unlock()
	close(call.done)
	return call.entry, call.ok
}

func fetchJikanEpisode(path, key string, animeID, episodeNo int) (jikanEpisodeCacheEntry, bool) {
	now := time.Now()
	nkey := jikanNegativeKey("ep", key)
	jikanCacheMu.Lock()
	cached := loadJikanCacheFile(path)
	entry, haveCached := cached.Episodes[key]
	negHit := jikanNegativeHit(cached, nkey, now)
	jikanCacheMu.Unlock()
	if haveCached && jikanEntryFresh(entry.FetchedAt, now) {
		return entry, true
	}
	if negHit || jikanHostDown(path, now) {
		// Jikan failed recently: don't pay another timeout. Stale data (if
		// any) still beats defaults; the outage re-probes after the TTL.
		return entry, haveCached
	}

	url := fmt.Sprintf("%s/anime/%d/episodes/%d", jikanBaseURL, animeID, episodeNo)
	response, err := makeGetRequest(url, nil)
	if err != nil {
		Log(fmt.Sprintf("Warning: Jikan API error: %v - continuing without filler data", err))
		if jikanFailureIsNegative(err) {
			jikanCacheMu.Lock()
			cached = loadJikanCacheFile(path)
			noteJikanNegative(&cached, nkey, time.Now())
			saveJikanCacheFile(path, cached)
			jikanCacheMu.Unlock()
		}
		return entry, haveCached
	}

	data, ok := response["data"].(map[string]interface{})
	if !ok {
		Log("Warning: Invalid Jikan API response - continuing without filler data")
		return entry, haveCached
	}
	getStringValue := func(field string) string {
		if value, ok := data[field].(string); ok {
			return value
		}
		return ""
	}
	getIntValue := func(field string) int {
		if value, ok := data[field].(float64); ok {
			return int(value)
		}
		return 0
	}
	getBoolValue := func(field string) bool {
		if value, ok := data[field].(bool); ok {
			return value
		}
		return false
	}

	entry = jikanEpisodeCacheEntry{
		TitleRomaji:   getStringValue("title_romanji"),
		TitleEnglish:  getStringValue("title"),
		TitleJapanese: getStringValue("title_japanese"),
		Aired:         getStringValue("aired"),
		Duration:      getIntValue("duration"),
		Synopsis:      getStringValue("synopsis"),
		IsFiller:      getBoolValue("filler"),
		IsRecap:       getBoolValue("recap"),
		FetchedAt:     time.Now(),
	}
	jikanCacheMu.Lock()
	cached = loadJikanCacheFile(path)
	cached.Episodes[key] = entry
	delete(cached.Negative, nkey)
	saveJikanCacheFile(path, cached)
	jikanCacheMu.Unlock()
	return entry, true
}

// jikanStatusError is a non-200 Jikan response.
type jikanStatusError struct {
	Code int
	Body string
}

func (e *jikanStatusError) Error() string {
	return fmt.Sprintf("failed with status %d: %s", e.Code, e.Body)
}

// jikanFailureIsNegative reports whether a failure says something about
// this specific lookup (worth suppressing for an hour). Rate limits and
// breaker short-circuits are transient and global: negative-caching them
// would hide an episode's recap flag for an hour after a 1s burst.
func jikanFailureIsNegative(err error) bool {
	if errors.Is(err, errJikanDown) {
		return false
	}
	var se *jikanStatusError
	if errors.As(err, &se) && se.Code == http.StatusTooManyRequests {
		return false
	}
	return true
}

// errJikanDown is returned without touching the network while the Jikan
// outage breaker is open.
var errJikanDown = fmt.Errorf("jikan unavailable (recent outage, retrying later)")

// makeGetRequest performs a Jikan GET. It is guarded by the host breaker:
// a timeout, transport error or 5xx opens it (persisted, so the next launch
// doesn't re-pay the timeout either), and any 2xx closes it.
func makeGetRequest(url string, headers map[string]string) (map[string]interface{}, error) {
	path := jikanCachePath()
	if jikanHostDown(path, time.Now()) {
		return nil, errJikanDown
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create GET request: %w", err)
	}

	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := jikanHTTPClient().Do(req)
	if err != nil {
		markJikanHostDown(path, time.Now())
		return nil, fmt.Errorf("failed to send GET request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		markJikanHostDown(path, time.Now())
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode >= 500 {
		markJikanHostDown(path, time.Now())
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &jikanStatusError{Code: resp.StatusCode, Body: truncateForLog(string(body), 200)}
	}
	markJikanHostUp(path)

	var responseData map[string]interface{}
	err = json.Unmarshal(body, &responseData)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return responseData, nil
}

// FetchJikanPictures fetches the pictures for an anime using the Jikan API.
// It returns a list of all raw image URLs.
func FetchJikanPictures(malID int) ([]string, error) {
	url := fmt.Sprintf("https://api.jikan.moe/v4/anime/%d/pictures", malID)

	response, err := makeGetRequest(url, nil)
	if err != nil {
		return nil, fmt.Errorf("Jikan API request failed: %v", err)
	}

	dataList, ok := response["data"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid Jikan API response format")
	}

	var urls []string
	for _, item := range dataList {
		if mapItem, ok := item.(map[string]interface{}); ok {
			for _, format := range []string{"jpg", "webp"} {
				if formatData, ok := mapItem[format].(map[string]interface{}); ok {
					if imgURL, ok := formatData["image_url"].(string); ok && imgURL != "" {
						urls = append(urls, imgURL)
					}
					if smallURL, ok := formatData["small_image_url"].(string); ok && smallURL != "" {
						urls = append(urls, smallURL)
					}
					if largeURL, ok := formatData["large_image_url"].(string); ok && largeURL != "" {
						urls = append(urls, largeURL)
					}
				}
			}
		}
	}

	return urls, nil
}

type JikanAnimeData struct {
	MalID         int     `json:"mal_id"`
	Title         string  `json:"title"`
	TitleEnglish  string  `json:"title_english"`
	TitleJapanese string  `json:"title_japanese"`
	Type          string  `json:"type"`
	Episodes      int     `json:"episodes"`
	Status        string  `json:"status"`
	Score         float64 `json:"score"`
	Season        string  `json:"season"`
	Year          int     `json:"year"`
}

func FetchJikanAnimeData(malID int) (*JikanAnimeData, error) {
	url := fmt.Sprintf("https://api.jikan.moe/v4/anime/%d", malID)
	response, err := makeGetRequest(url, nil)
	if err != nil {
		return nil, fmt.Errorf("Jikan API request failed: %v", err)
	}

	dataMap, ok := response["data"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid Jikan API response format")
	}

	dataBytes, err := json.Marshal(dataMap)
	if err != nil {
		return nil, err
	}

	var data JikanAnimeData
	err = json.Unmarshal(dataBytes, &data)
	if err != nil {
		return nil, err
	}

	return &data, nil
}

// JikanPrePlayBudget is the pre-play wait cap for episode metadata.
func JikanPrePlayBudget() time.Duration { return jikanPrePlayBudget }
