package anikoto

import "github.com/wraient/curd/internal/providers"

func init() {
	providers.Register(providers.Meta{
		Name:       "anikoto",
		Aliases:    []string{"anikoto.tv"},
		Referrer:   "https://anikototv.to/",
		NoPrefetch: true,
	}, func() providers.Provider {
		return &Provider{}
	})
}
