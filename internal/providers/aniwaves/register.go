package aniwaves

import "github.com/wraient/curd/internal/providers"

func init() {
	providers.Register(providers.Meta{
		Name:    "aniwaves",
		Aliases: []string{"aniwave", "ani-waves"},
		Referrer: "https://aniwaves.tv/",
		// vhscdn stream URLs are short-lived session URLs: prefetched
		// next-episode links die before the current episode ends.
		NoPrefetch: true,
	}, func() providers.Provider {
		return &Provider{}
	})
}
