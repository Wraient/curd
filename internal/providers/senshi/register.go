package senshi

import "github.com/wraient/curd/internal/providers"

func init() {
	providers.Register(providers.Meta{
		Name:    "senshi",
		Aliases: []string{"senshi.to", "senshi.live", "senshi project"},
		Referrer: "https://senshi.to/",
		// Upstream returns Cloudflare 523 (origin unreachable) on every
		// endpoint; keep it out of the default stack until it recovers so
		// every search and resolve does not wait for a dead provider.
		DefaultDisabled: true,
		DisableReason:   "senshi upstream is down (Cloudflare 523); re-enable when it recovers",
	}, func() providers.Provider {
		return &Provider{}
	})
}
