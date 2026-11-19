package anineko

import (
	"fmt"
	"strings"
	"sync"

	"github.com/wraient/curd/internal/providers"
)

func getEpisodeStreamsForMode(slug string, config providers.PlaybackConfig, epNo int) ([]string, map[string]providers.StreamPlaybackHint, error) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return nil, nil, fmt.Errorf("empty show id")
	}
	if epNo <= 0 {
		return nil, nil, fmt.Errorf("invalid episode number %d", epNo)
	}

	mode := providers.NormalizeTranslationType(config.SubOrDub)
	html, err := fetchString(watchURL(slug, epNo), baseURL+"/")
	if err != nil {
		return nil, nil, err
	}

	groups := extractLangEmbedURLs(html)
	embedURLs, err := embedURLsForMode(groups, mode, config.SubStyle)
	if err != nil {
		return nil, nil, err
	}
	if len(embedURLs) == 0 {
		return nil, nil, fmt.Errorf("no %s streams found for episode %d", mode, epNo)
	}

	// Embed candidates are independent: resolving them concurrently cuts
	// tail latency from the sum of per-embed attempts to roughly the
	// slowest one. Results are picked in list order below, so the winner
	// is identical to the sequential first-success walk.
	type embedResult struct {
		stream resolvedStream
		err    error
	}
	results := make([]embedResult, len(embedURLs))
	var wg sync.WaitGroup
	for i, embedURL := range embedURLs {
		wg.Add(1)
		go func(idx int, url string) {
			defer wg.Done()
			var result embedResult
			switch resolveEmbedHost(url) {
			case "bibiemb":
				result.stream, result.err = resolveBibiemb(url)
			case "vibeplayer":
				result.stream, result.err = resolveVibeplayer(url)
			default:
				result.err = fmt.Errorf("unknown embed host for %q", url)
			}
			results[idx] = result
		}(i, embedURL)
	}
	wg.Wait()

	for _, result := range results {
		if result.err != nil {
			continue
		}
		return singleStreamResult(result.stream)
	}

	return nil, nil, fmt.Errorf("no playable streams resolved for episode %d", epNo)
}

func singleStreamResult(stream resolvedStream) ([]string, map[string]providers.StreamPlaybackHint, error) {
	if strings.TrimSpace(stream.URL) == "" {
		return nil, nil, fmt.Errorf("empty stream url")
	}
	hints := map[string]providers.StreamPlaybackHint{
		stream.URL: {
			Referrer: stream.Referrer,
			Subtitle: stream.Subtitle,
		},
	}
	return []string{stream.URL}, hints, nil
}
