package internal

import (
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
)

func GetTokenFromFile(filePath string) (string, error) {
	// Read the token from the file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read token from file: %w", err)
	}

	// Convert the byte slice to a string and remove any surrounding whitespace or newlines
	token := strings.TrimSpace(string(data))

	return token, nil
}

func EditConfig(configFilePath string) {
	// Get the user's preferred editor from the EDITOR environment variable
	editor := os.Getenv("EDITOR")
	if editor == "" {
		// If EDITOR is not set, default to "vim"
		editor = "vim"
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
		fmt.Printf("Error opening config file: %v\n", err)
		return
	}

	fmt.Println("Config file edited successfully.")
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
func Log(data interface{}, logFile string) error {
	// Open or create the log file
	file, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		return err
	}
	defer file.Close() // Ensure the file is closed when done

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
	logMessage := fmt.Sprintf("[LOG] %s %s %d: %s\n", currentTime, filename, lineNumber, jsonData)
	_, err = fmt.Fprint(file, logMessage) // Write to the file
	if err != nil {
		return err
	}

	return nil
}

// ClearScreen clears the terminal screen and saves the state
func ClearScreen() {
    fmt.Print("\033[?1049h") // Switch to alternate screen buffer
    fmt.Print("\033[2J")     // Clear the entire screen
    fmt.Print("\033[H")      // Move cursor to the top left
}

// RestoreScreen restores the original terminal state
func RestoreScreen() {
    fmt.Print("\033[?1049l") // Switch back to the main screen buffer
}

func ExitCurd(err error) {
	RestoreScreen()
	if err != nil {
		fmt.Println(err)
		if runtime.GOOS == "windows" {
			fmt.Println("Press Enter to exit")
			var wait string
			fmt.Scanln(&wait)
			os.Exit(1)
		} else {
			fmt.Println("Have a great day!")
			os.Exit(1)
		}
	}
	os.Exit(0)
}

func UpdateCurd(repo, fileName string) error {
    // Get the path of the currently running executable
    executablePath, err := os.Executable()
    if err != nil {
        return fmt.Errorf("unable to find current executable: %v", err)
    }

    // Adjust file name for Windows
    if runtime.GOOS == "windows" {
        fileName += ".exe"
    }

    // GitHub release URL for curd
    url := fmt.Sprintf("https://github.com/%s/releases/latest/download/%s", repo, fileName)

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

    // Write the downloaded content to the temporary file
    _, err = io.Copy(out, resp.Body)
    if err != nil {
        return fmt.Errorf("failed to save downloaded file: %v", err)
    }

    // Close and rename the temporary file to replace the current executable
    out.Close()

    // Replace the current executable with the downloaded curd
    if err := os.Rename(tmpPath, executablePath); err != nil {
        return fmt.Errorf("failed to replace the current executable: %v", err)
    }
    fmt.Println("Downloaded curd executable to", executablePath)

	if runtime.GOOS != "windows" {
		// Ensure the new file has executable permissions
		if err := os.Chmod(executablePath, 0755); err != nil {
			return fmt.Errorf("failed to set permissions on the new file: %v", err)
		}
	}
	
    return nil
}

func AddNewAnime(userCurdConfig *CurdConfig, anime *Anime, user *User, databaseAnimes *[]Anime, logFile string) {
	fmt.Println("Enter the anime name:")
	var query string
	fmt.Scanln(&query)
	animeMap, err := SearchAnimeAnilist(query, user.Token)
	if err != nil {
		Log(fmt.Sprintf("Failed to search anime: %v", err), logFile)
		ExitCurd(fmt.Errorf("Failed to search anime"))
	}
	anilistSelectedOption, err := DynamicSelect(animeMap)
	if err != nil {
		Log(fmt.Sprintf("No anime available: %v", err), logFile)
		ExitCurd(fmt.Errorf("No anime available"))
	}
	animeID, err := strconv.Atoi(anilistSelectedOption.Key)
	if err != nil {
		Log(fmt.Sprintf("Failed to convert anime ID to integer: %v", err), logFile)
		ExitCurd(fmt.Errorf("Failed to convert anime ID to integer"))
	}
	err = AddAnimeToWatchingList(animeID, user.Token)
	if err != nil {
		Log(fmt.Sprintf("Failed to add anime to watching list: %v", err), logFile)
		ExitCurd(fmt.Errorf("Failed to add anime to watching list"))
	}
	anilistUserData, err := GetUserData(user.Token, user.Id)
	if err != nil {
		Log(fmt.Sprintf("Failed to get user data: %v", err), logFile)
		ExitCurd(fmt.Errorf("Failed to get user data"))
	}
	user.AnimeList = ParseAnimeList(anilistUserData)
}

func SetupCurd(userCurdConfig *CurdConfig, anime *Anime, user *User, databaseAnimes *[]Anime, logFile string) {
	var err error

	// Get user id, username and Anime list
	user.Id, user.Username, err = GetAnilistUserID(user.Token)
	if err != nil {
		Log(fmt.Sprintf("Failed to get user ID: %v", err), logFile)
		ExitCurd(fmt.Errorf("Failed to get user ID"))
	}
	anilistUserData, err := GetUserData(user.Token, user.Id)
	if err != nil {
		Log(fmt.Sprintf("Failed to get user data: %v", err), logFile)
		ExitCurd(fmt.Errorf("Failed to get user data"))
	}
	user.AnimeList = ParseAnimeList(anilistUserData)
	animeListMap := GetAnimeMap(user.AnimeList)
	var anilistSelectedOption SelectionOption
	var selectedAllanimeAnime SelectionOption
	var userQuery string

	if anime.Ep.ContinueLast {
		// Get the last watched anime ID from the curd_id file
		curdIDPath := filepath.Join(os.ExpandEnv(userCurdConfig.StoragePath), "curd_id")
		curdIDBytes, err := os.ReadFile(curdIDPath)
		if err != nil {
			Log(fmt.Sprintf("Error reading curd_id file: %v", err), logFile)
			ExitCurd(fmt.Errorf("Error reading curd_id file"))
		}

		lastWatchedID, err := strconv.Atoi(strings.TrimSpace(string(curdIDBytes)))
		if err != nil {
			Log(fmt.Sprintf("Error converting curd_id to integer: %v", err), logFile)
			ExitCurd(fmt.Errorf("Error converting curd_id to integer"))
		}

		anime.AnilistId = lastWatchedID
		anilistSelectedOption.Key = strconv.Itoa(lastWatchedID)
	} else {
		// Select anime to watch (Anilist)
		var err error
		anilistSelectedOption, err = DynamicSelect(animeListMap)
		if err != nil {
			Log(fmt.Sprintf("Error selecting anime: %v", err), logFile)
			ExitCurd(fmt.Errorf("Error selecting anime"))
		}

		if anilistSelectedOption.Key == "-1" {
			ExitCurd(nil)
		}

		if anilistSelectedOption.Label == "add_new" {
			AddNewAnime(userCurdConfig, anime, user, databaseAnimes, logFile)
		}
		
		userQuery = anilistSelectedOption.Label
		anime.AnilistId, err = strconv.Atoi(anilistSelectedOption.Key)
		if err != nil {
			Log(fmt.Sprintf("Error converting Anilist ID: %v", err), logFile)
			ExitCurd(fmt.Errorf("Error converting Anilist ID"))
		}
	}
	// Find anime in Local history
	animePointer := LocalFindAnime(*databaseAnimes, anime.AnilistId, "")	
	
	// Get anime entry
	selectedAnilistAnime, err := FindAnimeByAnilistID(user.AnimeList, anilistSelectedOption.Key)
	if err != nil {
		Log(fmt.Sprintf("Can not find the anime in anilist animelist: %v", err), logFile)
		ExitCurd(fmt.Errorf("Can not find the anime in anilist animelist"))
	}

	// Set anime entry
	anime.Title = selectedAnilistAnime.Media.Title
	anime.TotalEpisodes = selectedAnilistAnime.Media.Episodes
	anime.Ep.Number = selectedAnilistAnime.Progress+1

	// if anime not found in database, find it in animeList
	if animePointer == nil {
		// Get Anime list (All anime)
		animeList, err := SearchAnime(string(userQuery), userCurdConfig.SubOrDub)
		if err != nil {
			Log(fmt.Sprintf("Failed to select anime: %v", err), logFile)
			ExitCurd(fmt.Errorf("Failed to select anime"))
		}
		if len(animeList) == 0 {
			// fmt.Println("No results found.")
			ExitCurd(fmt.Errorf("No results found."))
		}

		// find anime in animeList
		anime.AllanimeId, err = FindKeyByValue(animeList, fmt.Sprintf("%v (%d episodes)", userQuery, selectedAnilistAnime.Media.Episodes))
		if err != nil {
			Log(fmt.Sprintf("Failed to find anime in animeList: %v", err), logFile)
		}
			
		// If unable to get Allanime id automatically get manually
		if anime.AllanimeId == "" {
			fmt.Println("Failed to automatically select anime")
			selectedAllanimeAnime, err := DynamicSelect(animeList)
			if err != nil {
				// fmt.Println("No anime available")
				ExitCurd(fmt.Errorf("No anime available"))
			}
			anime.AllanimeId = selectedAllanimeAnime.Key
		}
	} else {
		// if anime found in database, use it
		anime.AllanimeId = animePointer.AllanimeId
		anime.Ep.Player.PlaybackTime = animePointer.Ep.Player.PlaybackTime
		anime.Ep.Resume = true
		anime.Ep.Number = animePointer.Ep.Number
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
		Log(selectedAllanimeAnime, logFile)
		updatedAnime, err := GetAnimeDataByID(anime.AnilistId, user.Token)
		Log(updatedAnime, logFile)
		if err != nil {
			Log(fmt.Sprintf("Error getting updated anime data: %v", err), logFile)
		} else {
			anime.TotalEpisodes = updatedAnime.TotalEpisodes
			Log(fmt.Sprintf("Updated total episodes: %d", anime.TotalEpisodes), logFile)
		}
	}

	if anime.TotalEpisodes == 0 { // If failed to get anime data
		fmt.Println("Failed to get anime data. Attempting to retrieve from anime list.")
		animeList, err := SearchAnime(string(userQuery), userCurdConfig.SubOrDub)
		if err != nil {
			fmt.Println("Failed to retrieve anime list:", err)
		} else {
			for allanimeId, label := range animeList {
				if allanimeId == anime.AllanimeId {
					// Extract total episodes from the label
					if matches := regexp.MustCompile(`\((\d+) episodes\)`).FindStringSubmatch(label); len(matches) > 1 {
						anime.TotalEpisodes, _ = strconv.Atoi(matches[1])
						fmt.Printf("Retrieved total episodes: %d\n", anime.TotalEpisodes)
						break
					}
				}
			}
		}
		
		if anime.TotalEpisodes == 0 {
			fmt.Println("Still unable to determine total episodes.")
			fmt.Println("Your AniList progress:", selectedAnilistAnime.Progress)
			fmt.Print("Enter the episode you want to start from: ")
			var episodeNumber int
			fmt.Scanln(&episodeNumber)
			anime.Ep.Number = episodeNumber
		} else {
			anime.Ep.Number = selectedAnilistAnime.Progress + 1
		}
	} else if anime.TotalEpisodes < anime.Ep.Number { // Handle weird cases
		Log(fmt.Sprintf("Weird case: anime.TotalEpisodes < anime.Ep.Number: %v < %v", anime.TotalEpisodes, anime.Ep.Number), logFile)
		fmt.Printf("Would like to start the anime from beginning? (y/n)\n")
		var answer string
		fmt.Scanln(&answer)
		if answer == "y" {
			anime.Ep.Number = 1
		} else {
			anime.Ep.Number = anime.TotalEpisodes
		}
	}

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

func StartCurd(userCurdConfig *CurdConfig, anime *Anime, logFile string) string {

	// Get episode link
	link, err := GetEpisodeURL(*userCurdConfig, anime.AllanimeId, anime.Ep.Number)
	if err != nil {
		// If unable to get episode link automatically get manually
		episodeList, err := EpisodesList(anime.AllanimeId, userCurdConfig.SubOrDub)
		if err != nil {
			fmt.Println("No episode list found")
			RestoreScreen()
			os.Exit(1)
		}
		fmt.Printf("Enter the episode (%v episodes)\n", episodeList[len(episodeList)-1])
		fmt.Scanln(&anime.Ep.Number)
		link, err = GetEpisodeURL(*userCurdConfig, anime.AllanimeId, anime.Ep.Number)
		if err != nil {
			fmt.Println("Failed to get episode link")
			os.Exit(1)
		}
		// anime.Ep.Links = link
	}
	anime.Ep.Links = link

	if len(anime.Ep.Links) == 0 {
		fmt.Println("No episode links found")
		os.Exit(1)
	}

	Log(anime, logFile)

	// Write anime.AnilistId to curd_id in the storage path
	idFilePath := filepath.Join(os.ExpandEnv(userCurdConfig.StoragePath), "curd_id")
	Log(fmt.Sprintf("idFilePath: %v", idFilePath), logFile)
	if err := os.MkdirAll(filepath.Dir(idFilePath), 0755); err != nil {
		Log(fmt.Sprintf("Failed to create directory for curd_id: %v", err), logFile)
	} else {
		if err := os.WriteFile(idFilePath, []byte(fmt.Sprintf("%d", anime.AnilistId)), 0644); err != nil {
			Log(fmt.Sprintf("Failed to write AnilistId to file: %v", err), logFile)
		}
	}

	mpvSocketPath, err := StartVideo(PrioritizeLink(anime.Ep.Links), []string{})

	if err != nil {
		Log("Failed to start mpv", logFile)
		os.Exit(1)
	}

	return mpvSocketPath
}

