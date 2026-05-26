package internal

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func testHTTPResponse(req *http.Request, statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}
}

func withMyAnimeListTokenTestHooks(t *testing.T, client *http.Client) {
	t.Helper()

	previousClient := sharedHTTPClient
	previousAuth := authenticateMyAnimeList
	t.Cleanup(func() {
		sharedHTTPClient = previousClient
		authenticateMyAnimeList = previousAuth
	})

	sharedHTTPClient = client
}

func TestRefreshMyAnimeListTokenPreservesRefreshTokenWhenResponseOmitsOne(t *testing.T) {
	config := &CurdConfig{
		StoragePath:         t.TempDir(),
		MyAnimeListClientID: "client-id",
	}
	tokenPath := myAnimeListTokenPath(config)
	if err := saveToken(tokenPath, &OAuthToken{
		AccessToken:  "old-access",
		RefreshToken: "keep-refresh",
		ExpiresAt:    time.Now().Add(-time.Hour),
	}); err != nil {
		t.Fatalf("save expired token: %v", err)
	}

	withMyAnimeListTokenTestHooks(t, &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != myAnimeListTokenURL {
			t.Fatalf("unexpected request URL: %s", req.URL.String())
		}
		if err := req.ParseForm(); err != nil {
			t.Fatalf("parse refresh form: %v", err)
		}
		if got := req.Form.Get("refresh_token"); got != "keep-refresh" {
			t.Fatalf("refresh token = %q, want keep-refresh", got)
		}
		return testHTTPResponse(req, http.StatusOK, `{"access_token":"new-access","token_type":"Bearer","expires_in":3600}`), nil
	})})

	accessToken, err := GetMyAnimeListAccessToken(config)
	if err != nil {
		t.Fatalf("refresh access token: %v", err)
	}
	if accessToken != "new-access" {
		t.Fatalf("access token = %q, want new-access", accessToken)
	}

	storedToken, err := loadToken(tokenPath)
	if err != nil {
		t.Fatalf("load refreshed token: %v", err)
	}
	if storedToken.RefreshToken != "keep-refresh" {
		t.Fatalf("stored refresh token = %q, want keep-refresh", storedToken.RefreshToken)
	}
}

func TestGetMyAnimeListAccessTokenReauthenticatesWhenRefreshFails(t *testing.T) {
	config := &CurdConfig{
		StoragePath:         t.TempDir(),
		MyAnimeListClientID: "client-id",
	}
	tokenPath := myAnimeListTokenPath(config)
	if err := saveToken(tokenPath, &OAuthToken{
		AccessToken:  "expired-access",
		RefreshToken: "bad-refresh",
		ExpiresAt:    time.Now().Add(-time.Hour),
	}); err != nil {
		t.Fatalf("save expired token: %v", err)
	}

	withMyAnimeListTokenTestHooks(t, &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return testHTTPResponse(req, http.StatusBadRequest, `{"error":"invalid_grant"}`), nil
	})})
	authenticateMyAnimeList = func(config *CurdConfig, tokenPath string) (string, error) {
		token := &OAuthToken{
			AccessToken:  "reauth-access",
			RefreshToken: "reauth-refresh",
			ExpiresIn:    3600,
			ExpiresAt:    time.Now().Add(time.Hour),
		}
		if err := saveToken(tokenPath, token); err != nil {
			return "", err
		}
		return token.AccessToken, nil
	}

	accessToken, err := GetMyAnimeListAccessToken(config)
	if err != nil {
		t.Fatalf("reauthenticate access token: %v", err)
	}
	if accessToken != "reauth-access" {
		t.Fatalf("access token = %q, want reauth-access", accessToken)
	}
}

func TestMyAnimeListRequestReauthenticatesWhenUnauthorizedRefreshFails(t *testing.T) {
	config := &CurdConfig{
		StoragePath:         t.TempDir(),
		MyAnimeListClientID: "client-id",
	}
	tokenPath := myAnimeListTokenPath(config)
	if err := saveToken(tokenPath, &OAuthToken{
		AccessToken:  "revoked-access",
		RefreshToken: "bad-refresh",
		ExpiresAt:    time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("save revoked token fixture: %v", err)
	}

	apiAttempts := 0
	withMyAnimeListTokenTestHooks(t, &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.URL.String() == myAnimeListTokenURL:
			return testHTTPResponse(req, http.StatusBadRequest, `{"error":"invalid_grant"}`), nil
		case strings.HasPrefix(req.URL.String(), myAnimeListAPIBaseURL):
			apiAttempts++
			authHeader := req.Header.Get("Authorization")
			if apiAttempts == 1 {
				if authHeader != "Bearer revoked-access" {
					t.Fatalf("first API Authorization = %q, want revoked access", authHeader)
				}
				return testHTTPResponse(req, http.StatusUnauthorized, `{"error":"invalid_token"}`), nil
			}
			if authHeader != "Bearer reauth-access" {
				t.Fatalf("retried API Authorization = %q, want reauth access", authHeader)
			}
			return testHTTPResponse(req, http.StatusOK, `{"id":123,"name":"tester"}`), nil
		default:
			t.Fatalf("unexpected request URL: %s", req.URL.String())
			return nil, nil
		}
	})})
	authenticateMyAnimeList = func(config *CurdConfig, tokenPath string) (string, error) {
		token := &OAuthToken{
			AccessToken:  "reauth-access",
			RefreshToken: "reauth-refresh",
			ExpiresIn:    3600,
			ExpiresAt:    time.Now().Add(time.Hour),
		}
		if err := saveToken(tokenPath, token); err != nil {
			return "", err
		}
		return token.AccessToken, nil
	}

	var response myAnimeListUser
	if err := myAnimeListRequest(config, http.MethodGet, "/users/@me", url.Values{"fields": {"id,name"}}, &response); err != nil {
		t.Fatalf("request should reauthenticate and retry: %v", err)
	}
	if response.ID != 123 || response.Name != "tester" {
		t.Fatalf("unexpected response: %+v", response)
	}
	if apiAttempts != 2 {
		t.Fatalf("API attempts = %d, want 2", apiAttempts)
	}
}
