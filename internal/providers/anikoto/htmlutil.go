package anikoto

import (
	"html"
	"regexp"
	"strings"
)

var (
	htmlTagPattern    = regexp.MustCompile(`(?is)<[a-z][^>]*>`)
	htmlAttrPattern   = regexp.MustCompile(`([a-zA-Z_:][-a-zA-Z0-9_:.]*)\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s>]+))`)
	htmlBreakPattern  = regexp.MustCompile(`(?i)<br\s*/?>`)
	htmlAnyTagPattern = regexp.MustCompile(`(?is)<[^>]+>`)
)

func parseHTMLAttrs(tag string) map[string]string {
	attrs := make(map[string]string)
	for _, match := range htmlAttrPattern.FindAllStringSubmatch(tag, -1) {
		value := match[2]
		if value == "" {
			value = match[3]
		}
		if value == "" {
			value = match[4]
		}
		attrs[strings.ToLower(match[1])] = html.UnescapeString(value)
	}
	return attrs
}

func htmlHasClass(attrs map[string]string, class string) bool {
	for _, value := range strings.Fields(attrs["class"]) {
		if value == class {
			return true
		}
	}
	return false
}

func cleanHTMLText(value string) string {
	value = htmlBreakPattern.ReplaceAllString(value, "\n")
	value = htmlAnyTagPattern.ReplaceAllString(value, " ")
	value = html.UnescapeString(value)
	return strings.Join(strings.Fields(value), " ")
}

func absoluteAnikotoURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	if strings.HasPrefix(rawURL, "http://") || strings.HasPrefix(rawURL, "https://") {
		return rawURL
	}
	return baseURL + "/" + strings.TrimLeft(rawURL, "/")
}
