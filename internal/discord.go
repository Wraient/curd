package internal

import (
	"fmt"
	"github.com/hugolgst/rich-go/client"
)

func DiscordPresence(clientId string, anime Anime, IsPaused bool) error {
	err := client.Login(clientId)
	if err != nil {
		return err
	}

	var state string
	if IsPaused {
		state = fmt.Sprintf("\nEpisode %d - %s (Paused)", 
			anime.Ep.Number, 
			FormatTime(anime.Ep.Player.PlaybackTime),
		)
	} else {
		state = fmt.Sprintf("\nEpisode %d - %s / %s", 
			anime.Ep.Number, 
			FormatTime(anime.Ep.Player.PlaybackTime), 
			FormatTime(anime.Ep.Duration),
		)
	}

	err = client.SetActivity(client.Activity{
		Details:    fmt.Sprintf("%s", GetAnimeName(anime)), // Large text
		LargeImage: anime.CoverImage,
		LargeText:  GetAnimeName(anime), // Displays while hovering over the large image
		State:      state,

		//SmallImage: anime.SmallCoverImage, // Image for the bottom left corner
		//SmallText:  fmt.Sprintf("Episode: %s", anime.Ep.Title.English), // Text when hovering over the small image
		
		Buttons: []*client.Button{
			&client.Button{
				Label: "View on AniList", // Button label
				Url:   fmt.Sprintf("https://anilist.co/anime/%d", anime.AnilistId), // Button link
			},
			&client.Button{
				Label: "View on MAL", // Button label
				Url:   fmt.Sprintf("https://myanimelist.net/anime/%d", anime.MalId), // Button link
			},
		},
	})
	if err != nil {
		return err
	}
	return nil
}

func FormatTime(seconds int) string {
	hours := seconds / 3600
	minutes := (seconds % 3600) / 60
	remainingSeconds := seconds % 60
	return fmt.Sprintf("%d:%02d:%02d", hours, minutes, remainingSeconds)
}
