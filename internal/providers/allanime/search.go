package allanime

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/wraient/curd/internal/curdhost"
	"github.com/wraient/curd/internal/providers"
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

func searchAllAnime(query, mode string) ([]providers.SelectionOption, error) {
	preferredMode := providers.NormalizeTranslationType(mode)
	return searchAnimeByMode(query, preferredMode, preferredMode)
}

func searchAnimeByMode(query, mode, preferredMode string) ([]providers.SelectionOption, error) {
	const (
		agent        = "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:109.0) Gecko/20100101 Firefox/121.0"
		allanimeRef  = "https://allanime.to"
		allanimeBase = "allanime.day"
		allanimeAPI  = "https://api." + allanimeBase + "/api"
	)

	mode = providers.NormalizeTranslationType(mode)
	preferredMode = providers.NormalizeTranslationType(preferredMode)

	animeList := make([]providers.SelectionOption, 0)

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
		curdhost.Log(fmt.Sprintf("Error encoding request body to JSON: %v", err))
		return animeList, err
	}

	// Make the HTTP POST request
	req, err := http.NewRequest("POST", allanimeAPI, bytes.NewBuffer(requestBody))
	if err != nil {
		curdhost.Log(fmt.Sprintf("Error creating HTTP request: %v", err))
		return animeList, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", agent)
	req.Header.Set("Referer", allanimeRef)
	req.Header.Set("Origin", allanimeRef)

	resp, err := curdhost.HTTPClient().Do(req)
	if err != nil {
		curdhost.Log(fmt.Sprintf("Error making HTTP request: %v", err))
		return animeList, err
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		curdhost.Log(fmt.Sprintf("Error reading response body: %v", err))
		return animeList, err
	}

	// Debug: Log the response status and first part of the body
	curdhost.Log(fmt.Sprintf("Response Status: %s", resp.Status))
	curdhost.Log(fmt.Sprintf("Response Body (first 500 chars): %s", string(body[:min(len(body), 500)])))
	if !curdhost.HTTPStatusOK(resp.StatusCode) {
		err := curdhost.HTTPStatusError("allanime search", resp.StatusCode, body)
		curdhost.Log(err.Error())
		return animeList, err
	}

	// Parse the JSON response
	var response response
	err = json.Unmarshal(body, &response)
	if err != nil {
		curdhost.Log(fmt.Sprintf("Error parsing JSON for query '%s': %v\nBody: %s", query, err, string(body)))
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
		if anime.EnglishName != "" && curdhost.AnimeNameLanguage != nil && curdhost.AnimeNameLanguage() == "english" {
			displayName = anime.EnglishName
		}

		label := fmt.Sprintf("%s (%s episodes)", displayName, episodesStr)
		if mode != preferredMode {
			label = fmt.Sprintf("%s [%s]", label, mode)
		}

		animeList = append(animeList, providers.SelectionOption{
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
