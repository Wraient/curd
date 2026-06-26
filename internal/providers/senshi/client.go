package senshi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/wraient/curd/internal/curdhost"
)

const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

var baseURL = "https://senshi.live"

func newRequest(method, rawURL string) (*http.Request, error) {
	req, err := http.NewRequest(method, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Referer", baseURL+"/")
	return req, nil
}

func fetchJSON(method, rawURL string, payload any, dest any) error {
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}

	req, err := http.NewRequest(method, rawURL, body)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Referer", baseURL+"/")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

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
		return curdhost.HTTPStatusError("senshi request", resp.StatusCode, raw)
	}
	if dest == nil {
		return nil
	}
	if err := json.Unmarshal(raw, dest); err != nil {
		return fmt.Errorf("parse senshi response: %w", err)
	}
	return nil
}

func absoluteURL(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return baseURL + path
}

func posterURL(malID int) string {
	if malID <= 0 {
		return ""
	}
	return fmt.Sprintf("%s/posters/%d.webp", baseURL, malID)
}
