package internal

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wraient/curd/internal/providers/animepahe"
	"github.com/wraient/curd/internal/providers/anipub"
	"github.com/wraient/curd/internal/providers/senshi"
)

type ProviderMappingOutcome int

const (
	ProviderMappingOK ProviderMappingOutcome = iota
	ProviderMappingBack
	ProviderMappingQuit
)

// providerMappingSearchState carries the in-progress mapping query. Searches
// always cover the full configured stack; there is no per-provider stepping.
type providerMappingSearchState struct {
	query string
}

// mappingValidationCache memoizes live episode lists per provider+mode for
// the session so saved-mapping validation doesn't re-fetch every playback.
var (
	mappingValidationCacheMu sync.Mutex
	mappingValidationCache   = map[string][]string{}
)

func resetMappingValidationCacheForTest(t interface{ Helper() }) {
	t.Helper()
	mappingValidationCacheMu.Lock()
	defer mappingValidationCacheMu.Unlock()
	mappingValidationCache = map[string][]string{}
}

// validateSavedMapping checks a stored provider mapping against the live
// provider in the requested mode only (never the alternate mode). A lookup
// error is transient: the mapping is kept and playback surfaces the real
// error. Only a definitive miss (list loads but lacks the episode) rejects
// the mapping and forces a re-search.
func validateSavedMapping(config *CurdConfig, anime *Anime, epNo int) (episodes []string, usable bool) {
	providerName, providerID := AnimeProviderID(anime)
	if providerID == "" || !ProviderEnabled(providerName) || !ProviderStackContains(config, providerName) {
		return nil, false
	}
	mode := normalizeTranslationType(config.SubOrDub)
	cacheKey := providerName + "::" + providerID + "::" + mode
	mappingValidationCacheMu.Lock()
	cached, ok := mappingValidationCache[cacheKey]
	mappingValidationCacheMu.Unlock()
	if ok {
		episodes = cached
	} else {
		list, err := EpisodesList(QualifyProviderID(providerName, providerID), mode)
		if err != nil {
			return nil, true
		}
		episodes = list
		mappingValidationCacheMu.Lock()
		mappingValidationCache[cacheKey] = list
		mappingValidationCacheMu.Unlock()
	}
	if epNo > 0 && !episodeNumberInList(episodes, epNo) {
		return episodes, false
	}
	return episodes, true
}

// episodeNumberInList reports membership by numeric episode identity.
// Non-numeric entries (specials, decimals) never match a plain episode.
func episodeNumberInList(episodes []string, epNo int) bool {
	if epNo <= 0 {
		return false
	}
	for _, entry := range episodes {
		if n, err := strconv.Atoi(strings.TrimSpace(entry)); err == nil && n == epNo {
			return true
		}
	}
	return false
}

// maxEpisodeCountInResults returns the highest episode count advertised by
// the given search results, or 0 when none carry a count.
func maxEpisodeCountInResults(results []SelectionOption) int {
	maxCount := 0
	for _, option := range results {
		if count, ok := episodeCountFromSelectionOption(option); ok && count > maxCount {
			maxCount = count
		}
	}
	return maxCount
}

// truncateSearchQuery bounds user-typed queries so garbage input cannot fan
// out into oversized provider requests.
func truncateSearchQuery(query string) string {
	query = strings.TrimSpace(query)
	if len(query) > 200 {
		return query[:200]
	}
	return query
}

// stackedSearchWithExpansion runs one stacked search in the requested mode.
// When the primary query returns nothing, it tries the alternate tracker
// title (Romaji<->English) once and merges both result sets, deduplicated by
// qualified key then by normalized title within a provider.
func stackedSearchWithExpansion(config *CurdConfig, anime *Anime, primaryQuery, mode string) ([]SelectionOption, []ProviderSearchError, error) {
	results, failures, err := searchAnimeStack(config, primaryQuery, mode)
	if len(results) > 0 {
		return results, failures, err
	}
	alternate := alternateSearchTitle(anime, primaryQuery)
	if alternate == "" {
		return results, failures, err
	}
	altResults, altFailures, altErr := searchAnimeStack(config, alternate, mode)
	merged, mergedFailures := mergeDedupeSearchResults(results, altResults, failures, altFailures)
	if len(merged) > 0 {
		return merged, mergedFailures, nil
	}
	if err == nil {
		return merged, mergedFailures, altErr
	}
	if altErr != nil {
		return merged, mergedFailures, fmt.Errorf("%v; alternate %q: %v", err, alternate, altErr)
	}
	return merged, mergedFailures, err
}

func alternateSearchTitle(anime *Anime, primaryQuery string) string {
	if anime == nil {
		return ""
	}
	primary := normalizeSearchTitle(primaryQuery)
	romaji := normalizeSearchTitle(anime.Title.Romaji)
	english := normalizeSearchTitle(anime.Title.English)
	switch {
	case primary != "" && primary == romaji && english != "" && english != romaji:
		return strings.TrimSpace(anime.Title.English)
	case primary != "" && primary == english && romaji != "" && romaji != english:
		return strings.TrimSpace(anime.Title.Romaji)
	default:
		return ""
	}
}

func mergeDedupeSearchResults(primary, alternate []SelectionOption, primaryFailures, alternateFailures []ProviderSearchError) ([]SelectionOption, []ProviderSearchError) {
	merged := make([]SelectionOption, 0, len(primary)+len(alternate))
	seen := make(map[string]struct{})
	add := func(option SelectionOption) {
		if _, exists := seen[option.Key]; exists {
			return
		}
		seen[option.Key] = struct{}{}
		merged = append(merged, option)
	}
	for _, option := range primary {
		add(option)
	}
	for _, option := range alternate {
		add(option)
	}
	failures := append(append([]ProviderSearchError(nil), primaryFailures...), alternateFailures...)
	return merged, failures
}

// formatProviderFailureNotes renders per-provider errors for recovery
// messages: one short line each, overall capped for rofi -mesg.
func formatProviderFailureNotes(failures []ProviderSearchError) string {
	if len(failures) == 0 {
		return ""
	}
	lines := make([]string, 0, len(failures))
	total := 0
	for _, failure := range failures {
		note := strings.TrimSpace(failure.Err)
		if len(note) > 120 {
			note = note[:117] + "..."
		}
		line := failure.Provider + ": " + note
		if total+len(line) > 500 {
			break
		}
		total += len(line)
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n")
}

// maxAnimepaheDetailFetches bounds the exact-meta-tag verification fetches per
// mapping attempt; animepaheDetailFetchTimeout bounds each fetch. Beyond the
// budget, remaining candidates go to the manual picker.
const (
	maxAnimepaheDetailFetches      = 3
	animepaheDetailFetchTimeout    = 10 * time.Second
	mappingCustomQueryMaxBlanks    = 3
	mappingRecoveryMessageMaxChars = 500
)

func confirmProviderMatch(option SelectionOption, reason string) bool {
	label := option.Label
	if label == "" {
		label = option.Title
	}
	if label == "" {
		label = option.Key
	}

	CurdOut(fmt.Sprintf("Provider match found by %s: %s", reason, label))
	selected, err := promptSelect([]SelectionOption{
		{Key: "use", Label: "Use this match"},
		{Key: "manual", Label: "Select manually"},
	})
	if err != nil {
		Log(fmt.Sprintf("Error confirming provider match: %v", err))
		return false
	}
	return selected.Key == "use"
}

// matchJikanThumbnail auto-matches when a result thumbnail is byte-identical
// to a Jikan (MyAnimeList) picture for the anime's MAL ID. The Jikan picture
// list is fetched lazily and only when a MAL thumbnail candidate exists.
func matchJikanThumbnail(anime *Anime, animeList []SelectionOption) bool {
	malRegex := regexp.MustCompile(`myanimelist\.net/images/anime/[^/]+/([^/]+\.jpg)`)
	var jikanUrls []string
	fetchedJikan := false

	for _, option := range animeList {
		if !strings.Contains(option.Thumbnail, "myanimelist.net") {
			continue
		}
		matches := malRegex.FindStringSubmatch(option.Thumbnail)
		if len(matches) < 2 {
			continue
		}
		fileName := matches[1]

		if !fetchedJikan {
			if anime.MalId == 0 {
				anime.MalId, _ = GetAnimeMalID(anime.AnilistId)
			}
			if anime.MalId != 0 {
				urls, err := FetchJikanPictures(anime.MalId)
				if err != nil {
					Log(fmt.Sprintf("Failed to fetch Jikan pictures: %v", err))
				} else {
					jikanUrls = urls
				}
			}
			fetchedJikan = true
		}

		for _, url := range jikanUrls {
			if strings.HasSuffix(url, "/"+fileName) || strings.Contains(url, fileName) {
				anime.ProviderId = option.Key
				Log(fmt.Sprintf("Found MyAnimeList Thumbnail match (%s)! Setting ProviderId to: %s", fileName, anime.ProviderId))
				return true
			}
		}
	}
	return false
}

func autoMatchProviderListing(config *CurdConfig, anime *Anime, animeList []SelectionOption, userQuery string, anilistEntry *Entry) bool {
	// Silent auto-match requires cross-system identity: a MAL ID, an exact
	// image ID, or an exact animepahe meta tag. Title similarity only ranks
	// the picker list and never decides here.
	if anime.MalId == 0 {
		anime.MalId, _ = GetAnimeMalID(anime.AnilistId)
	}

	for i, option := range animeList {
		Log(fmt.Sprintf("Checking option %d: Key='%s', Label='%s', Thumbnail='%s'", i, option.Key, option.Label, option.Thumbnail))

		rawID := option.Key
		if _, parsed, ok := ParseProviderQualifiedID(option.Key); ok {
			rawID = parsed
		}
		if anime.MalId != 0 {
			if rawID == strconv.Itoa(anime.MalId) {
				anime.ProviderId = option.Key
				Log(fmt.Sprintf("Found MAL ID provider key match! Setting ProviderId to: %s", anime.ProviderId))
				return true
			}
			if malID := malIDFromProviderExtraData(option.ExtraData); malID != 0 && malID == anime.MalId {
				anime.ProviderId = option.Key
				Log(fmt.Sprintf("Found provider MAL ID extra-data match! Setting ProviderId to: %s", anime.ProviderId))
				return true
			}
		}

		if anime.AnilistId != 0 && thumbnailExactAnilistMatch(option.Thumbnail, anime.AnilistId) {
			anime.ProviderId = option.Key
			Log(fmt.Sprintf("Found Anilist Thumbnail match! Setting ProviderId to: %s", anime.ProviderId))
			return true
		}

		if anime.MalId != 0 && senshiPosterMatchesMalID(option.Thumbnail, anime.MalId) {
			anime.ProviderId = option.Key
			Log(fmt.Sprintf("Found Senshi MAL poster match! Setting ProviderId to: %s", anime.ProviderId))
			return true
		}
	}

	if matchJikanThumbnail(anime, animeList) {
		return true
	}

	if ProviderEnabled("animepahe") && ProviderStackContains(config, "animepahe") {
		Log("Attempting deep metadata matching and exact AniList meta tag check for Animepahe...")

		targetAnilistID := strconv.Itoa(anime.AnilistId)
		var bestMatch *SelectionOption
		var highestScore int
		detailFetches := 0

		if anime.MalId == 0 {
			anime.MalId, _ = GetAnimeMalID(anime.AnilistId)
		}

		var malData *JikanAnimeData
		if anime.MalId != 0 {
			malData, _ = FetchJikanAnimeData(anime.MalId)
		}

		for i := range animeList {
			opt := &animeList[i]
			optionProviderName, rawProviderID, ok := ParseProviderQualifiedID(opt.Key)
			if ok {
				if optionProviderName != "animepahe" {
					continue
				}
			} else {
				rawProviderID = opt.Key
				if configuredProviderNames(config)[0] != "animepahe" {
					continue
				}
			}
			score := 0

			if malData != nil && opt.ExtraData != nil {
				if paheData, ok := opt.ExtraData.(animepahe.SearchItem); ok {
					paheTitleLower := strings.ToLower(paheData.Title)
					malTitleLower := strings.ToLower(malData.Title)
					malTitleEngLower := strings.ToLower(malData.TitleEnglish)
					malTitleJapLower := strings.ToLower(malData.TitleJapanese)

					if paheTitleLower == malTitleLower || (malTitleEngLower != "" && paheTitleLower == malTitleEngLower) || (malTitleJapLower != "" && paheTitleLower == malTitleJapLower) {
						score += 5
					} else if strings.Contains(paheTitleLower, malTitleLower) || strings.Contains(malTitleLower, paheTitleLower) ||
						(malTitleEngLower != "" && (strings.Contains(paheTitleLower, malTitleEngLower) || strings.Contains(malTitleEngLower, paheTitleLower))) {
						score += 2
					}

					if paheData.Year > 0 && malData.Year > 0 && paheData.Year == malData.Year {
						score += 3
					}
					if paheData.Season != "" && malData.Season != "" && strings.EqualFold(paheData.Season, malData.Season) {
						score += 2
					}
					if paheData.Type != "" && malData.Type != "" && strings.EqualFold(paheData.Type, malData.Type) {
						score += 2
					}
					if paheData.Episodes > 0 && malData.Episodes > 0 && paheData.Episodes == malData.Episodes {
						score += 2
					} else if (paheData.Episodes == 0 && malData.Status == "Currently Airing") || (malData.Episodes == 0 && paheData.Status == "Currently Airing") {
						score += 2
					}
					if strings.EqualFold(paheData.Status, malData.Status) {
						score += 1
					}
				}
			} else if strings.Contains(strings.ToLower(opt.Title), strings.ToLower(string(userQuery))) {
				score += 2
			}

			if detailFetches >= maxAnimepaheDetailFetches {
				Log("Animepahe detail-fetch budget exhausted; remaining candidates go to manual pick.")
				break
			}
			if score >= 2 || len(animeList) <= 3 {
				animeRef := animepahe.ParseProviderID(rawProviderID)
				if animeRef.Session == "" {
					continue
				}
				animeUrl := fmt.Sprintf("https://animepahe.pw/anime/%s", animeRef.Session)
				Log(fmt.Sprintf("Fetching %s to check exact AniList meta tag...", animeUrl))
				req, err := animepahe.NewPageRequest(animeUrl)
				if err != nil {
					Log(fmt.Sprintf("Failed to build animepahe detail request %s: %v", animeUrl, err))
					continue
				}

				fetchCtx, cancel := context.WithTimeout(context.Background(), animepaheDetailFetchTimeout)
				resp, err := sharedHTTPClient.Do(req.WithContext(fetchCtx))
				cancel()
				detailFetches++
				if err != nil {
					Log(fmt.Sprintf("Failed to fetch animepahe detail page %s: %v", animeUrl, err))
					continue
				}
				body, readErr := io.ReadAll(resp.Body)
				resp.Body.Close()
				if readErr != nil {
					Log(fmt.Sprintf("Failed to read animepahe detail page %s: %v", animeUrl, readErr))
					continue
				}
				if resp.StatusCode == http.StatusOK {
					bodyStr := string(body)

					metaTag := fmt.Sprintf(`<meta name="anilist" content="%s">`, targetAnilistID)
					if strings.Contains(bodyStr, metaTag) {
						Log(fmt.Sprintf("FOUND EXACT ANILIST META TAG MATCH in %s!", opt.Title))
						bestMatch = opt
						highestScore = 100
						break
					}

					malMetaTag := fmt.Sprintf(`<meta name="mal" content="%d">`, anime.MalId)
					if anime.MalId != 0 && strings.Contains(bodyStr, malMetaTag) {
						Log(fmt.Sprintf("FOUND EXACT MAL META TAG MATCH in %s!", opt.Title))
						bestMatch = opt
						highestScore = 100
						break
					}
				}
			}

			if score > highestScore {
				highestScore = score
				bestMatch = opt
			}
		}

		if bestMatch != nil && highestScore == 100 {
			anime.ProviderId = bestMatch.Key
			Log(fmt.Sprintf("Found EXACT meta tag match! Score: %d. Setting ProviderId to: %s", highestScore, anime.ProviderId))
			return true
		}
		if bestMatch != nil {
			Log(fmt.Sprintf("Highest match score was %d (needed 100 for Animepahe exact meta tag match). Not selecting automatically to prevent false positives.", highestScore))
			if confirmProviderMatch(*bestMatch, "Animepahe metadata") {
				anime.ProviderId = bestMatch.Key
				Log(fmt.Sprintf("User confirmed Animepahe match. Setting ProviderId to: %s", anime.ProviderId))
				return true
			}
		}
	}

	if anilistEntry != nil {
		targetLabel := fmt.Sprintf("%v (%d episodes)", userQuery, anilistEntry.Media.Episodes)
		for _, option := range animeList {
			if fmt.Sprintf("%s (%d episodes)", option.Title, anilistEntry.Media.Episodes) == targetLabel {
				if confirmProviderMatch(option, "title and episode count") {
					anime.ProviderId = option.Key
					Log(fmt.Sprintf("User confirmed exact text match. Setting ProviderId to: %s", anime.ProviderId))
					return true
				}
				break
			}
		}
		Log(fmt.Sprintf("No exact match found for label '%s'. Will require manual selection.", targetLabel))
	}

	return anime.ProviderId != ""
}

// promptMappingRecovery shows the single mapping recovery menu (M1): rename,
// switch audio language, or go back — plus an episode row when results exist
// but the wanted episode is missing. It returns the chosen action and, for
// the episode action, the newly entered episode number (0 otherwise).
func promptMappingRecovery(config *CurdConfig, state *providerMappingSearchState, mode string, failures []ProviderSearchError, results []SelectionOption, epNo int, reason string) (action string, episode int) {
	alternate := alternateTranslationType(mode)
	options := []SelectionOption{
		{Key: "custom", Label: "Search with a different name"},
		{Key: "audio", Label: fmt.Sprintf("Try %s instead (search all providers in %s)", titleAudioMode(alternate), alternate)},
	}
	if len(results) > 0 && epNo > 0 {
		if maxEp := maxEpisodeCountInResults(results); maxEp > 0 {
			options = append(options, SelectionOption{Key: "episode", Label: fmt.Sprintf("Change episode (provider lists up to %d)", maxEp)})
		} else {
			options = append(options, SelectionOption{Key: "episode", Label: "Change episode number (if this one is wrong)"})
		}
	}
	options = append(options, SelectionOption{Key: "back", Label: "Back to menu"})

	message := reason
	if message == "" {
		message = fmt.Sprintf("No %s results for %q on %s.", mode, state.query, strings.Join(configuredProviderNames(config), ", "))
	}
	if notes := formatProviderFailureNotes(failures); notes != "" {
		message += "\n" + notes
	}
	if len(message) > mappingRecoveryMessageMaxChars {
		message = message[:mappingRecoveryMessageMaxChars-3] + "..."
	}

	selected, err := selectWithOptionalMessage(config, options, "Provider search", message)
	if err != nil {
		return "back", 0
	}
	selected = NormalizeSelectionKey(selected)
	if SelectionMeansQuit(selected) {
		return "quit", 0
	}
	if SelectionMeansBack(selected) || selected.Key == "" || selected.Key == "back" {
		return "back", 0
	}
	switch selected.Key {
	case "custom", "audio":
		return selected.Key, 0
	case "episode":
		episodeNumber, promptErr := promptPositiveEpisodeNumber(config, "Change episode number (if this one is wrong)")
		if promptErr != nil {
			Log("Invalid episode input: " + promptErr.Error())
			CurdOut("Invalid episode number")
			return "episode", 0
		}
		return "episode", episodeNumber
	default:
		return "back", 0
	}
}

// selectWithOptionalMessage uses Rofi -mesg when Rofi is on (no notify spam), else prints + ordered select.
func selectWithOptionalMessage(config *CurdConfig, options []SelectionOption, prompt, message string) (SelectionOption, error) {
	if config != nil && config.RofiSelection {
		return RofiSelectWithMessage(options, false, prompt, message)
	}
	if strings.TrimSpace(message) != "" {
		fmt.Println(message)
	}
	return promptSelectOrdered(options)
}

func activeProviderName(config *CurdConfig) string {
	names := configuredProviderNames(config)
	if len(names) > 0 {
		return names[0]
	}
	return firstEnabledProviderName()
}

func providerNameFromSelectionLabel(label string) string {
	label = strings.TrimSpace(label)
	if idx := strings.LastIndex(label, " ["); idx > 0 && strings.HasSuffix(label, "]") {
		name := strings.TrimSpace(label[idx+2 : len(label)-1])
		if normalized := normalizeProviderName(name); normalized != "" {
			return normalized
		}
	}
	return ""
}

func providerNameFromSelection(config *CurdConfig, selected SelectionOption) string {
	if providerName, _, ok := ParseProviderQualifiedID(selected.Key); ok {
		return providerName
	}
	if name := providerNameFromSelectionLabel(selected.Label); name != "" {
		return name
	}
	return activeProviderName(config)
}

func applyMatchedProviderMapping(config *CurdConfig, state *providerMappingSearchState, anime *Anime) {
	if providerName, rawProviderID, ok := ParseProviderQualifiedID(anime.ProviderId); ok {
		anime.ProviderName = providerName
		anime.ProviderId = rawProviderID
		return
	}
	anime.ProviderName = activeProviderName(config)
}

func applySelectedProviderMapping(config *CurdConfig, state *providerMappingSearchState, anime *Anime, selected SelectionOption) {
	anime.ProviderId = selected.Key
	if providerName, rawProviderID, ok := ParseProviderQualifiedID(anime.ProviderId); ok {
		anime.ProviderName = providerName
		anime.ProviderId = rawProviderID
		return
	}
	anime.ProviderName = providerNameFromSelection(config, selected)
}

// promptCustomProviderSearchQuery re-prompts for a provider search name.
// Blank input re-prompts instead of silently reusing the current query; after
// repeated blanks it backs out. Returns (query, back, quit, err).
func promptCustomProviderSearchQuery(config *CurdConfig, currentQuery, hint string) (string, bool, bool, error) {
	for attempts := 0; attempts < mappingCustomQueryMaxBlanks; attempts++ {
		manualQuery, err := promptText(config, hint, true)
		if err != nil {
			return "", false, false, err
		}
		if strings.TrimSpace(manualQuery) != "" {
			return truncateSearchQuery(manualQuery), false, false, nil
		}
		CurdOut("Please enter a name, or go back.")
	}
	return "", true, false, nil
}

func ResolveAnimeProviderMapping(config *CurdConfig, anime *Anime, query string, anilistEntry *Entry) (ProviderMappingOutcome, error) {
	return resolveAnimeProviderMapping(config, anime, query, anilistEntry, ManualProviderSearchEnabled(config))
}

// mappingEpisodeForRecovery returns the episode number the mapping recovery
// menu should reason about, or 0 when no episode is decided yet.
func mappingEpisodeForRecovery(anime *Anime) int {
	if anime == nil || anime.Ep.Number <= 0 {
		return 0
	}
	return anime.Ep.Number
}

func resolveAnimeProviderMapping(config *CurdConfig, anime *Anime, query string, anilistEntry *Entry, manualOnly bool) (ProviderMappingOutcome, error) {
	state := &providerMappingSearchState{query: truncateSearchQuery(query)}
	// The audio mode is loop-local: an M1 language flip re-searches in the
	// other mode for this resolve only and never writes config.SubOrDub.
	mode := normalizeTranslationType(config.SubOrDub)

	for {
		Log(fmt.Sprintf("Searching for anime with query: %s, mode: %s", state.query, mode))

		animeList, failures, err := stackedSearchWithExpansion(config, anime, state.query, mode)
		if err != nil || len(animeList) == 0 {
			reason := ""
			if err != nil {
				Log(fmt.Sprintf("Provider search failed: %v", err))
				reason = fmt.Sprintf("Provider search failed for '%s': %v", state.query, err)
			}
			action, episode := promptMappingRecovery(config, state, mode, failures, animeList, mappingEpisodeForRecovery(anime), reason)
			outcome, cont, actionErr := handleMappingRecoveryAction(config, state, anime, action, episode, &mode)
			if actionErr != nil {
				return ProviderMappingQuit, actionErr
			}
			if !cont {
				return outcome, nil
			}
			continue
		}

		anime.ProviderId = ""
		if !manualOnly && autoMatchProviderListing(config, anime, animeList, state.query, anilistEntry) {
			applyMatchedProviderMapping(config, state, anime)
			return ProviderMappingOK, nil
		}

		// One merged, ranked picker list — no intermediate "pick from
		// results" step. Ranking never decides; the user picks.
		ranked := rankProviderSearchOptions(animeList, anime, state.query, configuredProviderNames(config))
		hint := TrackerEpisodeHint(config, anilistEntry)
		if manualOnly {
			hint = ManualProviderSearchHint(config, anilistEntry, state.query, mode)
		}
		selected, selectErr := promptProviderSearchSelectionWithHint(config, ranked, anilistEntry, hint)
		if selectErr != nil {
			Log(fmt.Sprintf("Failed to select anime: %v", selectErr))
		} else {
			switch selected.Key {
			case "-1":
				// Dismissed picker routes to recovery.
			case "-2":
				return ProviderMappingBack, nil
			default:
				applySelectedProviderMapping(config, state, anime, selected)
				return ProviderMappingOK, nil
			}
		}

		action, episode := promptMappingRecovery(config, state, mode, failures, animeList, mappingEpisodeForRecovery(anime), "")
		outcome, cont, actionErr := handleMappingRecoveryAction(config, state, anime, action, episode, &mode)
		if actionErr != nil {
			return ProviderMappingQuit, actionErr
		}
		if !cont {
			return outcome, nil
		}
	}
}

// handleMappingRecoveryAction applies one M1 recovery action. Custom renames
// the query, audio flips the loop-local search mode, episode retargets the
// episode number. The mapping itself always stores provider only.
func handleMappingRecoveryAction(config *CurdConfig, state *providerMappingSearchState, anime *Anime, action string, episode int, mode *string) (ProviderMappingOutcome, bool, error) {
	switch action {
	case "custom":
		hint := fmt.Sprintf("Enter a search name for configured providers (current: '%s').", state.query)
		newQuery, back, quit, err := promptCustomProviderSearchQuery(config, state.query, hint)
		if err != nil {
			return ProviderMappingQuit, false, err
		}
		if quit {
			return ProviderMappingQuit, false, nil
		}
		if back {
			return ProviderMappingBack, false, nil
		}
		state.query = newQuery
		return ProviderMappingOK, true, nil
	case "audio":
		*mode = alternateTranslationType(*mode)
		return ProviderMappingOK, true, nil
	case "episode":
		if episode > 0 && anime != nil {
			anime.Ep.Number = episode
			anime.Ep.NextEpisode = NextEpisode{}
		}
		return ProviderMappingOK, true, nil
	case "back":
		return ProviderMappingBack, false, nil
	case "quit":
		return ProviderMappingQuit, false, nil
	default:
		return ProviderMappingBack, false, nil
	}
}

func RemapProviderAnime(userCurdConfig *CurdConfig, user *User, databaseAnimes *[]Anime) {
	if userCurdConfig == nil || user == nil {
		return
	}

	if err := RefreshUserAnimeList(userCurdConfig, user); err != nil {
		Log(fmt.Sprintf("Failed to refresh anime list before provider remap: %v", err))
	}

	if user.ListSync != nil {
		user.AnimeList = user.ListSync.Current()
	}

	options := buildCategorySelectionOptions(user.AnimeList, "ALL")
	if len(options) == 0 {
		CurdOut("No anime found in your lists.")
		return
	}

	var selected SelectionOption
	var err error
	if userCurdConfig.RofiSelection && userCurdConfig.ImagePreview {
		preview := buildCategoryPreviewOptions(user.AnimeList, "ALL")
		selected, err = DynamicSelectPreview(preview, false)
	} else {
		selected, err = DynamicSelect(options)
	}
	if err != nil {
		Log(fmt.Sprintf("Failed to select anime for provider remap: %v", err))
		return
	}
	if selected.Key == "-1" || selected.Key == "-2" {
		return
	}

	anilistEntry, err := FindAnimeByAnilistID(user.AnimeList, selected.Key)
	if err != nil {
		Log(fmt.Sprintf("Failed to resolve selected anime: %v", err))
		CurdOut("Could not find the selected anime in your lists.")
		return
	}

	historyPath := filepath.Join(os.ExpandEnv(userCurdConfig.StoragePath), "curd_history.txt")
	if databaseAnimes != nil {
		if existing := LocalFindAnime(*databaseAnimes, anilistEntry.Media.ID, ""); existing != nil {
			CurdOut(fmt.Sprintf("Current provider mapping: %s (%s)", CurrentAnimeProviderName(existing), existing.ProviderId))
		} else {
			CurdOut("No saved provider mapping in local history yet.")
		}
	}

	query := mediaDisplayTitle(anilistEntry.Media, userCurdConfig)
	anime := Anime{
		AnilistId:     anilistEntry.Media.ID,
		MalId:         anilistEntry.Media.MalID,
		Title:         anilistEntry.Media.Title,
		TotalEpisodes: anilistEntry.Media.Episodes,
		CoverImage:    anilistEntry.CoverImage,
	}

	outcome, mappingErr := resolveAnimeProviderMapping(userCurdConfig, &anime, query, anilistEntry, true)
	if mappingErr != nil {
		Log(fmt.Sprintf("Provider remap failed: %v", mappingErr))
		CurdOut("Failed to update provider mapping.")
		return
	}
	switch outcome {
	case ProviderMappingBack, ProviderMappingQuit:
		return
	case ProviderMappingOK:
		if err := persistRemappedProvider(historyPath, databaseAnimes, anilistEntry, &anime, query); err != nil {
			Log(fmt.Sprintf("Failed to save remapped provider: %v", err))
			CurdOut("Provider selected, but failed to save mapping to local history.")
			return
		}
		CurdOut(fmt.Sprintf("Updated provider mapping to %s (%s).", anime.ProviderName, anime.ProviderId))
	}
}

func persistRemappedProvider(historyPath string, databaseAnimes *[]Anime, anilistEntry *Entry, anime *Anime, animeName string) error {
	if anime == nil || anilistEntry == nil {
		return fmt.Errorf("missing anime data")
	}

	episode := 1
	playbackTime := 0
	duration := 0
	if databaseAnimes != nil {
		if existing := LocalFindAnime(*databaseAnimes, anime.AnilistId, ""); existing != nil {
			episode = existing.Ep.Number
			playbackTime = existing.Ep.Player.PlaybackTime
			duration = existing.Ep.Duration
			if savedName := GetAnimeName(*existing); savedName != "" {
				animeName = savedName
			}
		}
	}
	if episode <= 0 {
		episode = nextEpisodeFromProgress(anilistEntry.Progress)
		if episode <= 0 {
			episode = 1
		}
	}

	if err := LocalRemapAnimeProvider(historyPath, anime.AnilistId, anime.ProviderName, anime.ProviderId, animeName, episode, playbackTime, duration); err != nil {
		return err
	}
	if databaseAnimes != nil {
		*databaseAnimes = LocalGetAllAnime(historyPath)
	}
	return nil
}

func untrackedProviderSearchSelection(config *CurdConfig, query string, animeList []SelectionOption) (SelectionOption, error) {
	if ManualProviderSearchEnabled(config) {
		hint := ManualProviderSearchHint(config, nil, query, config.SubOrDub)
		return promptProviderSearchSelectionWithHint(config, animeList, nil, hint)
	}
	return promptProviderSearchSelection(config, animeList, nil)
}

func RemapAnimeProviderOnEpisodeFailure(config *CurdConfig, anime *Anime, anilistEntry *Entry) bool {
	query := GetAnimeName(*anime)
	if query == "" {
		query = anime.Title.Romaji
	}
	if query == "" {
		query = anime.Title.English
	}
	if query == "" {
		return false
	}

	CurdOut("Wrong match is common — search again and pick the right title.")
	// Remap always shows the manual picker: an automatic re-pick at playback
	// failure would risk re-selecting the same wrong show. Restore the
	// previous mapping if the user backs out.
	savedProviderID := anime.ProviderId
	savedProviderName := anime.ProviderName
	anime.ProviderId = ""
	anime.ProviderName = ""
	anime.Ep.NextEpisode = NextEpisode{}

	outcome, err := resolveAnimeProviderMapping(config, anime, query, anilistEntry, true)
	if err != nil {
		Log(fmt.Sprintf("Provider remap failed: %v", err))
		anime.ProviderId = savedProviderID
		anime.ProviderName = savedProviderName
		return false
	}
	if outcome != ProviderMappingOK {
		anime.ProviderId = savedProviderID
		anime.ProviderName = savedProviderName
		return false
	}
	return true
}

// episodeLinkFailureRecoveryOptions builds ranked recovery actions for a dead-end
// after preferred (and optional alternate) resolve already failed.
// Labels are numbered so DynamicSelect's alphabetical sort preserves priority.
func episodeLinkFailureRecoveryOptions(preferredMode string, includeAudio bool) []SelectionOption {
	preferredMode = normalizeTranslationType(preferredMode)
	alternateMode := alternateTranslationType(preferredMode)

	// No numeric indices — callers must use DynamicSelectPreserveOrder / Rofi (keeps order).
	options := []SelectionOption{
		{Key: "remap", Label: "Search for this anime again"},
	}
	if includeAudio {
		options = append(options, SelectionOption{
			Key:   "audio",
			Label: fmt.Sprintf("Try other audio (%s)", alternateMode),
		})
	}
	options = append(options, SelectionOption{
		Key:   "episode",
		Label: "Change episode number (if this one is wrong)",
	})
	return options
}

func episodeLinkFailureDiagnosis(config *CurdConfig, anime *Anime, lastErr error) string {
	title := "this anime"
	if anime != nil {
		if name := strings.TrimSpace(GetAnimeName(*anime)); name != "" {
			title = name
		}
	}

	epNo := 0
	if anime != nil {
		epNo = anime.Ep.Number
	}

	mode := "sub"
	if config != nil {
		mode = normalizeTranslationType(config.SubOrDub)
	}

	providers := configuredProviderNames(config)
	tried := "none"
	if len(providers) > 0 {
		tried = strings.Join(providers, ", ")
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Couldn't find Episode %d (%s) for %q\nTried: %s", epNo, mode, title, tried)
	if lastErr != nil {
		reason := strings.TrimSpace(lastErr.Error())
		if reason != "" {
			// Keep diagnosis readable — one short reason line.
			if len(reason) > 160 {
				reason = reason[:157] + "..."
			}
			fmt.Fprintf(&b, "\nReason: %s", reason)
		}
	}
	return b.String()
}

// promptEpisodeLinkFailureRecovery shows a diagnosed dead-end after full resolve
// failure. Returns: remap | audio | episode | back
// DynamicSelect still injects Back/Quit; both map to "back" (single exit path).
func promptEpisodeLinkFailureRecovery(config *CurdConfig, anime *Anime, lastErr error, includeAudio bool) string {
	preferredMode := "sub"
	if config != nil {
		preferredMode = normalizeTranslationType(config.SubOrDub)
	}

	diagnosis := episodeLinkFailureDiagnosis(config, anime, lastErr)
	options := episodeLinkFailureRecoveryOptions(preferredMode, includeAudio)

	var selected SelectionOption
	var err error
	if config != nil && config.RofiSelection {
		// Put diagnosis in Rofi -mesg — avoid one notify-send per CurdOut line.
		selected, err = RofiSelectWithMessage(options, false, "Playback recovery", diagnosis)
	} else {
		fmt.Println(diagnosis)
		selected, err = promptSelectOrdered(options)
	}
	if err != nil {
		return "back"
	}
	selected = NormalizeSelectionKey(selected)
	if SelectionMeansQuit(selected) {
		return "quit"
	}
	if SelectionMeansBack(selected) || selected.Key == "" {
		return "back"
	}
	switch selected.Key {
	case "remap", "audio", "episode":
		return selected.Key
	default:
		return "back"
	}
}

// resolveEpisodeLinksWithRecovery runs preferred-first playback resolve and only
// after that fails presents the diagnosed recovery menu. Returns ok=false when
// the user backs out.
func resolveEpisodeLinksWithRecovery(config *CurdConfig, anime *Anime, anilistEntry *Entry) (ProviderEpisodeResult, bool) {
	if config == nil || anime == nil {
		return ProviderEpisodeResult{}, false
	}

	var lastErr error
	includeAudio := true

	for {
		result, err := ResolveEpisodeURLForPlayback(*config, anime, anime.Ep.Number)
		if err == nil && len(result.Links) > 0 {
			return result, true
		}
		if err != nil {
			lastErr = err
		} else {
			lastErr = fmt.Errorf("no episode links found")
		}
		Log(fmt.Sprintf("ResolveEpisodeURLForPlayback failed for ep %d: %v", anime.Ep.Number, lastErr))

		// Stale mappings surface here and route to the diagnosed recovery
		// menu below; its remap path re-searches every provider.

		switch promptEpisodeLinkFailureRecovery(config, anime, lastErr, includeAudio) {
		case "remap":
			if RemapAnimeProviderOnEpisodeFailure(config, anime, anilistEntry) {
				// Full preferred-first resolve again after remap.
				continue
			}
			lastErr = fmt.Errorf("provider remap did not produce a new mapping")
		case "audio":
			// Explicit re-offer of alternate mode (user may have declined earlier).
			altResult, altErr := ResolveEpisodeURLAlternateModeWithPrompt(*config, anime, anime.Ep.Number, nil)
			if altErr == nil && len(altResult.Links) > 0 {
				return altResult, true
			}
			if altErr != nil {
				lastErr = altErr
				if strings.Contains(strings.ToLower(altErr.Error()), "no ") &&
					strings.Contains(strings.ToLower(altErr.Error()), "streams available") {
					includeAudio = false
				}
			} else {
				lastErr = fmt.Errorf("no %s streams available", alternateTranslationType(config.SubOrDub))
				includeAudio = false
			}
			CurdOut("Still no playable stream after trying other audio.")
		case "episode":
			episodePrompt := "Change episode number (if this one is wrong)"
			providerName, providerID := AnimeProviderID(anime)
			if providerID != "" {
				if episodeList, listErr := EpisodesList(QualifyProviderID(providerName, providerID), config.SubOrDub); listErr == nil && len(episodeList) > 0 {
					episodePrompt = fmt.Sprintf("Change episode number (provider lists up to %v)", episodeList[len(episodeList)-1])
				}
			}
			episodeNumber, promptErr := promptPositiveEpisodeNumber(config, episodePrompt)
			if promptErr != nil {
				Log("Invalid episode input: " + promptErr.Error())
				CurdOut("Invalid episode number")
				continue
			}
			anime.Ep.Number = episodeNumber
			anime.Ep.NextEpisode = NextEpisode{}
			// Re-run full preferred-first resolve for the new episode.
			continue
		default:
			return ProviderEpisodeResult{}, false
		}
	}
}

func ResolveUntrackedProviderSearch(config *CurdConfig, initialQuery string) (providerID, providerName string, back bool, err error) {
	state := &providerMappingSearchState{query: truncateSearchQuery(initialQuery)}
	anime := &Anime{}
	// The audio mode is loop-local: an M1 language flip re-searches in the
	// other mode for this resolve only and never writes config.SubOrDub.
	mode := normalizeTranslationType(config.SubOrDub)

	for {
		anime.Title.English = state.query
		anime.Title.Romaji = state.query

		animeList, failures, searchErr := stackedSearchWithExpansion(config, anime, state.query, mode)
		if searchErr != nil || len(animeList) == 0 {
			reason := ""
			if searchErr != nil {
				Log(fmt.Sprintf("Provider search failed: %v", searchErr))
				reason = fmt.Sprintf("Provider search failed for '%s': %v", state.query, searchErr)
			}
			action, episode := promptMappingRecovery(config, state, mode, failures, animeList, 0, reason)
			outcome, cont, actionErr := handleUntrackedRecoveryAction(config, state, action, episode, &mode)
			if actionErr != nil {
				return "", "", false, actionErr
			}
			if !cont {
				return "", "", outcome == ProviderMappingBack, nil
			}
			continue
		}

		ranked := rankProviderSearchOptions(animeList, anime, state.query, configuredProviderNames(config))
		selected, selectErr := untrackedProviderSearchSelection(config, state.query, ranked)
		if selectErr != nil {
			Log(fmt.Sprintf("Failed to select anime: %v", selectErr))
		} else {
			switch selected.Key {
			case "-1":
				// Dismissed picker routes to recovery.
			case "-2":
				return "", "", true, nil
			default:
				if name, rawID, ok := ParseProviderQualifiedID(selected.Key); ok {
					return rawID, name, false, nil
				}
				return selected.Key, providerNameFromSelection(config, selected), false, nil
			}
		}

		action, episode := promptMappingRecovery(config, state, mode, failures, animeList, 0, "")
		outcome, cont, actionErr := handleUntrackedRecoveryAction(config, state, action, episode, &mode)
		if actionErr != nil {
			return "", "", false, actionErr
		}
		if !cont {
			return "", "", outcome == ProviderMappingBack, nil
		}
	}
}

func handleUntrackedRecoveryAction(config *CurdConfig, state *providerMappingSearchState, action string, episode int, mode *string) (ProviderMappingOutcome, bool, error) {
	switch action {
	case "custom":
		hint := fmt.Sprintf("Enter a search name for configured providers (current: '%s').", state.query)
		newQuery, back, quit, err := promptCustomProviderSearchQuery(config, state.query, hint)
		if err != nil {
			return ProviderMappingQuit, false, err
		}
		if quit {
			return ProviderMappingQuit, false, nil
		}
		if back {
			return ProviderMappingBack, false, nil
		}
		state.query = newQuery
		return ProviderMappingOK, true, nil
	case "audio":
		*mode = alternateTranslationType(*mode)
		return ProviderMappingOK, true, nil
	case "back":
		return ProviderMappingBack, false, nil
	case "quit":
		return ProviderMappingQuit, false, nil
	default:
		return ProviderMappingBack, false, nil
	}
}

func malIDFromProviderExtraData(extra any) int {
	switch item := extra.(type) {
	case anipub.SearchItem:
		return item.MalID
	case senshi.SearchItem:
		return item.MalID
	default:
		return 0
	}
}
