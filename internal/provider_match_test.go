package internal

import (
	"regexp"
	"strings"
	"testing"

	"github.com/wraient/curd/internal/providers/senshi"
)

type fakeSearchProvider struct {
	name    string
	options []SelectionOption
}

func (p fakeSearchProvider) Name() string { return p.name }
func (p fakeSearchProvider) SearchAnime(query, mode string) ([]SelectionOption, error) {
	return p.options, nil
}
func (p fakeSearchProvider) EpisodesList(showID, mode string) ([]string, error) { return nil, nil }
func (p fakeSearchProvider) GetEpisodeURL(config CurdConfig, id string, epNo int) ([]string, error) {
	return nil, nil
}

func TestSelectBestProviderSearchResultMarchComesInLikeALion(t *testing.T) {
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
	best, ok := selectBestProviderSearchResult(options, anime, "3-gatsu no Lion")
	if !ok {
		t.Fatal("expected match")
	}
	if best.Key != "s1" {
		t.Fatalf("expected season 1, got %q (%q)", best.Key, best.Title)
	}
}

func TestConfidentProviderSearchMatchMarchComesInLikeALion(t *testing.T) {
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
	best, ok := confidentProviderSearchMatch(options, anime, "3-gatsu no Lion")
	if !ok {
		t.Fatal("expected confident auto-match")
	}
	if best.Key != "s1" {
		t.Fatalf("expected season 1, got %q", best.Key)
	}
}

func TestMalThumbnailMatchMarchComesInLikeALion(t *testing.T) {
	jikanUrls := []string{"https://cdn.myanimelist.net/images/anime/1637/108857.jpg"}
	thumb := "https://cdn.myanimelist.net/images/anime/1637/108857.jpg"
	malRegex := regexp.MustCompile(`myanimelist\.net/images/anime/[^/]+/([^/]+\.jpg)`)
	matches := malRegex.FindStringSubmatch(thumb)
	if len(matches) < 2 {
		t.Fatal("regex failed")
	}
	fileName := matches[1]
	found := false
	for _, url := range jikanUrls {
		if strings.HasSuffix(url, "/"+fileName) || strings.Contains(url, fileName) {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected MAL thumbnail match")
	}
}

func TestFindProviderIDForAnimeReplacesWeakSavedDifferentEntry(t *testing.T) {
	anime := &Anime{
		AnilistId:     11757,
		MalId:         11757,
		TotalEpisodes: 25,
		ProviderName:  "senshi",
		ProviderId:    "36475",
		Title:         AnimeTitle{English: "Sword Art Online", Romaji: "Sword Art Online"},
	}
	provider := fakeSearchProvider{
		name: "senshi",
		options: []SelectionOption{
			{
				Key:       "36475",
				Title:     "Sword Art Online Alternative: Gun Gale Online",
				Label:     "Sword Art Online Alternative: Gun Gale Online · TV · 2018 · 12 eps",
				Thumbnail: "https://senshi.live/posters/36475.webp",
				ExtraData: senshi.SearchItem{MalID: 36475, Title: "Sword Art Online Alternative: Gun Gale Online", Episodes: 12},
			},
			{
				Key:       "11757",
				Title:     "Sword Art Online",
				Label:     "Sword Art Online · TV · 2012 · 25 eps",
				Thumbnail: "https://senshi.live/posters/11757.webp",
				ExtraData: senshi.SearchItem{MalID: 11757, Title: "Sword Art Online", Episodes: 25},
			},
		},
	}

	got, err := findProviderIDForAnime(provider, anime, "sub")
	if err != nil {
		t.Fatalf("findProviderIDForAnime: %v", err)
	}
	if got != "11757" {
		t.Fatalf("expected stale GGO provider id to be replaced with SAO id 11757, got %q", got)
	}
}

func TestFindProviderIDForAnimeKeepsStrongSavedMatch(t *testing.T) {
	anime := &Anime{
		AnilistId:     11757,
		MalId:         11757,
		TotalEpisodes: 25,
		ProviderName:  "senshi",
		ProviderId:    "11757",
		Title:         AnimeTitle{English: "Sword Art Online", Romaji: "Sword Art Online"},
	}
	provider := fakeSearchProvider{
		name: "senshi",
		options: []SelectionOption{
			{
				Key:       "36475",
				Title:     "Sword Art Online Alternative: Gun Gale Online",
				Label:     "Sword Art Online Alternative: Gun Gale Online · TV · 2018 · 12 eps",
				Thumbnail: "https://senshi.live/posters/36475.webp",
				ExtraData: senshi.SearchItem{MalID: 36475, Title: "Sword Art Online Alternative: Gun Gale Online", Episodes: 12},
			},
			{
				Key:       "11757",
				Title:     "Sword Art Online",
				Label:     "Sword Art Online · TV · 2012 · 25 eps",
				Thumbnail: "https://senshi.live/posters/11757.webp",
				ExtraData: senshi.SearchItem{MalID: 11757, Title: "Sword Art Online", Episodes: 25},
			},
		},
	}

	got, err := findProviderIDForAnime(provider, anime, "sub")
	if err != nil {
		t.Fatalf("findProviderIDForAnime: %v", err)
	}
	if got != "11757" {
		t.Fatalf("expected strong saved SAO provider id to stay 11757, got %q", got)
	}
}
