package senshi

import (
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/wraient/curd/internal/curdhost"
)

const maxSenshiSubtitleSize = 8 << 20

var senshiVTTTimingLine = regexp.MustCompile(`^\s*(?:(?:\d{2}:)?\d{2}:\d{2}\.\d{3})\s+-->\s+(?:(?:\d{2}:)?\d{2}:\d{2}\.\d{3})(?:\s|$)`)

type senshiSubtitleTrack struct {
	Src     string `json:"src"`
	Label   string `json:"label"`
	Default bool   `json:"default"`
}

func senshiSubtitleManifestURL(item embedItem) string {
	if item.ServerFM != nil {
		if manifest := subtitleInfoFromURL(strings.TrimSpace(*item.ServerFM)); manifest != "" {
			return manifest
		}
	}
	base := strings.TrimSpace(item.MaskedBaseURL)
	if base == "" {
		return ""
	}
	return strings.TrimRight(base, "/") + "/sub_filemoon.json"
}

func subtitleInfoFromURL(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(parsed.Query().Get("sub.info"))
}

func fetchSenshiSubtitle(manifestURL string) (string, error) {
	manifestURL = strings.TrimSpace(manifestURL)
	if manifestURL == "" {
		return "", nil
	}

	var tracks []senshiSubtitleTrack
	if err := fetchJSON(http.MethodGet, manifestURL, nil, &tracks); err != nil {
		return "", err
	}
	return pickSenshiSubtitleTrack(tracks), nil
}

func pickSenshiSubtitleTrack(tracks []senshiSubtitleTrack) string {
	// Senshi can put a forced/signs-only English track before the full dialogue
	// track. Prefer a non-forced default before considering label fallbacks.
	for _, track := range tracks {
		file := strings.TrimSpace(track.Src)
		if file != "" && track.Default && !isSenshiForcedSubtitleLabel(track.Label) {
			return file
		}
	}
	for _, track := range tracks {
		file := strings.TrimSpace(track.Src)
		label := strings.ToLower(strings.TrimSpace(track.Label))
		if file != "" && strings.Contains(label, "eng") && !isSenshiForcedSubtitleLabel(label) {
			return file
		}
	}
	for _, track := range tracks {
		file := strings.TrimSpace(track.Src)
		if file != "" && track.Default {
			return file
		}
	}
	for _, track := range tracks {
		file := strings.TrimSpace(track.Src)
		label := strings.ToLower(strings.TrimSpace(track.Label))
		if file != "" && strings.Contains(label, "eng") {
			return file
		}
	}
	for _, track := range tracks {
		if file := strings.TrimSpace(track.Src); file != "" {
			return file
		}
	}
	return ""
}

func isSenshiForcedSubtitleLabel(label string) bool {
	label = strings.ToLower(strings.TrimSpace(label))
	return strings.Contains(label, "forced") ||
		strings.Contains(label, "sign") ||
		strings.Contains(label, "song")
}

func resolveSenshiSubtitle(item embedItem) string {
	manifestURL := senshiSubtitleManifestURL(item)
	if manifestURL == "" {
		return ""
	}
	subtitle, err := fetchSenshiSubtitle(manifestURL)
	if err != nil {
		return ""
	}
	return prepareSenshiSubtitle(subtitle)
}

func prepareSenshiSubtitle(subtitleURL string) string {
	subtitleURL = strings.TrimSpace(subtitleURL)
	if subtitleURL == "" {
		return ""
	}
	if styled := validatedSenshiASSURL(subtitleURL); styled != "" {
		return styled
	}
	if local, err := cacheSanitizedSenshiVTT(subtitleURL, senshiSubtitleCacheDir()); err == nil {
		return local
	}
	return subtitleURL
}

func validatedSenshiASSURL(subtitleURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(subtitleURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || !strings.EqualFold(filepath.Ext(parsed.Path), ".vtt") {
		return ""
	}
	parsed.Path = strings.TrimSuffix(parsed.Path, filepath.Ext(parsed.Path)) + ".ass"
	candidate := parsed.String()

	req, err := newRequest(http.MethodGet, candidate)
	if err != nil {
		return ""
	}
	req.Header.Set("Range", "bytes=0-8191")
	resp, err := curdhost.HTTPClient().Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return ""
	}
	prefix, err := io.ReadAll(io.LimitReader(resp.Body, 8192))
	if err != nil {
		return ""
	}
	text := strings.TrimSpace(strings.TrimPrefix(string(prefix), "\ufeff"))
	if !strings.HasPrefix(text, "[Script Info]") || !strings.Contains(text, "[V4+ Styles]") {
		return ""
	}
	return candidate
}

func senshiSubtitleCacheDir() string {
	return filepath.Join(os.TempDir(), "curd", "subtitles")
}

func cacheSanitizedSenshiVTT(subtitleURL, cacheDir string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(subtitleURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || !strings.EqualFold(filepath.Ext(parsed.Path), ".vtt") {
		return "", fmt.Errorf("not a remote WebVTT subtitle")
	}
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		return "", err
	}
	cleanupOldSenshiSubtitles(cacheDir)

	cacheKey := fmt.Sprintf("%x", sha256.Sum256([]byte(subtitleURL)))
	cachePath := filepath.Join(cacheDir, cacheKey+".vtt")
	if info, statErr := os.Stat(cachePath); statErr == nil && info.Size() > 0 && time.Since(info.ModTime()) < 12*time.Hour {
		return cachePath, nil
	}

	req, err := newRequest(http.MethodGet, subtitleURL)
	if err != nil {
		return "", err
	}
	resp, err := curdhost.HTTPClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("subtitle request failed with status %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxSenshiSubtitleSize+1))
	if err != nil {
		return "", err
	}
	if len(raw) > maxSenshiSubtitleSize {
		return "", fmt.Errorf("subtitle is too large")
	}
	sanitized, changed := sanitizeSenshiWebVTT(raw)
	if !changed {
		return subtitleURL, nil
	}
	tempPath := cachePath + ".tmp"
	if err := os.WriteFile(tempPath, sanitized, 0o600); err != nil {
		return "", err
	}
	if err := os.Rename(tempPath, cachePath); err != nil {
		_ = os.Remove(tempPath)
		return "", err
	}
	return cachePath, nil
}

func sanitizeSenshiWebVTT(raw []byte) ([]byte, bool) {
	text := strings.TrimPrefix(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\ufeff")
	lines := strings.Split(text, "\n")
	timingIndexes := make([]int, 0)
	for index, line := range lines {
		if senshiVTTTimingLine.MatchString(strings.TrimSpace(line)) {
			timingIndexes = append(timingIndexes, index)
		}
	}
	if len(timingIndexes) == 0 {
		return raw, false
	}

	changed := strings.Contains(text, "\\h")
	out := []string{"WEBVTT", ""}
	for cueIndex, timingIndex := range timingIndexes {
		end := len(lines)
		if cueIndex+1 < len(timingIndexes) {
			end = timingIndexes[cueIndex+1]
		}
		content := append([]string(nil), lines[timingIndex+1:end]...)
		for len(content) > 0 && strings.TrimSpace(content[len(content)-1]) == "" {
			content = content[:len(content)-1]
		}
		for len(content) > 0 && strings.TrimSpace(content[0]) == "" {
			content = content[1:]
		}
		if len(content) == 0 {
			continue
		}
		out = append(out, strings.TrimSpace(lines[timingIndex]))
		for _, line := range content {
			line = strings.ReplaceAll(strings.TrimSuffix(line, "\r"), "\\h", "\u00a0")
			if strings.TrimSpace(line) == "" {
				line = "\u00a0"
				changed = true
			}
			out = append(out, line)
		}
		out = append(out, "")
	}
	if !changed {
		return raw, false
	}
	return []byte(strings.Join(out, "\n")), true
}

func cleanupOldSenshiSubtitles(cacheDir string) {
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".vtt") {
			continue
		}
		info, err := entry.Info()
		if err == nil && time.Since(info.ModTime()) > 48*time.Hour {
			_ = os.Remove(filepath.Join(cacheDir, entry.Name()))
		}
	}
}

func isSubEmbedStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "hardsub", "softsub", "sub":
		return true
	default:
		return false
	}
}
