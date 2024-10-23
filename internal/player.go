package internal

import (
	// "fmt"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
)
var logFile = "debug.log"

func StartVideo(link string, args []string) (string, error) {
    // Generate a random number for the socket path
    randomBytes := make([]byte, 4)
    _, err := rand.Read(randomBytes)
    if err != nil {
        Log("Failed to generate random number", logFile)
        return "", fmt.Errorf("failed to generate random number: %w", err)
    }
    randomNumber := fmt.Sprintf("%x", randomBytes)

    // Create the mpv socket path with the random number
    mpvSocketPath := fmt.Sprintf("/tmp/curd/curd_mpvsocket_%s", randomNumber)
    argsStr := ""
    if len(args) > 0 {
        argsStr = " " + joinArgs(args)
    }

    // Build the complete command string
    command := fmt.Sprintf("mpv%s --no-terminal --really-quiet --input-ipc-server=%s %s", argsStr, mpvSocketPath, link)

    // Execute the command using exec
    cmd := exec.Command("bash", "-c", command)
    cmd.Start()

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
    conn, err := net.Dial("unix", ipcSocketPath)
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
    conn.Write(append(commandStr, '\n'))

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
