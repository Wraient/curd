package internal

type AllanimeProvider struct{}

func (p *AllanimeProvider) Name() string {
	return "allanime"
}

func (p *AllanimeProvider) SearchAnime(query, mode string) ([]SelectionOption, error) {
	return searchAllAnime(query, mode)
}

func (p *AllanimeProvider) EpisodesList(showID, mode string) ([]string, error) {
	return getAllAnimeEpisodesList(showID, mode)
}

func (p *AllanimeProvider) GetEpisodeURL(config CurdConfig, id string, epNo int) ([]string, error) {
	return p.GetEpisodeURLForMode(config, id, epNo, config.SubOrDub)
}

func (p *AllanimeProvider) GetEpisodeURLForMode(config CurdConfig, id string, epNo int, mode string) ([]string, error) {
	links, _, err := p.GetEpisodeURLForModeWithHints(config, id, epNo, mode)
	return links, err
}

func (p *AllanimeProvider) GetEpisodeURLForModeWithHints(config CurdConfig, id string, epNo int, mode string) ([]string, map[string]StreamPlaybackHint, error) {
	return getAllanimeEpisodeStreamsForMode(id, mode, epNo)
}
