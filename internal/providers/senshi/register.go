package senshi

import "github.com/wraient/curd/internal/providers"

func init() {
	providers.Register(providers.Meta{
		Name:     "senshi",
		Aliases:  []string{"senshi.live", "senshi project"},
		Referrer: "https://senshi.live/",
	}, func() providers.Provider {
		return &Provider{}
	})
}
