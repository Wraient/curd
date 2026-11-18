package aniwaves

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/wraient/curd/internal/providers"
)

var (
	// Each episode anchor carries its own data-num/data-sub/data-dub
	// attributes (both the released and the airing list layouts), so flags
	// are read from the same tag and can never shift between episodes.
	episodeItemRE = regexp.MustCompile(`<a\b[^>]*>`)
	episodeNumRE  = regexp.MustCompile(`data-num="(\d+)"`)
	episodeSubRE  = regexp.MustCompile(`data-sub="([01])"`)
	episodeDubRE  = regexp.MustCompile(`data-dub="([01])"`)
)

func parseEpisodeTag(tag string) (num int, sub, dub bool, ok bool) {
	if !strings.Contains(tag, "data-num=") {
		return 0, false, false, false
	}
	numMatch := episodeNumRE.FindStringSubmatch(tag)
	if numMatch == nil {
		return 0, false, false, false
	}
	n, err := strconv.Atoi(numMatch[1])
	if err != nil || n <= 0 {
		return 0, false, false, false
	}
	if subMatch := episodeSubRE.FindStringSubmatch(tag); subMatch != nil {
		sub = subMatch[1] == "1"
	}
	if dubMatch := episodeDubRE.FindStringSubmatch(tag); dubMatch != nil {
		dub = dubMatch[1] == "1"
	}
	return n, sub, dub, true
}

func episodesList(showID, mode string) ([]string, error) {
	showID = strings.TrimSpace(showID)
	if showID == "" {
		return nil, fmt.Errorf("empty show id")
	}
	numericID := showNumericID(showID)
	if numericID == "" {
		return nil, fmt.Errorf("invalid aniwaves show id %q", showID)
	}

	body, _, err := fetchBytes(baseURL+"/ajax/episode/list/"+numericID, baseURL+"/watch/"+showID, nil)
	if err != nil {
		return nil, err
	}
	var payload ajaxEnvelope
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("parse aniwaves episodes: %w", err)
	}
	if err := checkAjaxStatus(&payload, "episode list"); err != nil {
		return nil, err
	}
	var html string
	if err := json.Unmarshal(payload.Result, &html); err != nil {
		return nil, fmt.Errorf("parse aniwaves episode list: %w", err)
	}

	wantDub := providers.NormalizeTranslationType(mode) == "dub"
	tags := episodeItemRE.FindAllString(html, -1)
	result := make([]string, 0, len(tags))
	for _, tag := range tags {
		n, hasSub, hasDub, ok := parseEpisodeTag(tag)
		if !ok {
			continue
		}
		if wantDub && !hasDub {
			continue
		}
		if !wantDub && !hasSub {
			continue
		}
		result = append(result, strconv.Itoa(n))
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("no episodes found for show %s", showID)
	}
	return result, nil
}
