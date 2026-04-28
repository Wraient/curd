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
	return getAllanimeEpisodeURL(config, id, epNo)
}
