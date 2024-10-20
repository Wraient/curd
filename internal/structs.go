package internal

type AnimeTitle struct {
	Romaji		string
	English		string
	Japanese	string
}

type Anime struct {
	Title			AnimeTitle 
	Ep				Episode
	TotalEpisodes	int
	MalId			int
	AnilistId		int
	AllanimeId		string
}

type SkipTimes struct {
	Op	int
	Ed	int
}

type Episode struct {
	Tit 		AnimeTitle
	Number	    int
	Skip_times	SkipTimes 
	Player		playingVideo
	Links 		[]string
	Is_filler 	bool
	Is_recap 	bool
	Aired 		string
	Synopsis 	string
}

type playingVideo struct {
	Url string
	Speed float64
	PlaybackTime int
}

type User struct {
	Token 		string
	Username 	string
	Id 			int
	AnimeList 	AnimeList
}

// AniListAnime is the struct for the API response
type AniListAnime struct {
	ID    int `json:"id"`
	Title struct {
		Romaji  string `json:"romaji"`
		English string `json:"english"`
		Native  string `json:"native"`
	} `json:"title"`
}

// Page represents the page in AniList response
type Page struct {
	Media []AniListAnime `json:"media"`
}

// ResponseData represents the full response structure
type ResponseData struct {
	Page Page `json:"Page"`
}

type Media struct {
	Duration int    `json:"duration"`
	Episodes int    `json:"episodes"`
	ID       int    `json:"id"`
	Title    AnimeTitle  `json:"title"`
}

type Entry struct {
	Media    Media `json:"media"`
	Progress int   `json:"progress"`
	Score    float64 `json:"score"`
	Status   string `json:"status"`
}

type AnimeList struct {
	Watching	[]Entry `json:"watching"`
	Completed	[]Entry `json:"completed"`
	Paused		[]Entry `json:"paused"`
	Dropped 	[]Entry `json:"dropped"`
	Planning 	[]Entry `json:"planning"`
}