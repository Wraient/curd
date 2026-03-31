package internal

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
)

const releaseStateFileName = "release_state.json"

type githubRelease struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
}

type releaseState struct {
	LatestVersion string    `json:"latest_version"`
	ReleaseURL    string    `json:"release_url,omitempty"`
	CheckedAt     time.Time `json:"checked_at"`
}

func ResolveCurrentVersion(rawVersion string) string {
	version := normalizeVersion(rawVersion)
	if version != "" {
		return version
	}

	buildInfo, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}

	return normalizeVersion(buildInfo.Main.Version)
}

func NotifyAboutCachedUpdate(currentVersion, storagePath string) {
	currentVersion = normalizeVersion(currentVersion)
	if currentVersion == "" {
		return
	}

	state, err := loadReleaseState(storagePath)
	if err != nil {
		return
	}

	if compareVersions(state.LatestVersion, currentVersion) <= 0 {
		return
	}

	CurdOut(buildUpdatePrompt(currentVersion, state.LatestVersion))
}

func StartBackgroundReleaseCheck(currentVersion, storagePath string) {
	currentVersion = normalizeVersion(currentVersion)
	if currentVersion == "" {
		return
	}

	go func() {
		if err := refreshReleaseState(currentVersion, storagePath); err != nil {
			Log(fmt.Sprintf("Background release check failed: %v", err))
		}
	}()
}

func refreshReleaseState(currentVersion, storagePath string) error {
	release, err := fetchLatestRelease("wraient/curd", currentVersion)
	if err != nil {
		return err
	}

	state := releaseState{
		LatestVersion: normalizeVersion(release.TagName),
		ReleaseURL:    release.HTMLURL,
		CheckedAt:     time.Now().UTC(),
	}
	if state.LatestVersion == "" {
		return fmt.Errorf("latest release did not include a version tag")
	}

	return saveReleaseState(storagePath, state)
}

func UpdateCurd(repo string) (string, error) {
	executablePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("unable to find current executable: %w", err)
	}

	binaryName, err := currentReleaseBinaryName()
	if err != nil {
		return "", err
	}

	tmpPath := executablePath + ".tmp"
	currentVersion := ResolveCurrentVersion("")
	if err := downloadReleaseAsset(repo, binaryName, tmpPath, currentVersion); err != nil {
		return "", err
	}

	switch runtime.GOOS {
	case "windows":
		if err := scheduleWindowsBinarySwap(executablePath, tmpPath); err != nil {
			return "", wrapUpdateError(err)
		}
		return "Update downloaded. It will be installed after Curd exits. Open Curd again after this window closes.", nil
	default:
		if err := os.Rename(tmpPath, executablePath); err != nil {
			_ = os.Remove(tmpPath)
			return "", wrapUpdateError(fmt.Errorf("failed to replace executable: %w", err))
		}
		return "Program updated!", nil
	}
}

func currentReleaseBinaryName() (string, error) {
	switch runtime.GOOS {
	case "windows":
		if runtime.GOARCH == "arm64" {
			return "curd-windows-arm64.exe", nil
		}
		return "curd-windows-x86_64.exe", nil
	case "darwin":
		switch runtime.GOARCH {
		case "amd64":
			return "curd-macos-x86_64", nil
		case "arm64":
			return "curd-macos-arm64", nil
		default:
			return "curd-macos-universal", nil
		}
	case "linux":
		switch runtime.GOARCH {
		case "amd64":
			return "curd-linux-x86_64", nil
		case "arm64":
			return "curd-linux-arm64", nil
		default:
			return "", fmt.Errorf("unsupported Linux architecture: %s", runtime.GOARCH)
		}
	default:
		return "", fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}

func downloadReleaseAsset(repo, binaryName, destinationPath, currentVersion string) error {
	url := fmt.Sprintf("https://github.com/%s/releases/latest/download/%s", repo, binaryName)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create download request: %w", err)
	}

	req.Header.Set("User-Agent", userAgentForVersion(currentVersion))

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download file: received status code %d", resp.StatusCode)
	}

	out, err := os.Create(destinationPath)
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %w", err)
	}

	if err := out.Chmod(0755); err != nil {
		out.Close()
		_ = os.Remove(destinationPath)
		return fmt.Errorf("failed to set file permissions: %w", err)
	}

	if _, err := io.Copy(out, resp.Body); err != nil {
		out.Close()
		_ = os.Remove(destinationPath)
		return fmt.Errorf("failed to write to temporary file: %w", err)
	}

	if err := out.Close(); err != nil {
		_ = os.Remove(destinationPath)
		return fmt.Errorf("failed to close temporary file: %w", err)
	}

	return nil
}

func fetchLatestRelease(repo, currentVersion string) (githubRelease, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo), nil)
	if err != nil {
		return githubRelease{}, fmt.Errorf("failed to create release check request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", userAgentForVersion(currentVersion))

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return githubRelease{}, fmt.Errorf("failed to check latest release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return githubRelease{}, fmt.Errorf("release check failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return githubRelease{}, fmt.Errorf("failed to decode latest release response: %w", err)
	}

	return release, nil
}

func scheduleWindowsBinarySwap(executablePath, tmpPath string) error {
	scriptPath := filepath.Join(os.TempDir(), fmt.Sprintf("curd-update-%d.cmd", time.Now().UnixNano()))
	script := fmt.Sprintf(`@echo off
setlocal
set "TARGET=%s"
set "SOURCE=%s"
set "OLD=%s.old"
:wait_loop
tasklist /FI "PID eq %d" 2>NUL | find "%d" >NUL
if not errorlevel 1 (
    timeout /T 1 /NOBREAK >NUL
    goto wait_loop
)
if exist "%%OLD%%" del /F /Q "%%OLD%%" >NUL 2>NUL
if exist "%%TARGET%%" move /Y "%%TARGET%%" "%%OLD%%" >NUL
move /Y "%%SOURCE%%" "%%TARGET%%" >NUL
if exist "%%OLD%%" del /F /Q "%%OLD%%" >NUL 2>NUL
del /F /Q "%%~f0" >NUL 2>NUL
`, executablePath, tmpPath, executablePath, os.Getpid(), os.Getpid())

	if err := os.WriteFile(scriptPath, []byte(script), 0600); err != nil {
		return fmt.Errorf("failed to create Windows update script: %w", err)
	}

	cmd := exec.Command("cmd", "/C", "start", "", "/B", scriptPath)
	cmd.Dir = filepath.Dir(executablePath)
	if err := cmd.Start(); err != nil {
		_ = os.Remove(scriptPath)
		return fmt.Errorf("failed to launch Windows updater: %w", err)
	}

	return nil
}

func saveReleaseState(storagePath string, state releaseState) error {
	statePath, err := releaseStatePath(storagePath)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(statePath), 0755); err != nil {
		return fmt.Errorf("failed to create release cache directory: %w", err)
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal release state: %w", err)
	}

	if err := os.WriteFile(statePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write release state: %w", err)
	}

	return nil
}

func loadReleaseState(storagePath string) (releaseState, error) {
	statePath, err := releaseStatePath(storagePath)
	if err != nil {
		return releaseState{}, err
	}

	data, err := os.ReadFile(statePath)
	if err != nil {
		return releaseState{}, err
	}

	var state releaseState
	if err := json.Unmarshal(data, &state); err != nil {
		return releaseState{}, err
	}

	state.LatestVersion = normalizeVersion(state.LatestVersion)
	return state, nil
}

func releaseStatePath(storagePath string) (string, error) {
	expandedStorage := os.ExpandEnv(strings.TrimSpace(storagePath))
	if expandedStorage == "" {
		return "", fmt.Errorf("storage path is empty")
	}

	return filepath.Join(expandedStorage, releaseStateFileName), nil
}

func buildUpdatePrompt(currentVersion, latestVersion string) string {
	switch runtime.GOOS {
	case "windows":
		return fmt.Sprintf("A newer Curd release (%s) is available. You're on %s. Update it with the same package manager you used to install Curd, or run `curd -u` if you're using the standalone executable.", latestVersion, currentVersion)
	case "darwin":
		return fmt.Sprintf("A newer Curd release (%s) is available. You're on %s. Update it with the same package manager you used to install Curd, or run `sudo curd -u` if you installed the standalone binary in a protected location.", latestVersion, currentVersion)
	default:
		return fmt.Sprintf("A newer Curd release (%s) is available. You're on %s. Update it with the same package manager you used to install Curd, or run `sudo curd -u` for the standalone binary.", latestVersion, currentVersion)
	}
}

func compareVersions(leftVersion, rightVersion string) int {
	leftParts := versionParts(leftVersion)
	rightParts := versionParts(rightVersion)
	maxParts := len(leftParts)
	if len(rightParts) > maxParts {
		maxParts = len(rightParts)
	}

	for i := 0; i < maxParts; i++ {
		left := 0
		right := 0
		if i < len(leftParts) {
			left = leftParts[i]
		}
		if i < len(rightParts) {
			right = rightParts[i]
		}
		if left > right {
			return 1
		}
		if left < right {
			return -1
		}
	}

	return 0
}

func versionParts(version string) []int {
	cleanVersion := normalizeVersion(version)
	if cleanVersion == "" {
		return nil
	}

	segments := strings.FieldsFunc(cleanVersion, func(r rune) bool {
		return r < '0' || r > '9'
	})

	parts := make([]int, 0, len(segments))
	for _, segment := range segments {
		if segment == "" {
			continue
		}

		value, err := strconv.Atoi(segment)
		if err != nil {
			return nil
		}
		parts = append(parts, value)
	}

	return parts
}

func normalizeVersion(version string) string {
	trimmed := strings.TrimSpace(version)
	trimmed = strings.TrimPrefix(trimmed, "v")
	if trimmed == "" || trimmed == "(devel)" {
		return ""
	}

	if len(versionPartsWithoutNormalization(trimmed)) == 0 {
		return ""
	}

	return trimmed
}

func versionPartsWithoutNormalization(version string) []int {
	segments := strings.FieldsFunc(version, func(r rune) bool {
		return r < '0' || r > '9'
	})

	parts := make([]int, 0, len(segments))
	for _, segment := range segments {
		if segment == "" {
			continue
		}

		value, err := strconv.Atoi(segment)
		if err != nil {
			return nil
		}
		parts = append(parts, value)
	}

	return parts
}

func userAgentForVersion(currentVersion string) string {
	currentVersion = normalizeVersion(currentVersion)
	if currentVersion == "" {
		return "curd"
	}

	return "curd/" + currentVersion
}

func wrapUpdateError(err error) error {
	if err == nil {
		return nil
	}

	message := strings.ToLower(err.Error())
	if errors.Is(err, os.ErrPermission) || strings.Contains(message, "permission denied") || strings.Contains(message, "access is denied") {
		switch runtime.GOOS {
		case "windows":
			return fmt.Errorf("%w. Try running your terminal as Administrator, use the package manager you installed Curd with, or move the standalone executable somewhere writable before retrying `curd -u`", err)
		case "darwin":
			return fmt.Errorf("%w. Try updating with your package manager or rerun the standalone updater with `sudo curd -u` if Curd is installed in a protected location", err)
		default:
			return fmt.Errorf("%w. Try updating with your package manager or rerun the standalone updater with `sudo curd -u`", err)
		}
	}

	return err
}
