package internal

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
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
)

var animepaheCookiesBypassed bool
func getCookieFilePath() string {
	return filepath.Join(GetStoragePath(), "animepahe_cookies.json")
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
	json.Unmarshal(b, &cookies)
	return cookies
}

func checkCookiesValid() bool {
	req, _ := http.NewRequest("GET", "https://animepahe.pw/api?m=search&q=test", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://animepahe.pw/")
	req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	resp, err := sharedHTTPClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return false
	}
	
	contentType := resp.Header.Get("Content-Type")
	return strings.Contains(contentType, "application/json")
}

func (p *AnimepaheProvider) ensureBypass() error {
	if animepaheCookiesBypassed {
		return nil
	}

	cookies := loadCookies()
	if len(cookies) > 0 {
		u, _ := url.Parse("https://animepahe.pw")
		SetCookiesForAnimepahe(u, cookies)
		
		if checkCookiesValid() {
			animepaheCookiesBypassed = true
			Log("Successfully restored Animepahe session from cache.")
			return nil
		}
		Log("Cached Animepahe session expired. Requesting new session...")
	}

	CurdOut("Solving Animepahe DDoS-Guard challenge via headless browser...")
	
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
		return fmt.Errorf("failed to acquire DDoS guard cookies: %v", err)
	}

	u, _ := url.Parse("https://animepahe.pw")
	var httpCookies []*http.Cookie
	for _, cookie := range rodCookies {
		httpCookies = append(httpCookies, &http.Cookie{
			Name:  cookie.Name,
			Value: cookie.Value,
		})
	}
	SetCookiesForAnimepahe(u, httpCookies)
	
	if !checkCookiesValid() {
		return fmt.Errorf("bypassed cookies are still invalid")
	}

	saveCookies(httpCookies)
	animepaheCookiesBypassed = true
	CurdOut("Successfully bypassed DDoS-Guard.")
	return nil
}

type AnimepaheProvider struct{}

func (p *AnimepaheProvider) Name() string {
	return "animepahe"
}

type AnimepaheSearchItem struct {
	ID      int     `json:"id"`
	Title   string  `json:"title"`
	Type    string  `json:"type"`
	Episodes int    `json:"episodes"`
	Status  string  `json:"status"`
	Season  string  `json:"season"`
	Year    int     `json:"year"`
	Score   float64 `json:"score"`
	Poster  string  `json:"poster"`
	Session string  `json:"session"`
}

type animepaheSearchResponse struct {
	Total       int `json:"total"`
	PerPage     int `json:"per_page"`
	CurrentPage int `json:"current_page"`
	LastPage    int `json:"last_page"`
	Data        []AnimepaheSearchItem `json:"data"`
}

func (p *AnimepaheProvider) SearchAnime(query, mode string) ([]SelectionOption, error) {
	if err := p.ensureBypass(); err != nil {
		return nil, err
	}
	// animepahe doesn't distinguish sub/dub at the search level
	searchUrl := fmt.Sprintf("https://animepahe.pw/api?m=search&q=%s", url.QueryEscape(query))
	
	req, _ := http.NewRequest("GET", searchUrl, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://animepahe.pw/")
	req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	
	resp, err := sharedHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var searchResp animepaheSearchResponse
	err = json.Unmarshal(body, &searchResp)
	if err != nil {
		return nil, err
	}

	var result []SelectionOption
	for _, item := range searchResp.Data {
		label := fmt.Sprintf("%s (%d episodes) [animepahe]", item.Title, item.Episodes)
		result = append(result, SelectionOption{
			Title:     item.Title,
			Label:     label,
			Key:       item.Session, // Using session as ID
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

func (p *AnimepaheProvider) EpisodesList(showID, mode string) ([]string, error) {
	if err := p.ensureBypass(); err != nil {
		return nil, err
	}
	// showID here is the session ID for animepahe
	var allEpisodes []int
	page := 1

	for {
		epUrl := fmt.Sprintf("https://animepahe.pw/api?m=release&id=%s&sort=episode_asc&page=%d", showID, page)
		req, _ := http.NewRequest("GET", epUrl, nil)
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
		req.Header.Set("Referer", "https://animepahe.pw/")
		req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
		
		resp, err := sharedHTTPClient.Do(req)
		if err != nil {
			return nil, err
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}

		var epsResp animepaheEpisodesResponse
		err = json.Unmarshal(body, &epsResp)
		if err != nil {
			return nil, err
		}

		for _, ep := range epsResp.Data {
			allEpisodes = append(allEpisodes, ep.Episode)
		}

		if epsResp.CurrentPage >= epsResp.LastPage {
			break
		}
		page++
	}

	sort.Ints(allEpisodes)
	var result []string
	for _, ep := range allEpisodes {
		result = append(result, strconv.Itoa(ep))
	}

	return result, nil
}

func (p *AnimepaheProvider) GetEpisodeURL(config CurdConfig, id string, epNo int) ([]string, error) {
	if err := p.ensureBypass(); err != nil {
		return nil, err
	}
	// 1. Get episode session ID
	// Map the 1-based Anilist episode number to Animepahe's actual episode number
	eps, err := p.EpisodesList(id, "")
	if err != nil {
		return nil, err
	}
	if epNo < 1 || epNo > len(eps) {
		return nil, fmt.Errorf("episode %d out of bounds (found %d episodes)", epNo, len(eps))
	}
	
	mappedEpNo, err := strconv.Atoi(eps[epNo-1])
	if err != nil {
		return nil, fmt.Errorf("invalid episode number format in provider: %v", err)
	}

	// Let's do a simple scan for the episode session:
	var episodeSession string
	page := 1
	for {
		reqUrl := fmt.Sprintf("https://animepahe.pw/api?m=release&id=%s&sort=episode_asc&page=%d", id, page)
		req, _ := http.NewRequest("GET", reqUrl, nil)
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
		req.Header.Set("Referer", "https://animepahe.pw/")
		req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
		
		resp, err := sharedHTTPClient.Do(req)
		if err != nil {
			return nil, err
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}

		var epsResp animepaheEpisodesResponse
		err = json.Unmarshal(body, &epsResp)
		if err != nil {
			return nil, err
		}

		for _, item := range epsResp.Data {
			if item.Episode == mappedEpNo {
				episodeSession = item.Session
				break
			}
		}

		if episodeSession != "" || epsResp.CurrentPage >= epsResp.LastPage {
			break
		}
		page++
	}

	if episodeSession == "" {
		return nil, fmt.Errorf("episode %d (mapped to %d) not found", epNo, mappedEpNo)
	}
	
	// player page: https://animepahe.pw/play/<anime_session>/<episode_session>
	// stream links in player page: <button type=\"button\" data-src=\"https://kwik.cx/e/Jwd0hMNswksj\"
	playerUrl := fmt.Sprintf("https://animepahe.pw/play/%s/%s", id, episodeSession)
	req, _ := http.NewRequest("GET", playerUrl, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://animepahe.pw/")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	
	resp, err := sharedHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
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
	if config.SubOrDub == "dub" && len(dubLinks) > 0 {
		for _, l := range dubLinks {
			links = append(links, l.url)
		}
	} else if len(subLinks) > 0 {
		for _, l := range subLinks {
			links = append(links, l.url)
		}
	} else if len(dubLinks) > 0 {
		for _, l := range dubLinks {
			links = append(links, l.url)
		}
	}
	
	var finalLinks []string
	for _, kwikLink := range links {
		m3u8, err := p.extractKwikM3u8(kwikLink)
		if err == nil && m3u8 != "" {
			finalLinks = append(finalLinks, m3u8)
		}
	}
	
	if len(finalLinks) == 0 {
		return nil, fmt.Errorf("failed to extract any stream links from kwik")
	}

	return finalLinks, nil
}

func (p *AnimepaheProvider) extractKwikM3u8(kwikUrl string) (string, error) {
	req, _ := http.NewRequest("GET", kwikUrl, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://animepahe.pw/")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	
	resp, err := sharedHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	bodyStr := string(body)
	
	re := regexp.MustCompile(`eval\(function\(p,a,c,k,e,d\).*?\}\('(.*?)',(\d+),(\d+),'([^']+)'\.split\('\|'\).*?\)\)`)
	matches := re.FindAllStringSubmatch(bodyStr, -1)
	
	for _, match := range matches {
		pStr := match[1]
		a, _ := strconv.Atoi(match[2])
		c, _ := strconv.Atoi(match[3])
		k := strings.Split(match[4], "|")
		
		pStr = strings.ReplaceAll(pStr, "\\'", "'")
		unpacked := unpackKwik(pStr, a, c, k)
		
		if strings.Contains(unpacked, "m3u8") {
			urlRe := regexp.MustCompile(`(https://[a-zA-Z0-9\-\.\/]+m3u8)`)
			urlMatch := urlRe.FindStringSubmatch(unpacked)
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

	re := regexp.MustCompile(`[a-zA-Z0-9_]+`)
	unpacked := re.ReplaceAllStringFunc(p, func(match string) string {
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