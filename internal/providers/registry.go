package providers

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Meta describes a registered streaming provider.
type Meta struct {
	Name            string
	Aliases         []string
	Referrer        string
	DefaultDisabled bool
	DisableReason   string
	// OptOutToken excludes a provider from automatic fallback prompts, e.g. "no-animepahe".
	OptOutToken string
	// FallbackPrompt is shown when the host offers this provider as a fallback.
	FallbackPrompt string
}

type entry struct {
	meta    Meta
	factory func() Provider
}

var (
	registryMu sync.RWMutex
	registry   = map[string]entry{}
	aliases    = map[string]string{}
)

// Register adds a provider factory to the global registry.
func Register(meta Meta, factory func() Provider) {
	registryMu.Lock()
	defer registryMu.Unlock()

	meta.Name = strings.ToLower(strings.Trim(strings.TrimSpace(meta.Name), "\"'[]"))
	if meta.Name == "" {
		panic("providers.Register: Name is required")
	}
	if factory == nil {
		panic("providers.Register: factory is required")
	}

	meta.Name = strings.ReplaceAll(strings.ReplaceAll(meta.Name, "-", ""), " ", "")
	if meta.Name == "" {
		panic("providers.Register: Name is required")
	}

	registry[meta.Name] = entry{meta: meta, factory: factory}
	registerAlias(meta.Name, meta.Name)
	for _, alias := range meta.Aliases {
		registerAlias(alias, meta.Name)
	}
}

func registerAlias(alias, canonical string) {
	alias = strings.Trim(strings.TrimSpace(alias), "\"'[]")
	if alias == "" {
		return
	}
	aliases[strings.ToLower(strings.Join(strings.Fields(alias), " "))] = canonical
	compact := strings.ToLower(alias)
	compact = strings.ReplaceAll(compact, "-", "")
	compact = strings.ReplaceAll(compact, "_", "")
	compact = strings.ReplaceAll(compact, " ", "")
	if compact != "" {
		aliases[compact] = canonical
	}
}

// NormalizeName maps user/config input to a canonical provider name.
func NormalizeName(name string) string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return normalizeName(name)
}

func normalizeName(name string) string {
	name = strings.Trim(strings.TrimSpace(name), "\"'[]")
	if name == "" {
		return ""
	}
	key := strings.ToLower(strings.Join(strings.Fields(name), " "))
	if canonical, ok := aliases[key]; ok {
		return canonical
	}
	key = strings.ReplaceAll(key, "-", "")
	key = strings.ReplaceAll(key, "_", "")
	key = strings.ReplaceAll(key, " ", "")
	if canonical, ok := aliases[key]; ok {
		return canonical
	}
	return ""
}

// MetaFor returns registration metadata for a provider name.
func MetaFor(name string) (Meta, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()

	name = normalizeName(name)
	if name == "" {
		return Meta{}, false
	}
	entry, ok := registry[name]
	if !ok {
		return Meta{}, false
	}
	return entry.meta, true
}

// RegisteredNames returns sorted canonical provider names.
func RegisteredNames() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()

	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// New creates a provider instance by canonical name.
func New(name string) (Provider, error) {
	registryMu.RLock()
	defer registryMu.RUnlock()

	name = normalizeName(name)
	if name == "" {
		return nil, fmt.Errorf("unknown provider %q", name)
	}
	entry, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("unknown provider %q", name)
	}
	return entry.factory(), nil
}

// Referrer returns the stream referrer for a provider, if configured.
func Referrer(name string) string {
	meta, ok := MetaFor(name)
	if !ok {
		return ""
	}
	return meta.Referrer
}

// SetFactoryForTest overrides a provider factory for the duration of a test.
func SetFactoryForTest(name string, factory func() Provider) (restore func()) {
	registryMu.Lock()
	defer registryMu.Unlock()

	name = normalizeName(name)
	previous, ok := registry[name]
	if !ok {
		panic(fmt.Sprintf("providers.SetFactoryForTest: unknown provider %q", name))
	}

	registry[name] = entry{meta: previous.meta, factory: factory}
	return func() {
		registryMu.Lock()
		registry[name] = previous
		registryMu.Unlock()
	}
}
