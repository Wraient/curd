package internal

import (
	"strings"
	"testing"

	"github.com/wraient/curd/internal/providers/animepahe"
)

func TestFormatAnimeSearchLabel(t *testing.T) {
	if got := FormatAnimeSearchLabel("Frieren", 28); got != "Frieren (28 episodes)" {
		t.Fatalf("unexpected label: %q", got)
	}
	if got := FormatAnimeSearchLabel("Airing Show", 0); got != "Airing Show" {
		t.Fatalf("unexpected label without episodes: %q", got)
	}
}

func TestProviderSearchOptionsForDisplayMarksEpisodeMatch(t *testing.T) {
	options := []SelectionOption{
		{Key: "a", Label: "One Piece (1090 episodes) [allanime]", Title: "One Piece"},
		{Key: "b", Label: "One Pace (300 episodes) [anineko]", Title: "One Pace"},
	}
	entry := &Entry{Media: Media{Episodes: 1090}}

	display := ProviderSearchOptionsForDisplay(options, entry)
	if display[0].Label != "One Piece (1090 episodes) [allanime] ✓" {
		t.Fatalf("expected match marker on first option, got %q", display[0].Label)
	}
	if display[1].Label != "One Pace (300 episodes) [anineko]" {
		t.Fatalf("expected unchanged second option, got %q", display[1].Label)
	}
}

func TestManualProviderSearchHint(t *testing.T) {
	entry := &Entry{
		Media: Media{
			Format:   "TV",
			Episodes: 24,
			Title: AnimeTitle{
				Romaji: "Frieren",
			},
		},
	}
	hint := ManualProviderSearchHint(&CurdConfig{SubOrDub: "sub"}, entry, "frieren", "sub")
	if !strings.Contains(hint, `Searching for "Frieren"`) {
		t.Fatalf("unexpected hint: %q", hint)
	}
	if !strings.Contains(hint, "TV") || !strings.Contains(hint, "24 episodes") || !strings.Contains(hint, "sub") {
		t.Fatalf("expected type, episode count, and mode in hint: %q", hint)
	}
}

func TestManualProviderSearchEnabled(t *testing.T) {
	if ManualProviderSearchEnabled(&CurdConfig{ManualProviderSearch: true}) != true {
		t.Fatal("expected manual provider search to be enabled")
	}
	if ManualProviderSearchEnabled(&CurdConfig{}) != false {
		t.Fatal("expected manual provider search to be disabled by default")
	}
}

func TestEnsureEpisodeCountInLabelUsesExtraData(t *testing.T) {
	option := SelectionOption{
		Key:       "session",
		Label:     "Frieren [animepahe]",
		Title:     "Frieren",
		ExtraData: animepahe.SearchItem{Title: "Frieren", Episodes: 28},
	}

	display := ensureEpisodeCountInLabel(option)
	if display.Label != "Frieren (28 episodes) [animepahe]" {
		t.Fatalf("unexpected label: %q", display.Label)
	}
}
