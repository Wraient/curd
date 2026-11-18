package aniwaves

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strings"

	"github.com/wraient/curd/internal/providers"
	"github.com/wraient/curd/internal/providers/hlsverify"
	"github.com/wraient/curd/internal/providers/substyle"
)

var (
	serverTypeRE  = regexp.MustCompile(`data-type="(sub|dub)"`)
	serverLinkRE  = regexp.MustCompile(`data-link-id="([A-Za-z0-9]+)"`)
	embedDataIDRE = regexp.MustCompile(`data-id="([A-Za-z0-9]+)"`)
)

// splitServerSections cuts the server list HTML at each data-type marker so
// link IDs can be attributed to their sub/dub section without lookahead
// (unsupported by Go's regexp engine).
func splitServerSections(html string) [][2]string {
	locs := serverTypeRE.FindAllStringSubmatchIndex(html, -1)
	var sections [][2]string
	for i, loc := range locs {
		end := len(html)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		typ := html[loc[2]:loc[3]]
		sections = append(sections, [2]string{typ, html[loc[0]:end]})
	}
	return sections
}

func getEpisodeStreamsForMode(showID string, config providers.PlaybackConfig, epNo int) ([]string, map[string]providers.StreamPlaybackHint, error) {
	showID = strings.TrimSpace(showID)
	if showID == "" {
		return nil, nil, fmt.Errorf("empty show id")
	}
	if epNo <= 0 {
		return nil, nil, fmt.Errorf("invalid episode number %d", epNo)
	}

	mode := providers.NormalizeTranslationType(config.SubOrDub)
	if mode == "sub" {
		// aniwaves sub embeds carry no downloadable hardsubs and no
		// separate soft tracks; an explicit hard preference degrades to
		// the sub stream via the shared substyle contract.
		if _, err := substyle.Choose(true, false, config.SubStyle); err != nil {
			return nil, nil, err
		}
	}

	numericID := showNumericID(showID)
	if numericID == "" {
		return nil, nil, fmt.Errorf("invalid aniwaves show id %q", showID)
	}

	linkIDs, err := episodeServerLinks(numericID, showID, epNo, mode)
	if err != nil {
		return nil, nil, err
	}
	if len(linkIDs) == 0 {
		return nil, nil, fmt.Errorf("aniwaves has no %s servers for episode %d", mode, epNo)
	}

	var lastErr error
	for _, linkID := range linkIDs {
		links, hints, err := resolveServerLink(showID, linkID)
		if err != nil {
			lastErr = err
			continue
		}
		return links, hints, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no playable streams resolved for episode %d", epNo)
	}
	return nil, nil, lastErr
}

// episodeServerLinks returns the embed link IDs listed under the requested
// sub/dub server section, default server first.
func episodeServerLinks(numericID, showID string, epNo int, mode string) ([]string, error) {
	query := fmt.Sprintf("servers=%s&eps=%d", numericID, epNo)
	body, _, err := fetchBytes(baseURL+"/ajax/server/list?"+query, baseURL+"/watch/"+showID, nil)
	if err != nil {
		return nil, err
	}
	var payload ajaxEnvelope
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("parse aniwaves servers: %w", err)
	}
	if err := checkAjaxStatus(&payload, "server list"); err != nil {
		return nil, err
	}
	var html string
	if err := json.Unmarshal(payload.Result, &html); err != nil {
		return nil, fmt.Errorf("parse aniwaves server list: %w", err)
	}
	var ids []string
	for _, section := range splitServerSections(html) {
		if section[0] != mode {
			continue
		}
		for _, match := range serverLinkRE.FindAllStringSubmatch(section[1], -1) {
			ids = append(ids, match[1])
		}
	}
	return ids, nil
}

func resolveServerLink(showID, linkID string) ([]string, map[string]providers.StreamPlaybackHint, error) {
	body, _, err := fetchBytes(baseURL+"/ajax/sources?id="+url.QueryEscape(linkID), baseURL+"/watch/"+showID, nil)
	if err != nil {
		return nil, nil, err
	}
	var envelope ajaxEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, nil, fmt.Errorf("parse aniwaves sources: %w", err)
	}
	if err := checkAjaxStatus(&envelope, "sources"); err != nil {
		return nil, nil, err
	}
	var result sourcesResult
	if err := json.Unmarshal(envelope.Result, &result); err != nil {
		return nil, nil, fmt.Errorf("parse aniwaves sources result: %w", err)
	}
	embedURL := strings.TrimSpace(result.URL)
	if embedURL == "" {
		return nil, nil, fmt.Errorf("aniwaves returned no embed url")
	}

	// The vhscdn embed page sets a session cookie required by its
	// getSources call; carry the cookies forward.
	embedBody, cookies, err := fetchBytes(embedURL, baseURL+"/", nil)
	if err != nil {
		return nil, nil, err
	}
	match := embedDataIDRE.FindSubmatch(embedBody)
	if len(match) < 2 {
		return nil, nil, fmt.Errorf("aniwaves embed has no stream id")
	}
	dataID := string(match[1])

	parsed, err := url.Parse(embedURL)
	if err != nil {
		return nil, nil, fmt.Errorf("parse aniwaves embed url: %w", err)
	}
	sourcesURL := parsed.Scheme + "://" + parsed.Host + path.Dir(parsed.Path) + "/getSources?id=" + url.QueryEscape(dataID)
	jsonBody, _, err := fetchBytes(sourcesURL, embedURL, cookies)
	if err != nil {
		return nil, nil, err
	}
	var sources vhSources
	if err := json.Unmarshal(jsonBody, &sources); err != nil {
		return nil, nil, fmt.Errorf("parse aniwaves sources: %w", err)
	}
	if strings.TrimSpace(sources.Error) != "" {
		return nil, nil, fmt.Errorf("aniwaves stream unavailable: %s", sources.Error)
	}
	streamURL := strings.TrimSpace(sources.Sources)
	if streamURL == "" {
		return nil, nil, fmt.Errorf("aniwaves returned no stream url")
	}
	if err := hlsverify.Validate(streamURL, hlsverify.Options{Referer: embedURL, UserAgent: userAgent}); err != nil {
		return nil, nil, fmt.Errorf("aniwaves stream rejected: %w", err)
	}
	hints := map[string]providers.StreamPlaybackHint{
		streamURL: {Referrer: embedURL},
	}
	return []string{streamURL}, hints, nil
}
