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
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

const (
	defaultUpdateRepo          = "Wraient/curd"
	updatePendingFileName      = "update_pending.json"
	backgroundUpdateIdleDelay  = 4 * time.Second
	defaultRemindLaterDuration = 24 * time.Hour
	maxReleaseNotesRunes       = 1200
)

// updatePendingState is persisted under StoragePath so startup never blocks on
// network — a previous idle check stores availability for the next launch.
type updatePendingState struct {
	Available      bool   `json:"available"`
	LatestVersion  string `json:"latest_version"`
	LatestTag      string `json:"latest_tag"`
	ReleaseName    string `json:"release_name"`
	ReleaseNotes   string `json:"release_notes"`
	HTMLURL        string `json:"html_url"`
	AssetName      string `json:"asset_name"`
	CheckedAt      string `json:"checked_at"`
	SkippedVersion string `json:"skipped_version,omitempty"`
	RemindAfter    string `json:"remind_after,omitempty"`
}

type githubReleaseAPI struct {
	TagName string `json:"tag_name"`
	Name    string `json:"name"`
	Body    string `json:"body"`
	HTMLURL string `json:"html_url"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

var backgroundUpdateOnce sync.Once

func updatePendingPath(storagePath string) string {
	if strings.TrimSpace(storagePath) == "" {
		storagePath = GetStoragePath()
	}
	return filepath.Join(os.ExpandEnv(storagePath), updatePendingFileName)
}

func loadUpdatePendingState(storagePath string) updatePendingState {
	path := updatePendingPath(storagePath)
	data, err := os.ReadFile(path)
	if err != nil {
		return updatePendingState{}
	}
	var state updatePendingState
	if err := json.Unmarshal(data, &state); err != nil {
		Log(fmt.Sprintf("Ignoring corrupt update state: %v", err))
		return updatePendingState{}
	}
	return state
}

func saveUpdatePendingState(storagePath string, state updatePendingState) error {
	path := updatePendingPath(storagePath)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func curdReleaseBinaryName() (string, error) {
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

func normalizeReleaseVersion(tag string) string {
	tag = strings.TrimSpace(tag)
	tag = strings.TrimPrefix(tag, "v")
	tag = strings.TrimPrefix(tag, "V")
	return tag
}

func truncateReleaseNotes(notes string) string {
	notes = strings.TrimSpace(notes)
	if notes == "" {
		return "(No release notes provided.)"
	}
	runes := []rune(notes)
	if len(runes) <= maxReleaseNotesRunes {
		return notes
	}
	return string(runes[:maxReleaseNotesRunes]) + "\n… (truncated)"
}

func fetchLatestGitHubRelease(repo string) (githubReleaseAPI, error) {
	if strings.TrimSpace(repo) == "" {
		repo = defaultUpdateRepo
	}
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return githubReleaseAPI{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "curd-update-check")

	client := sharedHTTPClient
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return githubReleaseAPI{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return githubReleaseAPI{}, fmt.Errorf("github releases API status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var release githubReleaseAPI
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return githubReleaseAPI{}, err
	}
	if strings.TrimSpace(release.TagName) == "" {
		return githubReleaseAPI{}, fmt.Errorf("latest release has no tag")
	}
	return release, nil
}

func isUpdateNewer(latest, current string) bool {
	latest = normalizeReleaseVersion(latest)
	current = normalizeReleaseVersion(current)
	if latest == "" || current == "" {
		return false
	}
	return versionLess(current, latest)
}

// StartBackgroundUpdateCheck runs after a short idle delay so startup is not blocked.
// Results are written to StoragePath/update_pending.json for the next launch.
func StartBackgroundUpdateCheck(config *CurdConfig, currentVersion string) {
	if config == nil || !config.CheckUpdates {
		return
	}
	backgroundUpdateOnce.Do(func() {
		go func() {
			time.Sleep(backgroundUpdateIdleDelay)
			if err := checkForUpdateInBackground(config, currentVersion); err != nil {
				Log(fmt.Sprintf("Background update check failed: %v", err))
			}
		}()
	})
}

func checkForUpdateInBackground(config *CurdConfig, currentVersion string) error {
	storagePath := config.StoragePath
	state := loadUpdatePendingState(storagePath)

	// Honor "remind later" without hitting the network repeatedly if still waiting.
	if state.RemindAfter != "" {
		if until, err := time.Parse(time.RFC3339, state.RemindAfter); err == nil && time.Now().Before(until) {
			Log(fmt.Sprintf("Skipping update check until %s", state.RemindAfter))
			return nil
		}
	}

	release, err := fetchLatestGitHubRelease(defaultUpdateRepo)
	if err != nil {
		return err
	}
	latest := normalizeReleaseVersion(release.TagName)
	state.CheckedAt = time.Now().UTC().Format(time.RFC3339)
	state.LatestTag = release.TagName
	state.LatestVersion = latest
	state.ReleaseName = strings.TrimSpace(release.Name)
	if state.ReleaseName == "" {
		state.ReleaseName = "Curd " + latest
	}
	state.ReleaseNotes = truncateReleaseNotes(release.Body)
	state.HTMLURL = release.HTMLURL

	if asset, assetErr := curdReleaseBinaryName(); assetErr == nil {
		state.AssetName = asset
	}

	if state.SkippedVersion != "" && normalizeReleaseVersion(state.SkippedVersion) == latest {
		state.Available = false
		return saveUpdatePendingState(storagePath, state)
	}

	if !isUpdateNewer(latest, currentVersion) {
		state.Available = false
		return saveUpdatePendingState(storagePath, state)
	}

	state.Available = true
	// Clear remind-later once a check found something actionable after the window.
	if state.RemindAfter != "" {
		if until, err := time.Parse(time.RFC3339, state.RemindAfter); err == nil && !time.Now().Before(until) {
			state.RemindAfter = ""
		}
	}
	Log(fmt.Sprintf("Update available: %s → %s", currentVersion, latest))
	return saveUpdatePendingState(storagePath, state)
}

func pendingUpdateShouldPrompt(config *CurdConfig, currentVersion string, state updatePendingState) bool {
	if config == nil || !config.CheckUpdates {
		return false
	}
	if !state.Available || strings.TrimSpace(state.LatestVersion) == "" {
		return false
	}
	if !isUpdateNewer(state.LatestVersion, currentVersion) {
		return false
	}
	if normalizeReleaseVersion(state.SkippedVersion) == normalizeReleaseVersion(state.LatestVersion) {
		return false
	}
	if state.RemindAfter != "" {
		if until, err := time.Parse(time.RFC3339, state.RemindAfter); err == nil && time.Now().Before(until) {
			return false
		}
	}
	return true
}

func formatLocalTime(t time.Time) string {
	return t.In(time.Local).Format("Mon Jan 2 2006, 3:04 PM MST")
}

func escapePango(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

var (
	mdBoldRe  = regexp.MustCompile(`\*\*(.+?)\*\*`)
	mdCodeRe  = regexp.MustCompile("`([^`]+)`")
	mdLinkRe  = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	mdURLRe   = regexp.MustCompile(`https?://[^\s<>\]]+`)
	ansiStrip = regexp.MustCompile(`\x1b\[[0-9;]*m`)
	pangoStrip = regexp.MustCompile(`<[^>]*>`)
)

// markdownToPango turns common GitHub release markdown into Rofi-friendly Pango.
func markdownToPango(md string) string {
	lines := strings.Split(strings.ReplaceAll(md, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "":
			out = append(out, "")
			continue
		case strings.HasPrefix(trimmed, "### "):
			text := escapePango(strings.TrimPrefix(trimmed, "### "))
			out = append(out, `<span foreground="#FFD166"><b>`+text+`</b></span>`)
		case strings.HasPrefix(trimmed, "## "):
			text := escapePango(strings.TrimPrefix(trimmed, "## "))
			out = append(out, `<span foreground="#7CB9E8" size="large"><b>`+text+`</b></span>`)
		case strings.HasPrefix(trimmed, "# "):
			text := escapePango(strings.TrimPrefix(trimmed, "# "))
			out = append(out, `<span foreground="#7CFC98" size="large"><b>`+text+`</b></span>`)
		case strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* "):
			body := strings.TrimPrefix(strings.TrimPrefix(trimmed, "- "), "* ")
			out = append(out, `<span foreground="#98FB98">•</span> `+inlineMarkdownToPango(body))
		case strings.HasPrefix(trimmed, "**Full Changelog**") || strings.HasPrefix(strings.ToLower(trimmed), "**full changelog**"):
			out = append(out, `<span foreground="#B0B0B0">`+inlineMarkdownToPango(trimmed)+`</span>`)
		default:
			out = append(out, inlineMarkdownToPango(trimmed))
		}
	}
	return strings.Join(out, "\n")
}

func inlineMarkdownToPango(s string) string {
	// Links first (before escaping full string piece by piece)
	s = mdLinkRe.ReplaceAllStringFunc(s, func(m string) string {
		parts := mdLinkRe.FindStringSubmatch(m)
		if len(parts) != 3 {
			return escapePango(m)
		}
		return `<span foreground="#6EC6FF" underline="single">` + escapePango(parts[1]) + `</span>`
	})
	// Escape remaining raw text while preserving spans we inserted — do a simple pass:
	// split on existing span tags is hard; re-process from original for bold/code on non-link text.
	// Safer path: escape whole line then re-apply patterns on escaped text where ** still present.
	if !strings.Contains(s, "<span") {
		s = escapePango(s)
		s = mdBoldRe.ReplaceAllString(s, `<span foreground="#FFFFFF"><b>$1</b></span>`)
		s = mdCodeRe.ReplaceAllString(s, `<span foreground="#E0B0FF" face="monospace">$1</span>`)
		s = mdURLRe.ReplaceAllStringFunc(s, func(u string) string {
			return `<span foreground="#6EC6FF" underline="single">` + u + `</span>`
		})
		return s
	}
	// Already has link spans — only lightly touch remaining ** if any outside tags
	s = mdBoldRe.ReplaceAllString(s, `<b>$1</b>`)
	return s
}

// markdownToTerminal colors release notes for CLI (lipgloss), easy on the eyes.
func markdownToTerminal(md string) string {
	heading := lipgloss.NewStyle().Foreground(lipgloss.Color("#7CB9E8")).Bold(true)
	subhead := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFD166")).Bold(true)
	bullet := lipgloss.NewStyle().Foreground(lipgloss.Color("#98FB98"))
	body := lipgloss.NewStyle().Foreground(lipgloss.Color("#E6E6FA"))
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color("#9A9A9A"))
	link := lipgloss.NewStyle().Foreground(lipgloss.Color("#6EC6FF")).Underline(true)

	lines := strings.Split(strings.ReplaceAll(md, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "":
			out = append(out, "")
		case strings.HasPrefix(trimmed, "### "):
			out = append(out, subhead.Render(strings.TrimPrefix(trimmed, "### ")))
		case strings.HasPrefix(trimmed, "## "):
			out = append(out, heading.Render(strings.TrimPrefix(trimmed, "## ")))
		case strings.HasPrefix(trimmed, "# "):
			out = append(out, heading.Render(strings.TrimPrefix(trimmed, "# ")))
		case strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* "):
			item := strings.TrimPrefix(strings.TrimPrefix(trimmed, "- "), "* ")
			item = mdBoldRe.ReplaceAllString(item, "$1")
			item = mdLinkRe.ReplaceAllString(item, "$1")
			out = append(out, bullet.Render("• ")+body.Render(item))
		case strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://"):
			out = append(out, link.Render(trimmed))
		case strings.Contains(strings.ToLower(trimmed), "full changelog"):
			out = append(out, muted.Render(mdLinkRe.ReplaceAllString(trimmed, "$1 ($2)")))
		default:
			t := mdBoldRe.ReplaceAllString(trimmed, "$1")
			t = mdLinkRe.ReplaceAllString(t, "$1")
			out = append(out, body.Render(t))
		}
	}
	return strings.Join(out, "\n")
}

func buildUpdatePromptMessage(currentVersion string, state updatePendingState) (prompt, message string) {
	return buildUpdatePromptMessageMode(currentVersion, state, false)
}

// buildUpdatePromptMessageMode formats the update banner.
// forRofi=true emits Pango markup for Rofi -mesg; false emits lipgloss ANSI for the terminal.
func buildUpdatePromptMessageMode(currentVersion string, state updatePendingState, forRofi bool) (prompt, message string) {
	from := normalizeReleaseVersion(currentVersion)
	to := normalizeReleaseVersion(state.LatestVersion)
	prompt = fmt.Sprintf("✨ Update %s → %s", from, to)

	notes := strings.TrimSpace(state.ReleaseNotes)
	if notes == "" {
		notes = "(No release notes on GitHub for this release.)"
	}

	if forRofi {
		var b strings.Builder
		title := state.ReleaseName
		if title == "" {
			title = "Curd " + to
		}
		b.WriteString(`<span foreground="#7CFC98" size="large"><b>🚀 ` + escapePango(title) + `</b></span>` + "\n")
		b.WriteString(`<span foreground="#E6E6FA">Current </span>`)
		b.WriteString(`<span foreground="#FF8A80"><b>` + escapePango(from) + `</b></span>`)
		b.WriteString(`<span foreground="#E6E6FA">  →  Latest </span>`)
		b.WriteString(`<span foreground="#7CFC98"><b>` + escapePango(to) + `</b></span>` + "\n")
		if state.HTMLURL != "" {
			b.WriteString(`<span foreground="#6EC6FF" underline="single">` + escapePango(state.HTMLURL) + `</span>` + "\n")
		}
		b.WriteString("\n")
		b.WriteString(markdownToPango(notes))
		message = strings.TrimSpace(b.String())
		// Pango is verbose; soft-cap markup length.
		if len([]rune(message)) > maxReleaseNotesRunes*3 {
			r := []rune(message)
			message = string(r[:maxReleaseNotesRunes*3]) + "\n<span foreground=\"#9A9A9A\">… (truncated — full notes on GitHub)</span>"
		}
		return prompt, message
	}

	// CLI / terminal
	title := lipgloss.NewStyle().Foreground(lipgloss.Color("#7CFC98")).Bold(true)
	label := lipgloss.NewStyle().Foreground(lipgloss.Color("#E6E6FA"))
	oldV := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF8A80")).Bold(true)
	newV := lipgloss.NewStyle().Foreground(lipgloss.Color("#7CFC98")).Bold(true)
	link := lipgloss.NewStyle().Foreground(lipgloss.Color("#6EC6FF")).Underline(true)

	var b strings.Builder
	name := state.ReleaseName
	if name == "" {
		name = "Curd " + to
	}
	b.WriteString(title.Render("🚀 "+name) + "\n")
	b.WriteString(label.Render("Current ") + oldV.Render(from) + label.Render("  →  Latest ") + newV.Render(to) + "\n")
	if state.HTMLURL != "" {
		b.WriteString(link.Render(state.HTMLURL) + "\n")
	}
	b.WriteString("\n")
	b.WriteString(markdownToTerminal(notes))
	message = strings.TrimSpace(b.String())
	// Cap plain-ish length for terminal
	plain := ansiStrip.ReplaceAllString(message, "")
	if len([]rune(plain)) > maxReleaseNotesRunes {
		// Keep header + truncated notes roughly
		message = message + "\n" + label.Render("… (truncated — full notes on GitHub)")
	}
	return prompt, message
}

func updateActionOptions() []SelectionOption {
	// Emoji prefixes: lively, ordered, no numeric indices; preserveOrder keeps this order.
	return []SelectionOption{
		{Key: "update", Label: "🚀  Update now"},
		{Key: "later", Label: "⏰  Remind me later"},
		{Key: "skip", Label: "⏭️  Skip this version"},
		{Key: "disable", Label: "🔕  Turn off automatic update checks"},
		{Key: "continue", Label: "▶️  Continue without updating"},
	}
}

// refreshUpdateStateFromGitHub reloads tag/name/body/url from the live release API
// so the prompt shows real markdown notes instead of a stale/test seed.
func refreshUpdateStateFromGitHub(state *updatePendingState) {
	if state == nil {
		return
	}
	release, err := fetchLatestGitHubRelease(defaultUpdateRepo)
	if err != nil {
		Log(fmt.Sprintf("Could not refresh release notes from GitHub: %v", err))
		return
	}
	latest := normalizeReleaseVersion(release.TagName)
	if latest == "" {
		return
	}
	// If GitHub moved past what we flagged, still show the newest release.
	state.LatestTag = release.TagName
	state.LatestVersion = latest
	state.ReleaseName = strings.TrimSpace(release.Name)
	if state.ReleaseName == "" {
		state.ReleaseName = "Curd " + latest
	}
	// Full markdown body from the GitHub release page (API `body` field).
	// Do not seed/test stubs here — always prefer live API content when online.
	state.ReleaseNotes = strings.TrimSpace(release.Body)
	state.HTMLURL = release.HTMLURL
	state.CheckedAt = time.Now().UTC().Format(time.RFC3339)
	if asset, assetErr := curdReleaseBinaryName(); assetErr == nil {
		state.AssetName = asset
	}
	state.Available = true
}

// updateUserMessage prints a single status line. With Rofi mode, CurdOut becomes
// notify-send — so we only send one short notification (or log) for status.
func updateUserMessage(config *CurdConfig, msg string) {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return
	}
	Log(msg)
	if config != nil && config.RofiSelection {
		// One short desktop notification, not a barrage of CurdOut lines.
		_ = exec.Command("notify-send", "-a", "Curd",
			"-h", "string:x-canonical-private-synchronous:curd-update",
			"Curd", msg).Run()
		return
	}
	fmt.Println(msg)
}

// HandlePendingUpdatePrompt shows a previously detected update (from idle check).
// Returns true if the caller should exit (user updated or chose to quit the session).
func HandlePendingUpdatePrompt(config *CurdConfig, currentVersion string) bool {
	if config == nil || !config.CheckUpdates {
		return false
	}
	state := loadUpdatePendingState(config.StoragePath)
	if !pendingUpdateShouldPrompt(config, currentVersion, state) {
		return false
	}

	// Pull live release markdown from GitHub so notes match the release page.
	refreshUpdateStateFromGitHub(&state)
	if !pendingUpdateShouldPrompt(config, currentVersion, state) {
		// e.g. already up to date after refresh
		state.Available = false
		_ = saveUpdatePendingState(config.StoragePath, state)
		return false
	}
	_ = saveUpdatePendingState(config.StoragePath, state)

	// Fixed order (Update now first). Emoji labels; preserveOrder for CLI.
	options := updateActionOptions()
	prompt, message := buildUpdatePromptMessageMode(currentVersion, state, config.RofiSelection)

	var selected SelectionOption
	var err error
	if config.RofiSelection {
		// Pango-colored notes in -mesg; one Rofi UI, no notify spam.
		selected, err = RofiSelectWithMessage(options, false, prompt, message)
	} else {
		header := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFD166")).Bold(true)
		fmt.Println(header.Render(prompt))
		fmt.Println(message)
		fmt.Println()
		selected, err = promptSelectOrdered(options)
	}
	if err != nil {
		return false
	}
	selected = NormalizeSelectionKey(selected)
	// Quit from the pinned menu must exit the whole program (not fall through to curd).
	if SelectionMeansQuit(selected) {
		ExitCurd(nil)
		return true
	}
	// Back / empty = dismiss update prompt and continue the session.
	if SelectionMeansBack(selected) || selected.Key == "continue" || selected.Key == "" {
		return false
	}

	switch selected.Key {
	case "update":
		updateUserMessage(config, "Downloading and installing update…")
		if err := UpdateCurd(defaultUpdateRepo, "curd"); err != nil {
			updateUserMessage(config, fmt.Sprintf("Update failed: %v", err))
			Log(fmt.Sprintf("Update failed: %v", err))
			return false
		}
		state.Available = false
		state.RemindAfter = ""
		_ = saveUpdatePendingState(config.StoragePath, state)
		updateUserMessage(config, fmt.Sprintf("Updated to %s. Please restart curd.", state.LatestVersion))
		return true
	case "later":
		until := time.Now().Add(defaultRemindLaterDuration)
		state.RemindAfter = until.UTC().Format(time.RFC3339)
		state.Available = true
		_ = saveUpdatePendingState(config.StoragePath, state)
		updateUserMessage(config, fmt.Sprintf("Will remind again after %s.", formatLocalTime(until)))
		return false
	case "skip":
		state.SkippedVersion = state.LatestVersion
		state.Available = false
		state.RemindAfter = ""
		_ = saveUpdatePendingState(config.StoragePath, state)
		updateUserMessage(config, fmt.Sprintf("Skipping version %s.", state.LatestVersion))
		return false
	case "disable":
		if err := setConfigBoolOption(GlobalConfigPath, "CheckUpdates", false); err != nil {
			updateUserMessage(config, fmt.Sprintf("Could not write config: %v", err))
			Log(fmt.Sprintf("disable CheckUpdates: %v", err))
		} else {
			config.CheckUpdates = false
			updateUserMessage(config, "Automatic update checks disabled (CheckUpdates=false).")
		}
		state.Available = false
		_ = saveUpdatePendingState(config.StoragePath, state)
		return false
	default:
		return false
	}
}

func setConfigBoolOption(configPath, key string, value bool) error {
	if strings.TrimSpace(configPath) == "" {
		return fmt.Errorf("config path is empty")
	}
	configMap, err := LoadConfigFromFile(configPath)
	if err != nil {
		return err
	}
	_, existed := configMap[key]
	if value {
		configMap[key] = "true"
	} else {
		configMap[key] = "false"
	}
	if !existed {
		return appendConfigKeys(configPath, configMap, []string{key})
	}
	return SaveConfigToFile(configPath, configMap)
}

func isPermissionError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, os.ErrPermission) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "permission denied") ||
		strings.Contains(msg, "access is denied") ||
		strings.Contains(msg, "operation not permitted")
}

// isCrossDeviceError reports rename/link failures when src and dest are on
// different filesystems (e.g. /tmp tmpfs → ~/.local/bin on disk).
func isCrossDeviceError(err error) bool {
	if err == nil {
		return false
	}
	var linkErr *os.LinkError
	if errors.As(err, &linkErr) {
		err = linkErr.Err
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "cross-device") ||
		strings.Contains(msg, "cross device") ||
		strings.Contains(msg, "exdev") ||
		strings.Contains(msg, "invalid cross-device link")
}

func hasDisplay() bool {
	return os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != ""
}

func stdinIsTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// preferGUIPasswordPrompt decides whether to open a desktop password dialog.
//
//   - Rofi/desktop mode → GUI first (zenity/yad/kdialog)
//   - Plain CLI with a TTY (e.g. `curd -u` in a terminal) → terminal only
//   - No TTY but a display → GUI
func preferGUIPasswordPrompt() bool {
	rofi := false
	if cfg := GetGlobalConfig(); cfg != nil {
		rofi = cfg.RofiSelection
	}
	tty := stdinIsTerminal()

	// Interactive CLI: never pop a GUI dialog just because DISPLAY is set.
	if tty && !rofi {
		return false
	}
	if rofi {
		return true
	}
	// Headless of TTY (desktop launcher without a terminal): GUI if available.
	return !tty && hasDisplay()
}

// promptSudoPasswordGUI tries GTK/desktop password dialogs (zenity → yad → kdialog).
func promptSudoPasswordGUI(prompt string) (string, error) {
	if prompt == "" {
		prompt = "Enter your password to install the Curd update:"
	}

	type dialog struct {
		name string
		args []string
	}
	dialogs := []dialog{
		// GTK (GNOME / many desktops)
		{"zenity", []string{"--password", "--title=Curd Update", "--text=" + prompt}},
		// GTK-based yad
		{"yad", []string{"--entry", "--hide-text", "--title=Curd Update", "--text=" + prompt, "--button=OK:0", "--button=Cancel:1"}},
		// KDE
		{"kdialog", []string{"--title", "Curd Update", "--password", prompt}},
	}

	for _, d := range dialogs {
		bin, err := exec.LookPath(d.name)
		if err != nil {
			continue
		}
		cmd := exec.Command(bin, d.args...)
		out, err := cmd.Output()
		if err != nil {
			// User cancel or dialog error — try next tool only on "not found"; cancel stops.
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				// zenity/yad/kdialog: non-zero usually means cancel.
				return "", fmt.Errorf("password dialog cancelled")
			}
			Log(fmt.Sprintf("password dialog %s failed: %v", d.name, err))
			continue
		}
		password := strings.TrimRight(string(out), "\r\n")
		if password == "" {
			return "", fmt.Errorf("empty password")
		}
		Log(fmt.Sprintf("Collected sudo password via %s dialog", d.name))
		return password, nil
	}
	return "", fmt.Errorf("no GUI password dialog available (tried zenity, yad, kdialog)")
}

func promptSudoPassword(prompt string) (string, error) {
	if prompt == "" {
		prompt = "Administrator password (sudo) to install update: "
	}

	rofi := false
	if cfg := GetGlobalConfig(); cfg != nil {
		rofi = cfg.RofiSelection
	}
	tty := stdinIsTerminal()

	// Rofi/desktop: try zenity/yad/kdialog first.
	if preferGUIPasswordPrompt() {
		if password, err := promptSudoPasswordGUI(prompt); err == nil {
			return password, nil
		} else {
			Log(fmt.Sprintf("GUI password prompt unavailable (%v)", err))
			// Explicit cancel in GUI: only abort if we shouldn't fall back to TTY.
			// In rofi mode with no TTY, cancel means cancel.
			if strings.Contains(err.Error(), "cancelled") && (!tty || rofi) {
				// If we still have a real terminal under us (rare with rofi), allow TTY
				// fallback only when not in rofi mode.
				if !tty {
					return "", err
				}
				if rofi {
					// Rofi session: user closed the dialog on purpose.
					return "", err
				}
			}
			// GUI missing/failed → fall through to terminal when possible.
		}
	}

	// Terminal path (CLI `curd -u`, or GUI unavailable).
	if !tty {
		return "", fmt.Errorf("no terminal available for password entry; install zenity/yad/kdialog for a GUI prompt")
	}
	fmt.Fprint(os.Stderr, prompt)
	if !strings.HasSuffix(prompt, " ") {
		fmt.Fprint(os.Stderr, " ")
	}
	password, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	return string(password), nil
}

func installExecutableWithSudo(src, dest string) error {
	// Prefer install(1) for mode bits; fall back to cp + chmod.
	password, err := promptSudoPassword("Administrator password (sudo) to install update: ")
	if err != nil {
		return fmt.Errorf("read sudo password: %w", err)
	}
	if strings.TrimSpace(password) == "" {
		return fmt.Errorf("empty sudo password")
	}

	try := func(name string, args ...string) error {
		cmd := exec.Command(name, args...)
		cmd.Stdin = strings.NewReader(password + "\n")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	// -S: read password from stdin; -p '': no extra prompt
	if err := try("sudo", "-S", "-p", "", "install", "-m", "755", src, dest); err == nil {
		return nil
	} else {
		Log(fmt.Sprintf("sudo install failed: %v", err))
	}
	if err := try("sudo", "-S", "-p", "", "cp", src, dest); err != nil {
		return fmt.Errorf("sudo install failed: %w", err)
	}
	_ = try("sudo", "-S", "-p", "", "chmod", "755", dest)
	return nil
}

// createUpdateTempFile prefers a sibling of the executable (same FS as install
// path). Falls back to os.TempDir when the install directory is not writable.
func createUpdateTempFile(executablePath, binaryName string) (string, *os.File, error) {
	dir := filepath.Dir(executablePath)
	// Try same directory first for atomic rename.
	if f, err := os.CreateTemp(dir, ".curd-download-*"); err == nil {
		return f.Name(), f, nil
	}
	// Fall back to system temp (may be cross-device; replaceExecutable handles that).
	name := "curd-update-" + binaryName
	if binaryName == "" {
		name = "curd-update-bin"
	}
	path := filepath.Join(os.TempDir(), name)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return "", nil, err
	}
	return path, f, nil
}

func copyFileReplace(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	// Write to a sibling temp on the destination filesystem, then rename into place.
	dir := filepath.Dir(dst)
	tmp, err := os.CreateTemp(dir, ".curd-update-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()

	if err := tmp.Chmod(0755); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := io.Copy(tmp, in); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	// Atomic replace when possible (same directory = same filesystem).
	if err := os.Rename(tmpName, dst); err != nil {
		// Windows / busy binary: move dest aside first.
		oldPath := dst + ".old"
		_ = os.Remove(oldPath)
		if renOld := os.Rename(dst, oldPath); renOld != nil && !os.IsNotExist(renOld) {
			return renOld
		}
		if renNew := os.Rename(tmpName, dst); renNew != nil {
			_ = os.Rename(oldPath, dst)
			return renNew
		}
		_ = os.Remove(oldPath)
	}
	cleanup = false
	return nil
}

// replaceExecutable installs the downloaded binary over the running executable.
// Handles: same-FS rename, cross-device copy, busy binary swap, and sudo when needed.
func replaceExecutable(tmpPath, executablePath string) error {
	// 1) Fast path: rename when both paths share a filesystem.
	if err := os.Rename(tmpPath, executablePath); err == nil {
		return nil
	} else if isPermissionError(err) {
		updateUserMessage(GetGlobalConfig(), "Update needs elevated permissions to replace the installed binary.")
		if sudoErr := installExecutableWithSudo(tmpPath, executablePath); sudoErr != nil {
			return sudoErr
		}
		_ = os.Remove(tmpPath)
		return nil
	} else if !isCrossDeviceError(err) {
		// Unexpected rename failure — still try copy-into-place before giving up.
		Log(fmt.Sprintf("rename to %s failed (%v); trying copy replace", executablePath, err))
	}

	// 2) Cross-device (or other rename failure): copy onto dest filesystem then swap.
	if err := copyFileReplace(tmpPath, executablePath); err == nil {
		_ = os.Remove(tmpPath)
		return nil
	} else if isPermissionError(err) {
		updateUserMessage(GetGlobalConfig(), "Update needs elevated permissions to replace the installed binary.")
		if sudoErr := installExecutableWithSudo(tmpPath, executablePath); sudoErr != nil {
			return fmt.Errorf("failed to replace executable: %w", sudoErr)
		}
		_ = os.Remove(tmpPath)
		return nil
	} else {
		// Last resort: sudo install from original temp (covers weird FS layouts).
		if isCrossDeviceError(err) || runtime.GOOS != "windows" {
			Log(fmt.Sprintf("copy replace failed (%v); trying sudo", err))
			updateUserMessage(GetGlobalConfig(), "Update needs elevated permissions to replace the installed binary.")
			if sudoErr := installExecutableWithSudo(tmpPath, executablePath); sudoErr == nil {
				_ = os.Remove(tmpPath)
				return nil
			}
		}
		return fmt.Errorf("failed to replace executable: %w", err)
	}
}
