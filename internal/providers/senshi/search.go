package senshi

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/wraient/curd/internal/providers"
)

func searchAnime(query, mode string) ([]providers.SelectionOption, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("empty search query")
	}
	_ = mode

	var payload filterResponse
	err := fetchJSON(http.MethodPost, baseURL+"/anime/filter", map[string]any{
		"searchTerm": query,
		"page":       1,
		"limit":      25,
	}, &payload)
	if err != nil {
		return nil, err
	}
	if len(payload.Data) == 0 {
		return nil, fmt.Errorf("no results for %q", query)
	}

	options := make([]providers.SelectionOption, 0, len(payload.Data))
	for _, item := range payload.Data {
		malID := item.ID
		if malID <= 0 {
			continue
		}
		title := strings.TrimSpace(item.TitleEnglish)
		if title == "" {
			title = strings.TrimSpace(item.Title)
		}
		label := formatSearchLabel(item)
		options = append(options, providers.SelectionOption{
			Key:       strconv.Itoa(malID),
			Label:     label,
			Title:     title,
			Thumbnail: posterURL(malID),
			ExtraData: toSearchItem(item),
		})
	}
	if len(options) == 0 {
		return nil, fmt.Errorf("no results for %q", query)
	}
	return options, nil
}

func formatSearchLabel(item animeItem) string {
	title := strings.TrimSpace(item.TitleEnglish)
	if title == "" {
		title = strings.TrimSpace(item.Title)
	}
	parts := []string{title}
	if item.Type != "" {
		parts = append(parts, item.Type)
	}
	if item.AniYear > 0 {
		parts = append(parts, strconv.Itoa(item.AniYear))
	}
	if episodes, ok := parseEpisodeCount(item.AniEpisodes); ok {
		parts = append(parts, fmt.Sprintf("%d eps", episodes))
	}
	return strings.Join(parts, " · ")
}

func toSearchItem(item animeItem) SearchItem {
	episodes, _ := parseEpisodeCount(item.AniEpisodes)
	return SearchItem{
		MalID:    item.ID,
		PublicID: item.PublicID,
		Title:    item.Title,
		Type:     item.Type,
		Episodes: episodes,
		Year:     item.AniYear,
		Score:    item.Score,
		Status:   item.AniStatus,
	}
}

func parseEpisodeCount(raw string) (int, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "?" {
		return 0, false
	}
	count, err := strconv.Atoi(raw)
	if err != nil || count <= 0 {
		return 0, false
	}
	return count, true
}
