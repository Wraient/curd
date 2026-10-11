package anikoto

import (
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/wraient/curd/internal/providers"
)

func episodesList(showID, mode string) ([]string, error) {
	_, _, err := parseShowID(showID)
	if err != nil {
		return nil, err
	}
	mode = providers.NormalizeTranslationType(mode)
	episodes, err := fetchEpisodeMetadata(showID)
	if err != nil {
		return nil, err
	}
	numbers := make([]int, 0, len(episodes))
	seen := make(map[int]struct{})
	for _, episode := range episodes {
		if !episodeAvailableForMode(episode, mode) {
			continue
		}
		if _, exists := seen[episode.Number]; exists {
			continue
		}
		seen[episode.Number] = struct{}{}
		numbers = append(numbers, episode.Number)
	}
	sort.Ints(numbers)
	result := make([]string, 0, len(numbers))
	for _, number := range numbers {
		result = append(result, strconv.Itoa(number))
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("no Anikoto %s episodes found", mode)
	}
	return result, nil
}

func fetchEpisodeMetadata(showID string) ([]episodeMetadata, error) {
	slug, animeID, err := parseShowID(showID)
	if err != nil {
		return nil, err
	}
	if animeID == 0 {
		animeID, err = resolveAnimeID(slug)
		if err != nil {
			return nil, err
		}
	}
	endpoint := fmt.Sprintf("%s/ajax/episode/list/%d?vrf=%s", baseURL, animeID, encryptVRF(strconv.Itoa(animeID)))
	referer := watchURL(slug, 0)
	raw, err := fetchResource(endpoint, referer, "application/json, text/javascript, */*; q=0.01", true)
	if err != nil {
		return nil, err
	}
	var response ajaxHTMLResponse
	if err := decodeAjaxResponse(raw, &response); err != nil {
		return nil, err
	}
	episodes := parseEpisodeMetadata(response.Result)
	if len(episodes) == 0 {
		return nil, fmt.Errorf("no Anikoto episodes found for %q", slug)
	}
	return episodes, nil
}

func parseEpisodeMetadata(body string) []episodeMetadata {
	episodes := make([]episodeMetadata, 0)
	seen := make(map[int]struct{})
	for _, tag := range htmlTagPattern.FindAllString(body, -1) {
		if !strings.HasPrefix(strings.ToLower(tag), "<a") {
			continue
		}
		attrs := parseHTMLAttrs(tag)
		number, err := strconv.Atoi(attrs["data-num"])
		if err != nil || number <= 0 || strings.TrimSpace(attrs["data-ids"]) == "" {
			continue
		}
		if _, exists := seen[number]; exists {
			continue
		}
		seen[number] = struct{}{}
		episodes = append(episodes, episodeMetadata{
			Number: number,
			IDs:    strings.TrimSpace(attrs["data-ids"]),
			Sub:    attrs["data-sub"] == "1",
			Dub:    attrs["data-dub"] == "1",
		})
	}
	return episodes
}

func parseShowID(showID string) (string, int, error) {
	parts := strings.SplitN(strings.TrimSpace(showID), "#", 2)
	slug := strings.TrimSpace(parts[0])
	if slug == "" || strings.ContainsAny(slug, "/?#") {
		return "", 0, fmt.Errorf("invalid Anikoto show id %q", showID)
	}
	animeID := 0
	if len(parts) == 2 {
		parsed, err := strconv.Atoi(parts[1])
		if err != nil || parsed <= 0 {
			return "", 0, fmt.Errorf("invalid Anikoto anime id %q", parts[1])
		}
		animeID = parsed
	}
	return slug, animeID, nil
}

func resolveAnimeID(slug string) (int, error) {
	body, err := fetchResource(watchURL(slug, 1), baseURL+"/", "text/html,application/xhtml+xml", false)
	if err != nil {
		return 0, err
	}
	for _, tag := range htmlTagPattern.FindAllString(string(body), -1) {
		attrs := parseHTMLAttrs(tag)
		if attrs["id"] == "watch-main" {
			animeID, err := strconv.Atoi(attrs["data-id"])
			if err == nil && animeID > 0 {
				return animeID, nil
			}
		}
	}
	return 0, fmt.Errorf("Anikoto anime ID not found for %q", slug)
}

func episodeForNumber(showID string, number int) (episodeMetadata, error) {
	episodes, err := fetchEpisodeMetadata(showID)
	if err != nil {
		return episodeMetadata{}, err
	}
	for _, episode := range episodes {
		if episode.Number == number {
			return episode, nil
		}
	}
	return episodeMetadata{}, fmt.Errorf("Anikoto episode %d not found", number)
}

func episodeAvailableForMode(episode episodeMetadata, mode string) bool {
	if mode == "dub" {
		return episode.Dub
	}
	return episode.Sub
}

func watchURL(slug string, episode int) string {
	path := fmt.Sprintf("%s/watch/%s", strings.TrimRight(baseURL, "/"), url.PathEscape(slug))
	if episode > 0 {
		path += fmt.Sprintf("/ep-%d", episode)
	}
	return path
}
