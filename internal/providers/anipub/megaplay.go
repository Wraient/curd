package anipub

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/wraient/curd/internal/providers"
)

var (
	videoPathRE = regexp.MustCompile(`/video/(\d+)/(sub|dub)`)
	dataIDRE    = regexp.MustCompile(`data-id="(\d+)"`)
)

func resolveMegaplayStream(videoLink, mode string) (string, string, error) {
	videoLink = strings.TrimSpace(videoLink)
	if videoLink == "" {
		return "", "", fmt.Errorf("empty video link")
	}

	embedID, linkMode, err := parseVideoLink(videoLink)
	if err != nil {
		return "", "", err
	}
	mode = providers.NormalizeTranslationType(mode)
	if mode == "dub" {
		linkMode = "dub"
	} else {
		linkMode = "sub"
	}

	streamPage := fmt.Sprintf("%s/stream/s-2/%s/%s", megaplayBaseURL, embedID, linkMode)
	html, err := fetchString(streamPage, baseURL+"/")
	if err != nil {
		return "", "", err
	}

	dataID := dataIDRE.FindStringSubmatch(html)
	if len(dataID) < 2 {
		return "", "", fmt.Errorf("megaplay data-id not found")
	}

	sourcesURL := fmt.Sprintf("%s/stream/getSources?id=%s", megaplayBaseURL, dataID[1])
	var payload megaplaySourcesResponse
	if err := fetchJSON(sourcesURL, streamPage, &payload); err != nil {
		return "", "", err
	}

	streamURL := strings.TrimSpace(payload.Sources.File)
	if streamURL == "" {
		return "", "", fmt.Errorf("megaplay stream url missing")
	}
	subtitle := pickSubtitleTrack(payload, mode)
	return streamURL, subtitle, nil
}

func parseVideoLink(videoLink string) (embedID, mode string, err error) {
	parsed, err := url.Parse(videoLink)
	if err != nil {
		return "", "", fmt.Errorf("parse video link: %w", err)
	}
	matches := videoPathRE.FindStringSubmatch(parsed.Path)
	if len(matches) < 3 {
		return "", "", fmt.Errorf("unsupported video link %q", videoLink)
	}
	embedID = matches[1]
	mode = matches[2]
	if _, err := strconv.Atoi(embedID); err != nil {
		return "", "", fmt.Errorf("invalid embed id %q", embedID)
	}
	return embedID, mode, nil
}

func pickSubtitleTrack(payload megaplaySourcesResponse, mode string) string {
	if mode == "dub" {
		return ""
	}
	var fallback string
	for _, track := range payload.Tracks {
		file := strings.TrimSpace(track.File)
		if file == "" || !strings.EqualFold(strings.TrimSpace(track.Kind), "captions") {
			continue
		}
		label := strings.ToLower(strings.TrimSpace(track.Label))
		if track.Default || strings.Contains(label, "english") {
			return file
		}
		if fallback == "" {
			fallback = file
		}
	}
	return fallback
}
