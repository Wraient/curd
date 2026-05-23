package internal

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
	if config != nil && config.Provider == "animepahe" {
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
