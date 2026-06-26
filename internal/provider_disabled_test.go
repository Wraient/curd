package internal

import "testing"

func TestProviderEnabledDisablesAllanimeAndAnimepaheByDefault(t *testing.T) {
	if ProviderEnabled("allanime") != false {
		t.Fatal("expected allanime to be disabled by default")
	}
	if ProviderEnabled("animepahe") != false {
		t.Fatal("expected animepahe to be disabled by default")
	}
	if ProviderEnabled("senshi") != true {
		t.Fatal("expected senshi to stay enabled")
	}
	if ProviderEnabled("anineko") != true {
		t.Fatal("expected anineko to stay enabled")
	}
	if reason := ProviderDisabledReason("allanime"); reason == "" {
		t.Fatal("expected allanime disable reason")
	}
	if reason := ProviderDisabledReason("animepahe"); reason == "" {
		t.Fatal("expected animepahe disable reason")
	}
}

func TestConfiguredProviderNamesFiltersDisabledProviders(t *testing.T) {
	cases := []struct {
		name string
		cfg  *CurdConfig
		want []string
	}{
		{name: "empty", cfg: &CurdConfig{}, want: []string{"senshi", "anipub", "anineko"}},
		{name: "json list", cfg: &CurdConfig{Provider: `["allanime","animepahe"]`}, want: []string{"senshi"}},
		{name: "animepahe only", cfg: &CurdConfig{Provider: `["animepahe"]`}, want: []string{"senshi"}},
		{name: "allanime only", cfg: &CurdConfig{Provider: `["allanime"]`}, want: []string{"senshi"}},
		{name: "legacy alias", cfg: &CurdConfig{Provider: "stacked"}, want: []string{"senshi", "anipub", "anineko"}},
	}

	for _, tc := range cases {
		got := ConfiguredProviderNames(tc.cfg)
		if len(got) != len(tc.want) {
			t.Fatalf("%s: got %v, want %v", tc.name, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Fatalf("%s: got %v, want %v", tc.name, got, tc.want)
			}
		}
	}
}

func TestConfiguredProviderNamesHonorsEnabledProvidersWhenOverridden(t *testing.T) {
	withAllProvidersEnabledForTest(t)

	cfg := &CurdConfig{Provider: `["allanime","animepahe"]`}
	got := ConfiguredProviderNames(cfg)
	if len(got) != 2 || got[0] != "allanime" || got[1] != "animepahe" {
		t.Fatalf("got %v, want [allanime animepahe]", got)
	}
}

func TestProviderByNameRejectsDisabledProvider(t *testing.T) {
	for _, name := range []string{"animepahe", "allanime"} {
		if _, err := ProviderByName(name); err == nil {
			t.Fatalf("expected disabled provider error for %s", name)
		}
	}
}

func TestProviderByNameAllowsDisabledProviderWhenOverridden(t *testing.T) {
	withAllProvidersEnabledForTest(t)

	for _, name := range []string{"animepahe", "allanime"} {
		provider, err := ProviderByName(name)
		if err != nil {
			t.Fatalf("expected %s provider: %v", name, err)
		}
		if provider.Name() != name {
			t.Fatalf("got provider %q", provider.Name())
		}
	}
}
