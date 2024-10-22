package main

import (
	"fmt"
	"os"
	"time"
	"sync"
	"github.com/wraient/curd/internal"
)

func main() {

	discordClientId := "1287457464148820089"
	// fmt.Println(internal.LocalGetAllAnime("/home/wraient/Projects/curd/.config/curd/curd_history.txt"))
	// internal.ExitCurd()

	// Setup

	internal.ClearScreen()
	defer internal.RestoreScreen()

	var anime internal.Anime
	var user internal.User

	configFilePath := os.ExpandEnv("$HOME/Projects/curd/.config/curd/curd_config.txt")
	logFile := "debug.log"
	internal.ClearLogFile(logFile)

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

	// Create a channel to signal when to exit the skip loop
	var wg sync.WaitGroup
	
	// Main loop
	for {
		skipLoopDone := make(chan struct{})

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

		
		// Start curd
		if !anime.Ep.IsFiller {
			// fmt.Println("Not a filler episode, Starting: ", anime.Ep.Number)
			anime.Ep.Player.SocketPath = internal.StartCurd(&userCurdConfig, &anime, logFile)
			
			internal.Log(fmt.Sprint("Playback starting time: ", anime.Ep.Player.PlaybackTime), logFile)
			internal.Log(anime.Ep.Player.SocketPath, logFile)
		} else {
			fmt.Println("Filler episode, skipping: ", anime.Ep.Number)
		}

		wg.Add(1)
		// Get episode data
		go func(){
			defer wg.Done()
			err = internal.GetEpisodeData(anime.MalId, anime.Ep.Number, &anime)
			if err != nil {
				internal.Log("Error getting episode data: "+err.Error(), logFile)
			} else{
				internal.Log(anime, logFile)
				
				// if filler episode
				if anime.Ep.IsFiller {
					fmt.Println("Filler Episode, starting next episode: ", anime.Ep.Number+1)
					internal.Log("Filler episode detected", logFile)
					anime.Ep.Number++
					anime.Ep.Started = false
					internal.Log("Skipping filler episode, starting next.", logFile)
					internal.LocalUpdateAnime(databaseFile, anime.AnilistId, anime.AllanimeId, anime.Ep.Number, anime.Ep.Player.PlaybackTime, internal.GetAnimeName(anime))
					// Send command to close MPV
					_, err := internal.MPVSendCommand(anime.Ep.Player.SocketPath, []interface{}{"quit"})
					if err != nil {
						internal.Log("Error closing MPV: "+err.Error(), logFile)
					}
					// Exit the skip loop
					close(skipLoopDone)
				} else {
					// fmt.Println("Not a filler episode, Starting: ", anime.Ep.Number)
				}
			}
		}()

		wg.Add(1)
		// Thread to update Discord presence
		go func() {
			defer wg.Done()
			for {
				select {
				case <-skipLoopDone:
					return
				default:
					err = internal.DiscordPresence(discordClientId, anime)
					if err != nil {
						internal.Log("Error setting Discord presence: "+err.Error(), logFile)
					}
					time.Sleep(1 * time.Second)
				}
			}
		}()

		// Get skip times Parallel
		go func() {
			err = internal.GetAndParseAniSkipData(anime.MalId, anime.Ep.Number, 1, &anime)
			if err != nil {
				internal.Log("Error getting and parsing AniSkip data: "+err.Error(), logFile)
			}
			internal.Log(anime.Ep.SkipTimes, logFile)
		}()

		wg.Add(1)
		// Thread to update playback time in database
		go func() {
			defer wg.Done()
			for {
				select {
				case <-skipLoopDone:
					return
				default:
					time.Sleep(1 * time.Second)

					// Get current playback time
					internal.Log("Getting playback time "+anime.Ep.Player.SocketPath, logFile)
					timePos, err := internal.MPVSendCommand(anime.Ep.Player.SocketPath, []interface{}{"get_property", "time-pos"})
					if err != nil {
						internal.Log("Error getting playback time: "+err.Error(), logFile)

						// User closed the video
						if anime.Ep.Started {
							percentageWatched := internal.PercentageWatched(anime.Ep.Player.PlaybackTime, anime.Ep.Duration)
							if int(percentageWatched) >= userCurdConfig.PercentageToMarkComplete {			
								anime.Ep.Number++
								anime.Ep.Started = false
								internal.Log("Completed episode, starting next.", logFile)
								// Exit the skip loop
								close(skipLoopDone)
							} else {
								fmt.Println("Have a great day!")
								internal.ExitCurd()
							}
						}
						continue
					}

					// Convert timePos to integer
					if timePos != nil {
						if !anime.Ep.Started {
							anime.Ep.Started = true
						}

						// If resume is true, seek to the playback time
						if anime.Ep.Resume {
							internal.SeekMPV(anime.Ep.Player.SocketPath, anime.Ep.Player.PlaybackTime)
							anime.Ep.Resume = false
						}

						animePosition, ok := timePos.(float64)
						if !ok {
							internal.Log("Error: timePos is not a float64", logFile)
							continue
						}

						anime.Ep.Player.PlaybackTime = int(animePosition + 0.5) // Round to nearest integer
						// Update Local Database
						err = internal.LocalUpdateAnime(databaseFile, anime.AnilistId, anime.AllanimeId, anime.Ep.Number, anime.Ep.Player.PlaybackTime, internal.GetAnimeName(anime))
						if err != nil {
							internal.Log("Error updating local database: "+err.Error(), logFile)
						} else {
							internal.Log(fmt.Sprintf("Updated database: AnilistId=%d, AllanimeId=%s, EpNumber=%d, PlaybackTime=%d", 
								anime.AnilistId, anime.AllanimeId, anime.Ep.Number, anime.Ep.Player.PlaybackTime), logFile)
						}
					}
				}
			}
		}()

		// Skip OP and ED
		skipLoop:
		for {
			select {
				case <-skipLoopDone:
					// Exit signal received, break out of the skipLoop
					break skipLoop
				default:
					// Check if MPV has started
					if userCurdConfig.SkipOp {
					if anime.Ep.Player.PlaybackTime > anime.Ep.SkipTimes.Op.Start && anime.Ep.Player.PlaybackTime < anime.Ep.SkipTimes.Op.Start+2 && anime.Ep.SkipTimes.Op.Start != anime.Ep.SkipTimes.Op.End {
							internal.SeekMPV(anime.Ep.Player.SocketPath, anime.Ep.SkipTimes.Op.End)
						}
					}
					if userCurdConfig.SkipEd {
						if anime.Ep.Player.PlaybackTime > anime.Ep.SkipTimes.Ed.Start && anime.Ep.Player.PlaybackTime < anime.Ep.SkipTimes.Ed.Start+2 && anime.Ep.SkipTimes.Ed.Start != anime.Ep.SkipTimes.Ed.End {
							internal.SeekMPV(anime.Ep.Player.SocketPath, anime.Ep.SkipTimes.Ed.End)
						}
					}
			}
			time.Sleep(1 * time.Second) // Wait before checking again
		}

		// Wait for all goroutines to finish before starting the next iteration
		wg.Wait()

		// Reset the WaitGroup for the next loop
		wg = sync.WaitGroup{}

		continue
	}
}
