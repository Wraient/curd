package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/coder/websocket"
)

func StartDaemon(token string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	// Create a done channel for cleanup
	done := make(chan struct{})

	// Start signal handler
	go func() {
		sig := <-sigChan
		log.Printf("Received signal: %v", sig)
		cancel()
		close(done)
	}()

	// Setup logging
	userCurdConfig := GetGlobalConfig()
	logFile := filepath.Join(os.ExpandEnv(userCurdConfig.StoragePath), "daemon.log")
	
	f, err := os.OpenFile(logFile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		return fmt.Errorf("failed to open log file: %v", err)
	}
	defer f.Close()
	
	log.SetOutput(f)
	log.Printf("Starting Curd daemon...")

	// Connect to AniList WebSocket
	wsURL := "wss://graphql.anilist.co/websocket"
	c, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{
			"Authorization": []string{"Bearer " + token},
			"Content-Type": []string{"application/json"},
		},
	})
	if err != nil {
		return fmt.Errorf("websocket connection failed: %v", err)
	}
	defer c.Close(websocket.StatusNormalClosure, "")

	// Updated subscription message with notifications
	initMsg := map[string]interface{}{
		"type": "start",
		"id":   "1",
		"payload": map[string]interface{}{
			"query": `
				subscription {
					Notification {
						... on ActivityLikeNotification {
							id
							type
							userId
							activityId
							context
							createdAt
							user {
								name
							}
						}
						... on ActivityReplyNotification {
							id
							type
							userId
							activityId
							context
							createdAt
							user {
								name
							}
						}
						... on ActivityMentionNotification {
							id
							type
							userId
							activityId
							context
							createdAt
							user {
								name
							}
						}
						... on ActivityReplySubscribedNotification {
							id
							type
							userId
							activityId
							context
							createdAt
							user {
								name
							}
						}
						... on ThreadCommentMentionNotification {
							id
							type
							userId
							commentId
							context
							createdAt
							user {
								name
							}
						}
						... on ThreadCommentReplyNotification {
							id
							type
							userId
							commentId
							context
							createdAt
							user {
								name
							}
						}
						... on ThreadCommentSubscribedNotification {
							id
							type
							userId
							commentId
							context
							createdAt
							user {
								name
							}
						}
						... on ThreadLikeNotification {
							id
							type
							userId
							threadId
							context
							createdAt
							user {
								name
							}
						}
						... on RelatedMediaAdditionNotification {
							id
							type
							mediaId
							context
							createdAt
							media {
								title {
									userPreferred
								}
							}
						}
						... on MediaDataChangeNotification {
							id
							type
							mediaId
							context
							createdAt
							media {
								title {
									userPreferred
								}
							}
						}
						... on MediaMergeNotification {
							id
							type
							mediaId
							context
							createdAt
							media {
								title {
									userPreferred
								}
							}
						}
						... on MediaDeletionNotification {
							id
							type
							context
							createdAt
						}
					}
				}
			`,
		},
	}

	if err := c.Write(ctx, websocket.MessageText, mustJSON(initMsg)); err != nil {
		return fmt.Errorf("failed to send subscription: %v", err)
	}

	// Listen for updates
	go func() {
		for {
			select {
			case <-ctx.Done():
				log.Printf("Shutting down daemon...")
				c.Close(websocket.StatusNormalClosure, "")
				return
			default:
				_, message, err := c.Read(ctx)
				if err != nil {
					if websocket.CloseStatus(err) != websocket.StatusNormalClosure {
						log.Printf("WebSocket read error: %v", err)
					}
					continue
				}

				// Parse the notification
				var response struct {
					Type    string `json:"type"`
					Payload struct {
						Data struct {
							Notification map[string]interface{} `json:"Notification"`
						} `json:"data"`
					} `json:"payload"`
				}

				if err := json.Unmarshal(message, &response); err != nil {
					log.Printf("Failed to parse message: %v", err)
					continue
				}

				// Only process data messages
				if response.Type != "data" {
					continue
				}

				notification := response.Payload.Data.Notification
				if notification == nil {
					continue
				}

				// Log the notification
				log.Printf("Received notification: %s", string(message))

				// Send desktop notification
				notifType := notification["type"].(string)
				context := notification["context"].(string)
				
				var title, body string
				switch notifType {
				case "ACTIVITY_LIKE":
					username := notification["user"].(map[string]interface{})["name"].(string)
					title = "New Like"
					body = fmt.Sprintf("%s liked your activity", username)
				case "ACTIVITY_REPLY":
					username := notification["user"].(map[string]interface{})["name"].(string)
					title = "New Reply"
					body = fmt.Sprintf("%s replied to your activity: %s", username, context)
				// Add more cases as needed
				default:
					title = "AniList Notification"
					body = context
				}

				if err := SendNotification(title, body); err != nil {
					log.Printf("Failed to send notification: %v", err)
				}
			}
		}
	}()

	// Wait for shutdown signal
	<-done
	return nil
}

func mustJSON(v interface{}) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

// Add this new function to handle desktop notifications
func SendNotification(title, message string) error {
	cmd := exec.Command("notify-send", "--app-name=Curd", title, message)
	return cmd.Run()
}

func KillDaemon() error {
	userCurdConfig := GetGlobalConfig()
	pidFile := filepath.Join(os.ExpandEnv(userCurdConfig.StoragePath), "curd-daemon.pid")
	
	// Read PID file
	pidBytes, err := os.ReadFile(pidFile)
	if err != nil {
		return fmt.Errorf("daemon not running or PID file not found")
	}
	
	// Parse PID
	pid, err := strconv.Atoi(string(pidBytes))
	if err != nil {
		return fmt.Errorf("invalid PID in file: %v", err)
	}
	
	// Kill process
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("process not found: %v", err)
	}
	
	// Send SIGTERM signal
	if err := process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("failed to kill daemon: %v", err)
	}
	
	// Remove PID file
	if err := os.Remove(pidFile); err != nil {
		log.Printf("Warning: failed to remove PID file: %v", err)
	}
	
	return nil
}
