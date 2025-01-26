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

		// Set the new title
		_, err = MPVSendCommand(mpvSocketPath, []interface{}{"set_property", "force-media-title", title})
		if err != nil {
			Log(fmt.Sprintf("Failed to set title: %v", err), logFile)
		}

		return mpvSocketPath, nil
	}

	if anime.Ep.Player.SocketPath == "" {
    // Generate a random number for the socket path
    randomBytes := make([]byte, 4)
		_, err = rand.Read(randomBytes)
    if err != nil {
        Log("Failed to generate random number", logFile)
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
            Log("Failed to get mpv path.", logFile)
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
    conn, err := connectToPipe(ipcSocketPath)
    if err != nil {
        return nil, err
    }
    defer conn.Close()

    commandStr, err := json.Marshal(map[string]interface{}{
        "command": command,
    })
    if err != nil {
        return nil, err
    }

    // Send the command
    _, err = conn.Write(append(commandStr, '\n'))
    if err != nil {
        return nil, err
    }

    // Receive the response
    buf := make([]byte, 4096)
    n, err := conn.Read(buf)
    if err != nil {
        return nil, err
    }

    var response map[string]interface{}
    if err := json.Unmarshal(buf[:n], &response); err != nil {
        return nil, err
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
        Log("Failed to get playback speed.", logFile)
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
	// Get the time-pos property from MPV
	timePos, err := MPVSendCommand(ipcSocketPath, []interface{}{"get_property", "time-pos"})
	if err != nil {
		// Check specifically for "property unavailable" error
		if strings.Contains(err.Error(), "property unavailable") {
			// This indicates nothing is playing
			return false, nil
		}
		return false, fmt.Errorf("error getting time-pos: %w", err)
	}

	// If we got a valid response, something is playing
	if timePos != nil {
		return true, nil
	}

	// Default to false if we can't determine the status
	return false, nil
}

func IsMPVRunning(socketPath string) bool {
	if socketPath == "" {
		return false
	}

	// Try to connect to the socket
	conn, err := connectToPipe(socketPath)
	if err != nil {
		return false
	}
	defer conn.Close()

	// Send a simple command to check if MPV responds
	_, err = MPVSendCommand(socketPath, []interface{}{"get_property", "pid"})
	return err == nil
}
