package internal

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	// "strings"
)

type anime struct {
	ID               string      `json:"_id"`
	Name             string      `json:"name"`
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

// searchAnime performs the API call and fetches anime information
func SearchAnime(query, mode string) (map[string]string, error) {
	userCurdConfig := GetGlobalConfig()
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

	// Format and return the anime list
	animeList := make(map[string]string)
	
	searchGql := `query( $search: SearchInput $limit: Int $page: Int $translationType: VaildTranslationTypeEnumType $countryOrigin: VaildCountryOriginEnumType ) { shows( search: $search limit: $limit page: $page translationType: $translationType countryOrigin: $countryOrigin ) { edges { _id name availableEpisodes __typename } } }`

	// Build the request URL
	url := fmt.Sprintf("%s?variables={\"search\":{\"allowAdult\":false,\"allowUnknown\":false,\"query\":\"%s\"},\"limit\":40,\"page\":1,\"translationType\":\"%s\",\"countryOrigin\":\"ALL\"}&query=%s", allanimeAPI, query, mode, searchGql)

	// Make the HTTP request
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		Log(fmt.Sprint("Error creating HTTP request:", err), logFile)
		return animeList, err
	}
	req.Header.Set("User-Agent", agent)
	req.Header.Set("Referer", allanimeRef)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		Log(fmt.Sprint("Error making HTTP request:", err), logFile)
		return animeList, err
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		Log(fmt.Sprint("Error reading response body:", err), logFile)
		return animeList, err
	}

	// Parse the JSON response
	var response response
	err = json.Unmarshal(body, &response)
	if err != nil {
		Log(fmt.Sprint("Error parsing JSON:", err), logFile)
		return animeList, err
	}

	for _, anime := range response.Data.Shows.Edges {
		// availableEpisodes, _ := anime.AvailableEpisodes.Int64() // Converts json.Number to int64

		var episodesStr string

		// Log(anime.AvailableEpisodes, logFile)
		// episodesStr = anime.AvailableEpisodes.sub

		//anime = {"dub":0,"raw":0,"sub":4}

		if episodes, ok := anime.AvailableEpisodes.(map[string]interface{}); ok {
			// Log(episodes, logFile)
			if subEpisodes, ok := episodes["sub"].(float64); ok {
				episodesStr = fmt.Sprintf("%d", int(subEpisodes))
			} else {
				Log(subEpisodes, logFile)
				episodesStr = "Unknown"
			}
		}

		animeList[anime.ID] = fmt.Sprintf("%s (%s episodes)", anime.Name, episodesStr)
		// animeList.WriteString(fmt.Sprintf("%s\t%s (%s episodes)\n", anime.ID, anime.Name, episodesStr))
	}
	return animeList, nil
}
