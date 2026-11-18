package anilight

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/wraient/curd/internal/curdhost"
)

const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

var (
	baseURL      = "https://api.anilight.live"
	referrer     = "https://anilight.live/"
	streamCDN    = "https://media.satella.buzz"
	sessionToken = ""
	sessionExp   int64
	sessionMu    sync.Mutex
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
	req, err := newRequest(rawURL, referer)
	if err != nil {
		return err
	}
	resp, err := curdhost.HTTPClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if !curdhost.HTTPStatusOK(resp.StatusCode) {
		return curdhost.HTTPStatusError("anilight request", resp.StatusCode, body)
	}
	if dest == nil {
		return nil
	}
	if err := json.Unmarshal(body, dest); err != nil {
		return fmt.Errorf("parse anilight response: %w", err)
	}
	return nil
}

// doRequest executes a prepared request through the host HTTP client,
// enforcing the host's success-status contract on the body.
func doRequest(req *http.Request) ([]byte, error) {
	resp, err := curdhost.HTTPClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if !curdhost.HTTPStatusOK(resp.StatusCode) {
		return nil, curdhost.HTTPStatusError("anilight request", resp.StatusCode, body)
	}
	return body, nil
}

func decodeJSON(body []byte, dest any) error {
	if dest == nil {
		return nil
	}
	if err := json.Unmarshal(body, dest); err != nil {
		return fmt.Errorf("parse anilight response: %w", err)
	}
	return nil
}

// probeStreamHead issues a one-byte range request so dead CDN links are
// rejected before mpv ever sees them. mp4 Range responses come back as
// 200 or 206; anything else is a dead link.
func probeStreamHead(streamURL string) error {
	req, err := newRequest(streamURL, referrer)
	if err != nil {
		return err
	}
	req.Header.Set("Range", "bytes=0-1")
	resp, err := curdhost.HTTPClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("stream probe returned status %d", resp.StatusCode)
	}
	return nil
}

func searchURL(query string) string {
	return fmt.Sprintf("%s/api/search?q=%s", baseURL, url.QueryEscape(strings.TrimSpace(query)))
}

func sourcesURL(slug string, epNo int, lang string) string {
	query := url.Values{}
	query.Set("slug", strings.TrimSpace(slug))
	query.Set("episode", fmt.Sprintf("%d", epNo))
	query.Set("language", lang)
	return fmt.Sprintf("%s/api/sources?%s", baseURL, query.Encode())
}

// sessionTokenForSources exchanges the public verify handshake for a
// per-session token the sources endpoint requires. Tokens are cached until
// shortly before expiry; the handshake needs no login or captcha. Only the
// cache is guarded by the mutex: the handshake itself runs unlocked so one
// slow verify call cannot stall other stream resolutions.
func sessionTokenForSources() (string, error) {
	sessionMu.Lock()
	cached, expiry := sessionToken, sessionExp
	sessionMu.Unlock()
	if cached != "" && time.Now().UnixMilli() < expiry-300000 {
		return cached, nil
	}
	token, exp, err := fetchSessionToken()
	if err != nil {
		return "", err
	}
	sessionMu.Lock()
	sessionToken, sessionExp = token, exp
	sessionMu.Unlock()
	return token, nil
}

// invalidateSessionToken drops the cached token so the next lookup runs a
// fresh handshake. Used when the sources endpoint rejects the token the
// cache still considers valid.
func invalidateSessionToken() {
	sessionMu.Lock()
	sessionToken, sessionExp = "", 0
	sessionMu.Unlock()
}

func fetchSessionToken() (string, int64, error) {
	payload, _ := json.Marshal(map[string]string{"token": "direct"})
	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/sources/verify", bytes.NewReader(payload))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Referer", referrer)
	resp, err := curdhost.HTTPClient().Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return "", 0, err
	}
	if !curdhost.HTTPStatusOK(resp.StatusCode) {
		return "", 0, curdhost.HTTPStatusError("anilight session", resp.StatusCode, body)
	}
	var session verifyResponse
	if err := json.Unmarshal(body, &session); err != nil {
		return "", 0, fmt.Errorf("parse anilight session: %w", err)
	}
	if strings.TrimSpace(session.SessionToken) == "" {
		return "", 0, fmt.Errorf("anilight returned an empty session token")
	}
	return strings.TrimSpace(session.SessionToken), session.ExpiresAt, nil
}

func resetSessionForTest() {
	sessionMu.Lock()
	defer sessionMu.Unlock()
	sessionToken = ""
	sessionExp = 0
}

func setSessionForTest(token string, exp int64) {
	sessionMu.Lock()
	defer sessionMu.Unlock()
	sessionToken = token
	sessionExp = exp
}
