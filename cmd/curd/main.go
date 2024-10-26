package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"runtime"
	"sync"
	"time"
	"github.com/wraient/curd/internal"
)

func main() {

	discordClientId := "1287457464148820089"

	// Setup
	internal.ClearScreen()
	defer internal.RestoreScreen()

	var anime internal.Anime
	var user internal.User

    var homeDir string
    if runtime.GOOS == "windows" {
        homeDir = os.Getenv("USERPROFILE")
    } else {
        homeDir = os.Getenv("HOME")
    }

    configFilePath := filepath.Join(homeDir, ".config", "curd", "curd.conf")

	// load curd userCurdConfig
	userCurdConfig, err := internal.LoadConfig(configFilePath)
	if err != nil {
		fmt.Println("Error loading config:", err)
		return
	}

	logFile := filepath.Join(os.ExpandEnv(userCurdConfig.StoragePath), "debug.log")
	internal.ClearLogFile(logFile)

	// Flags configured here cause userconfig needs to be changed.
	flag.StringVar(&userCurdConfig.Player, "player", userCurdConfig.Player, "Player to use for playback (Only mpv supported currently)")
	flag.StringVar(&userCurdConfig.StoragePath, "storage-path", userCurdConfig.StoragePath, "Path to the storage directory")
	flag.StringVar(&userCurdConfig.SubsLanguage, "subs-lang", userCurdConfig.SubsLanguage, "Subtitles language")
	flag.IntVar(&userCurdConfig.PercentageToMarkComplete, "percentage-to-mark-complete", userCurdConfig.PercentageToMarkComplete, "Percentage to mark episode as complete")

	// Boolean flags that accept true/false
	flag.BoolVar(&userCurdConfig.NextEpisodePrompt, "next-episode-prompt", userCurdConfig.NextEpisodePrompt, "Prompt for the next episode (true/false)")
	flag.BoolVar(&userCurdConfig.SkipOp, "skip-op", userCurdConfig.SkipOp, "Skip opening (true/false)")
	flag.BoolVar(&userCurdConfig.SkipEd, "skip-ed", userCurdConfig.SkipEd, "Skip ending (true/false)")
	flag.BoolVar(&userCurdConfig.SkipFiller, "skip-filler", userCurdConfig.SkipFiller, "Skip filler episodes (true/false)")
	flag.BoolVar(&userCurdConfig.SkipRecap, "skip-recap", userCurdConfig.SkipRecap, "Skip recap (true/false)")
	flag.BoolVar(&userCurdConfig.ScoreOnCompletion, "score-on-completion", userCurdConfig.ScoreOnCompletion, "Score on episode completion (true/false)")
	flag.BoolVar(&userCurdConfig.SaveMpvSpeed, "save-mpv-speed", userCurdConfig.SaveMpvSpeed, "Save MPV speed setting (true/false)")
	flag.BoolVar(&userCurdConfig.DiscordPresence, "discord-presence", userCurdConfig.DiscordPresence, "Enable Discord presence (true/false)")
	continueLast := flag.Bool("c", false, "Continue last episode")
	addNewAnime := flag.Bool("new", false, "Add new anime")
	updateScript := flag.Bool("u", false, "Update the script")
	editConfig := flag.Bool("e", false, "Edit config")
	subFlag := flag.Bool("sub", false, "Watch sub version")
	dubFlag := flag.Bool("dub", false, "Watch dub version")
	
	// Custom help/usage function
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Curd is a CLI tool to manage anime playback with advanced features like skipping intro, outro, filler, recap, tracking progress, and integrating with Discord.\n")
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		flag.PrintDefaults() // This prints the default flag information
	}

	flag.Parse()
	
	anime.Ep.ContinueLast = *continueLast

	if *updateScript{
		repo := "wraient/curd"
		fileName := "curd"
		
		if err := internal.UpdateCurd(repo, fileName); err != nil {
			fmt.Printf("Error updating executable: %v\n", err)
			internal.ExitCurd(err)
		} else {
			fmt.Println("Program Updated!")
			internal.ExitCurd(nil)
		}
	}

	if *editConfig {
		internal.EditConfig(configFilePath)
		return
	}
		
	// Set SubOrDub based on the flags
	if *subFlag {
		userCurdConfig.SubOrDub = "sub"
		} else if *dubFlag {
			userCurdConfig.SubOrDub = "dub"
		}
		
	// Get the token from the token file
	user.Token, err = internal.GetTokenFromFile(filepath.Join(os.ExpandEnv(userCurdConfig.StoragePath), "token"))
	if err != nil {
		internal.Log("Error reading token", logFile)
	}
	if user.Token == "" {
		fmt.Println("No token found, please generate a token from https://anilist.co/api/v2/oauth/authorize?client_id=20686&response_type=token")
		fmt.Scanln(&user.Token)
		internal.WriteTokenToFile(user.Token, filepath.Join(os.ExpandEnv(userCurdConfig.StoragePath), "token"))
	}
	
	// Load animes in database
	databaseFile := filepath.Join(os.ExpandEnv(userCurdConfig.StoragePath), "curd_history.txt")
	databaseAnimes := internal.LocalGetAllAnime(databaseFile)
	
	if *addNewAnime {
		internal.AddNewAnime(&userCurdConfig, &anime, &user, &databaseAnimes, logFile)
		internal.ExitCurd(fmt.Errorf("Added new anime!"))
	}

	internal.SetupCurd(&userCurdConfig, &anime, &user, &databaseAnimes, logFile)
	
	temp_anime, err := internal.FindAnimeByAnilistID(user.AnimeList, strconv.Itoa(anime.AnilistId))
	if err != nil {
		internal.Log("Error finding anime by Anilist ID: "+err.Error(), logFile)
	}
	
	if anime.TotalEpisodes == temp_anime.Progress {
		internal.Log(temp_anime.Progress, logFile)
		internal.Log(anime.TotalEpisodes, logFile)
		internal.Log(user.AnimeList, logFile)
		internal.Log("Rewatching anime: "+internal.GetAnimeName(anime), logFile)
		anime.Rewatching = true	
	}

	anime.Ep.Player.Speed = 1.0

	// Main loop
	for {
		// Create a channel to signal when to exit the skip loop
		var wg sync.WaitGroup
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
		go func() {
			defer wg.Done()
			err = internal.GetEpisodeData(anime.MalId, anime.Ep.Number, &anime)
			if err != nil {
				internal.Log("Error getting episode data: "+err.Error(), logFile)
			} else {
				internal.Log(anime, logFile)

				// if filler episode or recap episode and skip is enabled
				if (userCurdConfig.SkipFiller || userCurdConfig.SkipRecap) && (anime.Ep.IsFiller || anime.Ep.IsRecap) {
					if anime.Ep.IsFiller && userCurdConfig.SkipFiller {
						fmt.Println("Filler Episode, starting next episode: ", anime.Ep.Number+1)
						internal.Log("Filler episode detected", logFile)
					} else if anime.Ep.IsRecap && userCurdConfig.SkipRecap {
						fmt.Println("Recap Episode, starting next episode: ", anime.Ep.Number+1)
						internal.Log("Recap episode detected", logFile)
					} 
					anime.Ep.Number++
					anime.Ep.Started = false
					anime.Ep.IsCompleted = true
					internal.Log("Skipping filler episode, starting next.", logFile)
					internal.LocalUpdateAnime(databaseFile, anime.AnilistId, anime.AllanimeId, anime.Ep.Number, anime.Ep.Player.PlaybackTime, internal.ConvertSecondsToMinutes(anime.Ep.Duration), internal.GetAnimeName(anime))
					// Send command to close MPV
					_, err := internal.MPVSendCommand(anime.Ep.Player.SocketPath, []interface{}{"quit"})
					if err != nil {
						internal.Log("Error closing MPV: "+err.Error(), logFile)
					}
					// Exit the skip loop
					close(skipLoopDone)
				} 
			}
		}()

		wg.Add(1)
		// Thread to update Discord presence
		go func() {
			defer wg.Done()
			if userCurdConfig.DiscordPresence {
				for {
					select {
					case <-skipLoopDone:
						return
					default:
						err = internal.DiscordPresence(discordClientId, anime)
						if err != nil {
							// internal.Log("Error setting Discord presence: "+err.Error(), logFile)
						}
						time.Sleep(1 * time.Second)
					}
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

		// Get video duration
		go func() {
			for {
				if anime.Ep.Started {
					if anime.Ep.Duration == 0 {
						// Get video duration
						durationPos, err := internal.MPVSendCommand(anime.Ep.Player.SocketPath, []interface{}{"get_property", "duration"})
						if err != nil {
							internal.Log("Error getting video duration: "+err.Error(), logFile)
							} else if durationPos != nil {
								if duration, ok := durationPos.(float64); ok {
									anime.Ep.Duration = int(duration + 0.5) // Round to nearest integer
									internal.Log(fmt.Sprintf("Video duration: %d seconds", anime.Ep.Duration), logFile)
									} else {
								internal.Log("Error: duration is not a float64", logFile)
							}
						}
						break	
					}
				}
				time.Sleep(1 * time.Second)
			}
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
					// internal.Log("Getting playback time "+anime.Ep.Player.SocketPath, logFile)
					timePos, err := internal.MPVSendCommand(anime.Ep.Player.SocketPath, []interface{}{"get_property", "time-pos"})
					if err != nil {
						internal.Log("Error getting playback time: "+err.Error(), logFile)

						// Check if the error is due to invalid JSON
						// User closed the video
						if anime.Ep.Started {
							percentageWatched := internal.PercentageWatched(anime.Ep.Player.PlaybackTime, anime.Ep.Duration)
							// Episode is completed
							internal.Log(fmt.Sprint(percentageWatched), logFile)
							internal.Log(fmt.Sprint(anime.Ep.Player.PlaybackTime), logFile)
							internal.Log(fmt.Sprint(anime.Ep.Duration), logFile)
							internal.Log(fmt.Sprint(userCurdConfig.PercentageToMarkComplete), logFile)
							if int(percentageWatched) >= userCurdConfig.PercentageToMarkComplete {
								anime.Ep.Number++
								anime.Ep.Started = false
								internal.Log("Completed episode, starting next.", logFile)
								anime.Ep.IsCompleted = true
								// Exit the skip loop
								close(skipLoopDone)
							} else if fmt.Sprintf("%v", err) == "invalid character '{' after top-level value" { // Episode is not completed
								internal.Log("Received invalid JSON response, continuing...", logFile)
							} else {
								internal.Log("Episode is not completed, exiting", logFile)
								internal.ExitCurd(nil)
							}
						}
					}

					// Convert timePos to integer
					if timePos != nil {
						if !anime.Ep.Started {
							anime.Ep.Started = true
							// Set the playback speed
							if userCurdConfig.SaveMpvSpeed {
								speedCmd := []interface{}{"set_property", "speed", anime.Ep.Player.Speed}
								_, err := internal.MPVSendCommand(anime.Ep.Player.SocketPath, speedCmd)
								if err != nil {
									internal.Log("Error setting playback speed: "+err.Error(), logFile)
								}
							}
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
						err = internal.LocalUpdateAnime(databaseFile, anime.AnilistId, anime.AllanimeId, anime.Ep.Number, anime.Ep.Player.PlaybackTime, internal.ConvertSecondsToMinutes(anime.Ep.Duration), internal.GetAnimeName(anime))
						if err != nil {
							internal.Log("Error updating local database: "+err.Error(), logFile)
						} else {
							// internal.Log(fmt.Sprintf("Updated database: AnilistId=%d, AllanimeId=%s, EpNumber=%d, PlaybackTime=%d",
							// anime.AnilistId, anime.AllanimeId, anime.Ep.Number, anime.Ep.Player.PlaybackTime), logFile)
						}
					}
				}
			}
		}()

		// Skip OP and ED and Save MPV Speed
		skipLoop:
		for {
			select {
			case <-skipLoopDone:
				// Exit signal received, break out of the skipLoop
				break skipLoop
			default:
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
				anime.Ep.Player.Speed, err = internal.GetMPVPlaybackSpeed(anime.Ep.Player.SocketPath)
				if err != nil {
					internal.Log("Failed to mpv speed"+err.Error(), logFile)
				}

			}

			time.Sleep(1 * time.Second) // Wait before checking again
		}

		// Wait for all goroutines to finish before starting the next iteration
		wg.Wait()

		// Reset the WaitGroup for the next loop
		wg = sync.WaitGroup{}

		if anime.Ep.IsCompleted && !anime.Rewatching {
			go func(){
				err = internal.UpdateAnimeProgress(user.Token, anime.AnilistId, anime.Ep.Number)
				if err != nil {
					internal.Log("Error updating Anilist progress: "+err.Error(), logFile)
				}
			}()

			anime.Ep.IsCompleted = false
			// fmt.Println(anime.Ep.Number, anime.TotalEpisodes)
			if anime.Ep.Number-1 == anime.TotalEpisodes && userCurdConfig.ScoreOnCompletion {
				anime.Ep.Number = anime.Ep.Number - 1
				var userScore float64
				fmt.Println("Completed anime.")
				fmt.Println("Rate this anime: ")
				fmt.Scanln(&userScore)
				internal.RateAnime(user.Token, anime.AnilistId, userScore)
				internal.LocalDeleteAnime(databaseFile, anime.AnilistId, anime.AllanimeId)
				internal.ExitCurd(nil)
			}
		}
		if anime.Rewatching && anime.Ep.IsCompleted && anime.Ep.Number-1 == anime.TotalEpisodes {
			anime.Ep.Number = anime.Ep.Number - 1
			fmt.Println("Completed anime. (Rewatching so no scoring)")
			internal.LocalDeleteAnime(databaseFile, anime.AnilistId, anime.AllanimeId)
			internal.ExitCurd(nil)
		}
		
		if userCurdConfig.NextEpisodePrompt {
			var answer string
			fmt.Println("Start next episode? (y/n)")
			fmt.Scanln(&answer)
			if answer == "y" || answer == "Y" || answer == "yes" || answer == "Yes" || answer == "YES" || answer == "" {
			} else {
				internal.ExitCurd(nil)
			}
		}
		
			fmt.Println("Starting next episode: "+fmt.Sprint(anime.Ep.Number))
			anime.Ep.Started = false		

	}
}