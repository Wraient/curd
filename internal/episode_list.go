package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

type EpisodesResponse struct {
	Data struct {
		Show struct {
			ID                   string                 `json:"_id"`
			AvailableEpisodesDetail map[string]interface{} `json:"availableEpisodesDetail"`
		} `json:"show"`
	} `json:"data"`
}

func main() {
	// Get environment variables
	// Read the ID from the file
	id := "ReooPAxPMsHM4KPMY"

	// Fetch episodes list
	episodeList := episodesList(string(id), "sub")

	// Write the episode list to a file
	fmt.Println(episodeList)
}

// episodesList performs the API call and fetches the episodes list
func episodesList(showID, mode string) string {
	const (
		agent         = "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:109.0) Gecko/20100101 Firefox/121.0"
		allanimeRef   = "https://allanime.to"
		allanimeBase  = "allanime.day"
		allanimeAPI   = "https://api." + allanimeBase + "/api"
	)

	episodesListGql := `query ($showId: String!) { show( _id: $showId ) { _id availableEpisodesDetail }}`
	
	// Build the request URL
	url := fmt.Sprintf("%s?variables={\"showId\":\"%s\"}&query=%s", allanimeAPI, showID, episodesListGql)

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
	var response EpisodesResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		fmt.Println("Error parsing JSON:", err)
		return ""
	}

	// Extract and sort the episodes
	episodes := extractEpisodes(response.Data.Show.AvailableEpisodesDetail, mode)
	return strings.Join(episodes, "\n")
}

// extractEpisodes extracts the episodes list from the availableEpisodesDetail field
func extractEpisodes(availableEpisodesDetail map[string]interface{}, mode string) []string {
	var episodes []float64

	// Check if the mode (e.g., "sub") exists in the map
	if eps, ok := availableEpisodesDetail[mode].([]interface{}); ok {
		for _, ep := range eps {
			if epNum, err := strconv.ParseFloat(fmt.Sprintf("%v", ep), 64); err == nil {
				episodes = append(episodes, epNum)
			}
		}
	}

	// Sort episodes numerically
	sort.Float64s(episodes)

	// Convert to string and return
	var episodesStr []string
	for _, ep := range episodes {
		episodesStr = append(episodesStr, fmt.Sprintf("%.1f", ep))
	}
	return episodesStr
}
