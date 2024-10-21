package internal

import (
	"encoding/json"
	"fmt"
	"os"
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

func SetupCurd(userCurdConfig *CurdConfig, anime *Anime, user *User, databaseAnimes *[]Anime, logFile string) {
	var err error

	// Get user id, username and Anime list
	user.Id, user.Username, err = GetAnilistUserID(user.Token)
	anilistUserData, err := GetUserData(user.Token, user.Id)
	user.AnimeList = ParseAnimeList(anilistUserData)
	animeListMap := GetAnimeMap(user.AnimeList)

	// Select anime to watch (Anilist)
	anilistSelectedOption, err := DynamicSelect(animeListMap)
	userQuery := anilistSelectedOption.Label
	anime.AnilistId, err = strconv.Atoi(anilistSelectedOption.Key)
	if err != nil {
		fmt.Println("Error converting Anilist ID:", err)
	}

	// Find anime in Local history
	animePointer := LocalFindAnime(*databaseAnimes, anime.AnilistId, "")	
	
	// Get anime entry
	selectedAnilistAnime, err := FindAnimeByAnilistID(user.AnimeList, anilistSelectedOption.Key)
	if err != nil {
		fmt.Println("Can not find the anime in anilist animelist")
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
			fmt.Println("Failed to select anime", logFile)
			os.Exit(1)
		}
		if len(animeList) == 0 {
			fmt.Println("No results found.")
			os.Exit(0)
		}

		// find anime in animeList
		anime.AllanimeId, err = FindKeyByValue(animeList, fmt.Sprintf("%v (%d episodes)", userQuery, selectedAnilistAnime.Media.Episodes))
			
		// If unable to get Allanime id automatically get manually
		if anime.AllanimeId == "" {
			fmt.Println("Failed to automatically select anime")
			selectedAllanimeAnime, err := DynamicSelect(animeList)
			if err != nil {
				fmt.Println("No anime available")
				os.Exit(0)
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

	// Handel weird cases
	if anime.TotalEpisodes < anime.Ep.Number {
		fmt.Printf("Would like to start the anime from beginning? (y/n)\n")
		anime.Rewatching = true
		var answer string
		fmt.Scanln(&answer)
		if answer == "y" {
			anime.Ep.Number = 1
		} else {
			anime.Ep.Number = anime.TotalEpisodes
		}
	}
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
	mpvSocketPath, err := StartVideo(anime.Ep.Links[len(anime.Ep.Links)-1], []string{})

	if err != nil {
		Log("Failed to start mpv", logFile)
		os.Exit(1)
	}

	return mpvSocketPath
}

