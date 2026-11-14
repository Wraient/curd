package anikoto

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/wraient/curd/internal/curdhost"
	"github.com/wraient/curd/internal/providers"
)

var (
	anikotoNameAnchorPattern = regexp.MustCompile(`(?is)<a\b([^>]*\bclass\s*=\s*["'][^"']*\bname\b[^"']*["'][^>]*)>(.*?)</a>`)
	anikotoPosterPattern     = regexp.MustCompile(`(?is)<div\b[^>]*\bclass\s*=\s*["'][^"']*\bposter\b[^"']*["'][^>]*>`)
	anikotoImagePattern      = regexp.MustCompile(`(?is)<img\b[^>]*>`)
	anikotoWatchSlugPattern  = regexp.MustCompile(`(?i)/watch/([^/?#]+)`)
)

func searchAnime(query, mode string) ([]providers.SelectionOption, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("empty search query")
	}
	_ = mode

	params := url.Values{}
	params.Set("keyword", query)
	params.Set("page", "1")
	params.Set("vrf", encryptVRFRaw(query))
	raw, err := fetchResource(baseURL+"/filter?"+params.Encode(), baseURL+"/", "text/html,application/xhtml+xml", false)
	if err != nil {
		return nil, err
	}

	options := parseSearchOptions(string(raw))
	if len(options) == 0 {
		return nil, fmt.Errorf("no Anikoto results for %q", query)
	}
	return options, nil
}

func parseSearchOptions(body string) []providers.SelectionOption {
	searchBody := body
	for _, location := range htmlTagPattern.FindAllStringIndex(body, -1) {
		attrs := parseHTMLAttrs(body[location[0]:location[1]])
		if strings.HasPrefix(strings.ToLower(body[location[0]:location[1]]), "<div") && htmlHasClass(attrs, "ani") && htmlHasClass(attrs, "items") {
			searchBody = body[location[1]:]
			break
		}
	}
	if navigation := strings.Index(searchBody, "<nav"); navigation >= 0 {
		searchBody = searchBody[:navigation]
	}

	tags := htmlTagPattern.FindAllStringIndex(searchBody, -1)
	itemStarts := make([]int, 0)
	for _, location := range tags {
		tag := searchBody[location[0]:location[1]]
		attrs := parseHTMLAttrs(tag)
		if strings.HasPrefix(strings.ToLower(tag), "<div") && htmlHasClass(attrs, "item") {
			itemStarts = append(itemStarts, location[0])
		}
	}

	options := make([]providers.SelectionOption, 0, len(itemStarts))
	seen := make(map[string]struct{})
	for index, start := range itemStarts {
		end := len(searchBody)
		if index+1 < len(itemStarts) {
			end = itemStarts[index+1]
		}
		block := searchBody[start:end]
		nameMatch := anikotoNameAnchorPattern.FindStringSubmatch(block)
		if len(nameMatch) < 3 {
			continue
		}
		anchorAttrs := parseHTMLAttrs(nameMatch[1])
		slug := anikotoWatchSlugPattern.FindStringSubmatch(anchorAttrs["href"])
		if len(slug) < 2 {
			continue
		}
		publicID := strings.TrimSpace(slug[1])
		if publicID == "" {
			continue
		}

		animeID := 0
		if poster := anikotoPosterPattern.FindString(block); poster != "" {
			animeID, _ = strconv.Atoi(parseHTMLAttrs(poster)["data-tip"])
		}
		key := publicID
		if animeID > 0 {
			key += "#" + strconv.Itoa(animeID)
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}

		englishTitle := cleanHTMLText(nameMatch[2])
		japaneseTitle := cleanHTMLText(anchorAttrs["data-jp"])
		title := englishTitle
		if curdhost.AnimeNameLanguage != nil && strings.EqualFold(curdhost.AnimeNameLanguage(), "romaji") && japaneseTitle != "" {
			title = japaneseTitle
		}
		if title == "" {
			continue
		}

		thumbnail := ""
		if image := anikotoImagePattern.FindString(block); image != "" {
			imageAttrs := parseHTMLAttrs(image)
			thumbnail = absoluteAnikotoURL(imageAttrs["data-src"])
			if thumbnail == "" {
				thumbnail = absoluteAnikotoURL(imageAttrs["src"])
			}
		}
		options = append(options, providers.SelectionOption{
			Key:       key,
			Label:     title,
			Title:     title,
			Thumbnail: thumbnail,
		})
	}
	return options
}
