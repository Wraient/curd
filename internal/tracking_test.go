package internal

import (
	"testing"
	"time"
)

func TestNormalizeRemoteTrackerAliases(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"":                    "none",
		"none":                "none",
		"off":                 "none",
		"local":               "none",
		"anilist":             "anilist",
		"ani-list":            "anilist",
		"mal":                 "myanimelist",
		"myanimelist":         "myanimelist",
		"my-anime-list":       "myanimelist",
		"my_anime_list":       "myanimelist",
		"both":                "anilist+myanimelist",
		"anilist,myanimelist": "anilist+myanimelist",
		"not-a-provider":      "",
	}

	for input, want := range cases {
		if got := normalizeRemoteTracker(input); got != want {
			t.Fatalf("normalizeRemoteTracker(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNormalizeTrackingConfigAlwaysKeepsLocalHistoryEnabled(t *testing.T) {
	t.Parallel()

	config := &CurdConfig{
		TrackingLocal:      false,
		TrackingRemote:     "none",
		TrackingConfigured: true,
	}

	normalizeTrackingConfig(config)

	if !config.TrackingLocal {
		t.Fatalf("expected local tracking to be re-enabled when both trackers are disabled")
	}
	if config.TrackingRemote != TrackingRemoteNone {
		t.Fatalf("expected remote tracking to stay disabled, got %q", config.TrackingRemote)
	}
}

func TestNormalizeTrackingConfigMarksInvalidRemoteForPrompt(t *testing.T) {
	t.Parallel()

	config := &CurdConfig{
		TrackingLocal:      true,
		TrackingRemote:     "totally-invalid",
		TrackingConfigured: true,
	}

	normalizeTrackingConfig(config)

	if config.TrackingConfigured {
		t.Fatalf("expected invalid tracking config to require setup prompt")
	}
	if config.TrackingRemote != TrackingRemoteAniList {
		t.Fatalf("expected invalid remote tracker to normalize to the legacy AniList default, got %q", config.TrackingRemote)
	}
	if !config.TrackingLocal {
		t.Fatalf("expected local history to stay enabled after normalization")
	}
}

func TestGetOrderedCategoriesHidesRemoteOnlyEntriesForLocalTracking(t *testing.T) {
	t.Parallel()

	config := &CurdConfig{
		MenuOrder:      "CURRENT,UPDATE,PLANNING,ALL,PROVIDER",
		TrackingLocal:  true,
		TrackingRemote: TrackingRemoteNone,
	}

	categories := getOrderedCategories(config)
	if len(categories) != 4 {
		t.Fatalf("expected 4 visible categories, got %d", len(categories))
	}
	if categories[0].Key != "CURRENT" || categories[1].Key != "ALL" || categories[2].Key != "PROVIDER" || categories[3].Key != "TRACKER" {
		t.Fatalf("unexpected categories for local-only tracking: %+v", categories)
	}
}

func TestLocalAnimeListFromHistoryUsesNextEpisodeAsProgressBase(t *testing.T) {
	t.Parallel()

	list := localAnimeListFromHistory([]Anime{
		{
			AnilistId: 123,
			MalId:     456,
			Title: AnimeTitle{
				English: "Frieren",
				Romaji:  "Sousou no Frieren",
			},
			Ep: Episode{
				Number: 7,
			},
			TotalEpisodes: 28,
		},
	})

	if len(list.Watching) != 1 {
		t.Fatalf("expected one local entry, got %d", len(list.Watching))
	}

	entry := list.Watching[0]
	if entry.Media.ID != 123 || entry.Media.MalID != 456 {
		t.Fatalf("unexpected IDs in local entry: %+v", entry.Media)
	}
	if entry.Progress != 6 {
		t.Fatalf("expected progress 6 from next-episode value 7, got %d", entry.Progress)
	}
	if entry.Status != "CURRENT" {
		t.Fatalf("expected CURRENT status, got %q", entry.Status)
	}
}

func TestMyAnimeListStatusConversion(t *testing.T) {
	t.Parallel()

	if got := myAnimeListStatusToAniListStatus("watching", true); got != "REPEATING" {
		t.Fatalf("expected rewatching MAL entry to map to REPEATING, got %q", got)
	}
	if got := myAnimeListStatusToAniListStatus("on_hold", false); got != "PAUSED" {
		t.Fatalf("expected on_hold to map to PAUSED, got %q", got)
	}
	if got := aniListStatusToMyAnimeListStatus("REPEATING"); got != "watching" {
		t.Fatalf("expected REPEATING to map to watching, got %q", got)
	}
	if got := aniListStatusToMyAnimeListStatus("PLANNING"); got != "plan_to_watch" {
		t.Fatalf("expected PLANNING to map to plan_to_watch, got %q", got)
	}
}

func TestDualSyncModeEnablesBothRemoteTrackers(t *testing.T) {
	t.Parallel()

	config := &CurdConfig{TrackingRemote: TrackingRemoteBoth}
	if !UsesAniListTracking(config) {
		t.Fatalf("expected dual-sync mode to include AniList")
	}
	if !UsesMyAnimeListTracking(config) {
		t.Fatalf("expected dual-sync mode to include MyAnimeList")
	}
	if !UsesDualRemoteTracking(config) {
		t.Fatalf("expected dual-sync mode to report itself as dual remote")
	}
}

func TestExtractOAuthCodeFromInput(t *testing.T) {
	t.Parallel()

	code, err := extractOAuthCodeFromInput("http://localhost:8123/oauth/callback?code=test-code&state=expected", "expected")
	if err != nil {
		t.Fatalf("expected callback URL to parse, got error: %v", err)
	}
	if code != "test-code" {
		t.Fatalf("expected parsed code to be test-code, got %q", code)
	}

	code, err = extractOAuthCodeFromInput("raw-code", "expected")
	if err != nil {
		t.Fatalf("expected raw code to parse, got error: %v", err)
	}
	if code != "raw-code" {
		t.Fatalf("expected raw code passthrough, got %q", code)
	}

	if _, err := extractOAuthCodeFromInput("http://localhost:8123/oauth/callback?code=test-code&state=wrong", "expected"); err == nil {
		t.Fatalf("expected state mismatch to fail")
	}
}

func TestMergeAnimeEntriesPrefersNewestRemoteUpdate(t *testing.T) {
	t.Parallel()

	older := Entry{
		Media:     Media{ID: 1, MalID: 101},
		Status:    "CURRENT",
		Progress:  3,
		UpdatedAt: time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC),
	}
	newer := Entry{
		Media:     Media{ID: 1, MalID: 101},
		Status:    "COMPLETED",
		Progress:  12,
		UpdatedAt: time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC),
	}

	merged := mergeAnimeEntries(older, newer)
	if merged.Status != "COMPLETED" || merged.Progress != 12 {
		t.Fatalf("expected latest entry to win, got %+v", merged)
	}
}

func TestBuildDualRemoteSyncPlanUsesNewestEntryAndPropagatesMissingAnime(t *testing.T) {
	t.Parallel()

	aniList := AnimeList{
		Watching: []Entry{{
			Media:     Media{ID: 1, MalID: 101},
			Status:    "CURRENT",
			Progress:  2,
			UpdatedAt: time.Date(2026, 5, 17, 9, 0, 0, 0, time.UTC),
		}},
	}
	myAnimeList := AnimeList{
		Completed: []Entry{{
			Media:     Media{ID: 1, MalID: 101},
			Status:    "COMPLETED",
			Progress:  12,
			UpdatedAt: time.Date(2026, 5, 17, 11, 0, 0, 0, time.UTC),
		}},
		Planning: []Entry{{
			Media:     Media{ID: 2, MalID: 202},
			Status:    "PLANNING",
			UpdatedAt: time.Date(2026, 5, 17, 8, 0, 0, 0, time.UTC),
		}},
	}

	plan := buildDualRemoteSyncPlan(aniList, myAnimeList)
	if len(plan.AniListUpdates) != 2 {
		t.Fatalf("expected AniList to receive 2 updates, got %d", len(plan.AniListUpdates))
	}
	if len(plan.MyAnimeListUpdates) != 0 {
		t.Fatalf("expected MyAnimeList to already match the winning entries, got %d updates", len(plan.MyAnimeListUpdates))
	}

	var mergedFirst *Entry
	for _, entry := range getEntriesByCategory(plan.Merged, "ALL") {
		if entry.Media.ID == 1 {
			mergedFirst = &entry
			break
		}
	}
	if mergedFirst == nil {
		t.Fatalf("expected merged list to contain media ID 1")
	}
	if mergedFirst.Status != "COMPLETED" || mergedFirst.Progress != 12 {
		t.Fatalf("expected merged entry to keep latest MAL state, got %+v", *mergedFirst)
	}
}
