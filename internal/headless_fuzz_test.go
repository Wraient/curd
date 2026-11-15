package internal

// Fuzz targets for the parsers that consume untrusted input: provider
// config strings (user-edited files), qualified provider IDs (history
// files and provider responses), and stream-link lists (provider CDNs).
// The invariant is never a panic, plus the documented output shape.
//
// Run briefly with, for example:
//
//	go test ./internal -run '^$' -fuzz FuzzParseProviderConfig -fuzztime 30s
//	go test ./internal -run '^$' -fuzz FuzzPrioritizeLink -fuzztime 30s

import (
	"strings"
	"testing"

	"github.com/wraient/curd/internal/providers"
)

func FuzzParseProviderConfig(f *testing.F) {
	for _, seed := range []string{
		"", "stacked", `["senshi","anipub"]`, "senshi,anipub", "senshi+animepahe",
		"no-animepahe", `["allanime","no-animepahe"]`, "[,", "💥", "\x00",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		names, _ := parseProviderConfig(raw)
		seen := map[string]bool{}
		for _, name := range names {
			if providers.NormalizeName(name) == "" {
				t.Fatalf("parseProviderConfig(%q) returned unregistered name %q", raw, name)
			}
			if seen[name] {
				t.Fatalf("parseProviderConfig(%q) returned duplicate %q in %v", raw, name, names)
			}
			seen[name] = true
		}
		// Formatting the result and re-parsing it must be stable.
		again, _ := parseProviderConfig(formatProviderConfigValue(names, false))
		if strings.Join(again, ",") != strings.Join(names, ",") && len(names) > 0 {
			t.Fatalf("config round-trip unstable: %q -> %v -> %v", raw, names, again)
		}
	})
}

func FuzzParseProviderQualifiedID(f *testing.F) {
	for _, seed := range []string{
		"", "52991", "senshi::52991", "::52991", "senshi::", "a::b::c", "::", "💥::💥",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		name, id, ok := ParseProviderQualifiedID(raw)
		if ok {
			if providers.NormalizeName(name) == "" || id == "" {
				t.Fatalf("ParseProviderQualifiedID(%q) = (%q, %q, true)", raw, name, id)
			}
			if rejoined := QualifyProviderID(name, id); rejoined == "" {
				t.Fatalf("QualifyProviderID(%q, %q) is empty", name, id)
			}
		}
		_ = QualifyProviderID(raw, raw)
		_ = QualifyProviderID("senshi", raw)
	})
}

func FuzzPrioritizeLink(f *testing.F) {
	f.Add("https://video.wixstatic.com/1080p.m3u8", "https://cdn.example/720p.m3u8")
	f.Add("", "")
	f.Add("💥", "not a url at all")
	f.Fuzz(func(t *testing.T, a, b string) {
		links := []string{a, b}
		got := PrioritizeLink(links)
		if got != a && got != b {
			t.Fatalf("PrioritizeLink(%q) = %q, not an input", links, got)
		}
		if got := PrioritizeLink(nil); got != "" {
			t.Fatalf("PrioritizeLink(nil) = %q", got)
		}
		_ = PrioritizeLink([]string{a})
		_ = PrioritizeLink([]string{b, a, b, a})
	})
}
