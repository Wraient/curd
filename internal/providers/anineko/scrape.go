package anineko

import (
	"net/url"
	"regexp"
	"strings"
)

var (
	dataVideoRE = regexp.MustCompile(`data-video="([^"]+)"`)
	subtitleRE  = regexp.MustCompile(`[?&](?:sub|caption_1|c1_file)=([^&"]+)`)
)

func extractLangEmbedURLs(html string) map[string][]string {
	groups := map[string][]string{
		"hsub": {},
		"sub":  {},
		"dub":  {},
	}
	markers := regexp.MustCompile(`data-id="(hsub|sub|dub)"`).FindAllStringSubmatchIndex(html, -1)
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

func partitionByHost(embedURLs []string) (bibiemb, vibeplayer, other []string) {
	for _, embedURL := range embedURLs {
		host := strings.ToLower(hostFromURL(embedURL))
		switch {
		case strings.Contains(host, "bibiemb."):
			bibiemb = append(bibiemb, embedURL)
		case strings.Contains(host, "vibeplayer."):
			vibeplayer = append(vibeplayer, embedURL)
		default:
			other = append(other, embedURL)
		}
	}
	return bibiemb, vibeplayer, other
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
