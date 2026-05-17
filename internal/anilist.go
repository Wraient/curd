package internal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// FindKeyByValue searches for a key associated with a given value in a map[string]string
func FindKeyByValue(m map[string]string, value string) (string, error) {
	for key, val := range m {
		if val == value {
			return key, nil // Return the key and true if the value is found
		}
	}
	return "", fmt.Errorf("no key with value %v", value) // Return empty string and false if the value is not found
}

// GetAnimeMap takes an AnimeList and returns a map with media.id as key and media.title.english as value.
func GetAnimeMap(animeList AnimeList) map[string]string {
	animeMap := make(map[string]string)
	userCurdConfig := GetGlobalConfig()

	// Helper function to populate the map from a slice of entries
	populateMap := func(entries []Entry) {
		for _, entry := range entries {
			// Only include entries with a non-empty English title

			if entry.Media.Title.English != "" && userCurdConfig.AnimeNameLanguage == "english" {
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
	populateMap(animeList.Rewatching) // Add Rewatching list

	return animeMap
}

// GetAnimeMapPreview takes an AnimeList and returns a map with media.id as key and media.title.english as value.
func GetAnimeMapPreview(animeList AnimeList) map[string]RofiSelectPreview {
	userCurdConfig := GetGlobalConfig()
	animeMap := make(map[string]RofiSelectPreview)

	// Helper function to populate the map from a slice of entries
	populateMap := func(entries []Entry) {
		for _, entry := range entries {
			// Only include entries with a non-empty English title
			Log(fmt.Errorf("AnimeNameLanguage: %v", userCurdConfig.AnimeNameLanguage))

			if entry.Media.Title.English != "" && userCurdConfig.AnimeNameLanguage == "english" {
				animeMap[strconv.Itoa(entry.Media.ID)] = RofiSelectPreview{
					Title:      entry.Media.Title.English,
					CoverImage: entry.CoverImage,
				}
			} else {
				animeMap[strconv.Itoa(entry.Media.ID)] = RofiSelectPreview{
					Title:      entry.Media.Title.Romaji,
					CoverImage: entry.CoverImage,
				}
			}
		}
	}

	// Populate the map for each category
	populateMap(animeList.Watching)
	populateMap(animeList.Completed)
	populateMap(animeList.Paused)
	populateMap(animeList.Dropped)
	populateMap(animeList.Planning)
	populateMap(animeList.Rewatching) // Add Rewatching list

	return animeMap
}

// fuzzy matching w/ Levenshtein distance
func levenshtein(a, b string) int {
	a = strings.ToLower(a)
	b = strings.ToLower(b)
	ar, br := []rune(a), []rune(b)
	alen, blen := len(ar), len(br)
	if alen == 0 {
		return blen
	}
	if blen == 0 {
		return alen
	}
	matrix := make([][]int, alen+1)
	for i := range matrix {
		matrix[i] = make([]int, blen+1)
	}
	for i := 0; i <= alen; i++ {
		matrix[i][0] = i
	}
	for j := 0; j <= blen; j++ {
		matrix[0][j] = j
	}
	for i := 1; i <= alen; i++ {
		for j := 1; j <= blen; j++ {
			cost := 0
			if ar[i-1] != br[j-1] {
				cost = 1
			}
			matrix[i][j] = min3(
				matrix[i-1][j]+1,
				matrix[i][j-1]+1,
				matrix[i-1][j-1]+cost,
			)
		}
	}
	return matrix[alen][blen]
}

func min3(a, b, c int) int {
	if a < b && a < c {
		return a
	}
	if b < c {
		return b
	}
	return c
}

func doAniListSearchRequest(url string, requestBody []byte, token string) ([]byte, error) {
	client := &http.Client{}
	for attempt := 0; attempt < 5; attempt++ {
		req, err := http.NewRequest("POST", url, bytes.NewBuffer(requestBody))
		if err != nil {
			return nil, fmt.Errorf("failed to create new request: %w", err)
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to make request: %w", err)
		}

		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("failed to read response body: %w", readErr)
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			if attempt == 4 {
				return nil, fmt.Errorf("failed to search for anime. Status Code: %d, Response: %s", resp.StatusCode, string(body))
			}
			time.Sleep(aniListRetryDelay(resp, attempt))
			continue
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("failed to search for anime. Status Code: %d, Response: %s", resp.StatusCode, string(body))
		}
		return body, nil
	}

	return nil, fmt.Errorf("AniList search failed after retries")
}

// SearchAnimeAnilist sends the query to AniList and returns a map of title to ID
func SearchAnimeAnilistPreview(query, token string) (map[string]RofiSelectPreview, error) {
	url := "https://graphql.anilist.co"

	queryString := `
	query ($search: String) {
		Page(page: 1, perPage: 50) {
			media(search: $search, type: ANIME) {
				id
				title {
					romaji
					english
					native
				}
				coverImage {
					large
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

	body, err := doAniListSearchRequest(url, requestBody, token)
	if err != nil {
		return nil, err
	}

	var responseData map[string]ResponseData
	err = json.Unmarshal(body, &responseData)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	animeList := responseData["data"].Page.Media
	animeDict := make(map[string]RofiSelectPreview)

	type scoredAnime struct {
		id    string
		title string
		cover string
		score int
	}
	var scored []scoredAnime
	for _, anime := range animeList {
		idStr := strconv.Itoa(anime.ID)
		title := anime.Title.English
		if title == "" {
			title = anime.Title.Romaji
		}
		cover := anime.CoverImage.Large
		score := levenshtein(title, query)
		scored = append(scored, scoredAnime{idStr, title, cover, score})
	}
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score < scored[j].score
	})
	for i, s := range scored {
		if i >= 10 {
			break
		}
		animeDict[s.id] = RofiSelectPreview{
			Title:      s.title,
			CoverImage: s.cover,
		}
	}
	return animeDict, nil
}

// SearchAnimeAnilist sends the query to AniList and returns a map of title to ID
func SearchAnimeAnilist(query, token string) ([]SelectionOption, error) {
	url := "https://graphql.anilist.co"

	queryString := `
	query ($search: String) {
		Page(page: 1, perPage: 50) {
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

	body, err := doAniListSearchRequest(url, requestBody, token)
	if err != nil {
		return nil, err
	}

	var responseData map[string]ResponseData
	err = json.Unmarshal(body, &responseData)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	animeList := responseData["data"].Page.Media
	var results []SelectionOption

	type scoredAnime struct {
		id    string
		title string
		score int
	}
	var scored []scoredAnime
	for _, anime := range animeList {
		idStr := strconv.Itoa(anime.ID)
		title := anime.Title.English
		if title == "" {
			title = anime.Title.Romaji
		}
		score := levenshtein(title, query)
		scored = append(scored, scoredAnime{idStr, title, score})
	}
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score < scored[j].score
	})
	for i, s := range scored {
		if i >= 10 {
			break
		}
		results = append(results, SelectionOption{
			Key:   s.id,
			Label: s.title,
		})
	}
	return results, nil
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
		"Content-Type": "application/json",
		"Accept":       "application/json",
	}
	if token != "" {
		headers["Authorization"] = "Bearer " + token
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
func AddAniListAnimeToWatchingList(animeID int, token string) error {
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

	headers := map[string]string{"Content-Type": "application/json"}
	if token != "" {
		headers["Authorization"] = "Bearer " + token
	}

	_, err := makePostRequest(url, mutation, variables, headers)
	if err != nil {
		return fmt.Errorf("failed to add anime: %w", err)
	}

	CurdOut(fmt.Sprintf("Anime with ID %d has been added to your watching list.", animeID))
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
	query := `
	query ($userId: Int, $type: MediaType) {
		MediaListCollection(userId: $userId, type: $type) {
			lists {
				entries {
					id
					media {
						id
						idMal
						episodes
						duration
						title {
							romaji
							english
							native
						}
						status
					}
					status
					score
					progress
					repeat
					updatedAt
					startedAt {
						year
						month
						day
					}
					completedAt {
						year
						month
						day
					}
				}
			}
		}
	}`

	variables := map[string]interface{}{
		"userId": userID,
		"type":   "ANIME",
	}

	headers := map[string]string{"Content-Type": "application/json"}
	if token != "" {
		headers["Authorization"] = "Bearer " + token
	}

	response, err := makePostRequest("https://graphql.anilist.co", query, variables, headers)
	if err != nil {
		return nil, err
	}

	return response, nil
}

func GetUserDataPreview(token string, userID int) (map[string]interface{}, error) {
	query := `
	query ($userId: Int, $type: MediaType) {
		MediaListCollection(userId: $userId, type: $type) {
			lists {
				entries {
					id
					media {
						id
						idMal
						episodes
						duration
						coverImage {
							large
						}
						title {
							romaji
							english
							native
						}
						status
					}
					status
					score
					progress
					repeat
					updatedAt
					startedAt {
						year
						month
						day
					}
					completedAt {
						year
						month
						day
					}
				}
			}
		}
	}`

	variables := map[string]interface{}{
		"userId": userID,
		"type":   "ANIME",
	}

	headers := map[string]string{"Content-Type": "application/json"}
	if token != "" {
		headers["Authorization"] = "Bearer " + token
	}

	response, err := makePostRequest("https://graphql.anilist.co", query, variables, headers)
	if err != nil {
		return nil, err
	}

	return response, nil
}

// Function to load a JSON file
func LoadJSONFile(filePath string) (map[string]interface{}, error) {
	data, err := os.ReadFile(filepath.Clean(filePath))
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
					"id":            media["id"],
					"progress":      entry.(map[string]interface{})["progress"],
					"romaji_title":  romajiTitle,
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
func UpdateAniListAnimeProgress(token string, mediaID, progress int) error {
	err := SaveAniListAnimeListEntry(token, mediaID, nil, &progress, nil, nil, nil, nil)
	if err != nil {
		return err
	}

	CurdOut(fmt.Sprint("Anime progress updated! Latest watched episode: ", progress))
	return nil
}

func UpdateAniListAnimeStatus(token string, mediaID int, status string) error {
	err := SaveAniListAnimeListEntry(token, mediaID, &status, nil, nil, nil, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to update anime status: %w", err)
	}

	statusMap := map[string]string{
		"CURRENT":   "Currently Watching",
		"COMPLETED": "Completed",
		"PAUSED":    "On Hold",
		"DROPPED":   "Dropped",
		"PLANNING":  "Plan to Watch",
		"REPEATING": "Rewatching",
	}

	CurdOut(fmt.Sprintf("Anime status updated to: %s", statusMap[status]))
	return nil
}

func SaveAniListAnimeListEntry(token string, mediaID int, status *string, progress *int, repeat *int, score *float64, startedAt *FuzzyDate, completedAt *FuzzyDate) error {
	url := "https://graphql.anilist.co"
	query := `
	mutation(
		$mediaId: Int
		$status: MediaListStatus
		$progress: Int
		$repeat: Int
		$score: Float
		$startedAt: FuzzyDateInput
		$completedAt: FuzzyDateInput
	) {
		SaveMediaListEntry(
			mediaId: $mediaId
			status: $status
			progress: $progress
			repeat: $repeat
			score: $score
			startedAt: $startedAt
			completedAt: $completedAt
		) {
			id
			status
			progress
			repeat
			startedAt {
				year
				month
				day
			}
			completedAt {
				year
				month
				day
			}
		}
	}`

	variables := map[string]interface{}{
		"mediaId": mediaID,
	}
	if status != nil {
		variables["status"] = *status
	}
	if progress != nil {
		variables["progress"] = *progress
	}
	if repeat != nil {
		variables["repeat"] = *repeat
	}
	if score != nil {
		variables["score"] = *score
	}
	if startedAt != nil {
		if startedAt.Year == 0 && startedAt.Month == 0 && startedAt.Day == 0 {
			variables["startedAt"] = nil
		} else {
			variables["startedAt"] = map[string]int{
				"year":  startedAt.Year,
				"month": startedAt.Month,
				"day":   startedAt.Day,
			}
		}
	}
	if completedAt != nil {
		if completedAt.Year == 0 && completedAt.Month == 0 && completedAt.Day == 0 {
			variables["completedAt"] = nil
		} else {
			variables["completedAt"] = map[string]int{
				"year":  completedAt.Year,
				"month": completedAt.Month,
				"day":   completedAt.Day,
			}
		}
	}

	headers := map[string]string{
		"Authorization": "Bearer " + token,
		"Content-Type":  "application/json",
	}

	_, err := makePostRequest(url, query, variables, headers)
	if err != nil {
		return err
	}
	return nil
}

func CompleteAniListAnimeRewatch(token string, anime Anime) error {
	status := "COMPLETED"
	progress := anime.Ep.Number
	repeat := anime.Repeat + 1
	completedAt := currentFuzzyDate()
	startedAt := anime.StartedAt
	if startedAt == (FuzzyDate{}) {
		startedAt = completedAt
	}
	return SaveAniListAnimeListEntry(token, anime.AnilistId, &status, &progress, &repeat, nil, &startedAt, &completedAt)
}

// Function to rate an anime on AniList
func RateAniListAnime(token string, mediaID int) error {
	score, err := promptAnimeScoreValue()
	if err != nil {
		return err
	}
	return saveAniListAnimeScore(token, mediaID, score)
}

func saveAniListAnimeScore(token string, mediaID int, score float64) error {
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

	CurdOut(fmt.Sprintf("Successfully rated anime (mediaId: %d) with score: %.2f", mediaID, score))
	return nil
}

func aniListRetryDelay(resp *http.Response, attempt int) time.Duration {
	if resp != nil {
		if retryAfter := strings.TrimSpace(resp.Header.Get("Retry-After")); retryAfter != "" {
			if seconds, err := strconv.Atoi(retryAfter); err == nil && seconds > 0 {
				return time.Duration(seconds) * time.Second
			}
		}
	}

	return time.Duration(attempt+1) * 2 * time.Second
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

	client := &http.Client{}
	for attempt := 0; attempt < 5; attempt++ {
		req, err := http.NewRequest("POST", url, bytes.NewBuffer(requestBody))
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		for key, value := range headers {
			req.Header.Set(key, value)
		}

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to send request: %w", err)
		}

		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("failed to read response body: %w", readErr)
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			if attempt == 4 {
				return nil, fmt.Errorf("failed with status %d: %s", resp.StatusCode, body)
			}
			time.Sleep(aniListRetryDelay(resp, attempt))
			continue
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("failed with status %d: %s", resp.StatusCode, body)
		}

		var responseData map[string]interface{}
		if err := json.Unmarshal(body, &responseData); err != nil {
			return nil, fmt.Errorf("failed to unmarshal response: %w", err)
		}

		return responseData, nil
	}

	return nil, fmt.Errorf("AniList request failed after retries")
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

	parseFuzzyDate := func(value interface{}) FuzzyDate {
		dateMap, ok := value.(map[string]interface{})
		if !ok || dateMap == nil {
			return FuzzyDate{}
		}

		return FuzzyDate{
			Year:  toInt(dateMap["year"]),
			Month: toInt(dateMap["month"]),
			Day:   toInt(dateMap["day"]),
		}
	}

	// Access the list entries in the input map
	if input["data"] == nil {
		Log("Anilist request failed")
		CurdOut("Anilist request failed")
		ExitCurd(fmt.Errorf("Anilist request failed"))
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
				ListID: toInt(entryData["id"]),
				Media: Media{
					Duration: toInt(media["duration"]),
					Episodes: toInt(media["episodes"]),
					ID:       toInt(media["id"]),
					MalID:    toInt(media["idMal"]),
					Title: AnimeTitle{
						English:  safeString(media["title"].(map[string]interface{})["english"]),
						Romaji:   safeString(media["title"].(map[string]interface{})["romaji"]),
						Japanese: safeString(media["title"].(map[string]interface{})["native"]),
					},
					Status: safeString(media["status"]),
				},
				Progress:    toInt(entryData["progress"]),
				Repeat:      toInt(entryData["repeat"]),
				Score:       entryData["score"].(float64),
				Status:      safeString(entryData["status"]), // Ensure status is fetched safely
				StartedAt:   parseFuzzyDate(entryData["startedAt"]),
				CompletedAt: parseFuzzyDate(entryData["completedAt"]),
				UpdatedAt:   time.Unix(int64(toInt(entryData["updatedAt"])), 0).UTC(),
			}

			if coverImage, ok := media["coverImage"].(map[string]interface{}); ok {
				animeEntry.CoverImage = safeString(coverImage["large"])
			}

			// Append entries based on their status
			switch animeEntry.Status {
			case "CURRENT":
				animeList.Watching = append(animeList.Watching, animeEntry)
			case "COMPLETED":
				animeList.Completed = append(animeList.Completed, animeEntry)
			case "PAUSED":
				animeList.Paused = append(animeList.Paused, animeEntry)
			case "DROPPED":
				animeList.Dropped = append(animeList.Dropped, animeEntry)
			case "PLANNING":
				animeList.Planning = append(animeList.Planning, animeEntry)
			case "REPEATING": // Anilist uses REPEATING for rewatching
				animeList.Rewatching = append(animeList.Rewatching, animeEntry)
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
		list.Rewatching, // Add Rewatching list
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

// GetAnimeDataByID retrieves detailed anime data from AniList using the anime's ID and user token
func GetAnimeDataByID(id int, token string) (Anime, error) {
	url := "https://graphql.anilist.co"
	query := `
	query ($id: Int) {
		Media(id: $id, type: ANIME) {
			id
			idMal
			episodes
			duration
			status
			title {
				romaji
				english
				native
			}
			coverImage {
				large
			}
			nextAiringEpisode {
				episode
			}
		}
	}`

	variables := map[string]interface{}{
		"id": id,
	}

	headers := map[string]string{"Content-Type": "application/json"}
	if token != "" {
		headers["Authorization"] = "Bearer " + token
	}

	response, err := makePostRequest(url, query, variables, headers)
	if err != nil {
		return Anime{}, fmt.Errorf("failed to get anime data: %w", err)
	}

	data, ok := response["data"].(map[string]interface{})
	if !ok {
		return Anime{}, fmt.Errorf("invalid response format: data field missing")
	}

	media, ok := data["Media"].(map[string]interface{})
	if !ok {
		return Anime{}, fmt.Errorf("invalid response format: Media field missing")
	}

	anime := Anime{
		AnilistId: id,
		IsAiring:  false,
	}

	if malID, ok := media["idMal"].(float64); ok {
		anime.MalId = int(malID)
	}

	// Safely handle episodes field which might be nil for currently airing shows
	if episodes, ok := media["episodes"].(float64); ok {
		anime.TotalEpisodes = int(episodes)
	}
	if duration, ok := media["duration"].(float64); ok {
		anime.Ep.Duration = int(duration) * 60
	}
	if title, ok := media["title"].(map[string]interface{}); ok {
		anime.Title = AnimeTitle{
			Romaji:   safeAniListString(title["romaji"]),
			English:  safeAniListString(title["english"]),
			Japanese: safeAniListString(title["native"]),
		}
	}
	if coverImage, ok := media["coverImage"].(map[string]interface{}); ok {
		anime.CoverImage = safeAniListString(coverImage["large"])
	}

	// Check status
	if status, ok := media["status"].(string); ok {
		anime.IsAiring = status == "RELEASING"
	}

	// Double check with nextAiringEpisode
	if nextEp, ok := media["nextAiringEpisode"].(map[string]interface{}); ok && nextEp != nil {
		anime.IsAiring = true
	}

	return anime, nil
}

// SequelInfo holds information about a sequel anime
type SequelInfo struct {
	ID         int
	Title      AnimeTitle
	CoverImage string
	Episodes   int
	Status     string // "FINISHED", "RELEASING", "NOT_YET_RELEASED"
	SiteURL    string
}

// GetAnimeSequel fetches sequel information for a given anime from AniList
// GetAnimeSequel fetches sequel information for a given anime from AniList
func GetAnimeSequel(animeID int, token string) ([]SequelInfo, error) {
	url := "https://graphql.anilist.co"
	query := `
	query ($id: Int) {
		Media(id: $id, type: ANIME) {
			relations {
				edges {
					relationType
					node {
						id
						title {
							romaji
							english
						}
						coverImage {
							large
						}
						episodes
						status
						siteUrl
					}
				}
			}
		}
	}`

	variables := map[string]interface{}{
		"id": animeID,
	}

	headers := map[string]string{"Content-Type": "application/json"}
	if token != "" {
		headers["Authorization"] = "Bearer " + token
	}

	response, err := makePostRequest(url, query, variables, headers)
	if err != nil {
		return nil, fmt.Errorf("failed to get anime relations: %w", err)
	}

	data, ok := response["data"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid response format: data field missing")
	}

	media, ok := data["Media"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid response format: Media field missing")
	}

	relations, ok := media["relations"].(map[string]interface{})
	if !ok {
		return nil, nil // No relations found
	}

	edges, ok := relations["edges"].([]interface{})
	if !ok || len(edges) == 0 {
		return nil, nil // No edges found
	}

	var sequels []SequelInfo

	// Look for a SEQUEL relation
	for _, edge := range edges {
		edgeData, ok := edge.(map[string]interface{})
		if !ok {
			continue
		}

		relationType, ok := edgeData["relationType"].(string)
		if !ok || relationType != "SEQUEL" {
			continue
		}

		node, ok := edgeData["node"].(map[string]interface{})
		if !ok {
			continue
		}

		var sequel SequelInfo

		// Parse ID
		if id, ok := node["id"].(float64); ok {
			sequel.ID = int(id)
		}

		// Parse title
		if title, ok := node["title"].(map[string]interface{}); ok {
			if romaji, ok := title["romaji"].(string); ok {
				sequel.Title.Romaji = romaji
			}
			if english, ok := title["english"].(string); ok {
				sequel.Title.English = english
			}
		}

		// Parse cover image
		if coverImage, ok := node["coverImage"].(map[string]interface{}); ok {
			if large, ok := coverImage["large"].(string); ok {
				sequel.CoverImage = large
			}
		}

		// Parse episodes
		if episodes, ok := node["episodes"].(float64); ok {
			sequel.Episodes = int(episodes)
		}

		// Parse status
		if status, ok := node["status"].(string); ok {
			sequel.Status = status
		}

		// Parse siteUrl
		if siteUrl, ok := node["siteUrl"].(string); ok {
			sequel.SiteURL = siteUrl
		}

		sequels = append(sequels, sequel)
	}

	if len(sequels) == 0 {
		return nil, nil // No sequel found
	}

	return sequels, nil
}

// AddAnimeToList adds an anime to a specified list (CURRENT, PLANNING, PAUSED, DROPPED)
func AddAniListAnimeToList(animeID int, status string, token string) error {
	url := "https://graphql.anilist.co"
	mutation := `
	mutation ($mediaId: Int, $status: MediaListStatus) {
		SaveMediaListEntry (mediaId: $mediaId, status: $status) {
			id
			status
		}
	}`

	variables := map[string]interface{}{
		"mediaId": animeID,
		"status":  status,
	}

	headers := map[string]string{
		"Authorization": "Bearer " + token,
		"Content-Type":  "application/json",
	}

	_, err := makePostRequest(url, mutation, variables, headers)
	if err != nil {
		return fmt.Errorf("failed to add anime to list: %w", err)
	}

	statusMap := map[string]string{
		"CURRENT":   "Currently Watching",
		"COMPLETED": "Completed",
		"PAUSED":    "On Hold",
		"DROPPED":   "Dropped",
		"PLANNING":  "Plan to Watch",
		"REPEATING": "Rewatching",
	}

	CurdOut(fmt.Sprintf("Anime added to: %s", statusMap[status]))
	return nil
}

func DeleteAniListListEntry(token string, listID int) error {
	url := "https://graphql.anilist.co"
	query := `
	mutation ($id: Int) {
		DeleteMediaListEntry(id: $id) {
			deleted
		}
	}`

	headers := map[string]string{"Content-Type": "application/json"}
	if token != "" {
		headers["Authorization"] = "Bearer " + token
	}

	_, err := makePostRequest(url, query, map[string]interface{}{"id": listID}, headers)
	return err
}

// FindSequelInAnimeList searches for a sequel in the user's anime list and returns its status
func FindSequelInAnimeList(list AnimeList, sequelID int) (string, bool) {
	// Check all categories
	for _, entry := range list.Watching {
		if entry.Media.ID == sequelID {
			return "CURRENT", true
		}
	}
	for _, entry := range list.Planning {
		if entry.Media.ID == sequelID {
			return "PLANNING", true
		}
	}
	for _, entry := range list.Completed {
		if entry.Media.ID == sequelID {
			return "COMPLETED", true
		}
	}
	for _, entry := range list.Paused {
		if entry.Media.ID == sequelID {
			return "PAUSED", true
		}
	}
	for _, entry := range list.Dropped {
		if entry.Media.ID == sequelID {
			return "DROPPED", true
		}
	}
	for _, entry := range list.Rewatching {
		if entry.Media.ID == sequelID {
			return "REWATCHING", true
		}
	}

	return "", false
}
