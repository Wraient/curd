package allanime

import "github.com/wraient/curd/internal/providers"

func init() {
	providers.Register(providers.Meta{
		Name:            "allanime",
		Aliases:         []string{"all-anime", "all anime"},
		Referrer:        "https://allanime.day/",
		DefaultDisabled: true,
		DisableReason:   "disabled by default; set Provider to include allanime to enable",
	}, func() providers.Provider {
		return &Provider{}
	})
}
