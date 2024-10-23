package internal

import (
	"fmt"
	"github.com/hugolgst/rich-go/client"
)

func DiscordPresence(clientId string, anime Anime) error {
	err := client.Login(clientId)
	if err != nil {
		return err
	}

	err = client.SetActivity(client.Activity{
		Details:    fmt.Sprintf("%s", GetAnimeName(anime)), // Large text
		LargeImage: anime.CoverImage,
		LargeText:  GetAnimeName(anime), // Would display while hovering over the large image
		State:      fmt.Sprintf("\nEpisode %d - %d:%02d / %d:%02d", 
			anime.Ep.Number, 
			ConvertSecondsToMinutes(anime.Ep.Player.PlaybackTime), 
			anime.Ep.Player.PlaybackTime % 60,
			ConvertSecondsToMinutes(anime.Ep.Duration),
			anime.Ep.Duration % 60),
		// SmallImage: anime.CoverImage, // Image would appear in the bottom left corner
		// SmallText:  fmt.Sprintf("%s", anime.Ep.Title.English), // Would display while hovering over the small image
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

func ConvertSecondsToMinutes(seconds int) int {
	return seconds / 60
}
