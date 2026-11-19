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

// sessionCall is one in-flight verify handshake shared by concurrent
// waiters so a cold start costs a single handshake, not one per goroutine.
type sessionCall struct {
	done  chan struct{}
	token string
	exp   int64
	err   error
}

var (
	sessionInflightMu sync.Mutex
	sessionInflight   *sessionCall
)

// episodeCounts remembers per-show sub/dub totals briefly. Search results
// already carry both counts, so an EpisodesList right after a search (the
// normal host flow) skips the detail round trip. Entries live 5 minutes;
// a miss falls back to the detail endpoint exactly as before.
type episodeCounts struct {
	sub     int
	dub     int
	expires int64
}

var (
	epCountMu sync.Mutex
	epCounts  = map[string]episodeCounts{}
)

// episodeCountTTLMillis bounds how long search-provided episode totals are
// reused before the detail endpoint is consulted again.
const episodeCountTTLMillis = 5 * 60 * 1000

// maxEpisodeCountEntries bounds the cache; expired entries are dropped on
// store and the whole map is shed if it still exceeds the cap, so a long
// session cannot accumulate one entry per show forever.
const maxEpisodeCountEntries = 256

func episodeCountsKey(showID string) string {
	return baseURL + "\x00" + showID
}

func episodeCountsFromCache(showID string) (sub, dub int, ok bool) {
	epCountMu.Lock()
	defer epCountMu.Unlock()
	cached, found := epCounts[episodeCountsKey(showID)]
	if !found || time.Now().UnixMilli() >= cached.expires {
		return 0, 0, false
	}
	return cached.sub, cached.dub, true
}

func storeEpisodeCounts(showID string, sub, dub int) {
	// Zero counts mean the source omitted the totals, not that the show
	// has no episodes: leave them uncached so the detail endpoint still
	// gets consulted instead of failing the lookup for 5 minutes.
	if sub <= 0 && dub <= 0 {
		return
	}
	now := time.Now().UnixMilli()
	epCountMu.Lock()
	defer epCountMu.Unlock()
	for key, entry := range epCounts {
		if now >= entry.expires {
			delete(epCounts, key)
		}
	}
	if len(epCounts) >= maxEpisodeCountEntries {
		epCounts = map[string]episodeCounts{}
	}
	epCounts[episodeCountsKey(showID)] = episodeCounts{sub: sub, dub: dub, expires: now + episodeCountTTLMillis}
}

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
	// Drain the two response bytes so the CDN connection is released back
	// to the pool instead of being discarded: the next probe to the same
	// CDN host then reuses it instead of paying for a fresh TLS handshake.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64))
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
	// Join an in-flight handshake instead of starting a duplicate one:
	// concurrent cold starts share one verify request. The fetch itself
	// still runs unlocked so a slow verify never blocks cache readers.
	sessionInflightMu.Lock()
	if sessionInflight != nil {
		call := sessionInflight
		sessionInflightMu.Unlock()
		<-call.done
		if call.err != nil {
			return "", call.err
		}
		return call.token, nil
	}
	call := &sessionCall{done: make(chan struct{})}
	sessionInflight = call
	sessionInflightMu.Unlock()

	token, exp, err := fetchSessionToken()
	if err == nil {
		sessionMu.Lock()
		sessionToken, sessionExp = token, exp
		sessionMu.Unlock()
		call.token, call.exp = token, exp
	} else {
		call.err = err
	}
	sessionInflightMu.Lock()
	sessionInflight = nil
	sessionInflightMu.Unlock()
	close(call.done)
	if err != nil {
		return "", err
	}
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
	sessionToken = ""
	sessionExp = 0
	sessionMu.Unlock()
	// The episode-count cache is also a short-lived session artifact, so
	// tests start from the same cold state production does.
	epCountMu.Lock()
	epCounts = map[string]episodeCounts{}
	epCountMu.Unlock()
}

func setSessionForTest(token string, exp int64) {
	sessionMu.Lock()
	defer sessionMu.Unlock()
	sessionToken = token
	sessionExp = exp
}
