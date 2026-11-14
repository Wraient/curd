package anikoto

import (
	"fmt"

	"github.com/wraient/curd/internal/curdhost"
	"github.com/wraient/curd/internal/providers"
	"github.com/wraient/curd/internal/providers/hlsverify"
)

// prioritizePlayableStreams moves streams that pass HLS validation ahead of
// ones that do not. Anikoto rotates MegaPlay CDN hosts per request and some of
// them prepend an undecodable ad image as the first segment, which makes mpv
// fail immediately. Validated streams are preferred; unvalidated streams are
// kept as a fallback so a transient validation failure never loses a link.
func prioritizePlayableStreams(links []string, hints map[string]providers.StreamPlaybackHint) []string {
	if len(links) < 2 {
		return links
	}

	validated := make([]string, 0, len(links))
	unvalidated := make([]string, 0, len(links))
	for _, link := range links {
		hint := hints[link]
		err := hlsverify.Validate(link, hlsverify.Options{
			Referer:   hint.Referrer,
			UserAgent: userAgent,
		})
		if err != nil {
			if curdhost.Log != nil {
				curdhost.Log(fmt.Sprintf("Anikoto deprioritized stream: %v", err))
			}
			unvalidated = append(unvalidated, link)
			continue
		}
		validated = append(validated, link)
	}
	return append(validated, unvalidated...)
}
