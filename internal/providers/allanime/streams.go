package allanime

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wraient/curd/internal/curdhost"
	"github.com/wraient/curd/internal/providers"
)

const allanimeUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:109.0) Gecko/20100101 Firefox/121.0"

type allanimeResolvedStream struct {
	URL          string
	Referrer     string
	SubtitleURL  string
	QualityScore int
}

var (
	allanimeClockRefererPattern = regexp.MustCompile(`"Referer":"([^"]+)"`)
	allanimeClockSubtitlePattern = regexp.MustCompile(`"subtitles":\[\{"lang":"en","label":"English","default":"default","src":"([^"]+)"`)
	allanimeStreamResolutionPattern = regexp.MustCompile(`RESOLUTION=(\d+)x(\d+)`)
	allanimeStreamBandwidthPattern  = regexp.MustCompile(`BANDWIDTH=(\d+)`)
)

func getAllanimeEpisodeStreamsForMode(id, mode string, epNo int) ([]string, map[string]providers.StreamPlaybackHint, error) {
	sourceUrls, err := fetchEpisodeSourcesForMode(id, mode, epNo)
	if err != nil {
		return nil, nil, err
	}
	return getLinksFromEncodedSourceUrls(sourceUrls)
}

func getLinksFromEncodedSourceUrls(sourceUrls []allanimeSource) ([]string, map[string]providers.StreamPlaybackHint, error) {
	type providerJob struct {
		index int
		name  string
		url   string
	}

	jobs := make([]providerJob, 0)
	for _, providerName := range allanimeNamedProviders {
		source, ok := findNamedAllanimeSource(sourceUrls, providerName)
		if !ok {
			continue
		}
		sourceURL := strings.TrimSpace(source.SourceUrl)
		if !strings.HasPrefix(sourceURL, "--") || len(sourceURL) <= 2 {
			continue
		}
		jobs = append(jobs, providerJob{
			index: len(jobs),
			name:  providerName,
			url:   sourceURL,
		})
	}
	if len(jobs) == 0 {
		return nil, nil, fmt.Errorf("no encoded Allanime provider sources found")
	}

	type streamResult struct {
		index   int
		streams []allanimeResolvedStream
		err     error
	}

	results := make(chan streamResult, len(jobs))
	var wg sync.WaitGroup
	for _, job := range jobs {
		wg.Add(1)
		go func(idx int, providerName, encodedURL string) {
			defer wg.Done()
			decodedProviderID := decodeProviderID(encodedURL[2:])
			curdhost.Log(fmt.Sprintf("Fetching Allanime provider %s via %s", providerName, decodedProviderID))
			streams, err := resolveAllanimeClockProvider(providerName, decodedProviderID)
			results <- streamResult{index: idx, streams: streams, err: err}
		}(job.index, job.name, job.url)
	}
	go func() {
		wg.Wait()
		close(results)
	}()

	orderedStreams := make([][]allanimeResolvedStream, len(jobs))
	var collectedErrors []error
	successCount := 0
	timeout := time.After(15 * time.Second)
	completedCount := 0
	for completedCount < len(jobs) {
		select {
		case res, ok := <-results:
			if !ok {
				completedCount = len(jobs)
				break
			}
			completedCount++
			if res.err != nil {
				curdhost.Log(fmt.Sprintf("Allanime provider %s failed: %v", jobs[res.index].name, res.err))
				collectedErrors = append(collectedErrors, fmt.Errorf("%s: %w", jobs[res.index].name, res.err))
				continue
			}
			if len(res.streams) > 0 {
				orderedStreams[res.index] = res.streams
				successCount++
				curdhost.Log(fmt.Sprintf("Allanime provider %s returned %d stream(s)", jobs[res.index].name, len(res.streams)))
			}
		case <-timeout:
			if successCount > 0 {
				return buildAllanimeLinkResult(orderedStreams)
			}
			return nil, nil, fmt.Errorf("timeout waiting for Allanime provider links")
		}
	}

	if successCount == 0 {
		return nil, nil, fmt.Errorf("no valid links found from Allanime providers: %v", collectedErrors)
	}
	return buildAllanimeLinkResult(orderedStreams)
}

func buildAllanimeLinkResult(orderedStreams [][]allanimeResolvedStream) ([]string, map[string]providers.StreamPlaybackHint, error) {
	allStreams := make([]allanimeResolvedStream, 0)
	for _, streams := range orderedStreams {
		allStreams = append(allStreams, streams...)
	}
	sort.SliceStable(allStreams, func(i, j int) bool {
		return allStreams[i].QualityScore > allStreams[j].QualityScore
	})

	links := make([]string, 0, len(allStreams))
	hints := make(map[string]providers.StreamPlaybackHint)
	seen := make(map[string]struct{})
	for _, stream := range allStreams {
		if stream.URL == "" || isUnreliableAllanimeDirectURL(stream.URL) {
			continue
		}
		if _, exists := seen[stream.URL]; exists {
			continue
		}
		seen[stream.URL] = struct{}{}
		links = append(links, stream.URL)
		referrer := stream.Referrer
		if referrer == "" {
			referrer = allanimeGraphQLReferer
		}
		hints[stream.URL] = providers.StreamPlaybackHint{
			Referrer: referrer,
			Subtitle: stream.SubtitleURL,
		}
	}
	if len(links) == 0 {
		return nil, nil, fmt.Errorf("no reliable Allanime streams found")
	}
	return links, hints, nil
}

func resolveAllanimeClockProvider(providerName, providerPath string) ([]allanimeResolvedStream, error) {
	providerPath = normalizeAllanimeProviderPath(providerPath)
	if strings.HasPrefix(providerPath, "http://") || strings.HasPrefix(providerPath, "https://") {
		if isUnreliableAllanimeDirectURL(providerPath) {
			return nil, fmt.Errorf("skipping unreliable fast4speed source")
		}
		return []allanimeResolvedStream{{
			URL:          providerPath,
			Referrer:     allanimeGraphQLReferer,
			QualityScore: 0,
		}}, nil
	}

	rawBody, videoData, err := fetchAllanimeClockResponse(providerPath)
	if err != nil {
		return nil, err
	}

	if providerName == "Fm-mp4" {
		if filemoonLinks := extractFilemoonLinks(videoData); len(filemoonLinks) > 0 {
			return streamsFromPlainURLs(filemoonLinks, allanimeGraphQLReferer, ""), nil
		}
	}

	referrer := extractAllanimeClockReferer(rawBody)
	subtitleURL := extractAllanimeClockSubtitle(rawBody)
	streams := make([]allanimeResolvedStream, 0)

	linksInterface, ok := videoData["links"].([]interface{})
	if !ok || len(linksInterface) == 0 {
		return nil, fmt.Errorf("no links field in Allanime extractor response")
	}

	for _, linkInterface := range linksInterface {
		linkMap, ok := linkInterface.(map[string]interface{})
		if !ok {
			continue
		}
		if _, hasLink := linkMap["link"]; !hasLink {
			if _, cacheOnly := linkMap["mp4"].(bool); cacheOnly {
				continue
			}
		}

		resolutionScore := allanimeResolutionScore(linkMap["resolutionStr"])
		if linkURL, ok := linkMap["link"].(string); ok && strings.TrimSpace(linkURL) != "" {
			streams = append(streams, resolveAllanimeLinkURL(linkURL, referrer, subtitleURL, resolutionScore)...)
			continue
		}

		if hlsMap, ok := linkMap["hls"].(map[string]interface{}); ok {
			if hlsURL, ok := hlsMap["url"].(string); ok && strings.TrimSpace(hlsURL) != "" {
				streams = append(streams, resolveAllanimeLinkURL(hlsURL, referrer, subtitleURL, resolutionScore)...)
			}
		}
	}

	if len(streams) == 0 {
		return nil, fmt.Errorf("no playable links in Allanime extractor response")
	}
	sort.SliceStable(streams, func(i, j int) bool {
		return streams[i].QualityScore > streams[j].QualityScore
	})
	return streams, nil
}

func fetchAllanimeClockResponse(providerPath string) ([]byte, map[string]interface{}, error) {
	requestURL := "https://allanime.day" + providerPath
	req, err := http.NewRequest("GET", requestURL, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Referer", allanimeGraphQLReferer)
	req.Header.Set("User-Agent", allanimeUserAgent)

	resp, err := curdhost.HTTPClient().Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}
	if !curdhost.HTTPStatusOK(resp.StatusCode) {
		return nil, nil, curdhost.HTTPStatusError("allanime source extractor", resp.StatusCode, body)
	}

	var videoData map[string]interface{}
	if err := json.Unmarshal(body, &videoData); err != nil {
		return nil, nil, fmt.Errorf("failed to parse Allanime extractor JSON: %w", err)
	}
	return body, videoData, nil
}

func resolveAllanimeLinkURL(linkURL, referrer, subtitleURL string, resolutionScore int) []allanimeResolvedStream {
	linkURL = strings.TrimSpace(linkURL)
	if linkURL == "" {
		return nil
	}

	if strings.Contains(linkURL, "repackager.wixmp.com") {
		expanded := expandWixmpLinks(linkURL)
		return streamsFromPlainURLs(expanded, referrer, subtitleURL, resolutionScore)
	}

	if strings.Contains(linkURL, "master.m3u8") {
		if streams, err := fetchAllanimeM3U8VariantStreams(linkURL, referrer, subtitleURL); err == nil && len(streams) > 0 {
			return streams
		}
	}

	qualityScore := resolutionScore
	if qualityScore == 0 {
		qualityScore = wixmpQualityScore(linkURL)
	}
	return []allanimeResolvedStream{{
		URL:          linkURL,
		Referrer:     referrer,
		SubtitleURL:  subtitleURL,
		QualityScore: qualityScore,
	}}
}

func fetchAllanimeM3U8VariantStreams(masterURL, referrer, subtitleURL string) ([]allanimeResolvedStream, error) {
	req, err := http.NewRequest("GET", masterURL, nil)
	if err != nil {
		return nil, err
	}
	if referrer == "" {
		referrer = allanimeGraphQLReferer
	}
	req.Header.Set("Referer", referrer)
	req.Header.Set("User-Agent", allanimeUserAgent)

	resp, err := curdhost.HTTPClient().Do(req)
	if err != nil {
		return nil, err
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, err
	}
	if !curdhost.HTTPStatusOK(resp.StatusCode) {
		return nil, curdhost.HTTPStatusError("allanime m3u8 master playlist", resp.StatusCode, body)
	}

	playlist := string(body)
	if !strings.Contains(playlist, "EXTM3U") {
		return nil, fmt.Errorf("response is not an m3u8 playlist")
	}

	baseURL := masterURL
	if idx := strings.LastIndex(baseURL, "/"); idx >= 0 {
		baseURL = baseURL[:idx+1]
	}

	lines := strings.Split(playlist, "\n")
	streams := make([]allanimeResolvedStream, 0)
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, "#EXT-X-STREAM-INF:") {
			continue
		}
		qualityScore := parseAllanimeStreamInfScore(line)
		i++
		for i < len(lines) {
			nextLine := strings.TrimSpace(lines[i])
			if nextLine == "" {
				i++
				continue
			}
			if strings.HasPrefix(nextLine, "#EXT-X-I-FRAME-STREAM-INF") {
				break
			}
			if strings.HasPrefix(nextLine, "#") {
				i++
				continue
			}
			streamURL := resolveAllanimeRelativeURL(baseURL, nextLine)
			streams = append(streams, allanimeResolvedStream{
				URL:          streamURL,
				Referrer:     referrer,
				SubtitleURL:  subtitleURL,
				QualityScore: qualityScore,
			})
			break
		}
	}

	if len(streams) == 0 {
		return nil, fmt.Errorf("no variant streams found in m3u8 playlist")
	}
	sort.SliceStable(streams, func(i, j int) bool {
		return streams[i].QualityScore > streams[j].QualityScore
	})
	return streams, nil
}

func parseAllanimeStreamInfScore(line string) int {
	if match := allanimeStreamResolutionPattern.FindStringSubmatch(line); len(match) == 3 {
		if width, err := strconv.Atoi(match[1]); err == nil && width > 0 {
			return width
		}
	}
	if match := allanimeStreamBandwidthPattern.FindStringSubmatch(line); len(match) == 2 {
		if bandwidth, err := strconv.Atoi(match[1]); err == nil && bandwidth > 0 {
			return bandwidth / 1000
		}
	}
	return 0
}

func resolveAllanimeRelativeURL(baseURL, uri string) string {
	uri = strings.TrimSpace(uri)
	if strings.HasPrefix(uri, "http://") || strings.HasPrefix(uri, "https://") {
		return uri
	}
	if strings.HasPrefix(uri, "/") {
		if parsed, err := url.Parse(baseURL); err == nil {
			return parsed.Scheme + "://" + parsed.Host + uri
		}
	}
	return baseURL + uri
}

func extractAllanimeClockReferer(rawBody []byte) string {
	if match := allanimeClockRefererPattern.FindSubmatch(rawBody); len(match) == 2 {
		return string(match[1])
	}
	return allanimeGraphQLReferer
}

func extractAllanimeClockSubtitle(rawBody []byte) string {
	if match := allanimeClockSubtitlePattern.FindSubmatch(rawBody); len(match) == 2 {
		return string(match[1])
	}
	return ""
}

func allanimeResolutionScore(value interface{}) int {
	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "" {
		return 0
	}
	if score := wixmpQualityScore(text); score > 0 {
		return score
	}
	if digits := regexp.MustCompile(`(\d{3,4})`).FindStringSubmatch(text); len(digits) == 2 {
		if score, err := strconv.Atoi(digits[1]); err == nil {
			return score
		}
	}
	return 0
}

func streamsFromPlainURLs(urls []string, referrer, subtitleURL string, qualityScores ...int) []allanimeResolvedStream {
	defaultScore := 0
	if len(qualityScores) > 0 {
		defaultScore = qualityScores[0]
	}
	streams := make([]allanimeResolvedStream, 0, len(urls))
	for _, linkURL := range urls {
		linkURL = strings.TrimSpace(linkURL)
		if linkURL == "" {
			continue
		}
		score := defaultScore
		if score == 0 {
			score = wixmpQualityScore(linkURL)
		}
		streams = append(streams, allanimeResolvedStream{
			URL:          linkURL,
			Referrer:     referrer,
			SubtitleURL:  subtitleURL,
			QualityScore: score,
		})
	}
	return streams
}
