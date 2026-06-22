package internal

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakePlaybackProvider struct {
	name              string
	linksByMode       map[string][]string
	errByMode         map[string]error
	episodeListByMode map[string][]string
	episodeErrByMode  map[string]error
	modes             []string
	episodeModes      []string
}

func (p *fakePlaybackProvider) Name() string {
	if p.name != "" {
		return p.name
	}
	return "fake"
}

func (p *fakePlaybackProvider) SearchAnime(query, mode string) ([]SelectionOption, error) {
	return nil, nil
}

func (p *fakePlaybackProvider) EpisodesList(showID, mode string) ([]string, error) {
	mode = normalizeTranslationType(mode)
	p.episodeModes = append(p.episodeModes, mode)
	if err := p.episodeErrByMode[mode]; err != nil {
		return nil, err
	}
	if episodes, ok := p.episodeListByMode[mode]; ok {
		return episodes, nil
	}
	return []string{"1"}, nil
}

func (p *fakePlaybackProvider) GetEpisodeURL(config CurdConfig, id string, epNo int) ([]string, error) {
	mode := normalizeTranslationType(config.SubOrDub)
	p.modes = append(p.modes, mode)
	if err := p.errByMode[mode]; err != nil {
		return nil, err
	}
	return p.linksByMode[mode], nil
}

func withPromptSelect(t *testing.T, fn func([]SelectionOption) (SelectionOption, error)) {
	t.Helper()
	previous := promptSelect
	promptSelect = fn
	t.Cleanup(func() {
		promptSelect = previous
	})
}

func withProvider(t *testing.T, provider Provider) {
	t.Helper()
	previous := CurrentProvider
	CurrentProvider = provider
	t.Cleanup(func() {
		CurrentProvider = previous
	})
}

func TestInferTotalEpisodesFromEpisodeListUsesHighestWholeEpisode(t *testing.T) {
	episodes := []string{"1", "12", "2", "12.5", "special", "0"}

	if got := inferTotalEpisodesFromEpisodeList(episodes); got != 12 {
		t.Fatalf("expected highest whole episode 12, got %d", got)
	}
}

func TestGetProviderTotalEpisodesUsesHighestEpisodeAcrossModes(t *testing.T) {
	provider := &fakePlaybackProvider{
		episodeListByMode: map[string][]string{
			"dub": {"1", "2", "12"},
			"sub": {"1", "99", "100", "100.5"},
		},
	}
	withProvider(t, provider)

	total, err := GetProviderTotalEpisodes("one-piece-id", "dub")
	if err != nil {
		t.Fatalf("expected provider total lookup to succeed: %v", err)
	}
	if total != 100 {
		t.Fatalf("expected highest provider episode across modes, got %d", total)
	}
	if got := strings.Join(provider.episodeModes, ","); got != "dub,sub" {
		t.Fatalf("expected preferred then alternate episode lookups, got %s", got)
	}
}

func TestGetProviderTotalEpisodesQueriesAnimepaheOnce(t *testing.T) {
	provider := &fakePlaybackProvider{
		name: "animepahe",
		episodeListByMode: map[string][]string{
			"sub": {"1", "500", "1100"},
		},
	}
	withProvider(t, provider)

	total, err := GetProviderTotalEpisodes("one-piece-id", "sub")
	if err != nil {
		t.Fatalf("expected animepahe total lookup to succeed: %v", err)
	}
	if total != 3 {
		t.Fatalf("expected animepahe provider total, got %d", total)
	}
	if got := strings.Join(provider.episodeModes, ","); got != "sub" {
		t.Fatalf("expected animepahe to be queried once, got %s", got)
	}
}

func TestGetProviderTotalEpisodesReturnsLookupErrorsWhenNoEpisodesFound(t *testing.T) {
	provider := &fakePlaybackProvider{
		episodeErrByMode: map[string]error{
			"sub": errors.New("no sub episodes"),
			"dub": errors.New("no dub episodes"),
		},
	}
	withProvider(t, provider)

	_, err := GetProviderTotalEpisodes("show-id", "sub")
	if err == nil {
		t.Fatalf("expected provider total lookup to fail")
	}
	if !strings.Contains(err.Error(), "no sub episodes") || !strings.Contains(err.Error(), "no dub episodes") {
		t.Fatalf("expected both lookup errors, got %v", err)
	}
}

func TestGetAllAnimeEpisodesListUsesValidGraphQLVariableSyntax(t *testing.T) {
	withAllProvidersEnabledForTest(t)
	previousClient := sharedHTTPClient
	t.Cleanup(func() {
		sharedHTTPClient = previousClient
	})

	sharedHTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		var payload struct {
			Query     string            `json:"query"`
			Variables map[string]string `json:"variables"`
		}
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			t.Fatalf("decode allanime episode request: %v", err)
		}
		if !strings.Contains(payload.Query, "query ($showId: String!)") {
			t.Fatalf("episode list query is missing valid variable syntax: %q", payload.Query)
		}
		if payload.Variables["showId"] != "show-id" {
			t.Fatalf("unexpected show id variable: %#v", payload.Variables)
		}
		return testHTTPResponse(req, http.StatusOK, `{"data":{"show":{"_id":"show-id","availableEpisodesDetail":{"sub":[1,2,12]}}}}`), nil
	})}

	episodes, err := func() ([]string, error) {
		provider, err := ProviderByName("allanime")
		if err != nil {
			return nil, err
		}
		return provider.EpisodesList("show-id", "sub")
	}()
	if err != nil {
		t.Fatalf("expected allanime episode list lookup to succeed: %v", err)
	}
	if got := strings.Join(episodes, ","); got != "1,2,12" {
		t.Fatalf("unexpected episodes: %s", got)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	originalStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stdout pipe: %v", err)
	}
	os.Stdout = writer

	fn()

	if err := writer.Close(); err != nil {
		t.Fatalf("close stdout writer: %v", err)
	}
	os.Stdout = originalStdout

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, reader); err != nil {
		t.Fatalf("read captured stdout: %v", err)
	}
	return buf.String()
}

func TestGetEpisodeURLForPlaybackUsesPreferredAudioOnlyWhenAvailable(t *testing.T) {
	provider := &fakePlaybackProvider{
		linksByMode: map[string][]string{"sub": {"sub-url"}, "dub": {"dub-url"}},
		errByMode:   map[string]error{},
	}
	withProvider(t, provider)

	links, mode, err := GetEpisodeURLForPlayback(CurdConfig{SubOrDub: "sub"}, "anime-id", 1)
	if err != nil {
		t.Fatalf("expected preferred audio to succeed: %v", err)
	}
	if mode != "sub" || len(links) != 1 || links[0] != "sub-url" {
		t.Fatalf("unexpected playback result: mode=%q links=%v", mode, links)
	}
	if got := strings.Join(provider.modes, ","); got != "sub" {
		t.Fatalf("expected only preferred mode lookup, got %s", got)
	}
}

func TestGetEpisodeURLForPlaybackRequiresExplicitFallbackApproval(t *testing.T) {
	provider := &fakePlaybackProvider{
		linksByMode: map[string][]string{"dub": {"dub-url"}},
		errByMode:   map[string]error{"sub": errors.New("no sub streams")},
	}
	withProvider(t, provider)
	withPromptSelect(t, func(options []SelectionOption) (SelectionOption, error) {
		if len(options) != 2 || options[0].Key != "play" {
			t.Fatalf("unexpected fallback options: %+v", options)
		}
		return SelectionOption{Key: "play"}, nil
	})

	links, mode, err := GetEpisodeURLForPlayback(CurdConfig{SubOrDub: "sub"}, "anime-id", 1)
	if err != nil {
		t.Fatalf("expected accepted fallback to succeed: %v", err)
	}
	if mode != "dub" || len(links) != 1 || links[0] != "dub-url" {
		t.Fatalf("unexpected fallback result: mode=%q links=%v", mode, links)
	}
	if got := strings.Join(provider.modes, ","); got != "sub,dub" {
		t.Fatalf("expected preferred then fallback lookup, got %s", got)
	}
}

func TestGetEpisodeURLForPlaybackCancelsAudioFallback(t *testing.T) {
	provider := &fakePlaybackProvider{
		linksByMode: map[string][]string{"dub": {"dub-url"}},
		errByMode:   map[string]error{"sub": errors.New("no sub streams")},
	}
	withProvider(t, provider)
	withPromptSelect(t, func(options []SelectionOption) (SelectionOption, error) {
		return SelectionOption{Key: "cancel"}, nil
	})

	links, mode, err := GetEpisodeURLForPlayback(CurdConfig{SubOrDub: "sub"}, "anime-id", 1)
	if err == nil {
		t.Fatalf("expected cancelled fallback to return preferred-mode error")
	}
	if mode != "sub" || len(links) != 0 {
		t.Fatalf("unexpected cancelled fallback result: mode=%q links=%v", mode, links)
	}
}

func TestShouldWriteRemoteTrackingHonorsSessionSkip(t *testing.T) {
	config := &CurdConfig{TrackingRemote: TrackingRemoteAniList}
	if !ShouldWriteRemoteTracking(config, &Anime{}) {
		t.Fatalf("expected normal remote tracking to write")
	}
	if ShouldWriteRemoteTracking(config, &Anime{SkipRemoteSync: true}) {
		t.Fatalf("expected session skip flag to suppress remote writes")
	}
	if ShouldWriteRemoteTracking(&CurdConfig{TrackingRemote: TrackingRemoteNone}, &Anime{}) {
		t.Fatalf("expected local-only mode to suppress remote writes")
	}
}

func TestMaybeImportAniListToMyAnimeListDismissesWithoutMarkingImported(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "curd.conf")
	if err := createDefaultConfig(configPath); err != nil {
		t.Fatalf("create config: %v", err)
	}

	previousConfigPath := GlobalConfigPath
	GlobalConfigPath = configPath
	t.Cleanup(func() {
		GlobalConfigPath = previousConfigPath
	})

	config := &CurdConfig{
		StoragePath:         tempDir,
		TrackingRemote:      TrackingRemoteMyAnimeList,
		TrackingConfigured:  true,
		MyAnimeListImported: false,
	}
	if err := os.WriteFile(filepath.Join(tempDir, "anilist_token.json"), []byte("legacy-token"), 0600); err != nil {
		t.Fatalf("write token: %v", err)
	}

	withPromptSelect(t, func(options []SelectionOption) (SelectionOption, error) {
		return SelectionOption{Key: "no"}, nil
	})

	if err := maybeImportAniListToMyAnimeList(config, &User{}); err != nil {
		t.Fatalf("maybe import: %v", err)
	}
	if config.MyAnimeListImported {
		t.Fatalf("declining import must not mark import completed")
	}
	if !config.MyAnimeListImportDismissed {
		t.Fatalf("declining import should mark the prompt dismissed")
	}
}

func TestWriteTrackingBackupCreatesSnapshot(t *testing.T) {
	config := &CurdConfig{StoragePath: t.TempDir()}
	aniList := AnimeList{Watching: []Entry{{Media: Media{ID: 1, MalID: 11}, Status: "CURRENT"}}}
	myAnimeList := AnimeList{Planning: []Entry{{Media: Media{ID: 2, MalID: 22}, Status: "PLANNING"}}}

	backupPath, err := writeTrackingBackup(config, "test action", aniList, myAnimeList)
	if err != nil {
		t.Fatalf("write backup: %v", err)
	}
	data, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if !strings.Contains(string(data), `"action": "test action"`) || !strings.Contains(string(data), `"id": 1`) || !strings.Contains(string(data), `"id": 2`) {
		t.Fatalf("backup did not contain expected snapshot data: %s", string(data))
	}
}

func TestConfirmProviderMatchRequiresUserApproval(t *testing.T) {
	withPromptSelect(t, func(options []SelectionOption) (SelectionOption, error) {
		if options[0].Key != "use" || options[1].Key != "manual" {
			t.Fatalf("unexpected provider confirmation options: %+v", options)
		}
		return SelectionOption{Key: "manual"}, nil
	})

	if confirmProviderMatch(SelectionOption{Label: "Example"}, "title") {
		t.Fatalf("manual selection should reject guessed provider match")
	}
}

func TestSelectSequelAndUnreleasedSequelActions(t *testing.T) {
	sequels := []SequelInfo{
		{ID: 1, Title: AnimeTitle{Romaji: "First"}, Status: "FINISHED"},
		{ID: 2, Title: AnimeTitle{Romaji: "Second"}, Status: "NOT_YET_RELEASED"},
	}
	withPromptSelect(t, func(options []SelectionOption) (SelectionOption, error) {
		return SelectionOption{Key: "1"}, nil
	})

	selected, ok := selectSequel(&CurdConfig{}, sequels)
	if !ok || selected.ID != 2 {
		t.Fatalf("expected second sequel selection, got ok=%v selected=%+v", ok, selected)
	}

	var captured []SelectionOption
	withPromptSelect(t, func(options []SelectionOption) (SelectionOption, error) {
		captured = append([]SelectionOption(nil), options...)
		return SelectionOption{Key: "skip"}, nil
	})
	summary := promptSequelAction(&CurdConfig{}, selected, "token")
	if summary != "sequel skipped" {
		t.Fatalf("expected skipped summary, got %q", summary)
	}
	for _, option := range captured {
		if option.Key == "watching" {
			t.Fatalf("not-yet-released sequel should not offer Watching action: %+v", captured)
		}
	}
}

func TestHandleLastEpisodeCompletionSummarizesSkippedTrackerWrites(t *testing.T) {
	previousConfig := GetGlobalConfig()
	config := &CurdConfig{
		RofiSelection:     false,
		ScoreOnCompletion: true,
		TrackingRemote:    TrackingRemoteAniList,
	}
	SetGlobalConfig(config)
	t.Cleanup(func() {
		SetGlobalConfig(previousConfig)
	})

	output := captureStdout(t, func() {
		HandleLastEpisodeCompletion(config, &Anime{
			AnilistId:      1,
			TotalEpisodes:  12,
			SkipRemoteSync: true,
			Ep: Episode{
				Number: 12,
			},
		}, "token")
	})

	if !strings.Contains(output, "Completion summary:") ||
		!strings.Contains(output, "tracker updates skipped") ||
		!strings.Contains(output, "sequel check skipped") {
		t.Fatalf("completion summary did not report skipped tracker writes: %q", output)
	}
}
