package anineko

import "github.com/wraient/curd/internal/providers"

func init() {
	providers.Register(providers.Meta{
		Name:            "anineko",
		Aliases:         []string{"ani-neko", "ani neko"},
		Referrer:        "https://anineko.to/",
		DefaultDisabled: true,
		DisableReason:   "anineko.to is down (origin unreachable)",
	}, func() providers.Provider {
		return &Provider{}
	})
}
