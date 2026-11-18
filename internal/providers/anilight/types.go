package anilight

// SearchItem is stored in SelectionOption.ExtraData for mapping hints.
type SearchItem struct {
	ID       int
	MalID    int
	Slug     string
	Title    string
	Romaji   string
	Native   string
	Type     string
	Episodes int
	SubCount int
	DubCount int
}

type searchTitle struct {
	Romaji  string `json:"romaji"`
	English string `json:"english"`
	Native  string `json:"native"`
}

type searchCover struct {
	Large string `json:"large"`
}

type searchItem struct {
	ID          int         `json:"id"`
	Slug        string      `json:"slug"`
	AnilistID   int         `json:"anilistId"`
	MalID       int         `json:"idMal"`
	Title       searchTitle `json:"title"`
	Cover       searchCover `json:"coverImage"`
	Format      string      `json:"format"`
	SeasonYear  int         `json:"seasonYear"`
	Episodes    int         `json:"episodes"`
	SubEpisodes int         `json:"subEpisodes"`
	DubEpisodes int         `json:"dubEpisodes"`
}

type verifyResponse struct {
	SessionToken string `json:"sessionToken"`
	ExpiresAt    int64  `json:"expiresAt"`
}

type sourceTrack struct {
	ID    string `json:"id"`
	URL   string `json:"url"`
	Kind  string `json:"kind"`
	Lang  string `json:"lang"`
	Label string `json:"label"`
}

type sourceEntry struct {
	URL     string `json:"url"`
	Quality string `json:"quality"`
	Type    string `json:"type"`
}

type sourcesResponse struct {
	Audio   *struct{}     `json:"audio"`
	Tracks  []sourceTrack `json:"tracks"`
	Sources []sourceEntry `json:"sources"`
	Error   string        `json:"error"`
}
