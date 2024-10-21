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
		return fmt.Errorf("error fetching data from Jikan (MyAnimeList) API: %w", err)
	}

	// Check if the 'data' field exists and is valid
	data, ok := response["data"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid response structure: missing or invalid 'data' field")
	}

	// Unmarshal the data into the Anime struct
	anime.Ep.Title.Romaji = data["title_romanji"].(string)
	anime.Ep.Title.English = data["title"].(string)
	anime.Ep.Title.Japanese = data["title_japanese"].(string)
	anime.Ep.Aired = data["aired"].(string)
	anime.Ep.Duration = int(data["duration"].(float64))
	anime.Ep.IsFiller = data["filler"].(bool)
	anime.Ep.IsRecap = data["recap"].(bool)
	anime.Ep.Synopsis = data["synopsis"].(string)
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