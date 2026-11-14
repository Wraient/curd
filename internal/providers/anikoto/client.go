package anikoto

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/wraient/curd/internal/curdhost"
)

const (
	userAgent        = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	maxResponseBytes = 16 << 20
)

var baseURL = "https://anikototv.to"

func newRequest(method, rawURL, referer, accept string, ajax bool) (*http.Request, error) {
	req, err := http.NewRequest(method, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Referer", referer)
	req.Header.Set("Accept", accept)
	if ajax {
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
	}
	return req, nil
}

func fetchResource(rawURL, referer, accept string, ajax bool) ([]byte, error) {
	req, err := newRequest(http.MethodGet, rawURL, referer, accept, ajax)
	if err != nil {
		return nil, err
	}
	return doRequest(req)
}

func decodeAjaxResponse(raw []byte, dest ajaxStatusResponse) error {
	if err := decodeJSON(raw, dest); err != nil {
		return err
	}
	if status := dest.statusCode(); status != 0 && status != http.StatusOK {
		return fmt.Errorf("Anikoto AJAX response returned status %d", status)
	}
	return nil
}

func decodeJSON(raw []byte, dest any) error {
	if err := json.Unmarshal(raw, dest); err != nil {
		return fmt.Errorf("parse Anikoto response: %w", err)
	}
	return nil
}

func doRequest(req *http.Request) ([]byte, error) {
	client := curdhost.HTTPClient()
	if client == nil {
		return nil, fmt.Errorf("Anikoto HTTP client is not configured")
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maxResponseBytes {
		return nil, fmt.Errorf("Anikoto response is too large")
	}
	if !curdhost.HTTPStatusOK(resp.StatusCode) {
		return nil, curdhost.HTTPStatusError("Anikoto request", resp.StatusCode, raw)
	}
	return raw, nil
}
