package anilight

import "github.com/wraient/curd/internal/providers"

func init() {
	providers.Register(providers.Meta{
		Name:     "anilight",
		Aliases:  []string{"ani.light", "ani-light"},
		Referrer: "https://anilight.live/",
	}, func() providers.Provider {
		return &Provider{}
	})
}
