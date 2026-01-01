package internal

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"

	// "io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/browser"
)

const (
	anilistOAuthURL     = "https://anilist.co/api/v2/oauth"
	anilistClientID     = "20686"
	anilistClientSecret = "APfx41cOgSQVMvi88v7PbN7g6kzed2ZQRcxmACod"
	anilistRedirectURI  = "http://localhost:8000/oauth/callback"
	anilistServerPort   = 8000
)

// AnilistToken represents the OAuth token response from Anilist
type AnilistToken struct {
	AccessToken  string    `json:"access_token"`
	TokenType    string    `json:"token_type"`
	ExpiresIn    int       `json:"expires_in"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// CurdConfig struct with field names that match the config keys
type CurdConfig struct {
	Player                   string   `config:"Player"`
	MpvArgs                  []string `config:MpvArgs`
	SubsLanguage             string   `config:"SubsLanguage"`
	SubOrDub                 string   `config:"SubOrDub"`
	StoragePath              string   `config:"StoragePath"`
	AnimeNameLanguage        string   `config:"AnimeNameLanguage"`
	MenuOrder                string   `config:"MenuOrder"`
	PercentageToMarkComplete int      `config:"PercentageToMarkComplete"`
	NextEpisodePrompt        bool     `config:"NextEpisodePrompt"`
	SkipOp                   bool     `config:"SkipOp"`
	SkipEd                   bool     `config:"SkipEd"`
	SkipFiller               bool     `config:"SkipFiller"`
	ImagePreview             bool     `config:"ImagePreview"`
	SkipRecap                bool     `config:"SkipRecap"`
	RofiSelection            bool     `config:"RofiSelection"`
	CurrentCategory          bool     `config:"CurrentCategory"`
	ScoreOnCompletion        bool     `config:"ScoreOnCompletion"`
	SaveMpvSpeed             bool     `config:"SaveMpvSpeed"`
	AddMissingOptions        bool     `config:"AddMissingOptions"`
	AlternateScreen          bool     `config:"AlternateScreen"`
	DiscordPresence          bool     `config:"DiscordPresence"`
	DiscordClientId          string   `config:"DiscordClientId"`
}

// Default configuration values as a map
func defaultConfigMap() map[string]string {
	return map[string]string{
		"Player":                   "mpv",
		"MpvArgs":                  "[]",
		"StoragePath":              "$HOME/.local/share/curd",
		"AnimeNameLanguage":        "english",
		"SubsLanguage":             "english",
		"MenuOrder":                "CURRENT,ALL,UNTRACKED,UPDATE,CONTINUE_LAST",
		"SubOrDub":                 "sub",
		"PercentageToMarkComplete": "85",
		"NextEpisodePrompt":        "false",
		"SkipOp":                   "true",
		"SkipEd":                   "true",
		"SkipFiller":               "true",
		"SkipRecap":                "true",
		"RofiSelection":            "false",
		"ImagePreview":             "false",
		"ScoreOnCompletion":        "true",
		"SaveMpvSpeed":             "true",
		"AddMissingOptions":        "true",
		"AlternateScreen":          "true",
		"DiscordPresence":          "true",
		"DiscordClientId":          "1287457464148820089",
	}
}

var globalConfig *CurdConfig

func SetGlobalConfig(config *CurdConfig) {
	globalConfig = config
}

func GetGlobalConfig() *CurdConfig {
	return globalConfig
}

// Helper function to parse string array from config
func parseStringArray(value string) []string {
	// Remove brackets and split by comma
	value = strings.TrimPrefix(value, "[")
	value = strings.TrimSuffix(value, "]")
	if value == "" {
		return nil
	}

	// Split by comma and trim spaces and quotes from each element
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		// Trim spaces and quotes
		part = strings.TrimSpace(part)
		part = strings.Trim(part, "\"")
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

// LoadConfig reads or creates the config file, adds missing fields, and returns the populated CurdConfig struct
func LoadConfig(configPath string) (CurdConfig, error) {
	configPath = os.ExpandEnv(configPath) // Substitute environment variables like $HOME

	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Create the config file with default values if it doesn't exist
		CurdOut("Config file not found. Creating default config...")
		if err := createDefaultConfig(configPath); err != nil {
			return CurdConfig{}, fmt.Errorf("error creating default config file: %v", err)
		}
	}

	// Load the config from file
	configMap, err := LoadConfigFromFile(configPath)
	if err != nil {
		return CurdConfig{}, fmt.Errorf("error loading config file: %v", err)
	}

	// Check AddMissingOptions setting first
	addMissing := true
	if val, exists := configMap["AddMissingOptions"]; exists {
		addMissing, _ = strconv.ParseBool(val)
	}

	// Add missing fields to the config map
	updated := false
	defaultConfigMap := defaultConfigMap()
	for key, defaultValue := range defaultConfigMap {
		if _, exists := configMap[key]; !exists {
			configMap[key] = defaultValue
			updated = true
		}
	}

	// Write updated config back to file only if AddMissingOptions is true
	if addMissing && updated {
		if err := SaveConfigToFile(configPath, configMap); err != nil {
			return CurdConfig{}, fmt.Errorf("error saving updated config file: %v", err)
		}
	}

	// Parse string arrays
	if mpvArgs, exists := configMap["MpvArgs"]; exists {
		configMap["MpvArgs"] = mpvArgs
	}

	// Populate the CurdConfig struct from the config map
	config := PopulateConfig(configMap)

	return config, nil
}

// Create a config file with default values in key=value format
// Ensure the directory exists before creating the file
func createDefaultConfig(path string) error {
	defaultConfig := defaultConfigMap()

	// Ensure the directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("error creating directory: %v", err)
	}

	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("error creating file: %v", err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	for key, value := range defaultConfig {
		line := fmt.Sprintf("%s=%s\n", key, value)
		if _, err := writer.WriteString(line); err != nil {
			return fmt.Errorf("error writing to file: %v", err)
		}
	}
	if err := writer.Flush(); err != nil {
		return fmt.Errorf("error flushing writer: %v", err)
	}
	return nil
}

// authenticateWithBrowser performs OAuth authentication using browser
func authenticateWithBrowser(tokenPath string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Try to load existing token first
	if token, err := loadToken(tokenPath); err == nil && isTokenValid(token) {
		return token.AccessToken, nil
	}

	// Start local server to handle OAuth callback
	callbackCh := make(chan string, 1)
	errCh := make(chan error, 1)
	mux := http.NewServeMux()
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", anilistServerPort),
		Handler: mux,
	}

	// Handle OAuth callback - for authorization code grant, code comes in query params
	mux.HandleFunc("/oauth/callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		errorParam := r.URL.Query().Get("error")

		w.Header().Set("Content-Type", "text/html")

		if errorParam != "" {
			w.WriteHeader(http.StatusBadRequest)
			html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <title>Curd Authentication</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 50px; text-align: center; background: #1a1a1a; color: white; }
        .error { color: #f44336; font-size: 18px; margin-bottom: 20px; }
    </style>
</head>
<body>
    <div class="error">Authentication failed: %s</div>
    <p>You can close this window and try again.</p>
</body>
</html>`, errorParam)
			fmt.Fprint(w, html)
			errCh <- fmt.Errorf("oauth error: %s", errorParam)
			return
		}

		if code == "" {
			w.WriteHeader(http.StatusBadRequest)
			html := `<!DOCTYPE html>
<html>
<head>
    <title>Curd Authentication</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 50px; text-align: center; background: #1a1a1a; color: white; }
        .error { color: #f44336; font-size: 18px; margin-bottom: 20px; }
    </style>
</head>
<body>
    <div class="error">No authorization code received</div>
    <p>You can close this window and try again.</p>
</body>
</html>`
			fmt.Fprint(w, html)
			errCh <- fmt.Errorf("no authorization code received")
			return
		}

		// Exchange authorization code for access token
		go func() {
			tokenURL := fmt.Sprintf("%s/token", anilistOAuthURL)
			data := url.Values{
				"grant_type":    {"authorization_code"},
				"client_id":     {anilistClientID},
				"client_secret": {anilistClientSecret},
				"redirect_uri":  {anilistRedirectURI},
				"code":          {code},
			}

			resp, err := http.PostForm(tokenURL, data)
			if err != nil {
				errCh <- fmt.Errorf("failed to exchange code for token: %w", err)
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				errCh <- fmt.Errorf("token exchange failed with status: %d", resp.StatusCode)
				return
			}

			var tokenResponse struct {
				AccessToken string `json:"access_token"`
				TokenType   string `json:"token_type"`
				ExpiresIn   int    `json:"expires_in"`
			}

			if err := json.NewDecoder(resp.Body).Decode(&tokenResponse); err != nil {
				errCh <- fmt.Errorf("failed to parse token response: %w", err)
				return
			}

			if tokenResponse.AccessToken == "" {
				errCh <- fmt.Errorf("no access token in response")
				return
			}

			callbackCh <- tokenResponse.AccessToken
		}()

		// Show success page immediately
		html := `<!DOCTYPE html>
<html>
<head>
    <title>Curd Authentication</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 50px; text-align: center; background: #1a1a1a; color: white; }
        .loading { color: #2196F3; font-size: 18px; margin-bottom: 20px; }
    </style>
</head>
<body>
    <div class="loading">Processing authentication...</div>
    <p>Exchanging authorization code for token. You can close this window.</p>
</body>
</html>`
		fmt.Fprint(w, html)
	})

	// Start server in background
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("failed to start server: %w", err)
		}
	}()
	defer srv.Shutdown(ctx)

	// Give server a moment to start
	time.Sleep(100 * time.Millisecond)

	// Open browser for authentication using Authorization Code Grant flow (response_type=code)
	authURL := fmt.Sprintf("%s/authorize?client_id=%s&redirect_uri=%s&response_type=code",
		anilistOAuthURL,
		anilistClientID,
		url.QueryEscape(anilistRedirectURI))

	fmt.Println("Opening browser for AniList authentication...")
	fmt.Printf("If the browser doesn't open automatically, visit: %s\n", authURL)

	if err := browser.OpenURL(authURL); err != nil {
		fmt.Printf("Failed to open browser automatically: %v\n", err)
		fmt.Println("Please copy and paste the URL above into your browser")
	}

	// Wait for token
	var accessToken string
	select {
	case accessToken = <-callbackCh:
	case err := <-errCh:
		return "", fmt.Errorf("authentication failed: %w", err)
	case <-ctx.Done():
		return "", fmt.Errorf("authentication timeout after 5 minutes")
	}

	// Create token object and save
	token := &AnilistToken{
		AccessToken: accessToken,
		TokenType:   "Bearer",
		ExpiresIn:   31536000, // AniList tokens are valid for 1 year
		ExpiresAt:   time.Now().Add(365 * 24 * time.Hour),
	}

	// Save token to file
	if err := saveToken(tokenPath, token); err != nil {
		return "", fmt.Errorf("failed to save token: %w", err)
	}

	fmt.Println("Authentication successful!")
	return token.AccessToken, nil
}

// loadToken loads the token from the token file
func loadToken(tokenPath string) (*AnilistToken, error) {
	data, err := os.ReadFile(tokenPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read token file: %w", err)
	}

	var token AnilistToken
	if err := json.Unmarshal(data, &token); err != nil {
		return nil, fmt.Errorf("failed to parse token file: %w", err)
	}

	return &token, nil
}

// saveToken saves the token to the token file
func saveToken(tokenPath string, token *AnilistToken) error {
	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(tokenPath), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	data, err := json.Marshal(token)
	if err != nil {
		return fmt.Errorf("failed to marshal token: %w", err)
	}

	return os.WriteFile(tokenPath, data, 0600)
}

// isTokenValid checks if the token is still valid
func isTokenValid(token *AnilistToken) bool {
	return token != nil && token.AccessToken != "" && time.Now().Before(token.ExpiresAt)
}

// GetTokenFromFile loads the token from the token file (supports both old text format and new JSON format)
func GetTokenFromFile(tokenPath string) (string, error) {
	data, err := os.ReadFile(tokenPath)
	if err != nil {
		return "", fmt.Errorf("failed to read token from file: %w", err)
	}

	// Try to parse as JSON first (new format)
	var token AnilistToken
	if err := json.Unmarshal(data, &token); err == nil {
		// It's JSON format, check if token is valid
		if isTokenValid(&token) {
			return token.AccessToken, nil
		}
		return "", fmt.Errorf("token has expired")
	}

	// Fall back to plain text format (old format)
	plainToken := strings.TrimSpace(string(data))
	if plainToken == "" {
		return "", fmt.Errorf("empty token file")
	}

	return plainToken, nil
}

func ChangeToken(config *CurdConfig, user *User) {
	var err error
	tokenPath := filepath.Join(os.ExpandEnv(config.StoragePath), "anilist_token.json")

	// Try browser-based OAuth first
	fmt.Println("Starting browser-based authentication...")
	user.Token, err = authenticateWithBrowser(tokenPath)

	if err != nil {
		Log("Browser authentication failed: " + err.Error())
		fmt.Printf("Browser authentication failed: %v\n", err)
		fmt.Println("Falling back to manual token entry...")

		// Simple CLI fallback
		fmt.Println("Please visit: https://anilist.co/api/v2/oauth/authorize?client_id=20686&response_type=token&redirect_uri=http://localhost:8000/oauth/callback")
		fmt.Print("Copy and paste your access token here: ")
		fmt.Scanln(&user.Token)

		if user.Token == "" {
			ExitCurd(fmt.Errorf("no token provided"))
		}

		// Save the manually entered token as JSON format
		token := &AnilistToken{
			AccessToken: user.Token,
			TokenType:   "Bearer",
			ExpiresIn:   31536000, // AniList tokens are valid for 1 year
			ExpiresAt:   time.Now().Add(365 * 24 * time.Hour),
		}

		if err := saveToken(tokenPath, token); err != nil {
			ExitCurd(fmt.Errorf("failed to save token: %w", err))
		}
	}

	if user.Token == "" {
		ExitCurd(fmt.Errorf("no token provided"))
	}

	fmt.Println("Token saved successfully!")
}

// LoadConfigFromFile loads config file from disk into a map (key=value format)
func LoadConfigFromFile(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	configMap := make(map[string]string)
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue // Skip empty lines and comments
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			configMap[key] = value
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return configMap, nil
}

// SaveConfigToFile saves updated config map to file in key=value format
func SaveConfigToFile(path string, configMap map[string]string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	for key, value := range configMap {
		line := fmt.Sprintf("%s=%s\n", key, value)
		if _, err := writer.WriteString(line); err != nil {
			return err
		}
	}
	return writer.Flush()
}

// PopulateConfig populates the CurdConfig struct from a map
func PopulateConfig(configMap map[string]string) CurdConfig {
	config := CurdConfig{}
	configValue := reflect.ValueOf(&config).Elem()

	for i := 0; i < configValue.NumField(); i++ {
		field := configValue.Type().Field(i)
		tag := field.Tag.Get("config")

		if value, exists := configMap[tag]; exists {
			fieldValue := configValue.FieldByName(field.Name)

			if fieldValue.CanSet() {
				switch fieldValue.Kind() {
				case reflect.String:
					fieldValue.SetString(value)
				case reflect.Int:
					intVal, _ := strconv.Atoi(value)
					fieldValue.SetInt(int64(intVal))
				case reflect.Bool:
					boolVal, _ := strconv.ParseBool(value)
					fieldValue.SetBool(boolVal)
				}
			}
		}
	}

	// Handle MpvArgs specially
	if mpvArgs, exists := configMap["MpvArgs"]; exists {
		config.MpvArgs = parseStringArray(mpvArgs)
	}

	return config
}

func getOrderedCategories(userCurdConfig *CurdConfig) []SelectionOption {
	// Define the default categories and all available labels
	defaultOrder := []string{"CURRENT", "ALL", "UNTRACKED", "UPDATE", "CONTINUE_LAST"}
	availableLabels := map[string]string{
		"CURRENT":       "Currently Watching",
		"ALL":           "Show All",
		"UNTRACKED":     "Untracked Watching",
		"UPDATE":        "Update (Episode, Status, Score)",
		"CONTINUE_LAST": "Continue Last Session",
		"PLANNING":      "Plan to Watch",
		"COMPLETED":     "Completed",
		"PAUSED":        "Paused",
		"DROPPED":       "Dropped",
		"REWATCHING":    "Rewatching",
	}

	// Create ordered list to store final result
	finalOrder := make([]string, 0)
	seen := make(map[string]bool)

	// If no menu order specified, use default order
	if userCurdConfig.MenuOrder == "" {
		finalOrder = defaultOrder
	} else {
		// Only show items explicitly specified by user
		menuItems := strings.Split(userCurdConfig.MenuOrder, ",")
		for _, key := range menuItems {
			key = strings.TrimSpace(key)
			if _, exists := availableLabels[key]; exists && !seen[key] {
				finalOrder = append(finalOrder, key)
				seen[key] = true
			}
		}
	}

	// Create the final ordered slice of SelectionOptions
	orderedCategories := make([]SelectionOption, 0, len(finalOrder))
	for _, key := range finalOrder {
		orderedCategories = append(orderedCategories, SelectionOption{
			Key:   key,
			Label: availableLabels[key],
		})
	}

	return orderedCategories
}
