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

	var anime Anime
	var user User

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
	user.token, err = internal.GetTokenFromFile("/home/wraient/.local/share/curd/token")
	if err != nil {
		internal.Log("Error reading token", logFile)
	}

	// Get user id, username and Anime list
	user.id, user.username, err = internal.GetAnilistUserID(user.token)
	anilistUserData, err := internal.GetUserData(user.token, user.id)
	user.animeList = internal.ParseAnimeList(anilistUserData)
	animeListMap := internal.GetAnimeMap(user.animeList)

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
	selectedAnilistAnime, err := internal.FindAnimeByAnilistID(user.animeList, anilistSelectedOption.Key)
	if err != nil {
		fmt.Println("Can not find the anime in anilist animelist")
	}

	anime.Title = AnimeTitle(selectedAnilistAnime.Media.Title)
	anime.TotalEpisodes = selectedAnilistAnime.Media.Episodes
	anime.Ep.Number = selectedAnilistAnime.Progress

	// Get allanime id of anime
	anime.AllanimeId, err = internal.FindKeyByValue(animeList, fmt.Sprintf("%v (%d episodes)", userQuery, selectedAnilistAnime.Media.Episodes))
	// If unable to get id automatically get manually
	if err != nil {
		fmt.Println("Failed to automatically select anime")
		selectedAllanimeAnime, err := internal.DynamicSelect(animeList)
		if err != nil {
			fmt.Println("No anime available")
			os.Exit(0)
		}

		anime.AllanimeId = selectedAllanimeAnime.Key
	}

	// internal.Log(userQuery, logFile)

	// os.Exit(0)
	link, err := internal.GetEpisodeURL(userCurdConfig, anime.AllanimeId, anime.Ep.Number)
	if err != nil {
		episodeList, err := internal.EpisodesList(anime.AllanimeId, userCurdConfig.SubOrDub)
		
		if err != nil{
			fmt.Println("No episode list found")
			internal.RestoreScreen()
			os.Exit(1)
		}
		
		if episodeList == nil { // if user selected "Add new anime option" 
			fmt.Println("Add new anime") // TODO: add new anime implementaion
			internal.RestoreScreen()
			os.Exit(0)
		}
	
		internal.Log(episodeList, logFile)
		fmt.Printf("Enter the episode (%v episodes)\n", episodeList[len(episodeList)-1])
		fmt.Scanln(&anime.Ep.Number)
		link, err := internal.GetEpisodeURL(userCurdConfig, anime.AllanimeId, anime.Ep.Number)
		anime.Ep.Links = link
	}

	anime.Ep.Links = link
	
	if err != nil {
		fmt.Println("Failed to get episode link")
		os.Exit(1)
	}
	
	internal.Log(anime, logFile)

	mpvSocketPath, err := internal.StartMpv(anime.Ep.Links[len(anime.Ep.Links)-1])

	if err != nil {
		internal.Log("Failed to start mpv", logFile)
		os.Exit(1)
	}

	for { // Loop while video is playing
		speed, err :=internal.GetMpvProperty(mpvSocketPath, "speed")

		if err != nil {
			// fmt.Println("failed to get mpv speed")
			// internal.Log(mpvSocketPath, logFile)
			// internal.Log(speed, logFile)
		} else {
			fmt.Println(speed)
		}
		time.Sleep(1)
	}
}