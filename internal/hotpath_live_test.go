package internal

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestLiveHotPathLatency times every network call that sits between
// "user picks a show" and "mpv is playing", against the user's real config.
// Read-only: it never writes tracker progress. Gated by CURD_LIVE_HOTPATH=1.
//
// Budget: each blocking step must finish (or be time-boxed) within
// hotPathBudget; slow metadata services must never gate playback.
func TestLiveHotPathLatency(t *testing.T) {
	if os.Getenv("CURD_LIVE_HOTPATH") != "1" {
		t.Skip("set CURD_LIVE_HOTPATH=1")
	}
	home, _ := os.UserHomeDir()
	cfgPath := filepath.Join(home, ".config", "curd", "curd.conf")
	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	// Isolate caches written by this probe from the user's real storage,
	// but read tokens from the real one (token refresh must persist there).
	SetGlobalConfig(&cfg)
	user := User{}
	SetGlobalUser(&user)

	const anilistID = 14719 // JoJo no Kimyou na Bouken (2012)
	const malID = 14719
	const ep = 5

	step := func(name string, fn func() error) time.Duration {
		start := time.Now()
		err := fn()
		d := time.Since(start)
		status := "ok"
		if err != nil {
			status = "ERR " + err.Error()
		}
		t.Logf("%-34s %8s  %s", name, d.Round(time.Millisecond), status)
		return d
	}

	step("LoadTokenForConfiguredTracking", func() error { return LoadTokenForConfiguredTracking(&cfg, &user) })
	step("FetchLatestMyAnimeList (bg refresh)", func() error {
		if !UsesMyAnimeListTracking(&cfg) {
			return nil
		}
		_, err := FetchLatestMyAnimeList(&cfg, &user)
		return err
	})
	step("GetAnimeMalID (AniList)", func() error { _, err := GetAnimeMalID(anilistID); return err })
	step("GetAnimeIDAndImage (AniList)", func() error { _, _, err := GetAnimeIDAndImage(anilistID); return err })
	step("AniSkip", func() error { _, err := GetAniSkipData(malID, ep); return err })

	// App-level Jikan paths, in an isolated storage dir (cold cache, as a
	// brand-new episode would be). Budgets are hard assertions.
	iso := cfg
	iso.StoragePath = t.TempDir()
	SetGlobalConfig(&iso)
	defer SetGlobalConfig(&cfg)

	budget := func(name string, max time.Duration, fn func() error) {
		if d := step(name, fn); d > max {
			t.Errorf("%s took %s, budget %s", name, d.Round(time.Millisecond), max)
		}
	}
	anime := &Anime{}
	budget("pre-play episode data (cold)", jikanPrePlayBudget+300*time.Millisecond, func() error {
		GetEpisodeDataWithin(malID, ep, anime, jikanPrePlayBudget)
		return nil
	})
	// Let the background lookup finish so the breaker/cache reflect it.
	_, _ = lookupJikanEpisode(malID, ep)
	budget("pre-play next episode", jikanPrePlayBudget+300*time.Millisecond, func() error {
		GetEpisodeDataWithin(malID, ep+1, anime, jikanPrePlayBudget)
		return nil
	})
	budget("filler list", jikanRequestTimeout+time.Second, func() error { _, err := FetchFillerEpisodes(malID); return err })
	budget("matching: Jikan pictures", jikanRequestTimeout+time.Second, func() error { _, err := FetchJikanPictures(malID); return err })
	budget("matching: Jikan anime data", jikanRequestTimeout+time.Second, func() error { _, err := FetchJikanAnimeData(malID); return err })
	budget("ID lookup (memoized)", 50*time.Millisecond, func() error { _, _, err := GetAnimeIDAndImage(anilistID); return err })
}
