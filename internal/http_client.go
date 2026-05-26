package internal

import (
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

var sharedHTTPClient *http.Client

func SetCookiesForAnimepahe(u *url.URL, cookies []*http.Cookie) {
	if sharedHTTPClient != nil && sharedHTTPClient.Jar != nil {
		sharedHTTPClient.Jar.SetCookies(u, cookies)
	}
}

func httpStatusOK(statusCode int) bool {
	return statusCode >= http.StatusOK && statusCode < http.StatusMultipleChoices
}

func httpStatusError(context string, statusCode int, body []byte) error {
	snippet := strings.TrimSpace(string(body))
	if len(snippet) > 300 {
		snippet = snippet[:300] + "..."
	}
	if snippet != "" {
		return fmt.Errorf("%s failed with status %d: %s", context, statusCode, snippet)
	}
	return fmt.Errorf("%s failed with status %d", context, statusCode)
}

func init() {
	jar, _ := cookiejar.New(nil)
	sharedHTTPClient = &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:        10,
			MaxIdleConnsPerHost: 5,
			IdleConnTimeout:     30 * time.Second,
		},
		Timeout: 15 * time.Second,
		Jar:     jar,
	}
}
