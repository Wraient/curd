package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"unicode"
)

type Config struct {
	Agent        string
	AllanimeRefr string
	AllanimeBase string
	AllanimeAPI  string
	Mode         string
	Quality      string
}

type AllanimeResponse struct {
	Data struct {
		Episode struct {
			SourceUrls []struct {
				SourceUrl string `json:"sourceUrl"`
			} `json:"sourceUrls"`
		} `json:"episode"`
	} `json:"data"`
}

func getDefaultConfig() Config {
	return Config{
		Agent:        "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:109.0) Gecko/20100101 Firefox/121.0",
		AllanimeRefr: "https://allanime.to",
		AllanimeBase: "allanime.day",
		AllanimeAPI:  "https://api.allanime.day",
		Mode:         "dub",
		Quality:      "best",
	}
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
		fmt.Println("Error creating request:", err)
		return videoData
	}

	// Add the headers
	req.Header.Set("Referer", "https://allanime.to")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:109.0) Gecko/20100101 Firefox/121.0")

	// Send the request
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Error sending request:", err)
		return videoData
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Error reading response:", err)
		return videoData
	}

	// Parse the JSON response
	err = json.Unmarshal(body, &videoData)
	if err != nil {
		fmt.Println("Error parsing JSON:", err)
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
func getEpisodeURL(config Config, id, epNo string) ([]string, error) {
	query := `query($showId:String!,$translationType:VaildTranslationTypeEnumType!,$episodeString:String!){episode(showId:$showId,translationType:$translationType,episodeString:$episodeString){episodeString sourceUrls}}`

	variables := map[string]string{
		"showId":          id,
		"translationType": config.Mode,
		"episodeString":   epNo,
	}

	variablesJSON, err := json.Marshal(variables)
	if err != nil {
		return nil, err
	}

	values := url.Values{}
	values.Set("query", query)
	values.Set("variables", string(variablesJSON))

	reqURL := fmt.Sprintf("%s/api?%s", config.AllanimeAPI, values.Encode())

	client := &http.Client{}
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", config.Agent)
	req.Header.Set("Referer", config.AllanimeRefr)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	responseStr := string(body)

	// Unmarshal the JSON data into the struct
	var response AllanimeResponse
	err = json.Unmarshal([]byte(responseStr), &response)
	if err != nil {
		fmt.Println("Error parsing JSON: ", err)
	}

	var allinks []string // This will be returned

	// Iterate through the SourceUrls and print each URL
	for _, url := range response.Data.Episode.SourceUrls {
		if len(url.SourceUrl) > 2 && unicode.IsDigit(rune(url.SourceUrl[2])) { // Source Url 3rd letter is a number (it stars as --32f23k31jk)
			decodedProviderID := decodeProviderID(url.SourceUrl[2:]) // Decode the source url to get the provider id
			extractedLinks := extractLinks(decodedProviderID) // Extract the links using provider id
			if linksInterface, ok := extractedLinks["links"].([]interface{}); ok {
				for _, linkInterface := range linksInterface {
					if linkMap, ok := linkInterface.(map[string]interface{}); ok {
						if link, ok := linkMap["link"].(string); ok {
							allinks = append(allinks, link) // Add all extracted links into allinks 
						}
					}
				}
			} else {
				fmt.Println("Links field is not of the expected type []interface{}")
			}
		}
	}

	return allinks, nil
}

func main() {
	config := getDefaultConfig()

	id := "ReooPAxPMsHM4KPMY" // One piece
	// id := "RezHft5pjutwWcE3B" // Death note
	epNo := "945"

	links, err := getEpisodeURL(config, id, epNo)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		if strings.Contains(err.Error(), "no source URLs found") {
			fmt.Println("Episode not released!")
		}
		os.Exit(1)
	}

	// Print all found links
	// fmt.Println("links:", links)
	for _, link := range links {
		fmt.Println(link)
	}
}