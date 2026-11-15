package anipm

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/wraient/curd/internal/curdhost"
)

const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

var (
	baseURL      = "https://ani.pm"
	embedBaseURL = "https://embed.settlar.io"
)

func newRequest(rawURL, referer string) (*http.Request, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	return req, nil
}

func fetchJSON(rawURL, referer string, dest any) error {
	body, err := fetchBytes(rawURL, referer, nil)
	if err != nil {
		return err
	}
	if dest == nil {
		return nil
	}
	if err := json.Unmarshal(body, dest); err != nil {
		return fmt.Errorf("parse anipm response: %w", err)
	}
	return nil
}

func fetchBytes(rawURL, referer string, cookies []*http.Cookie) ([]byte, error) {
	body, _, err := fetch(rawURL, referer, cookies)
	return body, err
}

// fetch performs a GET via the host HTTP client and returns the body plus
// any cookies the server set (the settlar embed session is cookie-bound).
func fetch(rawURL, referer string, cookies []*http.Cookie) ([]byte, []*http.Cookie, error) {
	req, err := newRequest(rawURL, referer)
	if err != nil {
		return nil, nil, err
	}
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}

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
		return nil, nil, curdhost.HTTPStatusError("anipm request", resp.StatusCode, body)
	}
	return body, resp.Cookies(), nil
}

func searchURL(query string) string {
	return fmt.Sprintf("%s/api/anime/search?q=%s", baseURL, url.QueryEscape(strings.TrimSpace(query)))
}

func seriesURL(showID string) string {
	return fmt.Sprintf("%s/api/anime/series/%s", baseURL, url.PathEscape(strings.TrimSpace(showID)))
}

func aniURL(showID string) string {
	return fmt.Sprintf("%s/api/anime/ani/%s?phase=core", baseURL, url.PathEscape(strings.TrimSpace(showID)))
}

func bootstrapURL(showID string, epNo int, lang string) string {
	return fmt.Sprintf("%s/api/anime/playback-bootstrap/settlar/%s?ep=%d&lang=%s",
		baseURL, url.PathEscape(strings.TrimSpace(showID)), epNo, url.QueryEscape(lang))
}

func settlarSessionURL(selection string, epNo int, channel string) string {
	query := url.Values{}
	query.Set("selection", selection)
	query.Set("provider", "anipm")
	query.Set("ep", fmt.Sprintf("%d", epNo))
	query.Set("channel", channel)
	query.Set("telemetry", "0")
	return fmt.Sprintf("%s/api/anime/settlar/session?%s", baseURL, query.Encode())
}
