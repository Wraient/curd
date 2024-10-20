package main

import (
	// "encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/wraient/curd/internal"
)

func main() {
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
	
	// os.Exit(0)
	
	// Get user id, username and Anime list
	user.Id, user.Username, err = internal.GetAnilistUserID(user.Token)
	anilistUserData, err := internal.GetUserData(user.Token, user.Id)
	user.AnimeList = internal.ParseAnimeList(anilistUserData)
	animeListMap := internal.GetAnimeMap(user.AnimeList)
	
	// Select anime to watch (Anilist)
	anilistSelectedOption, err := internal.DynamicSelect(animeListMap)
	userQuery := anilistSelectedOption.Label
	anime.AnilistId, err = strconv.Atoi(anilistSelectedOption.Key)
	if err != nil {
		fmt.Println("Error converting Anilist ID:", err)
		return
	}
	
	// Get Anime list (All anime)
	animeList, err := internal.SearchAnime(string(userQuery), userCurdConfig.SubOrDub)
	if err != nil {
		fmt.Println("Failed to select anime", logFile)
		os.Exit(1)
	}
	if len(animeList) == 0 {
		fmt.Println("No results found.")
		os.Exit(0)
	}
	
	// Get anime entry
	selectedAnilistAnime, err := internal.FindAnimeByAnilistID(user.AnimeList, anilistSelectedOption.Key)
	if err != nil {
		fmt.Println("Can not find the anime in anilist animelist")
	}
	
	anime.Title = selectedAnilistAnime.Media.Title
	anime.TotalEpisodes = selectedAnilistAnime.Media.Episodes
	anime.Ep.Number = selectedAnilistAnime.Progress+1
	
	// Find anime in Local history
	animePointer := internal.LocalFindAnime(databaseAnimes, anime.AnilistId, "")	

	// if anime not found in database, find it in animeList
	if animePointer == nil {
		// find anime in animeList
		anime.AllanimeId, err = internal.FindKeyByValue(animeList, fmt.Sprintf("%v (%d episodes)", userQuery, selectedAnilistAnime.Media.Episodes))
	} else {
		// if anime found in database, use it
		anime.AllanimeId = animePointer.AllanimeId
	}
	// If unable to get Allanime id automatically get manually
	if anime.AllanimeId == "" {
		fmt.Println("Failed to automatically select anime")
		selectedAllanimeAnime, err := internal.DynamicSelect(animeList)
		if err != nil {
			fmt.Println("No anime available")
			os.Exit(0)
		}
		
		anime.AllanimeId = selectedAllanimeAnime.Key
	}

	// Get episode link
	link, err := internal.GetEpisodeURL(userCurdConfig, anime.AllanimeId, anime.Ep.Number)
	if err != nil {
		// If unable to get episode link automatically get manually
		episodeList, err := internal.EpisodesList(anime.AllanimeId, userCurdConfig.SubOrDub)
		if err != nil {
			fmt.Println("No episode list found")
			internal.RestoreScreen()
			os.Exit(1)
		}
		fmt.Printf("Enter the episode (%v episodes)\n", episodeList[len(episodeList)-1])
		fmt.Scanln(&anime.Ep.Number)
		link, err = internal.GetEpisodeURL(userCurdConfig, anime.AllanimeId, anime.Ep.Number)
		if err != nil {
			fmt.Println("Failed to get episode link")
			os.Exit(1)
		}
		// anime.Ep.Links = link
	}
	anime.Ep.Links = link
	internal.Log(anime, logFile)
	mpvSocketPath, err := internal.StartMpv(anime.Ep.Links[len(anime.Ep.Links)-1])

	if err != nil {
		internal.Log("Failed to start mpv", logFile)
		os.Exit(1)
	}

	for { // Loop while video is playing
		speed, err := internal.GetMpvProperty(mpvSocketPath, "speed")
		anime.Ep.Player.Speed, err = strconv.ParseFloat(speed, 64)
		if err != nil {
			// fmt.Println("failed to get mpv speed")
			// internal.Log(mpvSocketPath, logFile)
			// internal.Log(speed, logFile)
		} else {
			// fmt.Println(speed)
		}
		time.Sleep(1)
	}
}
