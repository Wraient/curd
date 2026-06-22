package providers

// SelectionOption is a provider search or menu result item.
type SelectionOption struct {
	Key       string
	Label     string
	Title     string
	Thumbnail string
	ExtraData any
}

// PlaybackConfig carries playback preferences passed into providers.
type PlaybackConfig struct {
	SubOrDub string
	// SubStyle controls anineko subtitle stream tabs: ask, soft, or hard.
	SubStyle string
}

// StreamPlaybackHint carries MPV playback metadata for a resolved stream URL.
type StreamPlaybackHint struct {
	Referrer string
	Subtitle string
}

// Provider resolves catalog search, episode lists, and stream URLs.
type Provider interface {
	Name() string
	SearchAnime(query, mode string) ([]SelectionOption, error)
	EpisodesList(showID, mode string) ([]string, error)
	GetEpisodeURL(config PlaybackConfig, id string, epNo int) ([]string, error)
}

// ModeResolver resolves episode URLs for an explicit sub/dub mode.
type ModeResolver interface {
	GetEpisodeURLForMode(config PlaybackConfig, id string, epNo int, mode string) ([]string, error)
}

// HintResolver resolves episode URLs with MPV playback hints.
type HintResolver interface {
	GetEpisodeURLForModeWithHints(config PlaybackConfig, id string, epNo int, mode string) ([]string, map[string]StreamPlaybackHint, error)
}

// IDResolver refreshes or validates a provider-specific show ID.
type IDResolver interface {
	ResolveProviderID(providerID, query string) (string, error)
}
