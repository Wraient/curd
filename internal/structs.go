package internal

type AnimeTitle struct {
	Romaji   string `json:"title_romanji"`
	English  string `json:"title"`
	Japanese string `json:"title_japanese"`
}

type Anime struct {
	Title         AnimeTitle `json:"title"`
	Ep            Episode     `json:"ep"`
	CoverImage    string      `json:"url"` // Assuming this field corresponds to the cover image URL
	TotalEpisodes  int        `json:"total_episodes"` // If provided by the API
	MalId         int        `json:"mal_id"`
	AnilistId     int        `json:"anilist_id"` // Assuming you have an Anilist ID in your struct
	AllanimeId    string      // Can be populated as necessary
}

type Skip struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

type SkipTimes struct {
	Op Skip `json:"op"`
	Ed Skip `json:"ed"`
}

type Episode struct {
	Title     AnimeTitle `json:"title"`
	Number    int        `json:"number"`
	SkipTimes SkipTimes   `json:"skip_times"`
	Player    playingVideo `json:"player"`
	Resume    bool       `json:"resume"`
	Started   bool       `json:"started"`
	Duration  int        `json:"duration"`
	Links     []string   `json:"links"`
	IsFiller  bool       `json:"filler"`
	IsRecap   bool       `json:"recap"`
	Aired     string     `json:"aired"`
	Synopsis  string     `json:"synopsis"`
}

type playingVideo struct {
	Url          string
	Speed        float64 `json:"speed"`
	PlaybackTime int     `json:"playback_time"`
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