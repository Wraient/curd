package internal

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func isInvalidTokenResponse(statusCode int, body []byte) bool {
	lower := strings.ToLower(string(body))
	if strings.Contains(lower, "invalid token") || strings.Contains(lower, "invalid_token") {
		return true
	}
	return statusCode == http.StatusUnauthorized
}

func isInvalidTokenError(err error) bool {
	if err == nil {
		return false
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "invalid token") || strings.Contains(lower, "invalid_token")
}

func anilistTokenPath(config *CurdConfig) string {
	return filepath.Join(os.ExpandEnv(config.StoragePath), "anilist_token.json")
}

func ReauthenticateAniList(config *CurdConfig, user *User, reason string) (string, error) {
	if config == nil {
		config = GetGlobalConfig()
	}
	if config == nil {
		return "", fmt.Errorf("missing config for AniList reauthentication")
	}

	tokenPath := anilistTokenPath(config)
	if reason != "" {
		CurdOut(fmt.Sprintf("AniList sign-in required (%s). Opening browser...", reason))
		Log(fmt.Sprintf("AniList token invalid (%s), starting browser authentication", reason))
	} else {
		CurdOut("AniList sign-in required. Opening browser...")
		Log("AniList token invalid, starting browser authentication")
	}

	if err := os.Remove(tokenPath); err != nil && !os.IsNotExist(err) {
		Log(fmt.Sprintf("Failed to clear stale AniList token file: %v", err))
	}

	accessToken, err := authenticateWithBrowser(tokenPath)
	if err != nil {
		return "", err
	}

	if user != nil {
		user.Token = accessToken
	} else if globalUser := GetGlobalUser(); globalUser != nil {
		globalUser.Token = accessToken
	}

	return accessToken, nil
}

func tryRenewAniListTokenForAPI(currentToken string, reason string) (string, bool, error) {
	config := GetGlobalConfig()
	if config == nil {
		return currentToken, false, nil
	}

	newToken, err := ReauthenticateAniList(config, GetGlobalUser(), reason)
	if err != nil {
		return currentToken, false, err
	}
	return newToken, true, nil
}

func clearStoredTokenFile(tokenPath string) {
	if err := os.Remove(tokenPath); err != nil && !os.IsNotExist(err) {
		Log(fmt.Sprintf("Failed to clear stale token file %s: %v", tokenPath, err))
	}
}
