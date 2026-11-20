package internal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// Disk cache for Jikan metadata. Episode titles and filler/recap flags are
// effectively immutable once aired, but Jikan is slow and aggressively rate
// limited — and callers sit on the critical playback path (GetEpisodeData
// blocks every episode start, FetchFillerEpisodes pages per anime). Without
// this cache a hanging api.jikan.moe stalls each episode by the 15s HTTP
// timeout, repeatedly. TTL is a correctness backstop for corrected flags,
// not a freshness requirement.
const jikanCacheFileName = "jikan_cache.json"

// jikanCacheTTL bounds staleness of filler/recap flags (occasionally
// corrected upstream after airing). Episode titles never change.
const jikanCacheTTL = 7 * 24 * time.Hour

type jikanEpisodeCacheEntry struct {
	TitleRomaji   string    `json:"title_romaji"`
	TitleEnglish  string    `json:"title_english"`
	TitleJapanese string    `json:"title_japanese"`
	Aired         string    `json:"aired"`
	Duration      int       `json:"duration"`
	Synopsis      string    `json:"synopsis"`
	IsFiller      bool      `json:"is_filler"`
	IsRecap       bool      `json:"is_recap"`
	FetchedAt     time.Time `json:"fetched_at"`
}

type jikanFillerCacheEntry struct {
	Episodes  []int     `json:"episodes"`
	FetchedAt time.Time `json:"fetched_at"`
}

type jikanCacheFile struct {
	Episodes map[string]jikanEpisodeCacheEntry `json:"episodes"`
	Filler   map[string]jikanFillerCacheEntry  `json:"filler"`
	// Negative records failed lookups ("ep:<malID>:<ep>", "fl:<malID>") so a
	// Jikan outage costs one timeout per hour, not one per episode start.
	Negative map[string]time.Time `json:"negative"`
}

// jikanNegativeTTL bounds how long a failed lookup suppresses retries.
// Outages last hours; an hour of instant defaults beats an hour of stalls,
// and the next hour re-probes automatically.
const jikanNegativeTTL = time.Hour

func jikanNegativeKey(kind, key string) string {
	return kind + ":" + key
}

// jikanNegativeHit reports whether a failed lookup is still suppressed.
// Caller must hold jikanCacheMu (it reads the caller's loaded cache).
func jikanNegativeHit(c jikanCacheFile, nkey string, now time.Time) bool {
	ts, ok := c.Negative[nkey]
	return ok && now.Sub(ts) <= jikanNegativeTTL
}

// noteJikanNegative stamps a failed lookup on the caller's loaded cache;
// the caller saves afterwards. Caller must hold jikanCacheMu.
func noteJikanNegative(c *jikanCacheFile, nkey string, now time.Time) {
	if c.Negative == nil {
		c.Negative = make(map[string]time.Time)
	}
	c.Negative[nkey] = now
}

var jikanCacheMu sync.Mutex

// jikanCachePath resolves the cache file from global config. Empty means
// caching is unavailable (no storage configured) — callers fetch live.
func jikanCachePath() string {
	cfg := GetGlobalConfig()
	if cfg == nil {
		return ""
	}
	dir := os.ExpandEnv(cfg.StoragePath)
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, jikanCacheFileName)
}

func jikanEpisodeCacheKey(malID, episodeNo int) string {
	return itoa(malID) + ":" + itoa(episodeNo)
}

func jikanFillerCacheKey(malID int) string {
	return itoa(malID)
}

func itoa(n int) string {
	return strconv.Itoa(n)
}

// loadJikanCacheFile reads the cache tolerantly: missing or corrupt files
// yield an empty cache, never an error that could stall playback.
func loadJikanCacheFile(path string) jikanCacheFile {
	c := jikanCacheFile{
		Episodes: make(map[string]jikanEpisodeCacheEntry),
		Filler:   make(map[string]jikanFillerCacheEntry),
		Negative: make(map[string]time.Time),
	}
	if path == "" {
		return c
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return c
	}
	var parsed jikanCacheFile
	if err := json.Unmarshal(raw, &parsed); err != nil {
		Log("Jikan cache unreadable, starting fresh: " + err.Error())
		return c
	}
	if parsed.Episodes != nil {
		c.Episodes = parsed.Episodes
	}
	if parsed.Filler != nil {
		c.Filler = parsed.Filler
	}
	if parsed.Negative != nil {
		c.Negative = parsed.Negative
	} else {
		c.Negative = make(map[string]time.Time)
	}
	return c
}

// saveJikanCacheFile persists best-effort; failures are logged, never fatal.
func saveJikanCacheFile(path string, c jikanCacheFile) {
	if path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	// Expired negative entries carry no information; drop them so the
	// file doesn't grow forever.
	now := time.Now()
	for k, ts := range c.Negative {
		if now.Sub(ts) > jikanNegativeTTL && now.Sub(ts) > jikanHostDownTTL {
			delete(c.Negative, k)
		}
	}
	raw, err := json.Marshal(c)
	if err != nil {
		return
	}
	// Temp file + rename: a crash or a second curd instance mid-write must
	// not leave truncated JSON (which would discard the whole cache).
	tmp, err := os.CreateTemp(filepath.Dir(path), jikanCacheFileName+".*.tmp")
	if err != nil {
		Log("Jikan cache write failed: " + err.Error())
		return
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		Log("Jikan cache write failed: " + err.Error())
		return
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		Log("Jikan cache write failed: " + err.Error())
	}
}

func jikanEntryFresh(fetchedAt time.Time, now time.Time) bool {
	if fetchedAt.IsZero() {
		return false
	}
	return now.Sub(fetchedAt) <= jikanCacheTTL
}

// applyJikanEpisodeEntry copies a cache entry onto the live episode struct —
// the same fields GetEpisodeData fills from the network response.
func applyJikanEpisodeEntry(anime *Anime, e jikanEpisodeCacheEntry) {
	if anime == nil {
		return
	}
	anime.Ep.Title.Romaji = e.TitleRomaji
	anime.Ep.Title.English = e.TitleEnglish
	anime.Ep.Title.Japanese = e.TitleJapanese
	anime.Ep.Aired = e.Aired
	anime.Ep.Duration = e.Duration
	anime.Ep.Synopsis = e.Synopsis
	anime.Ep.IsFiller = e.IsFiller
	anime.Ep.IsRecap = e.IsRecap
}

// jikanHostDownTTL is how long one Jikan outage signal (timeout, transport
// error, 5xx) suppresses all Jikan calls. Persisted in the cache file so a
// relaunch during the outage doesn't re-pay the timeout on its first call.
var jikanHostDownTTL = 10 * time.Minute

const jikanHostNegativeKey = "host:jikan"

// jikanHostState mirrors the persisted breaker in memory, keyed by cache
// path so isolated storage dirs (tests, other profiles) don't share it.
var jikanHostState struct {
	mu        sync.Mutex
	path      string
	loaded    bool
	downSince time.Time
}

func jikanHostLoadLocked(path string) {
	if jikanHostState.loaded && jikanHostState.path == path {
		return
	}
	jikanHostState.path = path
	jikanHostState.loaded = true
	jikanHostState.downSince = time.Time{}
	if path == "" {
		return
	}
	jikanCacheMu.Lock()
	c := loadJikanCacheFile(path)
	jikanCacheMu.Unlock()
	jikanHostState.downSince = c.Negative[jikanHostNegativeKey]
}

// jikanHostDown reports whether the outage breaker is open.
func jikanHostDown(path string, now time.Time) bool {
	jikanHostState.mu.Lock()
	defer jikanHostState.mu.Unlock()
	jikanHostLoadLocked(path)
	since := jikanHostState.downSince
	return !since.IsZero() && now.Sub(since) <= jikanHostDownTTL && !since.After(now.Add(time.Minute))
}

func markJikanHostDown(path string, now time.Time) {
	jikanHostState.mu.Lock()
	jikanHostLoadLocked(path)
	jikanHostState.downSince = now
	jikanHostState.mu.Unlock()
	if path == "" {
		return
	}
	jikanCacheMu.Lock()
	c := loadJikanCacheFile(path)
	noteJikanNegative(&c, jikanHostNegativeKey, now)
	saveJikanCacheFile(path, c)
	jikanCacheMu.Unlock()
	Log("Jikan marked unavailable for " + jikanHostDownTTL.String() + "; metadata calls skipped until then")
}

func markJikanHostUp(path string) {
	jikanHostState.mu.Lock()
	jikanHostLoadLocked(path)
	wasDown := !jikanHostState.downSince.IsZero()
	jikanHostState.downSince = time.Time{}
	jikanHostState.mu.Unlock()
	if !wasDown || path == "" {
		return
	}
	jikanCacheMu.Lock()
	c := loadJikanCacheFile(path)
	delete(c.Negative, jikanHostNegativeKey)
	saveJikanCacheFile(path, c)
	jikanCacheMu.Unlock()
}
