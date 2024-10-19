package internal

import (
	// "fmt"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"time"
	"math/rand"
)
var logFile = "debug.log"

// StartMpv starts the mpv player with the given video URL and return ipc socket path.
func StartMpv(videoURL string) (string, error) {
	// Generate a random mpvsocket path (Linux only)
	randomNumber := rand.Intn(100) // Change 100 to the desired range
	fmt.Println("random number", randomNumber)
	mpvSocketPath := "/tmp/mpvsocket"+strconv.Itoa(randomNumber)
	
	cmd := exec.Command("mpv", "--input-ipc-server="+mpvSocketPath, videoURL)
	return mpvSocketPath, cmd.Start()
}

// sendIpcMessage sends a JSON message to the mpv IPC socket and reads the response.
func sendIpcMessage(socketPath string, msg map[string]interface{}) (map[string]interface{}, error) {
	// Create a Unix socket connection
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to IPC socket: %w", err)
	}
	defer conn.Close()

	// Marshal the message into JSON
	data, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON message: %w", err)
	}

	// Write the message to the socket
	_, err = conn.Write(append(data, '\n')) // Ensure newline at the end
	if err != nil {
		return nil, fmt.Errorf("failed to send message to IPC socket: %w", err)
	}

	// Set a read deadline to prevent blocking forever
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))

	// Read the response from the socket
	var response map[string]interface{}
	decoder := json.NewDecoder(conn)
	if err := decoder.Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return response, nil
}

// GetMpvProperty retrieves a property from mpv via IPC.
func GetMpvProperty(socketPath, property string) (string, error) {
	// Prepare the JSON message to request the property
	msg := map[string]interface{}{
		"command": []interface{}{"get_property", property},
	}

	// Send the message via IPC and get the response
	response, err := sendIpcMessage(socketPath, msg)
	if err != nil {
		return "", fmt.Errorf("failed to send IPC message: %w", err)
	}

	// Check the response for errors and extract the value
	if errVal, ok := response["error"]; ok && errVal != "success" {
		return "", fmt.Errorf("mpv returned an error: %v", errVal)
	}

	// Retrieve the value from the response
	if data, ok := response["data"]; ok {
		return fmt.Sprintf("%v", data), nil
	}

	return "", fmt.Errorf("no data in response")
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
