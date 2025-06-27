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
	MpvArgs                  []string `config:"MpvArgs"`
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
	configMap, err := loadConfigFromFile(configPath)
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
		if err := saveConfigToFile(configPath, configMap); err != nil {
			return CurdConfig{}, fmt.Errorf("error saving updated config file: %v", err)
		}
	}

	// Parse string arrays
	if mpvArgs, exists := configMap["MpvArgs"]; exists {
		configMap["MpvArgs"] = mpvArgs
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

func getTokenFromTempFile(isWindowsPlatform bool) (string, error) {
	if isWindowsPlatform {
		return getTokenFromPowerShell()
	}

	// Create a temporary file for the token (macOS and other platforms)
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

	// Open the file with appropriate editor (macOS and other platforms)
	var cmd *exec.Cmd
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "nano" // Default to nano if $EDITOR is not set
	}
	cmd = exec.Command(editor, tempPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

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

// getTokenFromPowerShell uses PowerShell to get token input on Windows
func getTokenFromPowerShell() (string, error) {
	// PowerShell script that shows a user-friendly input dialog
	psScript := `
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing

# Create the form
$form = New-Object System.Windows.Forms.Form
$form.Text = "Curd - AniList Token"
$form.Size = New-Object System.Drawing.Size(600, 300)
$form.StartPosition = "CenterScreen"
$form.FormBorderStyle = "FixedDialog"
$form.MaximizeBox = $false
$form.MinimizeBox = $false
$form.TopMost = $true

# Create instruction label
$label = New-Object System.Windows.Forms.Label
$label.Location = New-Object System.Drawing.Point(10, 10)
$label.Size = New-Object System.Drawing.Size(560, 60)
$label.Text = "Please generate a token from:" + [Environment]::NewLine + "https://anilist.co/api/v2/oauth/authorize?client_id=20686&response_type=token" + [Environment]::NewLine + [Environment]::NewLine + "Then paste your token below:"
$form.Controls.Add($label)

# Create URL button
$urlButton = New-Object System.Windows.Forms.Button
$urlButton.Location = New-Object System.Drawing.Point(10, 80)
$urlButton.Size = New-Object System.Drawing.Size(200, 30)
$urlButton.Text = "Open AniList Token Page"
$urlButton.Add_Click({
    Start-Process "https://anilist.co/api/v2/oauth/authorize?client_id=20686&response_type=token"
})
$form.Controls.Add($urlButton)

# Create token textbox
$textBox = New-Object System.Windows.Forms.TextBox
$textBox.Location = New-Object System.Drawing.Point(10, 120)
$textBox.Size = New-Object System.Drawing.Size(560, 60)
$textBox.Multiline = $true
$textBox.ScrollBars = "Vertical"
$textBox.Text = "Paste your AniList token here..."
$textBox.ForeColor = [System.Drawing.Color]::Gray
$textBox.Add_GotFocus({
    if ($textBox.Text -eq "Paste your AniList token here...") {
        $textBox.Text = ""
        $textBox.ForeColor = [System.Drawing.Color]::Black
    }
})
$textBox.Add_LostFocus({
    if ($textBox.Text -eq "") {
        $textBox.Text = "Paste your AniList token here..."
        $textBox.ForeColor = [System.Drawing.Color]::Gray
    }
})
$form.Controls.Add($textBox)

# Create OK button
$okButton = New-Object System.Windows.Forms.Button
$okButton.Location = New-Object System.Drawing.Point(400, 200)
$okButton.Size = New-Object System.Drawing.Size(80, 30)
$okButton.Text = "OK"
$okButton.DialogResult = [System.Windows.Forms.DialogResult]::OK
$form.AcceptButton = $okButton
$form.Controls.Add($okButton)

# Create Cancel button
$cancelButton = New-Object System.Windows.Forms.Button
$cancelButton.Location = New-Object System.Drawing.Point(490, 200)
$cancelButton.Size = New-Object System.Drawing.Size(80, 30)
$cancelButton.Text = "Cancel"
$cancelButton.DialogResult = [System.Windows.Forms.DialogResult]::Cancel
$form.CancelButton = $cancelButton
$form.Controls.Add($cancelButton)

# Show the form and get result
$result = $form.ShowDialog()

if ($result -eq [System.Windows.Forms.DialogResult]::OK) {
    $token = $textBox.Text.Trim()
    if ($token -eq "" -or $token -eq "Paste your AniList token here...") {
        Write-Error "No token provided"
        exit 1
    }
    Write-Output $token
} else {
    Write-Error "Token input cancelled"
    exit 1
}
`

	// Execute PowerShell script
	cmd := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", psScript)
	cmd.Stdin = os.Stdin

	output, err := cmd.Output()
	if err != nil {
		// Fallback to simple console input if PowerShell dialog fails
		Log(fmt.Sprintf("PowerShell dialog failed, falling back to console input: %v", err))
		return getTokenFromConsole()
	}

	token := strings.TrimSpace(string(output))
	if token == "" {
		return "", fmt.Errorf("no token provided")
	}

	return token, nil
}

// getTokenFromConsole provides fallback console input for Windows
func getTokenFromConsole() (string, error) {
	fmt.Println("Please generate a token from: https://anilist.co/api/v2/oauth/authorize?client_id=20686&response_type=token")
	fmt.Print("Enter your AniList token: ")

	var token string
	_, err := fmt.Scanln(&token)
	if err != nil {
		return "", fmt.Errorf("error reading token: %v", err)
	}

	return strings.TrimSpace(token), nil
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

	// Handle MpvArgs specially
	if mpvArgs, exists := configMap["MpvArgs"]; exists {
		config.MpvArgs = parseStringArray(mpvArgs)
	}

	return config
}

func getOrderedCategories(userCurdConfig *CurdConfig) map[string]string {
	// Define the default categories and their labels
	defaultOrder := []string{"CURRENT", "ALL", "UNTRACKED", "UPDATE", "CONTINUE_LAST"}
	defaultLabels := map[string]string{
		"CURRENT":       "Currently Watching",
		"ALL":           "Show All",
		"UNTRACKED":     "Untracked Watching",
		"UPDATE":        "Update (Episode, Status, Score)",
		"CONTINUE_LAST": "Continue Last Session",
	}

	// Create ordered map to store final result
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

	// Create the final ordered map
	orderedMap := make(map[string]string)
	for _, key := range finalOrder {
		orderedMap[key] = defaultLabels[key]
	}

	return orderedMap
}
