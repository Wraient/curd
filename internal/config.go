package internal

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
)

// CurdConfig struct with field names that match the config keys
type CurdConfig struct {
	Player                  string `config:"Player"`
	HistoryFile             string `config:"HistoryFile"`
	SubsLanguage            string `config:"SubsLanguage"`
	SubOrDub                string `config:"SubOrDub"`
	PercentageToMarkComplete int    `config:"PercentageToMarkComplete"`
	NextEpisodePrompt       bool   `config:"NextEpisodePrompt"`
	SkipOp                  bool   `config:"SkipOp"`
	SkipEd                  bool   `config:"SkipEd"`
	SkipFiller              bool   `config:"SkipFiller"`
	SkipRecap               bool   `config:"SkipRecap"`
	ScoreOnCompletion       bool   `config:"ScoreOnCompletion"`
	SaveMpvSpeed            bool   `config:"SaveMpvSpeed"`
	DiscordPresence         bool   `config:"DiscordPresence"`
}

// LoadConfig reads or creates the config file, adds missing fields, and returns the populated CurdConfig struct
func LoadConfig(configPath string) (CurdConfig, error) {
	configPath = os.ExpandEnv(configPath) // Substitute environment variables like $HOME

	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Create the config file with default values if it doesn't exist
		fmt.Println("Config file not found. Creating default config...")
		if err := createDefaultConfig(configPath); err != nil {
			return CurdConfig{}, fmt.Errorf("error creating default config file: %v", err)
		}
	}

	// Load the config from file
	configMap, err := loadConfigFromFile(configPath)
	if err != nil {
		return CurdConfig{}, fmt.Errorf("error loading config file: %v", err)
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

	// Write updated config back to file if there were any missing fields
	if updated {
		if err := saveConfigToFile(configPath, configMap); err != nil {
			return CurdConfig{}, fmt.Errorf("error saving updated config file: %v", err)
		}
	}

	// Populate the CurdConfig struct from the config map
	config := populateConfig(configMap)

	return config, nil
}

// Default configuration values as a map
func defaultConfigMap() map[string]string {
	return map[string]string{
		"Player":                  "mpv",
		"HistoryFile":             "$HOME/.local/share/curd/curd_history.txt",
		"SubsLanguage":            "english",
		"SubOrDub":                "sub",
		"PercentageToMarkComplete": "90",
		"NextEpisodePrompt":       "true",
		"SkipOp":                  "true",
		"SkipEd":                  "true",
		"SkipFiller":              "true",
		"SkipRecap":               "true",
		"ScoreOnCompletion":       "true",
		"SaveMpvSpeed":            "true",
		"DiscordPresence":         "true",
	}
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

// Load config file from disk into a map (key=value format)
func loadConfigFromFile(path string) (map[string]string, error) {
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
