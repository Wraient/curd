package internal

import (
	"fmt"
	"github.com/tr1xem/go-discordrpc/client"
	"time"
)

var discordClient *client.Client
var isLoggedIn bool
var lastPausedState bool
var lastEpisodeNumber int
var lastAnimeTitle string
var lastUpdateTime time.Time
var lastForceUpdateTime time.Time

func LoginClient(clientId string) error {
	if discordClient != nil && isLoggedIn {
		return nil // Already logged in
	}

	discordClient = client.NewClient(clientId)

	if err := discordClient.Login(); err != nil {
		return fmt.Errorf("login failed: %w", err)
	}

	isLoggedIn = true
	return nil
}

func DiscordPresence(anime Anime, IsPaused bool, currentPosition int, totalDuration int, clientId string) error {
	return DiscordPresenceWithForce(anime, IsPaused, currentPosition, totalDuration, clientId, false)
}

func DiscordPresenceWithForce(anime Anime, IsPaused bool, currentPosition int, totalDuration int, clientId string, forceUpdate bool) error {
	// Ensure client is logged in
	if discordClient == nil || !isLoggedIn {
		if err := LoginClient(clientId); err != nil {
			return err
		}
	}

	currentAnimeTitle := GetAnimeName(anime)
	now := time.Now()

	// Check if we need to update based on significant changes
	shouldUpdate := false

	// Force update every 2 minutes for Discord keep-alive
	if lastForceUpdateTime.IsZero() || time.Since(lastForceUpdateTime) >= 2*time.Minute {
		shouldUpdate = true
		lastForceUpdateTime = now
	}

	if lastUpdateTime.IsZero() ||
		lastPausedState != IsPaused ||
		lastEpisodeNumber != anime.Ep.Number ||
		lastAnimeTitle != currentAnimeTitle ||
		forceUpdate {
		shouldUpdate = true
	}

	if !shouldUpdate {
		return nil // Skip update
	}

	var timestamps *client.Timestamps
	var SmallImage = "pause-button"
	var SmallText = "pause-button"

	startTime := now.Add(-time.Duration(currentPosition) * time.Second)

	if IsPaused {
		timestamps = &client.Timestamps{
			Start: &startTime,
			End:   nil, // No end time when paused
		}
		SmallImage = "pause-button"
		SmallText = "Paused"
	} else {
		// Only set end time if we have a reasonable duration (more than 1 minute total)
		if totalDuration > 60 && totalDuration > currentPosition { // Real duration (more than 1 minute total)
			remainingSeconds := totalDuration - currentPosition
			endTime := now.Add(time.Duration(remainingSeconds) * time.Second)
			timestamps = &client.Timestamps{
				Start: &startTime,
				End:   &endTime,
			}
		} else {
			// Duration unknown, show elapsed time only
			timestamps = &client.Timestamps{
				Start: &startTime,
				End:   nil,
			}
		}
		SmallImage = ""
		SmallText = ""
	}

	err := discordClient.SetActivity(client.Activity{
		Type:       3, // Watching
		Name:       currentAnimeTitle,
		Details:    currentAnimeTitle, // Large text
		LargeImage: anime.CoverImage,
		LargeText:  currentAnimeTitle, // Would display while hovering over the large image
		State: fmt.Sprintf("Episode %d%s", anime.Ep.Number, func() string {
			if IsPaused {
				return " (Paused)"
			}
			return ""
		}()),
		SmallImage: SmallImage,
		SmallText:  SmallText,
		Timestamps: timestamps,
		Buttons: []*client.Button{
			{
				Label: "View on AniList",                                           // Button label
				Url:   fmt.Sprintf("https://anilist.co/anime/%d", anime.AnilistId), // Button link
			},
			{
				Label: "View on MAL",                                                // Button label
				Url:   fmt.Sprintf("https://myanimelist.net/anime/%d", anime.MalId), // Button link
			},
		},
	})

	if err != nil {
		return fmt.Errorf("failed to set Discord activity: %w", err)
	}

	lastPausedState = IsPaused
	lastEpisodeNumber = anime.Ep.Number
	lastAnimeTitle = currentAnimeTitle
	lastUpdateTime = now

	fmt.Println("Discord presence updated!", time.Now())
	return nil
}

func LogoutClient() error {
	if discordClient != nil && isLoggedIn {
		if err := discordClient.Logout(); err != nil {
			return fmt.Errorf("logout failed: %w", err)
		}
		isLoggedIn = false
		discordClient = nil
		// fmt.Println("Discord RPC logged out!")
	}
	return nil
}

func FormatTime(seconds int) string {
	hours := seconds / 3600
	minutes := (seconds % 3600) / 60
	remainingSeconds := seconds % 60

	if hours > 0 {
		return fmt.Sprintf("%d:%02d:%02d", hours, minutes, remainingSeconds)
	}
	return fmt.Sprintf("%d:%02d", minutes, remainingSeconds)
}

func ConvertSecondsToMinutes(seconds int) int {
	return seconds / 60
}
