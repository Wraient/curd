package internal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/wraient/curd/internal/loadproviders"
)

func TestProviderSelectionOptionsUsesDefaultAndSingleProviders(t *testing.T) {
	withAllProvidersEnabledForTest(t)
	options := providerSelectionOptions()
	if len(options) == 0 {
		t.Fatal("expected provider options")
	}
	if options[0].Key != "stacked" {
		t.Fatalf("first option key = %q, want stacked", options[0].Key)
	}
	if !strings.Contains(options[0].Label, "Default with fallback") {
		t.Fatalf("first option label = %q", options[0].Label)
	}
	for _, option := range options {
		if strings.Contains(option.Label, ", then ") {
			t.Fatalf("unexpected combo label %q", option.Label)
		}
	}
}

func TestMigrateProviderConfig(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		want    string
		changed bool
	}{
		{name: "legacy senshi default", raw: `["senshi"]`, want: "stacked", changed: true},
		{name: "legacy allanime default", raw: `["allanime"]`, want: "stacked", changed: true},
		{name: "stack alias", raw: "stack", want: "stacked", changed: true},
		{name: "already stacked", raw: "stacked", want: "stacked", changed: false},
		{name: "single anineko", raw: `["anineko"]`, want: `["anineko"]`, changed: false},
		{name: "legacy pair", raw: `["senshi","anineko"]`, want: "stacked", changed: true},
	}

	for _, tc := range cases {
		got, changed := migrateProviderConfig(tc.raw)
		if changed != tc.changed || got != tc.want {
			t.Fatalf("%s: got (%q, %v), want (%q, %v)", tc.name, got, changed, tc.want, tc.changed)
		}
	}
}

func TestCanonicalProviderConfigValuePrefersStackedToken(t *testing.T) {
	withAllProvidersEnabledForTest(t)
	if got := canonicalProviderConfigValue("stacked"); got != "stacked" {
		t.Fatalf("got %q, want stacked", got)
	}
	if got := canonicalProviderConfigValue(""); got != "stacked" {
		t.Fatalf("empty got %q, want stacked", got)
	}
}

func TestMigrateOnVersionUpgradeWritesVersionAndUpdatesProvider(t *testing.T) {
	withAllProvidersEnabledForTest(t)
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "curd.conf")
	storagePath := filepath.Join(tempDir, "share")
	if err := os.WriteFile(configPath, []byte("Provider=[\"senshi\"]\nStoragePath="+storagePath+"\n"), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if config.Provider != `["senshi"]` {
		t.Fatalf("pre-migration provider = %q", config.Provider)
	}

	updated, err := MigrateOnVersionUpgrade(configPath, &config, "2.1.0")
	if err != nil {
		t.Fatalf("MigrateOnVersionUpgrade: %v", err)
	}
	if !updated {
		t.Fatal("expected provider config update")
	}
	if config.Provider != "stacked" {
		t.Fatalf("provider = %q, want stacked", config.Provider)
	}

	versionBytes, err := os.ReadFile(filepath.Join(storagePath, "curd_version"))
	if err != nil {
		t.Fatalf("read version file: %v", err)
	}
	if strings.TrimSpace(string(versionBytes)) != "2.1.0" {
		t.Fatalf("version file = %q", string(versionBytes))
	}

	contents, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(contents), "Provider=stacked") {
		t.Fatalf("config not persisted: %s", string(contents))
	}

	updated, err = MigrateOnVersionUpgrade(configPath, &config, "2.1.0")
	if err != nil {
		t.Fatalf("second migration: %v", err)
	}
	if updated {
		t.Fatal("expected no update on same version")
	}
}

func TestConfiguredProviderNamesUsesStackedByDefault(t *testing.T) {
	withAllProvidersEnabledForTest(t)
	got := ConfiguredProviderNames(&CurdConfig{})
	want := []string{"senshi", "anipub", "anineko", "allanime", "animepahe"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}
