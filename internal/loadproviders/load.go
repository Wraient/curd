// Package loadproviders registers built-in streaming providers via init side effects.
package loadproviders

import (
	_ "github.com/wraient/curd/internal/providers/allanime"
	_ "github.com/wraient/curd/internal/providers/animepahe"
	_ "github.com/wraient/curd/internal/providers/anineko"
	_ "github.com/wraient/curd/internal/providers/anipub"
	_ "github.com/wraient/curd/internal/providers/senshi"
)
