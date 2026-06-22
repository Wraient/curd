package anineko

import (
	"fmt"
	"regexp"
	"strings"
)

var vibeplayerMasterRE = regexp.MustCompile(`const src = "(https://vibeplayer\.site/public/stream/[^"]+/master\.m3u8)"`)

func resolveVibeplayer(embedURL string) (resolvedStream, error) {
	embedURL = strings.TrimSpace(embedURL)
	if embedURL == "" {
		return resolvedStream{}, fmt.Errorf("empty vibeplayer embed url")
	}

	html, err := fetchString(embedURL, baseURL+"/")
	if err != nil {
		return resolvedStream{}, err
	}

	match := vibeplayerMasterRE.FindStringSubmatch(html)
	if len(match) < 2 {
		return resolvedStream{}, fmt.Errorf("vibeplayer master m3u8 not found")
	}

	proxyURL, err := registerVibeStream(match[1], embedURL)
	if err != nil {
		return resolvedStream{}, err
	}

	return resolvedStream{
		URL:      proxyURL,
		Referrer: embedURL,
		Subtitle: subtitleFromEmbedURL(embedURL),
	}, nil
}
