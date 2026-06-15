package internal

import "testing"

func TestProviderEnabledDisablesAnimepaheByDefault(t *testing.T) {
	if ProviderEnabled("allanime") != true {
		t.Fatal("expected allanime to stay enabled")
	}
	if ProviderEnabled("animepahe") != false {
		t.Fatal("expected animepahe to be disabled")
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
		{name: "empty", cfg: &CurdConfig{}, want: []string{"allanime"}},
		{name: "json list", cfg: &CurdConfig{Provider: `["allanime","animepahe"]`}, want: []string{"allanime"}},
		{name: "animepahe only", cfg: &CurdConfig{Provider: `["animepahe"]`}, want: []string{"allanime"}},
		{name: "legacy alias", cfg: &CurdConfig{Provider: "stacked"}, want: []string{"allanime"}},
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
	if _, err := ProviderByName("animepahe"); err == nil {
		t.Fatal("expected disabled provider error")
	}
}

func TestProviderByNameAllowsDisabledProviderWhenOverridden(t *testing.T) {
	withAllProvidersEnabledForTest(t)

	provider, err := ProviderByName("animepahe")
	if err != nil {
		t.Fatalf("expected animepahe provider: %v", err)
	}
	if provider.Name() != "animepahe" {
		t.Fatalf("got provider %q", provider.Name())
	}
}
