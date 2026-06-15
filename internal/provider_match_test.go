package internal

import (
	"regexp"
	"strings"
	"testing"
)

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
