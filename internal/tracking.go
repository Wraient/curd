package internal

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	TrackingRemoteNone        = "none"
	TrackingRemoteAniList     = "anilist"
	TrackingRemoteMyAnimeList = "myanimelist"
	TrackingRemoteBoth        = "anilist+myanimelist"
)

func UsesLocalTracking(config *CurdConfig) bool {
	return true
}

func UsesRemoteTracking(config *CurdConfig) bool {
	if config == nil {
		return true
	}
	return normalizeRemoteTracker(config.TrackingRemote) != TrackingRemoteNone
}

func UsesAniListTracking(config *CurdConfig) bool {
	if config == nil {
		return true
	}
	switch normalizeRemoteTracker(config.TrackingRemote) {
	case TrackingRemoteAniList, TrackingRemoteBoth:
		return true
	default:
		return false
	}
}

func UsesMyAnimeListTracking(config *CurdConfig) bool {
	if config == nil {
		return false
	}
	switch normalizeRemoteTracker(config.TrackingRemote) {
	case TrackingRemoteMyAnimeList, TrackingRemoteBoth:
		return true
	default:
		return false
	}
}

func UsesDualRemoteTracking(config *CurdConfig) bool {
	if config == nil {
		return false
	}
	return normalizeRemoteTracker(config.TrackingRemote) == TrackingRemoteBoth
}

func trackingCategoryEnabled(config *CurdConfig, key string) bool {
	switch key {
	case "CURRENT", "ALL", "UNTRACKED", "CONTINUE_LAST", "PROVIDER":
		return true
	case "UPDATE", "PLANNING", "COMPLETED", "PAUSED", "DROPPED", "REWATCHING":
		return UsesRemoteTracking(config)
	default:
		return true
	}
}

func EnsureTrackingConfigured(config *CurdConfig) error {
	if config == nil {
		return fmt.Errorf("missing config")
	}

	normalizeTrackingConfig(config)
	if config.TrackingConfigured {
		return persistTrackingConfig(config)
	}

	options := []SelectionOption{
		{Key: "local", Label: "local"},
		{Key: "anilist", Label: "anilist"},
		{Key: "myanimelist", Label: "myanimelist"},
		{Key: "anilist+myanimelist", Label: "anilist + myanimelist"},
	}

	CurdOut("Choose tracking mode. Local history stays enabled in every mode.")
	selected, err := DynamicSelect(options)
	if err != nil {
		return err
	}

	switch selected.Key {
	case "local":
		config.TrackingLocal = true
		config.TrackingRemote = TrackingRemoteNone
	case "anilist":
		config.TrackingLocal = true
		config.TrackingRemote = TrackingRemoteAniList
	case "myanimelist":
		config.TrackingLocal = true
		config.TrackingRemote = TrackingRemoteMyAnimeList
	case "anilist+myanimelist":
		config.TrackingLocal = true
		config.TrackingRemote = TrackingRemoteBoth
	case "-1", "-2":
		return fmt.Errorf("tracking setup cancelled")
	default:
		return fmt.Errorf("unsupported tracking selection: %s", selected.Key)
	}

	config.TrackingConfigured = true
	normalizeTrackingConfig(config)
	return persistTrackingConfig(config)
}

func persistTrackingConfig(config *CurdConfig) error {
	if config == nil || GlobalConfigPath == "" {
		return nil
	}

	configMap, err := LoadConfigFromFile(GlobalConfigPath)
	if err != nil {
		return err
	}

	configMap["TrackingLocal"] = strconv.FormatBool(config.TrackingLocal)
	configMap["TrackingRemote"] = config.TrackingRemote
	configMap["TrackingConfigured"] = strconv.FormatBool(config.TrackingConfigured)
	configMap["MyAnimeListClientID"] = config.MyAnimeListClientID
	configMap["MyAnimeListClientSecret"] = config.MyAnimeListClientSecret
	configMap["MyAnimeListImported"] = strconv.FormatBool(config.MyAnimeListImported)

	return SaveConfigToFile(GlobalConfigPath, configMap)
}

func promptTrackingInput(prompt string, allowEmpty bool) (string, error) {
	config := GetGlobalConfig()
	if config != nil && config.RofiSelection {
		value, err := GetUserInputFromRofi(prompt)
		if err != nil {
			return "", err
		}
		value = strings.TrimSpace(value)
		if value == "" && !allowEmpty {
			return "", fmt.Errorf("input required")
		}
		return value, nil
	}

	fmt.Print(prompt + ": ")
	reader := bufio.NewReader(os.Stdin)
	value, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	value = strings.TrimSpace(value)
	if value == "" && !allowEmpty {
		return "", fmt.Errorf("input required")
	}
	return value, nil
}

func promptAnimeScoreValue() (float64, error) {
	config := GetGlobalConfig()
	if config != nil && config.RofiSelection {
		userInput, err := GetUserInputFromRofi("Enter a score for the anime (0-10)")
		if err != nil {
			return 0, err
		}
		return strconv.ParseFloat(strings.TrimSpace(userInput), 64)
	}

	fmt.Println("Rate this anime: ")
	var score float64
	fmt.Scanln(&score)
	return score, nil
}

func ensureMyAnimeListCredentialsConfigured(config *CurdConfig) error {
	clientID, _ := myAnimeListClientCredentials(config)
	if clientID != "" {
		return nil
	}

	CurdOut("MyAnimeList sync needs MAL application credentials.")
	clientIDInput, err := promptTrackingInput("Enter your MyAnimeList client ID", false)
	if err != nil {
		return err
	}
	clientSecretInput, err := promptTrackingInput("Enter your MyAnimeList client secret (optional)", true)
	if err != nil {
		return err
	}

	config.MyAnimeListClientID = clientIDInput
	config.MyAnimeListClientSecret = clientSecretInput
	return persistTrackingConfig(config)
}

func ensureAniListTrackerReady(config *CurdConfig, user *User) error {
	tokenPath := filepath.Join(os.ExpandEnv(config.StoragePath), "anilist_token.json")
	if token, err := GetTokenFromFile(tokenPath); err == nil && strings.TrimSpace(token) != "" {
		if user != nil && user.Token == "" {
			user.Token = token
		}
		return nil
	}

	ChangeToken(config, user)
	return nil
}

func ensureMyAnimeListTrackerReady(config *CurdConfig, user *User) error {
	if err := ensureMyAnimeListCredentialsConfigured(config); err != nil {
		return err
	}
	if token, err := GetMyAnimeListAccessToken(config); err == nil && strings.TrimSpace(token) != "" {
		return nil
	}
	return ChangeMyAnimeListToken(config, user)
}

func EnsureConfiguredTrackersReady(config *CurdConfig, user *User) error {
	if config == nil {
		return fmt.Errorf("missing config")
	}

	if UsesAniListTracking(config) {
		if err := ensureAniListTrackerReady(config, user); err != nil {
			return err
		}
	}
	if UsesMyAnimeListTracking(config) {
		if err := ensureMyAnimeListTrackerReady(config, user); err != nil {
			return err
		}
	}

	return LoadTokenForConfiguredTracking(config, user)
}

func localAnimeListFromHistory(history []Anime) AnimeList {
	list := AnimeList{}
	for _, anime := range history {
		title := anime.Title
		if title.English == "" && title.Romaji == "" {
			title.English = strconv.Itoa(anime.AnilistId)
			title.Romaji = title.English
		}

		progress := 0
		if anime.Ep.Number > 0 {
			progress = anime.Ep.Number - 1
		}

		entry := Entry{
			Media: Media{
				ID:       anime.AnilistId,
				MalID:    anime.MalId,
				Title:    title,
				Episodes: anime.TotalEpisodes,
			},
			Progress: progress,
			Status:   "CURRENT",
		}
		list.Watching = append(list.Watching, entry)
	}
	return list
}

func BuildLocalAnimeList(storagePath string) AnimeList {
	historyPath := filepath.Join(os.ExpandEnv(storagePath), "curd_history.txt")
	return localAnimeListFromHistory(LocalGetAllAnime(historyPath))
}

func hasAnyEntries(list AnimeList) bool {
	return len(list.Watching)+len(list.Completed)+len(list.Paused)+len(list.Dropped)+len(list.Planning)+len(list.Rewatching) > 0
}

func currentFuzzyDate() FuzzyDate {
	now := time.Now()
	return FuzzyDate{
		Year:  now.Year(),
		Month: int(now.Month()),
		Day:   now.Day(),
	}
}

func entryFreshnessScore(entry Entry) int {
	score := entry.Progress * 100
	switch entry.Status {
	case "REPEATING":
		score += 60
	case "CURRENT":
		score += 50
	case "COMPLETED":
		score += 40
	case "PAUSED":
		score += 30
	case "PLANNING":
		score += 20
	case "DROPPED":
		score += 10
	}
	if entry.Score > 0 {
		score += int(entry.Score * 10)
	}
	if entry.Repeat > 0 {
		score += entry.Repeat
	}
	if entry.CoverImage != "" {
		score++
	}
	return score
}

func mergeEntryMetadata(preferred, fallback Entry) Entry {
	if preferred.ListID == 0 {
		preferred.ListID = fallback.ListID
	}
	if preferred.Media.ID == 0 {
		preferred.Media.ID = fallback.Media.ID
	}
	if preferred.Media.MalID == 0 {
		preferred.Media.MalID = fallback.Media.MalID
	}
	if preferred.Media.Duration == 0 {
		preferred.Media.Duration = fallback.Media.Duration
	}
	if preferred.Media.Episodes == 0 {
		preferred.Media.Episodes = fallback.Media.Episodes
	}
	if preferred.Media.Title.English == "" {
		preferred.Media.Title.English = fallback.Media.Title.English
	}
	if preferred.Media.Title.Romaji == "" {
		preferred.Media.Title.Romaji = fallback.Media.Title.Romaji
	}
	if preferred.Media.Title.Japanese == "" {
		preferred.Media.Title.Japanese = fallback.Media.Title.Japanese
	}
	if preferred.Media.Status == "" {
		preferred.Media.Status = fallback.Media.Status
	}
	if preferred.CoverImage == "" {
		preferred.CoverImage = fallback.CoverImage
	}
	if preferred.Repeat == 0 && fallback.Repeat > 0 {
		preferred.Repeat = fallback.Repeat
	}
	return preferred
}

func mergeAnimeEntries(existing, incoming Entry) Entry {
	switch {
	case existing.UpdatedAt.IsZero() && !incoming.UpdatedAt.IsZero():
		return mergeEntryMetadata(incoming, existing)
	case !existing.UpdatedAt.IsZero() && incoming.UpdatedAt.IsZero():
		return mergeEntryMetadata(existing, incoming)
	case existing.UpdatedAt.After(incoming.UpdatedAt):
		return mergeEntryMetadata(existing, incoming)
	case incoming.UpdatedAt.After(existing.UpdatedAt):
		return mergeEntryMetadata(incoming, existing)
	case entryFreshnessScore(incoming) > entryFreshnessScore(existing):
		return mergeEntryMetadata(incoming, existing)
	default:
		return mergeEntryMetadata(existing, incoming)
	}
}

func mergeAnimeLists(primary, secondary AnimeList) AnimeList {
	merged := make(map[int]Entry)
	for _, entry := range getEntriesByCategory(primary, "ALL") {
		merged[entry.Media.ID] = entry
	}
	for _, entry := range getEntriesByCategory(secondary, "ALL") {
		if existing, ok := merged[entry.Media.ID]; ok {
			merged[entry.Media.ID] = mergeAnimeEntries(existing, entry)
			continue
		}
		merged[entry.Media.ID] = entry
	}

	result := AnimeList{}
	for _, entry := range merged {
		switch entry.Status {
		case "CURRENT":
			result.Watching = append(result.Watching, entry)
		case "COMPLETED":
			result.Completed = append(result.Completed, entry)
		case "PAUSED":
			result.Paused = append(result.Paused, entry)
		case "DROPPED":
			result.Dropped = append(result.Dropped, entry)
		case "PLANNING":
			result.Planning = append(result.Planning, entry)
		case "REPEATING":
			result.Rewatching = append(result.Rewatching, entry)
		}
	}
	return result
}

func mapEntriesByMediaID(list AnimeList) map[int]Entry {
	entries := make(map[int]Entry)
	for _, entry := range getEntriesByCategory(list, "ALL") {
		entries[entry.Media.ID] = entry
	}
	return entries
}

func animeListFromEntryMap(entries map[int]Entry) AnimeList {
	ids := make([]int, 0, len(entries))
	for id := range entries {
		ids = append(ids, id)
	}
	sort.Ints(ids)

	result := AnimeList{}
	for _, id := range ids {
		entry := entries[id]
		switch entry.Status {
		case "CURRENT":
			result.Watching = append(result.Watching, entry)
		case "COMPLETED":
			result.Completed = append(result.Completed, entry)
		case "PAUSED":
			result.Paused = append(result.Paused, entry)
		case "DROPPED":
			result.Dropped = append(result.Dropped, entry)
		case "PLANNING":
			result.Planning = append(result.Planning, entry)
		case "REPEATING":
			result.Rewatching = append(result.Rewatching, entry)
		}
	}
	return result
}

func entriesEquivalentForSync(current, desired Entry) bool {
	current = mergeEntryMetadata(current, desired)
	desired = mergeEntryMetadata(desired, current)

	return current.Media.ID == desired.Media.ID &&
		current.Media.MalID == desired.Media.MalID &&
		current.Progress == desired.Progress &&
		current.Repeat == desired.Repeat &&
		current.Score == desired.Score &&
		current.Status == desired.Status &&
		current.StartedAt == desired.StartedAt &&
		current.CompletedAt == desired.CompletedAt
}

func myAnimeListEntriesEquivalentForSync(current, desired Entry) bool {
	if entriesEquivalentForSync(current, desired) {
		return true
	}

	if current.Repeat == 0 && desired.Repeat > 0 {
		current.Repeat = desired.Repeat
		return entriesEquivalentForSync(current, desired)
	}

	return false
}

type dualRemoteSyncPlan struct {
	Merged             AnimeList
	AniListUpdates     []Entry
	MyAnimeListUpdates []Entry
}

func buildDualRemoteSyncPlan(aniList, myAnimeList AnimeList) dualRemoteSyncPlan {
	aniMap := mapEntriesByMediaID(aniList)
	malMap := mapEntriesByMediaID(myAnimeList)
	unionIDs := make(map[int]struct{}, len(aniMap)+len(malMap))
	for id := range aniMap {
		unionIDs[id] = struct{}{}
	}
	for id := range malMap {
		unionIDs[id] = struct{}{}
	}

	orderedIDs := make([]int, 0, len(unionIDs))
	for id := range unionIDs {
		orderedIDs = append(orderedIDs, id)
	}
	sort.Ints(orderedIDs)

	mergedEntries := make(map[int]Entry, len(orderedIDs))
	plan := dualRemoteSyncPlan{}
	for _, id := range orderedIDs {
		aniEntry, hasAni := aniMap[id]
		malEntry, hasMal := malMap[id]

		switch {
		case hasAni && hasMal:
			winner := mergeAnimeEntries(aniEntry, malEntry)
			mergedEntries[id] = winner
			if !entriesEquivalentForSync(aniEntry, winner) {
				plan.AniListUpdates = append(plan.AniListUpdates, winner)
			}
			if !myAnimeListEntriesEquivalentForSync(malEntry, winner) {
				plan.MyAnimeListUpdates = append(plan.MyAnimeListUpdates, winner)
			}
		case hasAni:
			mergedEntries[id] = aniEntry
			plan.MyAnimeListUpdates = append(plan.MyAnimeListUpdates, aniEntry)
		case hasMal:
			mergedEntries[id] = malEntry
			plan.AniListUpdates = append(plan.AniListUpdates, malEntry)
		}
	}

	plan.Merged = animeListFromEntryMap(mergedEntries)
	return plan
}

func syncDualRemoteTrackers(config *CurdConfig, aniListToken string, aniListUser, myAnimeListUser *User) (AnimeList, error) {
	if config == nil {
		return AnimeList{}, fmt.Errorf("missing config")
	}
	if aniListUser == nil || myAnimeListUser == nil {
		return AnimeList{}, fmt.Errorf("missing tracker users")
	}

	plan := buildDualRemoteSyncPlan(aniListUser.AnimeList, myAnimeListUser.AnimeList)
	for index, entry := range plan.AniListUpdates {
		if index > 0 {
			time.Sleep(350 * time.Millisecond)
		}
		if err := saveAniListTrackedEntry(aniListToken, entry); err != nil {
			return AnimeList{}, err
		}
	}
	for _, entry := range plan.MyAnimeListUpdates {
		if err := saveMyAnimeListTrackedEntry(config, entry); err != nil {
			return AnimeList{}, err
		}
	}

	aniListUser.AnimeList = plan.Merged
	myAnimeListUser.AnimeList = plan.Merged
	if err := saveAniListAnimeListCache(config.StoragePath, aniListUser.Id, plan.Merged); err != nil {
		Log(fmt.Sprintf("Failed to save AniList anime list cache after dual sync: %v", err))
	}
	if err := saveMyAnimeListCache(config.StoragePath, myAnimeListUser.Id, plan.Merged); err != nil {
		Log(fmt.Sprintf("Failed to save MyAnimeList anime list cache after dual sync: %v", err))
	}
	return plan.Merged, nil
}

func InitializeCombinedRemoteAnimeList(config *CurdConfig, user *User) error {
	aniListToken, err := GetTokenFromFile(filepath.Join(os.ExpandEnv(config.StoragePath), "anilist_token.json"))
	if err != nil {
		return err
	}
	myAnimeListToken, err := GetMyAnimeListAccessToken(config)
	if err != nil {
		return err
	}

	aniListUser := &User{Token: aniListToken}
	if err := InitializeAniListUserAnimeList(config, aniListUser); err != nil {
		return err
	}
	myAnimeListUser := &User{Token: myAnimeListToken}
	if err := InitializeMyAnimeListUserAnimeList(config, myAnimeListUser); err != nil {
		return err
	}
	if err := maybeImportAniListToMyAnimeList(config, myAnimeListUser); err != nil {
		return err
	}

	merged, err := syncDualRemoteTrackers(config, aniListToken, aniListUser, myAnimeListUser)
	if err != nil {
		return err
	}

	user.Token = aniListToken
	user.AnimeList = merged
	user.ListSync = NewAnimeListSync(user.AnimeList)
	user.ListSync.MarkRefreshDone()
	return nil
}

func RefreshCombinedRemoteAnimeList(config *CurdConfig, user *User) error {
	aniListToken, err := GetTokenFromFile(filepath.Join(os.ExpandEnv(config.StoragePath), "anilist_token.json"))
	if err != nil {
		return err
	}
	myAnimeListToken, err := GetMyAnimeListAccessToken(config)
	if err != nil {
		return err
	}

	aniListUser := &User{Token: aniListToken}
	if err := RefreshAniListUserAnimeList(config, aniListUser); err != nil {
		if err := InitializeAniListUserAnimeList(config, aniListUser); err != nil {
			return err
		}
	}
	myAnimeListUser := &User{Token: myAnimeListToken}
	if err := RefreshMyAnimeListUserAnimeList(config, myAnimeListUser); err != nil {
		if err := InitializeMyAnimeListUserAnimeList(config, myAnimeListUser); err != nil {
			return err
		}
	}

	merged, err := syncDualRemoteTrackers(config, aniListToken, aniListUser, myAnimeListUser)
	if err != nil {
		return err
	}
	user.Token = aniListToken
	user.AnimeList = merged
	if user.ListSync == nil {
		user.ListSync = NewAnimeListSync(merged)
		user.ListSync.MarkRefreshDone()
		return nil
	}
	user.ListSync.Replace(merged, true)
	user.ListSync.MarkRefreshDone()
	return nil
}

func InitializeUserAnimeList(userCurdConfig *CurdConfig, user *User) error {
	if userCurdConfig == nil || user == nil {
		return fmt.Errorf("missing user or config")
	}

	switch normalizeRemoteTracker(userCurdConfig.TrackingRemote) {
	case TrackingRemoteAniList:
		return InitializeAniListUserAnimeList(userCurdConfig, user)
	case TrackingRemoteMyAnimeList:
		if err := InitializeMyAnimeListUserAnimeList(userCurdConfig, user); err != nil {
			return err
		}
		return maybeImportAniListToMyAnimeList(userCurdConfig, user)
	case TrackingRemoteBoth:
		if err := InitializeCombinedRemoteAnimeList(userCurdConfig, user); err != nil {
			return err
		}
		return maybeImportAniListToMyAnimeList(userCurdConfig, user)
	default:
		list := BuildLocalAnimeList(userCurdConfig.StoragePath)
		user.AnimeList = list
		user.ListSync = NewAnimeListSync(list)
		user.ListSync.MarkRefreshDone()
		return nil
	}
}

func RefreshUserAnimeList(userCurdConfig *CurdConfig, user *User) error {
	if userCurdConfig == nil || user == nil {
		return fmt.Errorf("missing user or config")
	}

	switch normalizeRemoteTracker(userCurdConfig.TrackingRemote) {
	case TrackingRemoteAniList:
		return RefreshAniListUserAnimeList(userCurdConfig, user)
	case TrackingRemoteMyAnimeList:
		return RefreshMyAnimeListUserAnimeList(userCurdConfig, user)
	case TrackingRemoteBoth:
		return RefreshCombinedRemoteAnimeList(userCurdConfig, user)
	default:
		list := BuildLocalAnimeList(userCurdConfig.StoragePath)
		user.AnimeList = list
		if user.ListSync == nil {
			user.ListSync = NewAnimeListSync(list)
			user.ListSync.MarkRefreshDone()
			return nil
		}
		user.ListSync.Replace(list, true)
		user.ListSync.MarkRefreshDone()
		return nil
	}
}

func LoadTokenForConfiguredTracking(config *CurdConfig, user *User) error {
	if config == nil || user == nil {
		return fmt.Errorf("missing config or user")
	}

	switch normalizeRemoteTracker(config.TrackingRemote) {
	case TrackingRemoteAniList:
		token, err := GetTokenFromFile(filepath.Join(os.ExpandEnv(config.StoragePath), "anilist_token.json"))
		if err != nil {
			return err
		}
		user.Token = token
	case TrackingRemoteMyAnimeList:
		token, err := GetMyAnimeListAccessToken(config)
		if err != nil {
			return err
		}
		user.Token = token
	case TrackingRemoteBoth:
		token, err := GetTokenFromFile(filepath.Join(os.ExpandEnv(config.StoragePath), "anilist_token.json"))
		if err == nil && strings.TrimSpace(token) != "" {
			user.Token = token
			return nil
		}
		token, err = GetMyAnimeListAccessToken(config)
		if err != nil {
			return err
		}
		user.Token = token
	default:
		user.Token = ""
	}

	return nil
}

func ChangeTrackingToken(config *CurdConfig, user *User) {
	switch normalizeRemoteTracker(config.TrackingRemote) {
	case TrackingRemoteAniList:
		ChangeToken(config, user)
	case TrackingRemoteMyAnimeList:
		if err := ChangeMyAnimeListToken(config, user); err != nil {
			ExitCurd(err)
		}
	case TrackingRemoteBoth:
		if err := ensureMyAnimeListCredentialsConfigured(config); err != nil {
			ExitCurd(err)
		}
		ChangeToken(config, user)
		if err := ChangeMyAnimeListToken(config, user); err != nil {
			ExitCurd(err)
		}
		if err := LoadTokenForConfiguredTracking(config, user); err != nil {
			ExitCurd(err)
		}
	default:
		CurdOut("Remote tracking is disabled.")
	}
}

func resolveMyAnimeListID(anilistID int) (int, error) {
	if anilistID == 0 {
		return 0, fmt.Errorf("invalid AniList ID")
	}

	if user := GetGlobalUser(); user != nil {
		if entry, err := FindAnimeByAnilistID(user.AnimeList, strconv.Itoa(anilistID)); err == nil && entry.Media.MalID != 0 {
			return entry.Media.MalID, nil
		}
	}

	return GetAnimeMalID(anilistID)
}

func UpdateAnimeProgress(token string, mediaID, progress int) error {
	config := GetGlobalConfig()
	switch {
	case !UsesRemoteTracking(config):
		return nil
	case UsesDualRemoteTracking(config):
		var firstErr error
		if err := UpdateAniListAnimeProgress(token, mediaID, progress); err != nil {
			firstErr = err
		}
		malID, err := resolveMyAnimeListID(mediaID)
		if err != nil {
			if firstErr != nil {
				return firstErr
			}
			return err
		}
		if err := updateMyAnimeListProgress(config, malID, progress); err != nil && firstErr == nil {
			firstErr = err
		}
		return firstErr
	case UsesAniListTracking(config):
		return UpdateAniListAnimeProgress(token, mediaID, progress)
	case UsesMyAnimeListTracking(config):
		malID, err := resolveMyAnimeListID(mediaID)
		if err != nil {
			return err
		}
		return updateMyAnimeListProgress(config, malID, progress)
	default:
		return nil
	}
}

func UpdateAnimeStatus(token string, mediaID int, status string) error {
	config := GetGlobalConfig()
	switch {
	case !UsesRemoteTracking(config):
		return nil
	case UsesDualRemoteTracking(config):
		var firstErr error
		if err := UpdateAniListAnimeStatus(token, mediaID, status); err != nil {
			firstErr = err
		}
		malID, err := resolveMyAnimeListID(mediaID)
		if err != nil {
			if firstErr != nil {
				return firstErr
			}
			return err
		}
		if err := updateMyAnimeListStatus(config, malID, status); err != nil && firstErr == nil {
			firstErr = err
		}
		return firstErr
	case UsesAniListTracking(config):
		return UpdateAniListAnimeStatus(token, mediaID, status)
	case UsesMyAnimeListTracking(config):
		malID, err := resolveMyAnimeListID(mediaID)
		if err != nil {
			return err
		}
		return updateMyAnimeListStatus(config, malID, status)
	default:
		return nil
	}
}

func RateAnime(token string, mediaID int) error {
	config := GetGlobalConfig()
	switch {
	case !UsesRemoteTracking(config):
		return nil
	case UsesDualRemoteTracking(config):
		score, err := promptAnimeScoreValue()
		if err != nil {
			return err
		}
		var firstErr error
		if err := saveAniListAnimeScore(token, mediaID, score); err != nil {
			firstErr = err
		}
		malID, err := resolveMyAnimeListID(mediaID)
		if err != nil {
			if firstErr != nil {
				return firstErr
			}
			return err
		}
		if err := rateMyAnimeListAnimeWithScore(config, malID, int(score+0.5)); err != nil && firstErr == nil {
			firstErr = err
		}
		return firstErr
	case UsesAniListTracking(config):
		return RateAniListAnime(token, mediaID)
	case UsesMyAnimeListTracking(config):
		malID, err := resolveMyAnimeListID(mediaID)
		if err != nil {
			return err
		}
		return rateMyAnimeListAnime(config, malID)
	default:
		return nil
	}
}

func AddAnimeToWatchingList(animeID int, token string) error {
	config := GetGlobalConfig()
	switch {
	case !UsesRemoteTracking(config):
		return nil
	case UsesDualRemoteTracking(config):
		var firstErr error
		if err := AddAniListAnimeToWatchingList(animeID, token); err != nil {
			firstErr = err
		}
		malID, err := resolveMyAnimeListID(animeID)
		if err != nil {
			if firstErr != nil {
				return firstErr
			}
			return err
		}
		if err := addMyAnimeListAnimeToList(config, malID, "CURRENT"); err != nil && firstErr == nil {
			firstErr = err
		}
		return firstErr
	case UsesAniListTracking(config):
		return AddAniListAnimeToWatchingList(animeID, token)
	case UsesMyAnimeListTracking(config):
		malID, err := resolveMyAnimeListID(animeID)
		if err != nil {
			return err
		}
		return addMyAnimeListAnimeToList(config, malID, "CURRENT")
	default:
		return nil
	}
}

func CompleteAnimeRewatch(token string, anime Anime) error {
	config := GetGlobalConfig()
	switch {
	case !UsesRemoteTracking(config):
		return nil
	case UsesDualRemoteTracking(config):
		var firstErr error
		if err := CompleteAniListAnimeRewatch(token, anime); err != nil {
			firstErr = err
		}
		malID := anime.MalId
		if malID == 0 {
			var err error
			malID, err = resolveMyAnimeListID(anime.AnilistId)
			if err != nil {
				if firstErr != nil {
					return firstErr
				}
				return err
			}
		}
		if err := completeMyAnimeListRewatch(config, malID, anime); err != nil && firstErr == nil {
			firstErr = err
		}
		return firstErr
	case UsesAniListTracking(config):
		return CompleteAniListAnimeRewatch(token, anime)
	case UsesMyAnimeListTracking(config):
		malID := anime.MalId
		if malID == 0 {
			var err error
			malID, err = resolveMyAnimeListID(anime.AnilistId)
			if err != nil {
				return err
			}
		}
		return completeMyAnimeListRewatch(config, malID, anime)
	default:
		return nil
	}
}

func StartAnimeRewatch(token string, anime Anime) error {
	config := GetGlobalConfig()
	startedAt := currentFuzzyDate()
	completedAt := FuzzyDate{}
	switch {
	case !UsesRemoteTracking(config):
		return nil
	case UsesDualRemoteTracking(config):
		var firstErr error
		status := "REPEATING"
		progress := 0
		if err := SaveAniListAnimeListEntry(token, anime.AnilistId, &status, &progress, nil, nil, &startedAt, &completedAt); err != nil {
			firstErr = err
		}
		malID := anime.MalId
		if malID == 0 {
			var err error
			malID, err = resolveMyAnimeListID(anime.AnilistId)
			if err != nil {
				if firstErr != nil {
					return firstErr
				}
				return err
			}
		}
		if err := updateMyAnimeListListStatus(config, malID, map[string]string{
			"status":               "watching",
			"is_rewatching":        "true",
			"num_watched_episodes": "0",
			"start_date":           formatMyAnimeListDate(startedAt),
			"finish_date":          "",
		}); err != nil && firstErr == nil {
			firstErr = err
		}
		return firstErr
	case UsesAniListTracking(config):
		status := "REPEATING"
		progress := 0
		return SaveAniListAnimeListEntry(token, anime.AnilistId, &status, &progress, nil, nil, &startedAt, &completedAt)
	case UsesMyAnimeListTracking(config):
		malID := anime.MalId
		if malID == 0 {
			var err error
			malID, err = resolveMyAnimeListID(anime.AnilistId)
			if err != nil {
				return err
			}
		}
		return updateMyAnimeListListStatus(config, malID, map[string]string{
			"status":               "watching",
			"is_rewatching":        "true",
			"num_watched_episodes": "0",
			"start_date":           formatMyAnimeListDate(startedAt),
			"finish_date":          "",
		})
	default:
		return nil
	}
}

func AddAnimeToList(animeID int, status string, token string) error {
	config := GetGlobalConfig()
	switch {
	case !UsesRemoteTracking(config):
		return nil
	case UsesDualRemoteTracking(config):
		var firstErr error
		if err := AddAniListAnimeToList(animeID, status, token); err != nil {
			firstErr = err
		}
		malID, err := resolveMyAnimeListID(animeID)
		if err != nil {
			if firstErr != nil {
				return firstErr
			}
			return err
		}
		if err := addMyAnimeListAnimeToList(config, malID, status); err != nil && firstErr == nil {
			firstErr = err
		}
		return firstErr
	case UsesAniListTracking(config):
		return AddAniListAnimeToList(animeID, status, token)
	case UsesMyAnimeListTracking(config):
		malID, err := resolveMyAnimeListID(animeID)
		if err != nil {
			return err
		}
		return addMyAnimeListAnimeToList(config, malID, status)
	default:
		return nil
	}
}

func maybeImportAniListToMyAnimeList(config *CurdConfig, user *User) error {
	if config == nil || user == nil || !UsesMyAnimeListTracking(config) || config.MyAnimeListImported || hasAnyEntries(user.AnimeList) {
		return nil
	}

	legacyTokenPath := filepath.Join(os.ExpandEnv(config.StoragePath), "anilist_token.json")
	if _, err := os.Stat(legacyTokenPath); err != nil {
		config.MyAnimeListImported = true
		return persistTrackingConfig(config)
	}

	options := []SelectionOption{
		{Key: "yes", Label: "Import your existing AniList progress into MyAnimeList"},
		{Key: "no", Label: "Start with MyAnimeList as-is"},
	}

	CurdOut("AniList tracking data was found.")
	selected, err := DynamicSelect(options)
	if err != nil {
		return err
	}

	if selected.Key == "yes" {
		if err := ImportAniListTrackingToMyAnimeList(config); err != nil {
			return err
		}
		if err := RefreshMyAnimeListUserAnimeList(config, user); err != nil {
			return err
		}
	}

	config.MyAnimeListImported = true
	return persistTrackingConfig(config)
}

func ImportAniListTrackingToMyAnimeList(config *CurdConfig) error {
	aniListTokenPath := filepath.Join(os.ExpandEnv(config.StoragePath), "anilist_token.json")
	aniListToken, err := GetTokenFromFile(aniListTokenPath)
	if err != nil {
		return err
	}

	userID, _, err := GetAnilistUserID(aniListToken)
	if err != nil {
		return err
	}

	userData, err := GetUserData(aniListToken, userID)
	if err != nil {
		return err
	}

	sourceList := ParseAnimeList(userData)
	for _, entry := range getEntriesByCategory(sourceList, "ALL") {
		if entry.Media.MalID == 0 {
			continue
		}

		payload := map[string]string{
			"status":               aniListStatusToMyAnimeListStatus(entry.Status),
			"num_watched_episodes": strconv.Itoa(entry.Progress),
		}
		if entry.Score > 0 {
			payload["score"] = strconv.Itoa(int(entry.Score + 0.5))
		}
		if entry.Repeat > 0 {
			payload["num_times_rewatched"] = strconv.Itoa(entry.Repeat)
		}
		if entry.Status == "REPEATING" {
			payload["is_rewatching"] = "true"
			payload["status"] = "watching"
		}

		if err := updateMyAnimeListListStatus(config, entry.Media.MalID, payload); err != nil {
			return err
		}
	}

	return nil
}

func trackingSummary(config *CurdConfig) string {
	parts := make([]string, 0, 2)
	if UsesLocalTracking(config) {
		parts = append(parts, "local")
	}
	remote := normalizeRemoteTracker(config.TrackingRemote)
	if remote != TrackingRemoteNone {
		parts = append(parts, remote)
	}
	return strings.Join(parts, "+")
}

func RemoteTrackingDisplayName(config *CurdConfig) string {
	switch normalizeRemoteTracker(config.TrackingRemote) {
	case TrackingRemoteBoth:
		return "AniList + MyAnimeList"
	case TrackingRemoteMyAnimeList:
		return "MyAnimeList"
	case TrackingRemoteAniList:
		return "AniList"
	default:
		return "remote tracking"
	}
}

func fetchAniListAnimeListFromToken(token string) (AnimeList, error) {
	userID, _, err := GetAnilistUserID(token)
	if err != nil {
		return AnimeList{}, err
	}
	userData, err := GetUserData(token, userID)
	if err != nil {
		return AnimeList{}, err
	}
	return ParseAnimeList(userData), nil
}

func saveAniListTrackedEntry(token string, entry Entry) error {
	status := entry.Status
	progress := entry.Progress
	repeat := entry.Repeat
	score := entry.Score
	return SaveAniListAnimeListEntry(token, entry.Media.ID, &status, &progress, &repeat, &score, &entry.StartedAt, &entry.CompletedAt)
}

func saveMyAnimeListTrackedEntry(config *CurdConfig, entry Entry) error {
	malID := entry.Media.MalID
	if malID == 0 {
		var err error
		malID, err = resolveMyAnimeListID(entry.Media.ID)
		if err != nil {
			return err
		}
	}

	payload := map[string]string{
		"status":               aniListStatusToMyAnimeListStatus(entry.Status),
		"num_watched_episodes": strconv.Itoa(entry.Progress),
		"score":                strconv.Itoa(int(entry.Score + 0.5)),
		"start_date":           formatMyAnimeListDate(entry.StartedAt),
		"finish_date":          formatMyAnimeListDate(entry.CompletedAt),
		"is_rewatching":        "false",
	}
	if entry.Status == "REPEATING" {
		payload["status"] = "watching"
		payload["is_rewatching"] = "true"
	}
	if entry.Status == "COMPLETED" && entry.Repeat > 0 {
		delete(payload, "is_rewatching")
		if err := updateMyAnimeListListStatus(config, malID, payload); err != nil {
			return err
		}
		return updateMyAnimeListListStatus(config, malID, map[string]string{
			"num_times_rewatched": strconv.Itoa(entry.Repeat),
		})
	}
	payload["num_times_rewatched"] = strconv.Itoa(entry.Repeat)
	return updateMyAnimeListListStatus(config, malID, payload)
}

func upsertAnimeListToAniList(token string, list AnimeList) error {
	for index, entry := range getEntriesByCategory(list, "ALL") {
		if index > 0 {
			time.Sleep(350 * time.Millisecond)
		}
		if err := saveAniListTrackedEntry(token, entry); err != nil {
			return err
		}
	}
	return nil
}

func upsertAnimeListToMyAnimeList(config *CurdConfig, list AnimeList) error {
	for _, entry := range getEntriesByCategory(list, "ALL") {
		if err := saveMyAnimeListTrackedEntry(config, entry); err != nil {
			return err
		}
	}
	return nil
}

func wipeAniListRemote(config *CurdConfig) error {
	token, err := GetTokenFromFile(filepath.Join(os.ExpandEnv(config.StoragePath), "anilist_token.json"))
	if err != nil {
		return err
	}
	list, err := fetchAniListAnimeListFromToken(token)
	if err != nil {
		return err
	}
	for index, entry := range getEntriesByCategory(list, "ALL") {
		if entry.ListID == 0 {
			continue
		}
		if index > 0 {
			time.Sleep(350 * time.Millisecond)
		}
		if err := DeleteAniListListEntry(token, entry.ListID); err != nil {
			return err
		}
	}
	return nil
}

func wipeMyAnimeListRemote(config *CurdConfig) error {
	list, err := FetchLatestMyAnimeList(config, &User{})
	if err != nil {
		return err
	}
	for _, entry := range getEntriesByCategory(list, "ALL") {
		if entry.Media.MalID == 0 {
			continue
		}
		if err := deleteMyAnimeListEntry(config, entry.Media.MalID); err != nil {
			return err
		}
	}
	return nil
}

func ReplaceMyAnimeListWithAniList(config *CurdConfig) error {
	token, err := GetTokenFromFile(filepath.Join(os.ExpandEnv(config.StoragePath), "anilist_token.json"))
	if err != nil {
		return err
	}
	sourceList, err := fetchAniListAnimeListFromToken(token)
	if err != nil {
		return err
	}
	if err := wipeMyAnimeListRemote(config); err != nil {
		return err
	}
	return upsertAnimeListToMyAnimeList(config, sourceList)
}

func ReplaceAniListWithMyAnimeList(config *CurdConfig) error {
	token, err := GetTokenFromFile(filepath.Join(os.ExpandEnv(config.StoragePath), "anilist_token.json"))
	if err != nil {
		return err
	}
	sourceList, err := FetchLatestMyAnimeList(config, &User{})
	if err != nil {
		return err
	}
	if err := wipeAniListRemote(config); err != nil {
		return err
	}
	return upsertAnimeListToAniList(token, sourceList)
}

func MergeRemoteLists(config *CurdConfig) error {
	token, err := GetTokenFromFile(filepath.Join(os.ExpandEnv(config.StoragePath), "anilist_token.json"))
	if err != nil {
		return err
	}
	aniListUser := &User{}
	aniList, err := fetchAniListAnimeListFromToken(token)
	if err != nil {
		return err
	}
	aniListUser.AnimeList = aniList

	myAnimeListUser := &User{}
	myAnimeList, err := FetchLatestMyAnimeList(config, myAnimeListUser)
	if err != nil {
		return err
	}
	myAnimeListUser.AnimeList = myAnimeList

	_, err = syncDualRemoteTrackers(config, token, aniListUser, myAnimeListUser)
	return err
}

func trackersCanCrossSync(config *CurdConfig) bool {
	if _, err := GetTokenFromFile(filepath.Join(os.ExpandEnv(config.StoragePath), "anilist_token.json")); err != nil {
		return false
	}
	if _, err := GetMyAnimeListAccessToken(config); err != nil {
		return false
	}
	return true
}

func ChangeTracker(config *CurdConfig, user *User) {
	options := []SelectionOption{
		{Key: TrackingRemoteNone, Label: "local"},
		{Key: TrackingRemoteAniList, Label: "anilist"},
		{Key: TrackingRemoteMyAnimeList, Label: "myanimelist"},
		{Key: TrackingRemoteBoth, Label: "anilist + myanimelist"},
	}

	selected, err := DynamicSelect(options)
	if err != nil || selected.Key == "-1" || selected.Key == "-2" {
		return
	}

	config.TrackingLocal = true
	config.TrackingRemote = selected.Key
	config.TrackingConfigured = true
	normalizeTrackingConfig(config)
	if err := persistTrackingConfig(config); err != nil {
		ExitCurd(err)
	}
	if err := EnsureConfiguredTrackersReady(config, user); err != nil {
		ExitCurd(err)
	}

	if trackersCanCrossSync(config) {
		syncOptions := []SelectionOption{
			{Key: "skip", Label: "Keep current lists"},
			{Key: "merge", Label: "Merge both lists and update both"},
			{Key: "anilist_to_mal", Label: "Replace MyAnimeList with AniList"},
			{Key: "mal_to_anilist", Label: "Replace AniList with MyAnimeList"},
		}
		CurdOut("Optional tracker sync action:")
		syncSelection, syncErr := DynamicSelect(syncOptions)
		if syncErr == nil {
			switch syncSelection.Key {
			case "merge":
				err = MergeRemoteLists(config)
			case "anilist_to_mal":
				err = ReplaceMyAnimeListWithAniList(config)
			case "mal_to_anilist":
				err = ReplaceAniListWithMyAnimeList(config)
			}
			if err != nil {
				ExitCurd(err)
			}
		}
	}

	if err := RefreshUserAnimeList(config, user); err != nil {
		ExitCurd(err)
	}
	CurdOut(fmt.Sprintf("Tracker changed to %s.", selected.Label))
}
