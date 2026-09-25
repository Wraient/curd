package anipub

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
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
	if streamURL == "" && strings.TrimSpace(payload.Enc) != "" {
		// Megaplay moved the stream URL into an AES-CBC encrypted "enc"
		// field. Decrypt it the same way their web player does.
		decrypted, err := decryptMegaplayEnc(payload.Enc)
		if err != nil {
			return "", "", fmt.Errorf("decrypt megaplay stream: %w", err)
		}
		streamURL = decrypted
	}
	if streamURL == "" {
		return "", "", fmt.Errorf("megaplay stream url missing")
	}
	subtitle := pickSubtitleTrack(payload, mode)
	return streamURL, subtitle, nil
}

// megaplayEncKey and megaplayEncIV mirror the AES-CBC parameters in
// megaplay.buzz's web player (lib/newclient.min.js). If they rotate the
// bundle, this breaks and the values need re-extracting from the new JS.
var (
	megaplayEncKey = []byte("i?LMTAx0Q6,:}50U")
	megaplayEncIV  = []byte("W0;27ToaUpl_P%'c")
)

// decryptMegaplayEnc decrypts the "enc" field of a megaplay getSources
// response and returns the stream file URL inside.
func decryptMegaplayEnc(enc string) (string, error) {
	s := strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(enc), "-", "+"), "_", "/")
	if m := len(s) % 4; m != 0 {
		s += strings.Repeat("=", 4-m)
	}
	ciphertext, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return "", fmt.Errorf("decode megaplay payload: %w", err)
	}
	if len(ciphertext) == 0 || len(ciphertext)%aes.BlockSize != 0 {
		return "", fmt.Errorf("invalid megaplay payload length %d", len(ciphertext))
	}
	key := make([]byte, 32)
	copy(key, megaplayEncKey)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	plain := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, megaplayEncIV).CryptBlocks(plain, ciphertext)
	pad := int(plain[len(plain)-1])
	if pad <= 0 || pad > aes.BlockSize || pad > len(plain) {
		return "", fmt.Errorf("invalid megaplay padding")
	}
	plain = plain[:len(plain)-pad]

	var payload struct {
		File string `json:"file"`
	}
	if err := json.Unmarshal(plain, &payload); err != nil {
		return "", fmt.Errorf("parse decrypted megaplay payload: %w", err)
	}
	if strings.TrimSpace(payload.File) == "" {
		return "", fmt.Errorf("decrypted megaplay payload has no file")
	}
	return strings.TrimSpace(payload.File), nil
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
