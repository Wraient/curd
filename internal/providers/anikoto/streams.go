package anikoto

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/wraient/curd/internal/providers"
)

var anikotoListItemPattern = regexp.MustCompile(`(?is)<li\b[^>]*>.*?</li>`)

func getEpisodeStreamsForMode(showID string, epNo int, mode string) ([]string, map[string]providers.StreamPlaybackHint, error) {
	slug, _, err := parseShowID(showID)
	if err != nil {
		return nil, nil, err
	}
	if epNo <= 0 {
		return nil, nil, fmt.Errorf("invalid episode number %d", epNo)
	}
	mode = providers.NormalizeTranslationType(mode)
	episode, err := episodeForNumber(showID, epNo)
	if err != nil {
		return nil, nil, err
	}
	if !episodeAvailableForMode(episode, mode) {
		return nil, nil, fmt.Errorf("Anikoto episode %d is unavailable in %s mode", epNo, mode)
	}

	referer := watchURL(slug, epNo)
	candidates, err := fetchServerCandidates(episode.IDs, referer, mode)
	if err != nil {
		return nil, nil, err
	}
	links := make([]string, 0, len(candidates))
	hints := make(map[string]providers.StreamPlaybackHint)
	seen := make(map[string]struct{})
	var resolutionErrors []error
	for _, candidate := range candidates {
		embedURL, err := resolveServerCandidate(candidate.ID, referer)
		if err != nil {
			resolutionErrors = append(resolutionErrors, fmt.Errorf("%s: %w", candidate.Name, err))
			continue
		}
		var stream resolvedStream
		switch {
		case isMegaPlayURL(embedURL):
			stream, err = resolveMegaPlay(embedURL, mode)
		case isDirectM3U8(embedURL):
			stream = resolvedStream{URL: embedURL, Referrer: baseURL + "/"}
		default:
			continue
		}
		if err != nil || strings.TrimSpace(stream.URL) == "" {
			if err != nil {
				resolutionErrors = append(resolutionErrors, fmt.Errorf("%s: %w", candidate.Name, err))
			}
			continue
		}
		if _, exists := seen[stream.URL]; exists {
			continue
		}
		seen[stream.URL] = struct{}{}
		links = append(links, stream.URL)
		hints[stream.URL] = providers.StreamPlaybackHint{Referrer: stream.Referrer, Subtitle: stream.Subtitle}
	}
	if len(links) == 0 {
		if err := errors.Join(resolutionErrors...); err != nil {
			return nil, nil, fmt.Errorf("no playable Anikoto %s streams for episode %d: %w", mode, epNo, err)
		}
		return nil, nil, fmt.Errorf("no playable Anikoto %s streams found for episode %d", mode, epNo)
	}
	return prioritizePlayableStreams(links, hints), hints, nil
}

func fetchServerCandidates(episodeIDs, referer, mode string) ([]serverCandidate, error) {
	params := url.Values{}
	params.Set("servers", episodeIDs)
	endpoint := baseURL + "/ajax/server/list?" + params.Encode()
	raw, err := fetchResource(endpoint, referer, "application/json, text/javascript, */*; q=0.01", true)
	if err != nil {
		return nil, err
	}
	var response ajaxHTMLResponse
	if err := decodeAjaxResponse(raw, &response); err != nil {
		return nil, err
	}
	candidates := parseServerCandidates(response.Result, mode)
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no Anikoto servers found")
	}
	return candidates, nil
}

func parseServerCandidates(body, mode string) []serverCandidate {
	tags := htmlTagPattern.FindAllStringIndex(body, -1)
	typeStarts := make([]int, 0)
	typeTags := make([]string, 0)
	for _, location := range tags {
		tag := body[location[0]:location[1]]
		attrs := parseHTMLAttrs(tag)
		if strings.HasPrefix(strings.ToLower(tag), "<div") && htmlHasClass(attrs, "type") {
			typeStarts = append(typeStarts, location[0])
			typeTags = append(typeTags, tag)
		}
	}

	candidates := make([]serverCandidate, 0)
	for index, start := range typeStarts {
		end := len(body)
		if index+1 < len(typeStarts) {
			end = typeStarts[index+1]
		}
		attrs := parseHTMLAttrs(typeTags[index])
		serverType := canonicalAnikotoServerType(attrs["data-type"])
		if serverType == "" || !anikotoServerTypeMatchesMode(serverType, mode) {
			continue
		}
		for _, item := range anikotoListItemPattern.FindAllString(body[start:end], -1) {
			openingTag := htmlTagPattern.FindString(item)
			itemAttrs := parseHTMLAttrs(openingTag)
			if htmlHasClass(itemAttrs, "download-icon") {
				continue
			}
			serverID := strings.TrimSpace(itemAttrs["data-link-id"])
			if serverID == "" {
				continue
			}
			name := cleanHTMLText(strings.TrimPrefix(item, openingTag))
			name = strings.TrimSpace(strings.TrimSuffix(name, "</li>"))
			if name == "" {
				name = serverID
			}
			candidates = append(candidates, serverCandidate{Type: serverType, ID: serverID, Name: name})
		}
	}
	return candidates
}

func canonicalAnikotoServerType(value string) string {
	normalized := strings.NewReplacer("-", "", " ", "", "_", "").Replace(strings.ToLower(strings.TrimSpace(value)))
	switch normalized {
	case "sub":
		return "Sub"
	case "hsub":
		return "HSub"
	case "ssub":
		return "SSub"
	case "dub":
		return "Dub"
	case "adub":
		return "ADub"
	default:
		return ""
	}
}

func anikotoServerTypeMatchesMode(serverType, mode string) bool {
	if mode == "" {
		return true
	}
	if mode == "dub" {
		return serverType == "Dub" || serverType == "ADub"
	}
	return serverType == "Sub" || serverType == "HSub" || serverType == "SSub"
}

func resolveServerCandidate(serverID, referer string) (string, error) {
	if strings.HasPrefix(serverID, "http://") || strings.HasPrefix(serverID, "https://") {
		return serverID, nil
	}
	params := url.Values{}
	params.Set("get", serverID)
	endpoint := baseURL + "/ajax/server?" + params.Encode()
	raw, err := fetchResource(endpoint, referer, "application/json, text/javascript, */*; q=0.01", true)
	if err != nil {
		return "", err
	}
	var response ajaxURLResponse
	if err := decodeAjaxResponse(raw, &response); err != nil {
		return "", err
	}
	if strings.TrimSpace(response.Result.URL) == "" {
		return "", fmt.Errorf("Anikoto embed URL is missing")
	}
	return strings.TrimSpace(response.Result.URL), nil
}

func isDirectM3U8(rawURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return false
	}
	path := strings.ToLower(parsed.Path)
	return strings.HasSuffix(path, ".m3u8") && !strings.Contains(path, "/stream/")
}
