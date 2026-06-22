package internal

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestClearLogFileCreatesParentAndTruncates(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "missing", "debug.log")

	if err := ClearLogFile(logPath); err != nil {
		t.Fatalf("ClearLogFile should create a missing parent directory: %v", err)
	}

	if err := os.WriteFile(logPath, []byte("old log data"), 0o644); err != nil {
		t.Fatalf("write log fixture: %v", err)
	}
	if err := ClearLogFile(logPath); err != nil {
		t.Fatalf("ClearLogFile should truncate an existing log: %v", err)
	}

	info, err := os.Stat(logPath)
	if err != nil {
		t.Fatalf("expected log file to exist: %v", err)
	}
	if info.Size() != 0 {
		t.Fatalf("expected log file to be truncated, got %d bytes", info.Size())
	}
}

func TestParseAnimeRowHandlesLegacyRows(t *testing.T) {
	withName := parseAnimeRow([]string{"123", "provider-id", "4", "55", "Legacy Title"})
	if withName == nil {
		t.Fatalf("expected legacy name row to parse")
	}
	if withName.Ep.Duration != 0 || withName.Title.Romaji != "Legacy Title" {
		t.Fatalf("unexpected legacy name row parse: %+v", withName)
	}

	withDuration := parseAnimeRow([]string{"123", "provider-id", "4", "55", "24"})
	if withDuration == nil {
		t.Fatalf("expected legacy duration row to parse")
	}
	if withDuration.Ep.Duration != 24 || withDuration.Title.Romaji != "123" {
		t.Fatalf("unexpected legacy duration row parse: %+v", withDuration)
	}

	current := parseAnimeRow([]string{"123", "provider-id", "4", "55", "24", "animepahe", "Current Title"})
	if current == nil {
		t.Fatalf("expected current row to parse")
	}
	if current.Ep.Duration != 24 || current.ProviderName != "animepahe" || current.Title.Romaji != "Current Title" {
		t.Fatalf("unexpected current row parse: %+v", current)
	}

	if invalid := parseAnimeRow([]string{"bad-id", "provider-id", "4", "55", "24", "allanime", "Broken Title"}); invalid != nil {
		t.Fatalf("expected invalid AniList ID row to be skipped, got %+v", invalid)
	}
}

func TestLocalDeleteAnimeNormalizesAnimepaheIDs(t *testing.T) {
	historyPath := filepath.Join(t.TempDir(), "history.csv")
	if err := LocalUpdateAnime(historyPath, 123, "123:session-id", 4, 0, 24, "Current Title", "animepahe"); err != nil {
		t.Fatalf("write animepahe history: %v", err)
	}

	LocalDeleteAnime(historyPath, 123, "123:other-session")

	if entries := LocalGetAllAnime(historyPath); len(entries) != 0 {
		t.Fatalf("expected animepahe history entry to be deleted, got %#v", entries)
	}
}

func TestGetAnimeNameWithoutGlobalConfig(t *testing.T) {
	previousConfig := GetGlobalConfig()
	SetGlobalConfig(nil)
	t.Cleanup(func() {
		SetGlobalConfig(previousConfig)
	})

	name := GetAnimeName(Anime{Title: AnimeTitle{English: "English Title", Romaji: "Romaji Title"}})
	if name != "English Title" {
		t.Fatalf("expected English fallback without config, got %q", name)
	}

	name = GetAnimeName(Anime{AnilistId: 42})
	if name != "42" {
		t.Fatalf("expected ID fallback for empty title, got %q", name)
	}
}

func TestNextEpisodeFromProgress(t *testing.T) {
	if got := nextEpisodeFromProgress(0); got != 1 {
		t.Fatalf("progress 0 should start episode 1, got %d", got)
	}
	if got := nextEpisodeFromProgress(7); got != 8 {
		t.Fatalf("progress 7 should start episode 8, got %d", got)
	}
	if got := nextEpisodeFromProgress(-3); got != 1 {
		t.Fatalf("negative progress should clamp to episode 1, got %d", got)
	}
}

func TestSelectionCtrlCSelectsQuit(t *testing.T) {
	model := &Model{
		filteredKeys: []SelectionOption{{Key: "anime", Label: "Some Anime"}},
		selected:     0,
		isHomeMenu:   false,
	}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	finalModel, ok := updated.(*Model)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	if len(finalModel.filteredKeys) != 1 || finalModel.filteredKeys[0].Key != "-1" {
		t.Fatalf("ctrl+c should select Quit, got %+v", finalModel.filteredKeys)
	}
}

func TestSelectionEnterWithNoMatchesDoesNotPanic(t *testing.T) {
	model := &Model{filteredKeys: nil, selected: 0}
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatalf("expected no quit command for empty selection")
	}
	if _, ok := updated.(*Model); !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
}

func TestVisibleItemsCountClampsTinyTerminals(t *testing.T) {
	if got := (Model{terminalHeight: 0}).visibleItemsCount(); got != 1 {
		t.Fatalf("expected at least one visible item, got %d", got)
	}
}

func TestHasMPVReferrerArgDetectsHeaderAppendForms(t *testing.T) {
	cases := [][]string{
		{"--http-header-fields-append=Referer: https://example.test/"},
		{"--http-header-fields-append", "Referer: https://example.test/"},
		{"--mpv-http-header-fields-append=Referer: https://example.test/"},
	}

	for _, args := range cases {
		if !hasMPVReferrerArg(args) {
			t.Fatalf("expected referrer detection for args %#v", args)
		}
	}
}

func TestEpisodeAndProgressInputValidation(t *testing.T) {
	if got, err := parsePositiveIntInput("12", "episode number"); err != nil || got != 12 {
		t.Fatalf("expected valid episode number, got %d err %v", got, err)
	}
	for _, input := range []string{"", "0", "-1", "one"} {
		if _, err := parsePositiveIntInput(input, "episode number"); err == nil {
			t.Fatalf("expected invalid episode input %q to fail", input)
		}
	}

	if got, err := parseNonNegativeIntInput("0", "progress"); err != nil || got != 0 {
		t.Fatalf("expected progress 0 to be valid, got %d err %v", got, err)
	}
	if _, err := parseNonNegativeIntInput("-1", "progress"); err == nil {
		t.Fatalf("expected negative progress to fail")
	}
}

func TestAffirmativeAnswerAcceptsYesVariants(t *testing.T) {
	for _, answer := range []string{"y", "Y", " yes "} {
		if !isAffirmativeAnswer(answer) {
			t.Fatalf("expected %q to be affirmative", answer)
		}
	}
	if isAffirmativeAnswer("no") {
		t.Fatalf("expected no to be negative")
	}
}

func TestPopulateConfigFallsBackToDefaultsOnInvalidValues(t *testing.T) {
	config := PopulateConfig(map[string]string{
		"PercentageToMarkComplete": "not-a-number",
		"SkipOp":                   "not-a-bool",
	})

	if config.PercentageToMarkComplete != 85 {
		t.Fatalf("expected default completion percentage, got %d", config.PercentageToMarkComplete)
	}
	if !config.SkipOp {
		t.Fatalf("expected invalid bool to fall back to default true")
	}
	if config.StoragePath == "" {
		t.Fatalf("expected absent fields to be populated from defaults")
	}
}

func TestParseStringArrayUsesJSONBeforeLegacySplit(t *testing.T) {
	args := parseStringArray(`["--script-opts=foo=a,b","--really-quiet"]`)
	if len(args) != 2 {
		t.Fatalf("expected 2 args, got %#v", args)
	}
	if args[0] != "--script-opts=foo=a,b" || args[1] != "--really-quiet" {
		t.Fatalf("unexpected args parse: %#v", args)
	}
}

func TestGetProviderNormalizesConfiguredProviderName(t *testing.T) {
	previousProvider := CurrentProvider
	previousConfig := GetGlobalConfig()
	t.Cleanup(func() {
		CurrentProvider = previousProvider
		SetGlobalConfig(previousConfig)
	})

	CurrentProvider = nil
	SetGlobalConfig(&CurdConfig{Provider: " AllAnime "})

	if got := GetProvider().Name(); got != "anineko" {
		t.Fatalf("expected anineko provider when allanime is disabled, got %q", got)
	}
}

func TestGetProviderFallsBackWhenConfiguredProviderDisabled(t *testing.T) {
	previousProvider := CurrentProvider
	previousConfig := GetGlobalConfig()
	t.Cleanup(func() {
		CurrentProvider = previousProvider
		SetGlobalConfig(previousConfig)
	})

	CurrentProvider = nil
	SetGlobalConfig(&CurdConfig{Provider: " AnimePahe "})

	if got := GetProvider().Name(); got != "anineko" {
		t.Fatalf("expected anineko fallback for disabled provider, got %q", got)
	}
}

func TestHTTPStatusErrorIncludesStatusAndBodySnippet(t *testing.T) {
	err := httpStatusError("provider request", 503, []byte("  service unavailable  "))
	if err == nil {
		t.Fatalf("expected status error")
	}
	if got := err.Error(); got != "provider request failed with status 503: service unavailable" {
		t.Fatalf("unexpected status error: %q", got)
	}
}

func TestMPVResponseDataReturnsCommandErrors(t *testing.T) {
	if _, err := mpvResponseData(map[string]interface{}{"error": "property unavailable"}); err == nil {
		t.Fatalf("expected MPV error field to be returned")
	}

	data, err := mpvResponseData(map[string]interface{}{"error": "success", "data": float64(12)})
	if err != nil {
		t.Fatalf("expected success response: %v", err)
	}
	if data != float64(12) {
		t.Fatalf("unexpected MPV data: %#v", data)
	}
}

func TestParseMyAnimeListDateRejectsInvalidValues(t *testing.T) {
	if got := parseMyAnimeListDate("2026-05-23"); got != (FuzzyDate{Year: 2026, Month: 5, Day: 23}) {
		t.Fatalf("unexpected parsed date: %+v", got)
	}
	for _, input := range []string{"bad", "2026-aa-23", "2026-13-01", "2026-01-00"} {
		if got := parseMyAnimeListDate(input); got != (FuzzyDate{}) {
			t.Fatalf("expected invalid date %q to return zero value, got %+v", input, got)
		}
	}
}

func TestParseAnimeListMalformedInputReturnsEmptyList(t *testing.T) {
	if got := ParseAnimeList(nil); len(getEntriesByCategory(got, "ALL")) != 0 {
		t.Fatalf("expected nil input to produce empty list")
	}
	if got := ParseAnimeList(map[string]interface{}{"data": nil}); len(getEntriesByCategory(got, "ALL")) != 0 {
		t.Fatalf("expected nil data input to produce empty list")
	}
}

func TestMediaDisplayTitleFallsBackToID(t *testing.T) {
	title := mediaDisplayTitle(Media{ID: 42}, &CurdConfig{AnimeNameLanguage: "english"})
	if title != "42" {
		t.Fatalf("expected ID fallback for empty titles, got %q", title)
	}
}

func TestMapEntriesByMediaIDSkipsZeroIDs(t *testing.T) {
	list := AnimeList{
		Watching: []Entry{
			{Media: Media{ID: 0}, Progress: 1},
			{Media: Media{ID: 10}, Progress: 2},
		},
	}

	entries := mapEntriesByMediaID(list)
	if len(entries) != 1 {
		t.Fatalf("expected only valid media IDs, got %#v", entries)
	}
	if entries[10].Progress != 2 {
		t.Fatalf("expected media 10 to be retained, got %#v", entries[10])
	}
}
