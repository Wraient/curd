package aniwaves

import "github.com/wraient/curd/internal/providers"

func init() {
	providers.Register(providers.Meta{
		Name:     "aniwaves",
		Aliases:  []string{"aniwave", "ani-waves"},
		Referrer: "https://aniwaves.tv/",
	}, func() providers.Provider {
		return &Provider{}
	})
}
