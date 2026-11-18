package animex

import "github.com/wraient/curd/internal/providers"

func init() {
	providers.Register(providers.Meta{
		Name:     "animex",
		Aliases:  []string{"anime-x"},
		Referrer: "https://animex.one/",
	}, func() providers.Provider {
		return &Provider{}
	})
}
