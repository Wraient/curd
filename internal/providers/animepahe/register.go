package animepahe

import "github.com/wraient/curd/internal/providers"

func init() {
	providers.Register(providers.Meta{
		Name:            "animepahe",
		Aliases:         []string{"pahe"},
		Referrer:        "https://kwik.cx/",
		DefaultDisabled: true,
		DisableReason:   "Cloudflare/DDoS-Guard protection cannot be bypassed reliably",
		OptOutToken:     "no-animepahe",
		FallbackPrompt:  "Animepahe may require downloading a Chromium browser for DDoS-Guard verification (~500 MB).",
	}, func() providers.Provider {
		return &Provider{}
	})
}
