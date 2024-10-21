package main

import (
	"fmt"
	"os"
	"time"
	"github.com/wraient/curd/internal"
)

func main() {

	discordClientId := "1287457464148820089"

	// Setup

	// internal.ClearScreen()
	// defer internal.RestoreScreen()

	var anime internal.Anime
	var user internal.User

	configFilePath := os.ExpandEnv("$HOME/Projects/curd/.config/curd/curd_config.txt")
	logFile := "debug.log"

	// load curd userCurdConfig
	userCurdConfig, err := internal.LoadConfig(configFilePath)
	if err != nil {
		fmt.Println("Error loading config:", err)
		return
	}

	internal.Log(userCurdConfig, logFile)

	// Get the token from the token file
	user.Token, err = internal.GetTokenFromFile("/home/wraient/.local/share/curd/token")
	if err != nil {
		internal.Log("Error reading token", logFile)
	}

	// Load anime in database
	databaseFile := os.ExpandEnv("$HOME/Projects/curd/.config/curd/curd_history.txt")
	databaseAnimes := internal.LocalGetAllAnime(databaseFile)

	internal.SetupCurd(&userCurdConfig, &anime, &user, &databaseAnimes, logFile)

	// Main loop
	// for {
		// Get MalId and CoverImage (only if discord presence is enabled)
		if userCurdConfig.DiscordPresence {
			anime.MalId, anime.CoverImage, err = internal.GetAnimeIDAndImage(anime.AnilistId)
			if err != nil {
				internal.Log("Error getting anime ID and image: "+err.Error(), logFile)
			}
			err = internal.DiscordPresence(discordClientId, anime)
			if err != nil {
				internal.Log("Error setting Discord presence: "+err.Error(), logFile)
			}
		} else {
			anime.MalId, err = internal.GetAnimeMalID(anime.AnilistId)
			if err != nil {
				internal.Log("Error getting anime MAL ID: "+err.Error(), logFile)
			}
		}

		// Get episode data
		go func(){
			err = internal.GetEpisodeData(anime.MalId, anime.Ep.Number, &anime)
			if err != nil {
				internal.Log("Error getting episode data: "+err.Error(), logFile)
			}
			internal.Log(anime, logFile)
		}()

		mpvSocketPath := internal.StartCurd(&userCurdConfig, &anime, logFile)
		
		internal.Log(fmt.Sprint("starting from: ", anime.Ep.Player.PlaybackTime), logFile)
		internal.Log(mpvSocketPath, logFile)

		// Thread to update Discord presence
		go func() {
			for {
				err = internal.DiscordPresence(discordClientId, anime)
				if err != nil {
					internal.Log("Error setting Discord presence: "+err.Error(), logFile)
				}
				time.Sleep(1 * time.Second)
			}
		}()

		// Thread to update playback time in database
		go func() {
			for {
				time.Sleep(1 * time.Second)
				// Get current playback time
				timePos, err := internal.MPVSendCommand(mpvSocketPath, []interface{}{"get_property", "time-pos"})
				if err != nil {
					internal.Log("Error getting playback time: "+err.Error(), logFile)

					// User closed the video
					if anime.Ep.Started {
						fmt.Println("Have a great day!")
						os.Exit(0)
					}
				}

				// Convert timePos to integer
				if timePos != nil {
					if !anime.Ep.Started {
						anime.Ep.Started = true
					}

					// If resume is true, seek to the playback time
					if anime.Ep.Resume {
						internal.SeekMPV(mpvSocketPath, anime.Ep.Player.PlaybackTime)
						anime.Ep.Resume = false
					}

					animePosition, ok := timePos.(float64)
					if !ok {
						internal.Log("Error: timePos is not a float64", logFile)
						continue
					}

					anime.Ep.Player.PlaybackTime = int(animePosition + 0.5) // Round to nearest integer
					// Update Local Database
					internal.LocalUpdateAnime(databaseFile, anime.AnilistId, anime.AllanimeId, anime.Ep.Number, anime.Ep.Player.PlaybackTime, anime.Title.English)
				}
			}
		}()

		// Main playback loop
		for {
			// Check if MPV has started
			currentTime, err := internal.MPVSendCommand(mpvSocketPath, []interface{}{"get_property", "time-pos"})
			if err != nil || currentTime == nil {
			}
			time.Sleep(1 * time.Second) // Wait before checking again
		}
}
