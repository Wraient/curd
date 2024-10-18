package main

import (
	"fmt"
	"github.com/wraient/curd/internal"	
	"os"
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
	// fmt.Println("You entered:", input)
	animeList, err := internal.SearchAnime(string(userQuery), "sub")

	if err != nil {
		fmt.Println("Failed to get anime list")
		os.Exit(1)
	}

	if len(animeList) == 0 {
        fmt.Println("No results found.")
        os.Exit(0)
    }
	if len(animeList) == 1 {
		internal.Log("one result",  logFile)
		internal.Log(animeList, logFile)
    }

	selectedOption, err := internal.DynamicSelect(animeList)

	// Output the selected anime and its internal key
	if err != nil {
		fmt.Println("No anime Selected")
		os.Exit(0)
	}

	internal.Log(animeList, logFile)
	internal.Log(selectedOption, logFile)

	episodeList, err := internal.EpisodesList(selectedOption.Key, "sub")

	if err != nil{
		fmt.Println("No episode list found")
	}

	internal.Log(episodeList, logFile)
	var episodeNumber string
	fmt.Printf("Enter the episode (%v episodes)", episodeList[len(episodeList)-1])
	fmt.Scanln(&episodeNumber)

	link, err := internal.GetEpisodeURL(internal.GetDefaultAllanimeConfig(), selectedOption.Key, episodeNumber)

	if err != nil {
		fmt.Println("Failed to get episode link")
	}

	internal.Log(link, logFile)

}