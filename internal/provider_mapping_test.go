package internal

import (
	"testing"

	_ "github.com/wraient/curd/internal/loadproviders"
)

func TestProviderMappingSearchStateNextProvider(t *testing.T) {
	t.Run("single provider hides next option", func(t *testing.T) {
		state := &providerMappingSearchState{allProviders: []string{"anineko"}}
		if got := state.nextProviderLabel(); got != "" {
			t.Fatalf("nextProviderLabel() = %q, want empty", got)
		}
		if state.advanceToNextProvider() {
			t.Fatal("advanceToNextProvider() = true, want false")
		}
	})

	t.Run("stacked search offers first provider", func(t *testing.T) {
		state := &providerMappingSearchState{allProviders: []string{"senshi", "anineko", "anipub"}}
		if got := state.nextProviderLabel(); got != "senshi" {
			t.Fatalf("nextProviderLabel() = %q, want senshi", got)
		}
	})

	t.Run("sequential advance walks stack", func(t *testing.T) {
		state := &providerMappingSearchState{allProviders: []string{"senshi", "anineko", "anipub"}}
		if !state.advanceToNextProvider() {
			t.Fatal("expected first advance to succeed")
		}
		if !state.sequential || state.providerIndex != 0 {
			t.Fatalf("expected sequential at index 0, got sequential=%v index=%d", state.sequential, state.providerIndex)
		}
		if got := state.nextProviderLabel(); got != "anineko" {
			t.Fatalf("nextProviderLabel() = %q, want anineko", got)
		}
		if !state.advanceToNextProvider() || state.providerIndex != 1 {
			t.Fatalf("expected index 1 after second advance, got %d", state.providerIndex)
		}
		if got := state.nextProviderLabel(); got != "anipub" {
			t.Fatalf("nextProviderLabel() = %q, want anipub", got)
		}
		if !state.advanceToNextProvider() || state.providerIndex != 2 {
			t.Fatalf("expected index 2 after third advance, got %d", state.providerIndex)
		}
		if got := state.nextProviderLabel(); got != "" {
			t.Fatalf("nextProviderLabel() = %q, want empty at end", got)
		}
		if state.advanceToNextProvider() {
			t.Fatal("expected final advance to fail")
		}
	})
}

func TestProviderNameFromSelectionUsesSequentialProvider(t *testing.T) {
	withAllProvidersEnabledForTest(t)
	config := &CurdConfig{Provider: `["senshi","anineko"]`}
	state := &providerMappingSearchState{
		allProviders:  []string{"senshi", "anineko"},
		sequential:    true,
		providerIndex: 1,
	}

	selected := SelectionOption{
		Key:   "frieren-beyond-journeys-end",
		Label: "Frieren: Beyond Journey's End",
		Title: "Frieren: Beyond Journey's End",
	}
	if got := providerNameFromSelection(config, state, selected); got != "anineko" {
		t.Fatalf("providerNameFromSelection() = %q, want anineko", got)
	}

	var anime Anime
	applySelectedProviderMapping(config, state, &anime, selected)
	if anime.ProviderName != "anineko" || anime.ProviderId != "frieren-beyond-journeys-end" {
		t.Fatalf("unexpected mapping %+v", anime)
	}
}

func TestProviderNameFromSelectionUsesQualifiedKey(t *testing.T) {
	withAllProvidersEnabledForTest(t)
	config := &CurdConfig{Provider: `["senshi","anineko"]`}
	state := &providerMappingSearchState{allProviders: []string{"senshi", "anineko"}}

	selected := SelectionOption{
		Key:   "anineko::frieren-beyond-journeys-end",
		Label: "Frieren: Beyond Journey's End [anineko]",
	}
	if got := providerNameFromSelection(config, state, selected); got != "anineko" {
		t.Fatalf("providerNameFromSelection() = %q, want anineko", got)
	}
}

func TestSwitchToSingleProviderStreamValidatesInput(t *testing.T) {
	withAllProvidersEnabledForTest(t)
	config := &CurdConfig{Provider: `["senshi","anineko"]`}
	user := &User{
		AnimeList: AnimeList{
			Watching: []Entry{{Media: Media{ID: 11757, Title: AnimeTitle{Romaji: "Sword Art Online"}}}},
		},
	}
	var databaseAnimes []Anime

	t.Run("empty provider name is rejected before any search", func(t *testing.T) {
		if _, err := SwitchToSingleProviderStream(config, user, &databaseAnimes, 11757, 3, ""); err == nil {
			t.Fatal("expected error for empty provider name")
		}
	})

	t.Run("invalid episode number is rejected before any search", func(t *testing.T) {
		if _, err := SwitchToSingleProviderStream(config, user, &databaseAnimes, 11757, 0, "anipub"); err == nil {
			t.Fatal("expected error for episode <= 0")
		}
	})

	t.Run("unknown anilist id is rejected before any search", func(t *testing.T) {
		if _, err := SwitchToSingleProviderStream(config, user, &databaseAnimes, 999999, 3, "anipub"); err == nil {
			t.Fatal("expected error for anime not present in the user's list")
		}
	})

	t.Run("nil user is rejected", func(t *testing.T) {
		if _, err := SwitchToSingleProviderStream(config, nil, &databaseAnimes, 11757, 3, "anipub"); err == nil {
			t.Fatal("expected error for nil user")
		}
	})
}

func TestApplyMatchedProviderMappingUsesSequentialProvider(t *testing.T) {
	withAllProvidersEnabledForTest(t)
	config := &CurdConfig{Provider: `["senshi","anineko"]`}
	state := &providerMappingSearchState{
		allProviders:  []string{"senshi", "anineko"},
		sequential:    true,
		providerIndex: 1,
	}

	anime := Anime{ProviderId: "frieren-beyond-journeys-end"}
	applyMatchedProviderMapping(config, state, &anime)
	if anime.ProviderName != "anineko" || anime.ProviderId != "frieren-beyond-journeys-end" {
		t.Fatalf("unexpected mapping %+v", anime)
	}
}
