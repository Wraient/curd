package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/wraient/curd/internal"
)

var version string // Will be set by ldflags during build

func main() {
	discordClientId := "1287457464148820089"

	var anime internal.Anime
	var user internal.User

	internal.SetGlobalAnime(&anime)

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
	internal.SetGlobalConfig(&userCurdConfig)

	// Setup
	internal.ClearScreen()
	defer internal.RestoreScreen()

	logFile := filepath.Join(os.ExpandEnv(userCurdConfig.StoragePath), "debug.log")
	internal.SetGlobalLogFile(logFile)
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
	rofiSelection := flag.Bool("rofi", false, "Open selection in rofi")
	noRofi := flag.Bool("no-rofi", false, "No rofi")
	imagePreview := flag.Bool("image-preview", false, "Show image preview")
	noImagePreview := flag.Bool("no-image-preview", false, "No image preview")
	changeToken := flag.Bool("change-token", false, "Change token")
	currentCategory := flag.Bool("current", false, "Current category")
	updateScript := flag.Bool("u", false, "Update the script")
	editConfig := flag.Bool("e", false, "Edit config")
	subFlag := flag.Bool("sub", false, "Watch sub version")
	dubFlag := flag.Bool("dub", false, "Watch dub version")
	versionFlag := flag.Bool("v", false, "Print version information")

	// Custom help/usage function
	flag.Usage = func() {
		internal.RestoreScreen()
		fmt.Fprintf(os.Stderr, "Curd is a CLI tool to manage anime playback with advanced features like skipping intro, outro, filler, recap, tracking progress, and integrating with Discord.\n")
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		flag.PrintDefaults() // This prints the default flag information
	}

	flag.Parse()

	// Check version before screen clearing
	if *versionFlag {
		internal.RestoreScreen()
		if version == "" {
			version = "1.1.2"
		}
		fmt.Printf("Curd version: %s\n", version)
		os.Exit(0)
	}

	anime.Ep.ContinueLast = *continueLast

	if *updateScript {
		repo := "wraient/curd"
		fileName := "curd"

		if err := internal.UpdateCurd(repo, fileName); err != nil {
			internal.CurdOut(fmt.Sprintf("Error updating executable: %v\n", err))
			internal.ExitCurd(err)
		} else {
			internal.CurdOut("Program Updated!")
			internal.ExitCurd(nil)
		}
	}

	if *changeToken {
		internal.ChangeToken(&userCurdConfig, &user)
		return
	}

	if *currentCategory {
		userCurdConfig.CurrentCategory = true
	}

	if *rofiSelection {
		userCurdConfig.RofiSelection = true
	}

	if *noRofi || runtime.GOOS == "windows" {
		userCurdConfig.RofiSelection = false
	}

	if *imagePreview {
		userCurdConfig.ImagePreview = true
	}

	if *noImagePreview || runtime.GOOS == "windows" {
		userCurdConfig.ImagePreview = false
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
		internal.Log("Error reading token")
	}
	if user.Token == "" {
		internal.ChangeToken(&userCurdConfig, &user)
	}

	if userCurdConfig.RofiSelection {
		// Define a slice of file names to check and download
		filesToCheck := []string{
			"selectanimepreview.rasi",
			"selectanime.rasi",
			"userinput.rasi",
		}

		// Call the function to check and download files
		err := internal.CheckAndDownloadFiles(os.ExpandEnv(userCurdConfig.StoragePath), filesToCheck)
		if err != nil {
			internal.Log(fmt.Sprintf("Error checking and downloading files: %v\n", err))
			internal.CurdOut(fmt.Sprintf("Error checking and downloading files: %v\n", err))
			internal.ExitCurd(err)
		}
	}

	// Load animes in database
	databaseFile := filepath.Join(os.ExpandEnv(userCurdConfig.StoragePath), "curd_history.txt")
	databaseAnimes := internal.LocalGetAllAnime(databaseFile)

	if *addNewAnime {
		internal.AddNewAnime(&userCurdConfig, &anime, &user, &databaseAnimes)
		// internal.ExitCurd(fmt.Errorf("Added new anime!"))
	}

	internal.SetupCurd(&userCurdConfig, &anime, &user, &databaseAnimes)

	temp_anime, err := internal.FindAnimeByAnilistID(user.AnimeList, strconv.Itoa(anime.AnilistId))
	if err != nil {
		internal.Log("Error finding anime by Anilist ID: " + err.Error())
	}

	if anime.TotalEpisodes == temp_anime.Progress {
		internal.Log(temp_anime.Progress)
		internal.Log(anime.TotalEpisodes)
		internal.Log(user.AnimeList)
		internal.Log("Rewatching anime: " + internal.GetAnimeName(anime))
		anime.Rewatching = true
	}

	anime.Ep.Player.Speed = 1.0

	// Get filler list concurrently
	go func() {
		// Get MAL ID first if not already set
		if anime.MalId == 0 {
			malID, err := internal.GetAnimeMalID(anime.AnilistId)
			if err != nil {
				internal.Log("Error getting MAL ID: " + err.Error())
				return
			}
			anime.MalId = malID
		}

		fillerList, err := internal.FetchFillerEpisodes(anime.MalId)
		if err != nil {
			internal.Log("Error getting filler list: " + err.Error())
		} else {
			anime.FillerEpisodes = fillerList
			internal.Log("Filler list fetched successfully")
			// fmt.Println("Filler episodes: ", anime.FillerEpisodes)
		}
	}()

	// Main loop (loop to keep starting new episodes)
	for {

		internal.Log(anime)

		// Create a channel to signal when to exit the skip loop
		var wg sync.WaitGroup
		skipLoopDone := make(chan struct{})
		skipLoopClosed := make(chan bool, 1) // Channel to track if skipLoopDone has been closed
		skipLoopClosed <- false              // Initialize to false (not closed yet)

		// Get MalId and CoverImage (only if discord presence is enabled)
		if userCurdConfig.DiscordPresence {
			discordAvailable := true
			err := internal.LoginClient(discordClientId)
			if err != nil {
				internal.Log("Discord connection failed, disabling presence: " + err.Error())
				discordAvailable = false
				userCurdConfig.DiscordPresence = false // Disable for this session
			}

			if discordAvailable {
				anime.MalId, anime.CoverImage, err = internal.GetAnimeIDAndImage(anime.AnilistId)
				if err != nil {
					internal.Log("Error getting anime ID and image: " + err.Error())
				}
				err = internal.DiscordPresence(anime, false)
				if err != nil {
					internal.Log("Discord presence error, disabling: " + err.Error())
					userCurdConfig.DiscordPresence = false
				}
			}
		} else if anime.MalId == 0 {
			anime.MalId, err = internal.GetAnimeMalID(anime.AnilistId)
			if err != nil {
				internal.Log("Error getting anime MAL ID: " + err.Error())
			}
		}

		// Start curd (loop while episode is playing)
		for {
			// Check if current episode is filler/recap
			err = internal.GetEpisodeData(anime.MalId, anime.Ep.Number, &anime)
			if err != nil {
				internal.Log("Error getting episode data, assuming non-filler: " + err.Error())
				break // Break the loop and continue with playback
			}

			// Check if episode is filler
			anime.Ep.IsFiller = internal.IsEpisodeFiller(anime.FillerEpisodes, anime.Ep.Number)

			// If not filler/recap (or skip is disabled), break and continue with playback
			if !((anime.Ep.IsFiller && userCurdConfig.SkipFiller) || (anime.Ep.IsRecap && userCurdConfig.SkipRecap)) {
				if anime.Ep.LastWasSkipped {
					go internal.UpdateAnimeProgress(user.Token, anime.AnilistId, anime.Ep.Number-1)
				}
				break
			}

			// If it is filler/recap, log it and move to next episode
			if anime.Ep.IsFiller {
				internal.CurdOut(fmt.Sprint("Filler episode, skipping: ", anime.Ep.Number))
				// Get next canon episode
				anime.Ep.Number = internal.GetNextCanonEpisode(anime.FillerEpisodes, anime.Ep.Number)
			} else {
				internal.CurdOut(fmt.Sprint("Recap episode, skipping: ", anime.Ep.Number))
				anime.Ep.Number++
			}

			anime.Ep.LastWasSkipped = true
			anime.Ep.Started = false
			internal.LocalUpdateAnime(databaseFile, anime.AnilistId, anime.AllanimeId, anime.Ep.Number, 0, 0, internal.GetAnimeName(anime))

			// Check if we've reached the end of the series
			if anime.Ep.Number > anime.TotalEpisodes {
				internal.CurdOut("Reached end of series")
				internal.ExitCurd(nil)
			}
		}

		// Now start playback for the non-filler episode
		anime.Ep.Player.SocketPath = internal.StartCurd(&userCurdConfig, &anime)
		internal.Log(fmt.Sprint("Playback starting time: ", anime.Ep.Player.PlaybackTime))
		internal.Log(anime.Ep.Player.SocketPath)

		wg.Add(1)
		// Get episode data
		go func() {
			defer wg.Done()
			err = internal.GetEpisodeData(anime.MalId, anime.Ep.Number, &anime)
			if err != nil {
				internal.Log("Error getting episode data: " + err.Error())
			} else {
				internal.Log(anime)

				// if filler episode or recap episode and skip is enabled
				if (anime.Ep.IsFiller && userCurdConfig.SkipFiller) || (anime.Ep.IsRecap && userCurdConfig.SkipRecap) {
					if anime.Ep.IsFiller && userCurdConfig.SkipFiller {
						internal.CurdOut(fmt.Sprint("Filler Episode, starting next episode: ", anime.Ep.Number+1))
						internal.Log("Filler episode detected")
					} else if anime.Ep.IsRecap && userCurdConfig.SkipRecap {
						internal.CurdOut(fmt.Sprint("Recap Episode, starting next episode: ", anime.Ep.Number+1))
						internal.Log("Recap episode detected")
					}

					anime.Ep.IsCompleted = true
					if !userCurdConfig.NextEpisodePrompt || userCurdConfig.RofiSelection {
						// fmt.Println("[DEBUG] Starting next episode from filler/recap skip")
						internal.StartNextEpisode(&anime, &userCurdConfig, databaseFile)
					}
					// Send command to close MPV
					_, err := internal.MPVSendCommand(anime.Ep.Player.SocketPath, []interface{}{"quit"})
					if err != nil {
						internal.Log("Error closing MPV: " + err.Error())
					}
					// Exit the skip loop - only close if not already closed
					select {
					case isClosed := <-skipLoopClosed:
						if !isClosed {
							close(skipLoopDone)
							skipLoopClosed <- true // Mark as closed
						}
					default:
						// Channel is busy, another goroutine is handling closure
					}
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
						isPaused, err := internal.MPVSendCommand(anime.Ep.Player.SocketPath, []interface{}{"get_property", "pause"})
						if err != nil {
							internal.Log("Error getting pause status: " + err.Error())
						}
						if isPaused == nil {
							isPaused = true
						} else {
							isPaused = isPaused.(bool)
						}
						err = internal.DiscordPresence(anime, isPaused.(bool))
						if err != nil {
							// internal.Log("Error setting Discord presence: "+err.Error())
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
				internal.Log("Error getting and parsing AniSkip data: " + err.Error())
			}
			internal.Log(anime.Ep.SkipTimes)
		}()

		// Get video duration
		go func() {
			for {
				if anime.Ep.Started {
					if anime.Ep.Duration == 0 {
						// Get video duration
						durationPos, err := internal.MPVSendCommand(anime.Ep.Player.SocketPath, []interface{}{"get_property", "duration"})
						if err != nil {
							internal.Log("Error getting video duration: " + err.Error())
						} else if durationPos != nil {
							if duration, ok := durationPos.(float64); ok {
								anime.Ep.Duration = int(duration + 0.5) // Round to nearest integer
								internal.Log(fmt.Sprintf("Video duration: %d seconds", anime.Ep.Duration))
							} else {
								internal.Log("Error: duration is not a float64")
							}
						}
						break
					}
				}
				time.Sleep(1 * time.Second)
			}
		}()

		// Thread to prompt for next episode in CLI Mode
		go func() {
			if !userCurdConfig.RofiSelection && userCurdConfig.NextEpisodePrompt {
				for {
					select {
					case <-skipLoopDone:
						return
					default:
						internal.NextEpisodePrompt(&userCurdConfig)
						// Exit the skip loop - only close if not already closed
						select {
						case isClosed := <-skipLoopClosed:
							if !isClosed {
								close(skipLoopDone)
								skipLoopClosed <- true // Mark as closed
							}
						default:
							// Channel is busy, another goroutine is handling closure
						}
						return
					}
				}
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
					// internal.Log("Getting playback time "+anime.Ep.Player.SocketPath)
					timePos, err := internal.MPVSendCommand(anime.Ep.Player.SocketPath, []interface{}{"get_property", "time-pos"})
					if err != nil {
						internal.Log("Error getting playback time: " + err.Error())

						// Check if the error is due to invalid JSON
						// User closed the video
						if anime.Ep.Started {
							percentageWatched := internal.PercentageWatched(anime.Ep.Player.PlaybackTime, anime.Ep.Duration)
							// Episode is completed
							internal.Log(fmt.Sprint(percentageWatched))
							internal.Log(fmt.Sprint(anime.Ep.Player.Speed))
							internal.Log(fmt.Sprint(anime.Ep.Player.PlaybackTime))
							internal.Log(fmt.Sprint(anime.Ep.Duration))
							internal.Log(fmt.Sprint(userCurdConfig.PercentageToMarkComplete))
							if int(percentageWatched) >= userCurdConfig.PercentageToMarkComplete {
								anime.Ep.IsCompleted = true
								if !userCurdConfig.NextEpisodePrompt {
									internal.StartNextEpisode(&anime, &userCurdConfig, databaseFile)
								} else {
									// When NextEpisodePrompt is enabled, show the prompt in both modes
									if userCurdConfig.RofiSelection {
										// For Rofi mode
										internal.CurdOut("Showing next episode prompt in Rofi mode (playback ended)")
										internal.ExitMPV(anime.Ep.Player.SocketPath)
										internal.NextEpisodePrompt(&userCurdConfig)
										return
									} else {
										// For CLI mode
										internal.NextEpisodePrompt(&userCurdConfig)
										return
									}
								}
							} else {
								internal.Log("Episode is not completed, exiting")
								internal.ExitCurd(nil)
							}
							// Exit the skip loop - only close if not already closed
							select {
							case isClosed := <-skipLoopClosed:
								if !isClosed {
									close(skipLoopDone)
									skipLoopClosed <- true // Mark as closed
								}
							default:
								// Channel is busy, another goroutine is handling closure
							}
							return
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
									internal.Log("Error setting playback speed: " + err.Error())
								}
							}

							// Apply OP/ED Chapters
							err = internal.SendSkipTimesToMPV(&anime)
							if err != nil {
								internal.Log("Error sending skip times to MPV: " + err.Error())
							}
						}

						// If resume is true, seek to the playback time
						if anime.Ep.Resume {
							internal.SeekMPV(anime.Ep.Player.SocketPath, anime.Ep.Player.PlaybackTime)
							anime.Ep.Resume = false
						}

						animePosition, ok := timePos.(float64)
						if !ok {
							internal.Log("Error: timePos is not a float64")
							continue
						}

						anime.Ep.Player.PlaybackTime = int(animePosition + 0.5) // Round to nearest integer
						// Update Local Database
						err = internal.LocalUpdateAnime(databaseFile, anime.AnilistId, anime.AllanimeId, anime.Ep.Number, anime.Ep.Player.PlaybackTime, internal.ConvertSecondsToMinutes(anime.Ep.Duration), internal.GetAnimeName(anime))
						if err != nil {
							internal.Log("Error updating local database: " + err.Error())
						}
					}

					// Check if anything is playing, if nothing is playing and episode was started, handle completion
					hasPlayback, err := internal.HasActivePlayback(anime.Ep.Player.SocketPath)
					if err != nil {
						internal.Log("Error checking playback status: " + err.Error())
					} else if !hasPlayback && anime.Ep.Started {
						// Wait for a moment to allow playback to start
						time.Sleep(2 * time.Second) // Wait for 2 seconds

						// Check playback status again
						hasPlayback, err = internal.HasActivePlayback(anime.Ep.Player.SocketPath)
						if err != nil {
							internal.Log("Error checking playback status: " + err.Error())
						} else if !hasPlayback {
							// Nothing is playing, check percentage watched
							percentageWatched := internal.PercentageWatched(anime.Ep.Player.PlaybackTime, anime.Ep.Duration)
							// fmt.Printf("[DEBUG] Playback stopped - Percentage watched: %d%%, Required: %d%%\n",
							// 	int(percentageWatched),
							// 	userCurdConfig.PercentageToMarkComplete)

							if int(percentageWatched) >= userCurdConfig.PercentageToMarkComplete {
								anime.Ep.IsCompleted = true
								if !userCurdConfig.NextEpisodePrompt {
									internal.StartNextEpisode(&anime, &userCurdConfig, databaseFile)
								} else {
									// When NextEpisodePrompt is enabled, show the prompt in both modes
									if userCurdConfig.RofiSelection {
										// For Rofi mode
										internal.CurdOut("Showing next episode prompt in Rofi mode (playback ended)")
										internal.ExitMPV(anime.Ep.Player.SocketPath)
										internal.NextEpisodePrompt(&userCurdConfig)
										return
									} else {
										// For CLI mode
										internal.NextEpisodePrompt(&userCurdConfig)
										return
									}
								}
							} else {
								internal.Log("Episode is not completed, exiting")
								internal.ExitCurd(nil)
							}
							// Exit the skip loop - only close if not already closed
							select {
							case isClosed := <-skipLoopClosed:
								if !isClosed {
									close(skipLoopDone)
									skipLoopClosed <- true // Mark as closed
								}
							default:
								// Channel is busy, another goroutine is handling closure
							}
							return
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
				_, err := internal.MPVSendCommand(anime.Ep.Player.SocketPath, []interface{}{"get_property", "time-pos"})
				if err == nil && anime.Ep.Started {
					anime.Ep.Player.Speed, err = internal.GetMPVPlaybackSpeed(anime.Ep.Player.SocketPath)
					if err != nil {
						internal.Log("Failed to get mpv speed " + err.Error())
					}
				}
			}

			time.Sleep(1 * time.Second) // Wait before checking again
		}

		// Wait for all goroutines to finish before starting the next iteration
		wg.Wait()

		// Reset the WaitGroup for the next loop
		wg = sync.WaitGroup{}

		// Exit the program if we're starting an episode beyond the total episodes
		if anime.Ep.Number > anime.TotalEpisodes && anime.TotalEpisodes > 0 {
			internal.CurdOut("Reached end of series")
			internal.ExitCurd(nil)
		}

		if anime.Ep.IsCompleted && !anime.Rewatching {
			// Update progress for both regular episodes and skipped fillers
			if anime.TotalEpisodes > 0 && anime.Ep.Number-1 != anime.TotalEpisodes {
				go func() {
					// Update progress for regular episodes
					err = internal.UpdateAnimeProgress(user.Token, anime.AnilistId, anime.Ep.Number-1)
					if err != nil {
						internal.Log("Error updating Anilist progress: " + err.Error())
					}
				}()
			} else {
				// Update progress for last episode

				// Exit MPV
				internal.ExitMPV(anime.Ep.Player.SocketPath)

				err = internal.UpdateAnimeProgress(user.Token, anime.AnilistId, anime.Ep.Number-1)
				if err != nil {
					internal.Log("Error updating Anilist progress: " + err.Error())
				}
			}

			anime.Ep.IsCompleted = false
			// Only mark as complete and prompt for rating if we've reached the total episodes
			// AND the anime is not currently airing (total episodes > 0)
			if anime.Ep.Number-1 == anime.TotalEpisodes && userCurdConfig.ScoreOnCompletion && anime.TotalEpisodes > 0 {

				// Get updated anime data to check if it's still airing
				updatedAnime, err := internal.GetAnimeDataByID(anime.AnilistId, user.Token)
				if err != nil {
					internal.Log("Error getting updated anime data: " + err.Error())
				} else if !updatedAnime.IsAiring {
					anime.Ep.Number = anime.Ep.Number - 1
					internal.CurdOut("Completed anime.")
					err = internal.RateAnime(user.Token, anime.AnilistId)
					if err != nil {
						internal.Log("Error rating anime: " + err.Error())
						internal.CurdOut("Error rating anime: " + err.Error())
					}
					internal.LocalDeleteAnime(databaseFile, anime.AnilistId, anime.AllanimeId)
					internal.ExitCurd(nil)
				}
			}
		}
		if anime.Rewatching && anime.Ep.IsCompleted && anime.Ep.Number-1 == anime.TotalEpisodes {
			anime.Ep.Number = anime.Ep.Number - 1
			internal.CurdOut("Completed anime. (Rewatching so no scoring)")
			internal.LocalDeleteAnime(databaseFile, anime.AnilistId, anime.AllanimeId)
			internal.ExitCurd(nil)
		}

		// Handle next episode logic based on config
		if anime.Ep.IsCompleted {
			if userCurdConfig.NextEpisodePrompt {
				// Handle the special case where playback is still active (might happen with Rofi)
				// This ensures we don't show duplicate prompts if it was already handled in the playback monitor
				isPlaying, err := internal.HasActivePlayback(anime.Ep.Player.SocketPath)
				if err != nil {
					internal.Log(fmt.Sprintf("Error checking playback for prompt: %v", err))
				}

				// Only show the prompt here if playback is still active - otherwise it was already handled
				if isPlaying {
					internal.CurdOut("Episode completed, showing next episode prompt from main loop")
					if userCurdConfig.RofiSelection {
						internal.NextEpisodePrompt(&userCurdConfig)
						// Exit after NextEpisodePrompt to avoid starting a new playback loop
						internal.ExitCurd(nil)
					}
					// For CLI mode, the prompt will be shown elsewhere
				} else {
					internal.Log("Playback already ended, prompt handled elsewhere")
				}
			} else {
				// When NextEpisodePrompt is off, we continue automatically in both CLI and Rofi modes
				internal.CurdOut("Auto-continuing to next episode")
				// Don't need to do anything - the loop will continue naturally
			}
		}

		// Wait for up to 5 seconds for prefetched links to become available
		for i := 0; i < 5; i++ {
			if anime.Ep.NextEpisode.Number == anime.Ep.Number && len(anime.Ep.NextEpisode.Links) > 0 {
				internal.Log("Using prefetched next episode link")
				anime.Ep.Links = anime.Ep.NextEpisode.Links
				break
			}
			time.Sleep(1 * time.Second)
		}

		// If we still don't have links, get them now
		if len(anime.Ep.Links) == 0 {
			links, err := internal.GetEpisodeURL(userCurdConfig, anime.AllanimeId, anime.Ep.Number)
			if err != nil {
				internal.Log("Failed to get episode links: " + err.Error())
				internal.CurdOut("Failed to get episode links. Try again later.")
				internal.ExitCurd(fmt.Errorf("failed to get episode links: %v", err))
				return
			}
			anime.Ep.Links = links
		}

		// Verify that we have links before starting
		if len(anime.Ep.Links) == 0 {
			internal.CurdOut("No episode links found. Try again later.")
			internal.ExitCurd(fmt.Errorf("no episode links found"))
			return
		}

	}
}
