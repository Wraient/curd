package curdhost

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// PromptOption is a minimal menu item for provider-driven prompts.
type PromptOption struct {
	Key   string
	Label string
}

// Host hooks are wired from the main application package during init.
var (
	HTTPClient             func() *http.Client
	Log                    func(string)
	Out                     func(string)
	PromptSelect            func(options []PromptOption) (PromptOption, error)
	CurrentSubStyle         func() string
	PersistSubStylePreference func(style string) error
	StoragePath             func() string
	AnimeNameLanguage      func() string
	SetCookiesForAnimepahe func(u *url.URL, cookies []*http.Cookie)
)

func HTTPStatusOK(statusCode int) bool {
	return statusCode >= http.StatusOK && statusCode < http.StatusMultipleChoices
}

func HTTPStatusError(context string, statusCode int, body []byte) error {
	snippet := strings.TrimSpace(string(body))
	if len(snippet) > 300 {
		snippet = snippet[:300] + "..."
	}
	if snippet != "" {
		return fmt.Errorf("%s failed with status %d: %s", context, statusCode, snippet)
	}
	return fmt.Errorf("%s failed with status %d", context, statusCode)
}
