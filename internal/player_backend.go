package internal

import (
	"crypto/rand"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const androidIntentSocketPath = "android-intent"

type PlayerBackend interface {
	Name() string
	Mode() string
	SupportsAutomation() bool
	Start(link string, args []string, title string, anime *Anime) (string, error)
	SendCommand(target string, command []interface{}) (interface{}, error)
}

type desktopPlayerBackend struct{}

type androidPlayerBackend struct {
	config *CurdConfig
}

func activePlayerBackend() PlayerBackend {
	config := GetGlobalConfig()
	if config != nil && config.IsAndroid() {
		return &androidPlayerBackend{config: config}
	}

	return &desktopPlayerBackend{}
}

func (b *desktopPlayerBackend) Name() string {
	return "desktop"
}

func (b *desktopPlayerBackend) Mode() string {
	return "ipc"
}

func (b *desktopPlayerBackend) SupportsAutomation() bool {
	return true
}

func (b *desktopPlayerBackend) Start(link string, args []string, title string, anime *Anime) (string, error) {
	return startDesktopVideo(link, args, title, anime)
}

func (b *desktopPlayerBackend) SendCommand(target string, command []interface{}) (interface{}, error) {
	return sendMPVCommandOverIPC(target, command)
}

func (b *androidPlayerBackend) Name() string {
	return "android"
}

func (b *androidPlayerBackend) Mode() string {
	if b.config == nil || strings.TrimSpace(b.config.AndroidPlayerMode) == "" {
		return "intent"
	}
	return strings.ToLower(strings.TrimSpace(b.config.AndroidPlayerMode))
}

func (b *androidPlayerBackend) SupportsAutomation() bool {
	return b.Mode() == "ipc"
}

func (b *androidPlayerBackend) Start(link string, args []string, title string, anime *Anime) (string, error) {
	if b.Mode() != "ipc" {
		return b.startIntent(link, title)
	}

	socketPath := androidConfiguredSocketPath(b.config)
	if socketPath == "" {
		CurdOut("Android IPC mode is missing a socket path. Falling back to intent mode.")
		return b.startIntent(link, title)
	}
	socketAssessment := assessAndroidSocketPath(socketPath)
	if !socketAssessment.Supported {
		for _, warning := range socketAssessment.Warnings {
			CurdOut("Android IPC mode unavailable: " + warning)
		}
		return b.startIntent(link, title)
	}
	cmdArgs := BuildAndroidIntentCommand(b.config, link, title, socketPath)
	cmd := exec.Command("am", cmdArgs...)
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("failed to start Android player in IPC mode: %w", err)
	}

	if !waitForAndroidSocket(socketPath, 5*time.Second) {
		CurdOut("Android player did not expose IPC control. Falling back to intent mode.")
		return b.startIntent(link, title)
	}

	if anime != nil {
		anime.Ep.Player.SocketPath = socketPath
	}
	return socketPath, nil
}

func (b *androidPlayerBackend) startIntent(link string, title string) (string, error) {
	cmdArgs := BuildAndroidIntentCommand(b.config, link, title, "")
	cmd := exec.Command("am", cmdArgs...)
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("failed to start Android intent player: %w", err)
	}

	return androidIntentSocketPath, nil
}

func (b *androidPlayerBackend) SendCommand(target string, command []interface{}) (interface{}, error) {
	if !b.SupportsAutomation() || target == androidIntentSocketPath {
		return nil, fmt.Errorf("android intent mode does not support playback automation")
	}

	return sendMPVCommandOverIPC(target, command)
}

func startDesktopVideo(link string, args []string, title string, anime *Anime) (string, error) {
	var command *exec.Cmd
	var mpvSocketPath string
	var err error

	userConfig := GetGlobalConfig()

	if userConfig != nil && userConfig.MpvArgs != nil {
		args = append(args, userConfig.MpvArgs...)
	}

	if anime.Ep.Player.SocketPath != "" && IsMPVRunning(anime.Ep.Player.SocketPath) {
		mpvSocketPath = anime.Ep.Player.SocketPath

		command := []interface{}{"loadfile", link}
		_, err = sendMPVCommandOverIPC(mpvSocketPath, command)
		if err != nil {
			return "", fmt.Errorf("failed to load file in existing MPV instance: %w", err)
		}

		time.Sleep(100 * time.Millisecond)

		titleCommand := []interface{}{"set_property", "force-media-title", title}
		_, err = sendMPVCommandOverIPC(mpvSocketPath, titleCommand)
		if err != nil {
			Log(fmt.Sprintf("Failed to update title: %v", err))
		}

		windowTitleCommand := []interface{}{"set_property", "title", title}
		_, err = sendMPVCommandOverIPC(mpvSocketPath, windowTitleCommand)
		if err != nil {
			Log(fmt.Sprintf("Failed to update window title: %v", err))
		}

		return mpvSocketPath, nil
	}

	if anime.Ep.Player.SocketPath == "" {
		randomBytes := make([]byte, 4)
		_, err = rand.Read(randomBytes)
		if err != nil {
			Log("Failed to generate random number")
			return "", fmt.Errorf("failed to generate random number: %w", err)
		}

		randomNumber := fmt.Sprintf("%x", randomBytes)
		if runtime.GOOS == "windows" {
			mpvSocketPath = fmt.Sprintf(`\\.\pipe\curd_mpvsocket_%s`, randomNumber)
		} else {
			mpvSocketPath = filepath.Join(os.TempDir(), fmt.Sprintf("curd_mpvsocket_%s", randomNumber))
		}
	} else {
		mpvSocketPath = anime.Ep.Player.SocketPath
	}

	titleArgs := []string{fmt.Sprintf("--title=%s", title), fmt.Sprintf("--force-media-title=%s", title)}
	args = append(args, "--force-window=yes", "--idle=yes")
	args = append(args, titleArgs...)

	var mpvArgs []string
	mpvArgs = append(mpvArgs, "--no-terminal", "--really-quiet", fmt.Sprintf("--input-ipc-server=%s", mpvSocketPath))
	if len(args) > 0 {
		mpvArgs = append(mpvArgs, args...)
	}
	mpvArgs = append(mpvArgs, link)

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
	if err = command.Start(); err != nil {
		CurdOut(fmt.Sprintf("Error: Failed to start %s process", effectivePlayerName))
		return "", fmt.Errorf("failed to start %s: %w", effectivePlayerName, err)
	}

	socketReady := false
	maxRetries := 10
	retryDelay := 300 * time.Millisecond

	Log(fmt.Sprintf("Waiting for MPV socket to be ready at %s", mpvSocketPath))
	for i := 0; i < maxRetries; i++ {
		time.Sleep(retryDelay)

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
	}

	return mpvSocketPath, nil
}
