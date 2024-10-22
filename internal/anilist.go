package internal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
)

// FindKeyByValue searches for a key associated with a given value in a map[string]string
func FindKeyByValue(m map[string]string, value string) (string, error) {
	for key, val := range m {
		if val == value {
			return key, nil // Return the key and true if the value is found
		}
	}
	return "", fmt.Errorf("No key with value %v", value) // Return empty string and false if the value is not found
}

// GetAnimeMap takes an AnimeList and returns a map with media.id as key and media.title.english as value.
func GetAnimeMap(animeList AnimeList) map[string]string {
	animeMap := make(map[string]string)

	// Helper function to populate the map from a slice of entries
	populateMap := func(entries []Entry) {
		for _, entry := range entries {
			// Only include entries with a non-empty English title
			if entry.Media.Title.English != "" {
				animeMap[strconv.Itoa(entry.Media.ID)] = entry.Media.Title.English
			} else {
				animeMap[strconv.Itoa(entry.Media.ID)] = entry.Media.Title.Romaji
			}
		}
	}

	// Populate the map for each category
	populateMap(animeList.Watching)
	populateMap(animeList.Completed)
	populateMap(animeList.Paused)
	populateMap(animeList.Dropped)
	populateMap(animeList.Planning)

	return animeMap
}
// SearchAnimeAnilist sends the query to AniList and returns a map of title to ID
func SearchAnimeAnilist(query, token string) (map[string]string, error) {
	url := "https://graphql.anilist.co"

	queryString := `
	query ($search: String) {
		Page(page: 1, perPage: 10) {
			media(search: $search, type: ANIME) {
				id
				title {
					romaji
					english
					native
				}
			}
		}
	}`

	variables := map[string]string{"search": query}
	requestBody, err := json.Marshal(map[string]interface{}{
		"query":     queryString,
		"variables": variables,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create new request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to search for anime. Status Code: %d, Response: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var responseData map[string]ResponseData
	err = json.Unmarshal(body, &responseData)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	animeList := responseData["data"].Page.Media
	animeDict := make(map[string]string)

	// Map titles to their IDs as strings
	for _, anime := range animeList {
		idStr := strconv.Itoa(anime.ID) // Convert ID to string
		if anime.Title.English != "" {
			animeDict[idStr] = anime.Title.English
		} else {
			animeDict[idStr] = anime.Title.Romaji
		}
	}

	return animeDict, nil
}

// Function to get AniList user ID and username
func GetAnilistUserID(token string) (int, string, error) {
	url := "https://graphql.anilist.co"
	query := `
	query {
		Viewer {
			id
			name
		}
	}`

	headers := map[string]string{
		"Authorization": "Bearer " + token,
		"Content-Type":  "application/json",
		"Accept":        "application/json",
	}

	response, err := makePostRequest(url, query, nil, headers)
	if err != nil {
		return 0, "", err
	}

	data := response["data"].(map[string]interface{})["Viewer"].(map[string]interface{})
	userID := int(data["id"].(float64))
	userName := data["name"].(string)

	return userID, userName, nil
}

// Function to add an anime to the watching list
func AddAnimeToWatchingList(animeID int, token string) error {
	url := "https://graphql.anilist.co"
	mutation := `
	mutation ($mediaId: Int) {
		SaveMediaListEntry (mediaId: $mediaId, status: CURRENT) {
			id
			status
		}
	}`

	variables := map[string]interface{}{
		"mediaId": animeID,
	}

	headers := map[string]string{
		"Authorization": "Bearer " + token,
		"Content-Type":  "application/json",
	}

	_, err := makePostRequest(url, mutation, variables, headers)
	if err != nil {
		return fmt.Errorf("failed to add anime: %w", err)
	}

	fmt.Printf("Anime with ID %d has been added to your watching list.\n", animeID)
	return nil
}

// Function to get MAL ID using AniList media ID
func GetAnimeMalID(anilistMediaID int) (int, error) {
	url := "https://graphql.anilist.co"
	query := `
	query ($id: Int) {
		Media(id: $id) {
			idMal
		}
	}`

	variables := map[string]interface{}{
		"id": anilistMediaID,
	}

	response, err := makePostRequest(url, query, variables, nil)
	if err != nil {
		return 0, err
	}

	malID := int(response["data"].(map[string]interface{})["Media"].(map[string]interface{})["idMal"].(float64))
	return malID, nil
}

// This function retrieves the MAL ID and cover image URL for an anime from AniList
func GetAnimeIDAndImage(anilistMediaID int) (int, string, error) {
	url := "https://graphql.anilist.co"
	query := `
	query ($id: Int) {
		Media(id: $id) {
			coverImage {
				large
			}
			idMal
		}
	}`

	variables := map[string]interface{}{
		"id": anilistMediaID,
	}

	response, err := makePostRequest(url, query, variables, nil)
	if err != nil {
		return 0, "", err
	}

	data := response["data"].(map[string]interface{})["Media"].(map[string]interface{})
	malID := int(data["idMal"].(float64))
	imageURL := data["coverImage"].(map[string]interface{})["large"].(string)

	return malID, imageURL, nil
}

// Function to get user data from AniList
func GetUserData(token string, userID int) (map[string]interface{}, error) {
	query := fmt.Sprintf(`
	{
		MediaListCollection(userId: %d, type: ANIME) {
			lists {
				entries {
					media {
						id
						episodes
						duration
						title {
							romaji
							english
							native
						}
					}
					status
					score
					progress
				}
			}
		}
	}`, userID)

	headers := map[string]string{
		"Authorization": "Bearer " + token,
		"Content-Type":  "application/json",
	}

	response, err := makePostRequest("https://graphql.anilist.co", query, nil, headers)
	if err != nil {
		return nil, err
	}

	return response, nil
}

// Function to load a JSON file
func LoadJSONFile(filePath string) (map[string]interface{}, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var jsonData map[string]interface{}
	err = json.Unmarshal(data, &jsonData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	return jsonData, nil
}

// Function to search for an anime by title in user data
func SearchAnimeByTitle(jsonData map[string]interface{}, searchTitle string) []map[string]interface{} {
	results := []map[string]interface{}{}

	if searchTitle == "1P" {
		searchTitle = "ONE PIECE"
	}

	lists := jsonData["data"].(map[string]interface{})["MediaListCollection"].(map[string]interface{})["lists"].([]interface{})
	for _, list := range lists {
		entries := list.(map[string]interface{})["entries"].([]interface{})
		for _, entry := range entries {
			media := entry.(map[string]interface{})["media"].(map[string]interface{})
			romajiTitle := media["title"].(map[string]interface{})["romaji"].(string)
			englishTitle := media["title"].(map[string]interface{})["english"].(string)
			episodes := int(media["episodes"].(float64))
			duration := int(media["duration"].(float64))

			if strings.Contains(strings.ToLower(romajiTitle), strings.ToLower(searchTitle)) || strings.Contains(strings.ToLower(englishTitle), strings.ToLower(searchTitle)) {
				result := map[string]interface{}{
					"id":           media["id"],
					"progress":     entry.(map[string]interface{})["progress"],
					"romaji_title": romajiTitle,
					"english_title": englishTitle,
					"episodes":      episodes,
					"duration":      duration,
				}
				results = append(results, result)
			}
		}
	}

	return results
}

// Function to update anime progress
func UpdateAnimeProgress(token string, mediaID, progress int) error {
	url := "https://graphql.anilist.co"
	query := `
	mutation($mediaId: Int, $progress: Int) {
		SaveMediaListEntry(mediaId: $mediaId, progress: $progress) {
			id
			progress
		}
	}`

	variables := map[string]interface{}{
		"mediaId":  mediaID,
		"progress": progress,
	}

	headers := map[string]string{
		"Authorization": "Bearer " + token,
		"Content-Type":  "application/json",
	}

	_, err := makePostRequest(url, query, variables, headers)
	if err != nil {
		return err
	}

	fmt.Println("Anime progress updated! Latest watched episode:", progress)
	return nil
}

// Function to rate an anime on AniList
func RateAnime(token string, mediaID int, score float64) error {
	url := "https://graphql.anilist.co"
	query := `
	mutation($mediaId: Int, $score: Float) {
		SaveMediaListEntry(mediaId: $mediaId, score: $score) {
			id
			mediaId
			score
		}
	}`

	variables := map[string]interface{}{
		"mediaId": mediaID,
		"score":   score,
	}

	headers := map[string]string{
		"Authorization": "Bearer " + token,
		"Content-Type":  "application/json",
	}

	_, err := makePostRequest(url, query, variables, headers)
	if err != nil {
		return err
	}

	fmt.Printf("Successfully rated anime (mediaId: %d) with score: %.2f\n", mediaID, score)
	return nil
}

// Helper function to make POST requests
func makePostRequest(url, query string, variables map[string]interface{}, headers map[string]string) (map[string]interface{}, error) {
	requestBody, err := json.Marshal(map[string]interface{}{
		"query":     query,
		"variables": variables,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")  // <-- Important!
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
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
	// Unmarshal the response into a map
	err = json.Unmarshal(body, &responseData)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return responseData, nil
}

func ParseAnimeList(input map[string]interface{}) AnimeList {
	var animeList AnimeList

	toInt := func(value interface{}) int {
		switch v := value.(type) {
		case int:
			return v
		case float64:
			return int(v) // You could also use int(math.Round(v)) to round
		default:
			return 0 // Default value for unexpected types
		}
	}

	safeString := func(value interface{}) string {
		if value == nil {
			return ""
		}

		// Attempt to assert the value as a string
		if str, ok := value.(string); ok {
			return str
		}

		// If it's not a string, return an empty string or handle it as needed
		return ""
	}

	// Access the list entries in the input map
	if input["data"] == nil {
		Log("Anilist request failed", logFile)
		fmt.Println("Anilist request failed")
		ExitCurd()
		return animeList
	}
	data := input["data"].(map[string]interface{})
	mediaList := data["MediaListCollection"].(map[string]interface{})["lists"].([]interface{})

	for _, list := range mediaList {
		entries := list.(map[string]interface{})["entries"].([]interface{})

		for _, entry := range entries {
			entryData := entry.(map[string]interface{})
			media := entryData["media"].(map[string]interface{})

			animeEntry := Entry{
				Media: Media{
					Duration: toInt(media["duration"]),
					Episodes: toInt(media["episodes"]),
					ID:       toInt(media["id"]),
					Title: AnimeTitle{
						English: safeString(media["title"].(map[string]interface{})["english"]),
						Romaji:  safeString(media["title"].(map[string]interface{})["romaji"]),
						Japanese:  safeString(media["title"].(map[string]interface{})["native"]),
					},
				},
				Progress: toInt(entryData["progress"]),
				Score:    entryData["score"].(float64),
				Status:   safeString(entryData["status"]), // Ensure status is fetched safely
			}

			// Append entries based on their status
			switch animeEntry.Status {
			case "CURRENT":
				animeList.Watching = append(animeList.Watching, animeEntry)
			case "COMPLETED":
				animeList.Completed = append(animeList.Completed, animeEntry) // Fix: Ensure Completed list is used
			case "PAUSED":
				animeList.Paused = append(animeList.Paused, animeEntry) // Fix: Append to Paused list
			case "DROPPED":
				animeList.Dropped = append(animeList.Dropped, animeEntry) // Fix: Append to Dropped list
			case "PLANNING":
				animeList.Planning = append(animeList.Planning, animeEntry) // Fix: Append to Planning list
			}
		}
	}

	return animeList
}

// FindAnimeByID searches for an anime by its ID in the AnimeList
func FindAnimeByAnilistID(list AnimeList, idStr string) (*Entry, error) {
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return nil, fmt.Errorf("invalid ID format: %s", idStr)
	}

	// Define a slice of pointers to hold categories
	categories := [][]Entry{
		list.Watching,
		list.Completed,
		list.Paused,
		list.Dropped,
		list.Planning,
	}

	// Iterate through each category
	for _, category := range categories {
		for _, entry := range category {
			if entry.Media.ID == id {
				return &entry, nil // Return a pointer to the found entry
			}
		}
	}

	return nil, fmt.Errorf("anime with ID %d not found", id) // Return an error if not found
}

// FindAnimeByAnilistIDInAnimes searches for an anime by its AniList ID in a slice of Anime
func FindAnimeByAnilistIDInAnimes(animes []Anime, anilistID int) (*Anime, error) {
	for i := range animes {
		if animes[i].AnilistId == anilistID {
			return &animes[i], nil
		}
	}
	return nil, fmt.Errorf("anime with ID %d not found", anilistID)
}