package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

const animeListCacheFileName = "anilist_list_cache.json"

type animeListCachePayload struct {
	AnimeList AnimeList `json:"anime_list"`
	UpdatedAt time.Time `json:"updated_at"`
	UserID    int       `json:"user_id"`
}

type AnimeListSync struct {
	mu          sync.RWMutex
	current     AnimeList
	updates     chan AnimeList
	refreshDone chan struct{} // closed exactly once when the background refresh finishes
	closeOnce   sync.Once
}

func NewAnimeListSync(initial AnimeList) *AnimeListSync {
	return &AnimeListSync{
		current:     initial,
		updates:     make(chan AnimeList, 1),
		refreshDone: make(chan struct{}),
	}
}

// MarkRefreshDone closes the refreshDone channel exactly once.
func (s *AnimeListSync) MarkRefreshDone() {
	s.closeOnce.Do(func() { close(s.refreshDone) })
}

// RefreshDone returns a channel that is closed when the background refresh finishes.
func (s *AnimeListSync) RefreshDone() <-chan struct{} {
	return s.refreshDone
}

func (s *AnimeListSync) Current() AnimeList {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current
}

func (s *AnimeListSync) Replace(list AnimeList, notify bool) bool {
	s.mu.Lock()
	changed := !animeListEqual(s.current, list)
	s.current = list
	s.mu.Unlock()

	if changed && notify {
		select {
		case s.updates <- list:
		default:
			select {
			case <-s.updates:
			default:
			}
			s.updates <- list
		}
	}

	return changed
}

func (s *AnimeListSync) Updates() <-chan AnimeList {
	return s.updates
}

func animeListCachePath(storagePath string) string {
	return filepath.Join(os.ExpandEnv(storagePath), animeListCacheFileName)
}

func loadAnimeListCache(storagePath string, userID int) (animeListCachePayload, error) {
	cacheFilePath := animeListCachePath(storagePath)
	data, err := os.ReadFile(cacheFilePath)
	if err != nil {
		return animeListCachePayload{}, err
	}

	var payload animeListCachePayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return animeListCachePayload{}, fmt.Errorf("failed to parse anime list cache: %w", err)
	}

	if payload.UserID != 0 && userID != 0 && payload.UserID != userID {
		return animeListCachePayload{}, fmt.Errorf("anime list cache belongs to a different AniList user")
	}

	return payload, nil
}

func saveAnimeListCache(storagePath string, userID int, list AnimeList) error {
	storagePath = os.ExpandEnv(storagePath)
	if err := os.MkdirAll(storagePath, 0o755); err != nil {
		return fmt.Errorf("failed to create storage directory: %w", err)
	}

	payload := animeListCachePayload{
		AnimeList: list,
		UpdatedAt: time.Now(),
		UserID:    userID,
	}

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal anime list cache: %w", err)
	}

	cacheFilePath := animeListCachePath(storagePath)
	tempFilePath := cacheFilePath + ".tmp"
	if err := os.WriteFile(tempFilePath, data, 0o644); err != nil {
		return fmt.Errorf("failed to write anime list cache: %w", err)
	}

	if err := os.Rename(tempFilePath, cacheFilePath); err != nil {
		_ = os.Remove(tempFilePath)
		return fmt.Errorf("failed to replace anime list cache: %w", err)
	}

	return nil
}

func animeListEqual(a, b AnimeList) bool {
	left, err := json.Marshal(a)
	if err != nil {
		return false
	}
	right, err := json.Marshal(b)
	if err != nil {
		return false
	}
	return string(left) == string(right)
}

func FetchLatestAnimeList(token string, userID int) (AnimeList, error) {
	userData, err := GetUserDataPreview(token, userID)
	if err != nil {
		return AnimeList{}, err
	}

	return ParseAnimeList(userData), nil
}

func refreshAnimeListInBackground(userCurdConfig *CurdConfig, user *User) {
	if user == nil || user.ListSync == nil {
		return
	}

	go func() {
		// Signal done regardless of success/failure so callers never block forever.
		defer user.ListSync.MarkRefreshDone()

		// Only fetch the user ID when we don't already have it (first run, no cache).
		// On cache hits user.Id is already seeded, so skip this extra round-trip.
		if user.Id == 0 {
			userID, username, err := GetAnilistUserID(user.Token)
			if err != nil {
				Log(fmt.Sprintf("Failed to refresh user ID in background: %v", err))
				return
			}
			user.Id = userID
			if user.Username == "" {
				user.Username = username
			}
		}

		latestList, err := FetchLatestAnimeList(user.Token, user.Id)
		if err != nil {
			Log(fmt.Sprintf("Failed to refresh anime list in background: %v", err))
			return
		}

		if err := saveAnimeListCache(userCurdConfig.StoragePath, user.Id, latestList); err != nil {
			Log(fmt.Sprintf("Failed to save refreshed anime list cache: %v", err))
		}

		user.ListSync.Replace(latestList, true)
	}()
}

func InitializeUserAnimeList(userCurdConfig *CurdConfig, user *User) error {
	cachedPayload, err := loadAnimeListCache(userCurdConfig.StoragePath, user.Id)
	if err == nil {
		// Seed user ID from cache so we skip the blocking GetAnilistUserID network call.
		if user.Id == 0 && cachedPayload.UserID != 0 {
			user.Id = cachedPayload.UserID
		}
		user.AnimeList = cachedPayload.AnimeList
		user.ListSync = NewAnimeListSync(cachedPayload.AnimeList)
		// Refresh user ID + anime list in the background (non-blocking).
		refreshAnimeListInBackground(userCurdConfig, user)
		return nil
	}

	if !os.IsNotExist(err) {
		Log(fmt.Sprintf("Failed to load anime list cache, fetching latest instead: %v", err))
	}

	// No cache — blocking fetch is unavoidable on first run.
	if user.Id == 0 {
		userID, username, idErr := GetAnilistUserID(user.Token)
		if idErr != nil {
			return idErr
		}
		user.Id = userID
		user.Username = username
	}

	latestList, err := FetchLatestAnimeList(user.Token, user.Id)
	if err != nil {
		return err
	}

	user.AnimeList = latestList
	user.ListSync = NewAnimeListSync(latestList)
	// Blocking fetch already has the freshest data — mark done immediately.
	user.ListSync.MarkRefreshDone()
	if err := saveAnimeListCache(userCurdConfig.StoragePath, user.Id, latestList); err != nil {
		Log(fmt.Sprintf("Failed to save anime list cache: %v", err))
	}

	return nil
}

func RefreshUserAnimeList(userCurdConfig *CurdConfig, user *User) error {
	if user.Id == 0 {
		userID, username, err := GetAnilistUserID(user.Token)
		if err != nil {
			return err
		}
		user.Id = userID
		if user.Username == "" {
			user.Username = username
		}
	}

	latestList, err := FetchLatestAnimeList(user.Token, user.Id)
	if err != nil {
		return err
	}

	user.AnimeList = latestList
	if user.ListSync == nil {
		user.ListSync = NewAnimeListSync(latestList)
	} else {
		user.ListSync.Replace(latestList, true)
	}

	if err := saveAnimeListCache(userCurdConfig.StoragePath, user.Id, latestList); err != nil {
		Log(fmt.Sprintf("Failed to save anime list cache: %v", err))
	}

	return nil
}

func buildCategorySelectionOptions(list AnimeList, category string) []SelectionOption {
	userCurdConfig := GetGlobalConfig()
	options := make([]SelectionOption, 0)

	for _, entry := range getEntriesByCategory(list, category) {
		title := entry.Media.Title.English
		if title == "" || userCurdConfig.AnimeNameLanguage == "romaji" {
			title = entry.Media.Title.Romaji
		}

		options = append(options, SelectionOption{
			Key:   strconv.Itoa(entry.Media.ID),
			Label: title,
		})
	}

	return options
}

func buildCategoryPreviewOptions(list AnimeList, category string) map[string]RofiSelectPreview {
	userCurdConfig := GetGlobalConfig()
	options := make(map[string]RofiSelectPreview)

	for _, entry := range getEntriesByCategory(list, category) {
		title := entry.Media.Title.English
		if title == "" || userCurdConfig.AnimeNameLanguage == "romaji" {
			title = entry.Media.Title.Romaji
		}

		options[strconv.Itoa(entry.Media.ID)] = RofiSelectPreview{
			Title:      title,
			CoverImage: entry.CoverImage,
		}
	}

	return options
}
