package senshi

// SearchItem is stored in SelectionOption.ExtraData for mapping hints.
type SearchItem struct {
	MalID     int
	PublicID  string
	Title     string
	Type      string
	Episodes  int
	Year      int
	Score     float64
	Status    string
}

type filterResponse struct {
	Data  []animeItem `json:"data"`
	Total int         `json:"total"`
}

type animeItem struct {
	ID            int     `json:"id"`
	PublicID      string  `json:"public_id"`
	AnimePicture  string  `json:"anime_picture"`
	Title         string  `json:"title"`
	TitleEnglish  string  `json:"title_english"`
	Type          string  `json:"type"`
	AniEpisodes   string  `json:"ani_episodes"`
	AniStatus     string  `json:"ani_status"`
	AniYear       int     `json:"ani_year"`
	Score         float64 `json:"score"`
}

type episodeItem struct {
	ID       int    `json:"id"`
	EpID     int    `json:"ep_id"`
	MalID    int    `json:"mal_id"`
	EpTitle  string `json:"ep_title"`
	EpFiller bool   `json:"ep_filler"`
	EpRecap  bool   `json:"ep_recap"`
}

type embedItem struct {
	URL      string  `json:"url"`
	Server2  *string `json:"server2"`
	ServerFM *string `json:"serverFM"`
	Status   string  `json:"status"`
}
