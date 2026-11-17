package internal

import (
	_ "github.com/wraient/curd/internal/loadproviders"

	"testing"

	"github.com/wraient/curd/internal/providers/anikoto"
	"github.com/wraient/curd/internal/providers/animepahe"
	"github.com/wraient/curd/internal/providers/anineko"
	"github.com/wraient/curd/internal/providers/anipm"
	"github.com/wraient/curd/internal/providers/anipub"
	"github.com/wraient/curd/internal/providers/senshi"
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
	match := func(options []SelectionOption, anime *Anime, query string) (SelectionOption, bool) {
		return identityMatchProviderOption(options, anime, identityScope{query: query})
	}

	t.Run("qualified mal key", func(t *testing.T) {
		options := []SelectionOption{{Title: "Unrelated Show", Key: "anipub::31646"}}
		matchOpt, ok := match(options, newAnime(), "something else")
		id := rawProviderKey(matchOpt.Key)
		if !ok || id != "31646" {
			t.Fatalf("expected raw MAL id match, got %q %v", id, ok)
		}
	})

	t.Run("fuzzy title does not match", func(t *testing.T) {
		options := []SelectionOption{{Title: "March comes in like a lion Season 2", Key: "s2"}}
		if _, ok := match(options, newAnime(), "3-gatsu no Lion"); ok {
			t.Fatal("fuzzy title must never identity-match")
		}
	})

	t.Run("unique exact title matches", func(t *testing.T) {
		options := []SelectionOption{
			{Title: "March comes in like a lion Season 2", Key: "s2"},
			{Title: "March comes in like a lion", Key: "s1"},
		}
		matchOpt, ok := match(options, newAnime(), "3-gatsu no Lion")
		if !ok || rawProviderKey(matchOpt.Key) != "s1" {
			t.Fatalf("expected unique exact title match s1, got %q %v", matchOpt.Key, ok)
		}
	})

	t.Run("duplicate exact titles do not match", func(t *testing.T) {
		options := []SelectionOption{
			{Title: "March comes in like a lion", Key: "s1"},
			{Title: "March comes in like a lion", Key: "s2"},
		}
		if _, ok := match(options, newAnime(), "March comes in like a lion"); ok {
			t.Fatal("ambiguous exact titles must go to the picker, not auto-match")
		}
	})

	t.Run("mal zero never matches", func(t *testing.T) {
		anime := newAnime()
		anime.MalId = 0
		anime.AnilistId = 0
		options := []SelectionOption{{Title: "Unrelated", Key: "0"}}
		if _, ok := match(options, anime, "other"); ok {
			t.Fatal("MAL 0 must never identity-match")
		}
	})
}

func TestIdentityMatchDisambiguatesByEpisodeCount(t *testing.T) {
	anime := &Anime{
		AnilistId: 1535,
		MalId:     1535,
		Title:     AnimeTitle{English: "Death Note", Romaji: "Death Note"},
	}
	options := []SelectionOption{
		{Title: "Death Note", Key: "anipub::1", ExtraData: anipub.SearchItem{Name: "Death Note", Episodes: 10}},
		{Title: "Death Note", Key: "anipub::2", ExtraData: anipub.SearchItem{Name: "Death Note", Episodes: 37}},
	}
	matched, ok := identityMatchProviderOption(options, anime, identityScope{query: "death note", episodes: 37})
	if !ok || rawProviderKey(matched.Key) != "2" {
		t.Fatalf("expected episode-count disambiguation to 37-ep series, got %q %v", matched.Key, ok)
	}
}

func TestIdentityMatchDisambiguatesByReleaseType(t *testing.T) {
	anime := &Anime{
		AnilistId: 32281,
		MalId:     32281,
		Title:     AnimeTitle{English: "Your Name", Romaji: "Kimi no Na wa"},
	}
	options := []SelectionOption{
		{Title: "Your Name", Key: "senshi::1", ExtraData: senshi.SearchItem{MalID: 0, Type: "TV", Episodes: 1}},
		{Title: "Your Name", Key: "senshi::2", ExtraData: senshi.SearchItem{MalID: 0, Type: "Movie", Episodes: 1}},
	}
	matched, ok := identityMatchProviderOption(options, anime, identityScope{query: "your name", episodes: 1, format: "MOVIE"})
	if !ok || rawProviderKey(matched.Key) != "2" {
		t.Fatalf("expected type disambiguation to movie, got %q %v", matched.Key, ok)
	}
}

func TestIdentityMatchUsesAnikotoJapaneseTitle(t *testing.T) {
	anime := &Anime{
		Title: AnimeTitle{Romaji: "Sousou no Frieren", English: "Frieren: Beyond Journey's End"},
	}
	options := []SelectionOption{
		{
			Title:     "Frieren: Beyond Journey's End",
			Key:       "anikoto::frieren-slug",
			ExtraData: anikoto.SearchItem{Title: "Frieren: Beyond Journey's End", JapaneseTitle: "Sousou no Frieren"},
		},
	}
	matched, ok := identityMatchProviderOption(options, anime, identityScope{query: "sousou no frieren"})
	if !ok || rawProviderKey(matched.Key) != "frieren-slug" {
		t.Fatalf("expected romaji match via data-jp, got %q %v", matched.Key, ok)
	}
}

func TestStripTrailingYearKeepsFullOtherwise(t *testing.T) {
	if got := stripTrailingYear("hunter x hunter 2011"); got != "hunter x hunter" {
		t.Fatalf("trailing catalog year must strip, got %q", got)
	}
	if got := stripTrailingYear("hunter x hunter (2011)"); got != "hunter x hunter (2011)" {
		t.Fatalf("raw parens form is handled after normalization, got %q", got)
	}
	if got := stripTrailingYear("frieren"); got != "frieren" {
		t.Fatalf("plain title must be untouched, got %q", got)
	}
	if got := stripTrailingYear("2012"); got != "2012" {
		t.Fatalf("bare year-only string must not vanish, got %q", got)
	}
	if got := stripTrailingYear("steins gate 0"); got != "steins gate 0" {
		t.Fatalf("short numeric suffix must stay, got %q", got)
	}
}

func TestIdentityMatchPrefersFullOverStrippedVariant(t *testing.T) {
	anime := &Anime{Title: AnimeTitle{Romaji: "Frieren: Beyond Journey's End"}}
	options := []SelectionOption{
		{Title: "Frieren: Beyond Journey's End", Key: "anipm::frieren-clean"},
		{Title: "Frieren: Beyond Journey's End (2023)", Key: "anikoto::frieren-2023"},
	}
	matched, ok := identityMatchProviderOption(options, anime, identityScope{query: "frieren beyond journey's end"})
	if !ok || rawProviderKey(matched.Key) != "frieren-clean" {
		t.Fatalf("full exact title must win over year-suffixed variant, got %q %v", matched.Key, ok)
	}
}

func TestIdentityMatchYearStrippedWithEpisodeCount(t *testing.T) {
	anime := &Anime{Title: AnimeTitle{English: "Hunter x Hunter", Romaji: "Hunter x Hunter"}}
	options := []SelectionOption{
		{Title: "Hunter x Hunter (1999)", Key: "anipub::1", ExtraData: anipub.SearchItem{Name: "Hunter x Hunter", Episodes: 62}},
		{Title: "Hunter x Hunter (2011)", Key: "anipub::2", ExtraData: anipub.SearchItem{Name: "Hunter x Hunter", Episodes: 148}},
	}
	matched, ok := identityMatchProviderOption(options, anime, identityScope{query: "hunter x hunter", episodes: 148})
	if !ok || rawProviderKey(matched.Key) != "2" {
		t.Fatalf("expected 2011 entry via episode count, got %q %v", matched.Key, ok)
	}
}

func TestIdentityMatchEnrichesMissingEpisodeCounts(t *testing.T) {
	anime := &Anime{Title: AnimeTitle{English: "Hunter x Hunter", Romaji: "Hunter x Hunter"}}
	options := []SelectionOption{
		{Title: "Hunter x Hunter", Key: "anikoto::hxh-1999"},
		{Title: "Hunter x Hunter", Key: "anikoto::hxh-2011"},
	}
	lists := map[string][]string{
		"hxh-1999": {"1", "2"},
		"hxh-2011": {"1", "2", "3"},
	}
	fetch := func(provider, rawID, mode string) ([]string, error) {
		return lists[rawID], nil
	}
	matched, ok := identityMatchProviderOption(options, anime, identityScope{query: "hunter x hunter", episodes: 3, fetchEpisodes: fetch})
	if !ok || rawProviderKey(matched.Key) != "hxh-2011" {
		t.Fatalf("expected enrichment to resolve 2011 entry, got %q %v", matched.Key, ok)
	}
}

func TestIdentityMatchEnrichmentBudgetSkipsAmbiguity(t *testing.T) {
	anime := &Anime{Title: AnimeTitle{English: "Hunter x Hunter", Romaji: "Hunter x Hunter"}}
	options := []SelectionOption{
		{Title: "Hunter x Hunter", Key: "anikoto::hxh-1999"},
		{Title: "Hunter x Hunter", Key: "anikoto::hxh-2011"},
	}
	// No fetcher: identical titles without episode data must not auto-match.
	if _, ok := identityMatchProviderOption(options, anime, identityScope{query: "hunter x hunter", episodes: 148}); ok {
		t.Fatal("ambiguous identical titles without episode data must go to the picker")
	}
}

func TestIdentityMatchEnrichesUnqualifiedPlaybackKeys(t *testing.T) {
	anime := &Anime{Title: AnimeTitle{English: "Hunter x Hunter", Romaji: "Hunter x Hunter"}}
	options := []SelectionOption{
		{Title: "Hunter x Hunter", Key: "hxh-1999"},
		{Title: "Hunter x Hunter", Key: "hxh-2011"},
	}
	lists := map[string][]string{
		"hxh-1999": {"1", "2"},
		"hxh-2011": {"1", "2", "3"},
	}
	fetch := func(providerName, rawID, mode string) ([]string, error) {
		if providerName != "anikoto" {
			t.Fatalf("expected anikoto fallback provider, got %q", providerName)
		}
		return lists[rawID], nil
	}
	scope := identityScope{query: "hunter x hunter", episodes: 3, mode: "sub", provider: "anikoto", fetchEpisodes: fetch}
	matched, ok := identityMatchProviderOption(options, anime, scope)
	if !ok || rawProviderKey(matched.Key) != "hxh-2011" {
		t.Fatalf("expected unqualified enrichment to resolve 2011 entry, got %q %v", matched.Key, ok)
	}
}

func TestIdentityMatchVetoesContradictingEpisodeCount(t *testing.T) {
	anime := &Anime{Title: AnimeTitle{English: "Hunter x Hunter", Romaji: "Hunter x Hunter"}}
	single := []SelectionOption{
		{Title: "Hunter x Hunter", Key: "anipub::1", ExtraData: anipub.SearchItem{Name: "Hunter x Hunter", Episodes: 62}},
	}
	if _, ok := identityMatchProviderOption(single, anime, identityScope{query: "hunter x hunter", episodes: 148}); ok {
		t.Fatal("a lone candidate with a contradicting episode count must not auto-match")
	}
	multi := []SelectionOption{
		{Title: "Hunter x Hunter", Key: "anipub::1", ExtraData: anipub.SearchItem{Name: "Hunter x Hunter", Episodes: 62}},
		{Title: "Hunter x Hunter", Key: "anipub::2", ExtraData: anipub.SearchItem{Name: "Hunter x Hunter", Episodes: 148}},
	}
	matched, ok := identityMatchProviderOption(multi, anime, identityScope{query: "hunter x hunter", episodes: 148})
	if !ok || rawProviderKey(matched.Key) != "2" {
		t.Fatalf("contradicting entry must be vetoed, 148-episode entry matched, got %q %v", matched.Key, ok)
	}
}

func TestIdentityMatchVetoesContradictingReleaseType(t *testing.T) {
	anime := &Anime{Title: AnimeTitle{English: "Your Name", Romaji: "Your Name"}}
	single := []SelectionOption{
		{Title: "Your Name", Key: "senshi::1", ExtraData: senshi.SearchItem{MalID: 0, Type: "TV", Episodes: 1}},
	}
	if _, ok := identityMatchProviderOption(single, anime, identityScope{query: "your name", episodes: 1, format: "MOVIE"}); ok {
		t.Fatal("a lone candidate with a contradicting release type must not auto-match")
	}
}

func TestNormalizeReleaseType(t *testing.T) {
	if normalizeReleaseType("TV_SHORT") != normalizeReleaseType("TV Short") {
		t.Fatal("TV_SHORT must equal TV Short")
	}
	if normalizeReleaseType("movie") != "MOVIE" {
		t.Fatalf("got %q", normalizeReleaseType("movie"))
	}
	if normalizeReleaseType("") != "" {
		t.Fatal("empty must stay empty")
	}
}

func TestProviderFactsCoverAllRichProviders(t *testing.T) {
	if got := providerFactsFromExtra(anipm.SearchItem{MalID: 1, Episodes: 2, Type: "TV"}); got.malID != 1 || got.episodes != 2 || got.typ != "TV" {
		t.Fatalf("anipm facts: %+v", got)
	}
	if got := providerFactsFromExtra(animepahe.SearchItem{Episodes: 3, Type: "TV"}); got.episodes != 3 || got.typ != "TV" {
		t.Fatalf("animepahe facts: %+v", got)
	}
	if got := providerFactsFromExtra(anineko.SearchItem{Type: "Movie"}); got.typ != "Movie" {
		t.Fatalf("anineko facts: %+v", got)
	}
	if got := malIDFromProviderExtraData(anipm.SearchItem{MalID: 42}); got != 42 {
		t.Fatalf("anipm MAL must resolve, got %d", got)
	}
}

func TestMappingTargetScopePrefersEntry(t *testing.T) {
	anime := &Anime{TotalEpisodes: 99}
	entry := &Entry{Media: Media{Episodes: 24, Format: "TV"}}
	episodes, format := mappingTargetScope(anime, entry)
	if episodes != 24 || format != "TV" {
		t.Fatalf("expected entry scope, got %d %q", episodes, format)
	}
	episodes, format = mappingTargetScope(anime, nil)
	if episodes != 99 || format != "" {
		t.Fatalf("expected anime fallback, got %d %q", episodes, format)
	}
}
