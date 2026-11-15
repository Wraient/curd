package anipm

import "github.com/wraient/curd/internal/providers"

func init() {
	providers.Register(providers.Meta{
		Name:     "anipm",
		Aliases:  []string{"ani.pm", "ani-pm"},
		Referrer: "https://ani.pm/",
	}, func() providers.Provider {
		return &Provider{}
	})
}
