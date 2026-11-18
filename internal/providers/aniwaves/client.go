package aniwaves

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
	baseURL  = "https://aniwaves.tv"
	referrer = "https://aniwaves.tv/"
)

func newRequest(rawURL, referer string) (*http.Request, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	return req, nil
}

// checkAjaxStatus rejects an explicit non-200 ajax status. A missing status
// field is tolerated: some list endpoints omit it on success, so only an
// explicit error status fails fast instead of surfacing later as a confusing
// unmarshal or empty-list error.
func checkAjaxStatus(payload *ajaxEnvelope, what string) error {
	if payload.Status != 0 && payload.Status != 200 {
		return fmt.Errorf("aniwaves %s failed with status %d", what, payload.Status)
	}
	return nil
}

// fetchBytes performs a GET via the host HTTP client and returns the body
// plus any cookies the server set. The ajax search/episode/server/sources
// endpoints are stateless and need no cookies; only the vhscdn embed page
// sets a session cookie, which is threaded through to the getSources call.
func fetchBytes(rawURL, referer string, cookies []*http.Cookie) ([]byte, []*http.Cookie, error) {
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
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, nil, err
	}
	if !curdhost.HTTPStatusOK(resp.StatusCode) {
		return nil, nil, curdhost.HTTPStatusError("aniwaves request", resp.StatusCode, body)
	}
	return body, resp.Cookies(), nil
}

func postSearch(query string) (string, error) {
	form := url.Values{}
	form.Set("keyword", strings.TrimSpace(query))
	req, err := http.NewRequest(http.MethodPost, baseURL+"/ajax/anime/search", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Referer", baseURL+"/home")
	resp, err := curdhost.HTTPClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", err
	}
	if !curdhost.HTTPStatusOK(resp.StatusCode) {
		return "", curdhost.HTTPStatusError("aniwaves search", resp.StatusCode, body)
	}
	var payload ajaxEnvelope
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("parse aniwaves search: %w", err)
	}
	if err := checkAjaxStatus(&payload, "search"); err != nil {
		return "", err
	}
	var result searchPayload
	if err := json.Unmarshal(payload.Result, &result); err != nil {
		return "", fmt.Errorf("parse aniwaves search result: %w", err)
	}
	return result.HTML, nil
}
