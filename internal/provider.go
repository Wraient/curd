package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// GetProviderToken gets the token for the configured provider
func GetProviderToken(config *CurdConfig) (string, error) {
	provider := strings.ToLower(config.AnimeProvider)

	var tokenPath string
	if provider == "mal" {
		tokenPath = filepath.Join(os.ExpandEnv(config.StoragePath), "mal_token.json")
	} else {
		tokenPath = filepath.Join(os.ExpandEnv(config.StoragePath), "anilist_token.json")
	}

	token, err := GetTokenFromFile(tokenPath)
	if err != nil {
		return "", err
	}

	return token, nil
}

// GetProviderUserID gets the user ID and username based on provider
func GetProviderUserID(config *CurdConfig, token string) (int, string, error) {
	provider := strings.ToLower(config.AnimeProvider)

	if provider == "mal" {
		user, err := GetMALUserInfo(token)
		if err != nil {
			return 0, "", fmt.Errorf("failed to get MAL user info: %w", err)
		}
		return user.ID, user.Name, nil
	}

	// Default to AniList
	return GetAnilistUserID(token)
}

// GetProviderAnimeList gets anime list from the configured provider
func GetProviderAnimeList(config *CurdConfig, token string, userID int) (AnimeList, error) {
	provider := strings.ToLower(config.AnimeProvider)

	if provider == "mal" {
		// Get all anime list statuses from MAL
		statuses := []string{"watching", "completed", "on_hold", "dropped", "plan_to_watch"}
		var allAnime AnimeList

		for _, status := range statuses {
			malList, err := GetMALAnimeList(token, status, 1000, 0)
			if err != nil {
				return AnimeList{}, fmt.Errorf("failed to get MAL anime list for status %s: %w", status, err)
			}

			// Parse and merge
			parsedList := ParseMALAnimeList(malList)
			allAnime.Watching = append(allAnime.Watching, parsedList.Watching...)
			allAnime.Completed = append(allAnime.Completed, parsedList.Completed...)
			allAnime.Paused = append(allAnime.Paused, parsedList.Paused...)
			allAnime.Dropped = append(allAnime.Dropped, parsedList.Dropped...)
			allAnime.Planning = append(allAnime.Planning, parsedList.Planning...)
			allAnime.Rewatching = append(allAnime.Rewatching, parsedList.Rewatching...)
		}

		return allAnime, nil
	}

	// Default to AniList
	if config.RofiSelection && config.ImagePreview {
		anilistUserData, err := GetUserDataPreview(token, userID)
		if err != nil {
			return AnimeList{}, fmt.Errorf("failed to get AniList user data preview: %w", err)
		}
		return ParseAnimeList(anilistUserData), nil
	}

	anilistUserData, err := GetUserData(token, userID)
	if err != nil {
		return AnimeList{}, fmt.Errorf("failed to get AniList user data: %w", err)
	}
	return ParseAnimeList(anilistUserData), nil
}

// SearchProviderAnime searches for anime using the configured provider
func SearchProviderAnime(config *CurdConfig, token, query string) ([]SelectionOption, error) {
	provider := strings.ToLower(config.AnimeProvider)

	if provider == "mal" {
		return SearchMALAnimeSimple(token, query)
	}

	// Default to AniList
	return SearchAnimeAnilist(query, token)
}

// UpdateProviderAnimeProgress updates anime progress on the configured provider
func UpdateProviderAnimeProgress(config *CurdConfig, token string, animeID, progress int) error {
	provider := strings.ToLower(config.AnimeProvider)

	if provider == "mal" {
		// Pass -1 for score to skip updating it (only update episodes)
		return UpdateMALAnimeStatus(token, animeID, "", -1, progress)
	}

	// Default to AniList
	return UpdateAnimeProgress(token, animeID, progress)
}

// UpdateProviderAnimeStatus updates anime status on the configured provider
func UpdateProviderAnimeStatus(config *CurdConfig, token string, animeID int, status string) error {
	provider := strings.ToLower(config.AnimeProvider)

	if provider == "mal" {
		malStatus := ConvertAnilistStatusToMAL(status)
		// Pass -1 for score and episodes to skip updating them (only update status)
		return UpdateMALAnimeStatus(token, animeID, malStatus, -1, -1)
	}

	// Default to AniList
	return UpdateAnimeStatus(token, animeID, status)
}

// RateProviderAnime rates an anime on the configured provider
func RateProviderAnime(config *CurdConfig, token string, animeID int, score float64) error {
	provider := strings.ToLower(config.AnimeProvider)

	if provider == "mal" {
		// MAL uses 0-10 integer scale
		malScore := int(score)
		if malScore > 10 {
			malScore = 10
		}
		return UpdateMALAnimeStatus(token, animeID, "", malScore, -1)
	}

	// Default to AniList
	return RateAnime(token, animeID)
}

// RateProviderAnimeWithPrompt prompts the user for a score and rates the anime on the configured provider
func RateProviderAnimeWithPrompt(config *CurdConfig, token string, animeID int) error {
	var score float64
	var err error

	if config.RofiSelection {
		userInput, err := GetUserInputFromRofi("Enter a score for the anime (0-10)")
		if err != nil {
			Log(fmt.Sprintf("Failed to get score from Rofi: %v", err))
			return err
		}
		score, err = strconv.ParseFloat(userInput, 64)
		if err != nil {
			Log(fmt.Sprintf("Failed to parse score: %v", err))
			return err
		}
	} else {
		CurdOut("Rate this anime (0-10): ")
		fmt.Scanln(&score)
	}

	Log(fmt.Sprintf("User entered score: %.2f for anime ID: %d", score, animeID))

	provider := strings.ToLower(config.AnimeProvider)

	if provider == "mal" {
		// MAL uses 0-10 integer scale
		malScore := int(score)
		if malScore > 10 {
			malScore = 10
		}
		if malScore < 0 {
			malScore = 0
		}
		Log(fmt.Sprintf("Calling UpdateMALAnimeStatus with score=%d for anime ID=%d", malScore, animeID))
		err = UpdateMALAnimeStatus(token, animeID, "", malScore, -1)
		if err != nil {
			Log(fmt.Sprintf("UpdateMALAnimeStatus failed: %v", err))
			return err
		}
		CurdOut(fmt.Sprintf("Successfully rated anime (ID: %d) with score: %d", animeID, malScore))
		return nil
	}

	// Default to AniList
	return RateAnime(token, animeID)
}

// AddProviderAnimeToWatching adds anime to watching list on the configured provider
func AddProviderAnimeToWatching(config *CurdConfig, token string, animeID int) error {
	provider := strings.ToLower(config.AnimeProvider)

	if provider == "mal" {
		return UpdateMALAnimeStatus(token, animeID, "watching", 0, 0)
	}

	// Default to AniList
	return AddAnimeToWatchingList(animeID, token)
}

// GetProviderAnimeDetails gets anime details from the configured provider
func GetProviderAnimeDetails(config *CurdConfig, token string, animeID int) (*Anime, error) {
	provider := strings.ToLower(config.AnimeProvider)

	if provider == "mal" {
		malAnime, err := GetMALAnimeDetails(token, animeID)
		if err != nil {
			return nil, fmt.Errorf("failed to get MAL anime details: %w", err)
		}

		// Convert MAL anime to internal Anime struct
		anime := &Anime{
			AnilistId:     animeID, // For MAL, we use MAL ID
			MalId:         animeID,
			TotalEpisodes: malAnime.NumEpisodes,
			Title: AnimeTitle{
				Romaji:   malAnime.Title,
				English:  malAnime.AlternativeTitles.En,
				Japanese: malAnime.AlternativeTitles.Ja,
			},
			CoverImage: malAnime.MainPicture.Large,
		}

		// Check if currently airing
		anime.IsAiring = malAnime.Status == "currently_airing"

		return anime, nil
	}

	// Default to AniList
	anime, err := GetAnimeDataByID(animeID, token)
	if err != nil {
		return nil, err
	}
	return &anime, nil
}

// FindProviderAnimeByID finds an anime in the anime list by ID
func FindProviderAnimeByID(config *CurdConfig, list AnimeList, idStr string) (*Entry, error) {
	// This function works the same regardless of provider since we normalize the data
	return FindAnimeByAnilistID(list, idStr)
}

// GetProviderAnimeMap gets a map of anime IDs to titles from the anime list
func GetProviderAnimeMap(config *CurdConfig, animeList AnimeList) map[string]string {
	// This function works the same regardless of provider since we normalize the data
	return GetAnimeMap(animeList)
}

// GetProviderAnimeMapPreview gets a map with cover images for rofi preview
func GetProviderAnimeMapPreview(config *CurdConfig, animeList AnimeList) map[string]RofiSelectPreview {
	// This function works the same regardless of provider since we normalize the data
	return GetAnimeMapPreview(animeList)
}

// GetProviderAnimeMalID gets the MAL ID for an anime
// For MAL provider, the anime ID is already the MAL ID
// For AniList provider, we need to fetch it from AniList
func GetProviderAnimeMalID(config *CurdConfig, animeID int) (int, error) {
	provider := strings.ToLower(config.AnimeProvider)

	if provider == "mal" {
		// Already using MAL ID
		return animeID, nil
	}

	// Get MAL ID from AniList
	return GetAnimeMalID(animeID)
}

// ConvertIDIfNeeded converts between provider IDs if needed
// This is useful when working with external APIs like MAL ID for filler lists
func ConvertIDIfNeeded(config *CurdConfig, animeID int, targetProvider string) (int, error) {
	currentProvider := strings.ToLower(config.AnimeProvider)
	targetProvider = strings.ToLower(targetProvider)

	if currentProvider == targetProvider {
		return animeID, nil
	}

	if currentProvider == "anilist" && targetProvider == "mal" {
		return GetAnimeMalID(animeID)
	}

	if currentProvider == "mal" && targetProvider == "anilist" {
		// There's no direct MAL to AniList conversion in the current codebase
		// This would require querying AniList with the MAL ID
		return 0, fmt.Errorf("MAL to AniList ID conversion not implemented")
	}

	return animeID, nil
}