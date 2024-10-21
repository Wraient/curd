package internal

import (
	"encoding/csv"
	"fmt"
	"os"
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
func LocalGetAllAnime(databaseFile string) []Anime {
	animeList := []Anime{}
	file, err := os.Open(databaseFile)
	if err != nil {
		fmt.Println("Error opening file:", err)
		return animeList
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		fmt.Println("Error reading file:", err)
		return animeList
	}

	for _, row := range records {
		if len(row) != 5 {
			fmt.Printf("Invalid row format: %v\n", row)
			continue
		}

		anilistID, _ := strconv.Atoi(row[0])
		watchingEpisode, _ := strconv.Atoi(row[2])
		playbackTime, _ := strconv.Atoi(row[3])

		anime := Anime{
			AnilistId:  anilistID,
			AllanimeId: row[1],
			Ep: Episode{
				Number: watchingEpisode,
				Player: playingVideo{
					PlaybackTime: playbackTime,
				},
			},
			Title: AnimeTitle{
				English: row[4],
				Romaji:  row[4],
			},
		}
		animeList = append(animeList, anime)
	}

	return animeList
}

// Function to get the anime name (English or Romaji) from an Anime struct
func getAnimeName(anime Anime) string {
	if anime.Title.English != "" {
		return anime.Title.English
	}
	return anime.Title.Romaji
}
// Function to update or add a new anime entry
func LocalUpdateAnime(databaseFile string, anilistID int, allanimeID string, watchingEpisode int, playbackTime int, animeName string) {
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
		return
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
			getAnimeName(anime),
		}
		if err := writer.Write(record); err != nil {
			fmt.Println("Error writing record:", err)
		}
	}
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
