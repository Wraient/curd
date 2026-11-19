package animepahe

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"

	"github.com/wraient/curd/internal/curdhost"
	"github.com/wraient/curd/internal/providers"
)

var animepaheCookiesBypassed bool
var solveAnimepaheBrowserChallenge = solveAnimepaheBrowserChallengeWithRod

// animepaheBypassMu serializes DDoS-Guard bypass attempts. Stacked searches
// run providers concurrently, so without this two goroutines could pass the
// bypassed check together and launch duplicate headless browsers while
// racing on the cookie file.
var animepaheBypassMu sync.Mutex

// maxAnimepaheBodyBytes caps every response body read. Real payloads here are
// tens to hundreds of kilobytes (search/release JSON, player and kwik pages),
// so this bound never truncates legitimate responses.
const maxAnimepaheBodyBytes = 4 << 20

func readAnimepaheBody(resp *http.Response) ([]byte, error) {
	return io.ReadAll(io.LimitReader(resp.Body, maxAnimepaheBodyBytes))
}

// kwikEvalRe, kwikM3u8Re and kwikTokenRe are the extractKwikM3u8/unpackKwik
// patterns compiled once. They are byte-identical to the per-call patterns
// used before; hoisting only avoids recompiling them per stream link.
var (
	kwikEvalRe  = regexp.MustCompile(`eval\(function\(p,a,c,k,e,d\).*?\}\('(.*?)',(\d+),(\d+),'([^']+)'\.split\('\|'\).*?\)\)`)
	kwikM3u8Re  = regexp.MustCompile(`(https://[a-zA-Z0-9\-\.\/]+m3u8)`)
	kwikTokenRe = regexp.MustCompile(`[a-zA-Z0-9_]+`)
)

func getCookieFilePath() string {
	return filepath.Join(curdhost.StoragePath(), "animepahe_cookies.json")
}

func saveCookies(cookies []*http.Cookie) {
	cookieFilePath := getCookieFilePath()
	os.MkdirAll(filepath.Dir(cookieFilePath), 0755)
	b, _ := json.Marshal(cookies)
	os.WriteFile(cookieFilePath, b, 0644)
}

func loadCookies() []*http.Cookie {
	cookieFilePath := getCookieFilePath()
	b, err := os.ReadFile(cookieFilePath)
	if err != nil {
		return nil
	}
	var cookies []*http.Cookie
	if err := json.Unmarshal(b, &cookies); err != nil {
		return nil
	}
	return cookies
}

func newAnimepaheAPIRequest(rawURL string) (*http.Request, error) {
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://animepahe.pw/")
	req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	return req, nil
}

// NewPageRequest builds an HTTP request for animepahe HTML pages.
func NewPageRequest(rawURL string) (*http.Request, error) {
	return newAnimepahePageRequest(rawURL)
}

func newAnimepahePageRequest(rawURL string) (*http.Request, error) {
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://animepahe.pw/")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	return req, nil
}

func checkCookiesValid() bool {
	req, err := newAnimepaheAPIRequest("https://animepahe.pw/api?m=search&q=test")
	if err != nil {
		return false
	}

	resp, err := curdhost.HTTPClient().Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	body, err := readAnimepaheBody(resp)
	if err != nil {
		return false
	}
	if resp.StatusCode != 200 {
		return false
	}
	if isAnimepaheChallengeBody(body) {
		return false
	}

	contentType := resp.Header.Get("Content-Type")
	return strings.Contains(contentType, "application/json")
}

func isAnimepaheChallengeResponse(resp *http.Response, body []byte) bool {
	if resp == nil {
		return false
	}
	if isAnimepaheChallengeBody(body) {
		return true
	}
	return resp.StatusCode == http.StatusForbidden && strings.Contains(strings.ToLower(resp.Header.Get("Server")), "ddos-guard")
}

func isAnimepaheChallengeBody(body []byte) bool {
	normalized := strings.ToLower(string(body))
	return strings.Contains(normalized, "ddos-guard") ||
		strings.Contains(normalized, "/.well-known/ddos-guard/") ||
		strings.Contains(normalized, "checking your browser")
}

func (p *Provider) ensureBypass() (err error) {
	animepaheBypassMu.Lock()
	defer animepaheBypassMu.Unlock()
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("failed to solve Animepahe DDoS-Guard challenge: %v", recovered)
		}
	}()

	if animepaheCookiesBypassed {
		return nil
	}

	cookies := loadCookies()
	if len(cookies) > 0 {
		u, _ := url.Parse("https://animepahe.pw")
		curdhost.SetCookiesForAnimepahe(u, cookies)

		if checkCookiesValid() {
			animepaheCookiesBypassed = true
			curdhost.Log("Successfully restored Animepahe session from cache.")
			return nil
		}
		curdhost.Log("Cached Animepahe session expired. Requesting new session...")
	}

	return p.refreshBypass()
}

func (p *Provider) refreshBypass() error {
	u, _ := url.Parse("https://animepahe.pw")
	animepaheCookiesBypassed = false
	if err := os.Remove(getCookieFilePath()); err != nil && !os.IsNotExist(err) {
		curdhost.Log(fmt.Sprintf("Failed to remove cached Animepahe cookies: %v", err))
	}

	curdhost.Out("Animepahe needs a browser challenge before Curd can search or play. Starting a temporary headless browser...")
	httpCookies, err := solveAnimepaheBrowserChallenge()
	if err != nil {
		curdhost.Out("Animepahe browser challenge failed. Try again later or switch provider.")
		return fmt.Errorf("animepahe browser challenge failed: %w", err)
	}
	if len(httpCookies) == 0 {
		return fmt.Errorf("animepahe browser challenge returned no cookies")
	}
	curdhost.SetCookiesForAnimepahe(u, httpCookies)

	if !checkCookiesValid() {
		return fmt.Errorf("bypassed cookies are still invalid")
	}

	saveCookies(httpCookies)
	animepaheCookiesBypassed = true
	curdhost.Out("Successfully bypassed DDoS-Guard.")
	return nil
}

func solveAnimepaheBrowserChallengeWithRod() (httpCookies []*http.Cookie, err error) {
	if err := rod.Try(func() {
		l := launcher.New().Headless(true)
		defer l.Cleanup()
		browser := rod.New().ControlURL(l.MustLaunch()).MustConnect()
		defer browser.MustClose()

		page := browser.MustPage("https://animepahe.pw/")
		page.MustWaitLoad()

		for i := 0; i < 30; i++ {
			info, err := page.Info()
			if err == nil && info.Title != "DDoS-Guard" && info.Title != "Just a moment..." && info.Title != "" {
				break
			}
			time.Sleep(500 * time.Millisecond)
		}

		rodCookies, err := page.Cookies(nil)
		if err != nil {
			panic(err)
		}
		for _, cookie := range rodCookies {
			httpCookies = append(httpCookies, &http.Cookie{
				Name:  cookie.Name,
				Value: cookie.Value,
			})
		}
	}); err != nil {
		return nil, err
	}
	return httpCookies, nil
}

type Provider struct{}

func (p *Provider) Name() string {
	return "animepahe"
}

// SearchItem is an animepahe API search result payload.
type SearchItem struct {
	ID       int     `json:"id"`
	Title    string  `json:"title"`
	Type     string  `json:"type"`
	Episodes int     `json:"episodes"`
	Status   string  `json:"status"`
	Season   string  `json:"season"`
	Year     int     `json:"year"`
	Score    float64 `json:"score"`
	Poster   string  `json:"poster"`
	Session  string  `json:"session"`
}

type AnimeRef struct {
	ReleaseID string
	Session   string
}

func (r AnimeRef) apiID() string {
	if r.Session != "" {
		return r.Session
	}
	return r.ReleaseID
}

func formatAnimepaheProviderID(item SearchItem) string {
	if item.ID > 0 && item.Session != "" {
		return fmt.Sprintf("%d:%s", item.ID, item.Session)
	}
	if item.Session != "" {
		return item.Session
	}
	if item.ID > 0 {
		return strconv.Itoa(item.ID)
	}
	return ""
}

// ParseProviderID parses a stored animepahe show identifier.
func ParseProviderID(providerID string) AnimeRef {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return AnimeRef{}
	}

	parts := strings.SplitN(providerID, ":", 2)
	if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
		if _, err := strconv.Atoi(parts[0]); err == nil {
			return AnimeRef{ReleaseID: parts[0], Session: parts[1]}
		}
	}

	if _, err := strconv.Atoi(providerID); err == nil {
		return AnimeRef{ReleaseID: providerID}
	}

	return AnimeRef{Session: providerID}
}

// StableProviderID returns the release id portion used for history matching.
func StableProviderID(providerID string) string {
	animeRef := ParseProviderID(providerID)
	if animeRef.ReleaseID != "" {
		return animeRef.ReleaseID
	}
	return providerID
}

func (p *Provider) ResolveProviderID(providerID, query string) (string, error) {
	animeRef := ParseProviderID(providerID)
	query = strings.TrimSpace(query)
	if query == "" {
		if animeRef.Session != "" {
			return providerID, nil
		}
		return "", fmt.Errorf("cannot resolve animepahe provider id %q without an anime title", providerID)
	}

	options, err := p.SearchAnime(query, "")
	if err != nil {
		if animeRef.Session != "" {
			curdhost.Log(fmt.Sprintf("Animepahe provider id refresh failed for %q, using existing session: %v", providerID, err))
			return providerID, nil
		}
		return "", err
	}

	for _, option := range options {
		item, ok := option.ExtraData.(SearchItem)
		if !ok {
			continue
		}

		if animeRef.ReleaseID != "" && strconv.Itoa(item.ID) == animeRef.ReleaseID {
			return formatAnimepaheProviderID(item), nil
		}
		if animeRef.Session != "" && item.Session == animeRef.Session {
			return formatAnimepaheProviderID(item), nil
		}
	}

	if animeRef.Session != "" {
		curdhost.Log(fmt.Sprintf("Animepahe provider id %q was not found in search results for %q, using existing session", providerID, query))
		return providerID, nil
	}

	return "", fmt.Errorf("animepahe id %q was not found in search results for %q", providerID, query)
}

type animepaheSearchResponse struct {
	Total       int                   `json:"total"`
	PerPage     int                   `json:"per_page"`
	CurrentPage int                   `json:"current_page"`
	LastPage    int                   `json:"last_page"`
	Data        []SearchItem `json:"data"`
}

func (p *Provider) SearchAnime(query, mode string) ([]providers.SelectionOption, error) {
	if err := p.ensureBypass(); err != nil {
		return nil, err
	}
	// animepahe doesn't distinguish sub/dub at the search level
	searchUrl := fmt.Sprintf("https://animepahe.pw/api?m=search&q=%s", url.QueryEscape(query))

	req, err := newAnimepaheAPIRequest(searchUrl)
	if err != nil {
		return nil, err
	}

	resp, err := curdhost.HTTPClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := readAnimepaheBody(resp)
	if err != nil {
		return nil, err
	}
	if !curdhost.HTTPStatusOK(resp.StatusCode) {
		return nil, curdhost.HTTPStatusError("animepahe search", resp.StatusCode, body)
	}

	var searchResp animepaheSearchResponse
	err = json.Unmarshal(body, &searchResp)
	if err != nil {
		return nil, err
	}

	var result []providers.SelectionOption
	for _, item := range searchResp.Data {
		label := fmt.Sprintf("%s (%d episodes) [animepahe]", item.Title, item.Episodes)
		result = append(result, providers.SelectionOption{
			Title:     item.Title,
			Label:     label,
			Key:       formatAnimepaheProviderID(item),
			Thumbnail: item.Poster,
			ExtraData: item,
		})
	}

	return result, nil
}

type animepaheEpisodesResponse struct {
	Total       int `json:"total"`
	PerPage     int `json:"per_page"`
	CurrentPage int `json:"current_page"`
	LastPage    int `json:"last_page"`
	Data        []struct {
		ID      int    `json:"id"`
		AnimeID int    `json:"anime_id"`
		Episode int    `json:"episode"`
		Session string `json:"session"`
	} `json:"data"`
}

// animepaheEpisode is one release-API entry: the catalog episode number plus
// the per-episode session needed to build the player URL.
type animepaheEpisode struct {
	number  int
	session string
}

// fetchReleaseEpisodes returns every release-API entry in API response order
// across all pages. errContext names the caller in status errors. The single
// DDoS-Guard session refresh matches the historical episode-list fetch,
// which is the fetch whose behavior and errors previously surfaced first on
// every path that now shares this helper.
func (p *Provider) fetchReleaseEpisodes(apiID, errContext string) ([]animepaheEpisode, error) {
	var all []animepaheEpisode
	page := 1
	challengeRetried := false

	for {
		epUrl := fmt.Sprintf("https://animepahe.pw/api?m=release&id=%s&sort=episode_asc&page=%d", apiID, page)
		req, err := newAnimepaheAPIRequest(epUrl)
		if err != nil {
			return nil, err
		}

		resp, err := curdhost.HTTPClient().Do(req)
		if err != nil {
			return nil, err
		}

		body, err := readAnimepaheBody(resp)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		if isAnimepaheChallengeResponse(resp, body) {
			if challengeRetried {
				return nil, fmt.Errorf("animepahe episode list is still blocked by DDoS-Guard after refreshing browser session")
			}
			curdhost.Log("Animepahe episode list returned DDoS-Guard challenge. Refreshing browser session...")
			if err := p.refreshBypass(); err != nil {
				return nil, err
			}
			challengeRetried = true
			continue
		}
		if !curdhost.HTTPStatusOK(resp.StatusCode) {
			return nil, curdhost.HTTPStatusError(errContext, resp.StatusCode, body)
		}

		var epsResp animepaheEpisodesResponse
		err = json.Unmarshal(body, &epsResp)
		if err != nil {
			return nil, err
		}

		for _, ep := range epsResp.Data {
			all = append(all, animepaheEpisode{number: ep.Episode, session: ep.Session})
		}

		if epsResp.CurrentPage >= epsResp.LastPage {
			break
		}
		page++
	}

	return all, nil
}

// sortedEpisodeNumbers returns the release entries' episode numbers in the
// same ascending string form EpisodesList has always produced.
func sortedEpisodeNumbers(pairs []animepaheEpisode) []string {
	numbers := make([]int, 0, len(pairs))
	for _, ep := range pairs {
		numbers = append(numbers, ep.number)
	}
	sort.Ints(numbers)
	result := make([]string, 0, len(numbers))
	for _, ep := range numbers {
		result = append(result, strconv.Itoa(ep))
	}
	return result
}

func (p *Provider) EpisodesList(showID, mode string) ([]string, error) {
	if err := p.ensureBypass(); err != nil {
		return nil, err
	}
	animeRef := ParseProviderID(showID)
	apiID := animeRef.apiID()
	if apiID == "" {
		return nil, fmt.Errorf("animepahe provider id %q is invalid; please reselect this anime", showID)
	}

	pairs, err := p.fetchReleaseEpisodes(apiID, "animepahe episode list")
	if err != nil {
		return nil, err
	}

	return sortedEpisodeNumbers(pairs), nil
}

func (p *Provider) GetEpisodeURL(config providers.PlaybackConfig, id string, epNo int) ([]string, error) {
	return p.GetEpisodeURLForMode(config, id, epNo, config.SubOrDub)
}

func (p *Provider) GetEpisodeURLForMode(config providers.PlaybackConfig, id string, epNo int, mode string) ([]string, error) {
	if err := p.ensureBypass(); err != nil {
		return nil, err
	}
	animeRef := ParseProviderID(id)
	if animeRef.Session == "" {
		return nil, fmt.Errorf("animepahe provider id %q is missing session id; please reselect this anime", id)
	}
	apiID := animeRef.apiID()

	// 1. Get episode session ID.
	// Map the 1-based Anilist episode number to Animepahe's actual episode
	// number. A single release-API fetch serves both the index mapping and
	// the session lookup; previously EpisodesList fetched every page and the
	// scan below fetched them all again. The error context preserves the
	// historical second-loop ("lookup") wording that surfaced here.
	pairs, err := p.fetchReleaseEpisodes(apiID, "animepahe episode lookup")
	if err != nil {
		return nil, err
	}
	eps := sortedEpisodeNumbers(pairs)
	if epNo < 1 || epNo > len(eps) {
		return nil, fmt.Errorf("episode %d out of bounds (found %d episodes)", epNo, len(eps))
	}

	mappedEpNo, err := strconv.Atoi(eps[epNo-1])
	if err != nil {
		return nil, fmt.Errorf("invalid episode number format in provider: %v", err)
	}

	// Scan for the episode session in API response order, as before.
	var episodeSession string
	for _, item := range pairs {
		if item.number == mappedEpNo {
			episodeSession = item.session
			break
		}
	}

	if episodeSession == "" {
		return nil, fmt.Errorf("episode %d (mapped to %d) not found", epNo, mappedEpNo)
	}

	// player page: https://animepahe.pw/play/<anime_session>/<episode_session>
	// stream links in player page: <button type=\"button\" data-src=\"https://kwik.cx/e/Jwd0hMNswksj\"
	playerUrl := fmt.Sprintf("https://animepahe.pw/play/%s/%s", animeRef.Session, episodeSession)
	req, err := newAnimepahePageRequest(playerUrl)
	if err != nil {
		return nil, err
	}

	resp, err := curdhost.HTTPClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := readAnimepaheBody(resp)
	if err != nil {
		return nil, err
	}
	if !curdhost.HTTPStatusOK(resp.StatusCode) {
		return nil, curdhost.HTTPStatusError("animepahe player page", resp.StatusCode, body)
	}

	bodyStr := string(body)
	type streamLink struct {
		url string
		res int
	}
	var subLinks []streamLink
	var dubLinks []streamLink

	parts := strings.Split(bodyStr, "<button")
	for i := 1; i < len(parts); i++ {
		part := parts[i]
		srcIdx := strings.Index(part, "data-src=\"")
		if srcIdx == -1 {
			continue
		}

		startIdx := srcIdx + 10
		endIdx := strings.Index(part[startIdx:], "\"")
		if endIdx == -1 {
			continue
		}

		link := part[startIdx : startIdx+endIdx]
		if !strings.Contains(link, "kwik.cx") {
			continue
		}

		resIdx := strings.Index(part, "data-resolution=\"")
		res := 0
		if resIdx != -1 {
			startRes := resIdx + 17
			endRes := strings.Index(part[startRes:], "\"")
			if endRes != -1 {
				res, _ = strconv.Atoi(part[startRes : startRes+endRes])
			}
		}

		sLink := streamLink{url: link, res: res}
		if strings.Contains(part, "data-audio=\"eng\"") {
			dubLinks = append(dubLinks, sLink)
		} else {
			subLinks = append(subLinks, sLink)
		}
	}

	// Sort by resolution descending
	sortFunc := func(links []streamLink) {
		sort.Slice(links, func(i, j int) bool {
			return links[i].res > links[j].res
		})
	}
	sortFunc(subLinks)
	sortFunc(dubLinks)

	var links []string
	requestedMode := providers.NormalizeTranslationType(mode)
	if requestedMode == "dub" {
		for _, l := range dubLinks {
			links = append(links, l.url)
		}
	} else {
		for _, l := range subLinks {
			links = append(links, l.url)
		}
	}
	if len(links) == 0 {
		return nil, fmt.Errorf("no %s streams found for episode %d", requestedMode, epNo)
	}

	// Resolve each kwik embed concurrently. Results land in the input slot,
	// so the collected links keep the same resolution-descending order as the
	// sequential loop, and per-link failures are still skipped individually.
	// The semaphore bounds simultaneous upstream fetches the way the
	// sibling providers bound theirs.
	m3u8ByIndex := make([]string, len(links))
	var kwikWG sync.WaitGroup
	kwikSem := make(chan struct{}, 6)
	for i, kwikLink := range links {
		kwikWG.Add(1)
		go func(i int, kwikLink string) {
			defer kwikWG.Done()
			kwikSem <- struct{}{}
			defer func() { <-kwikSem }()
			if m3u8, err := p.extractKwikM3u8(kwikLink); err == nil && m3u8 != "" {
				m3u8ByIndex[i] = m3u8
			}
		}(i, kwikLink)
	}
	kwikWG.Wait()

	var finalLinks []string
	for _, m3u8 := range m3u8ByIndex {
		if m3u8 != "" {
			finalLinks = append(finalLinks, m3u8)
		}
	}

	if len(finalLinks) == 0 {
		return nil, fmt.Errorf("failed to extract any %s stream links from kwik", requestedMode)
	}

	return finalLinks, nil
}

func (p *Provider) extractKwikM3u8(kwikUrl string) (string, error) {
	req, err := newAnimepahePageRequest(kwikUrl)
	if err != nil {
		return "", err
	}

	resp, err := curdhost.HTTPClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := readAnimepaheBody(resp)
	if err != nil {
		return "", err
	}
	if !curdhost.HTTPStatusOK(resp.StatusCode) {
		return "", curdhost.HTTPStatusError("kwik stream page", resp.StatusCode, body)
	}
	bodyStr := string(body)

	matches := kwikEvalRe.FindAllStringSubmatch(bodyStr, -1)

	for _, match := range matches {
		pStr := match[1]
		a, _ := strconv.Atoi(match[2])
		c, _ := strconv.Atoi(match[3])
		k := strings.Split(match[4], "|")

		pStr = strings.ReplaceAll(pStr, "\\'", "'")
		unpacked := unpackKwik(pStr, a, c, k)

		if strings.Contains(unpacked, "m3u8") {
			urlMatch := kwikM3u8Re.FindStringSubmatch(unpacked)
			if len(urlMatch) > 1 {
				return urlMatch[1], nil
			}
		}
	}
	return "", fmt.Errorf("m3u8 link not found in kwik page")
}

func unpackKwik(p string, a int, c int, k []string) string {
	e := func(c int) string {
		var helper func(c int) string
		helper = func(c int) string {
			res := ""
			if c >= a {
				res += helper(c / a)
			}
			mod := c % a
			if mod > 35 {
				res += string(rune(mod + 29))
			} else {
				res += strconv.FormatInt(int64(mod), 36)
			}
			return res
		}
		return helper(c)
	}

	d := make(map[string]string)
	for i := c - 1; i >= 0; i-- {
		val := k[i]
		if val == "" {
			val = e(i)
		}
		d[e(i)] = val
	}

	unpacked := kwikTokenRe.ReplaceAllStringFunc(p, func(match string) string {
		if val, ok := d[match]; ok && val != "" {
			return val
		}
		return match
	})

	return unpacked
}

func contains(slice []string, val string) bool {
	for _, item := range slice {
		if item == val {
			return true
		}
	}
	return false
}
