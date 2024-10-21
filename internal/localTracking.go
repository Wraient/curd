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

// Function to retrieve all anime entries
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
		anilistID, _ := strconv.Atoi(row[0])
		watchingEpisode, _ := strconv.Atoi(row[2])
		watchingTime, _ := strconv.Atoi(row[3])

		anime := Anime{
			AnilistId:  anilistID,
			AllanimeId: row[1],
			Ep: Episode{
				Number: watchingEpisode,
				Player: playingVideo{
					PlaybackTime: watchingTime,
				},
			},
			Title: AnimeTitle{
				Romaji: row[4],
			},
		}
		animeList = append(animeList, anime)
	}

	return animeList
}

// Function to update or add a new anime entry
func LocalUpdateAnime(databaseFile string, anilistID int, allanimeID string, watchingEpisode int, watchingTime int, animeName string) {
	animeList := LocalGetAllAnime(databaseFile)
	updated := false
	updatedAnimeList := [][]string{}

	// Check if the anime entry exists and update it
	for _, existingAnime := range animeList {
		if existingAnime.AnilistId == anilistID && existingAnime.AllanimeId == allanimeID {
			updatedAnimeList = append(updatedAnimeList, []string{
				strconv.Itoa(anilistID),
				allanimeID,
				strconv.Itoa(watchingEpisode),
				strconv.Itoa(watchingTime),
				animeName,
			})
			updated = true
		} else {
			updatedAnimeList = append(updatedAnimeList, []string{
				strconv.Itoa(existingAnime.AnilistId),
				existingAnime.AllanimeId,
				strconv.Itoa(existingAnime.Ep.Number),
				strconv.Itoa(existingAnime.Ep.Player.PlaybackTime),
				func() string {
					if existingAnime.Title.English != "" {
						return existingAnime.Title.English
					}
					return existingAnime.Title.Romaji
				}(), // Keep the original anime name (Romaji or English) for other entries
			})
		}
	}

	// If no entry was updated, add a new entry
	if !updated {
		updatedAnimeList = append(updatedAnimeList, []string{
			strconv.Itoa(anilistID),
			allanimeID,
			strconv.Itoa(watchingEpisode),
			strconv.Itoa(watchingTime),
			animeName,
		})
	}

	// Write all entries back to the file
	file, err := os.OpenFile(databaseFile, os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		fmt.Println("Error opening file for writing:", err)
		return
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	for _, updatedAnime := range updatedAnimeList {
		err = writer.Write(updatedAnime)
		if err != nil {
			fmt.Println("Error writing to file:", err)
			return
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
