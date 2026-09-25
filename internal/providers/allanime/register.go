package allanime

import "github.com/wraient/curd/internal/providers"

func init() {
	providers.Register(providers.Meta{
		Name:            "allanime",
		Aliases:         []string{"all-anime", "all anime"},
		Referrer:        "https://allanime.day/",
		DefaultDisabled: true,
		DisableReason:   "episode sources require rotating signed anti-bot tokens (AA_CRYPTO_MISSING); set Provider to include allanime to try anyway",
	}, func() providers.Provider {
		return &Provider{}
	})
}
