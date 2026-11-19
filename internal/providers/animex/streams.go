package animex

import (
	"fmt"
	"strings"

	"github.com/wraient/curd/internal/providers"
	"github.com/wraient/curd/internal/providers/hlsverify"
	"github.com/wraient/curd/internal/providers/substyle"
)

// providerPreference orders stream providers: the API marks nero as the
// default (fastest, highest quality); the others are fallbacks.
var providerPreference = []string{"nero", "yuki", "sora"}

func getEpisodeStreamsForMode(showID string, config providers.PlaybackConfig, epNo int) ([]string, map[string]providers.StreamPlaybackHint, error) {
	showID = strings.TrimSpace(showID)
	if showID == "" {
		return nil, nil, fmt.Errorf("empty show id")
	}
	if epNo <= 0 {
		return nil, nil, fmt.Errorf("invalid episode number %d", epNo)
	}

	mode := providers.NormalizeTranslationType(config.SubOrDub)
	lang := "dub"
	if mode == "sub" {
		lang = "sub"
		// animex serves soft subs (track-carrying HLS); there is no
		// hardsub channel, so an explicit hard preference degrades to
		// soft via the shared substyle contract.
		if _, err := substyle.Choose(true, false, config.SubStyle); err != nil {
			return nil, nil, err
		}
	}

	// The response carries both languages; sub and dub share one cached fetch
	// and select locally below.
	servers, err := cachedServers(showID, epNo)
	if err != nil {
		return nil, nil, err
	}
	available := servers.SubProviders
	if mode == "dub" {
		available = servers.DubProviders
	}
	ordered := orderProviders(available)
	if len(ordered) == 0 {
		return nil, nil, fmt.Errorf("animex has no %s servers for episode %d", mode, epNo)
	}

	var lastErr error
	for _, providerID := range ordered {
		links, hints, err := resolveProviderStream(showID, epNo, lang, providerID, mode)
		if err != nil {
			lastErr = err
			continue
		}
		return links, hints, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no playable streams resolved for episode %d", epNo)
	}
	return nil, nil, lastErr
}

func orderProviders(available []ppProvider) []string {
	byID := make(map[string]bool, len(available))
	for _, provider := range available {
		byID[strings.TrimSpace(provider.ID)] = true
	}
	ordered := make([]string, 0, len(available))
	for _, preferred := range providerPreference {
		if byID[preferred] {
			ordered = append(ordered, preferred)
		}
	}
	for _, provider := range available {
		name := strings.TrimSpace(provider.ID)
		if name == "" {
			continue
		}
		known := false
		for _, selected := range ordered {
			if selected == name {
				known = true
				break
			}
		}
		if !known {
			ordered = append(ordered, name)
		}
	}
	return ordered
}

func resolveProviderStream(showID string, epNo int, lang, providerID, mode string) ([]string, map[string]providers.StreamPlaybackHint, error) {
	var payload ppSources
	err := fetchJSON(restURL("/sources", map[string]string{
		"id":         showID,
		"epNum":      fmt.Sprintf("%d", epNo),
		"type":       lang,
		"providerId": providerID,
	}), referrer, &payload)
	if err != nil {
		return nil, nil, err
	}
	if strings.TrimSpace(payload.Error) != "" {
		return nil, nil, fmt.Errorf("animex stream unavailable: %s", payload.Error)
	}
	if len(payload.Sources) == 0 {
		return nil, nil, fmt.Errorf("animex returned no %s sources via %s", lang, providerID)
	}
	streamURL := strings.TrimSpace(payload.Sources[0].URL)
	if streamURL == "" {
		return nil, nil, fmt.Errorf("animex returned an empty stream url")
	}
	if err := hlsverify.Validate(streamURL, hlsverify.Options{Referer: referrer, UserAgent: userAgent}); err != nil {
		return nil, nil, fmt.Errorf("animex stream rejected: %w", err)
	}
	hints := map[string]providers.StreamPlaybackHint{
		streamURL: {Referrer: referrer},
	}
	return []string{streamURL}, hints, nil
}
