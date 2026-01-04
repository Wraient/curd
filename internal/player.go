package internal

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
	"errors"
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

		if _, err := os.Stat(mpvPath); err == nil {
			command = exec.Command(mpvPath, mpvArgs...)

		} else if errors.Is(err, os.ErrNotExist) {
			//for windows with mpv on PATH
			command = exec.Command("mpv", mpvArgs...)

		} else {
			Log("Failed to get mpv path.")
			return "", err
		}
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
					eventType := event.(string)
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
								propertyName := name.(string)
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
