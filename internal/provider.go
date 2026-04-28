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
