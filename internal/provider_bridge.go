package internal

import (
	"net/http"

	_ "github.com/wraient/curd/internal/loadproviders"

	"github.com/wraient/curd/internal/curdhost"
	"github.com/wraient/curd/internal/providers"
)

func init() {
	curdhost.HTTPClient = func() *http.Client { return sharedHTTPClient }
	curdhost.Log = func(msg string) { _ = Log(msg) }
	curdhost.Out = func(msg string) { CurdOut(msg) }
	curdhost.PromptSelect = func(options []curdhost.PromptOption) (curdhost.PromptOption, error) {
		mapped := make([]SelectionOption, 0, len(options))
		for _, option := range options {
			mapped = append(mapped, SelectionOption{
				Key:   option.Key,
				Label: option.Label,
			})
		}
		selected, err := promptSelect(mapped)
		if err != nil {
			return curdhost.PromptOption{}, err
		}
		return curdhost.PromptOption{Key: selected.Key, Label: selected.Label}, nil
	}
	curdhost.PersistSubStylePreference = persistSubStylePreference
	curdhost.CurrentSubStyle = func() string {
		if cfg := GetGlobalConfig(); cfg != nil {
			return cfg.SubStyle
		}
		return ""
	}
	curdhost.StoragePath = GetStoragePath
	curdhost.AnimeNameLanguage = func() string {
		if cfg := GetGlobalConfig(); cfg != nil {
			return cfg.AnimeNameLanguage
		}
		return "english"
	}
	curdhost.SetCookiesForAnimepahe = SetCookiesForAnimepahe
}

func normalizeTranslationType(mode string) string {
	return providers.NormalizeTranslationType(mode)
}

func alternateTranslationType(mode string) string {
	return providers.AlternateTranslationType(mode)
}

func toInternalSelectionOptions(options []providers.SelectionOption) []SelectionOption {
	if len(options) == 0 {
		return nil
	}
	result := make([]SelectionOption, 0, len(options))
	for _, option := range options {
		result = append(result, SelectionOption{
			Key:       option.Key,
			Label:     option.Label,
			Title:     option.Title,
			Thumbnail: option.Thumbnail,
			ExtraData: option.ExtraData,
		})
	}
	return result
}

func toProviderSelectionOptions(options []SelectionOption) []providers.SelectionOption {
	if len(options) == 0 {
		return nil
	}
	result := make([]providers.SelectionOption, 0, len(options))
	for _, option := range options {
		result = append(result, providers.SelectionOption{
			Key:       option.Key,
			Label:     option.Label,
			Title:     option.Title,
			Thumbnail: option.Thumbnail,
			ExtraData: option.ExtraData,
		})
	}
	return result
}

func toPlaybackConfig(config CurdConfig) providers.PlaybackConfig {
	return providers.PlaybackConfig{
		SubOrDub: config.SubOrDub,
		SubStyle: config.SubStyle,
	}
}

func fromStreamHints(hints map[string]providers.StreamPlaybackHint) map[string]StreamPlaybackHint {
	if len(hints) == 0 {
		return nil
	}
	result := make(map[string]StreamPlaybackHint, len(hints))
	for key, hint := range hints {
		result[key] = StreamPlaybackHint{
			Referrer: hint.Referrer,
			Subtitle: hint.Subtitle,
		}
	}
	return result
}

type providerAdapter struct {
	inner providers.Provider
}

func (a *providerAdapter) Name() string {
	return a.inner.Name()
}

func (a *providerAdapter) SearchAnime(query, mode string) ([]SelectionOption, error) {
	options, err := a.inner.SearchAnime(query, mode)
	return toInternalSelectionOptions(options), err
}

func (a *providerAdapter) EpisodesList(showID, mode string) ([]string, error) {
	return a.inner.EpisodesList(showID, mode)
}

func (a *providerAdapter) GetEpisodeURL(config CurdConfig, id string, epNo int) ([]string, error) {
	return a.inner.GetEpisodeURL(toPlaybackConfig(config), id, epNo)
}

func (a *providerAdapter) GetEpisodeURLForMode(config CurdConfig, id string, epNo int, mode string) ([]string, error) {
	if resolver, ok := a.inner.(providers.ModeResolver); ok {
		return resolver.GetEpisodeURLForMode(toPlaybackConfig(config), id, epNo, mode)
	}
	return a.inner.GetEpisodeURL(toPlaybackConfig(config), id, epNo)
}

func (a *providerAdapter) GetEpisodeURLForModeWithHints(config CurdConfig, id string, epNo int, mode string) ([]string, map[string]StreamPlaybackHint, error) {
	if resolver, ok := a.inner.(providers.HintResolver); ok {
		links, hints, err := resolver.GetEpisodeURLForModeWithHints(toPlaybackConfig(config), id, epNo, mode)
		return links, fromStreamHints(hints), err
	}
	links, err := a.GetEpisodeURLForMode(config, id, epNo, mode)
	return links, nil, err
}

func resolveProviderID(provider Provider, providerID, query string) (string, error) {
	inner := unwrapProvider(provider)
	if inner == nil {
		return providerID, nil
	}
	if resolver, ok := inner.(providers.IDResolver); ok {
		return resolver.ResolveProviderID(providerID, query)
	}
	return providerID, nil
}

func wrapProvider(provider providers.Provider) Provider {
	return &providerAdapter{inner: provider}
}

func unwrapProvider(provider Provider) providers.Provider {
	if adapter, ok := provider.(*providerAdapter); ok {
		return adapter.inner
	}
	return nil
}

func applyStreamPlaybackHints(anime *Anime, links []string, hints map[string]StreamPlaybackHint) {
	if anime == nil || len(links) == 0 {
		return
	}
	selected := PrioritizeLink(links)
	if hint, ok := hints[selected]; ok {
		anime.Ep.StreamReferrer = hint.Referrer
		anime.Ep.SubtitleURL = hint.Subtitle
		return
	}
	anime.Ep.StreamReferrer = ""
	anime.Ep.SubtitleURL = ""
}
