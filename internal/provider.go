package internal

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wraient/curd/internal/providers"
	"github.com/wraient/curd/internal/providers/allanime"
	"github.com/wraient/curd/internal/providers/anikoto"
	"github.com/wraient/curd/internal/providers/animepahe"
	"github.com/wraient/curd/internal/providers/anineko"
	"github.com/wraient/curd/internal/providers/anipm"
	"github.com/wraient/curd/internal/providers/anipub"
	"github.com/wraient/curd/internal/providers/senshi"
)

var (
	anilistThumbnailRE = regexp.MustCompile(`anilistcdn/media/anime/cover/(?:large|medium)/(?:bx)?(\d+)`)
	senshiPosterIDRE   = regexp.MustCompile(`/posters/(\d+)(?:\.webp)?`)
	trailingYearRE     = regexp.MustCompile(`\s+\d{4}$`)
)

// stripTrailingYear removes a catalog year suffix providers append to
// display titles ("Hunter x Hunter (2011)" normalizes to a trailing year
// token). It runs on already-normalized titles, so only a bare trailing
// 4-digit token is stripped. The stripped form is only ever an *additional*
// exact-match candidate alongside the full title.
func stripTrailingYear(normalizedTitle string) string {
	return strings.TrimSpace(trailingYearRE.ReplaceAllString(normalizedTitle, ""))
}

// thumbnailExactAnilistMatch reports whether a thumbnail URL embeds exactly
// the given AniList ID (AniList CDN URLs carry it in the path).
func thumbnailExactAnilistMatch(thumbnail string, anilistID int) bool {
	matches := anilistThumbnailRE.FindStringSubmatch(thumbnail)
	return len(matches) > 1 && matches[1] == strconv.Itoa(anilistID)
}

// senshiPosterMatchesMalID reports whether a senshi poster URL embeds exactly
// the given MAL ID (/posters/<malid>.webp).
func senshiPosterMatchesMalID(thumbnail string, malID int) bool {
	matches := senshiPosterIDRE.FindStringSubmatch(thumbnail)
	return len(matches) > 1 && matches[1] == strconv.Itoa(malID)
}

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
	LinkHints    map[string]StreamPlaybackHint
	ProviderName string
	ProviderID   string
	Mode         string
}

const providerIDSeparator = "::"

// providerSearchTimeout bounds a single provider search inside the stacked
// search so one slow provider cannot stall the whole lookup.
const providerSearchTimeout = 15 * time.Second

// Global variable to keep the current provider
var CurrentProvider Provider

func normalizeProviderName(providerName string) string {
	return providers.NormalizeName(providerName)
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
		return defaultEnabledProviderStack(), false
	}

	normalizedRaw := strings.ToLower(rawProvider)
	if normalizedRaw == "stacked" || normalizedRaw == "stack" || normalizedRaw == "auto" || normalizedRaw == "all" {
		return defaultEnabledProviderStack(), false
	}

	buildConfig := func(parts []string) ([]string, bool) {
		providers := make([]string, 0, len(parts))
		seen := make(map[string]struct{})
		animepaheDeclined := false

		for _, part := range parts {
			if isProviderOptOutToken(part) {
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
		providers = []string{firstEnabledProviderName()}
	}

	return providers, animepaheDeclined
}

func configuredProviderNames(config *CurdConfig) []string {
	rawProvider := stackedProviderConfigValue
	if config != nil && strings.TrimSpace(config.Provider) != "" {
		rawProvider = config.Provider
	}

	providers, _ := parseProviderConfig(rawProvider)
	return ensureEnabledProviderNames(providers)
}

func ConfiguredProviderNames(config *CurdConfig) []string {
	return configuredProviderNames(config)
}

func canonicalProviderConfigValue(rawProvider string) string {
	rawProvider = strings.TrimSpace(rawProvider)
	if isStackedProviderConfig(rawProvider) {
		return stackedProviderConfigValue
	}

	names, animepaheDeclined := parseProviderConfig(rawProvider)
	if !animepaheDeclined && providerListsEqual(names, defaultEnabledProviderStack()) {
		return stackedProviderConfigValue
	}
	return formatProviderConfigValue(names, animepaheDeclined)
}

func formatProviderConfigValue(names []string, animepaheDeclined bool) string {
	quotedNames := make([]string, 0, len(names)+1)
	for _, name := range names {
		quotedNames = append(quotedNames, fmt.Sprintf("%q", name))
	}
	if animepaheDeclined {
		if token := providerOptOutTokenForName("animepahe"); token != "" {
			quotedNames = append(quotedNames, fmt.Sprintf("%q", token))
		}
	}
	return "[" + strings.Join(quotedNames, ",") + "]"
}

func isProviderOptOutToken(token string) bool {
	token = normalizeProviderConfigToken(token)
	if token == "" {
		return false
	}
	for _, name := range providers.RegisteredNames() {
		meta, ok := providers.MetaFor(name)
		if !ok || meta.OptOutToken == "" {
			continue
		}
		if token == normalizeProviderConfigToken(meta.OptOutToken) {
			return true
		}
	}
	return false
}

func providerOptOutTokenForName(providerName string) string {
	meta, ok := providers.MetaFor(providerName)
	if !ok {
		return ""
	}
	return meta.OptOutToken
}

func animepaheDeclinedInConfig(config *CurdConfig) bool {
	if config == nil {
		return false
	}
	_, declined := parseProviderConfig(config.Provider)
	return declined
}

func providerNoPrefetch(providerName string) bool {
	meta, ok := providers.MetaFor(providerName)
	return ok && meta.NoPrefetch
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
	if reason := providerDisabledReason(providerName); reason != "" {
		return nil, fmt.Errorf("provider %q is disabled: %s", providerName, reason)
	}
	provider, err := providers.New(providerName)
	if err != nil {
		return nil, err
	}
	return wrapProvider(provider), nil
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
		if fallback, fallbackErr := ProviderByName(firstEnabledProviderName()); fallbackErr == nil {
			CurrentProvider = fallback
			return CurrentProvider
		}
		return provider
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
	if providerName != "" && !ProviderEnabled(providerName) {
		providerName = ""
	}
	if providerName != "" {
		if _, exists := seen[providerName]; !exists {
			result = append(result, providerName)
		}
	}
	if len(result) == 0 {
		return []string{firstEnabledProviderName()}
	}
	return result
}

func qualifySelectionOption(providerName string, option SelectionOption) SelectionOption {
	option.Key = QualifyProviderID(providerName, option.Key)
	if !strings.Contains(strings.ToLower(option.Label), "["+providerName+"]") {
		option.Label = fmt.Sprintf("%s [%s]", option.Label, providerName)
	}
	return option
}

// Wrap functions so they can be easily called
func SearchAnime(query, mode string) ([]SelectionOption, error) {
	results, _, err := searchAnimeStack(GetGlobalConfig(), query, mode)
	return results, err
}

// ProviderSearchError records a per-provider search failure so recovery menus
// can tell the user which providers failed instead of a bare "no results".
type ProviderSearchError struct {
	Provider string
	Err      string
}

// searchAnimeStack runs the configured-provider search including the
// Animepahe consent fallback. It returns the merged results plus the
// per-provider failures behind them.
func searchAnimeStack(config *CurdConfig, query, mode string) ([]SelectionOption, []ProviderSearchError, error) {
	providerNames := configuredProviderNames(config)
	results, failures, err := searchAnimeWithProviders(providerNames, query, mode)
	if err != nil || len(results) > 0 {
		return results, failures, err
	}

	if shouldOfferAnimepaheFallback(config, providerNames) {
		useAnimepahe, declinedAnimepahe, promptErr := promptAnimepaheFallbackConsent()
		if promptErr != nil {
			return results, failures, promptErr
		}
		if useAnimepahe {
			if updateErr := updateProviderConfig(config, appendProviderName(providerNames, "animepahe"), false); updateErr != nil {
				return results, failures, updateErr
			}
			return searchAnimeWithProviders(configuredProviderNames(config), query, mode)
		}
		if declinedAnimepahe {
			if updateErr := updateProviderConfig(config, providerNames, true); updateErr != nil {
				return results, failures, updateErr
			}
		}
	}

	return results, failures, nil
}

// searchProviderWithTimeout bounds a single provider search so one slow
// provider cannot stall the whole stacked search. The provider call is left
// running; only the wait is abandoned.
func searchProviderWithTimeout(provider Provider, query, mode string) ([]SelectionOption, error) {
	type outcome struct {
		options []SelectionOption
		err     error
	}
	done := make(chan outcome, 1)
	go func() {
		options, err := provider.SearchAnime(query, mode)
		done <- outcome{options: options, err: err}
	}()
	select {
	case result := <-done:
		return result.options, result.err
	case <-time.After(providerSearchTimeout):
		return nil, fmt.Errorf("search timed out after %s", providerSearchTimeout)
	}
}

func searchAnimeWithProviders(providerNames []string, query, mode string) ([]SelectionOption, []ProviderSearchError, error) {
	// Fan out across providers concurrently; a slow provider no longer
	// stalls the rest. Results are merged back in configured stack order so
	// ranking and display stay deterministic.
	type providerResult struct {
		index   int
		options []SelectionOption
		failure *ProviderSearchError
	}
	collected := make([]providerResult, len(providerNames))
	var wg sync.WaitGroup
	for i, providerName := range providerNames {
		wg.Add(1)
		go func(index int, name string) {
			defer wg.Done()
			provider, err := ProviderByName(name)
			if err != nil {
				collected[index] = providerResult{index: index, failure: &ProviderSearchError{Provider: name, Err: err.Error()}}
				return
			}
			options, err := searchProviderWithTimeout(provider, query, mode)
			if err != nil {
				Log(fmt.Sprintf("Provider %s search failed for %q: %v", name, query, err))
				collected[index] = providerResult{index: index, failure: &ProviderSearchError{Provider: name, Err: err.Error()}}
				return
			}
			collected[index] = providerResult{index: index, options: options}
		}(i, providerName)
	}
	wg.Wait()

	results := make([]SelectionOption, 0)
	seen := make(map[string]struct{})
	failures := make([]ProviderSearchError, 0)
	for _, item := range collected {
		providerName := providerNames[item.index]
		if item.failure != nil {
			failures = append(failures, *item.failure)
			continue
		}
		for _, option := range item.options {
			option = qualifySelectionOption(providerName, option)
			if _, exists := seen[option.Key]; exists {
				continue
			}
			seen[option.Key] = struct{}{}
			results = append(results, option)
		}
	}

	if len(results) == 0 && len(failures) > 0 {
		details := make([]string, 0, len(failures))
		for _, failure := range failures {
			details = append(details, fmt.Sprintf("%s: %v", failure.Provider, failure.Err))
		}
		return nil, failures, fmt.Errorf("all provider searches failed: %s", strings.Join(details, "; "))
	}
	return results, failures, nil
}

func shouldOfferAnimepaheFallback(config *CurdConfig, providerNames []string) bool {
	if config == nil || animepaheDeclinedInConfig(config) || !ProviderEnabled("animepahe") {
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
		return []string{firstEnabledProviderName()}
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

// determineProviderTotalEpisodes finds the total episode count for an anime by
// querying the configured provider stack. It first uses the anime's existing
// provider mapping when one exists, then falls back to searching each provider
// for the title and reading its episode list. This keeps the total findable
// even when the preferred provider is down or the anime has no mapping yet
// (e.g. an anime played straight from a MyAnimeList/AniList tracking list).
func determineProviderTotalEpisodes(config *CurdConfig, query string, anime *Anime, mode string) (int, error) {
	if anime == nil {
		return 0, fmt.Errorf("missing anime")
	}

	// 1) Use the anime's existing provider mapping when it has one.
	if providerName, providerID := AnimeProviderID(anime); providerID != "" {
		if provider, err := ProviderByName(providerName); err == nil {
			if total, err := getProviderTotalEpisodes(provider, providerID, mode); err == nil && total > 0 {
				return total, nil
			}
		}
	}

	// 2) Otherwise search each configured provider and read its episode list so
	// a single down provider can't block the total from being determined.
	var lookupErrors []string
	for _, providerName := range configuredProviderNames(config) {
		provider, err := ProviderByName(providerName)
		if err != nil {
			continue
		}
		options, err := provider.SearchAnime(query, mode)
		if err != nil || len(options) == 0 {
			if err != nil {
				lookupErrors = append(lookupErrors, fmt.Sprintf("%s search: %v", providerName, err))
			}
			continue
		}
		searchQuery := strings.TrimSpace(query)
		if searchQuery == "" {
			searchQuery = animeSearchTitle(anime)
		}
		// Identity only: totals must be read from a result proven to be the
		// same show, never from a fuzzy best guess.
		matched, ok := identityMatchProviderOption(options, anime, identityScope{query: searchQuery, episodes: anime.TotalEpisodes})
		if !ok {
			continue
		}
		providerID := rawProviderKey(matched.Key)
		if strings.TrimSpace(providerID) == "" {
			continue
		}
		if total, err := getProviderTotalEpisodes(provider, providerID, mode); err == nil && total > 0 {
			return total, nil
		}
	}

	if len(lookupErrors) > 0 {
		return 0, fmt.Errorf("could not determine total episodes from any provider: %s", strings.Join(lookupErrors, "; "))
	}
	return 0, fmt.Errorf("could not determine total episodes from any provider")
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

func getProviderEpisodeURLForModeWithHints(provider Provider, config CurdConfig, id string, epNo int, mode string) ([]string, map[string]StreamPlaybackHint, error) {
	mode = normalizeTranslationType(mode)
	if withHints, ok := provider.(interface {
		GetEpisodeURLForModeWithHints(CurdConfig, string, int, string) ([]string, map[string]StreamPlaybackHint, error)
	}); ok {
		return withHints.GetEpisodeURLForModeWithHints(config, id, epNo, mode)
	}
	if resolver, ok := provider.(ProviderModeResolver); ok {
		links, err := resolver.GetEpisodeURLForMode(config, id, epNo, mode)
		return links, nil, err
	}
	modeConfig := config
	modeConfig.SubOrDub = mode
	links, err := provider.GetEpisodeURL(modeConfig, id, epNo)
	return links, nil, err
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
		providerNames = []string{firstEnabledProviderName()}
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

		links, linkHints, err := getProviderEpisodeURLForModeWithHints(provider, config, providerID, epNo, mode)
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
			LinkHints:    linkHints,
			ProviderName: providerName,
			ProviderID:   providerID,
			Mode:         mode,
		}, nil
	}

	return ProviderEpisodeResult{}, fmt.Errorf("no %s episode links found across providers %s for %q episode %d: %s", mode, strings.Join(providerNames, ","), animeSearchTitle(anime), epNo, strings.Join(errors, "; "))
}

func filterExcludedProviders(providerNames []string, exclude []string) []string {
	excludeSet := make(map[string]struct{}, len(exclude))
	for _, name := range exclude {
		if normalized := normalizeProviderName(name); normalized != "" {
			excludeSet[normalized] = struct{}{}
		}
	}

	filtered := make([]string, 0, len(providerNames))
	for _, name := range providerNames {
		if _, excluded := excludeSet[normalizeProviderName(name)]; excluded {
			continue
		}
		filtered = append(filtered, name)
	}
	return filtered
}

// ResolveEpisodeURLExcludingProviders tries preferred SubOrDub across providers, skipping exclude.
func ResolveEpisodeURLExcludingProviders(config CurdConfig, anime *Anime, epNo int, exclude []string) (ProviderEpisodeResult, error) {
	return resolveEpisodeURLExcludingProvidersMode(config, anime, epNo, exclude, normalizeTranslationType(config.SubOrDub))
}

// ResolveEpisodeURLExcludingProvidersMode tries an explicit sub/dub mode across providers, skipping exclude.
func ResolveEpisodeURLExcludingProvidersMode(config CurdConfig, anime *Anime, epNo int, exclude []string, mode string) (ProviderEpisodeResult, error) {
	return resolveEpisodeURLExcludingProvidersMode(config, anime, epNo, exclude, mode)
}

func resolveEpisodeURLExcludingProvidersMode(config CurdConfig, anime *Anime, epNo int, exclude []string, mode string) (ProviderEpisodeResult, error) {
	mode = normalizeTranslationType(mode)
	providerNames := filterExcludedProviders(providerNamesForAnime(&config, anime), exclude)
	if len(providerNames) == 0 {
		return ProviderEpisodeResult{}, fmt.Errorf("no providers left to try")
	}
	return episodeModeResultWithProviders(config, anime, epNo, mode, providerNames)
}

// ResolveEpisodeURLAlternateModeWithPrompt probes the non-preferred sub/dub mode and asks
// before switching. Preferred mode is never switched silently.
func ResolveEpisodeURLAlternateModeWithPrompt(config CurdConfig, anime *Anime, epNo int, exclude []string) (ProviderEpisodeResult, error) {
	preferredMode := normalizeTranslationType(config.SubOrDub)
	fallbackMode := alternateTranslationType(preferredMode)

	providerNames := filterExcludedProviders(providerNamesForAnime(&config, anime), exclude)
	if len(providerNames) == 0 {
		return ProviderEpisodeResult{}, fmt.Errorf("no providers left to try for %s", fallbackMode)
	}

	// Probe on a copy so rejecting the prompt does not mutate the live anime mapping.
	probeAnime := anime
	if anime != nil {
		animeCopy := *anime
		probeAnime = &animeCopy
	}
	fallbackResult, fallbackErr := episodeModeResultWithProviders(config, probeAnime, epNo, fallbackMode, providerNames)
	if fallbackErr != nil || len(fallbackResult.Links) == 0 {
		if fallbackErr != nil {
			return ProviderEpisodeResult{}, fallbackErr
		}
		return ProviderEpisodeResult{}, fmt.Errorf("no %s streams available", fallbackMode)
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
		return ProviderEpisodeResult{}, fmt.Errorf("%s unavailable and %s fallback declined", preferredMode, fallbackMode)
	}

	if anime != nil {
		anime.ProviderName = fallbackResult.ProviderName
		anime.ProviderId = fallbackResult.ProviderID
	}
	return fallbackResult, nil
}

func ResolveEpisodeURL(config CurdConfig, anime *Anime, epNo int) (ProviderEpisodeResult, error) {
	return episodeModeResult(config, anime, epNo, config.SubOrDub)
}

func ResolveEpisodeURLForPlayback(config CurdConfig, anime *Anime, epNo int) (ProviderEpisodeResult, error) {
	preferredMode := normalizeTranslationType(config.SubOrDub)

	// 1) Exhaust preferred sub/dub across the full provider stack before any mode switch.
	result, err := episodeModeResult(config, anime, epNo, preferredMode)
	if err == nil && len(result.Links) > 0 {
		return result, nil
	}

	preferredErr := err
	runtimeConfig := configForProviderUpdate(config)
	providerNames := configuredProviderNames(runtimeConfig)

	// 2) Optional Animepahe still uses preferred mode only.
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

	// 3) Only after preferred is exhausted, probe alternate mode and always ask the user.
	fallbackResult, fallbackErr := ResolveEpisodeURLAlternateModeWithPrompt(config, anime, epNo, nil)
	if fallbackErr != nil || len(fallbackResult.Links) == 0 {
		if preferredErr != nil {
			return ProviderEpisodeResult{}, preferredErr
		}
		if fallbackErr != nil {
			return ProviderEpisodeResult{}, fallbackErr
		}
		return ProviderEpisodeResult{}, nil
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
	replacer := strings.NewReplacer(":", "", "-", " ", "_", " ", ".", " ", "'", "", "\"", "", "!", "", "?", "", "(", " ", ")", " ", "/", " ", ";", " ")
	title = replacer.Replace(title)
	return strings.Join(strings.Fields(title), " ")
}

func scoreProviderSearchOption(option SelectionOption, anime *Anime, query string) int {
	optionTitle := normalizeSearchTitle(option.Title)
	if optionTitle == "" {
		optionTitle = normalizeSearchTitle(option.Label)
	}

	targetTitles := []string{
		normalizeSearchTitle(query),
		normalizeSearchTitle(anime.Title.English),
		normalizeSearchTitle(anime.Title.Romaji),
		normalizeSearchTitle(anime.Title.Japanese),
	}

	// Ranking only: this score orders the picker list and never decides a
	// match. Position-in-list and substring heuristics were removed because
	// they are not identity.
	score := 0
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
	if anime.AnilistId != 0 && thumbnailExactTokenMatch(option.Thumbnail, anime.AnilistId) {
		score += 120
	}
	if anime.TotalEpisodes > 0 && strings.Contains(option.Label, fmt.Sprintf("(%d episodes)", anime.TotalEpisodes)) {
		score += 20
	}
	if item, ok := option.ExtraData.(animepahe.SearchItem); ok && anime.TotalEpisodes > 0 && item.Episodes == anime.TotalEpisodes {
		score += 20
	}
	return score
}

func selectBestProviderSearchResult(options []SelectionOption, anime *Anime, query string) (SelectionOption, bool) {
	ranked := rankProviderSearchOptions(options, anime, query, nil)
	if len(ranked) == 0 {
		return SelectionOption{}, false
	}
	return ranked[0], true
}

// rankProviderSearchOptions orders results by title/thumbnail signal for
// display. Ranking never decides a match — identity checks do that.
func rankProviderSearchOptions(options []SelectionOption, anime *Anime, query string, providerOrder []string) []SelectionOption {
	ranked := append([]SelectionOption(nil), options...)
	order := make(map[string]int, len(providerOrder))
	for i, name := range providerOrder {
		order[name] = i
	}
	providerRank := func(option SelectionOption) int {
		if name, _, ok := ParseProviderQualifiedID(option.Key); ok {
			if rank, exists := order[name]; exists {
				return rank
			}
		}
		return len(providerOrder)
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		scoreI := scoreProviderSearchOption(ranked[i], anime, query)
		scoreJ := scoreProviderSearchOption(ranked[j], anime, query)
		if scoreI != scoreJ {
			return scoreI > scoreJ
		}
		return providerRank(ranked[i]) < providerRank(ranked[j])
	})
	return ranked
}

// thumbnailExactTokenMatch reports whether an AniList ID appears as a whole
// numeric token in a thumbnail URL. A bare substring contains would match ID
// 1 against nearly every URL, so tokens are compared for equality.
func thumbnailExactTokenMatch(thumbnail string, anilistID int) bool {
	want := strconv.Itoa(anilistID)
	var token strings.Builder
	flush := func() bool {
		if token.Len() > 0 {
			matched := token.String() == want
			token.Reset()
			return matched
		}
		return false
	}
	for _, r := range thumbnail {
		if r >= '0' && r <= '9' {
			token.WriteRune(r)
			continue
		}
		if flush() {
			return true
		}
	}
	return flush()
}

// providerFacts carries identity signals extracted from a provider result's
// ExtraData. Every field comes from the search response itself — gathering
// facts never costs an extra network request.
type providerFacts struct {
	malID int
	// episodes is the catalog episode count when the provider reports one.
	episodes int
	// typ is the release type (TV, Movie, ...) when the provider reports one.
	typ string
	// japanese is an alternate original-language title (anikoto data-jp).
	japanese string
}

func providerFactsFromExtra(extra any) providerFacts {
	switch item := extra.(type) {
	case anipub.SearchItem:
		return providerFacts{malID: item.MalID, episodes: item.Episodes}
	case senshi.SearchItem:
		return providerFacts{malID: item.MalID, episodes: item.Episodes, typ: item.Type}
	case animepahe.SearchItem:
		return providerFacts{episodes: item.Episodes, typ: item.Type}
	case anipm.SearchItem:
		return providerFacts{malID: item.MalID, episodes: item.Episodes, typ: item.Type}
	case anikoto.SearchItem:
		return providerFacts{japanese: item.JapaneseTitle}
	case allanime.SearchItem:
		return providerFacts{episodes: item.Episodes}
	case anineko.SearchItem:
		return providerFacts{typ: item.Type}
	default:
		return providerFacts{}
	}
}

// normalizeReleaseType maps release-type spellings ("TV", "TV_SHORT",
// "TV Short", "Movie") to one comparable form for exact matching.
func normalizeReleaseType(raw string) string {
	upper := strings.ToUpper(strings.TrimSpace(raw))
	upper = strings.ReplaceAll(upper, "_", " ")
	upper = strings.ReplaceAll(upper, "-", " ")
	return strings.Join(strings.Fields(upper), " ")
}

// identityTargetTitles returns the normalized titles that identify the anime:
// the search query plus every known tracker title.
func identityTargetTitles(anime *Anime, query string) []string {
	targets := make([]string, 0, 4)
	for _, title := range []string{
		query,
		anime.Title.English,
		anime.Title.Romaji,
		anime.Title.Japanese,
	} {
		if normalized := normalizeSearchTitle(title); normalized != "" {
			targets = append(targets, normalized)
		}
	}
	return targets
}

func rawProviderKey(qualifiedKey string) string {
	if _, rawID, ok := ParseProviderQualifiedID(qualifiedKey); ok {
		return rawID
	}
	return qualifiedKey
}

// identityMatchProviderOption returns the provider-scoped ID of the option
// that shares cross-system identity with anime, using only data already in
// hand: an exact MAL ID (key or ExtraData), an exact AniList/senshi image
// ID, an exact title plus matching episode count, an exact title plus
// matching release type, or one unique exact title match. Anything else
// returns ok=false so playback never guesses a show.
// identityScope carries the tracker-known context used to disambiguate
// same-title provider results. Episodes and format gate the multi-axis
// rules; fetchEpisodes (nil = no fetching) enriches candidates that carry
// no episode count by reading their existing episode lists, bounded so one
// ambiguous match costs at most a few list fetches.
type identityScope struct {
	query    string
	episodes int
	format   string
	mode     string
	// provider names the single provider options came from when they
	// carry unqualified keys (direct per-provider searches).
	provider      string
	fetchEpisodes func(providerName, rawID, mode string) ([]string, error)
}

func identityMatchProviderOption(options []SelectionOption, anime *Anime, scope identityScope) (SelectionOption, bool) {
	query := scope.query
	trackerEpisodes := scope.episodes
	trackerFormat := scope.format
	for _, option := range options {
		key := rawProviderKey(option.Key)
		if anime.MalId != 0 {
			if key == strconv.Itoa(anime.MalId) {
				return option, true
			}
			if malID := malIDFromProviderExtraData(option.ExtraData); malID != 0 && malID == anime.MalId {
				return option, true
			}
		}
		if anime.AnilistId != 0 && thumbnailExactAnilistMatch(option.Thumbnail, anime.AnilistId) {
			return option, true
		}
		if anime.MalId != 0 && senshiPosterMatchesMalID(option.Thumbnail, anime.MalId) {
			return option, true
		}
	}

	type titleHit struct {
		option   SelectionOption
		episodes int
		typ      string
		full     bool
	}
	targets := identityTargetTitles(anime, query)
	var hits []titleHit
	anyFull := false
	for _, option := range options {
		facts := providerFactsFromExtra(option.ExtraData)
		base := normalizeSearchTitle(option.Title)
		if base == "" {
			base = normalizeSearchTitle(option.Label)
		}
		type candidate struct {
			title    string
			verbatim bool
		}
		candidates := []candidate{{title: base, verbatim: true}}
		if stripped := stripTrailingYear(base); stripped != "" && stripped != base {
			candidates = append(candidates, candidate{title: stripped})
		}
		if normalized := normalizeSearchTitle(facts.japanese); normalized != "" {
			candidates = append(candidates, candidate{title: normalized, verbatim: true})
			if stripped := stripTrailingYear(normalized); stripped != "" && stripped != normalized {
				candidates = append(candidates, candidate{title: stripped})
			}
		}
		matched, full := false, false
		for _, optionTitle := range candidates {
			if optionTitle.title == "" {
				continue
			}
			for _, target := range targets {
				if optionTitle.title == target {
					matched = true
					full = full || optionTitle.verbatim
					break
				}
			}
			if matched {
				break
			}
		}
		if matched {
			hits = append(hits, titleHit{option: option, episodes: facts.episodes, typ: facts.typ, full: full})
			anyFull = anyFull || full
		}
	}
	// A full exact title beats year-stripped variants: only fall back to
	// stripped candidates when nothing matches verbatim.
	if anyFull {
		full := hits[:0]
		for _, hit := range hits {
			if hit.full {
				full = append(full, hit)
			}
		}
		hits = full
	}
	if len(hits) == 0 {
		return SelectionOption{}, false
	}
	// Same exact title on several entries (seasons, remakes, movies):
	// disambiguate by exact episode count, then by exact release type.
	// Either signal must be known on both sides to count, and a *known*
	// contradiction vetoes: a candidate whose provider-reported data
	// disagrees with the tracker can never auto-match, even alone.
	// Candidates that carry no count are enriched via their existing
	// episode lists (bounded).
	if trackerEpisodes > 0 || normalizeReleaseType(trackerFormat) != "" {
		kept := make([]titleHit, 0, len(hits))
		fetches := 0
		for _, hit := range hits {
			episodes, typ := hit.episodes, hit.typ
			if episodes == 0 && trackerEpisodes > 0 && scope.fetchEpisodes != nil && fetches < 3 {
				name, _, _ := ParseProviderQualifiedID(hit.option.Key)
				if name == "" {
					// Unqualified keys from a direct per-provider search.
					name = scope.provider
				}
				if name != "" {
					if list, err := scope.fetchEpisodes(name, rawProviderKey(hit.option.Key), scope.mode); err == nil {
						episodes = inferTotalEpisodesFromEpisodeList(list)
					}
					fetches++
				}
			}
			if trackerEpisodes > 0 && episodes != 0 && episodes != trackerEpisodes {
				continue
			}
			if normalized := normalizeReleaseType(trackerFormat); normalized != "" && typ != "" && normalizeReleaseType(typ) != normalized {
				continue
			}
			hit.episodes = episodes
			kept = append(kept, hit)
		}
		hits = kept
	}
	if len(hits) == 0 {
		return SelectionOption{}, false
	}
	if trackerEpisodes > 0 {
		var episodeHits []titleHit
		for _, hit := range hits {
			if hit.episodes == trackerEpisodes {
				episodeHits = append(episodeHits, hit)
			}
		}
		if len(episodeHits) == 1 {
			return episodeHits[0].option, true
		}
	}
	if normalized := normalizeReleaseType(trackerFormat); normalized != "" {
		var typeHits []titleHit
		for _, hit := range hits {
			if hit.typ != "" && normalizeReleaseType(hit.typ) == normalized {
				typeHits = append(typeHits, hit)
			}
		}
		if len(typeHits) == 1 {
			return typeHits[0].option, true
		}
	}
	if len(hits) == 1 {
		return hits[0].option, true
	}
	return SelectionOption{}, false
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
	if len(options) == 0 {
		return "", fmt.Errorf("no %s search results for %q", provider.Name(), query)
	}
	// Identity only: playback must never adopt a fuzzy title guess as the
	// show. A miss here routes to the diagnosed recovery menu instead.
	// Ambiguous same-title candidates are disambiguated with their existing
	// episode lists (bounded, no new endpoints).
	playbackScope := identityScope{
		query:    query,
		episodes: anime.TotalEpisodes,
		mode:     normalizeTranslationType(mode),
		provider: provider.Name(),
		fetchEpisodes: func(providerName, rawID, lookupMode string) ([]string, error) {
			return EpisodesList(QualifyProviderID(providerName, rawID), lookupMode)
		},
	}
	if matched, ok := identityMatchProviderOption(options, anime, playbackScope); ok {
		id := rawProviderKey(matched.Key)
		Log(fmt.Sprintf("Mapped %q to provider %s id %s for %s fallback", query, provider.Name(), id, mode))
		return id, nil
	}
	return "", fmt.Errorf("no identity-matched %s result for %q", provider.Name(), query)
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
