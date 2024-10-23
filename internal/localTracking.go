package internal

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// Function to add an anime entry
func LocalAddAnime(databaseFile string, anilistID int, allanimeID string, watchingEpisode int, watchingTime int, animeName string) {
	file, err := os.OpenFile(databaseFile, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		fmt.Println("Error opening file:", err)
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
		animeName,
	})
	if err != nil {
		fmt.Println("Error writing to file:", err)
	} else {
		fmt.Println("Written to file")
	}
}

// Function to delete an anime entry by Anilist ID and Allanime ID
func LocalDeleteAnime(databaseFile string, anilistID int, allanimeID string) {
	animeList := [][]string{}
	file, err := os.Open(databaseFile)
	if err != nil {
		fmt.Println("Error opening file:", err)
		return
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		fmt.Println("Error reading file:", err)
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
		fmt.Println("Error opening file for writing:", err)
		return
	}
	defer fileWrite.Close()

	writer := csv.NewWriter(fileWrite)
	defer writer.Flush()

	err = writer.WriteAll(animeList)
	if err != nil {
		fmt.Println("Error writing to file:", err)
	}
}

// Function to get all anime entries from the database
func LocalGetAllAnime(databaseFile string) []Anime {
	animeList := []Anime{}

	// Ensure the directory exists
	dir := filepath.Dir(databaseFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Println("Error creating directory:", err)
		return animeList
	}

	// Open the file, create if it doesn't exist
	file, err := os.OpenFile(databaseFile, os.O_RDONLY|os.O_CREATE, 0644)
	if err != nil {
		fmt.Println("Error opening or creating file:", err)
		return animeList
	}
	defer file.Close()

	// If the file was just created, it will be empty, so return an empty list
	fileInfo, err := file.Stat()
	if err != nil {
		fmt.Println("Error getting file info:", err)
		return animeList
	}
	if fileInfo.Size() == 0 {
		return animeList
	}

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		fmt.Println("Error reading file:", err)
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
		fmt.Printf("Invalid row format: %v\n", row)
		return nil
	}

	anilistID, _ := strconv.Atoi(row[0])
	watchingEpisode, _ := strconv.Atoi(row[2])
	playbackTime, _ := strconv.Atoi(row[3])

	anime := &Anime{
		AnilistId:  anilistID,
		AllanimeId: row[1],
		Ep: Episode{
			Number: watchingEpisode,
			Player: playingVideo{
				PlaybackTime: playbackTime,
			},
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
func LocalUpdateAnime(databaseFile string, anilistID int, allanimeID string, watchingEpisode int, playbackTime int, animeName string) error {
	// Read existing entries
	animeList := LocalGetAllAnime(databaseFile)

	// Find and update existing entry or add new one
	updated := false
	for i, anime := range animeList {
		if anime.AnilistId == anilistID && anime.AllanimeId == allanimeID {
			animeList[i].Ep.Number = watchingEpisode
			animeList[i].Ep.Player.PlaybackTime = playbackTime
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
		fmt.Println("Error creating file:", err)
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
			GetAnimeName(anime),
		}
		if err := writer.Write(record); err != nil {
			fmt.Println("Error writing record:", err)
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
