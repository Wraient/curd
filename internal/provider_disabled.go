package internal

import (
	"sort"
	"strings"
	"testing"

	"github.com/wraient/curd/internal/providers"
)

var disabledProviderReasonForTest func(string) string

func providerDisabledReason(name string) string {
	name = normalizeProviderName(name)
	if name == "" {
		return ""
	}
	if disabledProviderReasonForTest != nil {
		return disabledProviderReasonForTest(name)
	}
	if reason := configDisabledProviderReason(name); reason != "" {
		return reason
	}
	meta, ok := providers.MetaFor(name)
	if !ok {
		return ""
	}
	if meta.DefaultDisabled {
		return meta.DisableReason
	}
	return ""
}

func configDisabledProviderReason(name string) string {
	config := GetGlobalConfig()
	if config == nil {
		return ""
	}
	for _, disabledName := range parseDisabledProviderNames(config.DisabledProviders) {
		if disabledName == name {
			if meta, ok := providers.MetaFor(name); ok && meta.DisableReason != "" {
				return meta.DisableReason
			}
			return "disabled in config"
		}
	}
	return ""
}

func parseDisabledProviderNames(raw string) []string {
	parts := parseStringArray(raw)
	if len(parts) == 0 {
		parts = strings.FieldsFunc(raw, func(r rune) bool {
			return r == ',' || r == '+' || r == '|' || r == ';'
		})
	}
	names := make([]string, 0, len(parts))
	seen := make(map[string]struct{})
	for _, part := range parts {
		name := normalizeProviderName(part)
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	return names
}

// ProviderEnabled reports whether a provider may be used at runtime.
func ProviderEnabled(name string) bool {
	return providerDisabledReason(name) == ""
}

// ProviderDisabledReason returns why a provider is disabled, or "" when enabled.
func ProviderDisabledReason(name string) string {
	return providerDisabledReason(name)
}

func filterEnabledProviders(names []string) []string {
	if len(names) == 0 {
		return nil
	}
	enabled := make([]string, 0, len(names))
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		name = normalizeProviderName(name)
		if name == "" || !ProviderEnabled(name) {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		enabled = append(enabled, name)
	}
	return enabled
}

func defaultEnabledProviderStack() []string {
	names := providers.RegisteredNames()
	return filterEnabledProviders(names)
}

func providerSelectionOptions() []SelectionOption {
	enabled := defaultEnabledProviderStack()
	options := make([]SelectionOption, 0, len(enabled)*len(enabled))
	for _, name := range enabled {
		options = append(options, SelectionOption{
			Key:   formatProviderConfigValue([]string{name}, false),
			Label: name,
		})
	}
	for _, primary := range enabled {
		for _, secondary := range enabled {
			if primary == secondary {
				continue
			}
			names := []string{primary, secondary}
			options = append(options, SelectionOption{
				Key:   formatProviderConfigValue(names, false),
				Label: primary + ", then " + secondary,
			})
		}
	}
	return options
}

func firstEnabledProviderName() string {
	enabled := defaultEnabledProviderStack()
	if len(enabled) > 0 {
		return enabled[0]
	}
	return "anineko"
}

func ensureEnabledProviderNames(names []string) []string {
	enabled := filterEnabledProviders(names)
	if len(enabled) > 0 {
		return enabled
	}
	fallback := filterEnabledProviders([]string{firstEnabledProviderName()})
	if len(fallback) > 0 {
		return fallback
	}
	if registered := providers.RegisteredNames(); len(registered) > 0 {
		for _, name := range registered {
			if ProviderEnabled(name) {
				return []string{name}
			}
		}
		return []string{registered[0]}
	}
	return []string{firstEnabledProviderName()}
}

func withAllProvidersEnabledForTest(t *testing.T) {
	t.Helper()
	previous := disabledProviderReasonForTest
	disabledProviderReasonForTest = func(string) string { return "" }
	t.Cleanup(func() {
		disabledProviderReasonForTest = previous
	})
}

// RegisteredProviderNames returns sorted registered provider names.
func RegisteredProviderNames() []string {
	names := providers.RegisteredNames()
	sort.Strings(names)
	return names
}
