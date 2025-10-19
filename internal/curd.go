package internal

import (
	"bufio"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/gen2brain/beeep"
)

// Global variable to store the last selected category for back navigation
var lastSelectedCategory SelectionOption

func EditConfig(configFilePath string) {
	// Get the user's preferred editor from the EDITOR environment variable
	editor := os.Getenv("EDITOR")
	if editor == "" {
		// If EDITOR is not set, use system-specific defaults
		if runtime.GOOS == "windows" {
			// Try Notepad++ first
			if _, err := exec.LookPath("notepad++"); err == nil {
				editor = "notepad++"
			} else {
				editor = "notepad.exe"
			}
		} else {
			if _, err := exec.LookPath("vim"); err == nil {
				editor = "vim"
			} else {
				editor = "nano"
			}
		}
	}

	// Construct the command to open the config file
	cmd := exec.Command(editor, configFilePath)

	// Set the command to run in the current terminal
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Run the editor command
	err := cmd.Run()
	if err != nil {
		CurdOut(fmt.Sprintf("Error opening config file: %v", err))
		return
	}

	CurdOut("Config file edited successfully.")
}

// ClearLogFile removes all contents from the specified log file
func ClearLogFile(logFile string) error {
	// Open the file with truncate flag to clear its contents
	file, err := os.OpenFile(logFile, os.O_WRONLY|os.O_TRUNC, 0666)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer file.Close()

	return nil
}

// LogData logs the input data into a specified log file with the format [LOG] time lineNumber: logData
func Log(data interface{}) error {
	logFile := GetGlobalLogFile()
	// Open or create the log file
	file, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		return err
	}
	defer file.Close()

	// Attempt to marshal the data into JSON
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	// Get the caller information
	_, filename, lineNumber, ok := runtime.Caller(1) // Caller 1 gives the caller of LogData
	if !ok {
		return fmt.Errorf("unable to get caller information")
	}

	// Log the current time and the JSON representation along with caller info
	currentTime := time.Now().Format("2006/01/02 15:04:05")
	logMessage := fmt.Sprintf("[LOG] %s %s:%d: %s\n", currentTime, filename, lineNumber, jsonData)
	_, err = fmt.Fprint(file, logMessage) // Write to the file
	if err != nil {
		return err
	}

	return nil
}

// ClearScreen clears the terminal screen and saves the state
func ClearScreen() {
	userCurdConfig := GetGlobalConfig()

	if userCurdConfig.AlternateScreen == false {
		return
	}

	fmt.Print("\033[?1049h") // Switch to alternate screen buffer
	fmt.Print("\033[2J")     // Clear the entire screen
	fmt.Print("\033[H")      // Move cursor to the top left
}

// RestoreScreen restores the original terminal state
func RestoreScreen() {
	userCurdConfig := GetGlobalConfig()

	if userCurdConfig.AlternateScreen == false {
		return
	}

	fmt.Print("\033[?1049l") // Switch back to the main screen buffer
}

func ExitCurd(err error) {
	RestoreScreen()

	anime := GetGlobalAnime()
	if (anime != nil) && (anime.Ep.Player.SocketPath != "") {
		_, err = MPVSendCommand(anime.Ep.Player.SocketPath, []interface{}{"quit"})
		if err != nil {
			Log("Error closing MPV: " + err.Error())
		}
	}

	CurdOut("Have a great day!")
	// If the error is not about the connection refused, print the error
	if err != nil && !strings.Contains(err.Error(), "dial unix "+anime.Ep.Player.SocketPath+": connect: connection refused") {
		CurdOut(fmt.Sprintf("Error: %v", err))
		if runtime.GOOS == "windows" {
			fmt.Println("Press Enter to exit")
			var wait string
			fmt.Scanln(&wait)
			os.Exit(1)
		} else {
			os.Exit(1)
		}
	}
	os.Exit(0)
}

func CurdOut(data interface{}) {
	userCurdConfig := GetGlobalConfig()
	if userCurdConfig == nil {
		userCurdConfig = &CurdConfig{}
	}
	if !userCurdConfig.RofiSelection {
		fmt.Println(fmt.Sprintf("%v", data))
	} else {
		switch runtime.GOOS {
		case "windows":
			err := beeep.Notify(
				"Curd",
				fmt.Sprintf("%v", data),
				"",
			)

			if err != nil {
				Log(fmt.Sprintf("Failed to send notification: %v", err))
			}
		case "linux":
			// Check if the input starts with "-i" for image notification
			dataStr := fmt.Sprintf("%v", data)
			if strings.HasPrefix(dataStr, "-i") && userCurdConfig.ImagePreview && userCurdConfig.RofiSelection {
				// Split the string to get image path and message
				parts := strings.SplitN(dataStr, " ", 3)
				if len(parts) == 3 {
					// Remove quotes from the message
					message := strings.Trim(parts[2], "\"")
					cmd := exec.Command("notify-send",
						"-a", "Curd",
						"-h", "string:x-canonical-private-synchronous:curd-notification",
						"Curd",
						"-i", parts[1],
						message)
					err := cmd.Run()
					if err != nil {
						Log(fmt.Sprintf("%v", cmd))
						Log(fmt.Sprintf("Failed to send notification: %v", err))
					}
				}
			} else {
				cmd := exec.Command("notify-send",
					"-a", "Curd",
					"-h", "string:x-canonical-private-synchronous:curd-notification",
					"Curd",
					dataStr)
				err := cmd.Run()
				if err != nil {
					Log(fmt.Sprintf("%v", cmd))
					Log(fmt.Sprintf("Failed to send notification: %v", err))
				}
			}
		}
	}
}

func UpdateAnimeEntry(userCurdConfig *CurdConfig, user *User) {
	// Create update options
	updateOptions := []SelectionOption{
		{Key: "-2", Label: "← Back"},
		{Key: "CATEGORY", Label: "Change Anime Category"},
		{Key: "PROGRESS", Label: "Change Progress"},
		{Key: "SCORE", Label: "Add/Change Score"},
	}

	// Select update option
	updateSelection, err := DynamicSelect(updateOptions)
	if err != nil {
		Log(fmt.Sprintf("Failed to select update option: %v", err))
		ExitCurd(fmt.Errorf("Failed to select update option"))
	}

	if updateSelection.Key == "-1" {
		ExitCurd(nil)
	}

	// Handle back button
	if updateSelection.Key == "-2" {
		return
	}

	// Get user's anime list
	var animeListOptions []SelectionOption
	var animeListMapPreview map[string]RofiSelectPreview

	if userCurdConfig.RofiSelection && userCurdConfig.ImagePreview {
		animeListMapPreview = make(map[string]RofiSelectPreview)
		// Include anime from all categories
		for _, entry := range user.AnimeList.Watching {
			title := entry.Media.Title.English
			if title == "" || userCurdConfig.AnimeNameLanguage == "romaji" {
				title = entry.Media.Title.Romaji
			}
			animeListMapPreview[strconv.Itoa(entry.Media.ID)] = RofiSelectPreview{
				Title:      title,
				CoverImage: entry.CoverImage,
			}
		}
		for _, entry := range user.AnimeList.Completed {
			title := entry.Media.Title.English
			if title == "" || userCurdConfig.AnimeNameLanguage == "romaji" {
				title = entry.Media.Title.Romaji
			}
			animeListMapPreview[strconv.Itoa(entry.Media.ID)] = RofiSelectPreview{
				Title:      title,
				CoverImage: entry.CoverImage,
			}
		}
		for _, entry := range user.AnimeList.Paused {
			title := entry.Media.Title.English
			if title == "" || userCurdConfig.AnimeNameLanguage == "romaji" {
				title = entry.Media.Title.Romaji
			}
			animeListMapPreview[strconv.Itoa(entry.Media.ID)] = RofiSelectPreview{
				Title:      title,
				CoverImage: entry.CoverImage,
			}
		}
		for _, entry := range user.AnimeList.Dropped {
			title := entry.Media.Title.English
			if title == "" || userCurdConfig.AnimeNameLanguage == "romaji" {
				title = entry.Media.Title.Romaji
			}
			animeListMapPreview[strconv.Itoa(entry.Media.ID)] = RofiSelectPreview{
				Title:      title,
				CoverImage: entry.CoverImage,
			}
		}
		for _, entry := range user.AnimeList.Planning {
			title := entry.Media.Title.English
			if title == "" || userCurdConfig.AnimeNameLanguage == "romaji" {
				title = entry.Media.Title.Romaji
			}
			animeListMapPreview[strconv.Itoa(entry.Media.ID)] = RofiSelectPreview{
				Title:      title,
				CoverImage: entry.CoverImage,
			}
		}
		for _, entry := range user.AnimeList.Rewatching {
			title := entry.Media.Title.English
			if title == "" || userCurdConfig.AnimeNameLanguage == "romaji" {
				title = entry.Media.Title.Romaji
			}
			animeListMapPreview[strconv.Itoa(entry.Media.ID)] = RofiSelectPreview{
				Title:      title,
				CoverImage: entry.CoverImage,
			}
		}
	} else {
		animeListOptions = make([]SelectionOption, 0)
		// Include anime from all categories
		for _, entry := range user.AnimeList.Watching {
			title := entry.Media.Title.English
			if title == "" || userCurdConfig.AnimeNameLanguage == "romaji" {
				title = entry.Media.Title.Romaji
			}
			animeListOptions = append(animeListOptions, SelectionOption{
				Key:   strconv.Itoa(entry.Media.ID),
				Label: title,
			})
		}
		for _, entry := range user.AnimeList.Completed {
			title := entry.Media.Title.English
			if title == "" || userCurdConfig.AnimeNameLanguage == "romaji" {
				title = entry.Media.Title.Romaji
			}
			animeListOptions = append(animeListOptions, SelectionOption{
				Key:   strconv.Itoa(entry.Media.ID),
				Label: title,
			})
		}
		for _, entry := range user.AnimeList.Paused {
			title := entry.Media.Title.English
			if title == "" || userCurdConfig.AnimeNameLanguage == "romaji" {
				title = entry.Media.Title.Romaji
			}
			animeListOptions = append(animeListOptions, SelectionOption{
				Key:   strconv.Itoa(entry.Media.ID),
				Label: title,
			})
		}
		for _, entry := range user.AnimeList.Dropped {
			title := entry.Media.Title.English
			if title == "" || userCurdConfig.AnimeNameLanguage == "romaji" {
				title = entry.Media.Title.Romaji
			}
			animeListOptions = append(animeListOptions, SelectionOption{
				Key:   strconv.Itoa(entry.Media.ID),
				Label: title,
			})
		}
		for _, entry := range user.AnimeList.Planning {
			title := entry.Media.Title.English
			if title == "" || userCurdConfig.AnimeNameLanguage == "romaji" {
				title = entry.Media.Title.Romaji
			}
			animeListOptions = append(animeListOptions, SelectionOption{
				Key:   strconv.Itoa(entry.Media.ID),
				Label: title,
			})
		}
		for _, entry := range user.AnimeList.Rewatching {
			title := entry.Media.Title.English
			if title == "" || userCurdConfig.AnimeNameLanguage == "romaji" {
				title = entry.Media.Title.Romaji
			}
			animeListOptions = append(animeListOptions, SelectionOption{
				Key:   strconv.Itoa(entry.Media.ID),
				Label: title,
			})
		}
	}

	// Select anime to update
	var selectedAnime SelectionOption
	if userCurdConfig.RofiSelection && userCurdConfig.ImagePreview {
		selectedAnime, err = DynamicSelectPreview(animeListMapPreview, false)
	} else {
		// Add back option
		animeListOptions = append([]SelectionOption{
			{Key: "-2", Label: "← Back"},
		}, animeListOptions...)
		selectedAnime, err = DynamicSelect(animeListOptions)
	}
	if err != nil {
		Log(fmt.Sprintf("Failed to select anime: %v", err))
		ExitCurd(fmt.Errorf("Failed to select anime"))
	}

	if selectedAnime.Key == "-1" {
		ExitCurd(nil)
	}

	// Handle back button - recursively call UpdateAnimeEntry
	if selectedAnime.Key == "-2" {
		ClearScreen()
		UpdateAnimeEntry(userCurdConfig, user)
		return
	}

	animeID, err := strconv.Atoi(selectedAnime.Key)
	if err != nil {
		Log(fmt.Sprintf("Failed to convert anime ID: %v", err))
		ExitCurd(fmt.Errorf("Failed to convert anime ID"))
	}

	// After getting animeID, get the current anime entry
	selectedAnilistAnime, err := FindAnimeByAnilistID(user.AnimeList, selectedAnime.Key)
	if err != nil {
		Log(fmt.Sprintf("Can not find the anime in anilist animelist: %v", err))
		ExitCurd(fmt.Errorf("Can not find the anime in anilist animelist"))
	}
	ClearScreen()
	switch updateSelection.Key {
	case "CATEGORY":
		categories := []SelectionOption{
			{Key: "-2", Label: "← Back"},
			{Key: "CURRENT", Label: "Currently Watching"},
			{Key: "COMPLETED", Label: "Completed"},
			{Key: "PAUSED", Label: "On Hold"},
			{Key: "DROPPED", Label: "Dropped"},
			{Key: "PLANNING", Label: "Plan to Watch"},
			{Key: "REPEATING", Label: "Rewatching"}, // Anilist uses REPEATING for rewatching
		}

		currentStatus := "None"
		if selectedAnilistAnime.Status != "" {
			// Find the label for the current status
			for _, cat := range categories {
				if cat.Key == selectedAnilistAnime.Status {
					currentStatus = cat.Label
					break
				}
			}
		}
		CurdOut(fmt.Sprintf("Current category: %s", currentStatus))

		categorySelection, err := DynamicSelect(categories)
		if err != nil {
			Log(fmt.Sprintf("Failed to select category: %v", err))
			ExitCurd(fmt.Errorf("Failed to select category"))
		}

		if categorySelection.Key == "-1" {
			ExitCurd(nil)
		}

		// Handle back button
		if categorySelection.Key == "-2" {
			ClearScreen()
			UpdateAnimeEntry(userCurdConfig, user)
			return
		}

		err = UpdateProviderAnimeStatus(userCurdConfig, user.Token, animeID, categorySelection.Key)
		if err != nil {
			Log(fmt.Sprintf("Failed to update anime status: %v", err))
			ExitCurd(fmt.Errorf("Failed to update anime status"))
		}

	case "PROGRESS":
		currentProgress := "None"
		if selectedAnilistAnime.Progress > 0 {
			currentProgress = strconv.Itoa(selectedAnilistAnime.Progress)
		}

		var progress string
		if userCurdConfig.RofiSelection {
			progress, err = GetUserInputFromRofi(fmt.Sprintf("Current progress: %s\nEnter new progress (episode number)", currentProgress))
			if err != nil {
				Log(fmt.Sprintf("Failed to get progress input: %v", err))
				ExitCurd(fmt.Errorf("Failed to get progress input"))
			}
		} else {
			CurdOut(fmt.Sprintf("Current progress: %s", currentProgress))
			CurdOut("Enter new progress (episode number):")
			fmt.Scanln(&progress)
		}

		progressNum, err := strconv.Atoi(progress)
		if err != nil {
			Log(fmt.Sprintf("Failed to convert progress to number: %v", err))
			ExitCurd(fmt.Errorf("Failed to convert progress to number"))
		}

		err = UpdateProviderAnimeProgress(userCurdConfig, user.Token, animeID, progressNum)
		if err != nil {
			Log(fmt.Sprintf("Failed to update anime progress: %v", err))
			ExitCurd(fmt.Errorf("Failed to update anime progress"))
		}

	case "SCORE":
		currentScore := "None"
		if selectedAnilistAnime.Score > 0 {
			currentScore = strconv.Itoa(int(selectedAnilistAnime.Score))
		}
		CurdOut(fmt.Sprintf("Current score: %s", currentScore))

		err = RateProviderAnimeWithPrompt(userCurdConfig, user.Token, animeID)
		if err != nil {
			Log(fmt.Sprintf("Failed to update anime score: %v", err))
			ExitCurd(fmt.Errorf("Failed to update anime score"))
		}
	}

	CurdOut("Anime updated successfully!")
}

func UpdateCurd(repo, fileName string) error {
	// Get the path of the currently running executable
	executablePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("unable to find current executable: %v", err)
	}

	// Determine the correct binary name based on OS and architecture
	var binaryName string
	switch runtime.GOOS {
	case "windows":
		if runtime.GOARCH == "arm64" {
			binaryName = "curd-windows-arm64.exe"
		} else {
			binaryName = "curd-windows-x86_64.exe"
		}
	case "darwin": // macOS
		switch runtime.GOARCH {
		case "amd64":
			binaryName = "curd-macos-x86_64"
		case "arm64":
			binaryName = "curd-macos-arm64"
		default:
			binaryName = "curd-macos-universal"
		}
	case "linux":
		switch runtime.GOARCH {
		case "amd64":
			binaryName = "curd-linux-x86_64"
		case "arm64":
			binaryName = "curd-linux-arm64"
		default:
			return fmt.Errorf("unsupported Linux architecture: %s", runtime.GOARCH)
		}
	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}

	// GitHub release URL for curd
	url := fmt.Sprintf("https://github.com/%s/releases/latest/download/%s", repo, binaryName)

	// Temporary path for the downloaded curd executable
	tmpPath := executablePath + ".tmp"

	// Download the curd executable
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to download file: %v", err)
	}
	defer resp.Body.Close()

	// Check if the download was successful
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download file: received status code %d", resp.StatusCode)
	}

	// Create a new temporary file
	out, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %v", err)
	}
	defer out.Close()

	// Set file permissions
	if err := out.Chmod(0755); err != nil {
		return fmt.Errorf("failed to set file permissions: %v", err)
	}

	// Copy the downloaded content to the temporary file
	if _, err := io.Copy(out, resp.Body); err != nil {
		return fmt.Errorf("failed to write to temporary file: %v", err)
	}

	// Close the file before renaming
	out.Close()

	// Replace the old executable with the new one
	if runtime.GOOS == "windows" {
		// On Windows, we need to rename the old file first
		oldPath := executablePath + ".old"
		err = os.Rename(executablePath, oldPath)
		if err != nil {
			return fmt.Errorf("failed to rename old executable: %v", err)
		}
		err = os.Rename(tmpPath, executablePath)
		if err != nil {
			// Try to restore the old executable if the rename fails
			os.Rename(oldPath, executablePath)
			return fmt.Errorf("failed to rename new executable: %v", err)
		}
		os.Remove(oldPath)
	} else {
		// On Unix systems, we can directly rename
		if err := os.Rename(tmpPath, executablePath); err != nil {
			return fmt.Errorf("failed to replace executable: %v", err)
		}
	}

	return nil
}

func AddNewAnime(userCurdConfig *CurdConfig, anime *Anime, user *User, databaseAnimes *[]Anime) SelectionOption {
	var query string
	// Remove the redeclared variable declaration since animeOptions is already declared above
	var animeMapPreview map[string]RofiSelectPreview
	var animeOptions []SelectionOption
	var err error
	var anilistSelectedOption SelectionOption

	if userCurdConfig.RofiSelection {
		userInput, err := GetUserInputFromRofi("Enter the anime name")
		if err != nil {
			Log("Error getting user input: " + err.Error())
			ExitCurd(fmt.Errorf("Error getting user input: " + err.Error()))
		}
		query = userInput
	} else {
		CurdOut("Enter the anime name:")
		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		query = strings.TrimSpace(input)
	}
	// Search using provider abstraction
	if userCurdConfig.RofiSelection && userCurdConfig.ImagePreview {
		// For image preview, use AniList-specific function for now
		// MAL doesn't provide preview in the same way
		animeMapPreview, err = SearchAnimeAnilistPreview(query, user.Token)
	} else {
		animeOptions, err = SearchProviderAnime(userCurdConfig, user.Token, query)
		if err != nil {
			Log(fmt.Sprintf("Failed to search anime: %v", err))
			ExitCurd(fmt.Errorf("Failed to search anime"))
		}
	}
	if err != nil {
		Log(fmt.Sprintf("Failed to search anime: %v", err))
		ExitCurd(fmt.Errorf("Failed to search anime"))
	}
	if userCurdConfig.RofiSelection && userCurdConfig.ImagePreview {
		anilistSelectedOption, err = DynamicSelectPreview(animeMapPreview, false)
	} else {
		// Add back option
		animeOptions = append([]SelectionOption{
			{Key: "-2", Label: "← Back"},
		}, animeOptions...)
		anilistSelectedOption, err = DynamicSelect(animeOptions)
	}

	if anilistSelectedOption.Key == "-1" {
		ExitCurd(nil)
	}

	// Handle back button - return special marker
	if anilistSelectedOption.Key == "-2" {
		return SelectionOption{Key: "-2", Label: "← Back"}
	}

	if err != nil {
		Log(fmt.Sprintf("No anime available: %v", err))
		ExitCurd(fmt.Errorf("No anime available"))
	}
	animeID, err := strconv.Atoi(anilistSelectedOption.Key)
	if err != nil {
		Log(fmt.Sprintf("Failed to convert anime ID to integer: %v", err))
		ExitCurd(fmt.Errorf("Failed to convert anime ID to integer"))
	}

	// Add category selection before adding to list
	categories := []SelectionOption{
		{Key: "-2", Label: "← Back"},
		{Key: "CURRENT", Label: "Currently Watching"},
		{Key: "COMPLETED", Label: "Completed"},
		{Key: "PAUSED", Label: "On Hold"},
		{Key: "DROPPED", Label: "Dropped"},
		{Key: "PLANNING", Label: "Plan to Watch"},
		{Key: "REPEATING", Label: "Rewatching"}, // Anilist uses REPEATING for rewatching
	}

	ClearScreen()
	CurdOut("Select which list to add the anime to:")

	categorySelection, err := DynamicSelect(categories)
	if err != nil {
		Log(fmt.Sprintf("Failed to select category: %v", err))
		ExitCurd(fmt.Errorf("Failed to select category"))
	}

	if categorySelection.Key == "-1" {
		ExitCurd(nil)
	}

	// Handle back button - return special marker
	if categorySelection.Key == "-2" {
		return SelectionOption{Key: "-2", Label: "← Back"}
	}

	err = UpdateProviderAnimeStatus(userCurdConfig, user.Token, animeID, categorySelection.Key)
	if err != nil {
		Log(fmt.Sprintf("Failed to add anime to list: %v", err))
		ExitCurd(fmt.Errorf("Failed to add anime to list"))
	}

	// Refresh user's anime list after adding
	user.AnimeList, err = GetProviderAnimeList(userCurdConfig, user.Token, user.Id)
	if err != nil {
		Log(fmt.Sprintf("Failed to refresh anime list: %v", err))
		ExitCurd(fmt.Errorf("Failed to refresh anime list"))
	}

	return anilistSelectedOption
}

func SetupCurd(userCurdConfig *CurdConfig, anime *Anime, user *User, databaseAnimes *[]Anime) {
	var err error

	// Filter anime list based on selected category
	var animeListOptions []SelectionOption
	var animeListMapPreview map[string]RofiSelectPreview

	// Get user id, username using provider abstraction
	user.Id, user.Username, err = GetProviderUserID(userCurdConfig, user.Token)
	if err != nil {
		Log(fmt.Sprintf("Failed to get user ID: %v", err))
		ExitCurd(fmt.Errorf("Failed to get user ID\nYou can reset the token by running `curd -change-token`"))
	}

	// Get the anime list data using provider abstraction
	user.AnimeList, err = GetProviderAnimeList(userCurdConfig, user.Token, user.Id)
	if err != nil {
		Log(fmt.Sprintf("Failed to get anime list: %v", err))
		ExitCurd(fmt.Errorf("Failed to get anime list\nYou can reset the token by running `curd -change-token`"))
	}

	// Declare selectedAnilistAnime for use throughout the function
	var selectedAnilistAnime *Entry

	// If continueLast flag is set, directly get the last watched anime
	if anime.Ep.ContinueLast {
		// Get the last anime ID from the curd_id file
		idFilePath := filepath.Join(os.ExpandEnv(userCurdConfig.StoragePath), "curd_id")
		idBytes, err := os.ReadFile(idFilePath)
		if err != nil {
			Log("Error reading curd_id file: " + err.Error())
			ExitCurd(fmt.Errorf("No last watched anime found"))
		}

		anilistID, err := strconv.Atoi(string(idBytes))
		if err != nil {
			Log("Error converting anilist ID: " + err.Error())
			ExitCurd(fmt.Errorf("Invalid anime ID in curd_id file"))
		}

		// Find the anime in database
		animePointer := LocalFindAnime(*databaseAnimes, anilistID, "")
		if animePointer == nil {
			ExitCurd(fmt.Errorf("Last watched anime not found in database"))
		}

		// Set the anime details
		anime.AnilistId = animePointer.AnilistId
		// anime.AllanimeId = animePointer.AllanimeId
		// anime.Title = animePointer.Title
		// anime.Ep.Number = animePointer.Ep.Number
		// anime.Ep.Player.PlaybackTime = animePointer.Ep.Player.PlaybackTime
		// anime.Ep.Resume = true

		// Get the anime entry from user's list
		selectedAnilistAnime, err = FindAnimeByAnilistID(user.AnimeList, strconv.Itoa(anilistID))
		if err != nil {
			Log(fmt.Sprintf("Can not find the anime in provider animelist: %v", err))
			ExitCurd(fmt.Errorf("Can not find the anime in provider animelist"))
		}

		// Set anime entry
		anime.Title = selectedAnilistAnime.Media.Title
		anime.TotalEpisodes = selectedAnilistAnime.Media.Episodes

	} else {
		// Navigation loop: allow going back to category selection from anime selection
		categorySelected := false
		for !categorySelected {
			// Skip category selection if Current flag is set OR if we have a last selected category (back navigation)
			var categorySelection SelectionOption
			if userCurdConfig.CurrentCategory {
				categorySelection = SelectionOption{
					Key:   "CURRENT",
					Label: "Currently Watching",
				}
			} else if lastSelectedCategory.Key != "" {
				// Use the last selected category (user pressed back from episode playback)
				categorySelection = lastSelectedCategory
				lastSelectedCategory = SelectionOption{} // Reset after use
				CurdOut(fmt.Sprintf("Returning to %s...", categorySelection.Label))
			} else {
				// Create category selection map
				// Get ordered categories
				orderedCategories := getOrderedCategories(userCurdConfig)

				// Use DynamicSelect with ordered categories directly
				categorySelection, err = DynamicSelect(orderedCategories)

				if err != nil {
					Log(fmt.Sprintf("Failed to select category: %v", err))
					ExitCurd(fmt.Errorf("Failed to select category"))
				}

				if categorySelection.Key == "-1" {
					ExitCurd(nil)
				}

				// Handle options
				if categorySelection.Key == "UPDATE" {
					ClearScreen()
					UpdateAnimeEntry(userCurdConfig, user)
					// Don't exit - let the loop continue to allow going back
					ClearScreen()
					continue
				} else if categorySelection.Key == "UNTRACKED" {
					ClearScreen()
					WatchUntracked(userCurdConfig)
					// Don't exit - let the loop continue to allow going back
					ClearScreen()
					continue
				} else if categorySelection.Key == "CONTINUE_LAST" {
					anime.Ep.ContinueLast = true
				}

				ClearScreen()
			}

			// Save the category selection for potential back navigation
			lastSelectedCategory = categorySelection

			if userCurdConfig.RofiSelection && userCurdConfig.ImagePreview {
				animeListMapPreview = make(map[string]RofiSelectPreview)
				for _, entry := range getEntriesByCategory(user.AnimeList, categorySelection.Key) {
					title := entry.Media.Title.English
					if title == "" || userCurdConfig.AnimeNameLanguage == "romaji" {
						title = entry.Media.Title.Romaji
					}
					animeListMapPreview[strconv.Itoa(entry.Media.ID)] = RofiSelectPreview{
						Title:      title,
						CoverImage: entry.CoverImage,
					}
				}
			} else {
				animeListOptions = make([]SelectionOption, 0)
				for _, entry := range getEntriesByCategory(user.AnimeList, categorySelection.Key) {
					title := entry.Media.Title.English
					if title == "" || userCurdConfig.AnimeNameLanguage == "romaji" {
						title = entry.Media.Title.Romaji
					}
					animeListOptions = append(animeListOptions, SelectionOption{
						Key:   strconv.Itoa(entry.Media.ID),
						Label: title,
					})
				}
			}

			// Anime selection with back option
			var anilistSelectedOption SelectionOption
			if userCurdConfig.RofiSelection && userCurdConfig.ImagePreview {
				anilistSelectedOption, err = DynamicSelectPreview(animeListMapPreview, true)
			} else {
				// Add "Back" option at the top
				animeListOptions = append([]SelectionOption{
					{
						Key:   "-2",
						Label: "← Back",
					},
				}, animeListOptions...)

				// Add "Add new anime" option to the slice
				animeListOptions = append(animeListOptions, SelectionOption{
					Key:   "add_new",
					Label: "Add new anime",
				})

				anilistSelectedOption, err = DynamicSelect(animeListOptions)
			}

			if err != nil {
				Log(fmt.Sprintf("Error selecting anime: %v", err))
				ExitCurd(fmt.Errorf("Error selecting anime"))
			}

			if anilistSelectedOption.Key == "-1" {
				ExitCurd(nil)
			}

			// Handle back button
			if anilistSelectedOption.Key == "-2" {
				ClearScreen()
				continue // Go back to category selection
			}

			if anilistSelectedOption.Label == "add_new" || anilistSelectedOption.Key == "add_new" {
				anilistSelectedOption = AddNewAnime(userCurdConfig, anime, user, databaseAnimes)
				// If user pressed back in AddNewAnime, go back to category selection
				if anilistSelectedOption.Key == "-2" {
					ClearScreen()
					continue
				}
			}

			anime.AnilistId, err = strconv.Atoi(anilistSelectedOption.Key)
			if err != nil {
				Log(fmt.Sprintf("Error converting Anilist ID: %v", err))
				ExitCurd(fmt.Errorf("Error converting Anilist ID"))
			}

			// Clear screen after anime selection
			ClearScreen()

			// Successfully selected an anime, exit the loop
			categorySelected = true

			// Store the selected option for later use
			selectedAnilistAnime, err = FindAnimeByAnilistID(user.AnimeList, anilistSelectedOption.Key)
			if err != nil {
				Log(fmt.Sprintf("Can not find the anime in anilist animelist: %v", err))
				ExitCurd(fmt.Errorf("Can not find the anime in anilist animelist"))
			}

			// Set anime entry
			anime.Title = selectedAnilistAnime.Media.Title
			anime.TotalEpisodes = selectedAnilistAnime.Media.Episodes
			anime.Ep.Number = selectedAnilistAnime.Progress + 1

		}
	}

	// Wrap the rest in a function with panic recovery to handle back navigation
	func() {
		defer func() {
			if r := recover(); r != nil {
				if r == "BACK_TO_ANIME_SELECTION" {
					// Restart the SetupCurd to go back to anime selection
					SetupCurd(userCurdConfig, anime, user, databaseAnimes)
					return
				}
				// Re-panic if it's not our signal
				panic(r)
			}
		}()

	var selectedAllanimeAnime SelectionOption
	var userQuery string

	// After the navigation loop, anime.AnilistId and anime.Title are already set
	// Find anime in Local history
	animePointer := LocalFindAnime(*databaseAnimes, anime.AnilistId, "")

	userQuery = anime.Title.Romaji
	var animeList []SelectionOption

	// if anime not found in database, find it in animeList
	if animePointer == nil {
		// Loop to allow going back from streaming source selection
		streamingSourceSelected := false
		for !streamingSourceSelected {
			Log("Anime not found in database, searching in animeList...")
			// Get Anime list (All anime)
			Log(fmt.Sprintf("Searching for anime with query: %s, SubOrDub: %s", userQuery, userCurdConfig.SubOrDub))

			animeList, err = SearchAnime(string(userQuery), userCurdConfig.SubOrDub)
			if err != nil {
				Log(fmt.Sprintf("Failed to select anime: %v", err))
				ExitCurd(fmt.Errorf("Failed to select anime"))
			}
			if len(animeList) == 0 {
				ExitCurd(fmt.Errorf("No results found."))
			}

			// Try to find anime automatically using multiple matching strategies
			// Set timeout for auto-matching (15 seconds)
			matchingStartTime := time.Now()
			matchingTimeout := 15 * time.Second

			found := false
			requiresConfirmation := false

			// Strategy 1: Exact match with episode count (HIGH CONFIDENCE)
			targetLabel := fmt.Sprintf("%v (%d episodes)", userQuery, selectedAnilistAnime.Media.Episodes)
		for i, option := range animeList {
			Log(fmt.Sprintf("Checking option %d: Key='%s', Label='%s'", i, option.Key, option.Label))
			if option.Label == targetLabel {
				anime.AllanimeId = option.Key
				Log(fmt.Sprintf("Found exact match! Setting AllanimeId to: %s", anime.AllanimeId))
				found = true
				requiresConfirmation = false // High confidence, no confirmation needed
				break
			}
		}

		// Check timeout after Strategy 1
		if time.Since(matchingStartTime) > matchingTimeout {
			Log("Auto-matching timeout after Strategy 1. Switching to manual selection.")
			found = false
		}

		// Strategy 2: Check if option label starts with the anime name (MEDIUM CONFIDENCE)
		if !found && time.Since(matchingStartTime) <= matchingTimeout {
			Log("Trying partial name match...")
			normalizedQuery := strings.ToLower(strings.TrimSpace(userQuery))
			for _, option := range animeList {
				normalizedLabel := strings.ToLower(option.Label)
				// Check if the label starts with the query (higher confidence)
				if strings.HasPrefix(normalizedLabel, normalizedQuery) {
					anime.AllanimeId = option.Key
					Log(fmt.Sprintf("Found prefix match! Setting AllanimeId to: %s (matched on '%s')", anime.AllanimeId, option.Label))
					found = true
					requiresConfirmation = false // Prefix match is fairly confident
					break
				}
			}
		}

		// Check timeout after Strategy 2
		if time.Since(matchingStartTime) > matchingTimeout {
			Log("Auto-matching timeout after Strategy 2. Switching to manual selection.")
			found = false
		}

		// Strategy 3: Check if query is contained anywhere in label (LOWER CONFIDENCE)
		if !found && time.Since(matchingStartTime) <= matchingTimeout {
			Log("Trying contains match...")
			normalizedQuery := strings.ToLower(strings.TrimSpace(userQuery))
			for _, option := range animeList {
				normalizedLabel := strings.ToLower(option.Label)
				if strings.Contains(normalizedLabel, normalizedQuery) {
					anime.AllanimeId = option.Key
					Log(fmt.Sprintf("Found contains match! Setting AllanimeId to: %s (matched on '%s')", anime.AllanimeId, option.Label))
					found = true
					requiresConfirmation = true // Lower confidence, ask user to confirm
					break
				}
			}
		}

		// Check timeout after Strategy 3
		if time.Since(matchingStartTime) > matchingTimeout {
			Log("Auto-matching timeout after Strategy 3. Switching to manual selection.")
			found = false
		}

		// Strategy 4: Try with English title if available
		if !found && anime.Title.English != "" && time.Since(matchingStartTime) <= matchingTimeout {
			Log(fmt.Sprintf("Trying with English title: %s", anime.Title.English))
			normalizedEnglish := strings.ToLower(strings.TrimSpace(anime.Title.English))
			for _, option := range animeList {
				normalizedLabel := strings.ToLower(option.Label)
				if strings.HasPrefix(normalizedLabel, normalizedEnglish) {
					anime.AllanimeId = option.Key
					Log(fmt.Sprintf("Found match with English title! Setting AllanimeId to: %s (matched on '%s')", anime.AllanimeId, option.Label))
					found = true
					requiresConfirmation = false // English title prefix is confident
					break
				} else if strings.Contains(normalizedLabel, normalizedEnglish) {
					anime.AllanimeId = option.Key
					Log(fmt.Sprintf("Found partial match with English title! Setting AllanimeId to: %s (matched on '%s')", anime.AllanimeId, option.Label))
					found = true
					requiresConfirmation = true // Contains match needs confirmation
					break
				}
			}
		}

		// Check timeout after Strategy 4
		if time.Since(matchingStartTime) > matchingTimeout {
			Log(fmt.Sprintf("Auto-matching timeout (%.2f seconds). Switching to manual selection.", time.Since(matchingStartTime).Seconds()))
			CurdOut(fmt.Sprintf("Auto-matching took too long (>%.0f seconds), please select manually", matchingTimeout.Seconds()))
			found = false
			anime.AllanimeId = "" // Clear any partial match
		}

		// If automatic matching succeeded and doesn't require confirmation, exit the loop
		if found && !requiresConfirmation && anime.AllanimeId != "" {
			Log(fmt.Sprintf("Auto-match successful! Using AllanimeId: %s", anime.AllanimeId))
			streamingSourceSelected = true
			continue // Exit the streaming source selection loop
		}

		// If automatic matching failed, require manual selection
		if !found {
			Log(fmt.Sprintf("No automatic match found for '%s'. Will require manual selection.", userQuery))
		}

		// Show manual selection if: no match found OR match requires confirmation
		if anime.AllanimeId == "" || requiresConfirmation {
			if requiresConfirmation {
				Log("Match found but confidence is low. Showing options for user confirmation.")
				CurdOut(fmt.Sprintf("Found possible match: %s", anime.AllanimeId))
				CurdOut("Please confirm or select correct anime:")
			} else {
				CurdOut("Failed to automatically select anime")
			}
			// Add back option
			animeList = append([]SelectionOption{
				{Key: "-2", Label: "← Back"},
			}, animeList...)

			selectedAllanimeAnime, err := DynamicSelect(animeList)

			if selectedAllanimeAnime.Key == "-1" {
				ExitCurd(nil)
			}

			// Handle back button - go back to anime selection
			if selectedAllanimeAnime.Key == "-2" {
				ClearScreen()
				// Signal to restart anime selection
				panic("BACK_TO_ANIME_SELECTION")
			}

			if err != nil {
				ExitCurd(fmt.Errorf("No anime available"))
			}
			anime.AllanimeId = selectedAllanimeAnime.Key
			streamingSourceSelected = true
		}
		} // End of streaming source selection loop
	} else {
		// if anime found in database, use it
		anime.AllanimeId = animePointer.AllanimeId
		anime.Ep.Player.PlaybackTime = animePointer.Ep.Player.PlaybackTime
		if anime.Ep.Number == animePointer.Ep.Number {
			anime.Ep.Resume = true
		}
	}

	if selectedAllanimeAnime.Key == "-1" {
		ExitCurd(nil)
	}

	// If anime is not in watching list, prompt user to add it into watching list
	isInWatchingList := false
	for _, entry := range user.AnimeList.Watching {
		if entry.Media.ID == anime.AnilistId {
			isInWatchingList = true
			break
		}
	}

	if !isInWatchingList {
		// Create options for the prompt
		options := []SelectionOption{
			{Key: "yes", Label: "Add to watching list"},
			{Key: "no", Label: "Continue without adding"},
		}

		var selectedOption SelectionOption
		var err error

		// Use rofi for selection
		selectedOption, err = DynamicSelect(options)
		if err != nil {
			Log("Error in selection: " + err.Error())
			ExitCurd(err)
		}

		if selectedOption.Key == "yes" {
			err = AddAnimeToWatchingList(anime.AnilistId, user.Token)
			if err != nil {
				Log("Error adding anime to watching list: " + err.Error())
				ExitCurd(err)
			}
			// Refresh user's anime list after adding
			if userCurdConfig.RofiSelection && userCurdConfig.ImagePreview {
				anilistUserDataPreview, err := GetUserDataPreview(user.Token, user.Id)
				if err != nil {
					Log("Error refreshing anime list: " + err.Error())
					ExitCurd(err)
				}
				user.AnimeList = ParseAnimeList(anilistUserDataPreview)
			} else {
				anilistUserData, err := GetUserData(user.Token, user.Id)
				if err != nil {
					Log("Error refreshing anime list: " + err.Error())
					ExitCurd(err)
				}
				user.AnimeList = ParseAnimeList(anilistUserData)
			}
		} else if selectedOption.Key == "-1" {
			ExitCurd(nil)
		}
	}

	// If upstream is ahead, update the episode number
	if temp_anime, err := FindAnimeByAnilistID(user.AnimeList, strconv.Itoa(anime.AnilistId)); err == nil {
		if temp_anime.Progress > anime.Ep.Number {
			anime.Ep.Number = temp_anime.Progress
			anime.Ep.Player.PlaybackTime = 0
			anime.Ep.Resume = false
		}
	}

	if anime.TotalEpisodes == 0 {
		// Get updated anime data
		Log(selectedAllanimeAnime)
		updatedAnime, err := GetAnimeDataByID(anime.AnilistId, user.Token)
		Log(updatedAnime)
		if err != nil {
			Log(fmt.Sprintf("Error getting updated anime data: %v", err))
		} else {
			anime.TotalEpisodes = updatedAnime.TotalEpisodes
			Log(fmt.Sprintf("Updated total episodes: %d", anime.TotalEpisodes))
		}
	}

	if anime.TotalEpisodes == 0 { // If failed to get anime data
		CurdOut("Failed to get anime data. Attempting to retrieve from anime list.")
		animeList, err := SearchAnime(string(userQuery), userCurdConfig.SubOrDub)
		if err != nil {
			CurdOut(fmt.Sprintf("Failed to retrieve anime list: %v", err))
		} else {
			for _, option := range animeList {
				if option.Key == anime.AllanimeId {
					// Extract total episodes from the label
					if matches := regexp.MustCompile(`\((\d+) episodes\)`).FindStringSubmatch(option.Label); len(matches) > 1 {
						anime.TotalEpisodes, _ = strconv.Atoi(matches[1])
						CurdOut(fmt.Sprintf("Retrieved total episodes: %d", anime.TotalEpisodes))
						break
					}
				}
			}
		}

		if anime.TotalEpisodes == 0 {
			CurdOut("Still unable to determine total episodes.")
			CurdOut(fmt.Sprintf("Your AniList progress: %d", selectedAnilistAnime.Progress))
			var episodeNumber int
			if userCurdConfig.RofiSelection {
				userInput, err := GetUserInputFromRofi("Enter the episode you want to start from")
				if err != nil {
					Log("Error getting user input: " + err.Error())
					ExitCurd(fmt.Errorf("Error getting user input: " + err.Error()))
				}
				episodeNumber, err = strconv.Atoi(userInput)
			} else {
				fmt.Print("Enter the episode you want to start from: ")
				fmt.Scanln(&episodeNumber)
			}
			anime.Ep.Number = episodeNumber
		} else {
			anime.Ep.Number = selectedAnilistAnime.Progress + 1
		}
	} else if anime.TotalEpisodes < anime.Ep.Number { // Handle weird cases
		Log(fmt.Sprintf("Weird case: anime.TotalEpisodes < anime.Ep.Number: %v < %v", anime.TotalEpisodes, anime.Ep.Number))
		var answer string
		if userCurdConfig.RofiSelection {
			userInput, err := GetUserInputFromRofi("Would like to start the anime from beginning? (y/n)")
			if err != nil {
				Log("Error getting user input: " + err.Error())
				ExitCurd(fmt.Errorf("Error getting user input: " + err.Error()))
			}
			answer = userInput
		} else {
			fmt.Printf("Would like to start the anime from beginning? (y/n)\n")
			fmt.Scanln(&answer)
		}
		if answer == "y" {
			anime.Ep.Number = 1
		} else {
			anime.Ep.Number = anime.TotalEpisodes
		}
	}

	}() // Close the anonymous function with defer/recover for back navigation
}

// CreateOrWriteTokenFile creates the token file if it doesn't exist and writes the token to it
func WriteTokenToFile(token string, filePath string) error {
	// Extract the directory path
	dir := filepath.Dir(filePath)

	// Create all necessary parent directories
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directories: %v", err)
	}

	// Write the token to the file, creating it if it doesn't exist
	err := os.WriteFile(filePath, []byte(token), 0644)
	if err != nil {
		return fmt.Errorf("failed to write token to file: %v", err)
	}

	return nil
}

func StartCurd(userCurdConfig *CurdConfig, anime *Anime) string {

	// Validate inputs
	if anime.AllanimeId == "" {
		CurdOut("Error: No anime ID found")
		os.Exit(1)
	}
	if anime.Ep.Number <= 0 {
		CurdOut("Error: Invalid episode number")
		os.Exit(1)
	}

	if (anime.Ep.NextEpisode.Number == anime.Ep.Number) && (len(anime.Ep.NextEpisode.Links) > 0) {
		anime.Ep.Links = anime.Ep.NextEpisode.Links
	} else {
		// Get episode link
		link, err := GetEpisodeURL(*userCurdConfig, anime.AllanimeId, anime.Ep.Number)
		if len(link) > 0 {
			Log(fmt.Sprintf("Links details: %+v", link))
		}
		if err != nil {
			// If unable to get episode link automatically get manually
			episodeList, err := EpisodesList(anime.AllanimeId, userCurdConfig.SubOrDub)
			if err != nil {
				CurdOut("No episode list found")
				RestoreScreen()
				os.Exit(1)
			}
			if userCurdConfig.RofiSelection {
				userInput, err := GetUserInputFromRofi(fmt.Sprintf("Enter the episode (%v episodes)", episodeList[len(episodeList)-1]))
				if err != nil {
					Log("Error getting user input: " + err.Error())
					ExitCurd(fmt.Errorf("Error getting user input: " + err.Error()))
				}
				anime.Ep.Number, err = strconv.Atoi(userInput)
			} else {
				CurdOut(fmt.Sprintf("Enter the episode (%v episodes)", episodeList[len(episodeList)-1]))
				fmt.Scanln(&anime.Ep.Number)
			}
			link, err = GetEpisodeURL(*userCurdConfig, anime.AllanimeId, anime.Ep.Number)
			if err != nil {
				CurdOut("Failed to get episode link")
				os.Exit(1)
			}
		} else {
			Log(fmt.Sprintf("Successfully retrieved episode link on first try. Links count: %d", len(link)))
		}
		anime.Ep.Links = link
	}

	if len(anime.Ep.Links) == 0 {
		CurdOut("No episode links found")
		os.Exit(1)
	} else {
		Log(fmt.Sprintf("Episode links validation passed. Found %d links", len(anime.Ep.Links)))
	}

	// Modify the goroutine in main.go where next episode links are fetched
	// Get next episode link in parallel
	go func() {
		nextEpNum := anime.Ep.Number + 1
		if nextEpNum <= anime.TotalEpisodes {
			// Get next canon episode number if filler skip is enabled
			if userCurdConfig.SkipFiller && IsEpisodeFiller(anime.FillerEpisodes, anime.Ep.Number) {
				nextEpNum = GetNextCanonEpisode(anime.FillerEpisodes, nextEpNum)
			}
			nextLinks, err := GetEpisodeURL(*userCurdConfig, anime.AllanimeId, nextEpNum)
			if err != nil {
				Log(fmt.Sprintf("Error getting next episode link for ep %d: %v", nextEpNum, err))
			} else {
				anime.Ep.NextEpisode = NextEpisode{
					Number: nextEpNum,
					Links:  nextLinks,
				}
			}
		} else {
			Log(fmt.Sprintf("Next episode %d exceeds total episodes %d, skipping prefetch", nextEpNum, anime.TotalEpisodes))
		}
	}()

	// Write anime.AnilistId to curd_id in the storage path
	idFilePath := filepath.Join(os.ExpandEnv(userCurdConfig.StoragePath), "curd_id")
	Log(fmt.Sprintf("idFilePath: %v", idFilePath))
	if err := os.MkdirAll(filepath.Dir(idFilePath), 0755); err != nil {
		Log(fmt.Sprintf("Failed to create directory for curd_id: %v", err))
	} else {
		if err := os.WriteFile(idFilePath, []byte(fmt.Sprintf("%d", anime.AnilistId)), 0644); err != nil {
			Log(fmt.Sprintf("Failed to write AnilistId to file: %v", err))
		}
	}

	// Display starting message with cover image and episode info
	if anime.CoverImage != "" && userCurdConfig.ImagePreview && userCurdConfig.RofiSelection {
		// Get the cached image path
		cacheDir := os.ExpandEnv("${HOME}/.cache/curd/images")
		filename := fmt.Sprintf("%x.jpg", md5.Sum([]byte(anime.CoverImage)))
		cachePath := filepath.Join(cacheDir, filename)

		// Display the image if it exists in cache
		_, err := os.Stat(cachePath)
		if err == nil {
			// File exists
			Log(fmt.Sprintf("Image found at %s", cachePath))
			CurdOut(fmt.Sprintf("-i %s \"%s - Episode %d\"", cachePath, GetAnimeName(*anime), anime.Ep.Number))
		} else {
			// File does not exist
			Log(fmt.Sprintf("Image does not exist at %s", cachePath))
			CurdOut(fmt.Sprintf("%s - Episode %d",
				GetAnimeName(*anime),
				anime.Ep.Number))

		}
	} else {
		CurdOut(fmt.Sprintf("%s - Episode %d", GetAnimeName(*anime), anime.Ep.Number))
	}
	mpvSocketPath, err := StartVideo(PrioritizeLink(anime.Ep.Links), []string{}, fmt.Sprintf("%s - Episode %d", GetAnimeName(*anime), anime.Ep.Number), anime)

	if err != nil {
		Log("Failed to start mpv")
		os.Exit(1)
	}

	return mpvSocketPath
}

func CheckAndDownloadFiles(storagePath string, filesToCheck []string) error {
	// Create storage directory if it doesn't exist
	storagePath = os.ExpandEnv(storagePath)
	if err := os.MkdirAll(storagePath, 0755); err != nil {
		return fmt.Errorf("failed to create storage directory: %v", err)
	}

	// Base URL for downloading config files
	baseURL := "https://raw.githubusercontent.com/Wraient/curd/refs/heads/main/rofi/"

	// Check each file
	for _, fileName := range filesToCheck {
		filePath := filepath.Join(os.ExpandEnv(storagePath), fileName)

		// Skip if file already exists
		if _, err := os.Stat(filePath); err == nil {
			continue
		}

		// Download file if it doesn't exist
		resp, err := http.Get(baseURL + fileName)
		if err != nil {
			return fmt.Errorf("failed to download %s: %v", fileName, err)
		}
		defer resp.Body.Close()

		// Create the file
		out, err := os.Create(filePath)
		if err != nil {
			return fmt.Errorf("failed to create file %s: %v", fileName, err)
		}
		defer out.Close()

		// Write the content
		if _, err := io.Copy(out, resp.Body); err != nil {
			return fmt.Errorf("failed to write file %s: %v", fileName, err)
		}
	}

	return nil
}

func getEntriesByCategory(list AnimeList, category string) []Entry {
	switch category {
	case "ALL":
		// Combine all categories into one slice
		allEntries := make([]Entry, 0)
		allEntries = append(allEntries, list.Watching...)
		allEntries = append(allEntries, list.Completed...)
		allEntries = append(allEntries, list.Paused...)
		allEntries = append(allEntries, list.Dropped...)
		allEntries = append(allEntries, list.Planning...)
		allEntries = append(allEntries, list.Rewatching...)
		return allEntries
	case "CURRENT":
		return list.Watching
	case "COMPLETED":
		return list.Completed
	case "PAUSED":
		return list.Paused
	case "DROPPED":
		return list.Dropped
	case "PLANNING":
		return list.Planning
	case "REWATCHING": // Added for completeness, though "ALL" covers it.
		return list.Rewatching
	default:
		return []Entry{}
	}
}

func NextEpisodePromptCLI(userCurdConfig *CurdConfig) bool {
	anime := GetGlobalAnime()

	// Show the next episode number that will be started
	nextEpisodeNum := anime.Ep.Number + 1
	CurdOut(fmt.Sprintf("Start next episode (%d)?", nextEpisodeNum))

	// Create options for the selection - no "quit" option since it's built into selection menu
	options := []SelectionOption{
		{Key: "yes", Label: fmt.Sprintf("Yes, continue to episode %d", nextEpisodeNum)},
		{Key: "-2", Label: "← Back"},
	}

	// Use DynamicSelect for CLI mode
	selectedOption, err := DynamicSelect(options)
	if err != nil {
		Log(fmt.Sprintf("Error in CLI next episode prompt selection: %v", err))
		return false
	}

	Log(fmt.Sprintf("CLI User Selected Key: '%s', Label: '%s'", selectedOption.Key, selectedOption.Label))

	if selectedOption.Key == "-1" {
		// User selected to quit via the built-in quit option
		CurdOut("Exiting")
		return false
	}

	// Handle back button
	if selectedOption.Key == "-2" {
		CurdOut("Going back...")
		return false
	}

	return selectedOption.Key == "yes"
}

// NextEpisodePromptContinuous provides a continuous next episode prompt for CLI mode
// This runs throughout the episode duration and handles completion logic
func NextEpisodePromptContinuous(userCurdConfig *CurdConfig, databaseFile string, userToken string) {
	anime := GetGlobalAnime()
	
	for {
		// Check if episode has started
		if !anime.Ep.Started {
			time.Sleep(1 * time.Second)
			continue
		}

		// Show the next episode number that will be started
		nextEpisodeNum := anime.Ep.Number + 1
		CurdOut(fmt.Sprintf("Continue to next episode (%d) or quit?", nextEpisodeNum))

		// Create options for the selection - no "quit" option since it's built into selection menu
		options := []SelectionOption{
			{Key: "yes", Label: "Yes, start next episode now"},
			{Key: "-2", Label: "← Back"},
		}

		// Use DynamicSelect for CLI mode
		selectedOption, err := DynamicSelect(options)
		if err != nil {
			Log(fmt.Sprintf("Error in CLI continuous next episode prompt: %v", err))
			break
		}

		Log(fmt.Sprintf("CLI Continuous User Selected Key: '%s', Label: '%s'", selectedOption.Key, selectedOption.Label))

		if selectedOption.Key == "-1" {
			// User selected to quit via the built-in quit option

			// Check completion percentage
			percentageWatched := PercentageWatched(anime.Ep.Player.PlaybackTime, anime.Ep.Duration)

			if int(percentageWatched) >= userCurdConfig.PercentageToMarkComplete {
				// Episode is considered completed, mark it and update progress
				anime.Ep.IsCompleted = true

				// Update local database
				err = LocalUpdateAnime(databaseFile, anime.AnilistId, anime.AllanimeId, anime.Ep.Number, anime.Ep.Player.PlaybackTime, ConvertSecondsToMinutes(anime.Ep.Duration), GetAnimeName(*anime))
				if err != nil {
					Log("Error updating local database on quit: " + err.Error())
				}

				// Update Anilist progress if not rewatching
				if !anime.Rewatching {
					go func() {
						err = UpdateAnimeProgress(userToken, anime.AnilistId, anime.Ep.Number)
						if err != nil {
							Log("Error updating Anilist progress on quit: " + err.Error())
						} else {
							CurdOut(fmt.Sprintf("Episode marked as completed! Progress updated: %d", anime.Ep.Number))
						}
					}()
				}

				CurdOut(fmt.Sprintf("Episode completed (%.1f%% watched). Exiting.", percentageWatched))
			} else {
				CurdOut(fmt.Sprintf("Episode not completed (%.1f%% watched). Exiting.", percentageWatched))
			}

			ExitMPV(anime.Ep.Player.SocketPath)
			ExitCurd(nil)
			return
		}

		if selectedOption.Key == "-2" {
			// User selected to go back to main menu

			// Check completion percentage
			percentageWatched := PercentageWatched(anime.Ep.Player.PlaybackTime, anime.Ep.Duration)

			if int(percentageWatched) >= userCurdConfig.PercentageToMarkComplete {
				// Episode is considered completed, mark it and update progress
				anime.Ep.IsCompleted = true

				// Update local database
				err = LocalUpdateAnime(databaseFile, anime.AnilistId, anime.AllanimeId, anime.Ep.Number, anime.Ep.Player.PlaybackTime, ConvertSecondsToMinutes(anime.Ep.Duration), GetAnimeName(*anime))
				if err != nil {
					Log("Error updating local database on back: " + err.Error())
				}

				// Update progress if not rewatching
				if !anime.Rewatching {
					err = UpdateAnimeProgress(userToken, anime.AnilistId, anime.Ep.Number)
					if err != nil {
						Log("Error updating progress on back: " + err.Error())
					} else {
						CurdOut(fmt.Sprintf("Episode marked as completed! Progress updated: %d", anime.Ep.Number))
					}
					// Give it a moment to update
					time.Sleep(500 * time.Millisecond)
				}

				CurdOut(fmt.Sprintf("Episode completed (%.1f%% watched).", percentageWatched))
			} else {
				CurdOut(fmt.Sprintf("Episode not completed (%.1f%% watched).", percentageWatched))
			}

			ExitMPV(anime.Ep.Player.SocketPath)

			// Signal that we want to go back to anime selection
			// We'll use panic with a special recovery mechanism
			panic("BACK_TO_ANIME_SELECTION")
		}

		if selectedOption.Key == "yes" {
			// User wants to start next episode immediately
			anime.Ep.IsCompleted = true
			
			// Update database with completed episode first
			err = LocalUpdateAnime(databaseFile, anime.AnilistId, anime.AllanimeId, anime.Ep.Number, anime.Ep.Player.PlaybackTime, ConvertSecondsToMinutes(anime.Ep.Duration), GetAnimeName(*anime))
			if err != nil {
				Log("Error updating local database with completed episode: " + err.Error())
			}
			
			// Update Anilist progress for the completed episode if not rewatching
			if !anime.Rewatching {
				go func() {
					err = UpdateAnimeProgress(userToken, anime.AnilistId, anime.Ep.Number)
					if err != nil {
						Log("Error updating Anilist progress: " + err.Error())
					} else {
						CurdOut(fmt.Sprintf("Episode completed! Progress updated: %d", anime.Ep.Number))
					}
				}()
			}
			
			// Increment to next episode and update database with next episode number and 0 playback time
			anime.Ep.Number++
			
			// Use prefetched links if available for the next episode
			if (anime.Ep.NextEpisode.Number == anime.Ep.Number) && (len(anime.Ep.NextEpisode.Links) > 0) {
				anime.Ep.Links = anime.Ep.NextEpisode.Links
				Log(fmt.Sprintf("Using prefetched links for episode %d", anime.Ep.Number))
			} else {
				// Clear links to force fetching new ones
				anime.Ep.Links = []string{}
				Log(fmt.Sprintf("No prefetched links available for episode %d, will fetch new ones", anime.Ep.Number))
			}
			
			err = LocalUpdateAnime(databaseFile, anime.AnilistId, anime.AllanimeId, anime.Ep.Number, 0, 0, GetAnimeName(*anime))
			if err != nil {
				Log("Error updating local database with next episode: " + err.Error())
			}
			
			CurdOut("Starting next episode now...")
			ExitMPV(anime.Ep.Player.SocketPath)
			return // Exit this function, let the main loop handle next episode
		}
	}
}

// Simple next episode prompt for Rofi mode - just asks if user wants to continue
func NextEpisodePromptRofi(userCurdConfig *CurdConfig) bool {
	anime := GetGlobalAnime()

	// Show the next episode number that will be started
	nextEpisodeNum := anime.Ep.Number + 1

	// Create options for the selection
	options := []SelectionOption{
		{Key: "yes", Label: fmt.Sprintf("Yes, start episode %d", nextEpisodeNum)},
		{Key: "-2", Label: "← Back"},
	}

	// Use DynamicSelect for Rofi mode
	selectedOption, err := DynamicSelect(options)
	if err != nil {
		Log(fmt.Sprintf("Error in next episode prompt selection: %v", err))
		return false
	}

	Log(fmt.Sprintf("Rofi User Selected Key: '%s', Label: '%s'", selectedOption.Key, selectedOption.Label))

	// Handle back button - return false to stop playback
	if selectedOption.Key == "-2" {
		return false
	}

	return selectedOption.Key == "yes"
}

// StartNextEpisode handles the logic for starting the next episode
// It updates the episode number, resets necessary flags, and handles database updates
func StartNextEpisode(anime *Anime, userCurdConfig *CurdConfig, databaseFile string, userToken string) {
	// Save previous episode number for progress update
	prevEpisode := anime.Ep.Number

	// Check if we just completed the last episode
	if anime.TotalEpisodes > 0 && anime.Ep.Number == anime.TotalEpisodes {
		// Handle scoring and completion for the last episode
		HandleLastEpisodeCompletion(userCurdConfig, anime, userToken)

		// Update Anilist progress for the last episode if not rewatching
		if !anime.Rewatching {
			go func() {
				err := UpdateAnimeProgress(userToken, anime.AnilistId, prevEpisode)
				if err != nil {
					Log("Error updating Anilist progress: " + err.Error())
				} else {
					CurdOut(fmt.Sprintf("Anime progress updated! Latest watched episode: %d", prevEpisode))
				}
			}()
		}

		CurdOut("Series completed!")
		ExitCurd(nil)
		return
	}

	// Increment episode number
	anime.Ep.Number++

	// Check if we've reached the end of the series
	if anime.TotalEpisodes > 0 && anime.Ep.Number > anime.TotalEpisodes {
		CurdOut("Reached end of series")
		ExitCurd(nil)
		return
	}

	// Use prefetched links if available for the next episode
	if (anime.Ep.NextEpisode.Number == anime.Ep.Number) && (len(anime.Ep.NextEpisode.Links) > 0) {
		anime.Ep.Links = anime.Ep.NextEpisode.Links
		Log(fmt.Sprintf("Using prefetched links for episode %d", anime.Ep.Number))
	} else {
		// Clear links to force fetching new ones
		anime.Ep.Links = []string{}
		Log(fmt.Sprintf("No prefetched links available for episode %d, will fetch new ones", anime.Ep.Number))
	}

	// Reset episode flags
	anime.Ep.Started = false
	anime.Ep.IsCompleted = false

	// Log the transition
	Log("Completed episode, starting next.")

	// Update local database
	err := LocalUpdateAnime(databaseFile, anime.AnilistId, anime.AllanimeId, anime.Ep.Number, 0, 0, GetAnimeName(*anime))
	if err != nil {
		Log("Error updating local database: " + err.Error())
	}

	// Update Anilist progress for the previous episode if not rewatching
	if !anime.Rewatching {
		go func() {
			err = UpdateAnimeProgress(userToken, anime.AnilistId, prevEpisode)
			if err != nil {
				Log("Error updating Anilist progress: " + err.Error())
			} else {
				CurdOut(fmt.Sprintf("Anime progress updated! Latest watched episode: %d", prevEpisode))
			}
		}()
	}

	// Output message to user
	CurdOut(fmt.Sprint("Starting next episode: ", anime.Ep.Number))
}

// HandleLastEpisodeCompletion handles scoring and completion for the last episode
func HandleLastEpisodeCompletion(userCurdConfig *CurdConfig, anime *Anime, userToken string) {
	// Check if this is the last episode and scoring is enabled
	if userCurdConfig.ScoreOnCompletion && anime.TotalEpisodes > 0 && anime.Ep.Number == anime.TotalEpisodes {
		// Prompt user to score the anime
		CurdOut("You've completed this anime! Would you like to rate it?")

		scoreOptions := []SelectionOption{
			{Key: "yes", Label: "Yes, rate this anime"},
			{Key: "no", Label: "No, skip rating"},
		}

		selectedOption, err := DynamicSelect(scoreOptions)
		if err != nil {
			Log(fmt.Sprintf("Error in score prompt selection: %v", err))
			return
		}

		if selectedOption.Key == "yes" {
			err = RateAnime(userToken, anime.AnilistId)
			if err != nil {
				Log(fmt.Sprintf("Error rating anime: %v", err))
				CurdOut("Failed to rate anime")
			} else {
				CurdOut("Anime rated successfully!")
			}
		}

		// Update anime status to completed on AniList
		if !anime.Rewatching {
			go func() {
				err := UpdateAnimeStatus(userToken, anime.AnilistId, "COMPLETED")
				if err != nil {
					Log("Error updating anime status to completed: " + err.Error())
				} else {
					CurdOut("Anime status updated to completed!")
				}
			}()
		}
	}
}
