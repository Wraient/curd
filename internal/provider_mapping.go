package internal

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

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

type providerMappingSearchState struct {
	query         string
	allProviders  []string
	sequential    bool
	providerIndex int
}

func (s *providerMappingSearchState) activeProviders() []string {
	if !s.sequential {
		return s.allProviders
	}
	if s.providerIndex >= len(s.allProviders) {
		return nil
	}
	return []string{s.allProviders[s.providerIndex]}
}

func (s *providerMappingSearchState) currentProviderLabel() string {
	if !s.sequential {
		return "all configured providers"
	}
	if s.providerIndex >= len(s.allProviders) {
		return ""
	}
	return s.allProviders[s.providerIndex]
}

func (s *providerMappingSearchState) nextProviderLabel() string {
	if len(s.allProviders) == 0 {
		return ""
	}
	if !s.sequential {
		if len(s.allProviders) <= 1 {
			return ""
		}
		return s.allProviders[0]
	}
	next := s.providerIndex + 1
	if next < len(s.allProviders) {
		return s.allProviders[next]
	}
	return ""
}

func (s *providerMappingSearchState) advanceToNextProvider() bool {
	if len(s.allProviders) == 0 {
		return false
	}
	if !s.sequential {
		if len(s.allProviders) <= 1 {
			return false
		}
		s.sequential = true
		s.providerIndex = 0
		return true
	}
	s.providerIndex++
	return s.providerIndex < len(s.allProviders)
}

func (s *providerMappingSearchState) resetToAllProviders() {
	s.sequential = false
	s.providerIndex = 0
}

func searchAnimeForMapping(config *CurdConfig, state *providerMappingSearchState, mode string) ([]SelectionOption, error) {
	providers := state.activeProviders()
	if len(providers) == 0 {
		return nil, nil
	}
	if !state.sequential && len(providers) == len(state.allProviders) {
		return SearchAnime(state.query, mode)
	}
	return searchAnimeWithProviders(providers, state.query, mode)
}

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

func autoMatchProviderListing(config *CurdConfig, anime *Anime, animeList []SelectionOption, userQuery string, anilistEntry *Entry) bool {
	anilistIDStr := strconv.Itoa(anime.AnilistId)
	var jikanUrls []string
	fetchedJikan := false

	anilistRegex := regexp.MustCompile(`anilistcdn/media/anime/cover/(?:large|medium)/(?:bx)?(\d+)`)
	malRegex := regexp.MustCompile(`myanimelist\.net/images/anime/[^/]+/([^/]+\.jpg)`)
	senshiPosterRE := regexp.MustCompile(`/posters/(\d+)(?:\.webp)?`)

	if anime.MalId == 0 {
		anime.MalId, _ = GetAnimeMalID(anime.AnilistId)
	}
	if anime.MalId != 0 {
		malIDStr := strconv.Itoa(anime.MalId)
		for i, option := range animeList {
			if option.Key == malIDStr {
				Log(fmt.Sprintf("Checking option %d: Key='%s', Label='%s', Thumbnail='%s'", i, option.Key, option.Label, option.Thumbnail))
				anime.ProviderId = option.Key
				Log(fmt.Sprintf("Found MAL ID provider key match! Setting ProviderId to: %s", anime.ProviderId))
				return true
			}
		}
	}

	for i, option := range animeList {
		Log(fmt.Sprintf("Checking option %d: Key='%s', Label='%s', Thumbnail='%s'", i, option.Key, option.Label, option.Thumbnail))

		if malID := malIDFromProviderExtraData(option.ExtraData); malID != 0 && anime.MalId != 0 && malID == anime.MalId {
			anime.ProviderId = option.Key
			Log(fmt.Sprintf("Found provider MAL ID extra-data match! Setting ProviderId to: %s", anime.ProviderId))
			return true
		}

		if strings.Contains(option.Thumbnail, "anilist.co") {
			matches := anilistRegex.FindStringSubmatch(option.Thumbnail)
			if len(matches) > 1 && matches[1] == anilistIDStr {
				anime.ProviderId = option.Key
				Log(fmt.Sprintf("Found Anilist Thumbnail match! Setting ProviderId to: %s", anime.ProviderId))
				return true
			}
		} else if strings.Contains(option.Thumbnail, "myanimelist.net") {
			matches := malRegex.FindStringSubmatch(option.Thumbnail)
			if len(matches) > 1 {
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
		} else if strings.Contains(option.Thumbnail, "/posters/") {
			matches := senshiPosterRE.FindStringSubmatch(option.Thumbnail)
			if len(matches) > 1 && anime.MalId != 0 && matches[1] == strconv.Itoa(anime.MalId) {
				anime.ProviderId = option.Key
				Log(fmt.Sprintf("Found Senshi MAL poster match! Setting ProviderId to: %s", anime.ProviderId))
				return true
			}
		}
	}

	if bestMatch, ok := confidentProviderSearchMatch(animeList, anime, userQuery); ok {
		anime.ProviderId = bestMatch.Key
		Log(fmt.Sprintf("Found confident provider title match! Setting ProviderId to: %s (%s)", anime.ProviderId, bestMatch.Label))
		return true
	}

	if ProviderEnabled("animepahe") && ProviderStackContains(config, "animepahe") {
		Log("Attempting deep metadata matching and exact AniList meta tag check for Animepahe...")

		targetAnilistID := strconv.Itoa(anime.AnilistId)
		var bestMatch *SelectionOption
		var highestScore int

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

				resp, err := sharedHTTPClient.Do(req)
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

func promptProviderSearchRecovery(config *CurdConfig, state *providerMappingSearchState, reason string) (action string, err error) {
	options := []SelectionOption{
		{Key: "custom", Label: "Search with a different name"},
	}
	if next := state.nextProviderLabel(); next != "" {
		options = append(options, SelectionOption{
			Key:   "next_provider",
			Label: fmt.Sprintf("Try next provider (%s)", next),
		})
	}
	options = append(options, SelectionOption{Key: "back", Label: "Back to menu"})

	message := reason
	if message == "" {
		message = fmt.Sprintf("No results found for '%s' on %s.", state.query, state.currentProviderLabel())
	}

	selected, err := selectWithOptionalMessage(config, options, "Provider search", message)
	if err != nil {
		return "", err
	}
	selected = NormalizeSelectionKey(selected)
	if SelectionMeansQuit(selected) {
		return "quit", nil
	}
	if SelectionMeansBack(selected) || selected.Key == "back" {
		return "back", nil
	}
	return selected.Key, nil
}

func promptProviderMatchRecovery(config *CurdConfig, state *providerMappingSearchState) (action string, err error) {
	options := []SelectionOption{
		{Key: "pick", Label: "Pick from search results"},
		{Key: "custom", Label: "Search with a different name"},
	}
	if next := state.nextProviderLabel(); next != "" {
		options = append(options, SelectionOption{
			Key:   "next_provider",
			Label: fmt.Sprintf("Try next provider (%s)", next),
		})
	}
	options = append(options, SelectionOption{Key: "back", Label: "Back to menu"})

	selected, err := selectWithOptionalMessage(config, options, "Provider match", "We didn't find an automatic provider match.")
	if err != nil {
		return "", err
	}
	selected = NormalizeSelectionKey(selected)
	if SelectionMeansQuit(selected) {
		return "quit", nil
	}
	if SelectionMeansBack(selected) || selected.Key == "back" {
		return "back", nil
	}
	return selected.Key, nil
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

func activeProviderName(config *CurdConfig, state *providerMappingSearchState) string {
	if state != nil && state.sequential {
		if name := state.currentProviderLabel(); name != "" {
			return name
		}
	}
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

func providerNameFromSelection(config *CurdConfig, state *providerMappingSearchState, selected SelectionOption) string {
	if providerName, _, ok := ParseProviderQualifiedID(selected.Key); ok {
		return providerName
	}
	if name := providerNameFromSelectionLabel(selected.Label); name != "" {
		return name
	}
	return activeProviderName(config, state)
}

func applyMatchedProviderMapping(config *CurdConfig, state *providerMappingSearchState, anime *Anime) {
	if providerName, rawProviderID, ok := ParseProviderQualifiedID(anime.ProviderId); ok {
		anime.ProviderName = providerName
		anime.ProviderId = rawProviderID
		return
	}
	anime.ProviderName = activeProviderName(config, state)
}

func applySelectedProviderMapping(config *CurdConfig, state *providerMappingSearchState, anime *Anime, selected SelectionOption) {
	anime.ProviderId = selected.Key
	if providerName, rawProviderID, ok := ParseProviderQualifiedID(anime.ProviderId); ok {
		anime.ProviderName = providerName
		anime.ProviderId = rawProviderID
		return
	}
	anime.ProviderName = providerNameFromSelection(config, state, selected)
}

func promptCustomProviderSearchQuery(config *CurdConfig, currentQuery, hint string) (string, bool, bool, error) {
	manualQuery, err := promptText(config, hint, true)
	if err != nil {
		return "", false, false, err
	}
	if manualQuery == "" {
		manualQuery = currentQuery
	}
	return manualQuery, false, false, nil
}

func ResolveAnimeProviderMapping(config *CurdConfig, anime *Anime, query string, anilistEntry *Entry) (ProviderMappingOutcome, error) {
	return resolveAnimeProviderMapping(config, anime, query, anilistEntry, ManualProviderSearchEnabled(config))
}

func resolveAnimeProviderMapping(config *CurdConfig, anime *Anime, query string, anilistEntry *Entry, manualOnly bool) (ProviderMappingOutcome, error) {
	state := &providerMappingSearchState{
		query:        query,
		allProviders: configuredProviderNames(config),
	}

	for {
		Log(fmt.Sprintf("Searching for anime with query: %s, SubOrDub: %s, scope: %s", state.query, config.SubOrDub, state.currentProviderLabel()))

		animeList, err := searchAnimeForMapping(config, state, config.SubOrDub)
		if err != nil {
			Log(fmt.Sprintf("Provider search failed: %v", err))
			action, actionErr := promptProviderSearchRecovery(config, state, fmt.Sprintf("Provider search failed for '%s': %v", state.query, err))
			if actionErr != nil {
				return ProviderMappingQuit, actionErr
			}
			outcome, cont, actionErr := handleProviderMappingAction(config, state, action, query)
			if actionErr != nil {
				return ProviderMappingQuit, actionErr
			}
			if !cont {
				return outcome, nil
			}
			continue
		}

		if len(animeList) == 0 {
			action, actionErr := promptProviderSearchRecovery(config, state, "")
			if actionErr != nil {
				return ProviderMappingQuit, actionErr
			}
			outcome, cont, actionErr := handleProviderMappingAction(config, state, action, query)
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

		if manualOnly {
			for {
				hint := ManualProviderSearchHint(config, anilistEntry, state.query, config.SubOrDub)
				selected, selectErr := promptProviderSearchSelectionWithHint(config, animeList, anilistEntry, hint)
				if selectErr != nil {
					Log(fmt.Sprintf("Failed to select anime: %v", selectErr))
				} else {
					switch selected.Key {
					case "-1":
						// fall through to recovery menu
					case "-2":
						return ProviderMappingBack, nil
					default:
						applySelectedProviderMapping(config, state, anime, selected)
						return ProviderMappingOK, nil
					}
				}

				action, actionErr := promptProviderMatchRecovery(config, state)
				if actionErr != nil {
					return ProviderMappingQuit, actionErr
				}
				switch action {
				case "pick":
					continue
				case "custom", "next_provider":
					outcome, cont, handleErr := handleProviderMappingAction(config, state, action, query)
					if handleErr != nil {
						return ProviderMappingQuit, handleErr
					}
					if !cont {
						return outcome, nil
					}
					break
				case "back":
					return ProviderMappingBack, nil
				case "quit":
					return ProviderMappingQuit, nil
				default:
					continue
				}
				break
			}
			continue
		}

		for {
			action, actionErr := promptProviderMatchRecovery(config, state)
			if actionErr != nil {
				return ProviderMappingQuit, actionErr
			}

			switch action {
			case "pick":
				CurdOut("Select the correct anime from the search results.")
				selected, selectErr := promptProviderSearchSelection(config, animeList, anilistEntry)
				if selectErr != nil {
					Log(fmt.Sprintf("Failed to select anime: %v", selectErr))
					continue
				}
				switch selected.Key {
				case "-1":
					continue
				case "-2":
					return ProviderMappingBack, nil
				default:
					applySelectedProviderMapping(config, state, anime, selected)
					return ProviderMappingOK, nil
				}
			case "custom", "next_provider":
				outcome, cont, handleErr := handleProviderMappingAction(config, state, action, query)
				if handleErr != nil {
					return ProviderMappingQuit, handleErr
				}
				if !cont {
					return outcome, nil
				}
				break
			case "back":
				return ProviderMappingBack, nil
			case "quit":
				return ProviderMappingQuit, nil
			default:
				continue
			}
			break
		}
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

// SwitchProviderStream is what curd-web's "switch source" button actually
// needs: a directly playable stream on one specific provider, right now.
type SwitchProviderStream struct {
	ProviderName string `json:"providerName"`
	ProviderID   string `json:"providerId"`
	Link         string `json:"link"`
	Referrer     string `json:"referrer"`
	Subtitle     string `json:"subtitle"`
}

// SwitchToSingleProviderStream re-resolves the provider mapping for a single
// anime against exactly one named provider, non-interactively, persists it to
// local history (best-effort — a future session's own stack-order search may
// still land elsewhere; see providerNamesForAnime, which deliberately always
// prefers config stack order over any saved provider), and resolves a
// directly playable stream on that provider for the given episode.
//
// This exists instead of just persisting the remap and telling curd to
// replay the episode because curd's own provider resolution
// (episodeModeResultWithProviders) always walks the configured provider
// stack in its fixed order and only falls back to a saved/remapped provider
// if every earlier provider in the stack fails outright — so a remap to a
// provider that isn't first in the stack could otherwise silently never take
// effect, even seconds after the user explicitly chose it. Resolving the
// stream here and having curd-web hand mpv the URL directly sidesteps that
// entirely for this one immediate "watch it on this source now" action.
//
// Unlike RemapProviderAnime (which walks the user through anime selection
// and, on a weak match, an interactive pick/search-again prompt), this only
// succeeds when autoMatchProviderListing finds a confident match on the
// requested provider — it never blocks on stdin. Intended for a fire-and-
// forget CLI invocation.
func SwitchToSingleProviderStream(userCurdConfig *CurdConfig, user *User, databaseAnimes *[]Anime, anilistID, episode int, providerName string) (*SwitchProviderStream, error) {
	if userCurdConfig == nil || user == nil {
		return nil, fmt.Errorf("missing config or user")
	}
	providerName = normalizeProviderName(providerName)
	if providerName == "" {
		return nil, fmt.Errorf("unknown provider name")
	}
	if episode <= 0 {
		return nil, fmt.Errorf("invalid episode number %d", episode)
	}

	anilistEntry, err := FindAnimeByAnilistID(user.AnimeList, strconv.Itoa(anilistID))
	if err != nil || anilistEntry == nil {
		return nil, fmt.Errorf("anime %d not found in anilist list: %w", anilistID, err)
	}

	query := mediaDisplayTitle(anilistEntry.Media, userCurdConfig)
	anime := Anime{
		AnilistId:     anilistEntry.Media.ID,
		MalId:         anilistEntry.Media.MalID,
		Title:         anilistEntry.Media.Title,
		TotalEpisodes: anilistEntry.Media.Episodes,
		CoverImage:    anilistEntry.CoverImage,
	}

	state := &providerMappingSearchState{
		query:         query,
		allProviders:  []string{providerName},
		sequential:    true,
		providerIndex: 0,
	}

	animeList, err := searchAnimeForMapping(userCurdConfig, state, userCurdConfig.SubOrDub)
	if err != nil {
		return nil, fmt.Errorf("provider search failed: %w", err)
	}
	if len(animeList) == 0 {
		return nil, fmt.Errorf("no results from provider %s", providerName)
	}

	anime.ProviderId = ""
	if !autoMatchProviderListing(userCurdConfig, &anime, animeList, query, anilistEntry) {
		return nil, fmt.Errorf("no confident match on provider %s", providerName)
	}
	applyMatchedProviderMapping(userCurdConfig, state, &anime)

	provider, err := ProviderByName(providerName)
	if err != nil {
		return nil, fmt.Errorf("provider %s unavailable: %w", providerName, err)
	}
	links, hints, err := getProviderEpisodeURLForModeWithHints(provider, *userCurdConfig, anime.ProviderId, episode, userCurdConfig.SubOrDub)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch episode %d from %s: %w", episode, providerName, err)
	}
	if len(links) == 0 {
		return nil, fmt.Errorf("no episode %d links found on %s", episode, providerName)
	}
	applyStreamPlaybackHints(&anime, links, hints)
	referrer := anime.Ep.StreamReferrer
	if referrer == "" && isHTTPStreamLink(links[0]) {
		referrer = streamReferrerForLink(links[0], providerName)
	}

	historyPath := filepath.Join(os.ExpandEnv(userCurdConfig.StoragePath), "curd_history.txt")
	if err := persistRemappedProvider(historyPath, databaseAnimes, anilistEntry, &anime, query); err != nil {
		return nil, fmt.Errorf("failed to save remapped provider: %w", err)
	}

	return &SwitchProviderStream{
		ProviderName: providerName,
		ProviderID:   anime.ProviderId,
		Link:         links[0],
		Referrer:     referrer,
		Subtitle:     anime.Ep.SubtitleURL,
	}, nil
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

func handleProviderMappingAction(config *CurdConfig, state *providerMappingSearchState, action, defaultQuery string) (ProviderMappingOutcome, bool, error) {
	switch action {
	case "custom":
		hint := fmt.Sprintf("Enter a search name for configured providers (current: '%s').", state.query)
		newQuery, _, _, err := promptCustomProviderSearchQuery(config, state.query, hint)
		if err != nil {
			return ProviderMappingQuit, false, err
		}
		state.query = newQuery
		state.resetToAllProviders()
		return ProviderMappingOK, true, nil
	case "next_provider":
		if !state.advanceToNextProvider() {
			CurdOut("No more providers left to try.")
			action, err := promptProviderSearchRecovery(config, state, "No more providers left to try.")
			if err != nil {
				return ProviderMappingQuit, false, err
			}
			return handleProviderMappingAction(config, state, action, defaultQuery)
		}
		return ProviderMappingOK, true, nil
	case "back":
		return ProviderMappingBack, false, nil
	case "quit":
		return ProviderMappingQuit, false, nil
	default:
		return ProviderMappingOK, true, nil
	}
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
	anime.ProviderId = ""
	anime.ProviderName = ""
	anime.Ep.NextEpisode = NextEpisode{}

	outcome, err := ResolveAnimeProviderMapping(config, anime, query, anilistEntry)
	if err != nil {
		Log(fmt.Sprintf("Provider remap failed: %v", err))
		return false
	}
	return outcome == ProviderMappingOK
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
func resolveEpisodeLinksWithRecovery(config *CurdConfig, anime *Anime, anilistEntry *Entry, allowAnimepaheReselect bool) (ProviderEpisodeResult, bool) {
	if config == nil || anime == nil {
		return ProviderEpisodeResult{}, false
	}

	var lastErr error
	includeAudio := true
	attemptedAnimepaheReselect := false

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

		// One-shot Animepahe stale-mapping reselect (tracked path helper).
		if allowAnimepaheReselect && !attemptedAnimepaheReselect {
			attemptedAnimepaheReselect = true
			if reselectProviderAnime(config, anime, lastErr) {
				result, err = ResolveEpisodeURLForPlayback(*config, anime, anime.Ep.Number)
				if err == nil && len(result.Links) > 0 {
					Log(fmt.Sprintf("Successfully retrieved %s/%s episode link after provider reselect", result.ProviderName, result.Mode))
					return result, true
				}
				if err != nil {
					lastErr = err
				} else {
					lastErr = fmt.Errorf("no episode links found after provider reselect")
				}
				Log(fmt.Sprintf("ResolveEpisodeURL still failed after provider reselect: %v", lastErr))
			}
		}

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
	state := &providerMappingSearchState{
		query:        initialQuery,
		allProviders: configuredProviderNames(config),
	}

	for {
		animeList, searchErr := searchAnimeForMapping(config, state, config.SubOrDub)
		if searchErr != nil {
			Log(fmt.Sprintf("Provider search failed: %v", searchErr))
			action, actionErr := promptProviderSearchRecovery(config, state, fmt.Sprintf("Provider search failed for '%s': %v", state.query, searchErr))
			if actionErr != nil {
				return "", "", false, actionErr
			}
			if action == "back" {
				return "", "", true, nil
			}
			if action == "quit" {
				return "", "", false, nil
			}
			_, cont, handleErr := handleProviderMappingAction(config, state, action, initialQuery)
			if handleErr != nil {
				return "", "", false, handleErr
			}
			if !cont {
				return "", "", false, nil
			}
			continue
		}

		if len(animeList) == 0 {
			action, actionErr := promptProviderSearchRecovery(config, state, "")
			if actionErr != nil {
				return "", "", false, actionErr
			}
			if action == "back" {
				return "", "", true, nil
			}
			if action == "quit" {
				return "", "", false, nil
			}
			_, cont, handleErr := handleProviderMappingAction(config, state, action, initialQuery)
			if handleErr != nil {
				return "", "", false, handleErr
			}
			if !cont {
				return "", "", false, nil
			}
			continue
		}

		selected, selectErr := untrackedProviderSearchSelection(config, state.query, animeList)
		if selectErr != nil {
			Log(fmt.Sprintf("Failed to select anime: %v", selectErr))
			action, actionErr := promptProviderMatchRecovery(config, state)
			if actionErr != nil {
				return "", "", false, actionErr
			}
			if action == "back" {
				return "", "", true, nil
			}
			if action == "quit" {
				return "", "", false, nil
			}
			_, cont, handleErr := handleProviderMappingAction(config, state, action, initialQuery)
			if handleErr != nil {
				return "", "", false, handleErr
			}
			if !cont {
				return "", "", false, nil
			}
			continue
		}

		switch selected.Key {
		case "-1":
			action, actionErr := promptProviderMatchRecovery(config, state)
			if actionErr != nil {
				return "", "", false, actionErr
			}
			if action == "back" {
				return "", "", true, nil
			}
			if action == "quit" {
				return "", "", false, nil
			}
			_, cont, handleErr := handleProviderMappingAction(config, state, action, initialQuery)
			if handleErr != nil {
				return "", "", false, handleErr
			}
			if !cont {
				return "", "", false, nil
			}
			continue
		case "-2":
			return "", "", true, nil
		default:
			if name, rawID, ok := ParseProviderQualifiedID(selected.Key); ok {
				return rawID, name, false, nil
			}
			return selected.Key, providerNameFromSelection(config, state, selected), false, nil
		}
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
