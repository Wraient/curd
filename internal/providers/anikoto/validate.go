package anikoto

import (
	"fmt"
	"sync"

	"github.com/wraient/curd/internal/curdhost"
	"github.com/wraient/curd/internal/providers"
	"github.com/wraient/curd/internal/providers/hlsverify"
)

// maxConcurrentAnikotoValidations bounds simultaneous HLS validations. Each
// validation walks master -> media -> segment fetches; they are independent
// per link, so they run concurrently and are re-partitioned in input order.
const maxConcurrentAnikotoValidations = 4

// prioritizePlayableStreams moves streams that pass HLS validation ahead of
// ones that do not. Anikoto rotates MegaPlay CDN hosts per request and some of
// them prepend an undecodable ad image as the first segment, which makes mpv
// fail immediately. Validated streams are preferred; unvalidated streams are
// kept as a fallback so a transient validation failure never loses a link.
func prioritizePlayableStreams(links []string, hints map[string]providers.StreamPlaybackHint) []string {
	if len(links) < 2 {
		return links
	}

	validationErrors := make([]error, len(links))
	var wg sync.WaitGroup
	sem := make(chan struct{}, maxConcurrentAnikotoValidations)
	for index, link := range links {
		wg.Add(1)
		go func(index int, link string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			hint := hints[link]
			validationErrors[index] = hlsverify.Validate(link, hlsverify.Options{
				Referer:   hint.Referrer,
				UserAgent: userAgent,
			})
		}(index, link)
	}
	wg.Wait()

	validated := make([]string, 0, len(links))
	unvalidated := make([]string, 0, len(links))
	for index, link := range links {
		if err := validationErrors[index]; err != nil {
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
