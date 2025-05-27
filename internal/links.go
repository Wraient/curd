package internal

import "strings"

// LinkPriorities defines the order of priority for link domains
var LinkPriorities = []string{
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

	// Create a map for quick lookup of priorities
	priorityMap := make(map[string]int)
	for i, p := range LinkPriorities {
		priorityMap[p] = len(LinkPriorities) - i // Higher index means higher priority
	}

	highestPriority := -1
	var bestLink string

	for _, link := range links {
		for domain, priority := range priorityMap {
			if strings.Contains(link, domain) {
				if priority > highestPriority {
					highestPriority = priority
					bestLink = link
				}
				break
			}
		}
	}

	// If no priority link found, return the first link
	if bestLink == "" {
		return links[0]
	}

	return bestLink
}
