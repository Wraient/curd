package internal

import (
	"os/exec"
)

func GetTokenFromRofi() (string, error) {
	// The URL to open
	url := "https://anilist.co/api/v2/oauth/authorize?client_id=20686&response_type=token"

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
	token, err := GetUserInputFromRofi("Enter the token:")
	if err != nil {
		return "", err
	}

	return token, nil
}
