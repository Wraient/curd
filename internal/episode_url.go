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
)

type Config struct {
	Agent        string
	AllanimeRefr string
	AllanimeBase string
	AllanimeAPI  string
	SubOrDub     string
	Mode         string
	Quality      string
}

func getDefaultConfig() Config {
	mode := os.Getenv("ANI_CLI_MODE")
	if mode == "" {
		mode = "sub"
	}
	return Config{
		Agent:        "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:109.0) Gecko/20100101 Firefox/121.0",
		AllanimeRefr: "https://allanime.to",
		AllanimeBase: "allanime.day",
		AllanimeAPI:  "https://api.allanime.day",
		SubOrDub:     "sub",
		Mode:         mode,
		Quality:      getEnvOrDefault("ANI_CLI_QUALITY", "best"),
	}
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
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

func getLinks(config Config, providerID string) ([]string, error) {
	url := fmt.Sprintf("https://%s%s", config.AllanimeBase, providerID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", config.Agent)
	req.Header.Set("Referer", config.AllanimeRefr)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	bodyStr := string(body)
	var links []string

	// Extract links with their qualities
	re := regexp.MustCompile(`link":"([^"]*)".*?"resolutionStr":"([^"]*)"`)
	matches := re.FindAllStringSubmatch(bodyStr, -1)
	for _, match := range matches {
		if len(match) >= 3 {
			link := fmt.Sprintf("%s>%s", match[2], match[1])
			links = append(links, link)
		}
	}

	// Extract m3u8 links
	re = regexp.MustCompile(`hls","url":"([^"]*)".*?"hardsub_lang":"en-US"`)
	matches = re.FindAllStringSubmatch(bodyStr, -1)
	for _, match := range matches {
		if len(match) >= 2 {
			links = append(links, match[1])
		}
	}

	// Process links based on provider type
	var processedLinks []string
	for _, link := range links {
		if strings.Contains(link, "repackager.wixmp.com") {
			parts := strings.Split(link, ">")
			if len(parts) == 2 {
				extractLink := parts[1]
				extractLink = strings.ReplaceAll(extractLink, "repackager.wixmp.com/", "")
				extractLink = regexp.MustCompile(`\.urlset.*`).ReplaceAllString(extractLink, "")

				re = regexp.MustCompile(`/,([^/]*),/mp4`)
				matches = re.FindAllStringSubmatch(link, -1)
				for _, match := range matches {
					if len(match) >= 2 {
						quality := match[1]
						newLink := fmt.Sprintf("%s>%s", quality, extractLink)
						newLink = strings.ReplaceAll(newLink, ","+quality, quality)
						processedLinks = append(processedLinks, newLink)
					}
				}
			}
		} else if strings.Contains(link, "vipanicdn") || strings.Contains(link, "anifastcdn") {
			if strings.Contains(link, "original.m3u") {
				processedLinks = append(processedLinks, link)
			} else {
				parts := strings.Split(link, ">")
				if len(parts) > 0 {
					extractLink := parts[0]
					relativeLink := regexp.MustCompile(`[^/]*$`).ReplaceAllString(extractLink, "")

					// Fetch m3u8 content
					req, err := http.NewRequest("GET", extractLink, nil)
					if err != nil {
						continue
					}
					req.Header.Set("User-Agent", config.Agent)
					req.Header.Set("Referer", config.AllanimeRefr)

					resp, err := client.Do(req)
					if err != nil {
						continue
					}
					m3u8Content, err := ioutil.ReadAll(resp.Body)
					resp.Body.Close()
					if err != nil {
						continue
					}

					lines := strings.Split(string(m3u8Content), "\n")
					for i := 0; i < len(lines); i++ {
						line := lines[i]
						if strings.HasPrefix(line, "#") {
							continue
						}
						quality := regexp.MustCompile(`x(\d+)`).ReplaceAllString(line, "$1")
						if quality != "" {
							processedLinks = append(processedLinks, fmt.Sprintf("%sp>%s%s", quality, relativeLink, line))
						}
					}
				}
			}
		} else if link != "" {
			processedLinks = append(processedLinks, link)
		}
	}

	return processedLinks, nil
}

func generateLink(config Config, resp string, providerType int) ([]string, error) {
	var pattern string
	switch providerType {
	case 1:
		pattern = `/Default :([^\n]+)`
	case 2:
		pattern = `/Sak :([^\n]+)`
	case 3:
		pattern = `/Kir :([^\n]+)`
	case 4:
		pattern = `/S-mp4 :([^\n]+)`
	default:
		pattern = `/Luf-mp4 :([^\n]+)`
	}

	re := regexp.MustCompile(pattern)
	matches := re.FindStringSubmatch(resp)
	if len(matches) < 2 {
		return nil, fmt.Errorf("no provider ID found")
	}

	providerID := decodeProviderID(matches[1])
	if strings.Contains(providerID, "/clock") {
		providerID = strings.Replace(providerID, "/clock", "/clock.json", 1)
	}

	return getLinks(config, providerID)
}

func extract_link(provider_id string) map[string]interface{} {
	allanime_base := "https://allanime.day"
	url := allanime_base+provider_id
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

	// Transform response similar to the bash script
	responseStr := string(body)
	responseStr = strings.ReplaceAll(responseStr, "\\u002F", "/")
	responseStr = strings.ReplaceAll(responseStr, "\\", "")
	responseStr = strings.ReplaceAll(responseStr, "{", "\n")
	responseStr = strings.ReplaceAll(responseStr, "}", "\n")

	// fmt.Println(responseStr)

	re := regexp.MustCompile(`sourceUrl":"--([^"]*)".*?sourceName":"([^"]*)"`)
	matches := re.FindAllStringSubmatch(responseStr, -1)

	if len(matches) == 0 {
		return nil, fmt.Errorf("no source URLs found")
	}

	var allLinks []string
	for providerType := 1; providerType <= 5; providerType++ {
		links, err := generateLink(config, responseStr, providerType)
		if err == nil && len(links) > 0 {
			allLinks = append(allLinks, links...)
		}
	}

	return allLinks, nil
}

func main() {
	config := getDefaultConfig()
	
	id := "RezHft5pjutwWcE3B"
	epNo := "12"
	
	links, err := getEpisodeURL(config, id, epNo)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		if strings.Contains(err.Error(), "no source URLs found") {
			fmt.Println("Episode not released!")
		}
		os.Exit(1)
	}

	// Print all found links
	fmt.Println("links:", links)
	for _, link := range links {
		fmt.Println(link)
	}
}