package internal

import (
	_ "github.com/wraient/curd/internal/loadproviders"

	"testing"
)

func TestRankProviderSearchOptionsOrdersBySignal(t *testing.T) {
	anime := &Anime{
		AnilistId:     21366,
		MalId:         31646,
		TotalEpisodes: 22,
		Title:         AnimeTitle{Romaji: "3-gatsu no Lion", English: "March comes in like a lion"},
	}
	options := []SelectionOption{
		{Title: "March comes in like a lion Season 2", Key: "s2", Label: "March comes in like a lion Season 2 (22 episodes)", Thumbnail: "https://s4.anilist.co/file/anilistcdn/media/anime/cover/large/bx98478-dF3mpSKiZkQu.jpg"},
		{Title: "March comes in like a lion", Key: "s1", Label: "March comes in like a lion (23 episodes)", Thumbnail: "https://cdn.myanimelist.net/images/anime/1637/108857.jpg"},
	}
	ranked := rankProviderSearchOptions(options, anime, "3-gatsu no Lion", nil)
	if len(ranked) != 2 {
		t.Fatalf("expected 2 ranked options, got %#v", ranked)
	}
	// Season 1 carries the AniList thumbnail identity signal and must rank first.
	if ranked[0].Key != "s1" {
		t.Fatalf("expected season 1 first, got %q (%q)", ranked[0].Key, ranked[0].Title)
	}
	if best, ok := selectBestProviderSearchResult(options, anime, "3-gatsu no Lion"); !ok || best.Key != "s1" {
		t.Fatalf("expected best result s1, got %#v %v", best, ok)
	}
}

func TestScoreIgnoresListPosition(t *testing.T) {
	anime := &Anime{Title: AnimeTitle{Romaji: "Example"}}
	a := SelectionOption{Title: "Example", Key: "a"}
	b := SelectionOption{Title: "Example", Key: "b"}
	if scoreProviderSearchOption(a, anime, "Example") != scoreProviderSearchOption(b, anime, "Example") {
		t.Fatal("identical options must score identically regardless of list position")
	}
}

func TestThumbnailExactTokenMatchRejectsSubstrings(t *testing.T) {
	if !thumbnailExactTokenMatch("https://s4.anilist.co/file/anilistcdn/media/anime/cover/large/bx21366-dF3mpSKiZkQu.jpg", 21366) {
		t.Fatal("expected exact AniList ID token match")
	}
	if thumbnailExactTokenMatch("https://s4.anilist.co/file/anilistcdn/media/anime/cover/large/bx213661-dF3mpSKiZkQu.jpg", 21366) {
		t.Fatal("ID 21366 must not match token 213661")
	}
	if thumbnailExactTokenMatch("https://cdn.example/poster-11.jpg", 1) {
		t.Fatal("ID 1 must not match token 11")
	}
}

func TestIdentityMatchAcceptsMalKeyAndExtraData(t *testing.T) {
	newAnime := func() *Anime {
		return &Anime{
			AnilistId: 21366,
			MalId:     31646,
			Title: AnimeTitle{
				Romaji:  "3-gatsu no Lion",
				English: "March comes in like a lion",
			},
		}
	}

	t.Run("qualified mal key", func(t *testing.T) {
		options := []SelectionOption{{Title: "Unrelated Show", Key: "anipub::31646"}}
		id, ok := identityMatchProviderOption(options, newAnime(), "something else")
		if !ok || id != "31646" {
			t.Fatalf("expected raw MAL id match, got %q %v", id, ok)
		}
	})

	t.Run("fuzzy title does not match", func(t *testing.T) {
		options := []SelectionOption{{Title: "March comes in like a lion Season 2", Key: "s2"}}
		if _, ok := identityMatchProviderOption(options, newAnime(), "3-gatsu no Lion"); ok {
			t.Fatal("fuzzy title must never identity-match")
		}
	})

	t.Run("unique exact title matches", func(t *testing.T) {
		options := []SelectionOption{
			{Title: "March comes in like a lion Season 2", Key: "s2"},
			{Title: "March comes in like a lion", Key: "s1"},
		}
		id, ok := identityMatchProviderOption(options, newAnime(), "3-gatsu no Lion")
		if !ok || id != "s1" {
			t.Fatalf("expected unique exact title match s1, got %q %v", id, ok)
		}
	})

	t.Run("duplicate exact titles do not match", func(t *testing.T) {
		options := []SelectionOption{
			{Title: "March comes in like a lion", Key: "s1"},
			{Title: "March comes in like a lion", Key: "s2"},
		}
		if _, ok := identityMatchProviderOption(options, newAnime(), "March comes in like a lion"); ok {
			t.Fatal("ambiguous exact titles must go to the picker, not auto-match")
		}
	})

	t.Run("mal zero never matches", func(t *testing.T) {
		anime := newAnime()
		anime.MalId = 0
		anime.AnilistId = 0
		options := []SelectionOption{{Title: "Unrelated", Key: "0"}}
		if _, ok := identityMatchProviderOption(options, anime, "other"); ok {
			t.Fatal("MAL 0 must never identity-match")
		}
	})
}
