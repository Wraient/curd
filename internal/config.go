package internal

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"os/exec"
	"runtime"
	"strings"
)

// CurdConfig struct with field names that match the config keys
type CurdConfig struct {
	Player                  string `config:"Player"`
	SubsLanguage            string `config:"SubsLanguage"`
	SubOrDub                string `config:"SubOrDub"`
	StoragePath             string `config:"StoragePath"`
	AnimeNameLanguage		string `config:"AnimeNameLanguage"`
	PercentageToMarkComplete int    `config:"PercentageToMarkComplete"`
	NextEpisodePrompt       bool   `config:"NextEpisodePrompt"`
	SkipOp                  bool   `config:"SkipOp"`
	SkipEd                  bool   `config:"SkipEd"`
	SkipFiller              bool   `config:"SkipFiller"`
	ImagePreview			bool   `config:"ImagePreview"`
	SkipRecap               bool   `config:"SkipRecap"`
	RofiSelection           bool   `config:"RofiSelection"`
	CurrentCategory			bool   `config:"CurrentCategory"`
	ScoreOnCompletion       bool   `config:"ScoreOnCompletion"`
	SaveMpvSpeed            bool   `config:"SaveMpvSpeed"`
	DiscordPresence         bool   `config:"DiscordPresence"`
}

// Default configuration values as a map
func defaultConfigMap() map[string]string {
	return map[string]string{
		"Player":                  "mpv",
		"StoragePath":             "$HOME/.local/share/curd",
		"AnimeNameLanguage":	   "english",
		"SubsLanguage":            "english",
		"SubOrDub":                "sub",
		"PercentageToMarkComplete": "85",
		"NextEpisodePrompt":       "false",
		"SkipOp":                  "true",
		"SkipEd":                  "true",
		"SkipFiller":              "true",
		"SkipRecap":               "true",
		"RofiSelection":           "false",
		"ImagePreview":            "false",
		"ScoreOnCompletion":       "true",
		"SaveMpvSpeed":            "true",
		"DiscordPresence":         "true",
	}
}

var globalConfig *CurdConfig

func SetGlobalConfig(config *CurdConfig) {
	globalConfig = config
}

func GetGlobalConfig() *CurdConfig {
	return globalConfig
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
	configMap, err := loadConfigFromFile(defaultConfigMap(), configPath)
	if err != nil {
		return CurdConfig{}, fmt.Errorf("error loading config file: %v", err)
	}

	// Populate the CurdConfig struct from the config map
	config := populateConfig(configMap)

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

func ChangeToken(config *CurdConfig, user *User) {
	var err error
	tokenPath := filepath.Join(os.ExpandEnv(config.StoragePath), "token")

	if runtime.GOOS == "windows" {
		// Create a temporary file for the token
		tempFile, err := os.CreateTemp("", "curd-token-*.txt")
		if err != nil {
			Log("Error creating temp file: "+err.Error(), logFile)
			ExitCurd(err)
		}
		tempPath := tempFile.Name()
		tempFile.Close()

		// Write instructions to the temp file
		instructions := "Please generate a token from https://anilist.co/api/v2/oauth/authorize?client_id=20686&response_type=token\n" +
			"Replace this text with your token and save the file.\n"
		if err := os.WriteFile(tempPath, []byte(instructions), 0644); err != nil {
			Log("Error writing instructions: "+err.Error(), logFile)
			ExitCurd(err)
		}

		// Open notepad with the temp file
		cmd := exec.Command("notepad.exe", tempPath)
		if err := cmd.Run(); err != nil {
			Log("Error opening notepad: "+err.Error(), logFile)
			ExitCurd(err)
		}

		// Read the token from the file
		content, err := os.ReadFile(tempPath)
		if err != nil {
			Log("Error reading token: "+err.Error(), logFile)
			ExitCurd(err)
		}

		// Clean up the temp file
		os.Remove(tempPath)

		// Extract token (remove instructions and whitespace)
		user.Token = strings.TrimSpace(string(content))
	} else if config.RofiSelection {
		user.Token, err = GetTokenFromRofi()
	} else {
		fmt.Println("Please generate a token from https://anilist.co/api/v2/oauth/authorize?client_id=20686&response_type=token")
		fmt.Scanln(&user.Token)
	}

	if err != nil {
		Log("Error getting user input: "+err.Error(), logFile)
		ExitCurd(err)
	}
	WriteTokenToFile(user.Token, tokenPath)
}

// Load config file from disk into a map (key=value format)
func loadConfigFromFile(defaults map[string]string, path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

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
			defaults[key] = value
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return defaults, nil
}

// Save updated config map to file in key=value format
func saveConfigToFile(path string, configMap map[string]string) error {
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

// Populate the CurdConfig struct from a map
func populateConfig(configMap map[string]string) CurdConfig {
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

	return config
}
