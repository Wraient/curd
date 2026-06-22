package allanime

import (
	"github.com/wraient/curd/internal/providers"
)

// Provider implements AllAnime catalog search and stream resolution.
type Provider struct{}

func (p *Provider) Name() string {
	return "allanime"
}

func (p *Provider) SearchAnime(query, mode string) ([]providers.SelectionOption, error) {
	return searchAllAnime(query, mode)
}

func (p *Provider) EpisodesList(showID, mode string) ([]string, error) {
	return getAllAnimeEpisodesList(showID, mode)
}

func (p *Provider) GetEpisodeURL(config providers.PlaybackConfig, id string, epNo int) ([]string, error) {
	return p.GetEpisodeURLForMode(config, id, epNo, config.SubOrDub)
}

func (p *Provider) GetEpisodeURLForMode(config providers.PlaybackConfig, id string, epNo int, mode string) ([]string, error) {
	links, _, err := p.GetEpisodeURLForModeWithHints(config, id, epNo, mode)
	return links, err
}

func (p *Provider) GetEpisodeURLForModeWithHints(config providers.PlaybackConfig, id string, epNo int, mode string) ([]string, map[string]providers.StreamPlaybackHint, error) {
	return getAllanimeEpisodeStreamsForMode(id, mode, epNo)
}
