package internal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type anime struct {
	ID                string      `json:"_id"`
	Name              string      `json:"name"`
	EnglishName       string      `json:"englishName"`
	Thumbnail         string      `json:"thumbnail"`
	AvailableEpisodes interface{} `json:"availableEpisodes"`
}

type response struct {
	Data struct {
		Shows struct {
			Edges []anime `json:"edges"`
		} `json:"shows"`
	} `json:"data"`
}

func normalizeTranslationType(mode string) string {
	if strings.EqualFold(strings.TrimSpace(mode), "dub") {
		return "dub"
	}
	return "sub"
}

func alternateTranslationType(mode string) string {
	if normalizeTranslationType(mode) == "dub" {
		return "sub"
	}
	return "dub"
}

// func main() {
// 	// Get environment variables
// 	mode := "sub"

// 	// Query for the anime (from a file in this example)
// 	query := "one piece"

// 	// Search anime
// 	animeList, err := SearchAnime(string(query), mode)
// 	if err != nil {

// 	}
// 	fmt.Println(animeList)
// }

func SearchAnime(query, mode string) ([]SelectionOption, error) {
	preferredMode := normalizeTranslationType(mode)
	alternateMode := alternateTranslationType(preferredMode)

	preferredResults, preferredErr := searchAnimeByMode(query, preferredMode, preferredMode)
	alternateResults, alternateErr := searchAnimeByMode(query, alternateMode, preferredMode)

	if preferredErr != nil {
		Log(fmt.Sprintf("Failed searching %s results for %q: %v", preferredMode, query, preferredErr))
	}
	if alternateErr != nil {
		Log(fmt.Sprintf("Failed searching %s results for %q: %v", alternateMode, query, alternateErr))
	}

	if preferredErr != nil && alternateErr != nil {
		return nil, preferredErr
	}

	animeList := make([]SelectionOption, 0, len(preferredResults)+len(alternateResults))
	seen := make(map[string]struct{}, len(preferredResults)+len(alternateResults))

	for _, option := range preferredResults {
		animeList = append(animeList, option)
		seen[option.Key] = struct{}{}
	}

	for _, option := range alternateResults {
		if _, exists := seen[option.Key]; exists {
			continue
		}
		animeList = append(animeList, option)
	}

	return animeList, nil
}

func searchAnimeByMode(query, mode, preferredMode string) ([]SelectionOption, error) {
	userCurdConfig := GetGlobalConfig()
	if userCurdConfig == nil {
		logFile = os.ExpandEnv("$HOME/.local/share/curd/debug.log")
	} else {
		logFile = filepath.Join(os.ExpandEnv(userCurdConfig.StoragePath), "debug.log")
	}
	const (
		agent        = "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:109.0) Gecko/20100101 Firefox/121.0"
		allanimeRef  = "https://allanime.to"
		allanimeBase = "allanime.day"
		allanimeAPI  = "https://api." + allanimeBase + "/api"
	)

	mode = normalizeTranslationType(mode)
	preferredMode = normalizeTranslationType(preferredMode)

	animeList := make([]SelectionOption, 0)

	searchGql := `query($search: SearchInput, $limit: Int, $page: Int, $translationType: VaildTranslationTypeEnumType, $countryOrigin: VaildCountryOriginEnumType) {
		shows(search: $search, limit: $limit, page: $page, translationType: $translationType, countryOrigin: $countryOrigin) {
			edges {
				_id
				name
				englishName
				thumbnail
				availableEpisodes
				__typename
			}
		}
	}`

	// Prepare the GraphQL variables
	variables := map[string]interface{}{
		"search": map[string]interface{}{
			"allowAdult":   false,
			"allowUnknown": false,
			"query":        query,
		},
		"limit":           40,
		"page":            1,
		"translationType": mode,
		"countryOrigin":   "ALL",
	}

	// Build POST request body
	requestBody, err := json.Marshal(map[string]interface{}{
		"query":     searchGql,
		"variables": variables,
	})
	if err != nil {
		Log(fmt.Sprintf("Error encoding request body to JSON: %v", err))
		return animeList, err
	}

	// Make the HTTP POST request
	req, err := http.NewRequest("POST", allanimeAPI, bytes.NewBuffer(requestBody))
	if err != nil {
		Log(fmt.Sprintf("Error creating HTTP request: %v", err))
		return animeList, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", agent)
	req.Header.Set("Referer", allanimeRef)
	req.Header.Set("Origin", allanimeRef)

	resp, err := sharedHTTPClient.Do(req)
	if err != nil {
		Log(fmt.Sprintf("Error making HTTP request: %v", err))
		return animeList, err
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		Log(fmt.Sprintf("Error reading response body: %v", err))
		return animeList, err
	}

	// Debug: Log the response status and first part of the body
	Log(fmt.Sprintf("Response Status: %s", resp.Status))
	Log(fmt.Sprintf("Response Body (first 500 chars): %s", string(body[:min(len(body), 500)])))

	// Parse the JSON response
	var response response
	err = json.Unmarshal(body, &response)
	if err != nil {
		Log(fmt.Sprintf("Error parsing JSON for query '%s': %v\nBody: %s", query, err, string(body)))
		return animeList, err
	}

	for _, anime := range response.Data.Shows.Edges {
		var episodesStr string
		if episodes, ok := anime.AvailableEpisodes.(map[string]interface{}); ok {
			if modeEpisodes, ok := episodes[mode].(float64); ok {
				episodesStr = fmt.Sprintf("%d", int(modeEpisodes))
			} else {
				episodesStr = "Unknown"
			}
		} else {
			episodesStr = "Unknown"
		}

		// Use English name if available and configured, otherwise use default name
		displayName := anime.Name
		if anime.EnglishName != "" && userCurdConfig != nil && userCurdConfig.AnimeNameLanguage == "english" {
			displayName = anime.EnglishName
		}

		label := fmt.Sprintf("%s (%s episodes)", displayName, episodesStr)
		if mode != preferredMode {
			label = fmt.Sprintf("%s [%s]", label, mode)
		}

		animeList = append(animeList, SelectionOption{
			Title:     displayName,
			Key:       anime.ID,
			Label:     label,
			Thumbnail: anime.Thumbnail,
		})
	}
	return animeList, nil
}

// Helper function
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
