package animepahe

import (
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/wraient/curd/internal/curdhost"
)

const animepaheChallengeHTML = `<!doctype html><html><head><title>DDoS-Guard</title></head><body>Checking your browser</body></html>`

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func animepaheTestResponse(req *http.Request, statusCode int, body string, headers map[string]string) *http.Response {
	resp := &http.Response{
		StatusCode: statusCode,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}
	for key, value := range headers {
		resp.Header.Set(key, value)
	}
	return resp
}

func withAnimepaheTestHooks(t *testing.T, client *http.Client, solver func() ([]*http.Cookie, error)) {
	t.Helper()

	previousClient := curdhost.HTTPClient
	previousLog := curdhost.Log
	previousOut := curdhost.Out
	previousCookies := curdhost.SetCookiesForAnimepahe
	previousSolver := solveAnimepaheBrowserChallenge
	previousBypassed := animepaheCookiesBypassed
	previousStoragePath := curdhost.StoragePath

	t.Cleanup(func() {
		curdhost.HTTPClient = previousClient
		curdhost.Log = previousLog
		curdhost.Out = previousOut
		curdhost.SetCookiesForAnimepahe = previousCookies
		solveAnimepaheBrowserChallenge = previousSolver
		animepaheCookiesBypassed = previousBypassed
		curdhost.StoragePath = previousStoragePath
	})

	curdhost.HTTPClient = func() *http.Client { return client }
	curdhost.Log = func(string) {}
	curdhost.Out = func(string) {}
	curdhost.SetCookiesForAnimepahe = func(*url.URL, []*http.Cookie) {}
	solveAnimepaheBrowserChallenge = solver
	curdhost.StoragePath = func() string { return t.TempDir() }
}

func TestAnimepaheEpisodesListRefreshesSessionWhenReleaseEndpointReturnsChallenge(t *testing.T) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("create cookie jar: %v", err)
	}

	var releaseCalls int
	var searchChecks int
	var solverCalls int
	client := &http.Client{Jar: jar, Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Query().Get("m") {
		case "release":
			releaseCalls++
			if releaseCalls == 1 {
				return animepaheTestResponse(req, http.StatusForbidden, animepaheChallengeHTML, map[string]string{
					"Content-Type": "text/html; charset=UTF-8",
					"Server":       "ddos-guard",
				}), nil
			}
			return animepaheTestResponse(req, http.StatusOK, `{"total":2,"per_page":30,"current_page":1,"last_page":1,"data":[{"episode":1},{"episode":2}]}`, map[string]string{
				"Content-Type": "application/json",
			}), nil
		case "search":
			searchChecks++
			return animepaheTestResponse(req, http.StatusOK, `{"total":0,"per_page":8,"current_page":1,"last_page":1,"data":[]}`, map[string]string{
				"Content-Type": "application/json",
			}), nil
		default:
			t.Fatalf("unexpected animepahe request: %s", req.URL.String())
			return nil, nil
		}
	})}
	withAnimepaheTestHooks(t, client, func() ([]*http.Cookie, error) {
		solverCalls++
		return []*http.Cookie{{Name: "__ddg2_", Value: "fresh"}}, nil
	})
	animepaheCookiesBypassed = true

	episodes, err := (&Provider{}).EpisodesList("4:session-id", "sub")
	if err != nil {
		t.Fatalf("expected episode list retry to succeed: %v", err)
	}
	if got := strings.Join(episodes, ","); got != "1,2" {
		t.Fatalf("unexpected animepahe episodes: %s", got)
	}
	if releaseCalls != 2 {
		t.Fatalf("expected release endpoint to be retried once, got %d calls", releaseCalls)
	}
	if searchChecks != 1 {
		t.Fatalf("expected refreshed session validation, got %d checks", searchChecks)
	}
	if solverCalls != 1 {
		t.Fatalf("expected one browser challenge refresh, got %d", solverCalls)
	}
}

func TestAnimepaheEpisodesListReportsPersistentChallengeAfterRefresh(t *testing.T) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("create cookie jar: %v", err)
	}

	var releaseCalls int
	client := &http.Client{Jar: jar, Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Query().Get("m") {
		case "release":
			releaseCalls++
			return animepaheTestResponse(req, http.StatusForbidden, animepaheChallengeHTML, map[string]string{
				"Content-Type": "text/html; charset=UTF-8",
				"Server":       "ddos-guard",
			}), nil
		case "search":
			return animepaheTestResponse(req, http.StatusOK, `{"total":0,"per_page":8,"current_page":1,"last_page":1,"data":[]}`, map[string]string{
				"Content-Type": "application/json",
			}), nil
		default:
			t.Fatalf("unexpected animepahe request: %s", req.URL.String())
			return nil, nil
		}
	})}
	withAnimepaheTestHooks(t, client, func() ([]*http.Cookie, error) {
		return []*http.Cookie{{Name: "__ddg2_", Value: "fresh"}}, nil
	})
	animepaheCookiesBypassed = true

	_, err = (&Provider{}).EpisodesList("4:session-id", "sub")
	if err == nil {
		t.Fatalf("expected persistent DDoS-Guard challenge to fail")
	}
	if !strings.Contains(err.Error(), "still blocked by DDoS-Guard") {
		t.Fatalf("unexpected error: %v", err)
	}
	if releaseCalls != 2 {
		t.Fatalf("expected exactly one retry after challenge refresh, got %d release calls", releaseCalls)
	}
}

func TestAnimepaheEpisodesListLiveOnePiece(t *testing.T) {
	if os.Getenv("CURD_LIVE_ANIMEPAHE_TEST") != "1" {
		t.Skip("set CURD_LIVE_ANIMEPAHE_TEST=1 to run live Animepahe episode-list verification")
	}

	previousBypassed := animepaheCookiesBypassed
	previousStoragePath := curdhost.StoragePath
	t.Cleanup(func() {
		animepaheCookiesBypassed = previousBypassed
		curdhost.StoragePath = previousStoragePath
	})
	curdhost.StoragePath = func() string { return t.TempDir() }
	animepaheCookiesBypassed = false

	episodes, err := (&Provider{}).EpisodesList("4:15a36fc8-73b9-8c2e-ee5f-fb1bcd4fcdc6", "sub")
	if err != nil {
		t.Fatalf("live Animepahe One Piece episode list failed: %v", err)
	}
	t.Logf("live Animepahe One Piece episode count: %d", len(episodes))
	if len(episodes) < 1000 {
		t.Fatalf("expected One Piece to return a large live episode list, got %d episodes", len(episodes))
	}
}
