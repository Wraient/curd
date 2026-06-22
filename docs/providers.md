# Curd streaming providers

Curd separates **what you watch** (AniList / local history) from **where streams come from** (providers). Providers are compile-time modules that implement a small interface and register themselves at startup. The host application handles menus, tracking, mpv playback, and provider stack fallback — providers only answer search, episode list, and stream URL questions.

This document explains the intent of the design, how to add a provider, how users enable or disable providers, and what to do when a new provider needs something the current contract does not cover.

---

## Design intent

### Goals

1. **Adding a provider should be mechanical** — new package, implement three methods, register in `init()`, add one import line. No edits to central maps, name switches, or `curd.go` provider-specific branches.
2. **Disabling a broken provider should not require a release** — users can turn providers off in `curd.conf` without rebuilding.
3. **Keep one binary** — providers ship inside the curd repo (or as Go packages imported at build time). We are not using runtime plugin binaries or `.so` loading.
4. **Host owns orchestration** — ordered fallback, sub/dub prompts, AniList → provider matching, and mpv IPC stay in `internal/`. Providers return data; they do not drive the watch loop.

### Non-goals (for now)

- Installable third-party plugin binaries
- Providers replacing AniList search or the TUI/Rofi selector
- Providers running arbitrary host code or reading OAuth tokens

### Architecture (high level)

```
┌─────────────────────────────────────────────────────────┐
│  internal/ (host)                                      │
│  SetupCurd · provider stack · mpv · tracking · config   │
│       │ uses providers.New() + adapter                   │
│       │ wires curdhost.* hooks for HTTP, log, storage    │
└───────┼─────────────────────────────────────────────────┘
        │
        ▼
┌─────────────────────────────────────────────────────────┐
│  internal/providers/                                     │
│  registry · types · Provider interface                   │
├─────────────────────────────────────────────────────────┤
│  allanime/   animepahe/   yourprovider/                │
│  search · episodes · streams · register.go               │
└─────────────────────────────────────────────────────────┘
```

Providers **must not** import `internal` (that would create an import cycle). They use `curdhost` for shared services and `providers` for types and registration.

---

## Provider contract

### Required interface

Defined in `internal/providers/types.go`:

```go
type Provider interface {
    Name() string
    SearchAnime(query, mode string) ([]SelectionOption, error)
    EpisodesList(showID, mode string) ([]string, error)
    GetEpisodeURL(config PlaybackConfig, id string, epNo int) ([]string, error)
}
```

| Method | Purpose |
|--------|---------|
| `Name()` | Canonical provider id (e.g. `allanime`). Must match `Meta.Name` from registration. |
| `SearchAnime` | Find shows on the streaming site. `mode` is `sub` or `dub` where relevant. |
| `EpisodesList` | Return episode numbers as strings for a provider show id. |
| `GetEpisodeURL` | Resolve playable URLs for episode `epNo` (1-based, aligned with AniList progress). |

### Shared types

```go
type SelectionOption struct {
    Key       string  // provider show id (stored in history / ProviderId)
    Label     string  // menu line
    Title     string  // plain title for matching
    Thumbnail string  // optional cover URL
    ExtraData any     // optional provider-specific payload (e.g. metadata for matching)
}

type PlaybackConfig struct {
    SubOrDub string // "sub" or "dub"
}
```

Use `providers.NormalizeTranslationType(mode)` and `providers.AlternateTranslationType(mode)` from `internal/providers/lang.go` for sub/dub handling.

### Optional interfaces

Implement only when the default behavior is not enough:

| Interface | When to implement |
|-----------|-------------------|
| `ModeResolver` | Sub and dub use different APIs or URLs; host calls `GetEpisodeURLForMode` explicitly during fallback. |
| `HintResolver` | Streams need per-URL mpv metadata (Referer header, external subtitle URL). Used by AllAnime. |
| `IDResolver` | Stored show ids go stale and must be refreshed via search (Animepahe sessions). |

The host discovers these via type assertion on the registered provider — no extra registration step.

### Show IDs and the provider stack

- Configured stack: `Provider = ["allanime", "animepahe"]` — tried left to right for search and playback.
- Qualified ids in menus when multiple providers are active: `allanime::ShowIdHere`.
- Host qualifies ids with `providername::id` when merging stacked search results.

Your `Key` in `SearchAnime` should be the **raw** provider id. The host adds the `provider::` prefix when needed.

---

## Adding a new provider

### 1. Create the package

```
internal/providers/yoursite/
  register.go    # providers.Register in init()
  provider.go    # Provider struct + interface methods
  search.go      # HTTP / API search (optional split)
  episodes.go
  streams.go
  provider_test.go
```

Use `allanime` as a reference for a GraphQL/REST provider, `animepahe` for cookie/browser-heavy sites.

### 2. Implement `Provider`

```go
package yoursite

import "github.com/wraient/curd/internal/providers"

type Provider struct{}

func (p *Provider) Name() string { return "yoursite" }

func (p *Provider) SearchAnime(query, mode string) ([]providers.SelectionOption, error) {
    // ...
}

func (p *Provider) EpisodesList(showID, mode string) ([]string, error) {
    // ...
}

func (p *Provider) GetEpisodeURL(config providers.PlaybackConfig, id string, epNo int) ([]string, error) {
    // ...
}
```

### 3. Register in `init()`

```go
func init() {
    providers.Register(providers.Meta{
        Name:     "yoursite",
        Aliases:  []string{"ys"},           // optional config/menu aliases
        Referrer: "https://yoursite.example/", // mpv Referer for streams
    }, func() providers.Provider {
        return &Provider{}
    })
}
```

`Meta` fields:

| Field | Meaning |
|-------|---------|
| `Name` | Canonical name (lowercase, no spaces; normalized to compact form e.g. `allanime`). |
| `Aliases` | Alternate names accepted in config. |
| `Referrer` | Default HTTP Referer for mpv when playing this provider's links. |
| `DefaultDisabled` | If true, provider is off until user enables it (see Animepahe). |
| `DisableReason` | Shown when a disabled provider is requested. |
| `OptOutToken` | Config token to permanently skip fallback prompts (e.g. `no-animepahe`). |
| `FallbackPrompt` | Reserved for host fallback UX (Animepahe chromium warning). |

### 4. Wire the package into the binary

Add a blank import in `internal/loadproviders/load.go`:

```go
import (
    _ "github.com/wraient/curd/internal/providers/allanime"
    _ "github.com/wraient/curd/internal/providers/animepahe"
    _ "github.com/wraient/curd/internal/providers/yoursite"
)
```

`internal` already imports `loadproviders` from `provider_bridge.go`, so registration runs on startup.

### 5. Use `curdhost` for host services

Providers must not call `internal` helpers directly. Use hooks in `internal/curdhost/host.go`:

| Hook | Use |
|------|-----|
| `curdhost.HTTPClient()` | Shared cookie jar HTTP client |
| `curdhost.Log(string)` | Debug log (`debug.log` in storage path) |
| `curdhost.Out(string)` | User-visible terminal message |
| `curdhost.StoragePath()` | `~/.local/share/curd` (or configured path) |
| `curdhost.AnimeNameLanguage()` | `"english"` or `"romaji"` for search result labels |
| `curdhost.HTTPStatusOK` / `HTTPStatusError` | Consistent HTTP error formatting |

Example:

```go
resp, err := curdhost.HTTPClient().Do(req)
if err != nil {
    return nil, err
}
body, _ := io.ReadAll(resp.Body)
resp.Body.Close()
if !curdhost.HTTPStatusOK(resp.StatusCode) {
    return nil, curdhost.HTTPStatusError("yoursite search", resp.StatusCode, body)
}
```

### 6. Tests

- Unit tests live in the provider package (`httptest` transport, no live site).
- Use `curdhost` hooks in test setup (see `animepahe/provider_test.go`).
- Host-level stack tests use `providers.SetFactoryForTest` via `withProviderFactories` in `provider_stack_test.go`.
- Live tests: gate behind env vars (e.g. `CURD_LIVE_ALLANIME_TEST=1`).

### 7. User configuration

Users add the provider to their stack in `~/.config/curd/curd.conf`:

```ini
Provider=["yoursite"]
# or fallback stack:
Provider=["yoursite","allanime"]
```

---

## Enabling and disabling providers

Disable order (first match wins):

1. **Test override** — `withAllProvidersEnabledForTest` in tests only.
2. **`DisabledProviders` in config** — runtime kill switch, no rebuild:

   ```ini
   DisabledProviders=["animepahe","yoursite"]
   ```

3. **`DefaultDisabled` in `Meta`** — compile-time default (Animepahe ships disabled with a reason).

To ship a provider that is **off by default** but opt-in capable, set `DefaultDisabled: true` and a clear `DisableReason`. Users remove it from `DisabledProviders` or we can add a menu entry later to toggle it.

---

## When a provider needs something new

Follow this order — prefer extending the contract over special-casing in `curd.go`.

### A. Needs fit an optional interface

| Need | Action |
|------|--------|
| Explicit sub/dub resolution | Implement `ModeResolver` |
| Per-stream Referer / subtitles | Implement `HintResolver` |
| Stale show id refresh | Implement `IDResolver` |

No registry or host changes required beyond what the interface already supports.

### B. Needs a new `Meta` field

Examples: capability flags (`RequiresBrowser: true`), max quality, consent text.

1. Add the field to `providers.Meta`.
2. Teach the host to read it via `providers.MetaFor(name)` (config UI, fallback prompts, doctor command).
3. Do **not** hardcode the provider name in `curd.go`.

### C. Needs a new host hook (in `curdhost`)

Examples: headless browser factory, shared rate limiter, proxy setting.

1. Add a function variable to `internal/curdhost/host.go`.
2. Wire it in `internal/provider_bridge.go` `init()` from existing `internal` infrastructure.
3. Document it in this file.
4. Use it only from provider packages that need it.

Avoid importing `internal` from providers.

### D. Needs new `PlaybackConfig` fields

If every provider needs a new playback preference (e.g. subtitle language on resolve):

1. Add the field to `providers.PlaybackConfig`.
2. Map it in `toPlaybackConfig()` in `provider_bridge.go`.
3. Update existing providers if the field is required; otherwise ignore in providers that do not care.

### E. Needs AniList matching behavior

Prefer putting provider-specific match data in `SelectionOption.ExtraData` and scoring in the host (see `scoreProviderSearchOption` in `provider.go`).

If matching is truly provider-specific and complex, implement `IDResolver` or export small helpers from your provider package (as `animepahe.ParseProviderID` does) and call them from the host through interfaces — **never** `if providerName == "yoursite"` in `curd.go`.

### F. Needs runtime install without rebuild

That is outside the current model. Options for a future iteration:

- External JSON/script providers invoked via subprocess
- Separate Go module imported at build time (`go install` with build tags)

Document the decision in a short ADR before building. The compile-time registry is intentional for reliability and packaging.

---

## Checklist for a new provider PR

- [ ] Package under `internal/providers/<name>/`
- [ ] `register.go` with `providers.Register` and complete `Meta`
- [ ] Blank import added in `internal/loadproviders/load.go`
- [ ] Uses `curdhost` only (no `internal` import)
- [ ] `provider_test.go` with HTTP mocks
- [ ] Episode numbers compatible with AniList 1-based progress
- [ ] Stream URLs return formats mpv can play (m3u8, mp4, etc.)
- [ ] `Referrer` set correctly if the CDN checks Referer
- [ ] No provider-specific branches added to `curd.go`
- [ ] README or this doc updated if new config keys or hooks were added

---

## Reference implementations

| Provider | Package | Notes |
|----------|---------|-------|
| AllAnime | `internal/providers/allanime` | GraphQL search/episodes, parallel stream resolution, `HintResolver` |
| Animepahe | `internal/providers/animepahe` | DDoS-Guard + rod browser, `IDResolver`, `DefaultDisabled`, `OptOutToken` |

Key host files:

| File | Role |
|------|------|
| `internal/providers/registry.go` | Registration and lookup |
| `internal/providers/types.go` | Interfaces and DTOs |
| `internal/loadproviders/load.go` | Built-in provider imports |
| `internal/provider_bridge.go` | Host hooks + internal adapter |
| `internal/provider.go` | Stack, search, resolve orchestration |
| `internal/provider_disabled.go` | Enable/disable logic |

---

## FAQ

**Why not dynamic plugins?**  
Curd is a single-session CLI binary. Compile-time modules give fast startup, simple packaging (AUR, nix, releases), and easy debugging. Config-based disable covers “site is broken right now” without a plugin marketplace.

**Can providers live in another repo?**  
Yes, as a Go module that imports `github.com/wraient/curd/internal/providers` and `curdhost`, calls `Register` in `init()`, and is blank-imported from a fork or custom `loadproviders` package. Same binary model, different import path.

**What if episode URL resolution is slow?**  
That is expected for some sites. Do heavy work inside the provider (parallel HTTP, caching cookies on disk under `curdhost.StoragePath()`). The host already prefetches the next episode in a goroutine during playback.

**Who owns breaking site changes?**  
The provider package. Fix the provider, release curd. Users can disable a broken provider with `DisabledProviders` until a fix ships.
