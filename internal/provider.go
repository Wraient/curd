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

// Global variable to keep the current provider
var CurrentProvider Provider

func GetProvider() Provider {
	if CurrentProvider != nil {
		return CurrentProvider
	}
	config := GetGlobalConfig()
	if config != nil && strings.EqualFold(strings.TrimSpace(config.Provider), "animepahe") {
		CurrentProvider = &AnimepaheProvider{}
	} else {
		CurrentProvider = &AllanimeProvider{}
	}
	return CurrentProvider
}

// Wrap functions so they can be easily called
func SearchAnime(query, mode string) ([]SelectionOption, error) {
	return GetProvider().SearchAnime(query, mode)
}

func EpisodesList(showID, mode string) ([]string, error) {
	return GetProvider().EpisodesList(showID, mode)
}

func GetProviderTotalEpisodes(showID, mode string) (int, error) {
	return getProviderTotalEpisodes(GetProvider(), showID, mode)
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
	return GetProvider().GetEpisodeURL(config, id, epNo)
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

func audioFallbackPrompt(preferredMode, fallbackMode string) string {
	return titleAudioMode(preferredMode) + " is unavailable for this episode. Play " + fallbackMode + " instead?"
}

func titleAudioMode(mode string) string {
	if normalizeTranslationType(mode) == "dub" {
		return "Dub"
	}
	return "Sub"
}
