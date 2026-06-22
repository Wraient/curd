package anineko

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/wraient/curd/internal/providers"
)

type searchResponse struct {
	Success bool `json:"success"`
	Results []struct {
		Title string `json:"title"`
		URL   string `json:"url"`
		Image string `json:"image"`
		Meta  string `json:"meta"`
	} `json:"results"`
}

var slugFromPathRE = regexp.MustCompile(`/watch/([^/?#]+)`)

func searchAnime(query, mode string) ([]providers.SelectionOption, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("empty search query")
	}

	rawURL := fmt.Sprintf("%s/ajax/search?q=%s", baseURL, url.QueryEscape(query))
	body, err := fetchString(rawURL, baseURL+"/")
	if err != nil {
		return nil, err
	}

	var payload searchResponse
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		return nil, fmt.Errorf("parse anineko search response: %w", err)
	}
	if !payload.Success {
		return nil, fmt.Errorf("anineko search failed")
	}

	options := make([]providers.SelectionOption, 0, len(payload.Results))
	for _, result := range payload.Results {
		slug := slugFromWatchURL(result.URL)
		if slug == "" {
			continue
		}
		label := strings.TrimSpace(result.Title)
		if meta := strings.TrimSpace(result.Meta); meta != "" {
			label = label + " — " + meta
		}
		options = append(options, providers.SelectionOption{
			Key:       slug,
			Label:     label,
			Title:     strings.TrimSpace(result.Title),
			Thumbnail: absoluteURL(result.Image),
		})
	}
	if len(options) == 0 {
		return nil, fmt.Errorf("no results for %q", query)
	}
	return options, nil
}

func slugFromWatchURL(watchPath string) string {
	match := slugFromPathRE.FindStringSubmatch(watchPath)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}
