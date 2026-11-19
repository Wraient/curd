package aniwaves

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/wraient/curd/internal/providers"
)

var (
	itemBlockRE = regexp.MustCompile(`(?s)<a class="item"[^>]*>(.*?)</a>`)
	itemLinkRE  = regexp.MustCompile(`href="(/watch/[^"]+)"`)
	itemTitleRE = regexp.MustCompile(`d-title[^>]*>([^<]+)<`)
	itemJpRE    = regexp.MustCompile(`data-jp="([^"]+)"`)
	itemTypeRE  = regexp.MustCompile(`<span class="dot">([^<]+)</span>`)
	itemThumbRE = regexp.MustCompile(`<img src="([^"]+)"`)
)

// parseSearchItem extracts one result's fields from its own <a class="item">
// block, so a result missing a field can never shift its neighbors' data.
func parseSearchItem(block string) (path, title, jp, typ, thumb string) {
	if match := itemLinkRE.FindStringSubmatch(block); match != nil {
		path = strings.TrimSpace(match[1])
	}
	if match := itemTitleRE.FindStringSubmatch(block); match != nil {
		title = htmlUnescape(strings.TrimSpace(match[1]))
	}
	if match := itemJpRE.FindStringSubmatch(block); match != nil {
		jp = htmlUnescape(strings.TrimSpace(match[1]))
	}
	if match := itemTypeRE.FindStringSubmatch(block); match != nil {
		typ = strings.TrimSpace(match[1])
	}
	if match := itemThumbRE.FindStringSubmatch(block); match != nil {
		thumb = strings.TrimSpace(match[1])
	}
	return path, title, jp, typ, thumb
}

func searchAnime(query, mode string) ([]providers.SelectionOption, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("empty search query")
	}
	_ = mode

	html, err := postSearch(query)
	if err != nil {
		return nil, err
	}
	html = htmlUnescape(html)

	blocks := itemBlockRE.FindAllString(html, -1)
	if len(blocks) == 0 {
		return nil, fmt.Errorf("no results for %q", query)
	}

	options := make([]providers.SelectionOption, 0, len(blocks))
	for _, block := range blocks {
		path, title, jp, typ, thumb := parseSearchItem(block)
		slug := strings.TrimPrefix(strings.TrimSpace(path), "/watch/")
		if slug == "" {
			continue
		}
		if title == "" {
			title = slug
		}
		label := title
		if typ != "" {
			label += " · " + typ
		}
		if showID := showNumericID(slug); showID != "" {
			label += " #" + showID
		}
		options = append(options, providers.SelectionOption{
			Key:       slug,
			Label:     label,
			Title:     title,
			Thumbnail: thumb,
			ExtraData: SearchItem{
				Slug:          slug,
				Title:         title,
				JapaneseTitle: jp,
				Type:          typ,
			},
		})
	}
	if len(options) == 0 {
		return nil, fmt.Errorf("no results for %q", query)
	}
	return options, nil
}

// htmlEscaper is hoisted to package level so bulk search HTML decoding and
// per-field decoding share one compiled replacer instead of rebuilding it on
// every htmlUnescape call (once per search plus twice per result block).
var htmlEscaper = strings.NewReplacer(
	`&#039;`, "'",
	`&quot;`, "\"",
	`&amp;`, "&",
	`&lt;`, "<",
	`&gt;`, ">",
	`\/`, "/",
)

// htmlUnescape unescapes the HTML entities the ajax endpoint escapes.
func htmlUnescape(s string) string {
	return htmlEscaper.Replace(s)
}

// showNumericID extracts the trailing numeric show id from a watch slug
// (e.g. /watch/sousou-no-frieren-74575 -> 74575), used for episode and
// server lookups.
func showNumericID(slug string) string {
	slug = strings.Trim(strings.TrimSpace(slug), "/")
	parts := strings.Split(slug, "-")
	if len(parts) == 0 {
		return ""
	}
	last := parts[len(parts)-1]
	if _, err := strconv.Atoi(last); err != nil {
		return ""
	}
	return last
}
