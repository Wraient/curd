package anipub

import (
	"fmt"

	"github.com/wraient/curd/internal/curdhost"
	"github.com/wraient/curd/internal/providers"
	"github.com/wraient/curd/internal/providers/substyle"
)

func getEpisodeStreamsForMode(showID string, config providers.PlaybackConfig, epNo int) ([]string, map[string]providers.StreamPlaybackHint, error) {
	showID, err := parseShowID(showID)
	if err != nil {
		return nil, nil, err
	}
	if epNo <= 0 {
		return nil, nil, fmt.Errorf("invalid episode number %d", epNo)
	}

	var details detailsResponse
	if err := fetchJSON(detailsURL(showID), baseURL+"/", &details); err != nil {
		return nil, nil, err
	}

	videoLink, err := episodeVideoLink(details, epNo)
	if err != nil {
		return nil, nil, err
	}

	mode := providers.NormalizeTranslationType(config.SubOrDub)
	if mode == "sub" {
		if _, err := substyle.Choose(true, false, config.SubStyle); err != nil {
			return nil, nil, err
		}
	}
	streamURL, subtitle, err := resolveMegaplayStream(videoLink, mode)
	if err != nil {
		return nil, nil, err
	}

	// Reject ad-injected decoy playlists served by the megaplay CDN so the
	// caller can fall back to another provider instead of opening an idle mpv.
	if err := validateResolvedStream(streamURL); err != nil {
		curdhost.Log(fmt.Sprintf("anipub stream %q rejected: %v", streamURL, err))
		return nil, nil, err
	}

	hints := map[string]providers.StreamPlaybackHint{
		streamURL: {
			Referrer: megaplayBaseURL + "/",
			Subtitle: subtitle,
		},
	}
	return []string{streamURL}, hints, nil
}
