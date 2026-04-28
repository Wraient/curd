package internal

import (
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"time"
)

var sharedHTTPClient *http.Client

func SetCookiesForAnimepahe(u *url.URL, cookies []*http.Cookie) {
	if sharedHTTPClient != nil && sharedHTTPClient.Jar != nil {
		sharedHTTPClient.Jar.SetCookies(u, cookies)
	}
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
