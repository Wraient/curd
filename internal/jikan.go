package internal

import (
	"encoding/json"
	"fmt"
	"net/http"
	"io"
)

// GetEpisodeData fetches episode data for a given anime ID and episode number
func GetEpisodeData(animeID int, episodeNo int, anime *Anime) error {
	url := fmt.Sprintf("https://api.jikan.moe/v4/anime/%d/episodes/%d", animeID, episodeNo)

	// Use the helper function for making the GET request
	response, err := makeGetRequest(url, nil)
	if err != nil {
		Log(fmt.Sprintf("Warning: Jikan API error: %v - continuing without filler data", err), logFile)
		// Set default values when API fails
		anime.Ep.IsFiller = false
		anime.Ep.IsRecap = false
		return nil // Return nil to allow the application to continue
	}

	Log(response, logFile)

	// Check if the 'data' field exists and is valid
	data, ok := response["data"].(map[string]interface{})
	if !ok {
		Log("Warning: Invalid Jikan API response - continuing without filler data", logFile)
		// Set default values when response is invalid
		anime.Ep.IsFiller = false
		anime.Ep.IsRecap = false
		return nil // Return nil to allow the application to continue
	}
	// Helper function to safely get string value
	getStringValue := func(field string) string {
		if value, ok := data[field].(string); ok {
			return value
		}
		return ""
	}

	// Helper function to safely get int value
	getIntValue := func(field string) int {
		if value, ok := data[field].(float64); ok {
			return int(value)
		}
		return 0
	}

	// Helper function to safely get bool value
	getBoolValue := func(field string) bool {
		if value, ok := data[field].(bool); ok {
			return value
		}
		return false
	}

	// Safely assign values to the Anime struct
	anime.Ep.Title.Romaji = getStringValue("title_romanji")
	anime.Ep.Title.English = getStringValue("title")
	anime.Ep.Title.Japanese = getStringValue("title_japanese")
	anime.Ep.Aired = getStringValue("aired")
	anime.Ep.Duration = getIntValue("duration")
	anime.Ep.IsFiller = getBoolValue("filler")
	anime.Ep.IsRecap = getBoolValue("recap")
	anime.Ep.Synopsis = getStringValue("synopsis")

	return nil
}

// Helper function to make GET requests
func makeGetRequest(url string, headers map[string]string) (map[string]interface{}, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create GET request: %w", err)
	}

	for key, value := range headers {
		req.Header.Set(key, value)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send GET request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed with status %d: %s", resp.StatusCode, body)
	}

	var responseData map[string]interface{}
	err = json.Unmarshal(body, &responseData)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return responseData, nil
}