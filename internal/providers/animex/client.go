package animex

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/wraient/curd/internal/curdhost"
)

const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

var (
	graphqlURL = "https://graphql.animex.one/graphql"
	restBase   = "https://pp.animex.one/rest/api"
	referrer   = "https://animex.one/"
)

// searchQuery is the site's own FastSearch query, trimmed to the fields we
// need. Keeping the exact shape avoids tripping the GraphQL validation.
const searchQuery = `query FastSearch($query: String, $limit: Int, $includeAdult: Boolean) { catalogAnime(filter: { query: $query, includeAdult: $includeAdult }, limit: $limit) { items { id anilistId malId titleRomaji titleEnglish format episodeCount seasonYear coverImage } } }`

func newRequest(rawURL, referer string) (*http.Request, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	return req, nil
}

func fetchJSON(rawURL, referer string, dest any) error {
	req, err := newRequest(rawURL, referer)
	if err != nil {
		return err
	}
	resp, err := curdhost.HTTPClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if !curdhost.HTTPStatusOK(resp.StatusCode) {
		return curdhost.HTTPStatusError("animex request", resp.StatusCode, body)
	}
	if dest == nil {
		return nil
	}
	if err := json.Unmarshal(body, dest); err != nil {
		return fmt.Errorf("parse animex response: %w", err)
	}
	return nil
}

func postGraphQL(payload any, dest any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, graphqlURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Referer", referrer)
	resp, err := curdhost.HTTPClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if !curdhost.HTTPStatusOK(resp.StatusCode) {
		return curdhost.HTTPStatusError("animex graphql", resp.StatusCode, raw)
	}
	if dest == nil {
		return nil
	}
	if err := json.Unmarshal(raw, dest); err != nil {
		return fmt.Errorf("parse animex graphql: %w", err)
	}
	return nil
}

func searchCatalog(query string) ([]catalogItem, error) {
	payload := map[string]any{
		"query": searchQuery,
		"variables": map[string]any{
			"query":        strings.TrimSpace(query),
			"limit":        24,
			"includeAdult": false,
		},
	}
	var response catalogResponse
	if err := postGraphQL(payload, &response); err != nil {
		return nil, err
	}
	return response.Data.Catalog.Items, nil
}

func restURL(path string, params map[string]string) string {
	query := url.Values{}
	for key, value := range params {
		query.Set(key, value)
	}
	endpoint := strings.TrimSuffix(restBase, "/") + "/" + strings.TrimPrefix(path, "/")
	if encoded := query.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}
	return endpoint
}
