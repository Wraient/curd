package anineko

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/wraient/curd/internal/curdhost"
)

const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

var baseURL = "https://anineko.to"

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
		return "", curdhost.HTTPStatusError("anineko request", resp.StatusCode, body)
	}
	return string(body), nil
}

func fetchBytes(rawURL, referer string) ([]byte, error) {
	req, err := newRequest(http.MethodGet, rawURL, referer)
	if err != nil {
		return nil, err
	}
	resp, err := curdhost.HTTPClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if !curdhost.HTTPStatusOK(resp.StatusCode) {
		return nil, curdhost.HTTPStatusError("anineko request", resp.StatusCode, body)
	}
	return body, nil
}

func absoluteURL(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	if strings.HasPrefix(path, "//") {
		return "https:" + path
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return baseURL + path
}

func watchURL(slug string, epNo int) string {
	if epNo > 0 {
		return fmt.Sprintf("%s/watch/%s/ep-%d", baseURL, slug, epNo)
	}
	return fmt.Sprintf("%s/watch/%s", baseURL, slug)
}
