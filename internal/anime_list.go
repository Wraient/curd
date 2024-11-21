package internal

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"net/url"
	// "strings"
)

type anime struct {
	ID               string      `json:"_id"`
	Name             string      `json:"name"`
	EnglishName      string      `json:"englishName"`
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

func SearchAnime(query, mode string) (map[string]string, error) {
	userCurdConfig := GetGlobalConfig()
	var logFile string
	if userCurdConfig == nil {
		logFile = os.ExpandEnv("$HOME/.local/share/curd/debug.log")
	} else {
		logFile = filepath.Join(os.ExpandEnv(userCurdConfig.StoragePath), "debug.log")
	}
	const (
		agent         = "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:109.0) Gecko/20100101 Firefox/121.0"
		allanimeRef   = "https://allanime.to"
		allanimeBase  = "allanime.day"
		allanimeAPI   = "https://api." + allanimeBase + "/api"
	)

	// Prepare the anime list
	animeList := make(map[string]string)
	
	searchGql := `query($search: SearchInput, $limit: Int, $page: Int, $translationType: VaildTranslationTypeEnumType, $countryOrigin: VaildCountryOriginEnumType) {
		shows(search: $search, limit: $limit, page: $page, translationType: $translationType, countryOrigin: $countryOrigin) {
			edges {
				_id
				name
				englishName
				availableEpisodes
				__typename
			}
		}
	}`

	// Prepare the GraphQL variables
	variables := map[string]interface{}{
		"search": map[string]interface{}{
			"allowAdult":    false,
			"allowUnknown":  false,
			"query":         query,
		},
		"limit":          40,
		"page":           1,
		"translationType": mode,
		"countryOrigin":   "ALL",
	}

	// Marshal the variables to JSON
	variablesJSON, err := json.Marshal(variables)
	if err != nil {
		Log(fmt.Sprintf("Error encoding variables to JSON: %v", err), logFile)
		return animeList, err
	}

	// Build the request URL
	url := fmt.Sprintf("%s?variables=%s&query=%s", allanimeAPI, url.QueryEscape(string(variablesJSON)), url.QueryEscape(searchGql))

	// Make the HTTP request
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		Log(fmt.Sprintf("Error creating HTTP request: %v", err), logFile)
		return animeList, err
	}
	req.Header.Set("User-Agent", agent)
	req.Header.Set("Referer", allanimeRef)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		Log(fmt.Sprintf("Error making HTTP request: %v", err), logFile)
		return animeList, err
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		Log(fmt.Sprintf("Error reading response body: %v", err), logFile)
		return animeList, err
	}

	// Debug: Log the response status and first part of the body
	Log(fmt.Sprintf("Response Status: %s", resp.Status), logFile)
	Log(fmt.Sprintf("Response Body (first 500 chars): %s", string(body[:min(len(body), 500)])), logFile)

	// Parse the JSON response
	var response response
	err = json.Unmarshal(body, &response)
	if err != nil {
		Log(fmt.Sprintf("Error parsing JSON for query '%s': %v\nBody: %s", query, err, string(body)), logFile)
		return animeList, err
	}

	for _, anime := range response.Data.Shows.Edges {
		var episodesStr string
		if episodes, ok := anime.AvailableEpisodes.(map[string]interface{}); ok {
			if subEpisodes, ok := episodes["sub"].(float64); ok {
				episodesStr = fmt.Sprintf("%d", int(subEpisodes))
			} else {
				Log(subEpisodes, logFile)
				episodesStr = "Unknown"
			}
		}
		
		// Use English name if available and configured, otherwise use default name
		displayName := anime.Name
		if anime.EnglishName != "" && userCurdConfig.AnimeNameLanguage == "english" {
			displayName = anime.EnglishName
		}
		
		animeList[anime.ID] = fmt.Sprintf("%s (%s episodes)", displayName, episodesStr)
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
