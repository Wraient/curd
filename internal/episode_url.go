package internal

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"
)

type allanimeResponse struct {
	Data struct {
		Episode struct {
			SourceUrls []struct {
				SourceUrl string `json:"sourceUrl"`
			} `json:"sourceUrls"`
		} `json:"episode"`
	} `json:"data"`
}

func decodeProviderID(encoded string) string {
	// Split the string into pairs of characters (.. equivalent of 'sed s/../&\n/g')
	re := regexp.MustCompile("..")
	pairs := re.FindAllString(encoded, -1)

	// Mapping for the replacements
	replacements := map[string]string{
		"01": "9", "08": "0", "05": "=", "0a": "2", "0b": "3", "0c": "4", "07": "?",
		"00": "8", "5c": "d", "0f": "7", "5e": "f", "17": "/", "54": "l", "09": "1",
		"48": "p", "4f": "w", "0e": "6", "5b": "c", "5d": "e", "0d": "5", "53": "k",
		"1e": "&", "5a": "b", "59": "a", "4a": "r", "4c": "t", "4e": "v", "57": "o",
		"51": "i",
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
	allanime_base := "https://allanime.day"
	url := allanime_base + provider_id
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	var videoData map[string]interface{}
	if err != nil {
		Log(fmt.Sprint("Error creating request:", err), logFile)
		return videoData
	}

	// Add the headers
	req.Header.Set("Referer", "https://allanime.to")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:109.0) Gecko/20100101 Firefox/121.0")

	// Send the request
	resp, err := client.Do(req)
	if err != nil {
		Log(fmt.Sprint("Error sending request:", err), logFile)
		return videoData
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		Log(fmt.Sprint("Error reading response:", err), logFile)
		return videoData
	}

	// Parse the JSON response
	err = json.Unmarshal(body, &videoData)
	if err != nil {
		Log(fmt.Sprint("Error parsing JSON:", err), logFile)
		return videoData
	}

	// Process the data as needed
	return videoData
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
func GetEpisodeURL(config CurdConfig, id string, epNo int) ([]string, error) {
	query := `query($showId:String!,$translationType:VaildTranslationTypeEnumType!,$episodeString:String!){episode(showId:$showId,translationType:$translationType,episodeString:$episodeString){episodeString sourceUrls}}`

	variables := map[string]string{
		"showId":          id,
		"translationType": config.SubOrDub,
		"episodeString":   fmt.Sprintf("%d", epNo),
	}

	variablesJSON, err := json.Marshal(variables)
	if err != nil {
		return nil, err
	}

	values := url.Values{}
	values.Set("query", query)
	values.Set("variables", string(variablesJSON))

	reqURL := fmt.Sprintf("%s/api?%s", "https://api.allanime.day", values.Encode())

	client := &http.Client{}
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:109.0) Gecko/20100101 Firefox/121.0")
	req.Header.Set("Referer", "https://allanime.to")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var response allanimeResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		Log(fmt.Sprint("Error parsing JSON: ", err), logFile)
		return nil, err
	}

	type result struct {
		index int
		links []string
		err   error
	}

	// Pre-count valid URLs and create slice to preserve order
	validURLs := make([]string, 0)
	for _, url := range response.Data.Episode.SourceUrls {
		if len(url.SourceUrl) > 2 && unicode.IsDigit(rune(url.SourceUrl[2])) {
			validURLs = append(validURLs, url.SourceUrl)
		}
	}

	if len(validURLs) == 0 {
		return nil, fmt.Errorf("no valid source URLs found in response")
	}

	// Create channels for results and a slice to store ordered results
	results := make(chan result, len(validURLs))
	orderedResults := make([][]string, len(validURLs))

	// Create rate limiter
	rateLimiter := time.NewTicker(50 * time.Millisecond)
	defer rateLimiter.Stop()

	// Launch goroutines
	for i, sourceUrl := range validURLs {
		go func(idx int, url string) {
			<-rateLimiter.C // Rate limit the requests

			decodedProviderID := decodeProviderID(url[2:])
			Log(fmt.Sprintf("Processing URL %d/%d with provider ID: %s", idx+1, len(validURLs), decodedProviderID), logFile)

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
					Log(fmt.Sprintf("Warning: invalid link format for provider %s", decodedProviderID), logFile)
					continue
				}

				link, ok := linkMap["link"].(string)
				if !ok {
					Log(fmt.Sprintf("Warning: link field is not string for provider %s", decodedProviderID), logFile)
					continue
				}

				links = append(links, link)
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

	// Collect results maintaining order
	for successCount < len(validURLs) {
		select {
		case res := <-results:
			if res.err != nil {
				Log(fmt.Sprintf("Error processing URL %d: %v", res.index+1, res.err), logFile)
				collectedErrors = append(collectedErrors, fmt.Errorf("URL %d: %w", res.index+1, res.err))
			} else {
				orderedResults[res.index] = res.links
				successCount++
				Log(fmt.Sprintf("Successfully processed URL %d/%d", res.index+1, len(validURLs)), logFile)
			}
		case <-timeout:
			if successCount > 0 {
				Log(fmt.Sprintf("Timeout reached with %d/%d successful results", successCount, len(validURLs)), logFile)
				// Flatten available results
				return flattenResults(orderedResults), nil
			}
			return nil, fmt.Errorf("timeout waiting for results after %d successful responses", successCount)
		}
	}

	// If we have any errors but also some successes, log errors but continue
	if len(collectedErrors) > 0 {
		Log(fmt.Sprintf("Completed with %d errors: %v", len(collectedErrors), collectedErrors), logFile)
	}

	// Flatten and return results
	allLinks := flattenResults(orderedResults)
	if len(allLinks) == 0 {
		return nil, fmt.Errorf("no valid links found from %d URLs: %v", len(validURLs), collectedErrors)
	}

	return allLinks, nil
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
