package internal

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func GetTokenFromRofi() (string, error) {
	// The URL to open - using authorization code flow for consistency
	url := "https://anilist.co/api/v2/oauth/authorize?client_id=20686&response_type=code&redirect_uri=http://localhost:8000/oauth/callback"

	// Use rofi to display a prompt with the URL
	message := "Press enter to open the anilist token page in your browser"
	_, err := GetUserInputFromRofi(message)
	if err != nil {
		return "", err
	}

	// Open the URL in the default browser
	err = exec.Command("xdg-open", url).Start()
	if err != nil {
		return "", err
	}

	// Use rofi again to get the token input from the user
	token, err := GetUserInputFromRofi("Enter the token (from the redirect URL after 'access_token=')")
	if err != nil {
		return "", err
	}

	return token, nil
}

// GetUserInputFromRofi prompts the user for input using Rofi with a custom message
func GetUserInputFromRofi(message string) (string, error) {
	userCurdConfig := GetGlobalConfig()
	if userCurdConfig.StoragePath == "" {
		userCurdConfig.StoragePath = os.ExpandEnv("${HOME}/.local/share/curd")
	}
	// Create the Rofi command
	cmd := exec.Command("rofi", "-dmenu", "-theme", filepath.Join(os.ExpandEnv(userCurdConfig.StoragePath), "userinput.rasi"), "-p", "Input", "-mesg", message)

	// Set up pipes for output
	var out bytes.Buffer
	cmd.Stdout = &out

	// Run the command
	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("failed to run Rofi: %w", err)
	}

	// Get the entered input
	userInput := strings.TrimSpace(out.String())

	return userInput, nil
}
