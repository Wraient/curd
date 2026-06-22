package anineko

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var (
	bibiembMasterRE = regexp.MustCompile(`const src = "(https?://[^"]+/master\.m3u8)"`)
	streamInfRE     = regexp.MustCompile(`(?m)^#EXT-X-STREAM-INF:.*NAME="([^"]+)".*\r?\n([^\r\n#]+)`)
)

var bibiembQualityOrder = []string{"1080p", "720p", "480p", "360p"}

type resolvedStream struct {
	URL      string
	Referrer string
	Subtitle string
}

func resolveBibiemb(embedURL string) (resolvedStream, error) {
	embedURL = strings.TrimSpace(embedURL)
	if embedURL == "" {
		return resolvedStream{}, fmt.Errorf("empty bibiemb embed url")
	}

	html, err := fetchString(embedURL, baseURL+"/")
	if err != nil {
		return resolvedStream{}, err
	}

	match := bibiembMasterRE.FindStringSubmatch(html)
	if len(match) < 2 {
		return resolvedStream{}, fmt.Errorf("bibiemb master m3u8 not found")
	}

	masterURL := match[1]
	variantURL, err := pickBibiembVariant(masterURL, embedURL)
	if err != nil {
		return resolvedStream{}, err
	}

	return resolvedStream{
		URL:      variantURL,
		Referrer: embedURL,
		Subtitle: subtitleFromEmbedURL(embedURL),
	}, nil
}

func pickBibiembVariant(masterURL, referer string) (string, error) {
	playlist, err := fetchString(masterURL, referer)
	if err != nil {
		return "", err
	}

	type variant struct {
		name string
		url  string
	}
	variants := make([]variant, 0)
	for _, match := range streamInfRE.FindAllStringSubmatch(playlist, -1) {
		if len(match) < 3 {
			continue
		}
		variants = append(variants, variant{
			name: strings.TrimSpace(match[1]),
			url:  resolvePlaylistURL(masterURL, strings.TrimSpace(match[2])),
		})
	}
	if len(variants) == 0 {
		return "", fmt.Errorf("no bibiemb variants in master playlist")
	}

	byName := map[string]string{}
	for _, item := range variants {
		byName[strings.ToLower(item.name)] = item.url
	}
	for _, quality := range bibiembQualityOrder {
		if streamURL, ok := byName[quality]; ok {
			return streamURL, nil
		}
	}
	return variants[0].url, nil
}

func resolvePlaylistURL(baseURL, entry string) string {
	if strings.HasPrefix(entry, "http://") || strings.HasPrefix(entry, "https://") {
		return entry
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return entry
	}
	if strings.HasPrefix(entry, "/") {
		parsed.Path = entry
		parsed.RawQuery = ""
		parsed.Fragment = ""
		return parsed.Scheme + "://" + parsed.Host + entry
	}
	if idx := strings.LastIndex(parsed.Path, "/"); idx >= 0 {
		parsed.Path = parsed.Path[:idx+1] + entry
	} else {
		parsed.Path = "/" + entry
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}
