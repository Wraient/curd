package anilight

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

	var results []searchItem
	if err := fetchJSON(searchURL(query), referrer, &results); err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no results for %q", query)
	}

	options := make([]providers.SelectionOption, 0, len(results))
	for _, item := range results {
		if item.ID <= 0 && strings.TrimSpace(item.Slug) == "" {
			continue
		}
		title := strings.TrimSpace(item.Title.English)
		if title == "" {
			title = strings.TrimSpace(item.Title.Romaji)
		}
		if title == "" {
			title = strings.TrimSpace(item.Slug)
		}
		label := title
		if strings.TrimSpace(item.Format) != "" {
			label += " · " + strings.TrimSpace(item.Format)
		}
		if item.SeasonYear > 0 {
			label += " · " + strconv.Itoa(item.SeasonYear)
		}
		if item.Episodes > 0 {
			label += fmt.Sprintf(" · %d eps", item.Episodes)
		}
		// The search payload already carries per-mode episode totals, so
		// remember them: a following EpisodesList for this slug can skip
		// its detail request. Same values, zero extra round trips.
		if slug := strings.TrimSpace(item.Slug); slug != "" {
			storeEpisodeCounts(slug, item.SubEpisodes, item.DubEpisodes)
		}
		options = append(options, providers.SelectionOption{
			Key:       strings.TrimSpace(item.Slug),
			Label:     label,
			Title:     title,
			Thumbnail: strings.TrimSpace(item.Cover.Large),
			ExtraData: SearchItem{
				ID:       item.ID,
				MalID:    item.MalID,
				Slug:     strings.TrimSpace(item.Slug),
				Title:    title,
				Romaji:   strings.TrimSpace(item.Title.Romaji),
				Native:   strings.TrimSpace(item.Title.Native),
				Type:     strings.TrimSpace(item.Format),
				Episodes: item.Episodes,
				SubCount: item.SubEpisodes,
				DubCount: item.DubEpisodes,
			},
		})
	}
	if len(options) == 0 {
		return nil, fmt.Errorf("no results for %q", query)
	}
	return options, nil
}
