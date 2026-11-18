package anilight

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/wraient/curd/internal/providers"
)

type detailResponse struct {
	Slug        string `json:"slug"`
	Episodes    int    `json:"episodes"`
	SubEpisodes int    `json:"subEpisodes"`
	DubEpisodes int    `json:"dubEpisodes"`
}

func episodeCountForMode(showID, mode string) (int, error) {
	showID = strings.TrimSpace(showID)
	if showID == "" {
		return 0, fmt.Errorf("empty show id")
	}
	var detail detailResponse
	if err := fetchJSON(fmt.Sprintf("%s/api/anime/%s", baseURL, url.PathEscape(showID)), referrer, &detail); err != nil {
		return 0, err
	}
	if wantDub := providers.NormalizeTranslationType(mode) == "dub"; wantDub {
		return detail.DubEpisodes, nil
	}
	return detail.SubEpisodes, nil
}

func episodesList(showID, mode string) ([]string, error) {
	count, err := episodeCountForMode(showID, mode)
	if err != nil {
		return nil, err
	}
	if count <= 0 {
		return nil, fmt.Errorf("no episodes found for show %s", showID)
	}
	result := make([]string, 0, count)
	for n := 1; n <= count; n++ {
		result = append(result, strconv.Itoa(n))
	}
	return result, nil
}
