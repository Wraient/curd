package internal

import (
	"fmt"
	"strconv"
	"strings"
)

// Provider interface defines methods for an anime provider.
type Provider interface {
	Name() string
	SearchAnime(query, mode string) ([]SelectionOption, error)
	EpisodesList(showID, mode string) ([]string, error)
	GetEpisodeURL(config CurdConfig, id string, epNo int) ([]string, error)
}

type ProviderModeResolver interface {
	GetEpisodeURLForMode(config CurdConfig, id string, epNo int, mode string) ([]string, error)
}

type ProviderEpisodeResult struct {
	Links        []string
	ProviderName string
	ProviderID   string
	Mode         string
}

const providerIDSeparator = "::"
const animepaheOptOutProvider = "no-animepahe"

// Global variable to keep the current provider
var CurrentProvider Provider

var providerFactories = map[string]func() Provider{
	"allanime":  func() Provider { return &AllanimeProvider{} },
	"animepahe": func() Provider { return &AnimepaheProvider{} },
}

func normalizeProviderName(providerName string) string {
	providerName = strings.Trim(strings.TrimSpace(providerName), "\"'[]")
	switch strings.ToLower(strings.TrimSpace(providerName)) {
	case "allanime", "all-anime", "all anime":
		return "allanime"
	case "animepahe", "pahe":
		return "animepahe"
	default:
		return ""
	}
}

func normalizeProviderConfigToken(token string) string {
	token = strings.Trim(strings.TrimSpace(token), "\"'[]")
	token = strings.ToLower(strings.Join(strings.Fields(token), "-"))
	token = strings.ReplaceAll(token, "_", "-")
	return token
}

func parseProviderConfigParts(rawProvider string) []string {
	parts := parseStringArray(rawProvider)
	if len(parts) == 0 {
		return strings.FieldsFunc(rawProvider, func(r rune) bool {
			return r == ',' || r == '+' || r == '|' || r == ';'
		})
	}
	return parts
}

func parseProviderConfig(rawProvider string) ([]string, bool) {
	rawProvider = strings.TrimSpace(rawProvider)
	if rawProvider == "" {
		rawProvider = "allanime"
	}

	normalizedRaw := strings.ToLower(rawProvider)
	if normalizedRaw == "stacked" || normalizedRaw == "stack" || normalizedRaw == "auto" || normalizedRaw == "all" {
		return []string{"allanime", "animepahe"}, false
	}

	buildConfig := func(parts []string) ([]string, bool) {
		providers := make([]string, 0, len(parts))
		seen := make(map[string]struct{})
		animepaheDeclined := false

		for _, part := range parts {
			if normalizeProviderConfigToken(part) == animepaheOptOutProvider {
				animepaheDeclined = true
				continue
			}

			name := normalizeProviderName(part)
			if name == "" {
				continue
			}
			if _, exists := seen[name]; exists {
				continue
			}
			seen[name] = struct{}{}
			providers = append(providers, name)
		}

		return providers, animepaheDeclined
	}

	parts := parseProviderConfigParts(rawProvider)
	providers, animepaheDeclined := buildConfig(parts)
	if len(providers) == 0 && !animepaheDeclined {
		parts = strings.FieldsFunc(rawProvider, func(r rune) bool {
			return r == ',' || r == '+' || r == '|' || r == ';'
		})
		providers, animepaheDeclined = buildConfig(parts)
	}

	if len(providers) == 0 {
		providers = []string{"allanime"}
	}

	return providers, animepaheDeclined
}

func configuredProviderNames(config *CurdConfig) []string {
	rawProvider := "allanime"
	if config != nil && strings.TrimSpace(config.Provider) != "" {
		rawProvider = config.Provider
	}

	providers, _ := parseProviderConfig(rawProvider)
	return providers
}

func ConfiguredProviderNames(config *CurdConfig) []string {
	return configuredProviderNames(config)
}

func canonicalProviderConfigValue(rawProvider string) string {
	names, animepaheDeclined := parseProviderConfig(rawProvider)
	return formatProviderConfigValue(names, animepaheDeclined)
}

func formatProviderConfigValue(names []string, animepaheDeclined bool) string {
	quotedNames := make([]string, 0, len(names)+1)
	for _, name := range names {
		quotedNames = append(quotedNames, fmt.Sprintf("%q", name))
	}
	if animepaheDeclined {
		quotedNames = append(quotedNames, fmt.Sprintf("%q", animepaheOptOutProvider))
	}
	return "[" + strings.Join(quotedNames, ",") + "]"
}

func animepaheDeclinedInConfig(config *CurdConfig) bool {
	if config == nil {
		return false
	}
	_, declined := parseProviderConfig(config.Provider)
	return declined
}

func ProviderStackContains(config *CurdConfig, providerName string) bool {
	providerName = normalizeProviderName(providerName)
	if providerName == "" {
		return false
	}
	for _, configuredName := range configuredProviderNames(config) {
		if configuredName == providerName {
			return true
		}
	}
	return false
}

func ProviderByName(providerName string) (Provider, error) {
	providerName = normalizeProviderName(providerName)
	factory, ok := providerFactories[providerName]
	if !ok {
		return nil, fmt.Errorf("unknown provider %q", providerName)
	}
	return factory(), nil
}

func GetProvider() Provider {
	if CurrentProvider != nil {
		return CurrentProvider
	}
	config := GetGlobalConfig()
	providerNames := configuredProviderNames(config)
	primaryProviderName := providerNames[0]
	provider, err := ProviderByName(primaryProviderName)
	if err != nil {
		CurrentProvider = &AllanimeProvider{}
		return CurrentProvider
	}
	CurrentProvider = provider
	return CurrentProvider
}

func QualifyProviderID(providerName, providerID string) string {
	providerName = normalizeProviderName(providerName)
	if providerName == "" || providerID == "" {
		return providerID
	}
	if qualifiedProviderName, _, ok := ParseProviderQualifiedID(providerID); ok && qualifiedProviderName == providerName {
		return providerID
	}
	return providerName + providerIDSeparator + providerID
}

func ParseProviderQualifiedID(providerID string) (providerName, rawProviderID string, ok bool) {
	parts := strings.SplitN(providerID, providerIDSeparator, 2)
	if len(parts) != 2 {
		return "", providerID, false
	}
	providerName = normalizeProviderName(parts[0])
	if providerName == "" || parts[1] == "" {
		return "", providerID, false
	}
	return providerName, parts[1], true
}

func providerNameForAnime(anime *Anime) string {
	if anime == nil {
		return GetProvider().Name()
	}
	if providerName, _, ok := ParseProviderQualifiedID(anime.ProviderId); ok {
		return providerName
	}
	if providerName := normalizeProviderName(anime.ProviderName); providerName != "" {
		return providerName
	}
	return GetProvider().Name()
}

func CurrentAnimeProviderName(anime *Anime) string {
	return providerNameForAnime(anime)
}

func AnimeProviderID(anime *Anime) (string, string) {
	return providerIDForAnime(anime)
}

func providerIDForAnime(anime *Anime) (providerName, providerID string) {
	if anime == nil {
		return GetProvider().Name(), ""
	}
	if providerName, providerID, ok := ParseProviderQualifiedID(anime.ProviderId); ok {
		return providerName, providerID
	}
	if providerName := normalizeProviderName(anime.ProviderName); providerName != "" {
		return providerName, anime.ProviderId
	}
	return GetProvider().Name(), anime.ProviderId
}

func providerNamesForAnime(config *CurdConfig, anime *Anime) []string {
	configuredNames := configuredProviderNames(config)
	providerName, _ := providerIDForAnime(anime)
	providerName = normalizeProviderName(providerName)

	result := make([]string, 0, len(configuredNames)+1)
	seen := make(map[string]struct{})
	for _, name := range configuredNames {
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		result = append(result, name)
	}
	if providerName == "animepahe" && !ProviderStackContains(config, "animepahe") {
		providerName = ""
	}
	if providerName != "" {
		if _, exists := seen[providerName]; !exists {
			result = append(result, providerName)
		}
	}
	if len(result) == 0 {
		return []string{"allanime"}
	}
	return result
}

func qualifySelectionOption(providerName string, option SelectionOption, stacked bool) SelectionOption {
	if !stacked {
		return option
	}
	option.Key = QualifyProviderID(providerName, option.Key)
	if !strings.Contains(strings.ToLower(option.Label), "["+providerName+"]") {
		option.Label = fmt.Sprintf("%s [%s]", option.Label, providerName)
	}
	return option
}

// Wrap functions so they can be easily called
func SearchAnime(query, mode string) ([]SelectionOption, error) {
	config := GetGlobalConfig()
	providerNames := configuredProviderNames(config)
	results, err := searchAnimeWithProviders(providerNames, query, mode)
	if err != nil || len(results) > 0 {
		return results, err
	}

	if shouldOfferAnimepaheFallback(config, providerNames) {
		useAnimepahe, declinedAnimepahe, promptErr := promptAnimepaheFallbackConsent()
		if promptErr != nil {
			return results, promptErr
		}
		if useAnimepahe {
			if updateErr := updateProviderConfig(config, appendProviderName(providerNames, "animepahe"), false); updateErr != nil {
				return results, updateErr
			}
			return searchAnimeWithProviders(configuredProviderNames(config), query, mode)
		}
		if declinedAnimepahe {
			if updateErr := updateProviderConfig(config, providerNames, true); updateErr != nil {
				return results, updateErr
			}
		}
	}

	return results, nil
}

func searchAnimeWithProviders(providerNames []string, query, mode string) ([]SelectionOption, error) {
	if len(providerNames) == 1 {
		provider, err := ProviderByName(providerNames[0])
		if err != nil {
			return nil, err
		}
		return provider.SearchAnime(query, mode)
	}

	results := make([]SelectionOption, 0)
	seen := make(map[string]struct{})
	var searchErrors []string
	for _, providerName := range providerNames {
		provider, err := ProviderByName(providerName)
		if err != nil {
			searchErrors = append(searchErrors, err.Error())
			continue
		}
		options, err := provider.SearchAnime(query, mode)
		if err != nil {
			Log(fmt.Sprintf("Provider %s search failed for %q: %v", providerName, query, err))
			searchErrors = append(searchErrors, fmt.Sprintf("%s: %v", providerName, err))
			continue
		}
		for _, option := range options {
			option = qualifySelectionOption(providerName, option, true)
			if _, exists := seen[option.Key]; exists {
				continue
			}
			seen[option.Key] = struct{}{}
			results = append(results, option)
		}
	}

	if len(results) == 0 && len(searchErrors) > 0 {
		return nil, fmt.Errorf("all provider searches failed: %s", strings.Join(searchErrors, "; "))
	}
	return results, nil
}

func shouldOfferAnimepaheFallback(config *CurdConfig, providerNames []string) bool {
	if config == nil || animepaheDeclinedInConfig(config) {
		return false
	}

	hasAllanime := false
	hasAnimepahe := false
	for _, providerName := range providerNames {
		switch providerName {
		case "allanime":
			hasAllanime = true
		case "animepahe":
			hasAnimepahe = true
		}
	}

	return hasAllanime && !hasAnimepahe
}

func promptAnimepaheFallbackConsent() (bool, bool, error) {
	CurdOut("AllAnime returned no results. Animepahe may require downloading a Chromium browser for DDoS-Guard verification (~500 MB). Use Animepahe fallback?")
	selected, err := promptSelect([]SelectionOption{
		{Key: "use", Label: "Use Animepahe fallback"},
		{Key: "never", Label: "Do not use Animepahe"},
	})
	if err != nil {
		return false, false, err
	}
	switch selected.Key {
	case "use":
		return true, false, nil
	case "never":
		return false, true, nil
	default:
		return false, false, nil
	}
}

func appendProviderName(providerNames []string, providerName string) []string {
	providerName = normalizeProviderName(providerName)
	result := make([]string, 0, len(providerNames)+1)
	seen := make(map[string]struct{})
	for _, name := range providerNames {
		name = normalizeProviderName(name)
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		result = append(result, name)
	}
	if providerName != "" {
		if _, exists := seen[providerName]; !exists {
			result = append(result, providerName)
		}
	}
	if len(result) == 0 {
		return []string{"allanime"}
	}
	return result
}

func updateProviderConfig(config *CurdConfig, providerNames []string, animepaheDeclined bool) error {
	providerValue := formatProviderConfigValue(providerNames, animepaheDeclined)
	config.Provider = providerValue
	CurrentProvider = nil

	if strings.TrimSpace(GlobalConfigPath) == "" {
		return nil
	}

	configMap, err := LoadConfigFromFile(GlobalConfigPath)
	if err != nil {
		return err
	}
	configMap["Provider"] = providerValue
	return SaveConfigToFile(GlobalConfigPath, configMap)
}

func EpisodesList(showID, mode string) ([]string, error) {
	providerName, providerID, ok := ParseProviderQualifiedID(showID)
	if !ok {
		return GetProvider().EpisodesList(showID, mode)
	}
	provider, err := ProviderByName(providerName)
	if err != nil {
		return nil, err
	}
	return provider.EpisodesList(providerID, mode)
}

func GetProviderTotalEpisodes(showID, mode string) (int, error) {
	providerName, providerID, ok := ParseProviderQualifiedID(showID)
	if !ok {
		return getProviderTotalEpisodes(GetProvider(), showID, mode)
	}
	provider, err := ProviderByName(providerName)
	if err != nil {
		return 0, err
	}
	return getProviderTotalEpisodes(provider, providerID, mode)
}

func getProviderTotalEpisodes(provider Provider, showID, mode string) (int, error) {
	if provider == nil {
		return 0, fmt.Errorf("provider is not configured")
	}
	if strings.TrimSpace(showID) == "" {
		return 0, fmt.Errorf("provider id is empty")
	}

	preferredMode := normalizeTranslationType(mode)
	lookupModes := []string{preferredMode}
	if !strings.EqualFold(provider.Name(), "animepahe") {
		lookupModes = append(lookupModes, alternateTranslationType(preferredMode))
	}

	bestTotal := 0
	var lookupErrors []string
	for _, lookupMode := range lookupModes {
		episodes, err := provider.EpisodesList(showID, lookupMode)
		if err != nil {
			lookupErrors = append(lookupErrors, fmt.Sprintf("%s: %v", lookupMode, err))
			continue
		}
		if total := inferProviderTotalEpisodes(provider.Name(), episodes); total > bestTotal {
			bestTotal = total
		}
	}

	if bestTotal > 0 {
		return bestTotal, nil
	}
	if len(lookupErrors) > 0 {
		return 0, fmt.Errorf("provider episode list lookup failed: %s", strings.Join(lookupErrors, "; "))
	}
	return 0, fmt.Errorf("provider returned no usable episode numbers")
}

func inferProviderTotalEpisodes(providerName string, episodes []string) int {
	if strings.EqualFold(providerName, "animepahe") {
		return countUsableEpisodeEntries(episodes)
	}
	return inferTotalEpisodesFromEpisodeList(episodes)
}

func inferTotalEpisodesFromEpisodeList(episodes []string) int {
	total := 0
	for _, episode := range episodes {
		episodeNumber, err := strconv.ParseFloat(strings.TrimSpace(episode), 64)
		if err != nil || episodeNumber <= 0 {
			continue
		}
		if wholeEpisode := int(episodeNumber); wholeEpisode > total {
			total = wholeEpisode
		}
	}
	return total
}

func countUsableEpisodeEntries(episodes []string) int {
	total := 0
	for _, episode := range episodes {
		episodeNumber, err := strconv.ParseFloat(strings.TrimSpace(episode), 64)
		if err != nil || episodeNumber <= 0 {
			continue
		}
		total++
	}
	return total
}

func GetEpisodeURL(config CurdConfig, id string, epNo int) ([]string, error) {
	providerName, providerID, ok := ParseProviderQualifiedID(id)
	if !ok {
		return getProviderEpisodeURLForMode(GetProvider(), config, id, epNo, config.SubOrDub)
	}
	provider, err := ProviderByName(providerName)
	if err != nil {
		return nil, err
	}
	return getProviderEpisodeURLForMode(provider, config, providerID, epNo, config.SubOrDub)
}

func GetEpisodeURLForPlayback(config CurdConfig, id string, epNo int) ([]string, string, error) {
	preferredMode := normalizeTranslationType(config.SubOrDub)
	links, err := GetEpisodeURL(config, id, epNo)
	if err == nil && len(links) > 0 {
		return links, preferredMode, nil
	}

	preferredErr := err
	fallbackMode := alternateTranslationType(preferredMode)
	fallbackConfig := config
	fallbackConfig.SubOrDub = fallbackMode
	fallbackLinks, fallbackErr := GetEpisodeURL(fallbackConfig, id, epNo)
	if fallbackErr != nil || len(fallbackLinks) == 0 {
		if preferredErr != nil {
			return nil, preferredMode, preferredErr
		}
		if fallbackErr != nil {
			return nil, preferredMode, fallbackErr
		}
		return nil, preferredMode, nil
	}

	CurdOut(audioFallbackPrompt(preferredMode, fallbackMode))
	selected, selectErr := promptSelect([]SelectionOption{
		{Key: "play", Label: "Play " + fallbackMode},
		{Key: "cancel", Label: "Cancel"},
	})
	if selectErr != nil {
		return nil, preferredMode, selectErr
	}
	if selected.Key != "play" {
		if preferredErr != nil {
			return nil, preferredMode, preferredErr
		}
		return nil, preferredMode, nil
	}

	return fallbackLinks, fallbackMode, nil
}

func getProviderEpisodeURLForMode(provider Provider, config CurdConfig, id string, epNo int, mode string) ([]string, error) {
	mode = normalizeTranslationType(mode)
	if resolver, ok := provider.(ProviderModeResolver); ok {
		return resolver.GetEpisodeURLForMode(config, id, epNo, mode)
	}
	modeConfig := config
	modeConfig.SubOrDub = mode
	return provider.GetEpisodeURL(modeConfig, id, epNo)
}

func episodeModeResult(config CurdConfig, anime *Anime, epNo int, mode string) (ProviderEpisodeResult, error) {
	if anime == nil {
		modeConfig := config
		modeConfig.SubOrDub = mode
		links, err := GetEpisodeURL(modeConfig, "", epNo)
		return ProviderEpisodeResult{Links: links, ProviderName: GetProvider().Name(), Mode: normalizeTranslationType(mode)}, err
	}

	mode = normalizeTranslationType(mode)
	providerNames := providerNamesForAnime(&config, anime)
	return episodeModeResultWithProviders(config, anime, epNo, mode, providerNames)
}

func episodeModeResultWithProviders(config CurdConfig, anime *Anime, epNo int, mode string, providerNames []string) (ProviderEpisodeResult, error) {
	mode = normalizeTranslationType(mode)
	if len(providerNames) == 0 {
		providerNames = []string{"allanime"}
	}

	var errors []string
	for _, providerName := range providerNames {
		provider, err := ProviderByName(providerName)
		if err != nil {
			errors = append(errors, err.Error())
			continue
		}

		providerID, err := findProviderIDForAnime(provider, anime, mode)
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s lookup: %v", providerName, err))
			continue
		}

		links, err := getProviderEpisodeURLForMode(provider, config, providerID, epNo, mode)
		if err != nil || len(links) == 0 {
			if err != nil {
				errors = append(errors, fmt.Sprintf("%s episode: %v", providerName, err))
			} else {
				errors = append(errors, fmt.Sprintf("%s episode: no links", providerName))
			}
			continue
		}

		anime.ProviderName = providerName
		anime.ProviderId = providerID
		return ProviderEpisodeResult{
			Links:        links,
			ProviderName: providerName,
			ProviderID:   providerID,
			Mode:         mode,
		}, nil
	}

	return ProviderEpisodeResult{}, fmt.Errorf("no %s episode links found across providers %s for %q episode %d: %s", mode, strings.Join(providerNames, ","), animeSearchTitle(anime), epNo, strings.Join(errors, "; "))
}

func ResolveEpisodeURL(config CurdConfig, anime *Anime, epNo int) (ProviderEpisodeResult, error) {
	return episodeModeResult(config, anime, epNo, config.SubOrDub)
}

func ResolveEpisodeURLForPlayback(config CurdConfig, anime *Anime, epNo int) (ProviderEpisodeResult, error) {
	preferredMode := normalizeTranslationType(config.SubOrDub)
	result, err := episodeModeResult(config, anime, epNo, preferredMode)
	if err == nil && len(result.Links) > 0 {
		return result, nil
	}

	preferredErr := err
	runtimeConfig := configForProviderUpdate(config)
	providerNames := configuredProviderNames(runtimeConfig)
	if shouldOfferAnimepaheFallback(runtimeConfig, providerNames) {
		useAnimepahe, declinedAnimepahe, promptErr := promptAnimepaheEpisodeFallbackConsent(preferredMode, epNo)
		if promptErr != nil {
			return ProviderEpisodeResult{}, promptErr
		}
		if useAnimepahe {
			if updateErr := updateProviderConfig(runtimeConfig, appendProviderName(providerNames, "animepahe"), false); updateErr != nil {
				return ProviderEpisodeResult{}, updateErr
			}
			config.Provider = runtimeConfig.Provider
			if anime != nil {
				result, err = episodeModeResultWithProviders(config, anime, epNo, preferredMode, []string{"animepahe"})
			} else {
				result, err = episodeModeResult(config, anime, epNo, preferredMode)
			}
			if err == nil && len(result.Links) > 0 {
				return result, nil
			}
			preferredErr = err
		} else if declinedAnimepahe {
			if updateErr := updateProviderConfig(runtimeConfig, providerNames, true); updateErr != nil {
				return ProviderEpisodeResult{}, updateErr
			}
			config.Provider = runtimeConfig.Provider
		}
	}

	fallbackMode := alternateTranslationType(preferredMode)
	fallbackAnime := anime
	if anime != nil {
		animeCopy := *anime
		fallbackAnime = &animeCopy
	}
	fallbackResult, fallbackErr := episodeModeResult(config, fallbackAnime, epNo, fallbackMode)
	if fallbackErr != nil || len(fallbackResult.Links) == 0 {
		if preferredErr != nil {
			return ProviderEpisodeResult{}, preferredErr
		}
		if fallbackErr != nil {
			return ProviderEpisodeResult{}, fallbackErr
		}
		return ProviderEpisodeResult{}, nil
	}

	CurdOut(audioFallbackPrompt(preferredMode, fallbackMode))
	selected, selectErr := promptSelect([]SelectionOption{
		{Key: "play", Label: "Play " + fallbackMode},
		{Key: "cancel", Label: "Cancel"},
	})
	if selectErr != nil {
		return ProviderEpisodeResult{}, selectErr
	}
	if selected.Key != "play" {
		if preferredErr != nil {
			return ProviderEpisodeResult{}, preferredErr
		}
		return ProviderEpisodeResult{}, nil
	}

	if anime != nil {
		anime.ProviderName = fallbackResult.ProviderName
		anime.ProviderId = fallbackResult.ProviderID
	}
	return fallbackResult, nil
}

func configForProviderUpdate(config CurdConfig) *CurdConfig {
	if globalConfig := GetGlobalConfig(); globalConfig != nil && globalConfig.Provider == config.Provider {
		return globalConfig
	}
	return &config
}

func promptAnimepaheEpisodeFallbackConsent(mode string, epNo int) (bool, bool, error) {
	CurdOut(fmt.Sprintf("No %s stream was found on AllAnime for episode %d. Animepahe may require downloading a Chromium browser for DDoS-Guard verification (~500 MB). Use Animepahe fallback?", normalizeTranslationType(mode), epNo))
	selected, err := promptSelect([]SelectionOption{
		{Key: "use", Label: "Use Animepahe fallback"},
		{Key: "never", Label: "Do not use Animepahe"},
	})
	if err != nil {
		return false, false, err
	}
	switch selected.Key {
	case "use":
		return true, false, nil
	case "never":
		return false, true, nil
	default:
		return false, false, nil
	}
}

func animeSearchTitle(anime *Anime) string {
	if anime == nil {
		return ""
	}
	if title := strings.TrimSpace(GetAnimeName(*anime)); title != "" {
		return title
	}
	if title := strings.TrimSpace(anime.Title.Romaji); title != "" {
		return title
	}
	return strings.TrimSpace(anime.Title.English)
}

func normalizeSearchTitle(title string) string {
	title = strings.ToLower(strings.TrimSpace(title))
	replacer := strings.NewReplacer(":", "", "-", " ", "_", " ", ".", " ", "'", "", "\"", "", "!", "", "?", "", "(", " ", ")", " ")
	title = replacer.Replace(title)
	return strings.Join(strings.Fields(title), " ")
}

func selectBestProviderSearchResult(options []SelectionOption, anime *Anime, query string) (SelectionOption, bool) {
	if len(options) == 0 {
		return SelectionOption{}, false
	}

	targetTitles := []string{
		normalizeSearchTitle(query),
		normalizeSearchTitle(anime.Title.English),
		normalizeSearchTitle(anime.Title.Romaji),
		normalizeSearchTitle(anime.Title.Japanese),
	}

	bestIndex := 0
	bestScore := -1
	for i, option := range options {
		optionTitle := normalizeSearchTitle(option.Title)
		if optionTitle == "" {
			optionTitle = normalizeSearchTitle(option.Label)
		}

		score := len(options) - i
		for _, targetTitle := range targetTitles {
			if targetTitle == "" {
				continue
			}
			if optionTitle == targetTitle {
				score += 100
			} else if strings.Contains(optionTitle, targetTitle) || strings.Contains(targetTitle, optionTitle) {
				score += 35
			}
		}
		if anime.AnilistId != 0 && strings.Contains(option.Thumbnail, fmt.Sprintf("%d", anime.AnilistId)) {
			score += 120
		}
		if anime.TotalEpisodes > 0 && strings.Contains(option.Label, fmt.Sprintf("(%d episodes)", anime.TotalEpisodes)) {
			score += 20
		}
		if item, ok := option.ExtraData.(AnimepaheSearchItem); ok && anime.TotalEpisodes > 0 && item.Episodes == anime.TotalEpisodes {
			score += 20
		}

		if score > bestScore {
			bestScore = score
			bestIndex = i
		}
	}

	return options[bestIndex], true
}

func findProviderIDForAnime(provider Provider, anime *Anime, mode string) (string, error) {
	currentProviderName, currentProviderID := providerIDForAnime(anime)
	if currentProviderName == provider.Name() && currentProviderID != "" {
		return currentProviderID, nil
	}

	query := animeSearchTitle(anime)
	if query == "" {
		return "", fmt.Errorf("cannot search %s without an anime title", provider.Name())
	}

	options, err := provider.SearchAnime(query, mode)
	if err != nil {
		return "", err
	}
	option, ok := selectBestProviderSearchResult(options, anime, query)
	if !ok {
		return "", fmt.Errorf("no %s search results for %q", provider.Name(), query)
	}

	Log(fmt.Sprintf("Mapped %q to provider %s id %s for %s fallback", query, provider.Name(), option.Key, mode))
	return option.Key, nil
}

func audioFallbackPrompt(preferredMode, fallbackMode string) string {
	return titleAudioMode(preferredMode) + " is unavailable for this episode. Play " + fallbackMode + " instead?"
}

func titleAudioMode(mode string) string {
	if normalizeTranslationType(mode) == "dub" {
		return "Dub"
	}
	return "Sub"
}
