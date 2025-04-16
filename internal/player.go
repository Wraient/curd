package internal

import (
	// "fmt"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

var logFile = "debug.log"

func getMPVPath() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", err
	}
	exeDir := filepath.Dir(exePath)
	mpvPath := filepath.Join(exeDir, "bin", "mpv.exe") // Adjust the relative path
	return mpvPath, nil
}

func StartVideo(link string, args []string, title string, anime *Anime) (string, error) {
	var command *exec.Cmd
	var mpvSocketPath string
	var err error

	userConfig := GetGlobalConfig()

	// Add custom MPV arguments from config if they exist
	if userConfig.MpvArgs != nil {
		args = append(args, userConfig.MpvArgs...)
	}

	// Check if we have an existing socket and if MPV is still running
	if anime.Ep.Player.SocketPath != "" && IsMPVRunning(anime.Ep.Player.SocketPath) {
		// Reuse existing socket
		mpvSocketPath = anime.Ep.Player.SocketPath

		// Load the new file in the existing MPV instance
		command := []interface{}{"loadfile", link}
		_, err = MPVSendCommand(mpvSocketPath, command)
		if err != nil {
			return "", fmt.Errorf("failed to load file in existing MPV instance: %w", err)
		}

		// Wait a brief moment for the file to load
		time.Sleep(100 * time.Millisecond)

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

	// Prepare arguments for mpv
	var mpvArgs []string
	mpvArgs = append(mpvArgs, "--no-terminal", "--really-quiet", fmt.Sprintf("--input-ipc-server=%s", mpvSocketPath), link)
	// Add any additional arguments passed
	if len(args) > 0 {
		mpvArgs = append(mpvArgs, args...)
	}

	if runtime.GOOS == "windows" {
		// Get the path to mpv.exe for Windows
		mpvPath, err := getMPVPath()
		if err != nil {
			CurdOut("Error: Failed to get MPV path")
			Log("Failed to get mpv path.")
			return "", err
		}

		// Create command for Windows
		command = exec.Command(mpvPath, mpvArgs...)
	} else {
		// Create command for Unix-like systems
		command = exec.Command("mpv", mpvArgs...)
	}

	// Start the mpv process
	err = command.Start()
	if err != nil {
		CurdOut("Error: Failed to start mpv process")
		return "", fmt.Errorf("failed to start mpv: %w", err)
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

		conn, err := connectToPipe(ipcSocketPath)
		if err != nil {
			lastErr = err
			Log(fmt.Sprintf("Connect error (attempt %d/%d): %v", attempt+1, maxRetries, err))
			continue // Try again
		}
		defer conn.Close()

		commandStr, err := json.Marshal(map[string]interface{}{
			"command": command,
		})
		if err != nil {
			return nil, err // Don't retry on JSON marshalling errors
		}

		// Send the command
		_, err = conn.Write(append(commandStr, '\n'))
		if err != nil {
			lastErr = err
			Log(fmt.Sprintf("Write error (attempt %d/%d): %v", attempt+1, maxRetries, err))
			continue // Try again
		}

		// Receive the response with timeout
		buf := make([]byte, 4096)
		// Set read deadline for 1 second
		if deadline, ok := conn.(interface{ SetReadDeadline(time.Time) error }); ok {
			deadline.SetReadDeadline(time.Now().Add(1 * time.Second))
		}

		n, err := conn.Read(buf)
		if err != nil {
			lastErr = err
			Log(fmt.Sprintf("Read error (attempt %d/%d): %v", attempt+1, maxRetries, err))
			continue // Try again
		}

		var response map[string]interface{}
		if err := json.Unmarshal(buf[:n], &response); err != nil {
			lastErr = err
			Log(fmt.Sprintf("JSON parse error (attempt %d/%d): %v", attempt+1, maxRetries, err))
			continue // Try again
		}

		// Success!
		if data, exists := response["data"]; exists {
			return data, nil
		}
		return nil, nil
	}

	// All retries failed
	return nil, fmt.Errorf("command failed after %d attempts: %w", maxRetries, lastErr)
}

func SeekMPV(ipcSocketPath string, time int) (interface{}, error) {
	command := []interface{}{"seek", time, "absolute"}
	return MPVSendCommand(ipcSocketPath, command)
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
