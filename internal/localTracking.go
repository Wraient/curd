package internal

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/wraient/curd/internal/providers/animepahe"
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
		return
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		CurdOut(fmt.Sprintf("Error flushing file: %v", err))
		return
	}
	CurdOut("Written to file")
}

func normalizeLocalProviderID(providerName, providerID, animeName string) string {
	if _, rawProviderID, ok := ParseProviderQualifiedID(providerID); ok {
		providerID = rawProviderID
	}

	if providerName != "animepahe" {
		return providerID
	}

	if providerID == "" {
		return ""
	}

	stableID := animepahe.StableProviderID(providerID)
	if stableID != "" {
		return stableID
	}

	return providerID
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
		anime := parseAnimeRow(row)
		if anime == nil {
			CurdOut(fmt.Sprintf("Skipping invalid local history row: %v", row))
			continue
		}

		existingProviderID := normalizeLocalProviderID(anime.ProviderName, anime.ProviderId, GetAnimeName(*anime))
		targetProviderID := normalizeLocalProviderID(anime.ProviderName, allanimeID, GetAnimeName(*anime))
		if anime.AnilistId != anilistID || existingProviderID != targetProviderID {
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
	err = writer.WriteAll(animeList)
	if err != nil {
		CurdOut(fmt.Sprintf("Error writing to file: %v", err))
		return
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		CurdOut(fmt.Sprintf("Error flushing file: %v", err))
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

	anilistID, ok := parseRequiredLocalInt(row[0], "AniList ID", row)
	if !ok {
		return nil
	}
	watchingEpisode, ok := parseRequiredLocalInt(row[2], "episode", row)
	if !ok {
		return nil
	}
	playbackTime, ok := parseRequiredLocalInt(row[3], "playback time", row)
	if !ok {
		return nil
	}
	animeDuration := 0
	animeName := strconv.Itoa(anilistID)
	providerName := "allanime"

	if len(row) >= 7 {
		animeDuration = parseLocalInt(row[4])
		providerName = row[5]
		animeName = row[6]
	} else if len(row) >= 6 {
		animeDuration = parseLocalInt(row[4])
		animeName = row[5]
	} else if duration, err := strconv.Atoi(row[4]); err == nil {
		animeDuration = duration
	} else {
		animeName = row[4]
	}

	anime := &Anime{
		AnilistId:  anilistID,
		ProviderId: row[1],
		Ep: Episode{
			Number: watchingEpisode,
			Player: playingVideo{
				PlaybackTime: playbackTime,
			},
			Duration: animeDuration,
		},
		ProviderName: providerName,
		Title: AnimeTitle{
			English: animeName,
			Romaji:  animeName,
		},
	}

	return anime
}

func parseLocalInt(value string) int {
	parsed, _ := strconv.Atoi(strings.TrimSpace(value))
	return parsed
}

func parseRequiredLocalInt(value, field string, row []string) (int, bool) {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		CurdOut(fmt.Sprintf("Invalid %s in local history row %v: %v", field, row, err))
		return 0, false
	}
	return parsed, true
}

// Function to get the anime name (English or Romaji) from an Anime struct
func GetAnimeName(anime Anime) string {
	userCurdConfig := GetGlobalConfig()
	if anime.Title.English != "" && useEnglishAnimeNames(userCurdConfig) {
		return anime.Title.English
	}
	if anime.Title.Romaji != "" {
		return anime.Title.Romaji
	}
	if anime.Title.English != "" {
		return anime.Title.English
	}
	if anime.AnilistId != 0 {
		return strconv.Itoa(anime.AnilistId)
	}
	return ""
}

// Function to update or add a new anime entry
func LocalUpdateAnime(databaseFile string, anilistID int, allanimeID string, watchingEpisode int, playbackTime int, animeDuration int, animeName string, providerName string) error {
	allanimeID = normalizeLocalProviderID(providerName, allanimeID, animeName)
	// Read existing entries
	animeList := LocalGetAllAnime(databaseFile)

	// Find and update existing entry or add new one
	updated := false
	for i, anime := range animeList {
		existingProviderID := normalizeLocalProviderID(anime.ProviderName, anime.ProviderId, GetAnimeName(anime))
		if anime.AnilistId == anilistID && existingProviderID == allanimeID {
			animeList[i].Ep.Number = watchingEpisode
			animeList[i].Ep.Player.PlaybackTime = playbackTime
			animeList[i].Ep.Duration = animeDuration
			animeList[i].ProviderId = allanimeID
			animeList[i].Title.English = animeName
			animeList[i].Title.Romaji = animeName
			animeList[i].ProviderName = providerName
			updated = true
			break
		}
	}

	if !updated {
		newAnime := Anime{
			AnilistId:  anilistID,
			ProviderId: allanimeID,
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
			ProviderName: providerName,
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

	for _, anime := range animeList {
		providerID := normalizeLocalProviderID(anime.ProviderName, anime.ProviderId, GetAnimeName(anime))
		record := []string{
			strconv.Itoa(anime.AnilistId),
			providerID,
			strconv.Itoa(anime.Ep.Number),
			strconv.Itoa(anime.Ep.Player.PlaybackTime),
			strconv.Itoa(anime.Ep.Duration),
			anime.ProviderName,
			GetAnimeName(anime),
		}
		if err := writer.Write(record); err != nil {
			CurdOut(fmt.Sprintf("Error writing record: %v", err))
			return err
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		CurdOut(fmt.Sprintf("Error flushing records: %v", err))
		return err
	}

	return nil
}

// Function to find an anime by either Anilist ID or Allanime ID
func LocalFindAnime(animeList []Anime, anilistID int, allanimeID string) *Anime {
	var bestMatch *Anime
	for i := range animeList {
		anime := &animeList[i]
		if anime.AnilistId == anilistID || (allanimeID != "" && anime.ProviderId == allanimeID) {
			if bestMatch == nil ||
				anime.Ep.Number > bestMatch.Ep.Number ||
				(anime.Ep.Number == bestMatch.Ep.Number && anime.Ep.Player.PlaybackTime > bestMatch.Ep.Player.PlaybackTime) {
				bestMatch = anime
			}
		}
	}
	return bestMatch
}

func LocalRemapAnimeProvider(databaseFile string, anilistID int, providerName, providerID, animeName string, watchingEpisode, playbackTime, animeDuration int) error {
	providerID = normalizeLocalProviderID(providerName, providerID, animeName)
	animeList := LocalGetAllAnime(databaseFile)

	target := LocalFindAnime(animeList, anilistID, "")
	if target == nil {
		newAnime := Anime{
			AnilistId:    anilistID,
			ProviderId:   providerID,
			ProviderName: providerName,
			Ep: Episode{
				Number:   watchingEpisode,
				Player:   playingVideo{PlaybackTime: playbackTime},
				Duration: animeDuration,
			},
			Title: AnimeTitle{
				English: animeName,
				Romaji:  animeName,
			},
		}
		animeList = append(animeList, newAnime)
	} else {
		oldProviderID := normalizeLocalProviderID(target.ProviderName, target.ProviderId, GetAnimeName(*target))
		updated := false
		for i := range animeList {
			entry := &animeList[i]
			if entry.AnilistId != anilistID {
				continue
			}
			entryProviderID := normalizeLocalProviderID(entry.ProviderName, entry.ProviderId, GetAnimeName(*entry))
			if entryProviderID != oldProviderID {
				continue
			}
			entry.ProviderId = providerID
			entry.ProviderName = providerName
			entry.Ep.Number = watchingEpisode
			entry.Ep.Player.PlaybackTime = playbackTime
			entry.Ep.Duration = animeDuration
			if animeName != "" {
				entry.Title.English = animeName
				entry.Title.Romaji = animeName
			}
			updated = true
		}
		if !updated {
			return fmt.Errorf("failed to update local history for AniList ID %d", anilistID)
		}
	}

	file, err := os.Create(databaseFile)
	if err != nil {
		return fmt.Errorf("create local history file: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	for _, anime := range animeList {
		record := []string{
			strconv.Itoa(anime.AnilistId),
			normalizeLocalProviderID(anime.ProviderName, anime.ProviderId, GetAnimeName(anime)),
			strconv.Itoa(anime.Ep.Number),
			strconv.Itoa(anime.Ep.Player.PlaybackTime),
			strconv.Itoa(anime.Ep.Duration),
			anime.ProviderName,
			GetAnimeName(anime),
		}
		if err := writer.Write(record); err != nil {
			return fmt.Errorf("write local history: %w", err)
		}
	}
	writer.Flush()
	return writer.Error()
}

func WatchUntracked(userCurdConfig *CurdConfig) {
	var query string
	var err error
	var anime Anime

	// Anime search and selection loop
	for {
		// Get anime name from user
		query, err = promptText(userCurdConfig, "Enter the anime name", false)
		if err != nil {
			Log("Error getting user input: " + err.Error())
			ExitCurd(fmt.Errorf("Error getting user input: " + err.Error()))
		}

		providerID, providerName, back, searchErr := ResolveUntrackedProviderSearch(userCurdConfig, query)
		if searchErr != nil {
			Log(fmt.Sprintf("Failed to search anime: %v", searchErr))
			ExitCurd(fmt.Errorf("Failed to search anime"))
		}
		if back {
			return
		}
		if providerID == "" {
			ExitCurd(nil)
		}

		anime.ProviderId = providerID
		anime.ProviderName = providerName
		anime.Title.English = query
		anime.Title.Romaji = query
		break
	}

	// Get episode number
	episodeNumber, err := promptPositiveEpisodeNumber(userCurdConfig, "Enter the episode number")
	if err != nil {
		Log(fmt.Sprintf("Invalid episode number: %v", err))
		ExitCurd(fmt.Errorf("Invalid episode number"))
	}

	anime.Ep.Number = episodeNumber

	for {
		// Prefer prefetched next-episode links when available (same as tracked path).
		if anime.Ep.NextEpisode.Number == anime.Ep.Number && len(anime.Ep.NextEpisode.Links) > 0 {
			anime.Ep.Links = anime.Ep.NextEpisode.Links
			anime.Ep.StreamReferrer = ""
			anime.Ep.SubtitleURL = ""
			if anime.Ep.NextEpisode.ProviderName != "" {
				anime.ProviderName = anime.Ep.NextEpisode.ProviderName
				anime.ProviderId = anime.Ep.NextEpisode.ProviderId
			}
			anime.Ep.NextEpisode = NextEpisode{}
		} else {
			// Preferred-first resolve; diagnosed recovery only after that fails.
			resolvedLink, ok := resolveEpisodeLinksWithRecovery(userCurdConfig, &anime, nil, true)
			if !ok {
				ExitCurd(nil)
			}
			anime.Ep.Links = resolvedLink.Links
			applyStreamPlaybackHints(&anime, anime.Ep.Links, resolvedLink.LinkHints)
		}

		if len(anime.Ep.Links) == 0 {
			ExitCurd(fmt.Errorf("No episode links found"))
		}

		title := fmt.Sprintf("%s - Episode %d", GetAnimeName(anime), anime.Ep.Number)
		CurdOut(title)

		// Prefetch next episode in preferred mode only (no audio-mode prompts in background).
		go prefetchNextUntrackedEpisode(userCurdConfig, &anime)

		// Start with provider fallback when playback never begins (shared with tracked path).
		mpvSocketPath := StartVideoWithProviderFallback(userCurdConfig, &anime, title)
		anime.Ep.Player.SocketPath = mpvSocketPath
		anime.Ep.Started = false
		anime.Ep.IsCompleted = false
		anime.Ep.Player.PlaybackTime = 0
		anime.Ep.Duration = 0
		anime.Ep.SkipTimes = SkipTimes{}

		Log(fmt.Sprintf("Started mpv with socket path: %s", anime.Ep.Player.SocketPath))

		// Android intent path: external player, no IPC monitoring.
		if mpvSocketPath == "android-intent" {
			CurdOut(fmt.Sprintf("\nOpened external player for Episode %d.", anime.Ep.Number))
			CurdOut("Press Enter when you have finished watching...")
			var input string
			fmt.Scanln(&input)
			anime.Ep.Number++
			anime.Ep.Started = false
			continue
		}

		// Get video duration once playback has started.
		go func() {
			for {
				if anime.Ep.Started {
					if anime.Ep.Duration == 0 {
						durationPos, err := MPVSendCommand(anime.Ep.Player.SocketPath, []interface{}{"get_property", "duration"})
						if err != nil {
							Log("Error getting video duration: " + err.Error())
						} else if durationPos != nil {
							if duration, ok := durationPos.(float64); ok {
								anime.Ep.Duration = int(duration + 0.5)
								Log(fmt.Sprintf("Video duration: %d seconds", anime.Ep.Duration))
							} else {
								Log("Error: duration is not a float64")
							}
						}
						break
					}
				}
				time.Sleep(1 * time.Second)
			}
		}()

		// Monitor playback: position, speed save, OP/ED skip (when times are available).
		for {
			timePos, err := MPVSendCommand(anime.Ep.Player.SocketPath, []interface{}{"get_property", "time-pos"})
			if err != nil {
				Log("Error getting playback time: " + err.Error())

				if anime.Ep.Started {
					percentageWatched := PercentageWatched(anime.Ep.Player.PlaybackTime, anime.Ep.Duration)
					action := ClassifyPlaybackLoss(
						anime.Ep.Player.SocketPath,
						anime.Ep.Started,
						percentageWatched,
						userCurdConfig.PercentageToMarkComplete,
					)
					Log(fmt.Sprintf("untracked playback loss: pct=%.1f action=%d", percentageWatched, action))
					switch action {
					case PlaybackLossWait:
						time.Sleep(200 * time.Millisecond)
						continue
					case PlaybackLossComplete:
						anime.Ep.Number++
						anime.Ep.Started = false
						Log("Completed episode, starting next.")
						anime.Ep.IsCompleted = true
						// Break the playback monitor loop (not just the switch).
						goto nextUntrackedEpisode
					case PlaybackLossExit:
						Log("Episode is not completed, exiting")
						ExitCurd(nil)
					}
				} else if isMPVConnectionGoneError(err) {
					// Should be rare after StartVideoWithProviderFallback, but exit cleanly.
					Log("MPV exited before untracked playback was marked started")
					ExitCurd(nil)
				}
			}

			if timePos != nil {
				if !anime.Ep.Started {
					anime.Ep.Started = true
					if userCurdConfig.SaveMpvSpeed && anime.Ep.Player.Speed > 0 {
						speedCmd := []interface{}{"set_property", "speed", anime.Ep.Player.Speed}
						if _, speedErr := MPVSendCommand(anime.Ep.Player.SocketPath, speedCmd); speedErr != nil {
							Log("Error setting playback speed: " + speedErr.Error())
						}
					}
					// Only push chapters when AniSkip-style times are actually present.
					if anime.Ep.SkipTimes.Op.Start != anime.Ep.SkipTimes.Op.End ||
						anime.Ep.SkipTimes.Ed.Start != anime.Ep.SkipTimes.Ed.End {
						if skipErr := SendSkipTimesToMPV(&anime); skipErr != nil {
							Log("Error sending skip times to MPV: " + skipErr.Error())
						}
					}
				}

				animePosition, ok := timePos.(float64)
				if !ok {
					Log("Error: timePos is not a float64")
					continue
				}

				anime.Ep.Player.PlaybackTime = int(animePosition + 0.5)

				// Skip OP/ED when skip times are known (non-tracking; needs MalId for AniSkip).
				if userCurdConfig.SkipOp {
					if anime.Ep.Player.PlaybackTime > anime.Ep.SkipTimes.Op.Start &&
						anime.Ep.Player.PlaybackTime < anime.Ep.SkipTimes.Op.Start+2 &&
						anime.Ep.SkipTimes.Op.Start != anime.Ep.SkipTimes.Op.End {
						SeekMPV(anime.Ep.Player.SocketPath, anime.Ep.SkipTimes.Op.End)
					}
				}
				if userCurdConfig.SkipEd {
					if anime.Ep.Player.PlaybackTime > anime.Ep.SkipTimes.Ed.Start &&
						anime.Ep.Player.PlaybackTime < anime.Ep.SkipTimes.Ed.Start+2 &&
						anime.Ep.SkipTimes.Ed.Start != anime.Ep.SkipTimes.Ed.End {
						SeekMPV(anime.Ep.Player.SocketPath, anime.Ep.SkipTimes.Ed.End)
					}
				}

				if speed, speedErr := GetMPVPlaybackSpeed(anime.Ep.Player.SocketPath); speedErr == nil {
					anime.Ep.Player.Speed = speed
				}
			}
			time.Sleep(1 * time.Second)
		}
	nextUntrackedEpisode:
	}

}

// prefetchNextUntrackedEpisode resolves the next episode in preferred SubOrDub only
// (no audio-mode prompts) so the next loop iteration can start faster.
func prefetchNextUntrackedEpisode(userCurdConfig *CurdConfig, anime *Anime) {
	if userCurdConfig == nil || anime == nil {
		return
	}
	nextEpNum := anime.Ep.Number + 1
	nextEpisode := *anime
	nextEpisode.ProviderId = anime.ProviderId
	nextEpisode.ProviderName = anime.ProviderName
	if providerNoPrefetch(CurrentAnimeProviderName(&nextEpisode)) {
		anime.Ep.NextEpisode = NextEpisode{}
		return
	}
	nextResult, err := ResolveEpisodeURL(*userCurdConfig, &nextEpisode, nextEpNum)
	if err != nil {
		Log(fmt.Sprintf("Error getting next untracked episode link for ep %d: %v", nextEpNum, err))
		return
	}
	if providerNoPrefetch(nextResult.ProviderName) {
		anime.Ep.NextEpisode = NextEpisode{}
		return
	}
	anime.Ep.NextEpisode = NextEpisode{
		Number:       nextEpNum,
		Links:        nextResult.Links,
		ProviderName: nextResult.ProviderName,
		ProviderId:   nextResult.ProviderID,
		Mode:         nextResult.Mode,
	}
}
