package anipm

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

	var payload searchResponse
	if err := fetchJSON(searchURL(query), baseURL+"/", &payload); err != nil {
		return nil, err
	}
	if len(payload.Items) == 0 {
		return nil, fmt.Errorf("no results for %q", query)
	}

	options := make([]providers.SelectionOption, 0, len(payload.Items))
	for _, item := range payload.Items {
		if item.ID <= 0 {
			continue
		}
		title := strings.TrimSpace(item.Title)
		if title == "" {
			title = strings.TrimSpace(item.Native)
		}
		label := title
		if item.Type != "" {
			label += " · " + item.Type
		}
		if item.Year > 0 {
			label += " · " + strconv.Itoa(item.Year)
		}
		if item.EpisodeCount > 0 {
			label += fmt.Sprintf(" · %d eps", item.EpisodeCount)
		}
		key := strings.TrimSpace(item.RouteID)
		if key == "" {
			key = strconv.Itoa(item.ID)
		}
		options = append(options, providers.SelectionOption{
			Key:       key,
			Label:     label,
			Title:     title,
			Thumbnail: absoluteURL(item.Poster),
			ExtraData: SearchItem{
				ID:       item.ID,
				MalID:    item.MalID,
				RouteID:  item.RouteID,
				Title:    title,
				Episodes: item.EpisodeCount,
				SubCount: item.SubCount,
				DubCount: item.DubCount,
			},
		})
	}
	if len(options) == 0 {
		return nil, fmt.Errorf("no results for %q", query)
	}
	return options, nil
}

func absoluteURL(path string) string {
	path = strings.TrimSpace(path)
	if path == "" || strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return baseURL + path
}
