package internal

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
	"time"
	"unicode"
)

type allanimeResponse struct {
	Data struct {
		M          string `json:"_m"`
		Tobeparsed string `json:"tobeparsed"`
		Episode    struct {
			SourceUrls []struct {
				SourceUrl  string `json:"sourceUrl"`
				SourceName string `json:"sourceName"`
			} `json:"sourceUrls"`
		} `json:"episode"`
	} `json:"data"`
}

type result struct {
	index int
	links []string
	err   error
}

type filemoonResponse struct {
	IV       string   `json:"iv"`
	Payload  string   `json:"payload"`
	KeyParts []string `json:"key_parts"`
}

func decodeTobeparsed(blob string) string {
	key := []byte("Xot36i3lK3:v1")
	hash := sha256.Sum256(key)

	data, err := base64.StdEncoding.DecodeString(blob)
	if err != nil {
		Log(fmt.Sprint("Error decoding base64:", err))
		return ""
	}

	if len(data) < 29 {
		Log("Data too short to contain tobeparsed payload")
		return ""
	}

	// The payload format is: 1-byte header, 12-byte IV, ciphertext, 16-byte trailer.
	iv := data[1:13]
	ctLen := len(data) - 13 - 16
	if ctLen <= 0 {
		Log("Ciphertext length is invalid in tobeparsed payload")
		return ""
	}
	ct := data[13 : 13+ctLen]

	ctrIV := make([]byte, 16)
	copy(ctrIV, iv)
	binary.BigEndian.PutUint32(ctrIV[12:], uint32(2))

	block, err := aes.NewCipher(hash[:])
	if err != nil {
		Log(fmt.Sprint("Error creating cipher:", err))
		return ""
	}

	stream := cipher.NewCTR(block, ctrIV)
	plain := make([]byte, len(ct))
	stream.XORKeyStream(plain, ct)

	result := string(plain)
	result = strings.ReplaceAll(result, "{", "\n")
	result = strings.ReplaceAll(result, "}", "\n")

	re := regexp.MustCompile(`"sourceUrl":"--([^"]+)".*"sourceName":"([^"]+)"`)
	matches := re.FindAllStringSubmatch(result, -1)

	var sb strings.Builder
	for _, match := range matches {
		if len(match) == 3 {
			sb.WriteString(match[2])
			sb.WriteString(" :")
			sb.WriteString(match[1])
			sb.WriteString("\n")
		}
	}

	return sb.String()
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

func extractLinks(provider_id string) map[string]interface{} {
	provider_id = normalizeAllanimeProviderPath(provider_id)

	// Check if provider_id is already a full URL (external link)
	if strings.HasPrefix(provider_id, "http://") || strings.HasPrefix(provider_id, "https://") {
		// It's an external direct video link, preserve it exactly as provided.
		Log(fmt.Sprintf("Direct external link detected: %s", provider_id))
		return map[string]interface{}{
			"links": []interface{}{
				map[string]interface{}{
					"link": provider_id,
				},
			},
		}
	}

	// It's a relative path for allanime API
	allanime_base := "https://allanime.day"
	url := allanime_base + provider_id
	req, err := http.NewRequest("GET", url, nil)
	var videoData map[string]interface{}
	if err != nil {
		Log(fmt.Sprint("Error creating request:", err))
		return videoData
	}

	// Add the headers
	req.Header.Set("Referer", "https://allanime.to")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:109.0) Gecko/20100101 Firefox/121.0")

	// Send the request
	resp, err := sharedHTTPClient.Do(req)
	if err != nil {
		Log(fmt.Sprint("Error sending request:", err))
		return videoData
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		Log(fmt.Sprint("Error reading response:", err))
		return videoData
	}

	// Parse the JSON response
	err = json.Unmarshal(body, &videoData)
	if err != nil {
		Log(fmt.Sprint("Error parsing JSON:", err))
		return videoData
	}

	// Filemoon extractor payload does not return a top-level "links" field.
	if _, hasLinks := videoData["links"]; !hasLinks {
		if filemoonLinks := extractFilemoonLinks(videoData); len(filemoonLinks) > 0 {
			links := make([]interface{}, 0, len(filemoonLinks))
			for _, link := range filemoonLinks {
				links = append(links, map[string]interface{}{"link": link})
			}
			videoData["links"] = links
		}
	}

	// Process the data as needed
	return videoData
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
		Log(fmt.Sprintf("Error marshaling filemoon payload: %v", err))
		return nil
	}

	var payload filemoonResponse
	if err := json.Unmarshal(raw, &payload); err != nil {
		Log(fmt.Sprintf("Error parsing filemoon payload: %v", err))
		return nil
	}

	if payload.IV == "" || payload.Payload == "" || len(payload.KeyParts) < 2 {
		return nil
	}

	keyPart1, err := decodeBase64URLRaw(payload.KeyParts[0])
	if err != nil {
		Log(fmt.Sprintf("Error decoding filemoon key part 1: %v", err))
		return nil
	}

	keyPart2, err := decodeBase64URLRaw(payload.KeyParts[1])
	if err != nil {
		Log(fmt.Sprintf("Error decoding filemoon key part 2: %v", err))
		return nil
	}

	iv, err := decodeBase64URLRaw(payload.IV)
	if err != nil {
		Log(fmt.Sprintf("Error decoding filemoon IV: %v", err))
		return nil
	}

	ciphertext, err := decodeBase64URLRaw(payload.Payload)
	if err != nil {
		Log(fmt.Sprintf("Error decoding filemoon ciphertext: %v", err))
		return nil
	}

	if len(ciphertext) <= 16 {
		Log("Filemoon ciphertext is too short")
		return nil
	}

	// Match jerry.sh behavior: decrypt all bytes except the final 16-byte trailer.
	ciphertext = ciphertext[:len(ciphertext)-16]

	keyHex := hex.EncodeToString(append(keyPart1, keyPart2...))
	key, err := hex.DecodeString(keyHex)
	if err != nil {
		Log(fmt.Sprintf("Error decoding filemoon key hex: %v", err))
		return nil
	}

	ctrIVHex := hex.EncodeToString(iv) + "00000002"
	ctrIV, err := hex.DecodeString(ctrIVHex)
	if err != nil {
		Log(fmt.Sprintf("Error decoding filemoon CTR IV: %v", err))
		return nil
	}

	if len(ctrIV) != aes.BlockSize {
		Log(fmt.Sprintf("Invalid filemoon CTR IV size: %d", len(ctrIV)))
		return nil
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		Log(fmt.Sprintf("Error creating filemoon cipher: %v", err))
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
		Log("Filemoon links fetched")
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
func getAllanimeEpisodeURL(config CurdConfig, id string, epNo int) ([]string, error) {
	preferredMode := normalizeTranslationType(config.SubOrDub)
	fallbackMode := alternateTranslationType(preferredMode)

	type modeResult struct {
		mode  string
		links []string
		err   error
	}

	ch := make(chan modeResult, 2)

	go func() {
		links, err := getEpisodeURLForMode(id, preferredMode, epNo)
		ch <- modeResult{mode: preferredMode, links: links, err: err}
	}()

	go func() {
		links, err := getEpisodeURLForMode(id, fallbackMode, epNo)
		ch <- modeResult{mode: fallbackMode, links: links, err: err}
	}()

	var preferredRes, fallbackRes modeResult
	hasPreferredRes := false
	hasFallbackRes := false
	for i := 0; i < 2; i++ {
		res := <-ch

		if res.mode == preferredMode {
			preferredRes = res
			hasPreferredRes = true
			if res.err == nil && len(res.links) > 0 {
				return res.links, nil
			}
			continue
		}

		if res.mode == fallbackMode {
			fallbackRes = res
			hasFallbackRes = true
		}
	}

	if hasPreferredRes && preferredRes.err == nil && len(preferredRes.links) > 0 {
		return preferredRes.links, nil
	}
	if hasFallbackRes && fallbackRes.err == nil && len(fallbackRes.links) > 0 {
		Log(fmt.Sprintf("Falling back to %s for anime %s episode %d", fallbackMode, id, epNo))
		return fallbackRes.links, nil
	}

	if hasPreferredRes && preferredRes.err != nil {
		return nil, preferredRes.err
	}
	if hasFallbackRes && fallbackRes.err != nil {
		return nil, fallbackRes.err
	}

	return nil, fmt.Errorf("no valid links found for anime %s episode %d", id, epNo)
}

func getEpisodeURLForMode(id, mode string, epNo int) ([]string, error) {
	const (
		episodeQueryHash = "d405d0edd690624b66baba3068e0edc3ac90f1597d898a1ec8db4e5c43c00fec"
	)

	episodeEmbedGQL := `query ($showId: String!, $translationType: VaildTranslationTypeEnumType!, $episodeString: String!) { episode( showId: $showId translationType: $translationType episodeString: $episodeString ) { episodeString sourceUrls }}`

	variables := map[string]interface{}{
		"showId":          id,
		"translationType": normalizeTranslationType(mode),
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
	req.Header.Set("Referer", "https://allmanga.to")
	req.Header.Set("Origin", "https://youtu-chan.com")

	resp, err := sharedHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send persisted query request: %w", err)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("failed to read persisted query response: %w", err)
	}

	var response allanimeResponse
	if err := json.Unmarshal(body, &response); err != nil {
		Log(fmt.Sprint("Error parsing persisted query JSON: ", err))
	}

	useFallback := response.Data.Tobeparsed == "" && len(response.Data.Episode.SourceUrls) == 0
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
		req.Header.Set("User-Agent", "Mozilla/5.0")
		req.Header.Set("Referer", "https://allmanga.to")
		req.Header.Set("Origin", "https://allanime.to")

		resp, err := sharedHTTPClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to send request: %w", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read response body: %w", err)
		}

		if err := json.Unmarshal(body, &response); err != nil {
			Log(fmt.Sprint("Error parsing fallback JSON: ", err))
			return nil, err
		}
	}

	if response.Data.Tobeparsed != "" {
		Log("Found tobeparsed field, using decoded response")
		decoded := decodeTobeparsed(response.Data.Tobeparsed)
		lines := strings.Split(strings.TrimSpace(decoded), "\n")
		var parsedURLs []struct {
			SourceName string
			SourceUrl  string
		}
		for _, line := range lines {
			if parts := strings.Split(line, " :"); len(parts) == 2 {
				parsedURLs = append(parsedURLs, struct {
					SourceName string
					SourceUrl  string
				}{SourceName: parts[0], SourceUrl: "--" + parts[1]})
			}
		}

		validURLs := make([]string, 0)
		for _, url := range parsedURLs {
			validURLs = append(validURLs, url.SourceUrl)
		}

		if len(validURLs) == 0 {
			return nil, fmt.Errorf("no valid source URLs found in decoded tobeparsed")
		}

		return getLinksFromURLs(validURLs)
	}

	return getLinksFromSourceUrls(response.Data.Episode.SourceUrls)
}

func getLinksFromSourceUrls(sourceUrls []struct {
	SourceUrl  string `json:"sourceUrl"`
	SourceName string `json:"sourceName"`
}) ([]string, error) {
	validURLs := make([]string, 0)
	highestPriority := -1
	var highestPriorityURL string

	for _, url := range sourceUrls {
		if len(url.SourceUrl) > 2 && unicode.IsDigit(rune(url.SourceUrl[2])) {
			decodedURL := decodeProviderID(url.SourceUrl[2:])
			if strings.Contains(decodedURL, LinkPriorities[0]) {
				priority := int(url.SourceUrl[2] - '0')
				if priority > highestPriority {
					highestPriority = priority
					highestPriorityURL = url.SourceUrl
				}
			} else {
				validURLs = append(validURLs, url.SourceUrl)
			}
		}
	}

	if highestPriorityURL != "" {
		validURLs = []string{highestPriorityURL}
	}

	if len(validURLs) == 0 {
		return nil, fmt.Errorf("no valid source URLs found in response")
	}

	return getLinksFromURLs(validURLs)
}

func getLinksFromURLs(validURLs []string) ([]string, error) {
	results := make(chan result, len(validURLs))
	orderedResults := make([][]string, len(validURLs))

	highPriorityLink := make(chan []string, 1)

	remainingURLs := len(validURLs)
	for i, sourceUrl := range validURLs {
		go func(idx int, url string) {
			decodedProviderID := decodeProviderID(url[2:])
			Log(fmt.Sprintf("Processing URL %d/%d with provider ID: %s", idx+1, len(validURLs), decodedProviderID))

			extractedLinks := extractLinks(decodedProviderID)

			if extractedLinks == nil {
				results <- result{
					index: idx,
					err:   fmt.Errorf("failed to extract links for provider %s", decodedProviderID),
				}
				return
			}

			linksInterface, ok := extractedLinks["links"].([]interface{})
			if !ok {
				results <- result{
					index: idx,
					err:   fmt.Errorf("links field is not []interface{} for provider %s", decodedProviderID),
				}
				return
			}

			var links []string
			for _, linkInterface := range linksInterface {
				linkMap, ok := linkInterface.(map[string]interface{})
				if !ok {
					Log(fmt.Sprintf("Warning: invalid link format for provider %s", decodedProviderID))
					continue
				}

				link, ok := linkMap["link"].(string)
				if !ok {
					Log(fmt.Sprintf("Warning: link field is not string for provider %s", decodedProviderID))
					continue
				}

				links = append(links, link)
			}

			// Check if any of the extracted links are high priority
			for _, link := range links {
				for _, domain := range LinkPriorities[:3] {
					if strings.Contains(link, domain) {
						// Found high priority link, send it immediately
						select {
						case highPriorityLink <- []string{link}:
						default:
							// Channel already has a high priority link
						}
						break
					}
				}
			}

			results <- result{
				index: idx,
				links: links,
			}
		}(i, sourceUrl)
	}

	// Collect results with timeout
	timeout := time.After(10 * time.Second)
	var collectedErrors []error
	successCount := 0

	// First, try to get a high priority link
	select {
	case links := <-highPriorityLink:
		// Continue extracting other links in background
		go collectRemainingResults(results, orderedResults, &successCount, &collectedErrors, remainingURLs)
		return links, nil
	case <-time.After(2 * time.Second): // Wait only briefly for high priority link
		// No high priority link found quickly, proceed with normal collection
	}

	// Continue with existing result collection logic
	// Collect results maintaining order
	for successCount < len(validURLs) {
		select {
		case res := <-results:
			if res.err != nil {
				Log(fmt.Sprintf("Error processing URL %d: %v", res.index+1, res.err))
				collectedErrors = append(collectedErrors, fmt.Errorf("URL %d: %w", res.index+1, res.err))
			} else {
				orderedResults[res.index] = res.links
				successCount++
				Log(fmt.Sprintf("Successfully processed URL %d/%d", res.index+1, len(validURLs)))
			}
		case <-timeout:
			if successCount > 0 {
				Log(fmt.Sprintf("Timeout reached with %d/%d successful results", successCount, len(validURLs)))
				// Flatten available results
				return flattenResults(orderedResults), nil
			}
			return nil, fmt.Errorf("timeout waiting for results after %d successful responses", successCount)
		}
	}

	// If we have any errors but also some successes, log errors but continue
	if len(collectedErrors) > 0 {
		Log(fmt.Sprintf("Completed with %d errors: %v", len(collectedErrors), collectedErrors))
	}

	// Flatten and return results
	allLinks := flattenResults(orderedResults)
	if len(allLinks) == 0 {
		return nil, fmt.Errorf("no valid links found from %d URLs: %v", len(validURLs), collectedErrors)
	}

	return allLinks, nil
}

// Helper function to collect remaining results in background
func collectRemainingResults(results chan result, orderedResults [][]string, successCount *int, collectedErrors *[]error, remainingURLs int) {
	for *successCount < remainingURLs {
		select {
		case res := <-results:
			if res.err != nil {
				Log(fmt.Sprintf("Error processing URL %d: %v", res.index+1, res.err))
				*collectedErrors = append(*collectedErrors, fmt.Errorf("URL %d: %w", res.index+1, res.err))
			} else {
				orderedResults[res.index] = res.links
				*successCount++
				Log(fmt.Sprintf("Successfully processed URL %d/%d", res.index+1, remainingURLs))
			}
		case <-time.After(10 * time.Second):
			return
		}
	}
}

// converts the ordered slice of link slices into a single slice
func flattenResults(results [][]string) []string {
	var totalLen int
	for _, r := range results {
		totalLen += len(r)
	}

	allLinks := make([]string, 0, totalLen)
	for _, links := range results {
		allLinks = append(allLinks, links...)
	}
	return allLinks
}
