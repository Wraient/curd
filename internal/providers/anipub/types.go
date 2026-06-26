package anipub

// SearchItem is stored in SelectionOption.ExtraData for mapping hints.
type SearchItem struct {
	ID       int
	MalID    int
	Name     string
	Finder   string
	Episodes int
}

type searchResult struct {
	Name   string `json:"Name"`
	ID     int    `json:"Id"`
	Image  string `json:"Image"`
	Finder string `json:"finder"`
}

type infoResponse struct {
	ID      int    `json:"_id"`
	Name    string `json:"Name"`
	MALID   string `json:"MALID"`
	EpCount int    `json:"epCount"`
	Image   string `json:"ImagePath"`
	Cover   string `json:"Cover"`
}

type detailsResponse struct {
	Local localDetails `json:"local"`
}

type localDetails struct {
	Link string        `json:"link"`
	Ep   []episodeLink `json:"ep"`
}

type episodeLink struct {
	Link string `json:"link"`
}

type megaplaySourcesResponse struct {
	Sources struct {
		File string `json:"file"`
	} `json:"sources"`
	Tracks []struct {
		File    string `json:"file"`
		Label   string `json:"label"`
		Kind    string `json:"kind"`
		Default bool   `json:"default"`
	} `json:"tracks"`
}
