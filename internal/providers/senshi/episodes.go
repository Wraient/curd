package senshi

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

func episodesList(showID, mode string) ([]string, error) {
	malID, err := parseMalID(showID)
	if err != nil {
		return nil, err
	}
	_ = mode

	var episodes []episodeItem
	if err := fetchJSON(http.MethodGet, fmt.Sprintf("%s/episodes/%d", baseURL, malID), nil, &episodes); err != nil {
		return nil, err
	}
	if len(episodes) == 0 {
		return nil, fmt.Errorf("no episodes found for mal id %d", malID)
	}

	seen := map[int]struct{}{}
	for _, ep := range episodes {
		epNo := ep.EpID
		if epNo <= 0 {
			epNo = ep.ID
		}
		if epNo > 0 {
			seen[epNo] = struct{}{}
		}
	}

	nums := make([]int, 0, len(seen))
	for epNo := range seen {
		nums = append(nums, epNo)
	}
	sort.Ints(nums)

	result := make([]string, 0, len(nums))
	for _, epNo := range nums {
		result = append(result, strconv.Itoa(epNo))
	}
	return result, nil
}

func parseMalID(showID string) (int, error) {
	showID = strings.TrimSpace(showID)
	if showID == "" {
		return 0, fmt.Errorf("empty show id")
	}
	malID, err := strconv.Atoi(showID)
	if err != nil || malID <= 0 {
		return 0, fmt.Errorf("invalid senshi mal id %q", showID)
	}
	return malID, nil
}
