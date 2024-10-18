package main

import (
	"fmt"
	"github.com/wraient/curd/internal"	
	"os"
	"math/rand"
)

type Title struct {
	Romanji_title string
	English_title string
	Japanese_title string
}

type Anime struct {
	Tit Title 
	Ep Episode
	Total_episodes int
	Mal_id int
	Allanime_id string
}

type SkipTimes struct {
	Op int
	Ed int
}

type Episode struct {
	Tit Title
	Number int
	Skip_times SkipTimes 
	links []string
	Is_filler bool
	Is_recap bool
	Aired string
	Synopsis string
}

func main(){
	internal.ClearScreen()
	defer internal.RestoreScreen()

	var userQuery string
	logFile := "debug.log"
	
	fmt.Println("Enter anime: ")
	fmt.Scanln(&userQuery) // Scan input until a newline
	animeList, err := internal.SearchAnime(string(userQuery), "sub")

	internal.Log(userQuery, logFile)

	if err != nil {
		fmt.Println("Failed to get anime list")
		os.Exit(1)
	}

	if len(animeList) == 0 {
        fmt.Println("No results found.")
        os.Exit(0)
    }

	selectedOption, err := internal.DynamicSelect(animeList)

	if err != nil {
		fmt.Println("No anime Selected")
		os.Exit(0)
	}

	internal.Log(animeList, logFile)
	internal.Log(selectedOption, logFile)

	episodeList, err := internal.EpisodesList(selectedOption.Key, "sub")

	if err != nil{
		fmt.Println("No episode list found")
		internal.RestoreScreen()
		os.Exit(1)
	}

	if episodeList == nil { // if user selected "Add new anime option"
		fmt.Println("Add new anime")
		internal.RestoreScreen()
		os.Exit(0)
	}

	internal.Log(episodeList, logFile)
	var episodeNumber string
	fmt.Printf("Enter the episode (%v episodes)", episodeList[len(episodeList)-1])
	fmt.Scanln(&episodeNumber)

	link, err := internal.GetEpisodeURL(internal.GetDefaultAllanimeConfig(), selectedOption.Key, episodeNumber)

	if err != nil {
		fmt.Println("Failed to get episode link")
		os.Exit(1)
	}

	internal.Log(link, logFile)

	// Generate a random number between 0 and 99
	randomNumber := rand.Intn(100) // Change 100 to the desired range

	err = internal.StartMpv(link[0], "/tmp/mpvsocket"+string(randomNumber))

	if err != nil {
		internal.Log("Failed to start mpv", logFile)
		os.Exit(1)
	}

	for { // Loop while video is playing
	}

}