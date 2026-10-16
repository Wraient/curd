package anipub

import "github.com/wraient/curd/internal/providers"

func init() {
	providers.Register(providers.Meta{
		Name:     "anipub",
		Aliases:  []string{"anipub.org"},
		Referrer: "https://megaplay.buzz/",
	}, func() providers.Provider {
		return &Provider{}
	})
}
