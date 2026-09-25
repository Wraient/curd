package internal

import (
	"bufio"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wraient/curd/internal/providers"
)

// Monotonic request id for mpv JSON IPC so we can ignore interleaved events.
var mpvRequestID atomic.Int64

var logFile = "debug.log"

const mpvPlaybackPollInterval = 500 * time.Millisecond

// This is not generic but we have MpvArgs in CurdConfig to add custom ones
const defaultStreamReferrer = "https://allanime.day/"

// CurdWebModeEnabled reports whether curd is running under curd-web's fake-mpv
// bridge. It gates curd-web-only behavior (like exposing the active provider
// name to the player) so a real, standalone mpv never sees a custom flag or
// property it doesn't understand.
func CurdWebModeEnabled() bool {
	return os.Getenv("CURD_WEB") == "1"
}

func streamReferrerForLink(link, provider string) string {
	if strings.Contains(strings.ToLower(link), "tools.fast4speed.rsvp") {
		return "https://allanime.to"
	}
	if referrer := providers.Referrer(provider); referrer != "" {
		return referrer
	}
	return defaultStreamReferrer
}

// We should really handle this by Provider but keeping simple string here for now

func getBundledMPVPath() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", err
	}
	exeDir := filepath.Dir(exePath)
	mpvPath := filepath.Join(exeDir, "bin", "mpv.exe")
	return mpvPath, nil
}

func resolveExecutable(binary string) (string, error) {
	binary = strings.TrimSpace(binary)
	if binary == "" {
		return "", fmt.Errorf("empty binary name")
	}

	if filepath.IsAbs(binary) || strings.Contains(binary, "/") || strings.Contains(binary, "\\") {
		if _, err := os.Stat(binary); err == nil {
			return binary, nil
		}
		return "", fmt.Errorf("binary path not found: %s", binary)
	}

	resolvedPath, err := exec.LookPath(binary)
	if err != nil {
		return "", err
	}

	return resolvedPath, nil
}

func candidatePlayerBinaries(configuredPlayer string) []string {
	player := strings.TrimSpace(configuredPlayer)
	if player == "" {
		player = "mpv"
	}

	var candidates []string
	addUnique := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		for _, existing := range candidates {
			if existing == value {
				return
			}
		}
		candidates = append(candidates, value)
	}

	addUnique(player)

	if strings.EqualFold(player, "iina") {
		// iina is mpv-based and may be exposed either on PATH or via app bundle.
		addUnique("iina")
		if runtime.GOOS == "darwin" {
			addUnique("/Applications/IINA.app/Contents/MacOS/IINA")
		}
	}

	return candidates
}

func resolveMPVBinary() (string, error) {
	if runtime.GOOS == "windows" {
		bundledMPVPath, err := getBundledMPVPath()
		if err == nil {
			if _, statErr := os.Stat(bundledMPVPath); statErr == nil {
				return bundledMPVPath, nil
			}
		}
	}

	return resolveExecutable("mpv")
}

func resolveConfiguredPlayerBinary(configuredPlayer string) (string, string, error) {
	configuredPlayer = strings.TrimSpace(configuredPlayer)
	if configuredPlayer == "" {
		configuredPlayer = "mpv"
	}

	for _, candidate := range candidatePlayerBinaries(configuredPlayer) {
		resolvedPath, err := resolveExecutable(candidate)
		if err == nil {
			return resolvedPath, configuredPlayer, nil
		}
	}

	mpvPath, mpvErr := resolveMPVBinary()
	if mpvErr != nil {
		return "", "", fmt.Errorf("configured player %q was not found and fallback to mpv failed: %w", configuredPlayer, mpvErr)
	}

	if !strings.EqualFold(configuredPlayer, "mpv") {
		warning := fmt.Sprintf("Configured player '%s' was not found. Falling back to mpv.", configuredPlayer)
		CurdOut(warning)
		Log(warning)
	}

	return mpvPath, "mpv", nil
}

func isIINAPlayer(effectivePlayerName string, resolvedPlayerBinary string) bool {
	if strings.EqualFold(strings.TrimSpace(effectivePlayerName), "iina") {
		return true
	}

	binaryName := strings.TrimSuffix(filepath.Base(resolvedPlayerBinary), filepath.Ext(resolvedPlayerBinary))
	return strings.EqualFold(binaryName, "iina")
}

func translateMPVArgsForIINA(mpvArgs []string) []string {
	translated := make([]string, 0, len(mpvArgs))
	for _, arg := range mpvArgs {
		if strings.HasPrefix(arg, "--") {
			if strings.HasPrefix(arg, "--mpv-") {
				translated = append(translated, arg)
				continue
			}
			translated = append(translated, "--mpv-"+strings.TrimPrefix(arg, "--"))
			continue
		}

		translated = append(translated, arg)
	}

	return translated
}

func isHTTPStreamLink(link string) bool {
	trimmedLink := strings.ToLower(strings.TrimSpace(link))
	return strings.HasPrefix(trimmedLink, "http://") || strings.HasPrefix(trimmedLink, "https://")
}

func hasMPVReferrerArg(args []string) bool {
	normalizeReferrerFlag := func(arg string) string {
		normalized := strings.ToLower(strings.TrimSpace(arg))
		if strings.HasPrefix(normalized, "--mpv-") {
			return "--" + strings.TrimPrefix(normalized, "--mpv-")
		}
		return normalized
	}

	for i, arg := range args {
		normalizedArg := normalizeReferrerFlag(arg)

		if strings.HasPrefix(normalizedArg, "--referrer=") || normalizedArg == "--referrer" {
			return true
		}

		if strings.HasPrefix(normalizedArg, "--http-header-fields=") && strings.Contains(normalizedArg, "referer:") {
			return true
		}

		if strings.HasPrefix(normalizedArg, "--http-header-fields-append=") && strings.Contains(normalizedArg, "referer:") {
			return true
		}

		if (normalizedArg == "--http-header-fields" || normalizedArg == "--http-header-fields-append") && i+1 < len(args) {
			nextArg := strings.ToLower(strings.TrimSpace(args[i+1]))
			if strings.Contains(nextArg, "referer:") {
				return true
			}
		}
	}

	return false
}

func hasMPVSubtitleArg(args []string) bool {
	for i, arg := range args {
		lowerArg := strings.ToLower(strings.TrimSpace(arg))
		if strings.HasPrefix(lowerArg, "--sub-file=") || lowerArg == "--sub-file" {
			return true
		}
		if lowerArg == "--sub-files" && i+1 < len(args) {
			return true
		}
	}
	return false
}

func normalizeReferrerValue(referrer string) string {
	referrer = strings.TrimSpace(referrer)
	if referrer == "" {
		return ""
	}

	if !strings.Contains(referrer, "://") {
		referrer = "https://" + referrer
	}

	parsed, err := url.Parse(referrer)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return referrer
	}

	if parsed.Path == "" {
		parsed.Path = "/"
	}

	return parsed.String()
}

func normalizeReferrerArgs(args []string) []string {
	normalized := make([]string, 0, len(args))

	for i := 0; i < len(args); i++ {
		arg := args[i]

		switch {
		case strings.HasPrefix(arg, "--referrer="):
			normalized = append(normalized, "--referrer="+normalizeReferrerValue(strings.TrimPrefix(arg, "--referrer=")))
		case arg == "--referrer" && i+1 < len(args):
			normalized = append(normalized, arg, normalizeReferrerValue(args[i+1]))
			i++
		case strings.HasPrefix(arg, "--mpv-referrer="):
			normalized = append(normalized, "--mpv-referrer="+normalizeReferrerValue(strings.TrimPrefix(arg, "--mpv-referrer=")))
		case arg == "--mpv-referrer" && i+1 < len(args):
			normalized = append(normalized, arg, normalizeReferrerValue(args[i+1]))
			i++
		default:
			normalized = append(normalized, arg)
		}
	}

	return normalized
}

func StartVideo(link string, args []string, title string, anime *Anime) (string, error) {
	var command *exec.Cmd
	var mpvSocketPath string
	var err error

	userConfig := GetGlobalConfig()
	if userConfig == nil {
		defaultConfig := PopulateConfig(map[string]string{})
		userConfig = &defaultConfig
	}
	if anime == nil {
		return "", fmt.Errorf("missing anime playback state")
	}
	if strings.TrimSpace(link) == "" {
		return "", fmt.Errorf("empty video link")
	}

	// Add custom MPV arguments from config if they exist
	if userConfig.MpvArgs != nil {
		args = append(args, userConfig.MpvArgs...)
	}
	callerHasSubtitleArg := hasMPVSubtitleArg(args)

	shouldSetDefaultReferrer := isHTTPStreamLink(link) && !hasMPVReferrerArg(args)
	referrer := strings.TrimSpace(anime.Ep.StreamReferrer)
	if referrer == "" && shouldSetDefaultReferrer {
		referrer = streamReferrerForLink(link, CurrentAnimeProviderName(anime))
	}
	if referrer != "" && shouldSetDefaultReferrer {
		args = append(args, fmt.Sprintf("--referrer=%s", referrer))
	}
	subtitleURL := strings.TrimSpace(anime.Ep.SubtitleURL)
	if subtitleURL != "" && !callerHasSubtitleArg {
		args = append(args, fmt.Sprintf("--sub-file=%s", subtitleURL))
	}
	args = normalizeReferrerArgs(args)

	// Check if we have an existing socket and if MPV is still running
	if anime.Ep.Player.SocketPath != "" && IsMPVRunning(anime.Ep.Player.SocketPath) {
		// Reuse existing socket
		mpvSocketPath = anime.Ep.Player.SocketPath

		if shouldSetDefaultReferrer {
			activeReferrer := strings.TrimSpace(anime.Ep.StreamReferrer)
			if activeReferrer == "" {
				activeReferrer = streamReferrerForLink(link, CurrentAnimeProviderName(anime))
			}
			if activeReferrer != "" {
				_, referrerErr := MPVSendCommand(mpvSocketPath, []interface{}{"set_property", "referrer", activeReferrer})
				if referrerErr != nil {
					Log(fmt.Sprintf("Failed to set referrer property: %v", referrerErr))
				}
			}
		}

		// Load the new file in the existing MPV instance
		command := []interface{}{"loadfile", link}
		_, err = MPVSendCommand(mpvSocketPath, command)
		if err != nil {
			return "", fmt.Errorf("failed to load file in existing MPV instance: %w", err)
		}

		if subtitleURL != "" && !callerHasSubtitleArg {
			if readyErr := waitForMPVFileReady(mpvSocketPath, link, 12*time.Second); readyErr != nil {
				Log(fmt.Sprintf("Timed out waiting to attach subtitle track: %v", readyErr))
			}
			_, subErr := MPVSendCommand(mpvSocketPath, []interface{}{"sub-add", subtitleURL, "select"})
			if subErr != nil {
				Log(fmt.Sprintf("Failed to load subtitle track: %v", subErr))
			}
		}

		// Update the window title
		titleCommand := []interface{}{"set_property", "force-media-title", title}
		_, err = MPVSendCommand(mpvSocketPath, titleCommand)
		if err != nil {
			Log(fmt.Sprintf("Failed to update title: %v", err))
		}

		// Also update the window title property
		windowTitleCommand := []interface{}{"set_property", "title", title}
		_, err = MPVSendCommand(mpvSocketPath, windowTitleCommand)
		if err != nil {
			Log(fmt.Sprintf("Failed to update window title: %v", err))
		}

		if CurdWebModeEnabled() {
			providerCommand := []interface{}{"set_property", "user-data/curd-web-provider", CurrentAnimeProviderName(anime)}
			if _, err = MPVSendCommand(mpvSocketPath, providerCommand); err != nil {
				Log(fmt.Sprintf("Failed to update curd-web provider property: %v", err))
			}
		}

		return mpvSocketPath, nil
	}

	if anime.Ep.Player.SocketPath == "" {
		// Generate a random number for the socket path
		randomBytes := make([]byte, 4)
		_, err = rand.Read(randomBytes)
		if err != nil {
			Log("Failed to generate random number")
			return "", fmt.Errorf("failed to generate random number: %w", err)
		}

		randomNumber := fmt.Sprintf("%x", randomBytes)

		// Create the mpv socket path with the random number
		if runtime.GOOS == "windows" {
			mpvSocketPath = fmt.Sprintf(`\\.\pipe\curd_mpvsocket_%s`, randomNumber)
		} else {
			mpvSocketPath = fmt.Sprintf("/tmp/curd_mpvsocket_%s", randomNumber)
		}
	} else {
		mpvSocketPath = anime.Ep.Player.SocketPath
	}

	// Add the title to MPV arguments
	titleArgs := []string{fmt.Sprintf("--title=%s", title), fmt.Sprintf("--force-media-title=%s", title)}

	// Keep the window open after episode completes, new episode starts in the same mpv window
	args = append(args, "--force-window=yes", "--idle=yes")
	args = append(args, titleArgs...)
	if CurdWebModeEnabled() {
		args = append(args, fmt.Sprintf("--curd-web-provider=%s", CurrentAnimeProviderName(anime)))
	}

	// Prepare arguments for mpv-compatible players.
	var mpvArgs []string
	mpvArgs = append(mpvArgs, "--no-terminal", "--really-quiet", fmt.Sprintf("--input-ipc-server=%s", mpvSocketPath))
	// Add any additional arguments passed
	if len(args) > 0 {
		mpvArgs = append(mpvArgs, args...)
	}
	mpvArgs = append(mpvArgs, link)

	// Detect Android strictly from GOOS to avoid false positives from PATH binaries.
	isAndroid := runtime.GOOS == "android"

	if isAndroid {
		amBinary, resolveErr := resolveExecutable("/system/bin/am")
		if resolveErr != nil {
			amBinary, resolveErr = resolveExecutable("am")
			if resolveErr != nil {
				CurdOut("Error: Android activity manager binary not found")
				return "", fmt.Errorf("failed to locate android activity manager binary: %w", resolveErr)
			}
		}

		// Only use MPV on Android via intent
		cmdArgs := []string{
			"start", "--user", "0",
			"-a", "android.intent.action.VIEW",
			"-d", link,
			"-n", "is.xyz.mpv/.MPVActivity",
		}

		command = exec.Command(amBinary, cmdArgs...)
		err = command.Start()
		if err != nil {
			CurdOut("Error: Failed to start android intent")
			return "", fmt.Errorf("failed to start android intent: %w", err)
		}
		return "android-intent", nil
	}

	resolvedPlayerBinary, effectivePlayerName, err := resolveConfiguredPlayerBinary(userConfig.Player)
	if err != nil {
		CurdOut("Error: Failed to resolve media player")
		Log(fmt.Sprintf("Player resolution failed for '%s': %v", userConfig.Player, err))
		return "", err
	}

	playerArgs := mpvArgs
	if isIINAPlayer(effectivePlayerName, resolvedPlayerBinary) {
		playerArgs = translateMPVArgsForIINA(mpvArgs)
		playerArgs = append(playerArgs, "--no-stdin")
	}

	command = exec.Command(resolvedPlayerBinary, playerArgs...)

	// Start the selected mpv-compatible player process
	err = command.Start()
	if err != nil {
		CurdOut(fmt.Sprintf("Error: Failed to start %s process", effectivePlayerName))
		return "", fmt.Errorf("failed to start %s: %w", effectivePlayerName, err)
	}

	// Wait for the socket to become available with retries
	socketReady := false
	maxRetries := 10
	retryDelay := 300 * time.Millisecond

	Log(fmt.Sprintf("Waiting for MPV socket to be ready at %s", mpvSocketPath))
	for i := 0; i < maxRetries; i++ {
		time.Sleep(retryDelay)

		// Try to connect to the socket
		conn, err := connectToPipe(mpvSocketPath)
		if err == nil {
			conn.Close()
			socketReady = true
			Log(fmt.Sprintf("MPV socket ready after %d attempts", i+1))
			break
		}

		Log(fmt.Sprintf("Attempt %d/%d - Socket not ready yet: %v", i+1, maxRetries, err))
	}

	if !socketReady {
		Log(fmt.Sprintf("Failed to connect to MPV socket after %d attempts", maxRetries))
		// Don't fail here, just warn and continue - the next commands will handle any further issues
	}

	return mpvSocketPath, nil
}

// WaitForMPVPlaybackStart polls MPV until time-pos is available or the timeout elapses.
// Returns true when playback has started, false when MPV exits or the timeout is reached.
func WaitForMPVPlaybackStart(ipcSocketPath string, timeout time.Duration) bool {
	if ipcSocketPath == "" || ipcSocketPath == "android-intent" {
		return true
	}

	deadline := time.Now().Add(timeout)
	Log(fmt.Sprintf("Waiting up to %s for MPV playback to start at %s", timeout, ipcSocketPath))

	for time.Now().Before(deadline) {
		timePos, err := MPVSendCommand(ipcSocketPath, []interface{}{"get_property", "time-pos"})
		if err == nil && timePos != nil {
			if _, ok := timePos.(float64); ok {
				Log("MPV playback started")
				return true
			}
		}

		if err != nil {
			if isMPVConnectionGoneError(err) {
				Log("MPV exited before playback could start")
				return false
			}
		}

		time.Sleep(mpvPlaybackPollInterval)
	}

	Log(fmt.Sprintf("MPV playback did not start within %s", timeout))
	return false
}

// StartVideoWithProviderFallback starts the current episode links and, if MPV never
// begins playback, retries other providers. Preferred SubOrDub is exhausted first;
// only then is an alternate sub/dub mode offered with an explicit user prompt.
func StartVideoWithProviderFallback(userCurdConfig *CurdConfig, anime *Anime, title string) string {
	if userCurdConfig == nil || anime == nil {
		Log("StartVideoWithProviderFallback: missing config or anime")
		exitWithRestore(1)
	}
	if strings.TrimSpace(title) == "" {
		title = fmt.Sprintf("%s - Episode %d", GetAnimeName(*anime), anime.Ep.Number)
	}

	var excludedProviders []string
	activeMode := normalizeTranslationType(userCurdConfig.SubOrDub)
	alternateModeOffered := false

	for {
		if len(anime.Ep.Links) == 0 {
			CurdOut("No episode links found")
			exitWithRestore(1)
		}

		mpvSocketPath, err := StartVideo(PrioritizeLink(anime.Ep.Links), []string{}, title, anime)
		if err != nil {
			Log("Failed to start mpv")
			exitWithRestore(1)
		}

		if mpvSocketPath == "android-intent" || WaitForMPVPlaybackStart(mpvSocketPath, MpvPlaybackStartTimeoutDuration(userCurdConfig)) {
			return mpvSocketPath
		}

		failedProvider := CurrentAnimeProviderName(anime)
		playbackTimeout := MpvPlaybackStartTimeoutDuration(userCurdConfig)
		Log(fmt.Sprintf("Playback did not start with provider %s/%s within %s", failedProvider, activeMode, playbackTimeout))
		CurdOut(fmt.Sprintf("Playback failed to start with %s. Trying another provider...", failedProvider))

		if mpvSocketPath != "" {
			ExitMPV(mpvSocketPath)
		}
		anime.Ep.Player.SocketPath = ""
		excludedProviders = append(excludedProviders, failedProvider)

		// Prefer remaining providers in the active (initially preferred) audio mode.
		episodeResult, err := ResolveEpisodeURLExcludingProvidersMode(*userCurdConfig, anime, anime.Ep.Number, excludedProviders, activeMode)
		if err == nil && len(episodeResult.Links) > 0 {
			anime.Ep.Links = episodeResult.Links
			applyStreamPlaybackHints(anime, anime.Ep.Links, episodeResult.LinkHints)
			Log(fmt.Sprintf("Retrying playback with %s/%s: %+v", episodeResult.ProviderName, episodeResult.Mode, episodeResult.Links))
			CurdOut(fmt.Sprintf("Retrying with %s...", episodeResult.ProviderName))
			continue
		}

		// Preferred/active mode exhausted — offer alternate sub/dub once, with a prompt.
		if !alternateModeOffered && activeMode == normalizeTranslationType(userCurdConfig.SubOrDub) {
			alternateModeOffered = true
			episodeResult, err = ResolveEpisodeURLAlternateModeWithPrompt(*userCurdConfig, anime, anime.Ep.Number, nil)
			if err == nil && len(episodeResult.Links) > 0 {
				// Alternate mode gets a fresh provider pass, including ones that failed preferred.
				excludedProviders = nil
				activeMode = normalizeTranslationType(episodeResult.Mode)
				anime.Ep.Links = episodeResult.Links
				applyStreamPlaybackHints(anime, anime.Ep.Links, episodeResult.LinkHints)
				Log(fmt.Sprintf("Retrying playback with %s/%s after audio fallback: %+v", episodeResult.ProviderName, episodeResult.Mode, episodeResult.Links))
				CurdOut(fmt.Sprintf("Retrying with %s (%s)...", episodeResult.ProviderName, episodeResult.Mode))
				continue
			}
		}

		CurdOut("No alternative provider could start playback for this episode.")
		if err != nil {
			Log(fmt.Sprintf("Provider fallback failed: %v", err))
		}
		exitWithRestore(1)
	}
}

func isMPVConnectionGoneError(err error) bool {
	if err == nil {
		return false
	}
	errMsg := strings.ToLower(err.Error())
	goneErrors := []string{
		"connect: connection refused",
		"connect: no such file or directory",
		"cannot find the file specified",
		"pipe has been ended",
		"pipe is being closed",
		"no process is on the other end of the pipe",
	}
	for _, goneError := range goneErrors {
		if strings.Contains(errMsg, goneError) {
			return true
		}
	}
	return false
}

func isMPVPropertyUnavailableError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "property unavailable")
}

// PlaybackLossAction tells the monitor how to react when time-pos/has-playback fails.
type PlaybackLossAction int

const (
	// PlaybackLossWait: transient gap (playlist switch, demuxer reload). Keep monitoring.
	PlaybackLossWait PlaybackLossAction = iota
	// PlaybackLossComplete: episode reached the completion threshold (or MPV quit after enough watch).
	PlaybackLossComplete
	// PlaybackLossExit: user quit MPV before the completion threshold.
	PlaybackLossExit
)

// ClassifyPlaybackLoss decides what to do when the monitor loses time-pos.
// Critical: "property unavailable" while MPV is still running is normal during
// playlist episode switches and must NOT exit curd. Only a dead MPV process
// (or a finished episode past the completion %) should end the session.
func ClassifyPlaybackLoss(socketPath string, started bool, percentageWatched float64, completeThreshold int) PlaybackLossAction {
	if !started {
		return PlaybackLossWait
	}
	if MPVPlaylistIsSwitching() {
		return PlaybackLossWait
	}
	if int(percentageWatched) >= completeThreshold {
		return PlaybackLossComplete
	}
	// MPV still open with incomplete watch → wait (playlist jump, pause, buffer).
	if socketPath != "" && IsMPVRunning(socketPath) {
		return PlaybackLossWait
	}
	return PlaybackLossExit
}

// Helper function to join args with a space
func joinArgs(args []string) string {
	result := ""
	for i, arg := range args {
		if i > 0 {
			result += " "
		}
		result += arg
	}
	return result
}

func MPVSendCommand(ipcSocketPath string, command []interface{}) (interface{}, error) {
	// Use a retry mechanism for transient errors
	var lastErr error
	maxRetries := 3
	retryDelay := 100 * time.Millisecond

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(retryDelay)
			Log(fmt.Sprintf("Retrying MPV command, attempt %d/%d", attempt+1, maxRetries))
		}

		data, err := mpvSendCommandOnce(ipcSocketPath, command)
		if err == nil {
			return data, nil
		}
		lastErr = err
		// property unavailable is a valid answer — don't spin retries on it
		if isMPVPropertyUnavailableError(err) {
			return nil, err
		}
		Log(fmt.Sprintf("MPV command error (attempt %d/%d): %v", attempt+1, maxRetries, err))
	}

	return nil, fmt.Errorf("command failed after %d attempts: %w", maxRetries, lastErr)
}

func mpvSendCommandOnce(ipcSocketPath string, command []interface{}) (interface{}, error) {
	conn, err := connectToPipe(ipcSocketPath)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	defer conn.Close()

	reqID := mpvRequestID.Add(1)
	payload, err := json.Marshal(map[string]interface{}{
		"command":    command,
		"request_id": reqID,
	})
	if err != nil {
		return nil, err
	}

	if deadline, ok := conn.(interface{ SetReadDeadline(time.Time) error }); ok {
		_ = deadline.SetReadDeadline(time.Now().Add(3 * time.Second))
	}
	if deadline, ok := conn.(interface{ SetWriteDeadline(time.Time) error }); ok {
		_ = deadline.SetWriteDeadline(time.Now().Add(2 * time.Second))
	}

	if _, err := conn.Write(append(payload, '\n')); err != nil {
		return nil, fmt.Errorf("write: %w", err)
	}

	// Read newline-delimited JSON; skip event messages until our request_id matches.
	reader := bufio.NewReader(conn)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if d, ok := conn.(interface{ SetReadDeadline(time.Time) error }); ok {
			_ = d.SetReadDeadline(time.Now().Add(2 * time.Second))
		}
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF && len(line) == 0 {
				return nil, fmt.Errorf("read: %w", err)
			}
			if len(line) == 0 {
				return nil, fmt.Errorf("read: %w", err)
			}
			// fall through with partial line if any
		}
		line = bytesTrimSpace(line)
		if len(line) == 0 {
			continue
		}

		var response map[string]interface{}
		if err := json.Unmarshal(line, &response); err != nil {
			// Multi-object garbage — try first line only already; skip bad line
			continue
		}

		// Skip pure events (property-change, etc.) that have no matching request_id.
		if rid, ok := response["request_id"]; ok {
			if idNum, ok := asInt64(rid); !ok || idNum != reqID {
				continue
			}
		} else if _, isEvent := response["event"]; isEvent {
			continue
		}

		// Response without request_id but with error/data — accept as command reply
		// only when it looks like a command result (has "error" field).
		if _, hasErr := response["error"]; !hasErr {
			if _, isEvent := response["event"]; isEvent {
				continue
			}
		}

		return mpvResponseData(response)
	}
	return nil, fmt.Errorf("timed out waiting for mpv response to request_id=%d", reqID)
}

func bytesTrimSpace(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}

func asInt64(v interface{}) (int64, bool) {
	switch n := v.(type) {
	case float64:
		return int64(n), true
	case int64:
		return n, true
	case int:
		return int64(n), true
	case json.Number:
		i, err := n.Int64()
		return i, err == nil
	default:
		return 0, false
	}
}


func mpvResponseData(response map[string]interface{}) (interface{}, error) {
	if errorValue, exists := response["error"]; exists {
		errorText, ok := errorValue.(string)
		if !ok {
			return nil, fmt.Errorf("mpv returned non-string error field: %T", errorValue)
		}
		if errorText != "" && errorText != "success" {
			return nil, fmt.Errorf("mpv command error: %s", errorText)
		}
	}

	if data, exists := response["data"]; exists {
		return data, nil
	}
	return nil, nil
}

func SeekMPV(ipcSocketPath string, time int) (interface{}, error) {
	command := []interface{}{"seek", time, "absolute"}
	return MPVSendCommand(ipcSocketPath, command)
}

type mpvCommandSender func(string, []interface{}) (interface{}, error)

func waitForMPVFileReady(ipcSocketPath, expectedPath string, timeout time.Duration) error {
	return waitForMPVFileReadyWith(MPVSendCommand, ipcSocketPath, expectedPath, timeout, 100*time.Millisecond)
}

func waitForMPVFileReadyWith(send mpvCommandSender, ipcSocketPath, expectedPath string, timeout, pollInterval time.Duration) error {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	if pollInterval <= 0 {
		pollInterval = 100 * time.Millisecond
	}
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		pathValue, pathErr := send(ipcSocketPath, []interface{}{"get_property", "path"})
		path, _ := pathValue.(string)
		pathReady := pathErr == nil && strings.TrimSpace(path) != ""
		if pathReady && expectedPath != "" {
			pathReady = path == expectedPath
		}
		if pathReady {
			if duration, durationErr := send(ipcSocketPath, []interface{}{"get_property", "duration"}); durationErr == nil && mpvNumber(duration) > 0 {
				return nil
			}
			if position, positionErr := send(ipcSocketPath, []interface{}{"get_property", "time-pos"}); positionErr == nil && mpvNumber(position) >= 0 {
				return nil
			}
		}
		if pathErr != nil {
			lastErr = pathErr
		}
		time.Sleep(pollInterval)
	}
	if lastErr != nil {
		return fmt.Errorf("MPV file did not become ready: %w", lastErr)
	}
	return fmt.Errorf("MPV file did not become ready before timeout")
}

func mpvNumber(value interface{}) float64 {
	switch number := value.(type) {
	case float64:
		return number
	case float32:
		return float64(number)
	case int:
		return float64(number)
	case int64:
		return float64(number)
	default:
		return -1
	}
}

func GetMPVPausedStatus(ipcSocketPath string) (bool, error) {
	status, err := MPVSendCommand(ipcSocketPath, []interface{}{"get_property", "pause"})
	if err != nil || status == nil {
		return false, err
	}

	paused, ok := status.(bool)
	if ok {
		return paused, nil
	}
	return false, nil
}

func GetMPVPlaybackSpeed(ipcSocketPath string) (float64, error) {
	speed, err := MPVSendCommand(ipcSocketPath, []interface{}{"get_property", "speed"})
	if err != nil || speed == nil {
		Log("Failed to get playback speed.")
		return 0, err
	}

	currentSpeed, ok := speed.(float64)
	if ok {
		return currentSpeed, nil
	}

	return 0, nil
}

func GetPercentageWatched(ipcSocketPath string) (float64, error) {
	currentTime, err := MPVSendCommand(ipcSocketPath, []interface{}{"get_property", "time-pos"})
	if err != nil || currentTime == nil {
		return 0, err
	}

	duration, err := MPVSendCommand(ipcSocketPath, []interface{}{"get_property", "duration"})
	if err != nil || duration == nil {
		return 0, err
	}

	currTime, ok1 := currentTime.(float64)
	dur, ok2 := duration.(float64)

	if ok1 && ok2 && dur > 0 {
		percentageWatched := (currTime / dur) * 100
		return percentageWatched, nil
	}

	return 0, nil
}

func PercentageWatched(playbackTime int, duration int) float64 {
	if duration > 0 {
		percentage := (float64(playbackTime) / float64(duration)) * 100
		return percentage
	}
	return float64(0)
}

func HasActivePlayback(ipcSocketPath string) (bool, error) {
	maxRetries := 3
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(200 * time.Millisecond)
		}

		// Get the time-pos property from MPV
		timePos, err := MPVSendCommand(ipcSocketPath, []interface{}{"get_property", "time-pos"})

		if err != nil {
			// Check specifically for "property unavailable" error - this is a valid state
			if strings.Contains(err.Error(), "property unavailable") {
				Log("HasActivePlayback: Property unavailable, nothing is playing")
				return false, nil
			}

			// Check for socket connection errors - these might be temporary
			if strings.Contains(err.Error(), "connect: connection refused") ||
				strings.Contains(err.Error(), "connect: no such file or directory") {
				lastErr = err
				Log(fmt.Sprintf("HasActivePlayback: Connection error (attempt %d/%d): %v",
					attempt+1, maxRetries, err))
				continue // Try again
			}

			// Other errors should be returned
			return false, fmt.Errorf("error getting time-pos: %w", err)
		}

		// If we got a valid response, something is playing
		if timePos != nil {
			return true, nil
		}

		// No error but no position either - likely nothing is playing
		return false, nil
	}

	// If we get here, all retries failed
	Log(fmt.Sprintf("HasActivePlayback: Failed after %d attempts: %v", maxRetries, lastErr))
	return false, fmt.Errorf("failed to check playback status: %w", lastErr)
}

func IsMPVRunning(socketPath string) bool {
	if socketPath == "" {
		return false
	}

	maxRetries := 3
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(200 * time.Millisecond)
			Log(fmt.Sprintf("Retrying MPV connection check, attempt %d/%d", attempt+1, maxRetries))
		}

		// Try to connect to the socket
		conn, err := connectToPipe(socketPath)
		if err != nil {
			Log(fmt.Sprintf("IsMPVRunning: Connection error (attempt %d/%d): %v",
				attempt+1, maxRetries, err))
			continue
		}
		defer conn.Close()

		// Send a simple command to check if MPV responds
		_, err = MPVSendCommand(socketPath, []interface{}{"get_property", "pid"})
		if err == nil {
			return true
		}

		Log(fmt.Sprintf("IsMPVRunning: Command failed (attempt %d/%d): %v",
			attempt+1, maxRetries, err))
	}

	// After all retries, conclude MPV is not running
	return false
}

func ExitMPV(ipcSocketPath string) error {
	// Send command to close MPV
	_, err := MPVSendCommand(ipcSocketPath, []interface{}{"quit"})
	if err != nil {
		Log("Error closing MPV: " + err.Error())
	}
	return err
}

// MPVEventListener represents a structure to track MPV events
type MPVEventListener struct {
	SocketPath        string
	LastPosition      float64
	LastPauseState    bool
	SeekDetected      bool
	PlayPauseDetected bool
	IsListening       bool
	mu                sync.Mutex // Add mutex for thread safety
}

// SetupMPVEventListening configures MPV to send property change notifications
func SetupMPVEventListening(ipcSocketPath string) error {
	Log("=== SETTING UP MPV EVENT LISTENING ===")
	Log("Socket path: " + ipcSocketPath)

	// Observe time-pos property for seek detection
	Log("Setting up time-pos observer...")
	response1, err := MPVSendCommand(ipcSocketPath, []interface{}{"observe_property", 1, "time-pos"})
	if err != nil {
		Log("FAILED: Error setting up time-pos observer: " + err.Error())
		return err
	}
	Log(fmt.Sprintf("SUCCESS: time-pos observer setup. Response: %v", response1))

	// Observe pause property for play/pause detection
	Log("Setting up pause observer...")
	response2, err := MPVSendCommand(ipcSocketPath, []interface{}{"observe_property", 2, "pause"})
	if err != nil {
		Log("FAILED: Error setting up pause observer: " + err.Error())
		return err
	}
	Log(fmt.Sprintf("SUCCESS: pause observer setup. Response: %v", response2))

	// Observe seeking property for direct seek detection
	Log("Setting up seeking observer...")
	response3, err := MPVSendCommand(ipcSocketPath, []interface{}{"observe_property", 3, "seeking"})
	if err != nil {
		Log("FAILED: Error setting up seeking observer: " + err.Error())
		return err
	}
	Log(fmt.Sprintf("SUCCESS: seeking observer setup. Response: %v", response3))

	Log("=== MPV EVENT LISTENING SETUP COMPLETED ===")
	return nil
}

// StartMPVEventListener starts a dedicated goroutine to listen for MPV events
func StartMPVEventListener(ipcSocketPath string, eventCallback func(string, interface{})) error {
	go func() {
		Log("Starting MPV event listener goroutine for socket: " + ipcSocketPath)

		conn, err := connectToPipe(ipcSocketPath)
		if err != nil {
			Log("Failed to connect to MPV socket for event listening: " + err.Error())
			return
		}
		defer conn.Close()

		Log("Successfully connected to MPV socket for event listening")

		buf := make([]byte, 4096)
		eventCount := 0

		for {
			// Set read timeout to avoid hanging indefinitely
			if deadline, ok := conn.(interface{ SetReadDeadline(time.Time) error }); ok {
				deadline.SetReadDeadline(time.Now().Add(10 * time.Second))
			}

			Log("Waiting for MPV events...")
			n, err := conn.Read(buf)
			if err != nil {
				Log("MPV event listener read error: " + err.Error())
				if strings.Contains(err.Error(), "timeout") {
					Log("Read timeout - continuing to wait for events...")
					continue
				}
				break
			}

			if n > 0 {
				eventCount++
				rawMessage := string(buf[:n])
				Log(fmt.Sprintf("Raw MPV message #%d (%d bytes): %s", eventCount, n, rawMessage))

				var response map[string]interface{}
				if err := json.Unmarshal(buf[:n], &response); err != nil {
					Log("MPV event listener JSON parse error: " + err.Error())
					Log("Raw data that failed to parse: " + rawMessage)
					continue
				}

				Log(fmt.Sprintf("Parsed MPV response: %+v", response))

				// Handle both events and property changes
				if event, exists := response["event"]; exists {
					eventType, ok := event.(string)
					if !ok {
						Log(fmt.Sprintf("MPV event field is not a string: %T", event))
						continue
					}
					Log(fmt.Sprintf("Event type detected: %s", eventType))

					// Handle specific MPV events
					switch eventType {
					case "playback-restart":
						Log("PLAYBACK-RESTART EVENT DETECTED (SEEK)")
						if eventCallback != nil {
							eventCallback("playback-restart", true)
						}

					case "pause":
						Log("PAUSE EVENT DETECTED")
						if eventCallback != nil {
							eventCallback("pause-event", true)
						}

					case "unpause":
						Log("UNPAUSE EVENT DETECTED")
						if eventCallback != nil {
							eventCallback("unpause-event", false)
						}

					case "property-change":
						if name, exists := response["name"]; exists {
							if data, exists := response["data"]; exists {
								propertyName, ok := name.(string)
								if !ok {
									Log(fmt.Sprintf("Property change name is not a string: %T", name))
									continue
								}
								Log(fmt.Sprintf("MPV PROPERTY CHANGE EVENT - %s: %v", propertyName, data))

								// Call the callback with the event details
								if eventCallback != nil {
									Log(fmt.Sprintf("Calling event callback for property: %s", propertyName))
									eventCallback(propertyName, data)
								} else {
									Log("WARNING: No event callback set!")
								}
							} else {
								Log("Property change event missing 'data' field")
							}
						} else {
							Log("Property change event missing 'name' field")
						}

					default:
						Log(fmt.Sprintf("📋 Other MPV event: %s (full data: %+v)", eventType, response))
						// Also forward unknown events to callback in case we need to handle more
						if eventCallback != nil {
							eventCallback(eventType, response)
						}
					}
				} else {
					Log("Non-event message received (probably command response)")
				}
			}
		}

		Log(fmt.Sprintf("=== MPV EVENT LISTENER EXITING (processed %d events) ===", eventCount))
	}()

	return nil
}

// MPVSeekDetector provides enhanced seek detection using actual MPV events
func CreateMPVSeekDetector(ipcSocketPath string) *MPVEventListener {
	detector := &MPVEventListener{
		SocketPath:        ipcSocketPath,
		LastPosition:      -1,
		LastPauseState:    false,
		SeekDetected:      false,
		PlayPauseDetected: false,
		IsListening:       false,
	}

	Log("Created MPV seek detector for socket: " + ipcSocketPath)
	return detector
}

// ProcessMPVEvent processes incoming MPV events and detects seeks and play/pause changes
func (detector *MPVEventListener) ProcessMPVEvent(propertyName string, data interface{}) {
	Log(fmt.Sprintf("=== PROCESSING MPV EVENT: %s ===", propertyName))
	Log(fmt.Sprintf("Event data: %v (type: %T)", data, data))

	detector.mu.Lock()
	defer detector.mu.Unlock()

	switch propertyName {
	case "playback-restart":
		Log("Processing playback-restart event (SEEK DETECTED)...")
		detector.SeekDetected = true
		Log(fmt.Sprintf("SEEK EVENT DETECTED VIA PLAYBACK-RESTART FLAG SET TO TRUE at %s", time.Now().Format("15:04:05.000")))

	case "pause-event":
		Log("Processing pause event...")
		detector.PlayPauseDetected = true
		detector.LastPauseState = true
		Log("  PAUSE EVENT DETECTED ")

	case "unpause-event":
		Log(" Processing unpause event...")
		detector.PlayPauseDetected = true
		detector.LastPauseState = false
		Log("UNPAUSE EVENT DETECTED")

	case "time-pos":
		Log("Processing time-pos event...")
		if data != nil {
			if position, ok := data.(float64); ok {
				Log(fmt.Sprintf("POSITION UPDATE: %f seconds (was: %f)", position, detector.LastPosition))

				if detector.LastPosition >= 0 {
					// Check for significant position jump (potential seek) - backup method
					positionDiff := position - detector.LastPosition
					Log(fmt.Sprintf("Position difference: %f seconds", positionDiff))

					if positionDiff < -2 || positionDiff > 5 { // Backwards seek or large forward jump
						detector.SeekDetected = true
						Log(fmt.Sprintf("BACKUP SEEK DETECTED Position jumped from %f to %f (diff: %f)",
							detector.LastPosition, position, positionDiff))
					} else {
						Log("Normal position progression - no seek detected")
					}
				} else {
					Log("First position update - no seek detection yet")
				}

				detector.LastPosition = position
			} else {
				Log(fmt.Sprintf("WARNING: time-pos data is not float64: %T", data))
			}
		} else {
			Log("WARNING: time-pos data is nil")
		}

	case "pause":
		Log("Processing pause property change...")
		if data != nil {
			if pauseState, ok := data.(bool); ok {
				Log(fmt.Sprintf(" PAUSE STATE UPDATE: %t (was: %t)", pauseState, detector.LastPauseState))

				if detector.LastPauseState != pauseState {
					detector.PlayPauseDetected = true
					Log(fmt.Sprintf("PLAY/PAUSE PROPERTY CHANGE DETECTED Changed from %t to %t",
						detector.LastPauseState, pauseState))
				} else {
					Log("Pause state unchanged - no play/pause event")
				}

				detector.LastPauseState = pauseState
			} else {
				Log(fmt.Sprintf("WARNING: pause data is not boolean: %T", data))
			}
		} else {
			Log("WARNING: pause data is nil")
		}

	case "seeking":
		Log("Processing seeking property change...")
		if data != nil {
			if seeking, ok := data.(bool); ok {
				Log(fmt.Sprintf("Seeking state: %t", seeking))
				if seeking {
					detector.SeekDetected = true
					Log("SEEKING PROPERTY CHANGE DETECTED  MPV reported seeking=true")
				} else {
					Log("Seeking ended (seeking=false)")
				}
			} else {
				Log(fmt.Sprintf("WARNING: seeking data is not boolean: %T", data))
			}
		} else {
			Log("WARNING: seeking data is nil")
		}

	default:
		Log(fmt.Sprintf("Unknown event: %s", propertyName))
	}

	Log("=== EVENT PROCESSING COMPLETE ===")
}

// HasSeekOccurred checks and resets the seek detection flag
func (detector *MPVEventListener) HasSeekOccurred() bool {
	detector.mu.Lock()
	defer detector.mu.Unlock()

	// Log(fmt.Sprintf("HasSeekOccurred called at %s - SeekDetected flag: %t", time.Now().Format("15:04:05.000"), detector.SeekDetected))
	if detector.SeekDetected {
		detector.SeekDetected = false
		Log(fmt.Sprintf("Seek event consumed and reset at %s - RETURNING TRUE", time.Now().Format("15:04:05.000")))
		return true
	}
	// Log(fmt.Sprintf("No seek event at %s - RETURNING FALSE", time.Now().Format("15:04:05.000")))
	return false
}

// HasPlayPauseChanged checks and resets the play/pause detection flag
func (detector *MPVEventListener) HasPlayPauseChanged() bool {
	detector.mu.Lock()
	defer detector.mu.Unlock()

	// Log(fmt.Sprintf("HasPlayPauseChanged called - PlayPauseDetected flag: %t", detector.PlayPauseDetected))
	if detector.PlayPauseDetected {
		detector.PlayPauseDetected = false
		Log("Play/pause event consumed and reset - RETURNING TRUE")
		return true
	}
	return false
}
