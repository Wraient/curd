package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const stackedProviderConfigValue = "stacked"

func isStackedProviderConfig(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "stacked", "stack", "auto", "all":
		return true
	default:
		return false
	}
}

func isFactoryDefaultProvider(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return true
	}
	if isStackedProviderConfig(raw) {
		return true
	}

	names, declined := parseProviderConfig(raw)
	if declined {
		return false
	}
	switch len(names) {
	case 0:
		return true
	case 1:
		switch names[0] {
		case "senshi", "allanime":
			return true
		}
	}
	return false
}

func providerListsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func migrateProviderConfig(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if isFactoryDefaultProvider(raw) {
		if raw == stackedProviderConfigValue {
			return raw, false
		}
		return stackedProviderConfigValue, true
	}

	if isStackedProviderConfig(raw) {
		if raw != stackedProviderConfigValue {
			return stackedProviderConfigValue, true
		}
		return raw, false
	}

	names, declined := parseProviderConfig(raw)
	if len(names) > 1 {
		return stackedProviderConfigValue, true
	}

	canonical := canonicalProviderConfigValue(raw)
	if canonical != raw {
		return canonical, true
	}
	_ = declined
	return raw, false
}

func readStoredCurdVersion(storagePath string) string {
	path := storageVersionFilePath(storagePath)
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

func writeStoredCurdVersion(storagePath, version string) error {
	storagePath = strings.TrimSpace(storagePath)
	version = strings.TrimSpace(version)
	if storagePath == "" || version == "" {
		return nil
	}
	if err := os.MkdirAll(storagePath, 0755); err != nil {
		return err
	}
	return os.WriteFile(storageVersionFilePath(storagePath), []byte(version+"\n"), 0644)
}

// MigrateOnVersionUpgrade updates stored state and config when curd is upgraded.
// Returns whether the config file was updated.
func MigrateOnVersionUpgrade(configPath string, config *CurdConfig, appVersion string) (bool, error) {
	if config == nil {
		return false, nil
	}

	appVersion = strings.TrimSpace(appVersion)
	if appVersion == "" {
		appVersion = CurdVersion()
	}

	storagePath := os.ExpandEnv(config.StoragePath)
	if storagePath == "" {
		storagePath = filepath.Join(os.ExpandEnv("$HOME"), ".local", "share", "curd")
	}

	storedVersion := readStoredCurdVersion(storagePath)
	configUpdated := false

	if storedVersion != appVersion {
		if nextProvider, changed := migrateProviderConfig(config.Provider); changed {
			config.Provider = nextProvider
			configUpdated = true
		}
	}

	if err := writeStoredCurdVersion(storagePath, appVersion); err != nil {
		return configUpdated, fmt.Errorf("write curd version file: %w", err)
	}

	if !configUpdated || strings.TrimSpace(configPath) == "" {
		return configUpdated, nil
	}

	configMap, err := LoadConfigFromFile(configPath)
	if err != nil {
		return configUpdated, err
	}
	configMap["Provider"] = config.Provider
	if err := SaveConfigToFile(configPath, configMap); err != nil {
		return configUpdated, err
	}
	return configUpdated, nil
}

func providerConfigDisplayLabel(raw string) string {
	if isStackedProviderConfig(raw) {
		names := defaultEnabledProviderStack()
		if len(names) == 0 {
			return "Default with fallback"
		}
		return fmt.Sprintf("Default with fallback (%s)", strings.Join(names, " → "))
	}
	names, _ := parseProviderConfig(raw)
	if len(names) == 1 {
		return names[0]
	}
	if len(names) > 1 {
		return strings.Join(names, " → ")
	}
	return raw
}
