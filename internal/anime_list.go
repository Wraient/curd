package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
)

type Anime struct {
	ID               string      `json:"_id"`
	Name             string      `json:"name"`
	AvailableEpisodes interface{} `json:"availableEpisodes"`
}

type Response struct {
	Data struct {
		Shows struct {
			Edges []Anime `json:"edges"`
		} `json:"shows"`
	} `json:"data"`
}

func main() {
	// Get environment variables
	mode := getEnv("ANI_CLI_MODE", "sub")

	// Query for the anime (from a file in this example)
	query := "One piece"

	// Search anime
	animeList := searchAnime(string(query), mode)
	fmt.Println(animeList)
}

// searchAnime performs the API call and fetches anime information
func searchAnime(query, mode string) string {
	const (
		agent         = "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:109.0) Gecko/20100101 Firefox/121.0"
		allanimeRef   = "https://allanime.to"
		allanimeBase  = "allanime.day"
		allanimeAPI   = "https://api." + allanimeBase + "/api"
	)

	searchGql := `query( $search: SearchInput $limit: Int $page: Int $translationType: VaildTranslationTypeEnumType $countryOrigin: VaildCountryOriginEnumType ) { shows( search: $search limit: $limit page: $page translationType: $translationType countryOrigin: $countryOrigin ) { edges { _id name availableEpisodes __typename } } }`

	// Build the request URL
	url := fmt.Sprintf("%s?variables={\"search\":{\"allowAdult\":false,\"allowUnknown\":false,\"query\":\"%s\"},\"limit\":40,\"page\":1,\"translationType\":\"%s\",\"countryOrigin\":\"ALL\"}&query=%s", allanimeAPI, query, mode, searchGql)

	// Make the HTTP request
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		fmt.Println("Error creating HTTP request:", err)
		return ""
	}
	req.Header.Set("User-Agent", agent)
	req.Header.Set("Referer", allanimeRef)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Error making HTTP request:", err)
		return ""
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Error reading response body:", err)
		return ""
	}

	// Parse the JSON response
	var response Response
	err = json.Unmarshal(body, &response)
	if err != nil {
		fmt.Println("Error parsing JSON:", err)
		return ""
	}

	// Format and return the anime list
	var animeList strings.Builder
	for _, anime := range response.Data.Shows.Edges {
		// availableEpisodes, _ := anime.AvailableEpisodes.Int64() // Converts json.Number to int64

		var episodesStr string

		switch v := anime.AvailableEpisodes.(type) {
		case float64:
			episodesStr = fmt.Sprintf("%d", int(v))  // Handle numbers
		case string:
			episodesStr = v                         // Handle string
		case nil:
			episodesStr = "N/A"                     // Handle nulls
		default:
			episodesStr = "Unknown"
		}


		animeList.WriteString(fmt.Sprintf("%s\t%s (%s episodes)\n", anime.ID, anime.Name, episodesStr))

		// In the formatting part
		// animeList.WriteString(fmt.Sprintf("%s\t%s (%d episodes)NEWLINEFROMHERE\n", anime.ID, anime.Name, availableEpisodes))

		// animeList.WriteString(fmt.Sprintf("%s\t%s (%d episodes)NEWLINEFROMHERE\n", anime.ID, anime.Name, anime.AvailableEpisodes))
	}
	return animeList.String()
}

// getEnv returns the value of an environment variable or a default value if it's not set
func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}