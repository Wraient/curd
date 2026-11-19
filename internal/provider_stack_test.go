package internal

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wraient/curd/internal/providers"
)

type stackStubProvider struct {
	name             string
	searchResults    map[string][]SelectionOption
	episodeResults   map[string]map[string][]string
	episodeErrors    map[string]map[string]error
	episodeLists     map[string][]string
	episodeListErr   error
	searchCalls      []string
	calls            []string
	episodeListCalls []string
	// linkRotator, when set, returns the links for the n-th stream
	// resolution instead of the static episodeResults map.
	linkRotator func(call int) []string
	// resolveDelay, when positive, stalls stream resolution to simulate a
	// slow provider in concurrency tests.
	resolveDelay time.Duration
	// mu guards the call logs: the stacked resolve fan-out drives one
	// provider per goroutine, so appends race without it.
	mu sync.Mutex
}

// callsSnapshot returns the recorded stream-resolution calls. Use it
// instead of reading calls directly whenever another goroutine may still
// be resolving.
func (s *stackStubProvider) callsSnapshot() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.calls...)
}

// searchCallsSnapshot is the same guard for the search log.
func (s *stackStubProvider) searchCallsSnapshot() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.searchCalls...)
}

// episodeListCallsSnapshot is the same guard for the episode-list log.
func (s *stackStubProvider) episodeListCallsSnapshot() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.episodeListCalls...)
}

// waitForStubCalls polls until the stub recorded at least n calls or the
// timeout lapses, returning whatever was recorded. Resolved losers can
// still be in flight when the winner returns, so exact-count assertions
// on shared stubs must wait instead of reading once.
func waitForStubCalls(stub *stackStubProvider, n int) []string {
	deadline := time.Now().Add(5 * time.Second)
	for {
		if got := stub.callsSnapshot(); len(got) >= n {
			return got
		}
		if time.Now().After(deadline) {
			return stub.callsSnapshot()
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (s *stackStubProvider) Name() string { return s.name }

func (s *stackStubProvider) SearchAnime(query, mode string) ([]SelectionOption, error) {
	mode = normalizeTranslationType(mode)
	s.mu.Lock()
	s.searchCalls = append(s.searchCalls, s.name+":"+mode+":"+query)
	s.mu.Unlock()
	return s.searchResults[mode], nil
}

func (s *stackStubProvider) EpisodesList(showID, mode string) ([]string, error) {
	mode = normalizeTranslationType(mode)
	s.mu.Lock()
	s.episodeListCalls = append(s.episodeListCalls, s.name+":"+mode+":"+showID)
	s.mu.Unlock()
	if s.episodeListErr != nil {
		return nil, s.episodeListErr
	}
	return s.episodeLists[mode], nil
}

func (s *stackStubProvider) GetEpisodeURL(config CurdConfig, id string, epNo int) ([]string, error) {
	return s.GetEpisodeURLForMode(config, id, epNo, config.SubOrDub)
}

func (s *stackStubProvider) GetEpisodeURLForMode(config CurdConfig, id string, epNo int, mode string) ([]string, error) {
	mode = normalizeTranslationType(mode)
	s.mu.Lock()
	s.calls = append(s.calls, s.name+":"+mode)
	call := len(s.calls)
	s.mu.Unlock()
	if s.resolveDelay > 0 {
		time.Sleep(s.resolveDelay)
	}
	if s.linkRotator != nil {
		return s.linkRotator(call), nil
	}
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

func TestProviderNoPrefetchMetadata(t *testing.T) {
	if !providerNoPrefetch("anikoto") {
		t.Fatal("expected Anikoto to disable next-episode prefetch")
	}
	// anipm (settlar) and aniwaves (vhscdn) hand out short-lived signed
	// URLs: prefetched next-episode links die before the current episode
	// ends, so they must resolve at play time instead.
	for _, name := range []string{"anipm", "aniwaves"} {
		if !providerNoPrefetch(name) {
			t.Fatalf("expected %s to disable next-episode prefetch", name)
		}
	}
	if providerNoPrefetch("senshi") {
		t.Fatal("expected Senshi to keep next-episode prefetch")
	}
}

func TestPrefetchNextUntrackedEpisodeSkipsNoPrefetchProvider(t *testing.T) {
	provider := &stackStubProvider{name: "anikoto"}
	withProviderFactories(t, provider)
	anime := &Anime{
		ProviderId:   "show-id",
		ProviderName: "anikoto",
		Ep:           Episode{Number: 1},
	}

	prefetchNextUntrackedEpisode(&CurdConfig{}, anime, *anime)

	if got := provider.callsSnapshot(); len(got) != 0 {
		t.Fatalf("no-prefetch provider was called: %#v", got)
	}
	if anime.Ep.NextEpisode.Number != 0 || len(anime.Ep.NextEpisode.Links) != 0 {
		t.Fatalf("unexpected prefetched episode: %#v", anime.Ep.NextEpisode)
	}
}

func TestPrefetchNextUntrackedEpisodeDiscardsNoPrefetchFallback(t *testing.T) {
	senshi := &stackStubProvider{
		name: "senshi",
		episodeErrors: map[string]map[string]error{
			"show-id": {"sub": errors.New("provider unavailable")},
		},
	}
	anikoto := &stackStubProvider{
		name: "anikoto",
		searchResults: map[string][]SelectionOption{
			"sub": {{Key: "show-id", Title: "Example"}},
		},
		episodeResults: map[string]map[string][]string{
			"show-id": {"sub": {"https://cdn.example/short-lived.m3u8"}},
		},
	}
	withProviderFactories(t, senshi, anikoto)
	config := &CurdConfig{Provider: `["senshi","anikoto"]`}
	anime := &Anime{
		Title:        AnimeTitle{Romaji: "Example"},
		ProviderId:   "show-id",
		ProviderName: "senshi",
		Ep:           Episode{Number: 1},
	}

	prefetchNextUntrackedEpisode(config, anime, *anime)

	senshiCalls, anikotoCalls := waitForStubCalls(senshi, 1), waitForStubCalls(anikoto, 1)
	if len(senshiCalls) == 0 || len(anikotoCalls) == 0 {
		t.Fatalf("expected stack fallback during prefetch: senshi=%#v anikoto=%#v", senshiCalls, anikotoCalls)
	}
	if anime.Ep.NextEpisode.Number != 0 || len(anime.Ep.NextEpisode.Links) != 0 {
		t.Fatalf("no-prefetch fallback was cached: %#v", anime.Ep.NextEpisode)
	}
}

func TestConfiguredProviderNamesAcceptsOrderedLists(t *testing.T) {
	withAllProvidersEnabledForTest(t)

	cases := []struct {
		name string
		cfg  *CurdConfig
		want []string
	}{
		{name: "empty", cfg: &CurdConfig{}, want: []string{"anipm", "anikoto", "anilight", "animex", "aniwaves", "anipub", "senshi", "anineko", "allanime", "animepahe"}},
		{name: "json list", cfg: &CurdConfig{Provider: `["allanime","animepahe"]`}, want: []string{"allanime", "animepahe"}},
		{name: "comma list", cfg: &CurdConfig{Provider: "animepahe,allanime"}, want: []string{"animepahe", "allanime"}},
		{name: "plus list", cfg: &CurdConfig{Provider: "allanime+animepahe"}, want: []string{"allanime", "animepahe"}},
		{name: "legacy alias", cfg: &CurdConfig{Provider: "stacked"}, want: []string{"anipm", "anikoto", "anilight", "animex", "aniwaves", "anipub", "senshi", "anineko", "allanime", "animepahe"}},
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

func TestFilterExcludedProviders(t *testing.T) {
	filtered := filterExcludedProviders([]string{"senshi", "allanime", "anipub"}, []string{"senshi", "SENSHI"})
	if len(filtered) != 2 || filtered[0] != "allanime" || filtered[1] != "anipub" {
		t.Fatalf("unexpected filtered providers: %#v", filtered)
	}
}

func TestResolveEpisodeURLExcludingProviders(t *testing.T) {
	allanime := &stackStubProvider{
		name: "allanime",
		episodeResults: map[string]map[string][]string{
			"allanime-id": {"sub": {"allanime-sub"}},
		},
		searchResults: map[string][]SelectionOption{
			"sub": {{Title: "Example", Key: "allanime-id"}},
		},
	}
	senshi := &stackStubProvider{
		name: "senshi",
		episodeResults: map[string]map[string][]string{
			"senshi-id": {"sub": {"senshi-sub"}},
		},
		searchResults: map[string][]SelectionOption{
			"sub": {{Title: "Example", Key: "senshi-id"}},
		},
	}
	withProviderFactories(t, allanime, senshi)

	cfg := CurdConfig{Provider: `["senshi","allanime"]`, SubOrDub: "sub"}
	anime := &Anime{
		Title:        AnimeTitle{Romaji: "Example"},
		ProviderName: "senshi",
		ProviderId:   "senshi-id",
	}

	result, err := ResolveEpisodeURLExcludingProviders(cfg, anime, 1, []string{"senshi"})
	if err != nil {
		t.Fatalf("ResolveEpisodeURLExcludingProviders returned error: %v", err)
	}
	if result.ProviderName != "allanime" || result.Links[0] != "allanime-sub" {
		t.Fatalf("expected allanime fallback, got %#v", result)
	}
}

func TestResolveEpisodeURLExcludingProvidersModeUsesExplicitMode(t *testing.T) {
	allanime := &stackStubProvider{
		name: "allanime",
		episodeResults: map[string]map[string][]string{
			"allanime-id": {"dub": {"allanime-dub"}, "sub": {"allanime-sub"}},
		},
		searchResults: map[string][]SelectionOption{
			"dub": {{Title: "Example", Key: "allanime-id"}},
			"sub": {{Title: "Example", Key: "allanime-id"}},
		},
	}
	withProviderFactories(t, allanime)

	cfg := CurdConfig{Provider: `["allanime"]`, SubOrDub: "sub"}
	anime := &Anime{
		Title:        AnimeTitle{Romaji: "Example"},
		ProviderName: "allanime",
		ProviderId:   "allanime-id",
	}

	result, err := ResolveEpisodeURLExcludingProvidersMode(cfg, anime, 1, nil, "dub")
	if err != nil {
		t.Fatalf("ResolveEpisodeURLExcludingProvidersMode returned error: %v", err)
	}
	if result.Mode != "dub" || result.Links[0] != "allanime-dub" {
		t.Fatalf("expected explicit dub mode, got %#v", result)
	}
}

func TestResolveEpisodeURLAlternateModeWithPromptRequiresApproval(t *testing.T) {
	allanime := &stackStubProvider{
		name: "allanime",
		episodeResults: map[string]map[string][]string{
			"allanime-id": {"dub": {"allanime-dub"}},
		},
		searchResults: map[string][]SelectionOption{
			"dub": {{Title: "Example", Key: "allanime-id"}},
		},
	}
	withProviderFactories(t, allanime)

	cfg := CurdConfig{Provider: `["allanime"]`, SubOrDub: "sub"}
	anime := &Anime{
		Title:        AnimeTitle{Romaji: "Example"},
		ProviderName: "allanime",
		ProviderId:   "allanime-id",
	}

	withPromptSelect(t, func(options []SelectionOption) (SelectionOption, error) {
		return SelectionOption{Key: "cancel"}, nil
	})
	if _, err := ResolveEpisodeURLAlternateModeWithPrompt(cfg, anime, 1, nil); err == nil {
		t.Fatal("expected declined alternate mode to error")
	}
	if anime.ProviderId != "allanime-id" {
		t.Fatalf("declined prompt should not change provider mapping, got %q", anime.ProviderId)
	}

	withPromptSelect(t, func(options []SelectionOption) (SelectionOption, error) {
		return SelectionOption{Key: "play"}, nil
	})
	result, err := ResolveEpisodeURLAlternateModeWithPrompt(cfg, anime, 1, nil)
	if err != nil {
		t.Fatalf("accepted alternate mode returned error: %v", err)
	}
	if result.Mode != "dub" || result.Links[0] != "allanime-dub" {
		t.Fatalf("expected dub fallback, got %#v", result)
	}
}

// Signed stream URLs expire within minutes, so the dub links probed to
// decide whether to offer the fallback must not be the ones handed to mpv:
// accepting the prompt has to resolve fresh links.
func TestResolveEpisodeURLAlternateModeWithPromptRefreshesLinksOnAccept(t *testing.T) {
	stub := &stackStubProvider{
		name: "allanime",
		linkRotator: func(call int) []string {
			return []string{fmt.Sprintf("allanime-dub-%d", call)}
		},
	}
	withProviderFactories(t, stub)

	cfg := CurdConfig{Provider: `["allanime"]`, SubOrDub: "sub"}
	anime := &Anime{
		Title:        AnimeTitle{Romaji: "Example"},
		ProviderName: "allanime",
		ProviderId:   "allanime-id",
	}

	withPromptSelect(t, func(options []SelectionOption) (SelectionOption, error) {
		return SelectionOption{Key: "play"}, nil
	})
	previousMaxAge := fallbackRefetchMaxAge
	fallbackRefetchMaxAge = 0
	t.Cleanup(func() { fallbackRefetchMaxAge = previousMaxAge })
	result, err := ResolveEpisodeURLAlternateModeWithPrompt(cfg, anime, 1, nil)
	if err != nil {
		t.Fatalf("accepted alternate mode returned error: %v", err)
	}
	if got := stub.callsSnapshot(); len(got) != 2 {
		t.Fatalf("expected probe + fresh resolution, got calls %#v", got)
	}
	if len(result.Links) != 1 || result.Links[0] != "allanime-dub-2" {
		t.Fatalf("expected freshly resolved links, got %#v", result.Links)
	}
}

// Fresh probe links must be handed to mpv as-is: re-resolving seconds-old
// signed URLs only adds latency without freshness gain.
func TestResolveEpisodeURLAlternateModeWithPromptSkipsRefreshWhenFresh(t *testing.T) {
	stub := &stackStubProvider{
		name: "allanime",
		linkRotator: func(call int) []string {
			return []string{fmt.Sprintf("allanime-dub-%d", call)}
		},
	}
	withProviderFactories(t, stub)

	cfg := CurdConfig{Provider: `["allanime"]`, SubOrDub: "sub"}
	anime := &Anime{
		Title:        AnimeTitle{Romaji: "Example"},
		ProviderName: "allanime",
		ProviderId:   "allanime-id",
	}

	withPromptSelect(t, func(options []SelectionOption) (SelectionOption, error) {
		return SelectionOption{Key: "play"}, nil
	})
	previousMaxAge := fallbackRefetchMaxAge
	fallbackRefetchMaxAge = time.Hour
	t.Cleanup(func() { fallbackRefetchMaxAge = previousMaxAge })
	result, err := ResolveEpisodeURLAlternateModeWithPrompt(cfg, anime, 1, nil)
	if err != nil {
		t.Fatalf("accepted alternate mode returned error: %v", err)
	}
	if got := stub.callsSnapshot(); len(got) != 1 {
		t.Fatalf("expected probe only, no re-resolution, got calls %#v", got)
	}
	if len(result.Links) != 1 || result.Links[0] != "allanime-dub-1" {
		t.Fatalf("expected probed links, got %#v", result.Links)
	}
}

func TestEpisodeModeResultPicksFirstStackOrderConcurrently(t *testing.T) {
	slow := &stackStubProvider{
		name:         "allanime",
		resolveDelay: 300 * time.Millisecond,
		searchResults: map[string][]SelectionOption{
			"dub": {{Title: "Example", Key: "show-id"}},
		},
		episodeResults: map[string]map[string][]string{
			"show-id": {"dub": {"slow-dub"}},
		},
	}
	fast := &stackStubProvider{
		name: "animepahe",
		searchResults: map[string][]SelectionOption{
			"dub": {{Title: "Example", Key: "show-id"}},
		},
		episodeResults: map[string]map[string][]string{
			"show-id": {"dub": {"fast-dub"}},
		},
	}
	withProviderFactories(t, slow, fast)

	cfg := CurdConfig{Provider: `["allanime","animepahe"]`, SubOrDub: "dub"}
	anime := &Anime{Title: AnimeTitle{Romaji: "Example"}}

	// The fast provider provably finishes first, yet the slow first-in-stack
	// provider must still win; fast being attempted at all proves the race.
	start := time.Now()
	result, err := episodeModeResultWithProviders(cfg, anime, 1, "dub", []string{"allanime", "animepahe"})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("concurrent resolve: %v", err)
	}
	if result.ProviderName != "allanime" || len(result.Links) != 1 || result.Links[0] != "slow-dub" {
		t.Fatalf("expected first-stack-order winner, got %#v", result)
	}
	if elapsed < 250*time.Millisecond {
		t.Fatalf("expected the winner to be awaited, returned in %v", elapsed)
	}
	// The loser may still be resolving when the winner returns; wait for
	// its attempt to land (bounded: the stub has no I/O, only the delay).
	fastAttempted := false
	for i := 0; i < 100 && !fastAttempted; i++ {
		if len(fast.callsSnapshot()) > 0 {
			fastAttempted = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !fastAttempted {
		t.Fatal("expected the fast provider to be attempted concurrently")
	}
	if result.ResolvedAt.IsZero() {
		t.Fatal("expected ResolvedAt to be stamped")
	}
	if anime.ProviderName != "allanime" {
		t.Fatalf("expected live mapping to follow the winner, got %q", anime.ProviderName)
	}
}

func TestEpisodeModeResultTimeoutFallsBackToCompletedSuccess(t *testing.T) {
	previousTimeout := providerResolveTimeout
	providerResolveTimeout = 500 * time.Millisecond
	t.Cleanup(func() { providerResolveTimeout = previousTimeout })

	slow := &stackStubProvider{
		name:         "allanime",
		resolveDelay: 2 * time.Second,
		searchResults: map[string][]SelectionOption{
			"dub": {{Title: "Example", Key: "show-id"}},
		},
		episodeResults: map[string]map[string][]string{
			"show-id": {"dub": {"slow-dub"}},
		},
	}
	fast := &stackStubProvider{
		name: "animepahe",
		searchResults: map[string][]SelectionOption{
			"dub": {{Title: "Example", Key: "show-id"}},
		},
		episodeResults: map[string]map[string][]string{
			"show-id": {"dub": {"fast-dub"}},
		},
	}
	withProviderFactories(t, slow, fast)

	cfg := CurdConfig{Provider: `["allanime","animepahe"]`, SubOrDub: "dub"}
	anime := &Anime{Title: AnimeTitle{Romaji: "Example"}}

	// No wall-clock assertion: under -race and full-suite load, scheduling
	// delays are unbounded. Correctness is fully pinned by the winner: the
	// sequential walk (and any no-timeout variant) returns the slow head
	// provider after ~2s, so a fast win proves the deadline cut in.
	result, err := episodeModeResultWithProviders(cfg, anime, 1, "dub", []string{"allanime", "animepahe"})
	if err != nil {
		t.Fatalf("timeout should fall back to the completed success: %v", err)
	}
	if result.ProviderName != "animepahe" || len(result.Links) != 1 || result.Links[0] != "fast-dub" {
		t.Fatalf("expected completed fallback winner, got %#v (slow calls=%v fast calls=%v)", result, slow.callsSnapshot(), fast.callsSnapshot())
	}
	if anime.ProviderName != "animepahe" {
		t.Fatalf("expected live mapping to follow the winner, got %q", anime.ProviderName)
	}
}

func TestEpisodeModeResultTimeoutReportsPendingProviders(t *testing.T) {
	previousTimeout := providerResolveTimeout
	providerResolveTimeout = 200 * time.Millisecond
	t.Cleanup(func() { providerResolveTimeout = previousTimeout })

	slow := &stackStubProvider{
		name:         "allanime",
		resolveDelay: 2 * time.Second,
		searchResults: map[string][]SelectionOption{
			"dub": {{Title: "Example", Key: "show-id"}},
		},
		episodeResults: map[string]map[string][]string{
			"show-id": {"dub": {"slow-dub"}},
		},
	}
	withProviderFactories(t, slow)

	cfg := CurdConfig{Provider: `["allanime"]`, SubOrDub: "dub"}
	anime := &Anime{Title: AnimeTitle{Romaji: "Example"}}

	_, err := episodeModeResultWithProviders(cfg, anime, 1, "dub", []string{"allanime"})
	if err == nil || !strings.Contains(err.Error(), "timed out waiting for allanime") {
		t.Fatalf("expected timeout naming the hung provider, got %v", err)
	}
}

func TestEpisodeModeResultSkipsFailedProviderConcurrently(t *testing.T) {
	broken := &stackStubProvider{
		name: "allanime",
		searchResults: map[string][]SelectionOption{
			"dub": {{Title: "Example", Key: "show-id"}},
		},
		episodeErrors: map[string]map[string]error{
			"show-id": {"dub": errors.New("no dub here")},
		},
	}
	good := &stackStubProvider{
		name: "animepahe",
		searchResults: map[string][]SelectionOption{
			"dub": {{Title: "Example", Key: "show-id"}},
		},
		episodeResults: map[string]map[string][]string{
			"show-id": {"dub": {"good-dub"}},
		},
	}
	withProviderFactories(t, broken, good)

	cfg := CurdConfig{Provider: `["allanime","animepahe"]`, SubOrDub: "dub"}
	anime := &Anime{Title: AnimeTitle{Romaji: "Example"}}

	result, err := episodeModeResultWithProviders(cfg, anime, 1, "dub", []string{"allanime", "animepahe"})
	if err != nil {
		t.Fatalf("concurrent resolve: %v", err)
	}
	if result.ProviderName != "animepahe" || len(result.Links) != 1 || result.Links[0] != "good-dub" {
		t.Fatalf("expected fallback winner, got %#v", result)
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
	for _, call := range allanime.callsSnapshot() {
		if call == "allanime:sub" {
			t.Fatalf("allanime sub fallback was tried before animepahe dub: %#v", call)
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
	if got := strings.Join(allanime.callsSnapshot(), ","); got != "allanime:dub" {
		t.Fatalf("expected one AllAnime dub attempt before Animepahe, got %s", got)
	}
	if got := strings.Join(animepahe.callsSnapshot(), ","); got != "animepahe:dub" {
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
	// Force the stale-link refresh so the re-resolution path is exercised
	// deterministically regardless of wall-clock timing.
	previousMaxAge := fallbackRefetchMaxAge
	fallbackRefetchMaxAge = 0
	t.Cleanup(func() { fallbackRefetchMaxAge = previousMaxAge })
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
	animepaheSearch, animepaheCalls := animepahe.searchCallsSnapshot(), animepahe.callsSnapshot()
	if len(animepaheSearch) != 0 || len(animepaheCalls) != 0 {
		t.Fatalf("Animepahe should not be touched after opt out, search=%#v episode=%#v", animepaheSearch, animepaheCalls)
	}
	// Preferred lookup, fallback probe, then a fresh fallback resolution
	// on accept: signed URLs may expire while the prompt is on screen.
	if got := strings.Join(waitForStubCalls(allanime, 3), ","); got != "allanime:dub,allanime:sub,allanime:sub" {
		t.Fatalf("expected AllAnime dub, probe, then fresh sub after declining Animepahe, got %s", got)
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
	// Force the stale-link refresh so the re-resolution path is exercised
	// deterministically regardless of wall-clock timing.
	previousMaxAge := fallbackRefetchMaxAge
	fallbackRefetchMaxAge = 0
	t.Cleanup(func() { fallbackRefetchMaxAge = previousMaxAge })
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
	animepaheSearch, animepaheCalls := animepahe.searchCallsSnapshot(), animepahe.callsSnapshot()
	if len(animepaheSearch) != 0 || len(animepaheCalls) != 0 {
		t.Fatalf("Animepahe should not be touched with no-animepahe, search=%#v episode=%#v", animepaheSearch, animepaheCalls)
	}
	// Preferred lookup, fallback probe, then a fresh fallback resolution
	// on accept: signed URLs may expire while the prompt is on screen.
	if got := strings.Join(waitForStubCalls(allanime, 3), ","); got != "allanime:dub,allanime:sub,allanime:sub" {
		t.Fatalf("expected AllAnime dub, probe, then fresh sub with no-animepahe, got %s", got)
	}
}
