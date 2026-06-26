package anipub

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/wraient/curd/internal/providers"
)

func searchAnime(query, mode string) ([]providers.SelectionOption, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("empty search query")
	}
	_ = mode

	results, err := fetchSearchResults(query)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no results for %q", query)
	}

	infos := fetchSearchInfos(results)
	options := make([]providers.SelectionOption, 0, len(results))
	for i, item := range results {
		if item.ID <= 0 {
			continue
		}
		info := infos[i]
		label := formatSearchLabel(item, info)
		extra := toSearchItem(item, info)
		options = append(options, providers.SelectionOption{
			Key:       strconv.Itoa(item.ID),
			Label:     label,
			Title:     strings.TrimSpace(item.Name),
			Thumbnail: thumbnailFromInfo(info, item.Image),
			ExtraData: extra,
		})
	}
	if len(options) == 0 {
		return nil, fmt.Errorf("no results for %q", query)
	}
	return options, nil
}

func fetchSearchResults(query string) ([]searchResult, error) {
	raw, err := fetchString(searchURL(query), baseURL+"/")
	if err != nil {
		return nil, err
	}
	return decodeSearchResults([]byte(raw))
}

func decodeSearchResults(raw []byte) ([]searchResult, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil, fmt.Errorf("empty search response")
	}

	var notFound struct {
		Found bool `json:"found"`
	}
	if err := json.Unmarshal(raw, &notFound); err == nil && strings.Contains(string(raw), `"found"`) && !notFound.Found {
		return nil, nil
	}

	var results []searchResult
	if err := json.Unmarshal(raw, &results); err == nil {
		return results, nil
	}

	var single searchResult
	if err := json.Unmarshal(raw, &single); err == nil && single.ID > 0 {
		return []searchResult{single}, nil
	}

	return nil, fmt.Errorf("parse anipub search response")
}

func fetchSearchInfos(results []searchResult) []infoResponse {
	infos := make([]infoResponse, len(results))
	var wg sync.WaitGroup
	wg.Add(len(results))
	for i, item := range results {
		i, item := i, item
		go func() {
			defer wg.Done()
			if item.ID <= 0 {
				return
			}
			var info infoResponse
			if err := fetchJSON(infoURL(strconv.Itoa(item.ID)), baseURL+"/", &info); err == nil {
				infos[i] = info
			}
		}()
	}
	wg.Wait()
	return infos
}

func formatSearchLabel(item searchResult, info infoResponse) string {
	parts := []string{strings.TrimSpace(item.Name)}
	if info.EpCount > 0 {
		parts = append(parts, fmt.Sprintf("%d eps", info.EpCount))
	}
	return strings.Join(parts, " · ")
}

func toSearchItem(item searchResult, info infoResponse) SearchItem {
	malID, _ := strconv.Atoi(strings.TrimSpace(info.MALID))
	episodes := info.EpCount
	if episodes <= 0 && info.ID > 0 {
		episodes = info.EpCount
	}
	return SearchItem{
		ID:       item.ID,
		MalID:    malID,
		Name:     strings.TrimSpace(item.Name),
		Finder:   strings.TrimSpace(item.Finder),
		Episodes: episodes,
	}
}
