package internal

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type AnimeFillerListEpisode struct {
	EpisodeID int  `json:"mal_id"`
	IsFiller  bool `json:"filler"`
}

type EpisodesResponse struct {
	Data       []AnimeFillerListEpisode `json:"data"`
	Pagination struct {
		HasNextPage bool `json:"has_next_page"`
	} `json:"pagination"`
}

func FetchFillerEpisodes(malID int) ([]int, error) {
	baseURL := fmt.Sprintf("https://api.jikan.moe/v4/anime/%d/episodes", malID)
	var fillerEpisodes []int
	page := 1
	rateLimiter := time.NewTicker(334 * time.Millisecond) // ~3 requests per second
	defer rateLimiter.Stop()

	for {
		<-rateLimiter.C // Wait for rate limit
		url := fmt.Sprintf("%s?page=%d", baseURL, page)
		resp, err := http.Get(url)
		if err != nil {
			return nil, fmt.Errorf("error fetching episodes: %v", err)
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			// Wait for 2 seconds and retry
			time.Sleep(2 * time.Second)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("received non-200 response: %d", resp.StatusCode)
		}

		var episodesResp EpisodesResponse
		if err := json.NewDecoder(resp.Body).Decode(&episodesResp); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("error decoding response: %v", err)
		}
		resp.Body.Close()

		for _, episode := range episodesResp.Data {
			if episode.IsFiller {
				fillerEpisodes = append(fillerEpisodes, episode.EpisodeID)
			}
		}

		if !episodesResp.Pagination.HasNextPage {
			break
		}
		page++
	}

	return fillerEpisodes, nil
}

// IsEpisodeFiller checks if a given episode number is in the filler episodes list
func IsEpisodeFiller(fillerEpisodes []int, episodeNumber int) bool {
	for _, fillerEp := range fillerEpisodes {
		if fillerEp == episodeNumber {
			return true
		}
	}
	return false
}

// GetNextCanonEpisode returns the next non-filler episode number after the current episode
func GetNextCanonEpisode(fillerEpisodes []int, currentEpisode int) int {
	nextEpisode := currentEpisode + 1

	// Keep incrementing episode number until we find a non-filler episode
	for IsEpisodeFiller(fillerEpisodes, nextEpisode) {
		nextEpisode++
	}

	return nextEpisode
}
