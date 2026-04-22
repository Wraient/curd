package internal

import (
	"net/http"
	"time"
)

var sharedHTTPClient = &http.Client{
	Transport: &http.Transport{
		MaxIdleConns:        10,
		MaxIdleConnsPerHost: 5,
		IdleConnTimeout:     30 * time.Second,
	},
	Timeout: 15 * time.Second,
}
