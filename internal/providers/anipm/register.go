package anipm

import "github.com/wraient/curd/internal/providers"

func init() {
	providers.Register(providers.Meta{
		Name:    "anipm",
		Aliases: []string{"ani.pm", "ani-pm"},
		// The browser plays from the settlar embed page: mpv must send
		// the embed origin as referer, not the catalog site.
		Referrer: "https://embed.settlar.io/",
		// Settlar object URLs are short-lived: prefetched next-episode
		// links die before the current episode ends.
		NoPrefetch: true,
	}, func() providers.Provider {
		return &Provider{}
	})
}
