package internal

import (
	_ "github.com/wraient/curd/internal/loadproviders"

	"fmt"
	"strings"
	"testing"

	"github.com/wraient/curd/internal/providers/senshi"
)

func TestMergeDedupeSearchResultsKeepsFirstAndDedupes(t *testing.T) {
	primary := []SelectionOption{
		{Key: "anipub::1", Title: "Frieren"},
		{Key: "anipub::2", Title: "Frieren"},
	}
	alternate := []SelectionOption{
		{Key: "anipub::1", Title: "Frieren"},
		{Key: "anipub::3", Title: "Frieren"},
		{Key: "senshi::9", Title: "Frieren"},
	}
	merged, _ := mergeDedupeSearchResults(primary, alternate, nil, nil)
	keys := make([]string, 0, len(merged))
	for _, option := range merged {
		keys = append(keys, option.Key)
	}
	// Dedupe is by qualified key only: same-title variants from one provider
	// stay visible so neither auto-match nor the picker can lose a show.
	if strings.Join(keys, ",") != "anipub::1,anipub::2,anipub::3,senshi::9" {
		t.Fatalf("unexpected merged keys %v", keys)
	}
}

func TestAlternateSearchTitleUsesOtherTrackerTitle(t *testing.T) {
	anime := &Anime{Title: AnimeTitle{Romaji: "Sousou no Frieren", English: "Frieren: Beyond Journey's End"}}
	if got := alternateSearchTitle(anime, "Sousou no Frieren"); got != "Frieren: Beyond Journey's End" {
		t.Fatalf("expected English alternate, got %q", got)
	}
	if got := alternateSearchTitle(anime, "Frieren: Beyond Journey's End"); got != "Sousou no Frieren" {
		t.Fatalf("expected Romaji alternate, got %q", got)
	}
	if got := alternateSearchTitle(anime, "frieren"); got != "" {
		t.Fatalf("unrelated query must not expand, got %q", got)
	}
}

func TestTruncateSearchQueryBoundsGarbage(t *testing.T) {
	if got := truncateSearchQuery("  frieren  "); got != "frieren" {
		t.Fatalf("expected trimmed query, got %q", got)
	}
	if got := truncateSearchQuery(strings.Repeat("x", 4096)); len(got) != 200 {
		t.Fatalf("expected 200-char cap, got %d", len(got))
	}
}

func TestEpisodeNumberInListMatchesNumerically(t *testing.T) {
	if !episodeNumberInList([]string{"1", "2", "12"}, 2) {
		t.Fatal("expected episode 2 member")
	}
	if episodeNumberInList([]string{"1", "2", "12"}, 12) == false {
		t.Fatal("expected episode 12 member")
	}
	if episodeNumberInList([]string{"1", "12"}, 2) {
		t.Fatal("episode 2 must not match entries 1 and 12")
	}
	if episodeNumberInList([]string{"special", "12.5", ""}, 12) {
		t.Fatal("specials and decimals must not match a plain episode")
	}
	if episodeNumberInList([]string{"1"}, 0) {
		t.Fatal("episode 0 never matches")
	}
}

func TestProviderNameFromSelectionUsesQualifiedKey(t *testing.T) {
	withAllProvidersEnabledForTest(t)
	config := &CurdConfig{Provider: `["senshi","anineko"]`}

	selected := SelectionOption{
		Key:   "anineko::frieren-beyond-journeys-end",
		Label: "Frieren: Beyond Journey's End [anineko]",
	}
	if got := providerNameFromSelection(config, selected); got != "anineko" {
		t.Fatalf("providerNameFromSelection() = %q, want anineko", got)
	}

	var anime Anime
	state := &providerMappingSearchState{query: "frieren"}
	applySelectedProviderMapping(config, state, &anime, selected)
	if anime.ProviderName != "anineko" || anime.ProviderId != "frieren-beyond-journeys-end" {
		t.Fatalf("unexpected mapping %+v", anime)
	}
}

func TestProviderNameFromSelectionFallsBackToConfigured(t *testing.T) {
	withAllProvidersEnabledForTest(t)
	config := &CurdConfig{Provider: `["senshi","anineko"]`}

	selected := SelectionOption{Key: "raw-id", Label: "Some Title"}
	if got := providerNameFromSelection(config, selected); got != "senshi" {
		t.Fatalf("providerNameFromSelection() = %q, want senshi", got)
	}
}

func TestApplyMatchedProviderMappingParsesQualifiedID(t *testing.T) {
	withAllProvidersEnabledForTest(t)
	config := &CurdConfig{Provider: `["senshi","anineko"]`}
	state := &providerMappingSearchState{query: "frieren"}

	anime := Anime{ProviderId: "senshi::52991"}
	applyMatchedProviderMapping(config, state, &anime)
	if anime.ProviderName != "senshi" || anime.ProviderId != "52991" {
		t.Fatalf("unexpected mapping %+v", anime)
	}
}

func TestPromptMappingRecoveryShowsRenameAudioBack(t *testing.T) {
	withAllProvidersEnabledForTest(t)
	config := &CurdConfig{Provider: `["anipub","senshi"]`, SubOrDub: "sub"}
	state := &providerMappingSearchState{query: "missing"}

	withPromptSelect(t, func(options []SelectionOption) (SelectionOption, error) {
		keys := make([]string, 0, len(options))
		for _, option := range options {
			keys = append(keys, option.Key)
		}
		joined := strings.Join(keys, ",")
		for _, want := range []string{"custom", "audio", "back"} {
			if !strings.Contains(joined, want) {
				t.Fatalf("M1 missing %q in %s", want, joined)
			}
		}
		for _, banned := range []string{"next_provider", "pick", "episode"} {
			if strings.Contains(joined, banned) {
				t.Fatalf("M1 must not offer %q: %s", banned, joined)
			}
		}
		return SelectionOption{Key: "back"}, nil
	})

	action, episode := promptMappingRecovery(config, state, "sub", []ProviderSearchError{{Provider: "anipub", Err: "boom"}}, nil, 0, "")
	if action != "back" || episode != 0 {
		t.Fatalf("expected back, got %q %d", action, episode)
	}
}

func TestPromptMappingRecoveryEpisodeRowOnlyWithResultsAndEpisode(t *testing.T) {
	withAllProvidersEnabledForTest(t)
	config := &CurdConfig{Provider: `["anipub"]`, SubOrDub: "sub"}
	state := &providerMappingSearchState{query: "frieren"}
	results := []SelectionOption{{Key: "anipub::1", Title: "Frieren", Label: "Frieren (28 episodes)"}}

	withPromptSelect(t, func(options []SelectionOption) (SelectionOption, error) {
		for _, option := range options {
			if option.Key == "episode" {
				if !strings.Contains(option.Label, "28") {
					t.Fatalf("episode row should carry the provider max, got %q", option.Label)
				}
				return SelectionOption{Key: "back"}, nil
			}
		}
		t.Fatalf("expected episode row with results+episode, got %#v", options)
		return SelectionOption{}, nil
	})
	action, _ := promptMappingRecovery(config, state, "sub", nil, results, 3, "")
	if action != "back" {
		t.Fatalf("expected back, got %q", action)
	}
}

func TestPromptMappingRecoveryUnknownKeyMapsToBack(t *testing.T) {
	withAllProvidersEnabledForTest(t)
	config := &CurdConfig{Provider: `["anipub"]`, SubOrDub: "sub"}
	state := &providerMappingSearchState{query: "frieren"}

	withPromptSelect(t, func(options []SelectionOption) (SelectionOption, error) {
		return SelectionOption{Key: "something-unexpected"}, nil
	})
	if action, _ := promptMappingRecovery(config, state, "sub", nil, nil, 0, ""); action != "back" {
		t.Fatalf("expected back for unknown key, got %q", action)
	}
}

func TestPromptMappingRecoveryQuitKeyMapsToQuit(t *testing.T) {
	withAllProvidersEnabledForTest(t)
	config := &CurdConfig{Provider: `["anipub"]`, SubOrDub: "sub"}
	state := &providerMappingSearchState{query: "frieren"}

	withPromptSelect(t, func(options []SelectionOption) (SelectionOption, error) {
		return SelectionOption{Key: "-1", Label: "Quit"}, nil
	})
	if action, _ := promptMappingRecovery(config, state, "sub", nil, nil, 0, ""); action != "quit" {
		t.Fatalf("expected quit for quit key, got %q", action)
	}
}

func TestHandleMappingRecoveryActionAudioFlipsModeOnly(t *testing.T) {
	config := &CurdConfig{Provider: `["anipub"]`, SubOrDub: "sub"}
	state := &providerMappingSearchState{query: "frieren"}
	anime := &Anime{}

	mode := "sub"
	outcome, cont, err := handleMappingRecoveryAction(config, state, anime, "audio", 0, &mode)
	if err != nil || !cont || outcome != ProviderMappingOK {
		t.Fatalf("audio flip failed: %v %v %v", outcome, cont, err)
	}
	if mode != "dub" {
		t.Fatalf("expected loop-local mode flip to dub, got %q", mode)
	}
	if config.SubOrDub != "sub" {
		t.Fatalf("config mode must stay sub, got %q", config.SubOrDub)
	}
}

func TestHandleMappingRecoveryActionEpisodeSetsNumber(t *testing.T) {
	config := &CurdConfig{Provider: `["anipub"]`}
	state := &providerMappingSearchState{query: "frieren"}
	anime := &Anime{}
	mode := "sub"

	outcome, cont, err := handleMappingRecoveryAction(config, state, anime, "episode", 7, &mode)
	if err != nil || !cont || outcome != ProviderMappingOK {
		t.Fatalf("episode action failed: %v %v %v", outcome, cont, err)
	}
	if anime.Ep.Number != 7 {
		t.Fatalf("expected episode 7, got %d", anime.Ep.Number)
	}
}

func TestValidateSavedMappingCachesAndRejectsMissingEpisode(t *testing.T) {
	provider := &stackStubProvider{
		name:         "anipub",
		episodeLists: map[string][]string{"sub": {"1", "2"}},
	}
	withProviderFactories(t, provider)
	resetMappingValidationCacheForTest(t)

	config := &CurdConfig{Provider: `["anipub"]`, SubOrDub: "sub"}
	anime := &Anime{ProviderName: "anipub", ProviderId: "2454", Title: AnimeTitle{Romaji: "Example"}}

	episodes, usable := validateSavedMapping(config, anime, 2)
	if !usable || len(episodes) != 2 {
		t.Fatalf("expected usable mapping with episodes, got %v %#v", usable, episodes)
	}
	callsAfterFirst := len(provider.episodeListCalls)
	if _, usable := validateSavedMapping(config, anime, 2); !usable {
		t.Fatal("expected cached usable mapping")
	}
	if len(provider.episodeListCalls) != callsAfterFirst {
		t.Fatalf("expected cached list, provider called again: %#v", provider.episodeListCalls)
	}

	if _, usable := validateSavedMapping(config, anime, 9); usable {
		t.Fatal("episode 9 is not in the listing; mapping must be rejected")
	}
}

func TestValidateSavedMappingTransientErrorKeepsMapping(t *testing.T) {
	provider := &stackStubProvider{
		name:           "anipub",
		episodeListErr: fmt.Errorf("upstream exploded"),
	}
	withProviderFactories(t, provider)
	resetMappingValidationCacheForTest(t)

	config := &CurdConfig{Provider: `["anipub"]`, SubOrDub: "sub"}
	anime := &Anime{ProviderName: "anipub", ProviderId: "2454"}
	if _, usable := validateSavedMapping(config, anime, 1); !usable {
		t.Fatal("transient lookup errors must keep the mapping; playback surfaces the error")
	}
}

func TestFormatProviderFailureNotesTruncates(t *testing.T) {
	failures := []ProviderSearchError{
		{Provider: "anipub", Err: strings.Repeat("e", 200)},
		{Provider: "senshi", Err: "boom"},
	}
	notes := formatProviderFailureNotes(failures)
	if !strings.Contains(notes, "anipub:") || !strings.Contains(notes, "senshi: boom") {
		t.Fatalf("notes must name both providers:\n%s", notes)
	}
	if len(notes) > mappingRecoveryMessageMaxChars {
		t.Fatalf("notes exceed %d chars: %d", mappingRecoveryMessageMaxChars, len(notes))
	}
}

func TestStackedSearchWithExpansionMergesAlternateTitle(t *testing.T) {
	romaji := &stackStubProvider{
		name: "anipub",
		searchResults: map[string][]SelectionOption{
			"sub": {},
		},
	}
	withProviderFactories(t, romaji)

	anime := &Anime{Title: AnimeTitle{Romaji: "Sousou no Frieren", English: "Frieren"}}
	results, _, err := stackedSearchWithExpansion(&CurdConfig{Provider: `["anipub"]`, SubOrDub: "sub"}, anime, "Sousou no Frieren", "sub")
	if err != nil {
		t.Fatalf("expected no error with stub provider, got %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected no results with stub provider, got %#v", results)
	}
	// Both Romaji and English queries must have been attempted.
	queries := map[string]bool{}
	for _, call := range romaji.searchCalls {
		queries[call] = true
	}
	if !queries["anipub:sub:Sousou no Frieren"] || !queries["anipub:sub:Frieren"] {
		t.Fatalf("expected Romaji then English searches, got %#v", romaji.searchCalls)
	}
}

func TestMalIDFromProviderExtraDataCoversSenshi(t *testing.T) {
	if got := malIDFromProviderExtraData(senshi.SearchItem{MalID: 52991}); got != 52991 {
		t.Fatalf("expected senshi MAL id, got %d", got)
	}
}
