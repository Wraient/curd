package internal

import "strings"

// LinkPriorities defines the order of priority for link domains
var LinkPriorities = []string{
	"video.wixstatic.com",
	"sharepoint.com",
	"wixmp.com",
	"dropbox.com",
	"wetransfer.com",
	"gogoanime.com",
	// Add more domains in order of priority
}

// PrioritizeLink takes an array of links and returns a single link based on priority
func PrioritizeLink(links []string) string {
	if len(links) == 0 {
		return ""
	}

	bestLink := links[0]
	bestScore := linkSelectionScore(bestLink)
	for _, link := range links[1:] {
		score := linkSelectionScore(link)
		if score > bestScore {
			bestScore = score
			bestLink = link
		}
	}
	return bestLink
}

func linkSelectionScore(link string) int {
	score := 0
	for i, domain := range LinkPriorities {
		if strings.Contains(link, domain) {
			score += (len(LinkPriorities) - i) * 1000
			break
		}
	}
	score += wixmpQualityScore(link)
	return score
}
