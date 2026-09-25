package senshi

import "github.com/wraient/curd/internal/providers"

func init() {
	providers.Register(providers.Meta{
		Name:     "senshi",
		Aliases:  []string{"senshi.to", "senshi.live", "senshi project"},
		Referrer: "https://senshi.to/",
	}, func() providers.Provider {
		return &Provider{}
	})
}
