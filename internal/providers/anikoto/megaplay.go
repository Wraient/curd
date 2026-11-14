package anikoto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	megaPlayAESKey      = "i?LMTAx0Q6,:}50U"
	megaPlayAESIV       = "W0;27ToaUpl_P%'c"
	megaPlayTokenSecret = "MpCdnT0k3n!9f2K#xQ7vL5mR8wN1pY4s"
)

var (
	megaPlayMediaIDPattern = regexp.MustCompile(`(?i)data-id\s*=\s*["']([^"']+)["']`)
	megaPlayFileIDPattern  = regexp.MustCompile(`(?i)File\s+(\d+)`)
	megaPlayFilePattern    = regexp.MustCompile(`(?i)"file"\s*:\s*"([^"]+)"`)
	megaPlayURLPattern     = regexp.MustCompile(`https?://[^"\s]+`)
	megaPlayPathKeyPattern = regexp.MustCompile(`(?i)/([a-f0-9]{32})/([a-f0-9]{32})/`)
)

func resolveMegaPlay(embedURL, mode string) (resolvedStream, error) {
	embedURL = strings.TrimSpace(embedURL)
	parsed, err := url.Parse(embedURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return resolvedStream{}, fmt.Errorf("invalid Anikoto MegaPlay URL %q", embedURL)
	}

	page, err := fetchResource(embedURL, baseURL+"/", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8", true)
	if err != nil {
		return resolvedStream{}, err
	}
	mediaID := ""
	if mediaIDMatch := megaPlayMediaIDPattern.FindSubmatch(page); len(mediaIDMatch) > 1 {
		mediaID = strings.TrimSpace(string(mediaIDMatch[1]))
	}
	if mediaID == "" {
		if mediaIDMatch := megaPlayFileIDPattern.FindSubmatch(page); len(mediaIDMatch) > 1 {
			mediaID = strings.TrimSpace(string(mediaIDMatch[1]))
		}
	}
	if mediaID == "" {
		return resolvedStream{}, fmt.Errorf("Anikoto MegaPlay media ID not found")
	}

	endpoint := *parsed
	endpoint.Path = "/stream/getSources"
	endpoint.RawPath = ""
	query := endpoint.Query()
	query.Set("id", mediaID)
	endpoint.RawQuery = query.Encode()
	raw, err := fetchResource(endpoint.String(), embedURL, "application/json,*/*", true)
	if err != nil {
		return resolvedStream{}, err
	}
	var payload megaPlaySourcesResponse
	if err := decodeJSON(raw, &payload); err != nil {
		return resolvedStream{}, err
	}
	streamURL, err := resolveMegaPlaySource(payload)
	if err != nil {
		return resolvedStream{}, err
	}
	referrer := parsed.Scheme + "://" + parsed.Host + "/"
	return resolvedStream{
		URL:      streamURL,
		Referrer: referrer,
		Subtitle: pickMegaPlaySubtitle(payload.Tracks, mode),
	}, nil
}

func resolveMegaPlaySource(payload megaPlaySourcesResponse) (string, error) {
	source := strings.TrimSpace(string(payload.Sources))
	decrypted := false
	if strings.TrimSpace(payload.Enc) != "" {
		plain, err := decryptMegaPlaySource(payload.Enc)
		if err != nil {
			if source == "" {
				return "", fmt.Errorf("decrypt Anikoto MegaPlay source: %w", err)
			}
		} else if file := extractMegaPlayFile(plain); file != "" {
			source = file
			decrypted = true
		}
	}
	if source == "" {
		return "", fmt.Errorf("Anikoto MegaPlay source is missing")
	}
	if !decrypted {
		return source, nil
	}
	return signMegaPlayURL(source)
}

func decryptMegaPlaySource(encoded string) (string, error) {
	data, err := decodeMegaPlayBase64(encoded)
	if err != nil {
		return "", err
	}
	if len(data) == 0 || len(data)%aes.BlockSize != 0 {
		return "", fmt.Errorf("encrypted Anikoto MegaPlay source has invalid length %d", len(data))
	}
	key := make([]byte, 32)
	copy(key, []byte(megaPlayAESKey))
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	decrypted := make([]byte, len(data))
	cipher.NewCBCDecrypter(block, []byte(megaPlayAESIV)).CryptBlocks(decrypted, data)
	unpadded, err := unpadPKCS7(decrypted, aes.BlockSize)
	if err != nil {
		return "", err
	}
	return string(unpadded), nil
}

func unpadPKCS7(data []byte, blockSize int) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty Anikoto MegaPlay plaintext")
	}
	padding := int(data[len(data)-1])
	if padding == 0 || padding > blockSize || padding > len(data) {
		return nil, fmt.Errorf("invalid Anikoto MegaPlay padding")
	}
	for _, value := range data[len(data)-padding:] {
		if int(value) != padding {
			return nil, fmt.Errorf("invalid Anikoto MegaPlay padding")
		}
	}
	return data[:len(data)-padding], nil
}

func decodeMegaPlayBase64(encoded string) ([]byte, error) {
	encoded = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(encoded, "-", "+"), "_", "/"))
	encodings := []*base64.Encoding{base64.RawStdEncoding, base64.StdEncoding, base64.RawURLEncoding, base64.URLEncoding}
	var lastErr error
	for _, encoding := range encodings {
		data, err := encoding.DecodeString(encoded)
		if err == nil {
			return data, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func extractMegaPlayFile(plain string) string {
	plain = strings.TrimSpace(plain)
	if strings.HasPrefix(plain, "http://") || strings.HasPrefix(plain, "https://") {
		return plain
	}
	var object struct {
		File string `json:"file"`
	}
	if err := json.Unmarshal([]byte(plain), &object); err == nil && strings.TrimSpace(object.File) != "" {
		return strings.TrimSpace(object.File)
	}
	if match := megaPlayFilePattern.FindStringSubmatch(plain); len(match) > 1 {
		return strings.TrimSpace(match[1])
	}
	if match := megaPlayURLPattern.FindString(plain); match != "" {
		return strings.TrimSpace(match)
	}
	return ""
}

func signMegaPlayURL(rawURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid Anikoto MegaPlay stream URL %q", rawURL)
	}
	query := parsed.Query()
	if query.Get("token") != "" {
		return parsed.String(), nil
	}
	match := megaPlayPathKeyPattern.FindStringSubmatch(parsed.Path)
	if len(match) < 3 {
		return parsed.String(), nil
	}
	payload := fmt.Sprintf("%d|%s/%s", time.Now().Unix()+90, strings.ToLower(match[1]), strings.ToLower(match[2]))
	mac := hmac.New(sha256.New, []byte(megaPlayTokenSecret))
	_, _ = mac.Write([]byte(payload))
	token := base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	query.Set("token", token)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func pickMegaPlaySubtitle(tracks []megaPlayTrack, mode string) string {
	if mode == "dub" {
		return ""
	}
	fallback := ""
	for _, track := range tracks {
		file := strings.TrimSpace(track.File)
		if file == "" || !strings.EqualFold(strings.TrimSpace(track.Kind), "captions") {
			continue
		}
		label := strings.ToLower(strings.TrimSpace(track.Label))
		if track.Default && (label == "" || strings.Contains(label, "english") || strings.Contains(label, "eng")) {
			return file
		}
		if fallback == "" {
			fallback = file
		}
	}
	for _, track := range tracks {
		label := strings.ToLower(strings.TrimSpace(track.Label))
		if strings.TrimSpace(track.File) != "" && strings.EqualFold(strings.TrimSpace(track.Kind), "captions") && strings.Contains(label, "english") {
			return strings.TrimSpace(track.File)
		}
	}
	return fallback
}

func isMegaPlayURL(rawURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return strings.HasPrefix(host, "megaplay.") || strings.Contains(strings.ToLower(parsed.Path), "/stream/")
}
