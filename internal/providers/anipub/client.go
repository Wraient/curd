package anipub

import (
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
	baseURL         = "https://anipub.xyz"
	megaplayBaseURL = "https://megaplay.buzz"
)

func newRequest(method, rawURL, referer string) (*http.Request, error) {
	req, err := http.NewRequest(method, rawURL, nil)
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
	req, err := newRequest(http.MethodGet, rawURL, referer)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := curdhost.HTTPClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if !curdhost.HTTPStatusOK(resp.StatusCode) {
		return curdhost.HTTPStatusError("anipub request", resp.StatusCode, raw)
	}
	if dest == nil {
		return nil
	}
	if err := json.Unmarshal(raw, dest); err != nil {
		return fmt.Errorf("parse anipub response: %w", err)
	}
	return nil
}

func fetchString(rawURL, referer string) (string, error) {
	req, err := newRequest(http.MethodGet, rawURL, referer)
	if err != nil {
		return "", err
	}
	resp, err := curdhost.HTTPClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if !curdhost.HTTPStatusOK(resp.StatusCode) {
		return "", curdhost.HTTPStatusError("anipub request", resp.StatusCode, body)
	}
	return string(body), nil
}

func searchURL(query string) string {
	return fmt.Sprintf("%s/api/search/%s", baseURL, url.PathEscape(strings.TrimSpace(query)))
}

func infoURL(showID string) string {
	return fmt.Sprintf("%s/api/info/%s", baseURL, url.PathEscape(strings.TrimSpace(showID)))
}

func detailsURL(showID string) string {
	return fmt.Sprintf("%s/v1/api/details/%s", baseURL, url.PathEscape(strings.TrimSpace(showID)))
}

func thumbnailFromInfo(info infoResponse, fallback string) string {
	for _, candidate := range []string{info.Image, info.Cover, fallback} {
		candidate = strings.TrimSpace(candidate)
		if candidate != "" {
			return candidate
		}
	}
	return ""
}
