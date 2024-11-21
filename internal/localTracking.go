package internal

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// Function to add an anime entry
func LocalAddAnime(databaseFile string, anilistID int, allanimeID string, watchingEpisode int, watchingTime int, animeDuration int, animeName string) {
	file, err := os.OpenFile(databaseFile, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		CurdOut(fmt.Sprintf("Error opening file: %v", err))
		return
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	err = writer.Write([]string{
		strconv.Itoa(anilistID),
		allanimeID,
		strconv.Itoa(watchingEpisode),
		strconv.Itoa(watchingTime),
		strconv.Itoa(animeDuration),
		animeName,
	})
	if err != nil {
		CurdOut(fmt.Sprintf("Error writing to file: %v", err))
	} else {
		CurdOut("Written to file")
	}
}

// Function to delete an anime entry by Anilist ID and Allanime ID
func LocalDeleteAnime(databaseFile string, anilistID int, allanimeID string) {
	animeList := [][]string{}
	file, err := os.Open(databaseFile)
	if err != nil {
		CurdOut(fmt.Sprintf("Error opening file: %v", err))
		return
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		CurdOut(fmt.Sprintf("Error reading file: %v", err))
		return
	}

	// Filter out the anime entry
	for _, row := range records {
		aid, _ := strconv.Atoi(row[0]) // Anilist ID
		if aid != anilistID || row[1] != allanimeID {
			animeList = append(animeList, row)
		}
	}

	// Write the filtered list back to the file
	fileWrite, err := os.OpenFile(databaseFile, os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		CurdOut(fmt.Sprintf("Error opening file for writing: %v", err))
		return
	}
	defer fileWrite.Close()

	writer := csv.NewWriter(fileWrite)
	defer writer.Flush()

	err = writer.WriteAll(animeList)
	if err != nil {
		CurdOut(fmt.Sprintf("Error writing to file: %v", err))
	}
}

// Function to get all anime entries from the database
func LocalGetAllAnime(databaseFile string) []Anime {
	animeList := []Anime{}

	// Ensure the directory exists
	dir := filepath.Dir(databaseFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		CurdOut(fmt.Sprintf("Error creating directory: %v", err))
		return animeList
	}

	// Open the file, create if it doesn't exist
	file, err := os.OpenFile(databaseFile, os.O_RDONLY|os.O_CREATE, 0644)
	if err != nil {
		CurdOut(fmt.Sprintf("Error opening or creating file: %v", err))
		return animeList
	}
	defer file.Close()

	// If the file was just created, it will be empty, so return an empty list
	fileInfo, err := file.Stat()
	if err != nil {
		CurdOut(fmt.Sprintf("Error getting file info: %v", err))
		return animeList
	}
	if fileInfo.Size() == 0 {
		return animeList
	}

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		CurdOut(fmt.Sprintf("Error reading file: %v", err))
		return animeList
	}

	for _, row := range records {
		anime := parseAnimeRow(row)
		if anime != nil {
			animeList = append(animeList, *anime)
		}
	}

	return animeList
}

// Function to parse a single row of anime data
func parseAnimeRow(row []string) *Anime {
	if len(row) < 5 {
		CurdOut(fmt.Sprintf("Invalid row format: %v", row))
		return nil
	}

	anilistID, _ := strconv.Atoi(row[0])
	watchingEpisode, _ := strconv.Atoi(row[2])
	playbackTime, _ := strconv.Atoi(row[3])
	animeDuration, _ := strconv.Atoi(row[4])

	anime := &Anime{
		AnilistId:  anilistID,
		AllanimeId: row[1],
		Ep: Episode{
			Number: watchingEpisode,
			Player: playingVideo{
				PlaybackTime: playbackTime,
			},
			Duration: animeDuration,
		},
	}

	if len(row) == 6 {
		anime.Title = AnimeTitle{
			English: row[5],
			Romaji:  row[5],
		}
	} else if len(row) == 5 {
		anime.Title = AnimeTitle{
			English: row[4],
			Romaji:  row[4],
		}
	}

	return anime
}

// Function to get the anime name (English or Romaji) from an Anime struct
func GetAnimeName(anime Anime) string {
	if anime.Title.English != "" {
		return anime.Title.English
	}
	return anime.Title.Romaji
}

// Function to update or add a new anime entry
func LocalUpdateAnime(databaseFile string, anilistID int, allanimeID string, watchingEpisode int, playbackTime int, animeDuration int, animeName string) error {
	// Read existing entries
	animeList := LocalGetAllAnime(databaseFile)

	// Find and update existing entry or add new one
	updated := false
	for i, anime := range animeList {
		if anime.AnilistId == anilistID && anime.AllanimeId == allanimeID {
			animeList[i].Ep.Number = watchingEpisode
			animeList[i].Ep.Player.PlaybackTime = playbackTime
			animeList[i].Ep.Duration = animeDuration
			animeList[i].Title.English = animeName
			animeList[i].Title.Romaji = animeName
			updated = true
			break
		}
	}

	if !updated {
		newAnime := Anime{
			AnilistId:  anilistID,
			AllanimeId: allanimeID,
			Ep: Episode{
				Number: watchingEpisode,
				Player: playingVideo{
					PlaybackTime: playbackTime,
				},
				Duration: animeDuration,
			},
			Title: AnimeTitle{
				English: animeName,
				Romaji:  animeName,
			},
		}
		animeList = append(animeList, newAnime)
	}

	// Write updated list back to file
	file, err := os.Create(databaseFile)
	if err != nil {
		CurdOut(fmt.Sprintf("Error creating file: %v", err))
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	for _, anime := range animeList {
		record := []string{
			strconv.Itoa(anime.AnilistId),
			anime.AllanimeId,
			strconv.Itoa(anime.Ep.Number),
			strconv.Itoa(anime.Ep.Player.PlaybackTime),
			strconv.Itoa(anime.Ep.Duration),
			GetAnimeName(anime),
		}
		if err := writer.Write(record); err != nil {
			CurdOut(fmt.Sprintf("Error writing record: %v", err))
		}
	}
	
	return nil
}

// Function to find an anime by either Anilist ID or Allanime ID
func LocalFindAnime(animeList []Anime, anilistID int, allanimeID string) *Anime {
	for _, anime := range animeList {
		if anime.AnilistId == anilistID || anime.AllanimeId == allanimeID {
			return &anime
		}
	}
	return nil
}

func WatchUntracked(userCurdConfig *CurdConfig, logFile string) {
	var query string
	var animeList map[string]string
	var err error
	var anime Anime

	// Get anime name from user
	if userCurdConfig.RofiSelection {
		userInput, err := GetUserInputFromRofi("Enter the anime name")
		if err != nil {
			Log("Error getting user input: "+err.Error(), logFile)
			ExitCurd(fmt.Errorf("Error getting user input: "+err.Error()))
		}
		query = userInput
	} else {
		CurdOut("Enter the anime name:")
		fmt.Scanln(&query)
	}

	// Search for the anime
	animeList, err = SearchAnime(query, userCurdConfig.SubOrDub)
	if err != nil {
		Log(fmt.Sprintf("Failed to search anime: %v", err), logFile)
		ExitCurd(fmt.Errorf("Failed to search anime"))
	}

	if len(animeList) == 0 {
		ExitCurd(fmt.Errorf("No results found."))
	}

	// Select anime from search results
	selectedAnime, err := DynamicSelect(animeList, false)
	if err != nil {
		Log(fmt.Sprintf("Failed to select anime: %v", err), logFile)
		ExitCurd(fmt.Errorf("Failed to select anime"))
	}

	if selectedAnime.Key == "-1" {
		ExitCurd(nil)
	}

	anime.AllanimeId = selectedAnime.Key
	anime.Title.English = selectedAnime.Label

	// Get episode number
	var episodeNumber int
	if userCurdConfig.RofiSelection {
		userInput, err := GetUserInputFromRofi("Enter the episode number")
		if err != nil {
			Log("Error getting episode number: "+err.Error(), logFile)
			ExitCurd(fmt.Errorf("Error getting episode number: "+err.Error()))
		}
		episodeNumber, err = strconv.Atoi(userInput)
		if err != nil {
			Log(fmt.Sprintf("Invalid episode number: %v", err), logFile)
			ExitCurd(fmt.Errorf("Invalid episode number"))
		}
	} else {
		CurdOut("Enter the episode number:")
		fmt.Scanln(&episodeNumber)
	}

	anime.Ep.Number = episodeNumber
	
	for {
		// Get episode link
		link, err := GetEpisodeURL(*userCurdConfig, anime.AllanimeId, anime.Ep.Number)
		if err != nil {
			Log(fmt.Sprintf("Failed to get episode link: %v", err), logFile)
			ExitCurd(fmt.Errorf("Failed to get episode link"))
		}

		if len(link) == 0 {
			ExitCurd(fmt.Errorf("No episode links found"))
		}

		CurdOut(fmt.Sprintf("%s - Episode %d", GetAnimeName(anime), anime.Ep.Number))

		// Start video playback
		mpvSocketPath, err := StartVideo(PrioritizeLink(link), []string{})
		if err != nil {
			Log("Failed to start mpv", logFile)
			os.Exit(1)
		}

		anime.Ep.Player.SocketPath = mpvSocketPath
		anime.Ep.Started = false
		
		Log(fmt.Sprintf("Started mpvsocketpath ", anime.Ep.Player.SocketPath), logFile)

		// Get video duration
		go func() {
			for {
				if anime.Ep.Started {
					if anime.Ep.Duration == 0 {
						// Get video duration
						durationPos, err := MPVSendCommand(anime.Ep.Player.SocketPath, []interface{}{"get_property", "duration"})
						if err != nil {
							Log("Error getting video duration: "+err.Error(), logFile)
						} else if durationPos != nil {
							if duration, ok := durationPos.(float64); ok {
								anime.Ep.Duration = int(duration + 0.5) // Round to nearest integer
								Log(fmt.Sprintf("Video duration: %d seconds", anime.Ep.Duration), logFile)
							} else {
								Log("Error: duration is not a float64", logFile)
							}
						}
						break
					}
				}
				time.Sleep(1 * time.Second)
			}
		}()

		// Listen for video started
		for {
			timePos, err := MPVSendCommand(anime.Ep.Player.SocketPath, []interface{}{"get_property", "time-pos"})
			if err != nil {
				Log("Error getting playback time: "+err.Error(), logFile)

				// Check if the error is due to invalid JSON
				// User closed the video
				if anime.Ep.Started {
					percentageWatched := PercentageWatched(anime.Ep.Player.PlaybackTime, anime.Ep.Duration)
					// Episode is completed
					Log(fmt.Sprint(percentageWatched), logFile)
					Log(fmt.Sprint(anime.Ep.Player.PlaybackTime), logFile)
					Log(fmt.Sprint(anime.Ep.Duration), logFile)
					Log(fmt.Sprint(userCurdConfig.PercentageToMarkComplete), logFile)
					if int(percentageWatched) >= userCurdConfig.PercentageToMarkComplete {
						anime.Ep.Number++
						anime.Ep.Started = false
						Log("Completed episode, starting next.", logFile)
						anime.Ep.IsCompleted = true
						// Exit the skip loop
						break
					} else if fmt.Sprintf("%v", err) == "invalid character '{' after top-level value" { // Episode is not completed
						Log("Received invalid JSON response, continuing...", logFile)
					} else {
						Log("Episode is not completed, exiting", logFile)
						ExitCurd(nil)
					}
				}
			}

			// Convert timePos to integer
			if timePos != nil {
				if !anime.Ep.Started {
					anime.Ep.Started = true
				}

				animePosition, ok := timePos.(float64)
				if !ok {
					Log("Error: timePos is not a float64", logFile)
					continue
				}

				anime.Ep.Player.PlaybackTime = int(animePosition + 0.5) // Round to nearest integer
			}
			time.Sleep(1 * time.Second)

		}
	}

}
