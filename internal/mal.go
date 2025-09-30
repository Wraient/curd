package internal

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// MAL API endpoints
const (
	MALBaseURL        = "https://api.myanimelist.net/v2"
	MALAuthURL        = "https://myanimelist.net/v1/oauth2/authorize"
	MALTokenURL       = "https://myanimelist.net/v1/oauth2/token"
	MALRedirectURI    = "http://localhost:8080/callback" // Change to your registered redirect URI
)

// MALTokenResponse represents the OAuth token response from MAL
type MALTokenResponse struct {
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

// MALUser represents the authenticated MAL user
type MALUser struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Picture  string `json:"picture"`
	Gender   string `json:"gender"`
	Birthday string `json:"birthday"`
	Location string `json:"location"`
	JoinedAt string `json:"joined_at"`
}

// MALAnimeListStatus represents the user's anime list status
type MALAnimeListStatus struct {
	Status             string  `json:"status"`
	Score              int     `json:"score"`
	NumEpisodesWatched int     `json:"num_episodes_watched"`
	IsRewatching       bool    `json:"is_rewatching"`
	UpdatedAt          string  `json:"updated_at"`
	Priority           int     `json:"priority"`
	NumTimesRewatched  int     `json:"num_times_rewatched"`
	ReWatchValue       int     `json:"rewatch_value"`
	Tags               []string `json:"tags"`
	Comments           string  `json:"comments"`
}

// MALAnime represents an anime from MAL API
type MALAnime struct {
	ID              int                 `json:"id"`
	Title           string              `json:"title"`
	MainPicture     MALPicture          `json:"main_picture"`
	AlternativeTitles MALAlternativeTitles `json:"alternative_titles"`
	NumEpisodes     int                 `json:"num_episodes"`
	Status          string              `json:"status"`
	MyListStatus    *MALAnimeListStatus `json:"my_list_status"`
}

// MALPicture represents anime cover images
type MALPicture struct {
	Medium string `json:"medium"`
	Large  string `json:"large"`
}

// MALAlternativeTitles represents alternative titles
type MALAlternativeTitles struct {
	Synonyms []string `json:"synonyms"`
	En       string   `json:"en"`
	Ja       string   `json:"ja"`
}

// MALAnimeListResponse represents the anime list API response
type MALAnimeListResponse struct {
	Data   []MALAnimeNode `json:"data"`
	Paging MALPaging      `json:"paging"`
}

// MALAnimeNode wraps an anime with its node structure
type MALAnimeNode struct {
	Node MALAnime `json:"node"`
}

// MALPaging contains pagination information
type MALPaging struct {
	Next     string `json:"next"`
	Previous string `json:"previous"`
}

// GenerateCodeVerifier generates a random code verifier for PKCE (43-128 characters)
func GenerateCodeVerifier() (string, error) {
	b := make([]byte, 64) // 64 bytes = 86 base64 characters (within 43-128 range)
	_, err := rand.Read(b)
	if err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	// URL-safe base64 encoding without padding
	verifier := base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(b)
	return verifier, nil
}

// GenerateCodeChallenge generates a code challenge from the verifier
// MAL supports "plain" method, so we just return the verifier
func GenerateCodeChallenge(verifier string) string {
	return verifier
}

// GenerateState generates a random state parameter for CSRF protection
func GenerateState() (string, error) {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	if err != nil {
		return "", fmt.Errorf("failed to generate state: %w", err)
	}
	return base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(b), nil
}

// GetMALAuthorizationURL generates the OAuth authorization URL
func GetMALAuthorizationURL(clientID, redirectURI, state, codeChallenge string) string {
	params := url.Values{}
	params.Add("response_type", "code")
	params.Add("client_id", clientID)
	params.Add("redirect_uri", redirectURI)
	params.Add("state", state)
	params.Add("code_challenge", codeChallenge)
	params.Add("code_challenge_method", "plain")

	return fmt.Sprintf("%s?%s", MALAuthURL, params.Encode())
}

// ExchangeCodeForToken exchanges the authorization code for access token
func ExchangeCodeForToken(clientID, code, redirectURI, codeVerifier string) (*MALTokenResponse, error) {
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("client_id", clientID)
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)
	data.Set("code_verifier", codeVerifier)

	req, err := http.NewRequest("POST", MALTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code for token: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token exchange failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp MALTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	return &tokenResp, nil
}

// RefreshMALToken refreshes an expired access token
func RefreshMALToken(clientID, refreshToken string) (*MALTokenResponse, error) {
	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("client_id", clientID)
	data.Set("refresh_token", refreshToken)

	req, err := http.NewRequest("POST", MALTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create refresh request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to refresh token: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read refresh response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token refresh failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp MALTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse refresh response: %w", err)
	}

	return &tokenResp, nil
}

// GetMALUserInfo retrieves the authenticated user's information
func GetMALUserInfo(accessToken string) (*MALUser, error) {
	url := fmt.Sprintf("%s/users/@me", MALBaseURL)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create user info request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get user info: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read user info response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get user info with status %d: %s", resp.StatusCode, string(body))
	}

	var user MALUser
	if err := json.Unmarshal(body, &user); err != nil {
		return nil, fmt.Errorf("failed to parse user info: %w", err)
	}

	return &user, nil
}

// GetMALAnimeList retrieves the user's anime list
func GetMALAnimeList(accessToken, status string, limit, offset int) (*MALAnimeListResponse, error) {
	baseURL := fmt.Sprintf("%s/users/@me/animelist", MALBaseURL)

	params := url.Values{}
	params.Add("fields", "list_status,num_episodes,status,alternative_titles")
	if status != "" && status != "all" {
		params.Add("status", status)
	}
	if limit > 0 {
		params.Add("limit", strconv.Itoa(limit))
	}
	if offset > 0 {
		params.Add("offset", strconv.Itoa(offset))
	}

	requestURL := fmt.Sprintf("%s?%s", baseURL, params.Encode())

	req, err := http.NewRequest("GET", requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create anime list request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get anime list: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read anime list response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get anime list with status %d: %s", resp.StatusCode, string(body))
	}

	var listResp MALAnimeListResponse
	if err := json.Unmarshal(body, &listResp); err != nil {
		return nil, fmt.Errorf("failed to parse anime list: %w", err)
	}

	return &listResp, nil
}

// SearchMALAnime searches for anime on MAL
func SearchMALAnime(accessToken, query string, limit int) (*MALAnimeListResponse, error) {
	baseURL := fmt.Sprintf("%s/anime", MALBaseURL)

	params := url.Values{}
	params.Add("q", query)
	params.Add("fields", "alternative_titles,num_episodes,status")
	if limit > 0 {
		params.Add("limit", strconv.Itoa(limit))
	} else {
		params.Add("limit", "10")
	}

	requestURL := fmt.Sprintf("%s?%s", baseURL, params.Encode())

	req, err := http.NewRequest("GET", requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create search request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to search anime: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read search response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search failed with status %d: %s", resp.StatusCode, string(body))
	}

	var searchResp MALAnimeListResponse
	if err := json.Unmarshal(body, &searchResp); err != nil {
		return nil, fmt.Errorf("failed to parse search results: %w", err)
	}

	return &searchResp, nil
}

// UpdateMALAnimeStatus updates the anime list status for a specific anime
func UpdateMALAnimeStatus(accessToken string, animeID int, status string, score, episodesWatched int) error {
	requestURL := fmt.Sprintf("%s/anime/%d/my_list_status", MALBaseURL, animeID)

	data := url.Values{}
	if status != "" {
		data.Set("status", status)
	}
	if score > 0 {
		data.Set("score", strconv.Itoa(score))
	}
	if episodesWatched >= 0 {
		data.Set("num_watched_episodes", strconv.Itoa(episodesWatched))
	}

	req, err := http.NewRequest("PUT", requestURL, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create update request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to update anime status: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read update response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("update failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// DeleteMALAnimeFromList removes an anime from the user's list
func DeleteMALAnimeFromList(accessToken string, animeID int) error {
	requestURL := fmt.Sprintf("%s/anime/%d/my_list_status", MALBaseURL, animeID)

	req, err := http.NewRequest("DELETE", requestURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create delete request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete anime: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// GetMALAnimeDetails retrieves detailed information about a specific anime
func GetMALAnimeDetails(accessToken string, animeID int) (*MALAnime, error) {
	requestURL := fmt.Sprintf("%s/anime/%d?fields=alternative_titles,num_episodes,status,my_list_status", MALBaseURL, animeID)

	req, err := http.NewRequest("GET", requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create anime details request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get anime details: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read anime details response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get anime details with status %d: %s", resp.StatusCode, string(body))
	}

	var anime MALAnime
	if err := json.Unmarshal(body, &anime); err != nil {
		return nil, fmt.Errorf("failed to parse anime details: %w", err)
	}

	return &anime, nil
}

// ConvertMALStatusToAnilistStatus converts MAL status to AniList status format
func ConvertMALStatusToAnilistStatus(malStatus string) string {
	switch malStatus {
	case "watching":
		return "CURRENT"
	case "completed":
		return "COMPLETED"
	case "on_hold":
		return "PAUSED"
	case "dropped":
		return "DROPPED"
	case "plan_to_watch":
		return "PLANNING"
	default:
		return "CURRENT"
	}
}

// ConvertAnilistStatusToMAL converts AniList status to MAL status format
func ConvertAnilistStatusToMAL(anilistStatus string) string {
	switch anilistStatus {
	case "CURRENT":
		return "watching"
	case "COMPLETED":
		return "completed"
	case "PAUSED":
		return "on_hold"
	case "DROPPED":
		return "dropped"
	case "PLANNING":
		return "plan_to_watch"
	default:
		return "watching"
	}
}

// ParseMALAnimeList converts MAL anime list response to internal AnimeList format
func ParseMALAnimeList(malList *MALAnimeListResponse) AnimeList {
	animeList := AnimeList{}

	for _, node := range malList.Data {
		anime := node.Node

		// Create entry
		entry := Entry{
			Media: Media{
				ID:       anime.ID,
				Episodes: anime.NumEpisodes,
				Duration: 24, // MAL doesn't provide duration, default to 24 minutes
				Title: AnimeTitle{
					Romaji:   anime.Title,
					English:  anime.AlternativeTitles.En,
					Japanese: anime.AlternativeTitles.Ja,
				},
			},
			Score: 0,
		}

		// If anime has list status, add progress and score
		if anime.MyListStatus != nil {
			entry.Progress = anime.MyListStatus.NumEpisodesWatched
			entry.Score = float64(anime.MyListStatus.Score)
			entry.Status = ConvertMALStatusToAnilistStatus(anime.MyListStatus.Status)
			entry.CoverImage = anime.MainPicture.Large

			// Add to appropriate category
			switch anime.MyListStatus.Status {
			case "watching":
				animeList.Watching = append(animeList.Watching, entry)
			case "completed":
				animeList.Completed = append(animeList.Completed, entry)
			case "on_hold":
				animeList.Paused = append(animeList.Paused, entry)
			case "dropped":
				animeList.Dropped = append(animeList.Dropped, entry)
			case "plan_to_watch":
				animeList.Planning = append(animeList.Planning, entry)
			}

			// Check for rewatching
			if anime.MyListStatus.IsRewatching {
				animeList.Rewatching = append(animeList.Rewatching, entry)
			}
		}
	}

	return animeList
}

// SearchMALAnimeSimple searches for anime and returns results in SelectionOption format
func SearchMALAnimeSimple(accessToken, query string) ([]SelectionOption, error) {
	searchResp, err := SearchMALAnime(accessToken, query, 10)
	if err != nil {
		return nil, err
	}

	var results []SelectionOption
	for _, node := range searchResp.Data {
		anime := node.Node
		title := anime.Title
		if anime.AlternativeTitles.En != "" {
			title = anime.AlternativeTitles.En
		}

		results = append(results, SelectionOption{
			Key:   strconv.Itoa(anime.ID),
			Label: title,
		})
	}

	return results, nil
}