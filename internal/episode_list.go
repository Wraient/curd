package internal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
)

type episodesResponse struct {
	Data struct {
		Show struct {
			ID                      string                 `json:"_id"`
			AvailableEpisodesDetail map[string]interface{} `json:"availableEpisodesDetail"`
		} `json:"show"`
	} `json:"data"`
}

// func main() {
// 	// Get environment variables
// 	// Read the ID from the file
// 	id := "ReooPAxPMsHM4KPMY"

// 	// Fetch episodes list
// 	episodeList := episodesList(string(id), "sub")

// 	// Write the episode list to a file
// 	fmt.Println(episodeList)
// }

// episodesList performs the API call and fetches the episodes list
func getAllAnimeEpisodesList(showID, mode string) ([]string, error) {
	preferredMode := normalizeTranslationType(mode)

	episodesListGql := `query ($showId String!) { show( _id: $showId ) { _id availableEpisodesDetail }}`

	// Build POST request body
	requestBody, err := json.Marshal(map[string]interface{}{
		"query":     episodesListGql,
		"variables": map[string]string{"showId": showID},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	// Make the HTTP POST request
	req, err := http.NewRequest("POST", "https://api.allanime.day/api", bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://allanime.to")
	req.Header.Set("Origin", "https://allanime.to")

	resp, err := sharedHTTPClient.Do(req)
	if err != nil {
		Log(fmt.Sprint("Error making HTTP request:", err))
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		Log(fmt.Sprint("Error reading response body:", err))
		return nil, err
	}

	// Parse the JSON response
	var response episodesResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		Log(fmt.Sprint("Error parsing JSON:", err))
		return nil, err
	}

	// Extract and sort the episodes
	episodes := extractEpisodes(response.Data.Show.AvailableEpisodesDetail, preferredMode)
	if len(episodes) == 0 {
		fallbackMode := alternateTranslationType(preferredMode)
		episodes = extractEpisodes(response.Data.Show.AvailableEpisodesDetail, fallbackMode)
		if len(episodes) > 0 {
			Log(fmt.Sprintf("Falling back to %s episode list for anime %s", fallbackMode, showID))
		}
	}
	if len(episodes) == 0 {
		return episodes, fmt.Errorf("no episodes found for anime %s", showID)
	}
	return episodes, nil
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
		episodesStr = append(episodesStr, fmt.Sprintf("%v", ep))
	}
	return episodesStr
}
