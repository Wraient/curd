package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"
)

type liveTestAnime struct {
	Title     string
	AniListID int
	MalID     int
}

var (
	resolveLiveTestAnimeOnce sync.Once
	resolvedLiveTestAnime    []liveTestAnime
	resolvedLiveTestAnimeErr error
)

func TestTrackerLiveIntegration(t *testing.T) {
	config, anilistToken, _, cleanup := loadLiveTrackerTestContext(t)
	defer cleanup()

	testAnime := resolveLiveTestAnime(t)

	t.Run("dual-sync keeps latest remote update", func(t *testing.T) {
		resetLiveTrackerState(t, config, anilistToken)

		older := liveEntry(testAnime[0], "CURRENT", 2, 3, FuzzyDate{Year: 2026, Month: 1, Day: 2}, FuzzyDate{})
		if err := saveAniListTrackedEntry(anilistToken, older); err != nil {
			t.Fatalf("seed AniList entry: %v", err)
		}

		waitForRemoteTimestampTick()

		newer := liveEntry(testAnime[0], "PAUSED", 4, 8, FuzzyDate{Year: 2026, Month: 3, Day: 4}, FuzzyDate{})
		if err := saveMyAnimeListTrackedEntry(config, newer); err != nil {
			t.Fatalf("seed MyAnimeList entry: %v", err)
		}

		refreshBothTrackers(t, config)

		aniList, myAnimeList := fetchBothLists(t, config, anilistToken)
		assertEntryState(t, aniList, newer)
		assertEntryState(t, myAnimeList, newer)
	})

	t.Run("dual-sync copies AniList-only entry to MyAnimeList", func(t *testing.T) {
		resetLiveTrackerState(t, config, anilistToken)

		entry := liveEntry(testAnime[1], "CURRENT", 5, 6, FuzzyDate{Year: 2025, Month: 12, Day: 24}, FuzzyDate{})
		if err := saveAniListTrackedEntry(anilistToken, entry); err != nil {
			t.Fatalf("seed AniList entry: %v", err)
		}

		refreshBothTrackers(t, config)

		_, myAnimeList := fetchBothLists(t, config, anilistToken)
		assertEntryState(t, myAnimeList, entry)
	})

	t.Run("dual-sync copies MyAnimeList-only entry to AniList", func(t *testing.T) {
		resetLiveTrackerState(t, config, anilistToken)

		entry := liveEntry(testAnime[2], "PLANNING", 0, 7, FuzzyDate{}, FuzzyDate{})
		if err := saveMyAnimeListTrackedEntry(config, entry); err != nil {
			t.Fatalf("seed MyAnimeList entry: %v", err)
		}

		refreshBothTrackers(t, config)

		aniList, _ := fetchBothLists(t, config, anilistToken)
		assertEntryState(t, aniList, entry)
	})

	t.Run("replace MyAnimeList with AniList", func(t *testing.T) {
		resetLiveTrackerState(t, config, anilistToken)

		sourceEntry := liveEntry(testAnime[0], "CURRENT", 3, 5, FuzzyDate{Year: 2026, Month: 2, Day: 1}, FuzzyDate{})
		targetOnlyEntry := liveEntry(testAnime[3], "PAUSED", 1, 9, FuzzyDate{}, FuzzyDate{})

		if err := saveAniListTrackedEntry(anilistToken, sourceEntry); err != nil {
			t.Fatalf("seed AniList entry: %v", err)
		}
		if err := saveMyAnimeListTrackedEntry(config, targetOnlyEntry); err != nil {
			t.Fatalf("seed MyAnimeList entry: %v", err)
		}

		if err := ReplaceMyAnimeListWithAniList(config); err != nil {
			t.Fatalf("replace MAL with AniList: %v", err)
		}

		_, myAnimeList := fetchBothLists(t, config, anilistToken)
		assertEntryState(t, myAnimeList, sourceEntry)
		assertEntryMissing(t, myAnimeList, targetOnlyEntry.Media.ID)
	})

	t.Run("replace AniList with MyAnimeList", func(t *testing.T) {
		resetLiveTrackerState(t, config, anilistToken)

		sourceEntry := liveEntry(testAnime[1], "PAUSED", 2, 7, FuzzyDate{Year: 2026, Month: 2, Day: 5}, FuzzyDate{})
		targetOnlyEntry := liveEntry(testAnime[4], "CURRENT", 6, 4, FuzzyDate{}, FuzzyDate{})

		if err := saveMyAnimeListTrackedEntry(config, sourceEntry); err != nil {
			t.Fatalf("seed MyAnimeList entry: %v", err)
		}
		if err := saveAniListTrackedEntry(anilistToken, targetOnlyEntry); err != nil {
			t.Fatalf("seed AniList entry: %v", err)
		}

		if err := ReplaceAniListWithMyAnimeList(config); err != nil {
			t.Fatalf("replace AniList with MAL: %v", err)
		}

		aniList, _ := fetchBothLists(t, config, anilistToken)
		assertEntryState(t, aniList, sourceEntry)
		assertEntryMissing(t, aniList, targetOnlyEntry.Media.ID)
	})

	t.Run("merge remote lists keeps union and newest conflict", func(t *testing.T) {
		resetLiveTrackerState(t, config, anilistToken)

		aniOnly := liveEntry(testAnime[2], "CURRENT", 2, 6, FuzzyDate{}, FuzzyDate{})
		malOnly := liveEntry(testAnime[3], "PLANNING", 0, 0, FuzzyDate{}, FuzzyDate{})
		conflictOld := liveEntry(testAnime[4], "CURRENT", 1, 5, FuzzyDate{Year: 2026, Month: 1, Day: 10}, FuzzyDate{})
		conflictNew := liveEntry(testAnime[4], "PAUSED", 4, 9, FuzzyDate{Year: 2026, Month: 4, Day: 11}, FuzzyDate{})

		if err := saveAniListTrackedEntry(anilistToken, aniOnly); err != nil {
			t.Fatalf("seed AniList-only entry: %v", err)
		}
		if err := saveAniListTrackedEntry(anilistToken, conflictOld); err != nil {
			t.Fatalf("seed older AniList conflict entry: %v", err)
		}
		if err := saveMyAnimeListTrackedEntry(config, malOnly); err != nil {
			t.Fatalf("seed MAL-only entry: %v", err)
		}

		waitForRemoteTimestampTick()

		if err := saveMyAnimeListTrackedEntry(config, conflictNew); err != nil {
			t.Fatalf("seed newer MAL conflict entry: %v", err)
		}

		if err := MergeRemoteLists(config); err != nil {
			t.Fatalf("merge remote lists: %v", err)
		}

		aniList, myAnimeList := fetchBothLists(t, config, anilistToken)
		for _, list := range []AnimeList{aniList, myAnimeList} {
			assertEntryState(t, list, aniOnly)
			assertEntryState(t, list, malOnly)
			assertEntryState(t, list, conflictNew)
		}
	})

	t.Run("tracker mode transitions local anilist myanimelist both", func(t *testing.T) {
		resetLiveTrackerState(t, config, anilistToken)

		aniEntry := liveEntry(testAnime[0], "CURRENT", 3, 6, FuzzyDate{}, FuzzyDate{})
		malEntry := liveEntry(testAnime[1], "PAUSED", 1, 8, FuzzyDate{}, FuzzyDate{})
		if err := saveAniListTrackedEntry(anilistToken, aniEntry); err != nil {
			t.Fatalf("seed AniList transition entry: %v", err)
		}
		if err := saveMyAnimeListTrackedEntry(config, malEntry); err != nil {
			t.Fatalf("seed MAL transition entry: %v", err)
		}

		tempStorage := t.TempDir()
		copyTrackerFileForTest(t, filepath.Join(os.ExpandEnv(config.StoragePath), "anilist_token.json"), filepath.Join(tempStorage, "anilist_token.json"))
		copyTrackerFileForTest(t, filepath.Join(os.ExpandEnv(config.StoragePath), "myanimelist_token.json"), filepath.Join(tempStorage, "myanimelist_token.json"))

		localHistoryPath := filepath.Join(tempStorage, "curd_history.txt")
		LocalUpdateAnime(localHistoryPath, testAnime[2].AniListID, "local-provider-id", 7, 0, 24, testAnime[2].Title, "allanime")

		tempConfig := *config
		tempConfig.StoragePath = tempStorage
		tempConfig.MyAnimeListImported = true

		localUser := &User{}
		tempConfig.TrackingRemote = TrackingRemoteNone
		SetGlobalConfig(&tempConfig)
		SetGlobalUser(localUser)
		if err := RefreshUserAnimeList(&tempConfig, localUser); err != nil {
			t.Fatalf("refresh local mode: %v", err)
		}
		assertEntryState(t, localUser.AnimeList, Entry{
			Media:    Media{ID: testAnime[2].AniListID},
			Status:   "CURRENT",
			Progress: 6,
		})

		aniUser := &User{}
		tempConfig.TrackingRemote = TrackingRemoteAniList
		SetGlobalUser(aniUser)
		if err := EnsureConfiguredTrackersReady(&tempConfig, aniUser); err != nil {
			t.Fatalf("ensure AniList mode ready: %v", err)
		}
		if err := RefreshUserAnimeList(&tempConfig, aniUser); err != nil {
			t.Fatalf("refresh AniList mode: %v", err)
		}
		assertEntryState(t, aniUser.AnimeList, aniEntry)
		assertEntryMissing(t, aniUser.AnimeList, testAnime[2].AniListID)

		malUser := &User{}
		tempConfig.TrackingRemote = TrackingRemoteMyAnimeList
		SetGlobalUser(malUser)
		if err := EnsureConfiguredTrackersReady(&tempConfig, malUser); err != nil {
			t.Fatalf("ensure MAL mode ready: %v", err)
		}
		if err := RefreshUserAnimeList(&tempConfig, malUser); err != nil {
			t.Fatalf("refresh MAL mode: %v", err)
		}
		assertEntryState(t, malUser.AnimeList, malEntry)

		bothUser := &User{}
		tempConfig.TrackingRemote = TrackingRemoteBoth
		SetGlobalUser(bothUser)
		if err := EnsureConfiguredTrackersReady(&tempConfig, bothUser); err != nil {
			t.Fatalf("ensure both mode ready: %v", err)
		}
		if err := RefreshUserAnimeList(&tempConfig, bothUser); err != nil {
			t.Fatalf("refresh both mode: %v", err)
		}
		assertEntryState(t, bothUser.AnimeList, aniEntry)
		assertEntryState(t, bothUser.AnimeList, malEntry)
	})
}

func TestTrackerLiveRuntimeIntegration(t *testing.T) {
	config, anilistToken, _, cleanup := loadLiveTrackerTestContext(t)
	defer cleanup()

	testAnime := resolveLiveTestAnime(t)

	t.Run("runtime helpers update both trackers in dual mode", func(t *testing.T) {
		resetLiveTrackerState(t, config, anilistToken)

		bothConfig := *config
		bothConfig.TrackingRemote = TrackingRemoteBoth
		SetGlobalConfig(&bothConfig)

		if err := AddAnimeToWatchingList(testAnime[0].AniListID, anilistToken); err != nil {
			t.Fatalf("add anime to watching list: %v", err)
		}
		if err := UpdateAnimeProgress(anilistToken, testAnime[0].AniListID, 3); err != nil {
			t.Fatalf("update anime progress: %v", err)
		}
		if err := UpdateAnimeStatus(anilistToken, testAnime[0].AniListID, "PAUSED"); err != nil {
			t.Fatalf("update anime status: %v", err)
		}
		withInjectedStdin(t, "9\n", func() {
			if err := RateAnime(anilistToken, testAnime[0].AniListID); err != nil {
				t.Fatalf("rate anime: %v", err)
			}
		})

		aniList, myAnimeList := fetchBothLists(t, &bothConfig, anilistToken)
		expected := liveEntry(testAnime[0], "PAUSED", 3, 9, FuzzyDate{}, FuzzyDate{})
		for _, list := range []AnimeList{aniList, myAnimeList} {
			assertEntryState(t, list, expected)
		}
	})

	t.Run("runtime rewatch cycle updates both trackers", func(t *testing.T) {
		resetLiveTrackerState(t, config, anilistToken)

		bothConfig := *config
		bothConfig.TrackingRemote = TrackingRemoteBoth
		SetGlobalConfig(&bothConfig)

		completed := liveEntry(testAnime[1], "COMPLETED", 12, 8, FuzzyDate{Year: 2025, Month: 1, Day: 1}, FuzzyDate{Year: 2025, Month: 2, Day: 2})
		if err := saveAniListTrackedEntry(anilistToken, completed); err != nil {
			t.Fatalf("seed AniList completed entry: %v", err)
		}
		if err := saveMyAnimeListTrackedEntry(&bothConfig, completed); err != nil {
			t.Fatalf("seed MAL completed entry: %v", err)
		}
		seededAniList, _ := fetchBothLists(t, &bothConfig, anilistToken)
		existingEntry := findEntryByMediaID(seededAniList, testAnime[1].AniListID)
		if existingEntry == nil {
			t.Fatalf("expected seeded completed entry to exist")
		}
		finalProgress := existingEntry.Progress

		anime := Anime{
			AnilistId:   testAnime[1].AniListID,
			MalId:       testAnime[1].MalID,
			Repeat:      0,
			StartedAt:   completed.StartedAt,
			CompletedAt: completed.CompletedAt,
			Ep: Episode{
				Number: finalProgress,
			},
		}
		if err := StartAnimeRewatch(anilistToken, anime); err != nil {
			t.Fatalf("start rewatch: %v", err)
		}

		aniList, myAnimeList := fetchBothLists(t, &bothConfig, anilistToken)
		rewatching := liveEntry(testAnime[1], "REPEATING", 0, 8, FuzzyDate{}, FuzzyDate{})
		for _, list := range []AnimeList{aniList, myAnimeList} {
			assertEntryState(t, list, rewatching)
		}

		if err := CompleteAnimeRewatch(anilistToken, anime); err != nil {
			t.Fatalf("complete rewatch: %v", err)
		}

		aniList, myAnimeList = fetchBothLists(t, &bothConfig, anilistToken)
		rewatchDone := liveEntry(testAnime[1], "COMPLETED", finalProgress, 8, FuzzyDate{}, FuzzyDate{})
		rewatchDone.Repeat = 1
		assertEntryState(t, aniList, rewatchDone)
		assertEntryState(t, myAnimeList, liveEntry(testAnime[1], "COMPLETED", finalProgress, 8, FuzzyDate{}, FuzzyDate{}))

		combinedUser := &User{}
		SetGlobalConfig(&bothConfig)
		SetGlobalUser(combinedUser)
		if err := RefreshUserAnimeList(&bothConfig, combinedUser); err != nil {
			t.Fatalf("refresh combined user after rewatch completion: %v", err)
		}
		assertEntryState(t, combinedUser.AnimeList, rewatchDone)
	})

	t.Run("runtime AniList-only updates sync later in dual mode", func(t *testing.T) {
		resetLiveTrackerState(t, config, anilistToken)

		aniConfig := *config
		aniConfig.TrackingRemote = TrackingRemoteAniList
		SetGlobalConfig(&aniConfig)

		if err := AddAnimeToWatchingList(testAnime[2].AniListID, anilistToken); err != nil {
			t.Fatalf("add AniList-only anime: %v", err)
		}
		if err := UpdateAnimeProgress(anilistToken, testAnime[2].AniListID, 4); err != nil {
			t.Fatalf("update AniList-only progress: %v", err)
		}

		_, myAnimeList := fetchBothLists(t, &aniConfig, anilistToken)
		assertEntryMissing(t, myAnimeList, testAnime[2].AniListID)

		refreshBothTrackers(t, &aniConfig)

		_, myAnimeList = fetchBothLists(t, &aniConfig, anilistToken)
		assertEntryState(t, myAnimeList, liveEntry(testAnime[2], "CURRENT", 4, 0, FuzzyDate{}, FuzzyDate{}))
	})

	t.Run("runtime MyAnimeList-only updates sync later in dual mode", func(t *testing.T) {
		resetLiveTrackerState(t, config, anilistToken)

		malConfig := *config
		malConfig.TrackingRemote = TrackingRemoteMyAnimeList
		SetGlobalConfig(&malConfig)

		if err := AddAnimeToWatchingList(testAnime[3].AniListID, anilistToken); err != nil {
			t.Fatalf("add MAL-only anime: %v", err)
		}
		if err := UpdateAnimeStatus(anilistToken, testAnime[3].AniListID, "PAUSED"); err != nil {
			t.Fatalf("pause MAL-only anime: %v", err)
		}

		aniList, _ := fetchBothLists(t, &malConfig, anilistToken)
		assertEntryMissing(t, aniList, testAnime[3].AniListID)

		refreshBothTrackers(t, &malConfig)

		aniList, _ = fetchBothLists(t, &malConfig, anilistToken)
		assertEntryState(t, aniList, liveEntry(testAnime[3], "PAUSED", 0, 0, FuzzyDate{}, FuzzyDate{}))
	})

	t.Run("next episode runtime path updates local history and tracker progress", func(t *testing.T) {
		resetLiveTrackerState(t, config, anilistToken)

		bothConfig := *config
		bothConfig.TrackingRemote = TrackingRemoteBoth
		SetGlobalConfig(&bothConfig)

		if err := AddAnimeToWatchingList(testAnime[4].AniListID, anilistToken); err != nil {
			t.Fatalf("seed next-episode anime: %v", err)
		}

		tempStorage := t.TempDir()
		localHistoryPath := filepath.Join(tempStorage, "curd_history.txt")
		anime := &Anime{
			Title: AnimeTitle{
				English: testAnime[4].Title,
				Romaji:  testAnime[4].Title,
			},
			AnilistId:     testAnime[4].AniListID,
			MalId:         testAnime[4].MalID,
			TotalEpisodes: 12,
			ProviderId:    "runtime-provider-id",
			Ep: Episode{
				Number: 1,
			},
		}

		StartNextEpisode(anime, &bothConfig, localHistoryPath, anilistToken)
		waitForAsyncTrackerUpdate()

		if anime.Ep.Number != 2 {
			t.Fatalf("expected next episode number 2, got %d", anime.Ep.Number)
		}

		localList := BuildLocalAnimeList(tempStorage)
		assertEntryState(t, localList, Entry{
			Media:    Media{ID: testAnime[4].AniListID},
			Status:   "CURRENT",
			Progress: 1,
		})

		aniList, myAnimeList := fetchBothLists(t, &bothConfig, anilistToken)
		expected := liveEntry(testAnime[4], "CURRENT", 1, 0, FuzzyDate{}, FuzzyDate{})
		for _, list := range []AnimeList{aniList, myAnimeList} {
			assertEntryState(t, list, expected)
		}
	})
}

func loadLiveTrackerTestContext(t *testing.T) (*CurdConfig, string, []liveTestAnime, func()) {
	t.Helper()

	if os.Getenv("CURD_RUN_TRACKER_LIVE_INTEGRATION") != "1" {
		t.Skip("set CURD_RUN_TRACKER_LIVE_INTEGRATION=1 to run live tracker integration tests")
	}

	configPath := os.Getenv("CURD_CONFIG_PATH")
	if configPath == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			t.Fatalf("resolve home dir: %v", err)
		}
		configPath = filepath.Join(homeDir, ".config", "curd", "curd.conf")
	}

	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	config.TrackingConfigured = true
	config.TrackingLocal = true

	previousGlobalConfig := GetGlobalConfig()
	previousGlobalUser := GetGlobalUser()
	SetGlobalConfig(&config)

	user := &User{}
	SetGlobalUser(user)
	if err := EnsureConfiguredTrackersReady(&config, user); err != nil {
		t.Fatalf("ensure trackers ready: %v", err)
	}

	anilistToken, err := GetTokenFromFile(filepath.Join(os.ExpandEnv(config.StoragePath), "anilist_token.json"))
	if err != nil {
		t.Fatalf("load AniList token: %v", err)
	}

	originalAniList, err := fetchAniListAnimeListFromToken(anilistToken)
	if err != nil {
		t.Fatalf("fetch original AniList state: %v", err)
	}
	originalMyAnimeList, err := FetchLatestMyAnimeList(&config, &User{})
	if err != nil {
		t.Fatalf("fetch original MyAnimeList state: %v", err)
	}

	cleanup := func() {
		restoreLiveTrackerState(t, &config, anilistToken, originalAniList, originalMyAnimeList)
		SetGlobalConfig(previousGlobalConfig)
		SetGlobalUser(previousGlobalUser)
	}

	return &config, anilistToken, resolveLiveTestAnime(t), cleanup
}

func resolveLiveTestAnime(t *testing.T) []liveTestAnime {
	t.Helper()

	resolveLiveTestAnimeOnce.Do(func() {
		titles := []string{
			"Cowboy Bebop",
			"Odd Taxi",
			"Barakamon",
			"Death Parade",
			"A Place Further than the Universe",
		}

		results := make([]liveTestAnime, 0, len(titles))
		for _, title := range titles {
			options, err := SearchAnimeAnilist(title, "")
			if err != nil {
				resolvedLiveTestAnimeErr = err
				return
			}
			if len(options) == 0 {
				resolvedLiveTestAnimeErr = fmt.Errorf("no AniList result for %q", title)
				return
			}
			anilistID, err := strconv.Atoi(options[0].Key)
			if err != nil {
				resolvedLiveTestAnimeErr = err
				return
			}
			malID, err := GetAnimeMalID(anilistID)
			if err != nil {
				resolvedLiveTestAnimeErr = err
				return
			}
			if malID == 0 {
				resolvedLiveTestAnimeErr = fmt.Errorf("AniList title %q has no MAL mapping", title)
				return
			}
			results = append(results, liveTestAnime{
				Title:     title,
				AniListID: anilistID,
				MalID:     malID,
			})
		}
		resolvedLiveTestAnime = results
	})

	if resolvedLiveTestAnimeErr != nil {
		t.Fatalf("resolve live test anime: %v", resolvedLiveTestAnimeErr)
	}
	return resolvedLiveTestAnime
}

func restoreLiveTrackerState(t *testing.T, config *CurdConfig, anilistToken string, aniListSnapshot, myAnimeListSnapshot AnimeList) {
	t.Helper()

	if err := wipeAniListRemote(config); err != nil {
		t.Fatalf("wipe AniList during restore: %v", err)
	}
	if err := wipeMyAnimeListRemote(config); err != nil {
		t.Fatalf("wipe MyAnimeList during restore: %v", err)
	}
	if err := upsertAnimeListToAniList(anilistToken, aniListSnapshot); err != nil {
		t.Fatalf("restore AniList snapshot: %v", err)
	}
	if err := upsertAnimeListToMyAnimeList(config, myAnimeListSnapshot); err != nil {
		t.Fatalf("restore MyAnimeList snapshot: %v", err)
	}
}

func resetLiveTrackerState(t *testing.T, config *CurdConfig, _ string) {
	t.Helper()

	if err := wipeAniListRemote(config); err != nil {
		t.Fatalf("wipe AniList for test: %v", err)
	}
	if err := wipeMyAnimeListRemote(config); err != nil {
		t.Fatalf("wipe MyAnimeList for test: %v", err)
	}
}

func liveEntry(anime liveTestAnime, status string, progress int, score float64, startedAt, completedAt FuzzyDate) Entry {
	return Entry{
		Media: Media{
			ID:    anime.AniListID,
			MalID: anime.MalID,
			Title: AnimeTitle{
				English: anime.Title,
				Romaji:  anime.Title,
			},
		},
		Progress:    progress,
		Score:       score,
		Status:      status,
		StartedAt:   startedAt,
		CompletedAt: completedAt,
	}
}

func refreshBothTrackers(t *testing.T, config *CurdConfig) {
	t.Helper()

	testConfig := *config
	testConfig.TrackingRemote = TrackingRemoteBoth
	testUser := &User{}
	SetGlobalConfig(&testConfig)
	SetGlobalUser(testUser)
	if err := RefreshUserAnimeList(&testConfig, testUser); err != nil {
		t.Fatalf("refresh both trackers: %v", err)
	}
}

func fetchBothLists(t *testing.T, config *CurdConfig, anilistToken string) (AnimeList, AnimeList) {
	t.Helper()

	aniList, err := fetchAniListAnimeListFromToken(anilistToken)
	if err != nil {
		t.Fatalf("fetch AniList: %v", err)
	}
	myAnimeList, err := FetchLatestMyAnimeList(config, &User{})
	if err != nil {
		t.Fatalf("fetch MyAnimeList: %v", err)
	}
	return aniList, myAnimeList
}

func findEntryByMediaID(list AnimeList, mediaID int) *Entry {
	for _, entry := range getEntriesByCategory(list, "ALL") {
		if entry.Media.ID == mediaID {
			copied := entry
			return &copied
		}
	}
	return nil
}

func assertEntryState(t *testing.T, list AnimeList, expected Entry) {
	t.Helper()

	entry := findEntryByMediaID(list, expected.Media.ID)
	if entry == nil {
		t.Fatalf("expected media ID %d to exist", expected.Media.ID)
	}
	if entry.Status != expected.Status {
		t.Fatalf("media ID %d status = %s, want %s", expected.Media.ID, entry.Status, expected.Status)
	}
	if entry.Progress != expected.Progress {
		t.Fatalf("media ID %d progress = %d, want %d", expected.Media.ID, entry.Progress, expected.Progress)
	}
	if entry.Score != expected.Score {
		t.Fatalf("media ID %d score = %.1f, want %.1f", expected.Media.ID, entry.Score, expected.Score)
	}
	if expected.Repeat != 0 && entry.Repeat != expected.Repeat {
		t.Fatalf("media ID %d repeat = %d, want %d", expected.Media.ID, entry.Repeat, expected.Repeat)
	}
	if expected.StartedAt != (FuzzyDate{}) && entry.StartedAt != expected.StartedAt {
		t.Fatalf("media ID %d startedAt = %+v, want %+v", expected.Media.ID, entry.StartedAt, expected.StartedAt)
	}
	if expected.CompletedAt != (FuzzyDate{}) && entry.CompletedAt != expected.CompletedAt {
		t.Fatalf("media ID %d completedAt = %+v, want %+v", expected.Media.ID, entry.CompletedAt, expected.CompletedAt)
	}
}

func assertEntryMissing(t *testing.T, list AnimeList, mediaID int) {
	t.Helper()

	if entry := findEntryByMediaID(list, mediaID); entry != nil {
		t.Fatalf("expected media ID %d to be missing, found %+v", mediaID, *entry)
	}
}

func copyTrackerFileForTest(t *testing.T, sourcePath, destinationPath string) {
	t.Helper()

	data, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read %s: %v", sourcePath, err)
	}
	if err := os.WriteFile(destinationPath, data, 0o600); err != nil {
		t.Fatalf("write %s: %v", destinationPath, err)
	}
}

func waitForRemoteTimestampTick() {
	time.Sleep(2 * time.Second)
}

func waitForAsyncTrackerUpdate() {
	time.Sleep(3 * time.Second)
}

func withInjectedStdin(t *testing.T, input string, fn func()) {
	t.Helper()

	tempFile, err := os.CreateTemp(t.TempDir(), "stdin-*")
	if err != nil {
		t.Fatalf("create temp stdin file: %v", err)
	}
	if _, err := tempFile.WriteString(input); err != nil {
		t.Fatalf("write temp stdin file: %v", err)
	}
	if _, err := tempFile.Seek(0, 0); err != nil {
		t.Fatalf("seek temp stdin file: %v", err)
	}

	originalStdin := os.Stdin
	os.Stdin = tempFile
	defer func() {
		os.Stdin = originalStdin
		tempFile.Close()
	}()

	fn()
}
