package internal

import (
	// "fmt"
	"fmt"
	"os/exec"
    "encoding/json"
    "os"
)
var logFile = "debug.log"

// StartMpv starts the mpv player with the given video URL and IPC socket path.
func StartMpv(videoURL, socketPath string) error {
	cmd := exec.Command("mpv", "--input-ipc-server="+socketPath, videoURL)
	return cmd.Start()
}

// GetMpvProperty retrieves a property from mpv via IPC.
func GetMpvProperty(socketPath, property string) (string, error) {
	// Prepare the command to read from the socket
	cmd := exec.Command("mpv", "--input-ipc-server="+socketPath, "get_property", property)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get property '%s': %w", property, err)
	}
	return string(output), nil
}

// SendMpvCommand sends a command to mpv via IPC.
func SendMpvCommand(socketPath, command string, args ...string) error {
	msg := map[string]interface{}{
		"command": append([]interface{}{command}, convertArgs(args)...),
	}
	return SendMpvMessage(socketPath, msg)
}

// ConvertArgs converts []string to []interface{}.
func convertArgs(args []string) []interface{} {
	interfaceArgs := make([]interface{}, len(args))
	for i, v := range args {
		interfaceArgs[i] = v
	}
	return interfaceArgs
}

// SendMpvMessage sends a JSON message to mpv via IPC.
func SendMpvMessage(socketPath string, msg map[string]interface{}) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	socket, err := os.OpenFile(socketPath, os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		return fmt.Errorf("failed to open IPC socket: %w", err)
	}
	defer socket.Close()

	_, err = socket.Write(data)
	if err != nil {
		return fmt.Errorf("failed to send message to mpv: %w", err)
	}
	return nil
}
