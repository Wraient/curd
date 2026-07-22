package internal

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
)

var curdVersion = "dev"

// SetCurdVersion records the running application version for storage migrations.
func SetCurdVersion(version string) {
	version = strings.TrimSpace(version)
	if version != "" {
		curdVersion = version
	}
}

// CurdVersion returns the running application version.
func CurdVersion() string {
	if curdVersion == "" {
		return "dev"
	}
	return curdVersion
}

// GetLatestVersion returns the latest version in the repo release page.
// If repo is empty, returns an error.
func GetLatestVersion(repo string) (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)

	resp, err := sharedHTTPClient.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to fetch releases: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch releases: status %d", resp.StatusCode)
	}

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", fmt.Errorf("failed to decode release JSON: %w", err)
	}

	return release.TagName, nil
}

func storageVersionFilePath(storagePath string) string {
	return filepath.Join(strings.TrimSpace(storagePath), "curd_version")
}
