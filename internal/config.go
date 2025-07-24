package internal

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
)

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

func getTokenFromTempFile(isWindowsPlatform bool) (string, error) {
	// Create a temporary file for the token
	tempFile, err := os.CreateTemp("", "curd-token-*.txt")
	if err != nil {
		return "", fmt.Errorf("error creating temp file: %v", err)
	}
	tempPath := tempFile.Name()
	tempFile.Close()

	// Write instructions to the temp file
	instructions := "Please generate a token from https://anilist.co/api/v2/oauth/authorize?client_id=20686&response_type=token\n" +
		"Replace this text with your token and save the file.\n"
	if err := os.WriteFile(tempPath, []byte(instructions), 0644); err != nil {
		return "", fmt.Errorf("error writing instructions: %v", err)
	}

	// Open the file with appropriate editor
	var cmd *exec.Cmd
	if isWindowsPlatform {
		cmd = exec.Command("notepad.exe", tempPath)
	} else {
		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = "nano" // Default to nano if $EDITOR is not set
		}
		cmd = exec.Command(editor, tempPath)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("error opening editor: %v", err)
	}

	// Read the token from the file
	content, err := os.ReadFile(tempPath)
	if err != nil {
		return "", fmt.Errorf("error reading token: %v", err)
	}

	// Clean up the temp file
	os.Remove(tempPath)

	// Extract token (remove instructions and whitespace)
	return strings.TrimSpace(string(content)), nil
}

func ChangeToken(config *CurdConfig, user *User) {
	var err error
	tokenPath := filepath.Join(os.ExpandEnv(config.StoragePath), "token")

	switch {
	case runtime.GOOS == "darwin" || runtime.GOOS == "windows":
		user.Token, err = getTokenFromTempFile(runtime.GOOS == "windows")
	case config.RofiSelection:
		user.Token, err = GetTokenFromRofi()
	default:
		fmt.Println("Please generate a token from https://anilist.co/api/v2/oauth/authorize?client_id=20686&response_type=token")
		fmt.Scanln(&user.Token)
	}

	if err != nil {
		Log("Error getting user input: " + err.Error())
		ExitCurd(err)
	}

	WriteTokenToFile(user.Token, tokenPath)
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
	// Define the default categories and their labels
	defaultOrder := []string{"CURRENT", "ALL", "UNTRACKED", "UPDATE", "CONTINUE_LAST"}
	defaultLabels := map[string]string{
		"CURRENT":       "Currently Watching",
		"ALL":           "Show All",
		"UNTRACKED":     "Untracked Watching",
		"UPDATE":        "Update (Episode, Status, Score)",
		"CONTINUE_LAST": "Continue Last Session",
	}

	// Create ordered list to store final result
	finalOrder := make([]string, 0)
	seen := make(map[string]bool)

	// If no menu order specified, use default order
	if userCurdConfig.MenuOrder == "" {
		finalOrder = defaultOrder
	} else {
		// First, process user-specified order
		menuItems := strings.Split(userCurdConfig.MenuOrder, ",")
		for _, key := range menuItems {
			key = strings.TrimSpace(key)
			if _, exists := defaultLabels[key]; exists && !seen[key] {
				finalOrder = append(finalOrder, key)
				seen[key] = true
			}
		}

		// Add remaining default items at the end
		for _, key := range defaultOrder {
			if !seen[key] {
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
			Label: defaultLabels[key],
		})
	}

	return orderedCategories
}
