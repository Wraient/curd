package anipm

// SearchItem is stored in SelectionOption.ExtraData for mapping hints.
type SearchItem struct {
	ID       int
	MalID    int
	RouteID  string
	Title    string
	Episodes int
	SubCount int
	DubCount int
}

type searchResponse struct {
	Items []searchItem `json:"items"`
}

type searchItem struct {
	ID           int    `json:"id"`
	RouteID      string `json:"routeId"`
	Slug         string `json:"slug"`
	Title        string `json:"title"`
	Native       string `json:"native"`
	Poster       string `json:"poster"`
	Year         int    `json:"year"`
	Type         string `json:"type"`
	EpisodeCount int    `json:"episodeCount"`
	SubCount     int    `json:"subCount"`
	DubCount     int    `json:"dubCount"`
	MalID        int    `json:"malId"`
}

type seriesResponse struct {
	ID       int             `json:"id"`
	Title    string          `json:"title"`
	Episodes []seriesEpisode `json:"episodes"`
}

type seriesEpisode struct {
	Number int  `json:"number"`
	Sub    bool `json:"sub"`
	Dub    bool `json:"dub"`
}

type bootstrapResponse struct {
	Episode           int    `json:"episode"`
	EffectiveLanguage string `json:"effectiveLanguage"`
	SettlarSelection  string `json:"settlarSelection"`
	Availability      struct {
		Sub bool `json:"sub"`
		Dub bool `json:"dub"`
	} `json:"availability"`
	Packages struct {
		Episodes map[string]packageEpisode `json:"episodes"`
	} `json:"anipmPackages"`
	Error string `json:"error"`
}

type packageEpisode struct {
	Sub     bool `json:"sub"`
	Dub     bool `json:"dub"`
	SubHard bool `json:"subhard"`
}

type sessionResponse struct {
	EmbedURL string `json:"embedUrl"`
	Error    string `json:"error"`
}

type embedSessionResponse struct {
	Kind      string          `json:"kind"`
	Source    string          `json:"source"`
	AudioLang string          `json:"audioLang"`
	Subtitles []embedSubtitle `json:"subtitles"`
	Error     string          `json:"error"`
}

type embedSubtitle struct {
	URL        string `json:"url"`
	AssURL     string `json:"assUrl"`
	Label      string `json:"label"`
	SrcLang    string `json:"srclang"`
	Default    bool   `json:"default"`
	AutoSelect bool   `json:"autoSelect"`
}
