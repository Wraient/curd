package internal

import (
	"sort"
	"testing"
)

// disabledProviders lists providers kept in source but excluded from runtime use.
// Remove a provider from this map to re-enable it without restoring deleted code.
var disabledProviders = map[string]string{
	"animepahe": "Cloudflare/DDoS-Guard protection cannot be bypassed reliably",
}

var disabledProviderReasonForTest func(string) string

func providerDisabledReason(name string) string {
	name = normalizeProviderName(name)
	if name == "" {
		return ""
	}
	if disabledProviderReasonForTest != nil {
		return disabledProviderReasonForTest(name)
	}
	return disabledProviders[name]
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
	names := make([]string, 0, len(providerFactories))
	for name := range providerFactories {
		names = append(names, name)
	}
	sort.Strings(names)
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

func ensureEnabledProviderNames(names []string) []string {
	enabled := filterEnabledProviders(names)
	if len(enabled) > 0 {
		return enabled
	}
	return []string{"allanime"}
}

func withAllProvidersEnabledForTest(t *testing.T) {
	t.Helper()
	previous := disabledProviderReasonForTest
	disabledProviderReasonForTest = func(string) string { return "" }
	t.Cleanup(func() {
		disabledProviderReasonForTest = previous
	})
}
