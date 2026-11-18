package animex

import (
	"fmt"
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

	items, err := searchCatalog(query)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("no results for %q", query)
	}

	options := make([]providers.SelectionOption, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.ID) == "" {
			continue
		}
		title := strings.TrimSpace(item.English)
		if title == "" {
			title = strings.TrimSpace(item.Romaji)
		}
		if title == "" {
			title = strings.TrimSpace(item.ID)
		}
		label := title
		if strings.TrimSpace(item.Format) != "" {
			label += " · " + strings.TrimSpace(item.Format)
		}
		if item.SeasonYear > 0 {
			label += " · " + strconv.Itoa(item.SeasonYear)
		}
		if item.EpisodeCnt > 0 {
			label += fmt.Sprintf(" · %d eps", item.EpisodeCnt)
		}
		options = append(options, providers.SelectionOption{
			Key:       strings.TrimSpace(item.ID),
			Label:     label,
			Title:     title,
			Thumbnail: item.coverURL(),
			ExtraData: SearchItem{
				ID:        strings.TrimSpace(item.ID),
				AnilistID: item.AnilistID,
				MalID:     item.MalID,
				Title:     title,
				Romaji:    strings.TrimSpace(item.Romaji),
				Type:      strings.TrimSpace(item.Format),
				Episodes:  item.EpisodeCnt,
				Year:      item.SeasonYear,
			},
		})
	}
	if len(options) == 0 {
		return nil, fmt.Errorf("no results for %q", query)
	}
	return options, nil
}
