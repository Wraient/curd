package anineko

import (
	"net/url"
	"regexp"
	"strings"
)

var (
	dataVideoRE      = regexp.MustCompile(`data-video="([^"]+)"`)
	langMarkerRE     = regexp.MustCompile(`data-id="(hsub|sub|dub)"`)
	subtitleRE       = regexp.MustCompile(`[?&](?:sub|caption_1|c1_file)=([^&"]+)`)
	embedSubtitleRE  = regexp.MustCompile(`const subtitle = "([^"]+)"`)
	embedTrackFileRE = regexp.MustCompile(`file:\s*"([^"]+\.(?:vtt|ass|srt))"`)
)

func extractLangEmbedURLs(html string) map[string][]string {
	groups := map[string][]string{
		"hsub": {},
		"sub":  {},
		"dub":  {},
	}
	markers := langMarkerRE.FindAllStringSubmatchIndex(html, -1)
	for i, marker := range markers {
		if len(marker) < 4 {
			continue
		}
		lang := html[marker[2]:marker[3]]
		start := marker[1]
		end := len(html)
		if i+1 < len(markers) {
			end = markers[i+1][0]
		}
		block := html[start:end]
		seen := map[string]struct{}{}
		for _, videoMatch := range dataVideoRE.FindAllStringSubmatch(block, -1) {
			if len(videoMatch) < 2 {
				continue
			}
			embedURL := strings.TrimSpace(videoMatch[1])
			if embedURL == "" {
				continue
			}
			if _, ok := seen[embedURL]; ok {
				continue
			}
			seen[embedURL] = struct{}{}
			groups[lang] = append(groups[lang], embedURL)
		}
	}
	return groups
}

func isVibeplayerHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return false
	}
	if strings.Contains(host, "vibeplayer.") || strings.Contains(host, "vivibebe.") {
		return true
	}
	return strings.HasPrefix(host, "vibe") && strings.Contains(host, "be.")
}

func resolveEmbedHost(embedURL string) string {
	switch {
	case strings.Contains(strings.ToLower(hostFromURL(embedURL)), "bibiemb."):
		return "bibiemb"
	case isVibeplayerHost(hostFromURL(embedURL)):
		return "vibeplayer"
	default:
		return ""
	}
}

func subtitleFromEmbedURL(embedURL string) string {
	parsed, err := url.Parse(embedURL)
	if err != nil {
		return ""
	}
	for key, values := range parsed.Query() {
		switch strings.ToLower(key) {
		case "sub", "caption_1", "c1_file":
			if len(values) > 0 {
				if decoded, err := url.QueryUnescape(values[0]); err == nil {
					return decoded
				}
				return values[0]
			}
		}
	}
	match := subtitleRE.FindStringSubmatch(embedURL)
	if len(match) < 2 {
		return ""
	}
	if decoded, err := url.QueryUnescape(match[1]); err == nil {
		return decoded
	}
	return match[1]
}

func subtitleFromEmbedHTML(html string) string {
	html = strings.TrimSpace(html)
	if html == "" {
		return ""
	}
	if match := embedSubtitleRE.FindStringSubmatch(html); len(match) >= 2 {
		if subtitle := strings.TrimSpace(match[1]); subtitle != "" {
			return subtitle
		}
	}
	if match := embedTrackFileRE.FindStringSubmatch(html); len(match) >= 2 {
		return strings.TrimSpace(match[1])
	}
	return ""
}

func resolveSubtitle(embedURL, html string) string {
	if subtitle := subtitleFromEmbedURL(embedURL); subtitle != "" {
		return subtitle
	}
	return subtitleFromEmbedHTML(html)
}

func hostFromURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return parsed.Host
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
