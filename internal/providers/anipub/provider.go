package anipub

import "github.com/wraient/curd/internal/providers"

// Provider implements anipub.xyz catalog search and MegaPlay stream resolution.
type Provider struct{}

func (p *Provider) Name() string {
	return "anipub"
}

func (p *Provider) SearchAnime(query, mode string) ([]providers.SelectionOption, error) {
	return searchAnime(query, mode)
}

func (p *Provider) EpisodesList(showID, mode string) ([]string, error) {
	return episodesList(showID, mode)
}

func (p *Provider) GetEpisodeURL(config providers.PlaybackConfig, id string, epNo int) ([]string, error) {
	links, _, err := p.GetEpisodeURLForModeWithHints(config, id, epNo, config.SubOrDub)
	return links, err
}

func (p *Provider) GetEpisodeURLForMode(config providers.PlaybackConfig, id string, epNo int, mode string) ([]string, error) {
	links, _, err := p.GetEpisodeURLForModeWithHints(config, id, epNo, mode)
	return links, err
}

func (p *Provider) GetEpisodeURLForModeWithHints(config providers.PlaybackConfig, id string, epNo int, mode string) ([]string, map[string]providers.StreamPlaybackHint, error) {
	playback := config
	playback.SubOrDub = providers.NormalizeTranslationType(mode)
	return getEpisodeStreamsForMode(id, playback, epNo)
}
