package animex

import "strings"

// SearchItem is stored in SelectionOption.ExtraData for mapping hints.
type SearchItem struct {
	ID        string
	AnilistID int
	MalID     int
	Title     string
	Romaji    string
	Type      string
	Episodes  int
	Year      int
}

type catalogItem struct {
	ID         string `json:"id"`
	AnilistID  int    `json:"anilistId"`
	MalID      int    `json:"malId"`
	Romaji     string `json:"titleRomaji"`
	English    string `json:"titleEnglish"`
	Format     string `json:"format"`
	EpisodeCnt int    `json:"episodeCount"`
	SeasonYear int    `json:"seasonYear"`
	// coverImage is a JSON scalar shaped like AniList's cover image
	// ({extraLarge, large, medium, color}).
	CoverImage map[string]string `json:"coverImage"`
}

// coverURL prefers the largest cover variant the API returned.
func (item catalogItem) coverURL() string {
	for _, key := range []string{"extraLarge", "large", "medium"} {
		if url := strings.TrimSpace(item.CoverImage[key]); url != "" {
			return url
		}
	}
	return ""
}

type catalogResponse struct {
	Data struct {
		Catalog struct {
			Items []catalogItem `json:"items"`
		} `json:"catalogAnime"`
	} `json:"data"`
}

type ppEpisode struct {
	Number   int  `json:"number"`
	HasSub   bool `json:"hasSub"`
	HasDub   bool `json:"hasDub"`
	IsFiller bool `json:"isFiller"`
}

type ppProvider struct {
	ID      string `json:"id"`
	Default bool   `json:"default"`
}

type ppServers struct {
	SubProviders []ppProvider `json:"subProviders"`
	DubProviders []ppProvider `json:"dubProviders"`
}

type ppSource struct {
	URL  string `json:"url"`
	Type string `json:"type"`
}

type ppSources struct {
	Sources []ppSource       `json:"sources"`
	Tracks  []map[string]any `json:"tracks"`
	Error   string           `json:"error"`
}
