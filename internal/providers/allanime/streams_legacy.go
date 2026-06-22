package allanime

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/wraient/curd/internal/curdhost"
	"github.com/wraient/curd/internal/providers"
)

type allanimeSource struct {
	SourceUrl  string  `json:"sourceUrl"`
	SourceName string  `json:"sourceName"`
	Priority   float64 `json:"priority"`
}

type allanimeResponse struct {
	Data struct {
		M          string `json:"_m"`
		Tobeparsed string `json:"tobeparsed"`
		Episode    struct {
			SourceUrls []allanimeSource `json:"sourceUrls"`
		} `json:"episode"`
	} `json:"data"`
}

type allanimeTobeparsedPayload struct {
	Episode struct {
		SourceUrls []allanimeSource `json:"sourceUrls"`
	} `json:"episode"`
}

type filemoonResponse struct {
	IV       string   `json:"iv"`
	Payload  string   `json:"payload"`
	KeyParts []string `json:"key_parts"`
}

func decodeTobeparsed(blob string) ([]allanimeSource, error) {
	plain, err := decryptTobeparsedPlain(blob)
	if err != nil {
		return nil, err
	}

	var payload allanimeTobeparsedPayload
	if err := json.Unmarshal(plain, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse decrypted tobeparsed payload: %w", err)
	}

	if len(payload.Episode.SourceUrls) == 0 {
		return nil, fmt.Errorf("no source URLs found in decrypted tobeparsed payload")
	}

	return payload.Episode.SourceUrls, nil
}

func decryptTobeparsedPlain(blob string) ([]byte, error) {
	key := []byte("Xot36i3lK3:v1")
	hash := sha256.Sum256(key)

	data, err := base64.StdEncoding.DecodeString(blob)
	if err != nil {
		return nil, fmt.Errorf("error decoding base64: %w", err)
	}

	if len(data) < 29 {
		return nil, fmt.Errorf("data too short to contain tobeparsed payload")
	}

	iv := data[1:13]
	ctLen := len(data) - 13 - 16
	if ctLen <= 0 {
		return nil, fmt.Errorf("ciphertext length is invalid in tobeparsed payload")
	}
	ct := data[13 : 13+ctLen]

	ctrIV := make([]byte, 16)
	copy(ctrIV, iv)
	binary.BigEndian.PutUint32(ctrIV[12:], uint32(2))

	block, err := aes.NewCipher(hash[:])
	if err != nil {
		return nil, fmt.Errorf("error creating cipher: %w", err)
	}

	stream := cipher.NewCTR(block, ctrIV)
	plain := make([]byte, len(ct))
	stream.XORKeyStream(plain, ct)
	return plain, nil
}

func decodeProviderID(encoded string) string {
	// Split the string into pairs of characters (.. equivalent of 'sed s/../&\n/g')
	re := regexp.MustCompile("..")
	pairs := re.FindAllString(encoded, -1)

	// Mapping for the replacements
	replacements := map[string]string{
		// Uppercase letters
		"79": "A", "7a": "B", "7b": "C", "7c": "D", "7d": "E", "7e": "F", "7f": "G",
		"70": "H", "71": "I", "72": "J", "73": "K", "74": "L", "75": "M", "76": "N", "77": "O",
		"68": "P", "69": "Q", "6a": "R", "6b": "S", "6c": "T", "6d": "U", "6e": "V", "6f": "W",
		"60": "X", "61": "Y", "62": "Z",
		// Lowercase letters
		"59": "a", "5a": "b", "5b": "c", "5c": "d", "5d": "e", "5e": "f", "5f": "g",
		"50": "h", "51": "i", "52": "j", "53": "k", "54": "l", "55": "m", "56": "n", "57": "o",
		"48": "p", "49": "q", "4a": "r", "4b": "s", "4c": "t", "4d": "u", "4e": "v", "4f": "w",
		"40": "x", "41": "y", "42": "z",
		// Numbers
		"08": "0", "09": "1", "0a": "2", "0b": "3", "0c": "4", "0d": "5", "0e": "6", "0f": "7",
		"00": "8", "01": "9",
		// Special characters
		"15": "-", "16": ".", "67": "_", "46": "~", "02": ":", "17": "/", "07": "?", "1b": "#",
		"63": "[", "65": "]", "78": "@", "19": "!", "1c": "$", "1e": "&", "10": "(", "11": ")",
		"12": "*", "13": "+", "14": ",", "03": ";", "05": "=", "1d": "%",
	}

	// Perform the replacement equivalent to sed 's/^../.../'
	for i, pair := range pairs {
		if val, exists := replacements[pair]; exists {
			pairs[i] = val
		}
	}

	// Join the modified pairs back into a single string
	result := strings.Join(pairs, "")

	// Replace "/clock" with "/clock.json" equivalent of sed "s/\/clock/\/clock\.json/"
	result = strings.ReplaceAll(result, "/clock", "/clock.json")

	// Print the final result
	return result
}

var allanimeNamedProviders = []string{
	"Default",
	"Yt-mp4",
	"S-mp4",
	"Fm-mp4",
	"Luf-Mp4",
}

const (
	allanimePersistedQueryReferer = "https://youtu-chan.com"
	allanimeGraphQLReferer        = "https://allanime.to"
)

func isDirectPlayableAllanimeSource(source allanimeSource) bool {
	sourceURL := strings.TrimSpace(source.SourceUrl)
	if sourceURL == "" {
		return false
	}
	if strings.HasPrefix(sourceURL, "--") {
		return false
	}
	if !strings.HasPrefix(sourceURL, "http://") && !strings.HasPrefix(sourceURL, "https://") {
		return false
	}

	if isUnreliableAllanimeDirectURL(sourceURL) {
		return false
	}

	parsedURL, err := url.Parse(sourceURL)
	if err != nil {
		return false
	}

	path := strings.ToLower(parsedURL.Path)
	if strings.HasSuffix(path, ".mp4") || strings.HasSuffix(path, ".m3u8") {
		return true
	}

	host := strings.ToLower(parsedURL.Host)
	return strings.Contains(host, "sharepoint.com") || strings.Contains(host, "wixmp.com")
}

func isUnreliableAllanimeDirectURL(sourceURL string) bool {
	lowerURL := strings.ToLower(strings.TrimSpace(sourceURL))
	return strings.Contains(lowerURL, "tools.fast4speed.rsvp/")
}

func sortAllanimeSourcesByPriority(sourceUrls []allanimeSource) []allanimeSource {
	if len(sourceUrls) == 0 {
		return nil
	}

	sorted := append([]allanimeSource(nil), sourceUrls...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Priority > sorted[j].Priority
	})

	return sorted
}

func normalizeAllanimeProviderPath(providerID string) string {
	const allanimePrefix = "/https://allanime.day"

	if strings.HasPrefix(providerID, allanimePrefix) {
		trimmed := strings.TrimPrefix(providerID, allanimePrefix)
		if trimmed == "" {
			return "/"
		}
		if !strings.HasPrefix(trimmed, "/") {
			return "/" + trimmed
		}
		return trimmed
	}

	return providerID
}

func decodeBase64URLRaw(input string) ([]byte, error) {
	if decoded, err := base64.RawURLEncoding.DecodeString(input); err == nil {
		return decoded, nil
	}
	return base64.URLEncoding.DecodeString(input)
}

func extractFilemoonLinks(videoData map[string]interface{}) []string {
	raw, err := json.Marshal(videoData)
	if err != nil {
		curdhost.Log(fmt.Sprintf("Error marshaling filemoon payload: %v", err))
		return nil
	}

	var payload filemoonResponse
	if err := json.Unmarshal(raw, &payload); err != nil {
		curdhost.Log(fmt.Sprintf("Error parsing filemoon payload: %v", err))
		return nil
	}

	if payload.IV == "" || payload.Payload == "" || len(payload.KeyParts) < 2 {
		return nil
	}

	keyPart1, err := decodeBase64URLRaw(payload.KeyParts[0])
	if err != nil {
		curdhost.Log(fmt.Sprintf("Error decoding filemoon key part 1: %v", err))
		return nil
	}

	keyPart2, err := decodeBase64URLRaw(payload.KeyParts[1])
	if err != nil {
		curdhost.Log(fmt.Sprintf("Error decoding filemoon key part 2: %v", err))
		return nil
	}

	iv, err := decodeBase64URLRaw(payload.IV)
	if err != nil {
		curdhost.Log(fmt.Sprintf("Error decoding filemoon IV: %v", err))
		return nil
	}

	ciphertext, err := decodeBase64URLRaw(payload.Payload)
	if err != nil {
		curdhost.Log(fmt.Sprintf("Error decoding filemoon ciphertext: %v", err))
		return nil
	}

	if len(ciphertext) <= 16 {
		curdhost.Log("Filemoon ciphertext is too short")
		return nil
	}

	// Match jerry.sh behavior: decrypt all bytes except the final 16-byte trailer.
	ciphertext = ciphertext[:len(ciphertext)-16]

	keyHex := hex.EncodeToString(append(keyPart1, keyPart2...))
	key, err := hex.DecodeString(keyHex)
	if err != nil {
		curdhost.Log(fmt.Sprintf("Error decoding filemoon key hex: %v", err))
		return nil
	}

	ctrIVHex := hex.EncodeToString(iv) + "00000002"
	ctrIV, err := hex.DecodeString(ctrIVHex)
	if err != nil {
		curdhost.Log(fmt.Sprintf("Error decoding filemoon CTR IV: %v", err))
		return nil
	}

	if len(ctrIV) != aes.BlockSize {
		curdhost.Log(fmt.Sprintf("Invalid filemoon CTR IV size: %d", len(ctrIV)))
		return nil
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		curdhost.Log(fmt.Sprintf("Error creating filemoon cipher: %v", err))
		return nil
	}

	plain := make([]byte, len(ciphertext))
	cipher.NewCTR(block, ctrIV).XORKeyStream(plain, ciphertext)

	decoded := strings.ReplaceAll(string(plain), `\u0026`, "&")
	decoded = strings.ReplaceAll(decoded, `\u003D`, "=")

	type qualityLink struct {
		height int
		link   string
	}

	collected := make([]qualityLink, 0)
	seen := make(map[string]struct{})

	reURLFirst := regexp.MustCompile(`"url":"([^"]+)".*?"height":([0-9]+)`)
	reHeightFirst := regexp.MustCompile(`"height":([0-9]+).*?"url":"([^"]+)"`)

	for _, match := range reURLFirst.FindAllStringSubmatch(decoded, -1) {
		if len(match) != 3 {
			continue
		}
		height, err := strconv.Atoi(match[2])
		if err != nil {
			continue
		}
		if _, exists := seen[match[1]]; exists {
			continue
		}
		seen[match[1]] = struct{}{}
		collected = append(collected, qualityLink{height: height, link: match[1]})
	}

	for _, match := range reHeightFirst.FindAllStringSubmatch(decoded, -1) {
		if len(match) != 3 {
			continue
		}
		height, err := strconv.Atoi(match[1])
		if err != nil {
			continue
		}
		if _, exists := seen[match[2]]; exists {
			continue
		}
		seen[match[2]] = struct{}{}
		collected = append(collected, qualityLink{height: height, link: match[2]})
	}

	sort.Slice(collected, func(i, j int) bool {
		return collected[i].height > collected[j].height
	})

	links := make([]string, 0, len(collected))
	for _, entry := range collected {
		links = append(links, entry.link)
	}

	if len(links) > 0 {
		curdhost.Log("Filemoon links fetched")
	}

	return links
}

// Get anime episode url respective to given config
// If the link is found, it returns a list of links. Otherwise, it returns an error.
//
// Parameters:
// - config: Configuration of the anime search.
// - id: Allanime id of the anime to search for.
// - epNo: Anime episode number to get links for.
//
// Returns:
// - []string: a list of links for specified episode.
// - error: an error if the episode is not found or if there is an issue during the search.
func getAllanimeEpisodeURL(config providers.PlaybackConfig, id string, epNo int) ([]string, error) {
	preferredMode := providers.NormalizeTranslationType(config.SubOrDub)
	return getAllanimeEpisodeURLForMode(id, preferredMode, epNo)
}

func getAllanimeEpisodeURLForMode(id, mode string, epNo int) ([]string, error) {
	return getEpisodeURLForMode(id, providers.NormalizeTranslationType(mode), epNo)
}

func fetchAllanimeEpisodeSources(id, mode string, epNo int) ([]allanimeSource, error) {
	return fetchEpisodeSourcesForMode(id, providers.NormalizeTranslationType(mode), epNo)
}

func fetchEpisodeSourcesForMode(id, mode string, epNo int) ([]allanimeSource, error) {
	const (
		episodeQueryHash = "d405d0edd690624b66baba3068e0edc3ac90f1597d898a1ec8db4e5c43c00fec"
	)

	episodeEmbedGQL := `query ($showId: String!, $translationType: VaildTranslationTypeEnumType!, $episodeString: String!) { episode( showId: $showId translationType: $translationType episodeString: $episodeString ) { episodeString sourceUrls }}`

	variables := map[string]interface{}{
		"showId":          id,
		"translationType": providers.NormalizeTranslationType(mode),
		"episodeString":   fmt.Sprintf("%d", epNo),
	}

	extensions := map[string]interface{}{
		"persistedQuery": map[string]interface{}{
			"version":    1,
			"sha256Hash": episodeQueryHash,
		},
	}

	variablesJSON, err := json.Marshal(variables)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal persisted query variables: %w", err)
	}

	extensionsJSON, err := json.Marshal(extensions)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal persisted query extensions: %w", err)
	}

	persistedURL := "https://api.allanime.day/api?variables=" + url.QueryEscape(string(variablesJSON)) + "&extensions=" + url.QueryEscape(string(extensionsJSON))

	req, err := http.NewRequest("GET", persistedURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create persisted query request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:109.0) Gecko/20100101 Firefox/121.0")
	req.Header.Set("Referer", allanimePersistedQueryReferer)
	req.Header.Set("Origin", allanimePersistedQueryReferer)

	resp, err := curdhost.HTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send persisted query request: %w", err)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("failed to read persisted query response: %w", err)
	}
	if !curdhost.HTTPStatusOK(resp.StatusCode) {
		return nil, curdhost.HTTPStatusError("allanime episode persisted query", resp.StatusCode, body)
	}

	var response allanimeResponse
	if err := json.Unmarshal(body, &response); err != nil {
		curdhost.Log(fmt.Sprint("Error parsing persisted query JSON: ", err))
	}

	useFallback := response.Data.Tobeparsed == "" && len(response.Data.Episode.SourceUrls) == 0
	if !useFallback && response.Data.Tobeparsed == "" && !strings.Contains(string(body), "tobeparsed") {
		useFallback = true
	}
	if useFallback {
		query := episodeEmbedGQL

		// Build POST request body
		requestBody, err := json.Marshal(map[string]interface{}{
			"query":     query,
			"variables": variables,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}

		req, err := http.NewRequest("POST", "https://api.allanime.day/api", bytes.NewBuffer(requestBody))
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:109.0) Gecko/20100101 Firefox/121.0")
		req.Header.Set("Referer", allanimeGraphQLReferer)
		req.Header.Set("Origin", allanimeGraphQLReferer)

		resp, err := curdhost.HTTPClient().Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to send request: %w", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read response body: %w", err)
		}
		if !curdhost.HTTPStatusOK(resp.StatusCode) {
			return nil, curdhost.HTTPStatusError("allanime episode fallback query", resp.StatusCode, body)
		}

		if err := json.Unmarshal(body, &response); err != nil {
			curdhost.Log(fmt.Sprint("Error parsing fallback JSON: ", err))
			return nil, err
		}
	}

	if response.Data.Tobeparsed != "" {
		curdhost.Log("Found tobeparsed field, decoding source URLs")
		decodedSources, err := decodeTobeparsed(response.Data.Tobeparsed)
		if err != nil {
			return nil, err
		}

		curdhost.Log(fmt.Sprintf("Decoded %d Allanime source URLs from tobeparsed payload", len(decodedSources)))
		return decodedSources, nil
	}

	return response.Data.Episode.SourceUrls, nil
}

func getEpisodeURLForMode(id, mode string, epNo int) ([]string, error) {
	links, _, err := getAllanimeEpisodeStreamsForMode(id, mode, epNo)
	return links, err
}

func findNamedAllanimeSource(sourceUrls []allanimeSource, providerName string) (allanimeSource, bool) {
	for _, source := range sourceUrls {
		if strings.EqualFold(strings.TrimSpace(source.SourceName), providerName) {
			return source, true
		}
	}
	return allanimeSource{}, false
}

var wixstaticURLSetPattern = regexp.MustCompile(`^(https://video\.wixstatic\.com/video/[^/,]+/),([^/]*),/mp4/file\.mp4`)

func expandWixmpLinks(link string) []string {
	normalized := strings.TrimSpace(link)
	if normalized == "" {
		return nil
	}

	if strings.Contains(normalized, "repackager.wixmp.com") {
		normalized = strings.TrimPrefix(normalized, "https://repackager.wixmp.com/")
		normalized = strings.TrimPrefix(normalized, "http://repackager.wixmp.com/")
		if !strings.HasPrefix(normalized, "http") {
			normalized = "https://" + normalized
		}
	}

	if idx := strings.Index(normalized, ".urlset"); idx != -1 {
		normalized = normalized[:idx]
	}

	if match := wixstaticURLSetPattern.FindStringSubmatch(normalized); len(match) == 3 {
		base := match[1]
		qualities := strings.Split(match[2], ",")
		expanded := make([]string, 0, len(qualities))
		for _, quality := range qualities {
			quality = strings.TrimSpace(quality)
			if quality == "" {
				continue
			}
			expanded = append(expanded, base+quality+"/mp4/file.mp4")
		}
		if len(expanded) > 0 {
			sort.Slice(expanded, func(i, j int) bool {
				return wixmpQualityScore(expanded[i]) > wixmpQualityScore(expanded[j])
			})
			return expanded
		}
	}

	return []string{link}
}

func wixmpQualityScore(link string) int {
	for _, quality := range []string{"1080p", "720p", "480p", "360p", "240p"} {
		if strings.Contains(link, quality) {
			switch quality {
			case "1080p":
				return 1080
			case "720p":
				return 720
			case "480p":
				return 480
			case "360p":
				return 360
			case "240p":
				return 240
			}
		}
	}
	return 0
}
