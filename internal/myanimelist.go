package internal

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/browser"
)

type OAuthToken = AnilistToken

const (
	myAnimeListAuthorizeURL      = "https://myanimelist.net/v1/oauth2/authorize"
	myAnimeListTokenURL          = "https://myanimelist.net/v1/oauth2/token"
	myAnimeListAPIBaseURL        = "https://api.myanimelist.net/v2"
	myAnimeListRedirectURI       = "http://localhost:8123/oauth/callback"
	myAnimeListServerPort        = 8123
	myAnimeListListCacheFileName = "myanimelist_list_cache.json"
	trackingIDCacheFileName      = "tracking_id_cache.json"
	myAnimeListPendingAuthFile   = "myanimelist_oauth_pending.json"
)

type myAnimeListUser struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type myAnimeListPicture struct {
	Medium string `json:"medium"`
	Large  string `json:"large"`
}

type myAnimeListAlternativeTitles struct {
	En string `json:"en"`
	Ja string `json:"ja"`
}

type myAnimeListAnimeNode struct {
	ID                     int                          `json:"id"`
	Title                  string                       `json:"title"`
	MainPicture            myAnimeListPicture           `json:"main_picture"`
	AlternativeTitles      myAnimeListAlternativeTitles `json:"alternative_titles"`
	NumEpisodes            int                          `json:"num_episodes"`
	AverageEpisodeDuration int                          `json:"average_episode_duration"`
	Status                 string                       `json:"status"`
}

type myAnimeListListStatus struct {
	Status             string `json:"status"`
	Score              int    `json:"score"`
	NumEpisodesWatched int    `json:"num_episodes_watched"`
	IsRewatching       bool   `json:"is_rewatching"`
	NumTimesRewatched  int    `json:"num_times_rewatched"`
	StartDate          string `json:"start_date"`
	FinishDate         string `json:"finish_date"`
	UpdatedAt          string `json:"updated_at"`
}

type myAnimeListListResponse struct {
	Data []struct {
		Node       myAnimeListAnimeNode  `json:"node"`
		ListStatus myAnimeListListStatus `json:"list_status"`
	} `json:"data"`
	Paging struct {
		Next string `json:"next"`
	} `json:"paging"`
}

type trackingIDCachePayload struct {
	MalToAniList map[string]int `json:"mal_to_anilist"`
}

type pendingMyAnimeListAuth struct {
	CodeVerifier string    `json:"code_verifier"`
	State        string    `json:"state"`
	CreatedAt    time.Time `json:"created_at"`
}

func myAnimeListTokenPath(config *CurdConfig) string {
	return filepath.Join(os.ExpandEnv(config.StoragePath), "myanimelist_token.json")
}

func myAnimeListCachePath(storagePath string) string {
	return filepath.Join(os.ExpandEnv(storagePath), myAnimeListListCacheFileName)
}

func trackingIDCachePath(storagePath string) string {
	return filepath.Join(os.ExpandEnv(storagePath), trackingIDCacheFileName)
}

func myAnimeListPendingAuthPath(config *CurdConfig) string {
	return filepath.Join(os.ExpandEnv(config.StoragePath), myAnimeListPendingAuthFile)
}

func savePendingMyAnimeListAuth(config *CurdConfig, pending pendingMyAnimeListAuth) error {
	storagePath := os.ExpandEnv(config.StoragePath)
	if err := os.MkdirAll(storagePath, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(pending, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(myAnimeListPendingAuthPath(config), data, 0o600)
}

func loadPendingMyAnimeListAuth(config *CurdConfig) (*pendingMyAnimeListAuth, error) {
	data, err := os.ReadFile(myAnimeListPendingAuthPath(config))
	if err != nil {
		return nil, err
	}
	var pending pendingMyAnimeListAuth
	if err := json.Unmarshal(data, &pending); err != nil {
		return nil, err
	}
	return &pending, nil
}

func clearPendingMyAnimeListAuth(config *CurdConfig) {
	_ = os.Remove(myAnimeListPendingAuthPath(config))
}

func extractOAuthCodeFromInput(input string, expectedState string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", fmt.Errorf("empty callback input")
	}

	if strings.Contains(input, "://") || strings.Contains(input, "code=") {
		if strings.Contains(input, "://") {
			parsed, err := url.Parse(input)
			if err != nil {
				return "", err
			}
			query := parsed.Query()
			if expectedState != "" {
				if state := query.Get("state"); state != "" && state != expectedState {
					return "", fmt.Errorf("callback state does not match the pending login")
				}
			}
			if code := query.Get("code"); code != "" {
				return code, nil
			}
			return "", fmt.Errorf("callback URL does not include an authorization code")
		}

		query, err := url.ParseQuery(strings.TrimPrefix(input, "?"))
		if err != nil {
			return "", err
		}
		if expectedState != "" {
			if state := query.Get("state"); state != "" && state != expectedState {
				return "", fmt.Errorf("callback state does not match the pending login")
			}
		}
		if code := query.Get("code"); code != "" {
			return code, nil
		}
	}

	return input, nil
}

func exchangeMyAnimeListAuthorizationCode(config *CurdConfig, tokenPath string, codeVerifier string, code string) (string, error) {
	clientID, clientSecret := myAnimeListClientCredentials(config)
	form := url.Values{
		"client_id":     {clientID},
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {myAnimeListRedirectURI},
		"code_verifier": {codeVerifier},
	}
	if clientSecret != "" {
		form.Set("client_secret", clientSecret)
	}

	req, err := http.NewRequest("POST", myAnimeListTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := sharedHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("MyAnimeList token exchange failed: %s", strings.TrimSpace(string(body)))
	}

	var token OAuthToken
	if err := json.Unmarshal(body, &token); err != nil {
		return "", err
	}
	if strings.TrimSpace(token.AccessToken) == "" {
		return "", fmt.Errorf("MyAnimeList token response did not include an access token")
	}
	token.ExpiresAt = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
	if err := saveToken(tokenPath, &token); err != nil {
		return "", err
	}
	clearPendingMyAnimeListAuth(config)
	return token.AccessToken, nil
}

func myAnimeListClientCredentials(config *CurdConfig) (string, string) {
	if config == nil {
		return "", ""
	}

	clientID := strings.TrimSpace(config.MyAnimeListClientID)
	if clientID == "" {
		clientID = strings.TrimSpace(os.Getenv("CURD_MAL_CLIENT_ID"))
	}
	if clientID == "" {
		clientID = strings.TrimSpace(os.Getenv("MYANIMELIST_CLIENT_ID"))
	}

	clientSecret := strings.TrimSpace(config.MyAnimeListClientSecret)
	if clientSecret == "" {
		clientSecret = strings.TrimSpace(os.Getenv("CURD_MAL_CLIENT_SECRET"))
	}
	if clientSecret == "" {
		clientSecret = strings.TrimSpace(os.Getenv("MYANIMELIST_CLIENT_SECRET"))
	}

	return clientID, clientSecret
}

func randomOAuthString(length int) (string, error) {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-._~"
	if length < 43 {
		length = 43
	}
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}

	for i := range buf {
		buf[i] = chars[int(buf[i])%len(chars)]
	}
	return string(buf), nil
}

func authenticateMyAnimeListWithBrowser(config *CurdConfig, tokenPath string) (string, error) {
	clientID, _ := myAnimeListClientCredentials(config)
	if clientID == "" {
		return "", fmt.Errorf("MyAnimeList client ID is not configured")
	}

	if token, err := loadToken(tokenPath); err == nil {
		if isTokenValid(token) {
			return token.AccessToken, nil
		}
		if token.RefreshToken != "" {
			refreshed, refreshErr := refreshMyAnimeListToken(config, tokenPath, token.RefreshToken)
			if refreshErr == nil {
				return refreshed.AccessToken, nil
			}
		}
	}

	if pending, err := loadPendingMyAnimeListAuth(config); err == nil {
		CurdOut("Found an unfinished MyAnimeList login.")
		callbackInput, inputErr := promptTrackingInput("Paste the full MyAnimeList callback URL (or just the code), or press Enter to start over", true)
		if inputErr != nil {
			return "", inputErr
		}
		if strings.TrimSpace(callbackInput) != "" {
			code, parseErr := extractOAuthCodeFromInput(callbackInput, pending.State)
			if parseErr != nil {
				return "", parseErr
			}
			return exchangeMyAnimeListAuthorizationCode(config, tokenPath, pending.CodeVerifier, code)
		}
		clearPendingMyAnimeListAuth(config)
	}

	codeVerifier, err := randomOAuthString(64)
	if err != nil {
		return "", err
	}
	state, err := randomOAuthString(32)
	if err != nil {
		return "", err
	}
	if err := savePendingMyAnimeListAuth(config, pendingMyAnimeListAuth{
		CodeVerifier: codeVerifier,
		State:        state,
		CreatedAt:    time.Now(),
	}); err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	callbackCh := make(chan string, 1)
	errCh := make(chan error, 1)
	mux := http.NewServeMux()
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", myAnimeListServerPort),
		Handler: mux,
	}

	mux.HandleFunc("/oauth/callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		returnedState := r.URL.Query().Get("state")
		if returnedState != state {
			http.Error(w, "invalid state", http.StatusBadRequest)
			errCh <- fmt.Errorf("invalid state returned from MyAnimeList")
			return
		}
		if code == "" {
			http.Error(w, "missing authorization code", http.StatusBadRequest)
			errCh <- fmt.Errorf("missing authorization code")
			return
		}

		fmt.Fprint(w, "<html><body><p>Authentication complete. You can close this window.</p></body></html>")
		callbackCh <- code
	})

	go func() {
		if serveErr := srv.ListenAndServe(); serveErr != nil && serveErr != http.ErrServerClosed {
			errCh <- serveErr
		}
	}()
	defer srv.Shutdown(ctx)

	time.Sleep(100 * time.Millisecond)

	authURL := fmt.Sprintf(
		"%s?response_type=code&client_id=%s&state=%s&redirect_uri=%s&code_challenge=%s&code_challenge_method=plain",
		myAnimeListAuthorizeURL,
		url.QueryEscape(clientID),
		url.QueryEscape(state),
		url.QueryEscape(myAnimeListRedirectURI),
		url.QueryEscape(codeVerifier),
	)

	fmt.Println("Opening browser for MyAnimeList authentication...")
	fmt.Printf("If the browser doesn't open automatically, visit: %s\n", authURL)
	if err := browser.OpenURL(authURL); err != nil {
		fmt.Printf("Failed to open browser automatically: %v\n", err)
	}
	fmt.Println("If curd does not continue after the browser reaches localhost, rerun the command and paste the full callback URL when prompted.")

	var code string
	select {
	case code = <-callbackCh:
	case err := <-errCh:
		return "", fmt.Errorf("%w. rerun the command and paste the callback URL to finish login", err)
	case <-ctx.Done():
		callbackInput, inputErr := promptTrackingInput("Paste the full MyAnimeList callback URL or just the code", false)
		if inputErr != nil {
			return "", fmt.Errorf("MyAnimeList authentication timed out; rerun the command and paste the callback URL to finish login")
		}
		code, err = extractOAuthCodeFromInput(callbackInput, state)
		if err != nil {
			return "", err
		}
	}

	return exchangeMyAnimeListAuthorizationCode(config, tokenPath, codeVerifier, code)
}

var authenticateMyAnimeList = authenticateMyAnimeListWithBrowser

func refreshMyAnimeListToken(config *CurdConfig, tokenPath string, refreshToken string) (*OAuthToken, error) {
	clientID, clientSecret := myAnimeListClientCredentials(config)
	if clientID == "" {
		return nil, fmt.Errorf("MyAnimeList client ID is not configured")
	}
	if refreshToken == "" {
		return nil, fmt.Errorf("MyAnimeList refresh token is missing")
	}

	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {clientID},
	}
	if clientSecret != "" {
		form.Set("client_secret", clientSecret)
	}

	req, err := http.NewRequest("POST", myAnimeListTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := sharedHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed refreshing MyAnimeList token: %s", strings.TrimSpace(string(body)))
	}

	var token OAuthToken
	if err := json.Unmarshal(body, &token); err != nil {
		return nil, err
	}
	if strings.TrimSpace(token.AccessToken) == "" {
		return nil, fmt.Errorf("MyAnimeList refresh response did not include an access token")
	}
	if strings.TrimSpace(token.RefreshToken) == "" {
		token.RefreshToken = refreshToken
	}
	token.ExpiresAt = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
	if err := saveToken(tokenPath, &token); err != nil {
		return nil, err
	}

	if user := GetGlobalUser(); user != nil {
		user.Token = token.AccessToken
	}

	return &token, nil
}

func renewMyAnimeListAccessToken(config *CurdConfig, tokenPath string, token *OAuthToken, reason string) (string, error) {
	if token != nil && strings.TrimSpace(token.RefreshToken) != "" {
		refreshed, refreshErr := refreshMyAnimeListToken(config, tokenPath, token.RefreshToken)
		if refreshErr == nil {
			return refreshed.AccessToken, nil
		}
		Log(fmt.Sprintf("Failed to refresh MyAnimeList token after %s: %v", reason, refreshErr))
	}

	CurdOut("MyAnimeList sign-in required. Opening browser...")
	Log(fmt.Sprintf("Starting MyAnimeList browser authentication after %s", reason))
	clearStoredTokenFile(tokenPath)

	accessToken, authErr := authenticateMyAnimeList(config, tokenPath)
	if authErr != nil {
		return "", authErr
	}
	if user := GetGlobalUser(); user != nil {
		user.Token = accessToken
	}
	return accessToken, nil
}

func GetMyAnimeListAccessToken(config *CurdConfig) (string, error) {
	tokenPath := myAnimeListTokenPath(config)
	token, err := loadToken(tokenPath)
	if err != nil {
		Log(fmt.Sprintf("Failed to load MyAnimeList token, starting login: %v", err))
		return renewMyAnimeListAccessToken(config, tokenPath, nil, "token load failure")
	}
	if isTokenValid(token) {
		if user := GetGlobalUser(); user != nil {
			user.Token = token.AccessToken
		}
		return token.AccessToken, nil
	}
	if token.RefreshToken == "" {
		Log("MyAnimeList token is expired and has no refresh token, starting login")
		return renewMyAnimeListAccessToken(config, tokenPath, token, "expired token")
	}
	return renewMyAnimeListAccessToken(config, tokenPath, token, "expired token")
}

func ChangeMyAnimeListToken(config *CurdConfig, user *User) error {
	tokenPath := myAnimeListTokenPath(config)
	accessToken, err := authenticateMyAnimeList(config, tokenPath)
	if err != nil {
		return err
	}
	user.Token = accessToken
	return nil
}

func myAnimeListRequest(config *CurdConfig, method, requestURL string, form url.Values, out interface{}) error {
	token, err := GetMyAnimeListAccessToken(config)
	if err != nil {
		return err
	}

	buildRequest := func(accessToken string) (*http.Request, error) {
		urlString := requestURL
		var body io.Reader
		if !strings.HasPrefix(urlString, "http://") && !strings.HasPrefix(urlString, "https://") {
			urlString = myAnimeListAPIBaseURL + requestURL
		}

		if method == http.MethodGet && form != nil && len(form) > 0 {
			separator := "?"
			if strings.Contains(urlString, "?") {
				separator = "&"
			}
			urlString += separator + form.Encode()
		}

		if method != http.MethodGet && form != nil {
			body = strings.NewReader(form.Encode())
		}

		req, reqErr := http.NewRequest(method, urlString, body)
		if reqErr != nil {
			return nil, reqErr
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)
		if method != http.MethodGet && form != nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		return req, nil
	}

	for attempt := 0; attempt < 2; attempt++ {
		req, err := buildRequest(token)
		if err != nil {
			return err
		}

		resp, err := sharedHTTPClient.Do(req)
		if err != nil {
			return err
		}

		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return readErr
		}

		if resp.StatusCode == http.StatusUnauthorized || isInvalidTokenResponse(resp.StatusCode, body) {
			if attempt == 0 {
				tokenPath := myAnimeListTokenPath(config)
				storedToken, tokenErr := loadToken(tokenPath)
				if tokenErr != nil {
					Log(fmt.Sprintf("Failed to load MyAnimeList token after unauthorized response, starting login: %v", tokenErr))
					accessToken, authErr := renewMyAnimeListAccessToken(config, tokenPath, nil, "unauthorized API response")
					if authErr != nil {
						return authErr
					}
					token = accessToken
					continue
				}
				accessToken, renewErr := renewMyAnimeListAccessToken(config, tokenPath, storedToken, "unauthorized API response")
				if renewErr != nil {
					return renewErr
				}
				token = accessToken
				continue
			}
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("MyAnimeList API %s %s failed: %s", method, requestURL, strings.TrimSpace(string(body)))
		}

		if out != nil {
			if err := json.Unmarshal(body, out); err != nil {
				return err
			}
		}
		return nil
	}

	return fmt.Errorf("MyAnimeList API request failed after retry")
}

func parseMyAnimeListDate(value string) FuzzyDate {
	if value == "" {
		return FuzzyDate{}
	}
	parts := strings.Split(value, "-")
	if len(parts) != 3 {
		return FuzzyDate{}
	}

	year, yearErr := strconv.Atoi(parts[0])
	month, monthErr := strconv.Atoi(parts[1])
	day, dayErr := strconv.Atoi(parts[2])
	if yearErr != nil || monthErr != nil || dayErr != nil || year <= 0 || month < 1 || month > 12 || day < 1 || day > 31 {
		return FuzzyDate{}
	}
	return FuzzyDate{Year: year, Month: month, Day: day}
}

func parseMyAnimeListTimestamp(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}

	formats := []string{time.RFC3339Nano, time.RFC3339}
	for _, format := range formats {
		if parsed, err := time.Parse(format, value); err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
}

func formatMyAnimeListDate(value FuzzyDate) string {
	if value.Year == 0 || value.Month == 0 || value.Day == 0 {
		return ""
	}
	return fmt.Sprintf("%04d-%02d-%02d", value.Year, value.Month, value.Day)
}

func myAnimeListStatusToAniListStatus(status string, isRewatching bool) string {
	if isRewatching {
		return "REPEATING"
	}
	switch strings.ToLower(status) {
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

func aniListStatusToMyAnimeListStatus(status string) string {
	switch strings.ToUpper(strings.TrimSpace(status)) {
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
	case "REPEATING":
		return "watching"
	default:
		return "watching"
	}
}

func myAnimeListMediaStatusToAniListStatus(status string) string {
	switch strings.ToLower(status) {
	case "currently_airing":
		return "RELEASING"
	case "not_yet_aired":
		return "NOT_YET_RELEASED"
	default:
		return "FINISHED"
	}
}

func loadTrackingIDCache(storagePath string) (trackingIDCachePayload, error) {
	data, err := os.ReadFile(trackingIDCachePath(storagePath))
	if err != nil {
		return trackingIDCachePayload{}, err
	}

	var payload trackingIDCachePayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return trackingIDCachePayload{}, err
	}
	if payload.MalToAniList == nil {
		payload.MalToAniList = map[string]int{}
	}
	return payload, nil
}

func saveTrackingIDCache(storagePath string, payload trackingIDCachePayload) error {
	storagePath = os.ExpandEnv(storagePath)
	if err := os.MkdirAll(storagePath, 0o755); err != nil {
		return err
	}

	if payload.MalToAniList == nil {
		payload.MalToAniList = map[string]int{}
	}

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}

	cachePath := trackingIDCachePath(storagePath)
	tempPath := cachePath + ".tmp"
	if err := os.WriteFile(tempPath, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tempPath, cachePath); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	return nil
}

func lookupAniListMediaByMalID(malID int) (Media, string, error) {
	query := `
	query ($idMal: Int) {
		Media(idMal: $idMal, type: ANIME) {
			id
			idMal
			episodes
			duration
			status
			format
			title {
				romaji
				english
				native
			}
			coverImage {
				large
			}
		}
	}`

	response, err := makePostRequest("https://graphql.anilist.co", query, map[string]interface{}{"idMal": malID}, map[string]string{"Content-Type": "application/json"})
	if err != nil {
		return Media{}, "", err
	}

	data, ok := response["data"].(map[string]interface{})
	if !ok {
		return Media{}, "", fmt.Errorf("invalid AniList response")
	}
	mediaData, ok := data["Media"].(map[string]interface{})
	if !ok || mediaData == nil {
		return Media{}, "", fmt.Errorf("AniList media not found for MAL ID %d", malID)
	}

	aniListID, ok := mediaData["id"].(float64)
	if !ok || aniListID == 0 {
		return Media{}, "", fmt.Errorf("AniList media response for MAL ID %d is missing id", malID)
	}

	media := Media{
		ID:       int(aniListID),
		MalID:    malID,
		Episodes: 0,
		Duration: 0,
		Status:   safeAniListString(mediaData["status"]),
		Format:   safeAniListString(mediaData["format"]),
		Title: AnimeTitle{
			Romaji:   safeAniListTitle(mediaData, "romaji"),
			English:  safeAniListTitle(mediaData, "english"),
			Japanese: safeAniListTitle(mediaData, "native"),
		},
	}

	if episodes, ok := mediaData["episodes"].(float64); ok {
		media.Episodes = int(episodes)
	}
	if duration, ok := mediaData["duration"].(float64); ok {
		media.Duration = int(duration)
	}

	cover := ""
	if coverImage, ok := mediaData["coverImage"].(map[string]interface{}); ok {
		if large, ok := coverImage["large"].(string); ok {
			cover = large
		}
	}

	return media, cover, nil
}

func safeAniListString(value interface{}) string {
	if str, ok := value.(string); ok {
		return str
	}
	return ""
}

func safeAniListTitle(mediaData map[string]interface{}, key string) string {
	titleMap, ok := mediaData["title"].(map[string]interface{})
	if !ok {
		return ""
	}
	return safeAniListString(titleMap[key])
}

type aniListMediaLookupResult struct {
	Media Media
	Cover string
}

func buildMediaFromMyAnimeListNode(node myAnimeListAnimeNode, aniListID int) aniListMediaLookupResult {
	media := Media{
		ID:    aniListID,
		MalID: node.ID,
		Title: AnimeTitle{
			English:  node.AlternativeTitles.En,
			Romaji:   node.Title,
			Japanese: node.AlternativeTitles.Ja,
		},
		Episodes: node.NumEpisodes,
		Status:   myAnimeListMediaStatusToAniListStatus(node.Status),
	}
	if node.AverageEpisodeDuration > 0 {
		media.Duration = node.AverageEpisodeDuration / 60
	}
	return aniListMediaLookupResult{
		Media: media,
		Cover: node.MainPicture.Large,
	}
}

func lookupAniListMediaByMalIDs(malIDs []int) (map[int]aniListMediaLookupResult, error) {
	if len(malIDs) == 0 {
		return map[int]aniListMediaLookupResult{}, nil
	}

	var query strings.Builder
	query.WriteString("query {")
	for idx, malID := range malIDs {
		fmt.Fprintf(&query, " m%d: Media(idMal: %d, type: ANIME) { id idMal episodes duration status format title { romaji english native } coverImage { large } }", idx, malID)
	}
	query.WriteString(" }")

	headers := map[string]string{"Content-Type": "application/json"}
	var response map[string]interface{}
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		response, err = makePostRequest("https://graphql.anilist.co", query.String(), nil, headers)
		if err == nil {
			break
		}
		if !strings.Contains(err.Error(), "429") {
			return nil, err
		}
		time.Sleep(time.Duration(attempt+1) * 750 * time.Millisecond)
	}
	if err != nil {
		return nil, err
	}

	data, ok := response["data"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid AniList batch response")
	}

	results := make(map[int]aniListMediaLookupResult, len(malIDs))
	for idx, malID := range malIDs {
		alias := fmt.Sprintf("m%d", idx)
		rawMedia, ok := data[alias].(map[string]interface{})
		if !ok || rawMedia == nil {
			continue
		}
		aniListID, ok := rawMedia["id"].(float64)
		if !ok || aniListID == 0 {
			continue
		}
		media := Media{
			ID:       int(aniListID),
			MalID:    malID,
			Episodes: 0,
			Duration: 0,
			Status:   safeAniListString(rawMedia["status"]),
			Format:   safeAniListString(rawMedia["format"]),
			Title: AnimeTitle{
				Romaji:   safeAniListTitle(rawMedia, "romaji"),
				English:  safeAniListTitle(rawMedia, "english"),
				Japanese: safeAniListTitle(rawMedia, "native"),
			},
		}
		if episodes, ok := rawMedia["episodes"].(float64); ok {
			media.Episodes = int(episodes)
		}
		if duration, ok := rawMedia["duration"].(float64); ok {
			media.Duration = int(duration)
		}
		cover := ""
		if coverImage, ok := rawMedia["coverImage"].(map[string]interface{}); ok {
			cover = safeAniListString(coverImage["large"])
		}
		results[malID] = aniListMediaLookupResult{Media: media, Cover: cover}
	}

	return results, nil
}

func resolveAniListMediaForMyAnimeList(config *CurdConfig, malID int, fallbackTitle string) (Media, string, error) {
	cachePayload, err := loadTrackingIDCache(config.StoragePath)
	if err != nil && !os.IsNotExist(err) {
		Log(fmt.Sprintf("Failed to load MAL tracking ID cache: %v", err))
	}
	if cachePayload.MalToAniList == nil {
		cachePayload.MalToAniList = map[string]int{}
	}

	cacheKey := strconv.Itoa(malID)
	if cachedID := cachePayload.MalToAniList[cacheKey]; cachedID != 0 {
		media, cover, err := lookupAniListMediaByMalID(malID)
		if err == nil {
			media.ID = cachedID
			return media, cover, nil
		}
	}

	media, cover, err := lookupAniListMediaByMalID(malID)
	if err != nil {
		return Media{}, "", fmt.Errorf("failed to resolve AniList metadata for MAL %d (%s): %w", malID, fallbackTitle, err)
	}

	cachePayload.MalToAniList[cacheKey] = media.ID
	if saveErr := saveTrackingIDCache(config.StoragePath, cachePayload); saveErr != nil {
		Log(fmt.Sprintf("Failed to save MAL tracking ID cache: %v", saveErr))
	}

	return media, cover, nil
}

func loadMyAnimeListCache(storagePath string, userID int) (animeListCachePayload, error) {
	cacheFilePath := myAnimeListCachePath(storagePath)
	data, err := os.ReadFile(cacheFilePath)
	if err != nil {
		return animeListCachePayload{}, err
	}

	var payload animeListCachePayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return animeListCachePayload{}, err
	}
	if payload.UserID != 0 && userID != 0 && payload.UserID != userID {
		return animeListCachePayload{}, fmt.Errorf("anime list cache belongs to a different MyAnimeList user")
	}
	return payload, nil
}

func saveMyAnimeListCache(storagePath string, userID int, list AnimeList) error {
	storagePath = os.ExpandEnv(storagePath)
	if err := os.MkdirAll(storagePath, 0o755); err != nil {
		return err
	}

	payload := animeListCachePayload{
		AnimeList: list,
		UpdatedAt: time.Now(),
		UserID:    userID,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}

	cacheFilePath := myAnimeListCachePath(storagePath)
	tempFilePath := cacheFilePath + ".tmp"
	if err := os.WriteFile(tempFilePath, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tempFilePath, cacheFilePath); err != nil {
		_ = os.Remove(tempFilePath)
		return err
	}
	return nil
}

func FetchLatestMyAnimeList(config *CurdConfig, user *User) (AnimeList, error) {
	var result AnimeList
	nextURL := "/users/@me/animelist"
	queryValues := url.Values{
		"limit":  {"1000"},
		"fields": {"list_status,num_episodes,average_episode_duration,alternative_titles,main_picture,status"},
		"sort":   {"list_updated_at"},
	}

	cachePayload, err := loadTrackingIDCache(config.StoragePath)
	if err != nil && !os.IsNotExist(err) {
		Log(fmt.Sprintf("Failed to load MAL tracking ID cache: %v", err))
	}
	if cachePayload.MalToAniList == nil {
		cachePayload.MalToAniList = map[string]int{}
	}

	for {
		var response myAnimeListListResponse
		if err := myAnimeListRequest(config, http.MethodGet, nextURL, queryValues, &response); err != nil {
			return AnimeList{}, err
		}
		queryValues = nil

		resolvedMedia := make(map[int]aniListMediaLookupResult, len(response.Data))
		unresolvedIDs := make([]int, 0)
		for _, item := range response.Data {
			cacheKey := strconv.Itoa(item.Node.ID)
			if cachedID := cachePayload.MalToAniList[cacheKey]; cachedID != 0 {
				resolvedMedia[item.Node.ID] = buildMediaFromMyAnimeListNode(item.Node, cachedID)
				continue
			}
			unresolvedIDs = append(unresolvedIDs, item.Node.ID)
		}

		for start := 0; start < len(unresolvedIDs); start += 25 {
			end := start + 25
			if end > len(unresolvedIDs) {
				end = len(unresolvedIDs)
			}
			batchResults, err := lookupAniListMediaByMalIDs(unresolvedIDs[start:end])
			if err != nil {
				Log(fmt.Sprintf("AniList batch lookup failed for MAL IDs %v: %v", unresolvedIDs[start:end], err))
				continue
			}
			for _, malID := range unresolvedIDs[start:end] {
				if lookup, ok := batchResults[malID]; ok {
					resolvedMedia[malID] = lookup
					cachePayload.MalToAniList[strconv.Itoa(malID)] = lookup.Media.ID
				}
			}
		}

		for _, item := range response.Data {
			lookup, ok := resolvedMedia[item.Node.ID]
			if !ok {
				Log(fmt.Sprintf("Skipping MAL anime %d because AniList mapping failed after batch lookup", item.Node.ID))
				continue
			}
			media := lookup.Media
			cover := lookup.Cover

			if media.Title.English == "" && item.Node.AlternativeTitles.En != "" {
				media.Title.English = item.Node.AlternativeTitles.En
			}
			if media.Title.Romaji == "" {
				media.Title.Romaji = item.Node.Title
			}
			if media.Title.Japanese == "" && item.Node.AlternativeTitles.Ja != "" {
				media.Title.Japanese = item.Node.AlternativeTitles.Ja
			}
			if media.Episodes == 0 {
				media.Episodes = item.Node.NumEpisodes
			}
			if media.Duration == 0 && item.Node.AverageEpisodeDuration > 0 {
				media.Duration = item.Node.AverageEpisodeDuration / 60
			}
			if media.Status == "" {
				media.Status = myAnimeListMediaStatusToAniListStatus(item.Node.Status)
			}

			entry := Entry{
				Media:       media,
				Progress:    item.ListStatus.NumEpisodesWatched,
				Repeat:      item.ListStatus.NumTimesRewatched,
				Score:       float64(item.ListStatus.Score),
				Status:      myAnimeListStatusToAniListStatus(item.ListStatus.Status, item.ListStatus.IsRewatching),
				StartedAt:   parseMyAnimeListDate(item.ListStatus.StartDate),
				CompletedAt: parseMyAnimeListDate(item.ListStatus.FinishDate),
				CoverImage:  cover,
				UpdatedAt:   parseMyAnimeListTimestamp(item.ListStatus.UpdatedAt),
			}
			if entry.CoverImage == "" {
				entry.CoverImage = item.Node.MainPicture.Large
			}

			switch entry.Status {
			case "CURRENT":
				result.Watching = append(result.Watching, entry)
			case "COMPLETED":
				result.Completed = append(result.Completed, entry)
			case "PAUSED":
				result.Paused = append(result.Paused, entry)
			case "DROPPED":
				result.Dropped = append(result.Dropped, entry)
			case "PLANNING":
				result.Planning = append(result.Planning, entry)
			case "REPEATING":
				result.Rewatching = append(result.Rewatching, entry)
			}
		}
		if saveErr := saveTrackingIDCache(config.StoragePath, cachePayload); saveErr != nil {
			Log(fmt.Sprintf("Failed to save MAL tracking ID cache: %v", saveErr))
		}

		if response.Paging.Next == "" {
			break
		}
		nextURL = response.Paging.Next
	}

	if user != nil {
		var me myAnimeListUser
		if err := myAnimeListRequest(config, http.MethodGet, "/users/@me", url.Values{"fields": {"id,name"}}, &me); err == nil {
			user.Id = me.ID
			user.Username = me.Name
		}
	}

	return result, nil
}

func refreshMyAnimeListInBackground(config *CurdConfig, user *User) {
	if user == nil || user.ListSync == nil {
		return
	}

	go func() {
		defer user.ListSync.MarkRefreshDone()

		latestList, err := FetchLatestMyAnimeList(config, user)
		if err != nil {
			Log(fmt.Sprintf("Failed to refresh MyAnimeList anime list in background: %v", err))
			return
		}
		if err := saveMyAnimeListCache(config.StoragePath, user.Id, latestList); err != nil {
			Log(fmt.Sprintf("Failed to save MyAnimeList cache: %v", err))
		}
		user.ListSync.Replace(latestList, true)
	}()
}

func InitializeMyAnimeListUserAnimeList(config *CurdConfig, user *User) error {
	cachedPayload, err := loadMyAnimeListCache(config.StoragePath, user.Id)
	if err == nil {
		if user.Id == 0 && cachedPayload.UserID != 0 {
			user.Id = cachedPayload.UserID
		}
		user.AnimeList = cachedPayload.AnimeList
		user.ListSync = NewAnimeListSync(cachedPayload.AnimeList)
		refreshMyAnimeListInBackground(config, user)
		return nil
	}

	latestList, err := FetchLatestMyAnimeList(config, user)
	if err != nil {
		return err
	}
	user.AnimeList = latestList
	user.ListSync = NewAnimeListSync(latestList)
	user.ListSync.MarkRefreshDone()
	if err := saveMyAnimeListCache(config.StoragePath, user.Id, latestList); err != nil {
		Log(fmt.Sprintf("Failed to save MyAnimeList anime list cache: %v", err))
	}
	return nil
}

func RefreshMyAnimeListUserAnimeList(config *CurdConfig, user *User) error {
	latestList, err := FetchLatestMyAnimeList(config, user)
	if err != nil {
		return err
	}
	user.AnimeList = latestList
	if user.ListSync == nil {
		user.ListSync = NewAnimeListSync(latestList)
		user.ListSync.MarkRefreshDone()
	} else {
		user.ListSync.Replace(latestList, true)
	}
	if err := saveMyAnimeListCache(config.StoragePath, user.Id, latestList); err != nil {
		Log(fmt.Sprintf("Failed to save MyAnimeList anime list cache: %v", err))
	}
	return nil
}

func updateMyAnimeListListStatus(config *CurdConfig, malID int, payload map[string]string) error {
	form := url.Values{}
	for key, value := range payload {
		if strings.TrimSpace(value) == "" && key != "start_date" && key != "finish_date" {
			continue
		}
		form.Set(key, value)
	}

	var response myAnimeListListStatus
	return myAnimeListRequest(config, http.MethodPut, fmt.Sprintf("/anime/%d/my_list_status", malID), form, &response)
}

func updateMyAnimeListProgress(config *CurdConfig, malID int, progress int) error {
	if err := updateMyAnimeListListStatus(config, malID, map[string]string{
		"num_watched_episodes": strconv.Itoa(progress),
	}); err != nil {
		return err
	}
	CurdOut(fmt.Sprint("Anime progress updated! Latest watched episode: ", progress))
	return nil
}

func updateMyAnimeListStatus(config *CurdConfig, malID int, status string) error {
	payload := map[string]string{
		"status":        aniListStatusToMyAnimeListStatus(status),
		"is_rewatching": "false",
	}
	if strings.EqualFold(status, "REPEATING") {
		payload["status"] = "watching"
		payload["is_rewatching"] = "true"
	}
	if err := updateMyAnimeListListStatus(config, malID, payload); err != nil {
		return err
	}
	CurdOut(fmt.Sprintf("Anime status updated to: %s", status))
	return nil
}

func rateMyAnimeListAnime(config *CurdConfig, malID int) error {
	score, err := promptAnimeScoreValue()
	if err != nil {
		return err
	}
	return rateMyAnimeListAnimeWithScore(config, malID, int(score+0.5))
}

func rateMyAnimeListAnimeWithScore(config *CurdConfig, malID int, score int) error {
	if err := updateMyAnimeListListStatus(config, malID, map[string]string{
		"score": strconv.Itoa(score),
	}); err != nil {
		return err
	}

	CurdOut(fmt.Sprintf("Successfully rated anime (malId: %d) with score: %d", malID, score))
	return nil
}

func addMyAnimeListAnimeToList(config *CurdConfig, malID int, status string) error {
	payload := map[string]string{
		"status":        aniListStatusToMyAnimeListStatus(status),
		"is_rewatching": "false",
	}
	if strings.EqualFold(status, "REPEATING") {
		payload["status"] = "watching"
		payload["is_rewatching"] = "true"
	}
	if err := updateMyAnimeListListStatus(config, malID, payload); err != nil {
		return err
	}
	CurdOut(fmt.Sprintf("Anime added to: %s", status))
	return nil
}

func completeMyAnimeListRewatch(config *CurdConfig, malID int, anime Anime) error {
	completedAt := currentFuzzyDate()
	startedAt := anime.StartedAt
	if startedAt == (FuzzyDate{}) {
		startedAt = completedAt
	}
	payload := map[string]string{
		"status":               "completed",
		"is_rewatching":        "false",
		"num_watched_episodes": strconv.Itoa(anime.Ep.Number),
		"start_date":           formatMyAnimeListDate(startedAt),
		"finish_date":          formatMyAnimeListDate(completedAt),
	}
	if err := updateMyAnimeListListStatus(config, malID, payload); err != nil {
		return err
	}
	return updateMyAnimeListListStatus(config, malID, map[string]string{
		"num_times_rewatched": strconv.Itoa(anime.Repeat + 1),
	})
}

func deleteMyAnimeListEntry(config *CurdConfig, malID int) error {
	return myAnimeListRequest(config, http.MethodDelete, fmt.Sprintf("/anime/%d/my_list_status", malID), nil, nil)
}

func maybeMarshalMALResponse(v interface{}) string {
	body, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(bytes.TrimSpace(body))
}
