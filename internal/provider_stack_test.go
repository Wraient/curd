package internal

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wraient/curd/internal/providers"
)

type stackStubProvider struct {
	name           string
	searchResults  map[string][]SelectionOption
	episodeResults map[string]map[string][]string
	episodeErrors  map[string]map[string]error
	searchCalls    []string
	calls          []string
}

func (s *stackStubProvider) Name() string { return s.name }

func (s *stackStubProvider) SearchAnime(query, mode string) ([]SelectionOption, error) {
	mode = normalizeTranslationType(mode)
	s.searchCalls = append(s.searchCalls, s.name+":"+mode+":"+query)
	return s.searchResults[mode], nil
}

func (s *stackStubProvider) EpisodesList(showID, mode string) ([]string, error) {
	return nil, nil
}

func (s *stackStubProvider) GetEpisodeURL(config CurdConfig, id string, epNo int) ([]string, error) {
	return s.GetEpisodeURLForMode(config, id, epNo, config.SubOrDub)
}

func (s *stackStubProvider) GetEpisodeURLForMode(config CurdConfig, id string, epNo int, mode string) ([]string, error) {
	mode = normalizeTranslationType(mode)
	s.calls = append(s.calls, s.name+":"+mode)
	if byID, ok := s.episodeErrors[id]; ok {
		if err := byID[mode]; err != nil {
			return nil, err
		}
	}
	if byID, ok := s.episodeResults[id]; ok {
		return byID[mode], nil
	}
	return nil, nil
}

func withProviderFactories(t *testing.T, stubs ...*stackStubProvider) {
	t.Helper()
	withAllProvidersEnabledForTest(t)
	restores := make([]func(), 0, len(stubs))
	for _, stub := range stubs {
		stub := stub
		restores = append(restores, providers.SetFactoryForTest(stub.name, func() providers.Provider {
			return &stackStubProviderBridge{stub}
		}))
	}
	t.Cleanup(func() {
		for i := len(restores) - 1; i >= 0; i-- {
			restores[i]()
		}
	})
}

type stackStubProviderBridge struct {
	*stackStubProvider
}

func (s *stackStubProviderBridge) SearchAnime(query, mode string) ([]providers.SelectionOption, error) {
	options, err := s.stackStubProvider.SearchAnime(query, mode)
	return toProviderSelectionOptions(options), err
}

func (s *stackStubProviderBridge) EpisodesList(showID, mode string) ([]string, error) {
	return s.stackStubProvider.EpisodesList(showID, mode)
}

func (s *stackStubProviderBridge) GetEpisodeURL(config providers.PlaybackConfig, id string, epNo int) ([]string, error) {
	return s.stackStubProvider.GetEpisodeURL(CurdConfig{SubOrDub: config.SubOrDub}, id, epNo)
}

func (s *stackStubProviderBridge) GetEpisodeURLForMode(config providers.PlaybackConfig, id string, epNo int, mode string) ([]string, error) {
	return s.stackStubProvider.GetEpisodeURLForMode(CurdConfig{SubOrDub: config.SubOrDub}, id, epNo, mode)
}

func TestConfiguredProviderNamesAcceptsOrderedLists(t *testing.T) {
	withAllProvidersEnabledForTest(t)

	cases := []struct {
		name string
		cfg  *CurdConfig
		want []string
	}{
		{name: "empty", cfg: &CurdConfig{}, want: []string{"senshi", "anipub", "anineko", "allanime", "animepahe"}},
		{name: "json list", cfg: &CurdConfig{Provider: `["allanime","animepahe"]`}, want: []string{"allanime", "animepahe"}},
		{name: "comma list", cfg: &CurdConfig{Provider: "animepahe,allanime"}, want: []string{"animepahe", "allanime"}},
		{name: "plus list", cfg: &CurdConfig{Provider: "allanime+animepahe"}, want: []string{"allanime", "animepahe"}},
		{name: "legacy alias", cfg: &CurdConfig{Provider: "stacked"}, want: []string{"senshi", "anipub", "anineko", "allanime", "animepahe"}},
	}

	for _, tc := range cases {
		got := ConfiguredProviderNames(tc.cfg)
		if len(got) != len(tc.want) {
			t.Fatalf("%s: got %v, want %v", tc.name, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Fatalf("%s: got %v, want %v", tc.name, got, tc.want)
			}
		}
	}
}

func TestCanonicalProviderConfigValueMigratesLegacyValuesWithoutAddingAnimepahe(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{name: "legacy allanime", raw: "allanime", want: `["allanime"]`},
		{name: "legacy animepahe", raw: "animepahe", want: `["animepahe"]`},
		{name: "ordered stack", raw: "animepahe,allanime", want: `["animepahe","allanime"]`},
		{name: "animepahe opt out", raw: "allanime,no-animepahe", want: `["allanime","no-animepahe"]`},
		{name: "empty default", raw: "", want: "stacked"},
	}

	for _, tc := range cases {
		if got := canonicalProviderConfigValue(tc.raw); got != tc.want {
			t.Fatalf("%s: got %s, want %s", tc.name, got, tc.want)
		}
	}
}

func TestLoadConfigMigratesLegacyProviderToListWithoutAddingAnimepahe(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "curd.conf")
	if err := os.WriteFile(configPath, []byte("Provider=allanime\nAddMissingOptions=true\n"), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}
	if config.Provider != `["allanime"]` {
		t.Fatalf("got provider %s, want [\"allanime\"]", config.Provider)
	}

	contents, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(contents), `Provider=["allanime"]`) {
		t.Fatalf("config file was not migrated to allanime-only list:\n%s", string(contents))
	}
	if strings.Contains(string(contents), "animepahe") {
		t.Fatalf("migration added animepahe without consent:\n%s", string(contents))
	}
}

func TestSearchAnimeDecliningAnimepaheWritesOptOutAndDoesNotAskAgain(t *testing.T) {
	allanime := &stackStubProvider{
		name:          "allanime",
		searchResults: map[string][]SelectionOption{"sub": {}},
	}
	animepahe := &stackStubProvider{
		name: "animepahe",
		searchResults: map[string][]SelectionOption{
			"sub": {{Title: "Hidden", Key: "pahe-id", Label: "Hidden"}},
		},
	}
	withProviderFactories(t, allanime, animepahe)

	configPath := filepath.Join(t.TempDir(), "curd.conf")
	if err := os.WriteFile(configPath, []byte("Provider=[\"allanime\"]\n"), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	previousConfigPath := GlobalConfigPath
	GlobalConfigPath = configPath
	config := &CurdConfig{Provider: `["allanime"]`}
	previousConfig := GetGlobalConfig()
	SetGlobalConfig(config)
	t.Cleanup(func() {
		GlobalConfigPath = previousConfigPath
		SetGlobalConfig(previousConfig)
	})

	withPromptSelect(t, func(options []SelectionOption) (SelectionOption, error) {
		if len(options) < 2 || options[0].Key != "use" || options[1].Key != "never" {
			t.Fatalf("unexpected Animepahe fallback options: %#v", options)
		}
		return SelectionOption{Key: "never"}, nil
	})

	results, err := SearchAnime("missing", "sub")
	if err != nil {
		t.Fatalf("SearchAnime returned error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected no results after declining Animepahe, got %#v", results)
	}
	if config.Provider != `["allanime","no-animepahe"]` {
		t.Fatalf("provider config was not updated with opt out: %s", config.Provider)
	}
	contents, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(contents), `Provider=["allanime","no-animepahe"]`) {
		t.Fatalf("config file missing opt out:\n%s", string(contents))
	}

	withPromptSelect(t, func(options []SelectionOption) (SelectionOption, error) {
		t.Fatalf("Animepahe prompt should not be shown after no-animepahe opt out")
		return SelectionOption{}, nil
	})
	if _, err := SearchAnime("missing again", "sub"); err != nil {
		t.Fatalf("SearchAnime after opt out returned error: %v", err)
	}
}

func TestSearchAnimeAcceptingAnimepaheAddsItBeforeSearching(t *testing.T) {
	allanime := &stackStubProvider{
		name:          "allanime",
		searchResults: map[string][]SelectionOption{"sub": {}},
	}
	animepahe := &stackStubProvider{
		name: "animepahe",
		searchResults: map[string][]SelectionOption{
			"sub": {{Title: "Fallback", Key: "pahe-id", Label: "Fallback"}},
		},
	}
	withProviderFactories(t, allanime, animepahe)

	configPath := filepath.Join(t.TempDir(), "curd.conf")
	if err := os.WriteFile(configPath, []byte("Provider=[\"allanime\"]\n"), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	previousConfigPath := GlobalConfigPath
	GlobalConfigPath = configPath
	config := &CurdConfig{Provider: `["allanime"]`}
	previousConfig := GetGlobalConfig()
	SetGlobalConfig(config)
	t.Cleanup(func() {
		GlobalConfigPath = previousConfigPath
		SetGlobalConfig(previousConfig)
	})

	withPromptSelect(t, func(options []SelectionOption) (SelectionOption, error) {
		return SelectionOption{Key: "use"}, nil
	})

	results, err := SearchAnime("missing", "sub")
	if err != nil {
		t.Fatalf("SearchAnime returned error: %v", err)
	}
	if len(results) != 1 || results[0].Key != "animepahe::pahe-id" {
		t.Fatalf("expected Animepahe fallback result, got %#v", results)
	}
	if config.Provider != `["allanime","animepahe"]` {
		t.Fatalf("provider config was not updated with Animepahe: %s", config.Provider)
	}
	contents, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(contents), `Provider=["allanime","animepahe"]`) {
		t.Fatalf("config file missing Animepahe fallback:\n%s", string(contents))
	}
}

func TestSearchAnimeReturnsProviderQualifiedStackResultsInOrder(t *testing.T) {
	allanime := &stackStubProvider{
		name: "allanime",
		searchResults: map[string][]SelectionOption{
			"sub": {{Title: "First", Key: "allanime-id", Label: "First"}},
		},
	}
	animepahe := &stackStubProvider{
		name: "animepahe",
		searchResults: map[string][]SelectionOption{
			"sub": {{Title: "Second", Key: "pahe-id", Label: "Second"}},
		},
	}
	withProviderFactories(t, allanime, animepahe)

	previousConfig := GetGlobalConfig()
	SetGlobalConfig(&CurdConfig{Provider: `["allanime","animepahe"]`})
	t.Cleanup(func() { SetGlobalConfig(previousConfig) })

	results, err := SearchAnime("query", "sub")
	if err != nil {
		t.Fatalf("SearchAnime returned error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2: %#v", len(results), results)
	}
	if results[0].Key != "allanime::allanime-id" || results[1].Key != "animepahe::pahe-id" {
		t.Fatalf("results were not provider-qualified in config order: %#v", results)
	}
}

func TestResolveEpisodeURLUsesProviderListOrder(t *testing.T) {
	allanime := &stackStubProvider{
		name: "allanime",
		episodeResults: map[string]map[string][]string{
			"allanime-id": {"sub": {"allanime-sub"}},
		},
	}
	animepahe := &stackStubProvider{
		name: "animepahe",
		episodeResults: map[string]map[string][]string{
			"pahe-id": {"sub": {"animepahe-sub"}},
		},
		searchResults: map[string][]SelectionOption{
			"sub": {{Title: "Example", Key: "pahe-id"}},
		},
	}
	withProviderFactories(t, allanime, animepahe)

	cfg := CurdConfig{Provider: `["animepahe","allanime"]`, SubOrDub: "sub"}
	anime := &Anime{
		Title:        AnimeTitle{Romaji: "Example"},
		ProviderName: "allanime",
		ProviderId:   "allanime-id",
	}

	result, err := ResolveEpisodeURL(cfg, anime, 1)
	if err != nil {
		t.Fatalf("ResolveEpisodeURL returned error: %v", err)
	}
	if result.ProviderName != "animepahe" || result.ProviderID != "pahe-id" || result.Links[0] != "animepahe-sub" {
		t.Fatalf("expected animepahe to win by config order, got %#v", result)
	}
}

func TestResolveEpisodeURLForPlaybackTriesAllPreferredProvidersBeforeAudioFallback(t *testing.T) {
	allanime := &stackStubProvider{
		name: "allanime",
		searchResults: map[string][]SelectionOption{
			"dub": {{Title: "Example", Key: "shared-id"}},
			"sub": {{Title: "Example", Key: "shared-id"}},
		},
		episodeResults: map[string]map[string][]string{
			"shared-id": {"sub": {"allanime-sub"}},
		},
		episodeErrors: map[string]map[string]error{
			"shared-id": {"dub": errors.New("no allanime dub")},
		},
	}
	animepahe := &stackStubProvider{
		name: "animepahe",
		episodeResults: map[string]map[string][]string{
			"pahe-id": {"dub": {"animepahe-dub"}},
		},
		searchResults: map[string][]SelectionOption{
			"dub": {{Title: "Example", Key: "pahe-id"}},
		},
	}
	withProviderFactories(t, allanime, animepahe)
	withPromptSelect(t, func(options []SelectionOption) (SelectionOption, error) {
		t.Fatalf("audio fallback prompt should not be shown while another provider has preferred audio")
		return SelectionOption{}, nil
	})

	cfg := CurdConfig{Provider: `["allanime","animepahe"]`, SubOrDub: "dub"}
	anime := &Anime{
		Title:        AnimeTitle{Romaji: "Example"},
		ProviderName: "allanime",
		ProviderId:   "shared-id",
	}

	result, err := ResolveEpisodeURLForPlayback(cfg, anime, 1)
	if err != nil {
		t.Fatalf("ResolveEpisodeURLForPlayback returned error: %v", err)
	}
	if result.ProviderName != "animepahe" || result.Mode != "dub" || result.Links[0] != "animepahe-dub" {
		t.Fatalf("expected animepahe dub before sub fallback, got %#v", result)
	}
	for _, call := range allanime.calls {
		if call == "allanime:sub" {
			t.Fatalf("allanime sub fallback was tried before animepahe dub: %#v", allanime.calls)
		}
	}
}

func TestResolveEpisodeURLForPlaybackAcceptingAnimepaheUsesPreferredBeforeSubFallback(t *testing.T) {
	allanime := &stackStubProvider{
		name: "allanime",
		episodeResults: map[string]map[string][]string{
			"shared-id": {"sub": {"allanime-sub"}},
		},
		episodeErrors: map[string]map[string]error{
			"shared-id": {"dub": errors.New("no allanime dub")},
		},
	}
	animepahe := &stackStubProvider{
		name: "animepahe",
		episodeResults: map[string]map[string][]string{
			"pahe-id": {"dub": {"animepahe-dub"}},
		},
		searchResults: map[string][]SelectionOption{
			"dub": {{Title: "Example", Key: "pahe-id"}},
		},
	}
	withProviderFactories(t, allanime, animepahe)

	configPath := filepath.Join(t.TempDir(), "curd.conf")
	if err := os.WriteFile(configPath, []byte("Provider=[\"allanime\"]\n"), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	previousConfigPath := GlobalConfigPath
	GlobalConfigPath = configPath
	config := &CurdConfig{Provider: `["allanime"]`, SubOrDub: "dub"}
	previousConfig := GetGlobalConfig()
	SetGlobalConfig(config)
	t.Cleanup(func() {
		GlobalConfigPath = previousConfigPath
		SetGlobalConfig(previousConfig)
	})

	withPromptSelect(t, func(options []SelectionOption) (SelectionOption, error) {
		switch options[0].Key {
		case "use":
			return SelectionOption{Key: "use"}, nil
		case "play":
			t.Fatalf("sub fallback prompt should not be shown while Animepahe has dub")
		default:
			t.Fatalf("unexpected prompt options: %#v", options)
		}
		return SelectionOption{}, nil
	})

	anime := &Anime{
		Title:        AnimeTitle{Romaji: "Example"},
		ProviderName: "allanime",
		ProviderId:   "shared-id",
	}
	result, err := ResolveEpisodeURLForPlayback(*config, anime, 155)
	if err != nil {
		t.Fatalf("ResolveEpisodeURLForPlayback returned error: %v", err)
	}
	if result.ProviderName != "animepahe" || result.Mode != "dub" || result.Links[0] != "animepahe-dub" {
		t.Fatalf("expected Animepahe dub before sub fallback, got %#v", result)
	}
	if config.Provider != `["allanime","animepahe"]` {
		t.Fatalf("provider config was not updated with Animepahe: %s", config.Provider)
	}
	contents, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(contents), `Provider=["allanime","animepahe"]`) {
		t.Fatalf("config file missing Animepahe fallback:\n%s", string(contents))
	}
	if got := strings.Join(allanime.calls, ","); got != "allanime:dub" {
		t.Fatalf("expected one AllAnime dub attempt before Animepahe, got %s", got)
	}
	if got := strings.Join(animepahe.calls, ","); got != "animepahe:dub" {
		t.Fatalf("expected Animepahe dub before sub fallback, got %s", got)
	}
}

func TestResolveEpisodeURLForPlaybackDecliningAnimepaheFallsBackToAllanimeSub(t *testing.T) {
	allanime := &stackStubProvider{
		name: "allanime",
		episodeResults: map[string]map[string][]string{
			"shared-id": {"sub": {"allanime-sub"}},
		},
		episodeErrors: map[string]map[string]error{
			"shared-id": {"dub": errors.New("no allanime dub")},
		},
	}
	animepahe := &stackStubProvider{
		name: "animepahe",
		episodeResults: map[string]map[string][]string{
			"pahe-id": {"dub": {"animepahe-dub"}, "sub": {"animepahe-sub"}},
		},
		searchResults: map[string][]SelectionOption{
			"dub": {{Title: "Example", Key: "pahe-id"}},
			"sub": {{Title: "Example", Key: "pahe-id"}},
		},
	}
	withProviderFactories(t, allanime, animepahe)

	configPath := filepath.Join(t.TempDir(), "curd.conf")
	if err := os.WriteFile(configPath, []byte("Provider=[\"allanime\"]\n"), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	previousConfigPath := GlobalConfigPath
	GlobalConfigPath = configPath
	config := &CurdConfig{Provider: `["allanime"]`, SubOrDub: "dub"}
	previousConfig := GetGlobalConfig()
	SetGlobalConfig(config)
	t.Cleanup(func() {
		GlobalConfigPath = previousConfigPath
		SetGlobalConfig(previousConfig)
	})

	sawSubPrompt := false
	withPromptSelect(t, func(options []SelectionOption) (SelectionOption, error) {
		switch options[0].Key {
		case "use":
			return SelectionOption{Key: "never"}, nil
		case "play":
			sawSubPrompt = true
			return SelectionOption{Key: "play"}, nil
		default:
			t.Fatalf("unexpected prompt options: %#v", options)
		}
		return SelectionOption{}, nil
	})

	anime := &Anime{
		Title:        AnimeTitle{Romaji: "Example"},
		ProviderName: "allanime",
		ProviderId:   "shared-id",
	}
	result, err := ResolveEpisodeURLForPlayback(*config, anime, 155)
	if err != nil {
		t.Fatalf("ResolveEpisodeURLForPlayback returned error: %v", err)
	}
	if result.ProviderName != "allanime" || result.Mode != "sub" || result.Links[0] != "allanime-sub" {
		t.Fatalf("expected AllAnime sub fallback after declining Animepahe, got %#v", result)
	}
	if !sawSubPrompt {
		t.Fatalf("expected prompt before playing sub fallback")
	}
	if config.Provider != `["allanime","no-animepahe"]` {
		t.Fatalf("provider config was not updated with opt out: %s", config.Provider)
	}
	contents, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(contents), `Provider=["allanime","no-animepahe"]`) {
		t.Fatalf("config file missing opt out:\n%s", string(contents))
	}
	if len(animepahe.searchCalls) != 0 || len(animepahe.calls) != 0 {
		t.Fatalf("Animepahe should not be touched after opt out, search=%#v episode=%#v", animepahe.searchCalls, animepahe.calls)
	}
	if got := strings.Join(allanime.calls, ","); got != "allanime:dub,allanime:sub" {
		t.Fatalf("expected AllAnime dub then AllAnime sub after declining Animepahe, got %s", got)
	}
}

func TestResolveEpisodeURLForPlaybackNoAnimepaheSkipsConsentAndOffersAllanimeSub(t *testing.T) {
	allanime := &stackStubProvider{
		name: "allanime",
		searchResults: map[string][]SelectionOption{
			"dub": {{Title: "Example", Key: "shared-id"}},
			"sub": {{Title: "Example", Key: "shared-id"}},
		},
		episodeResults: map[string]map[string][]string{
			"shared-id": {"sub": {"allanime-sub"}},
		},
		episodeErrors: map[string]map[string]error{
			"shared-id": {"dub": errors.New("no allanime dub")},
		},
	}
	animepahe := &stackStubProvider{
		name: "animepahe",
		episodeResults: map[string]map[string][]string{
			"pahe-id": {"dub": {"animepahe-dub"}, "sub": {"animepahe-sub"}},
		},
		searchResults: map[string][]SelectionOption{
			"dub": {{Title: "Example", Key: "pahe-id"}},
			"sub": {{Title: "Example", Key: "pahe-id"}},
		},
	}
	withProviderFactories(t, allanime, animepahe)

	config := &CurdConfig{Provider: `["allanime","no-animepahe"]`, SubOrDub: "dub"}
	previousConfig := GetGlobalConfig()
	SetGlobalConfig(config)
	t.Cleanup(func() { SetGlobalConfig(previousConfig) })

	sawSubPrompt := false
	withPromptSelect(t, func(options []SelectionOption) (SelectionOption, error) {
		switch options[0].Key {
		case "use":
			t.Fatalf("Animepahe consent prompt should not be shown when no-animepahe is configured")
		case "play":
			sawSubPrompt = true
			return SelectionOption{Key: "play"}, nil
		default:
			t.Fatalf("unexpected prompt options: %#v", options)
		}
		return SelectionOption{}, nil
	})

	anime := &Anime{
		Title:        AnimeTitle{Romaji: "Example"},
		ProviderName: "animepahe",
		ProviderId:   "pahe-id",
	}
	result, err := ResolveEpisodeURLForPlayback(*config, anime, 155)
	if err != nil {
		t.Fatalf("ResolveEpisodeURLForPlayback returned error: %v", err)
	}
	if result.ProviderName != "allanime" || result.Mode != "sub" || result.Links[0] != "allanime-sub" {
		t.Fatalf("expected AllAnime sub fallback with no-animepahe, got %#v", result)
	}
	if !sawSubPrompt {
		t.Fatalf("expected prompt before playing sub fallback")
	}
	if len(animepahe.searchCalls) != 0 || len(animepahe.calls) != 0 {
		t.Fatalf("Animepahe should not be touched with no-animepahe, search=%#v episode=%#v", animepahe.searchCalls, animepahe.calls)
	}
	if got := strings.Join(allanime.calls, ","); got != "allanime:dub,allanime:sub" {
		t.Fatalf("expected AllAnime dub then AllAnime sub with no-animepahe, got %s", got)
	}
}
