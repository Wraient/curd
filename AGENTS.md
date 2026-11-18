# curd provider guide (for agents)

Read this before adding or changing a streaming provider. It distills three
provider integrations (anilight, animex, aniwaves), the multi-axis matching
work, and two adversarial audits. Follow it exactly; the audit will check.

## 1. Package layout (mechanical)

New provider = new package `internal/providers/<name>/` with these files:

- `client.go` — base URLs, request builders, shared fetch helpers, auth/token cache
- `search.go` — `SearchAnime`, result parsing
- `episodes.go` — `EpisodesList`, per-episode flags
- `streams.go` — `GetEpisodeStreamsForMode` (+ hints), stream validation
- `provider.go` — `Provider` struct implementing `Provider` (+ `HintResolver`)
- `register.go` — `init()` registration
- `types.go` — `SearchItem` + wire-format structs
- `provider_test.go` — offline fixture tests (httptest, never live)
- `live_test.go` — `CURD_LIVE_<NAME>=1` gated end-to-end test

Implement the interface in [types.go](/home/wraient/Projects/curd/internal/providers/types.go):
`Name()`, `SearchAnime(query, mode)`, `EpisodesList(showID, mode)`,
`GetEpisodeURL(...)`. Always also implement `HintResolver`
(`GetEpisodeURLForModeWithHints`) so mpv gets `Referrer`/`Subtitle` hints.

Hard rules:

- Providers must NOT import `internal` (import cycle). Use `curdhost`
  (HTTP client, status helpers), `providers` (types), `substyle`, `hlsverify`.
- Shared HTTP client timeout is 15s; stacked search gives each provider
  15s (`providerSearchTimeout` in [provider.go](/home/wraient/Projects/curd/internal/provider.go)).
  Never add sleeps/retries that can exceed it.
- Cap every body read (`io.LimitReader`, 1–4 MiB). Never read unbounded bodies.
- `mode` is `sub` or `dub` (use `providers.NormalizeTranslationType`).
  `epNo <= 0` and empty IDs fail fast client-side, before any request.

## 2. Matching: expose every metadata field you get

The matcher ([provider.go](/home/wraient/Projects/curd/internal/provider.go) `providerFactsFromExtra`,
~line 1182) consumes ONLY what `SearchAnime` returns in each
`SelectionOption`. If you do not expose a field, matching cannot use it.
Populate all of these from the search response itself — no extra detail
round-trips:

| `SearchItem` field | Used for | Source examples |
|---|---|---|
| MAL ID (`MalID`) | exact-ID match + veto (strongest signal) | anilight `idMal`, animex `malId` |
| AniList ID | exact match | anilight `anilistId`, animex `anilistId` |
| Native title (`Romaji`/`Native`/`JapaneseTitle`) | alt-title matching vs AniList romaji | anikoto `data-jp`, aniwaves `data-jp` |
| `Type`/`Format` | TV vs movie vs OVA disambiguation | `TV`, `MOVIE`, `OVA`, `ONA` |
| `Episodes` | episode-count match/veto vs tracker | `episodeCount`, `episodes` |
| `Year` | year tie-break in labels | `seasonYear` |

Plus on the option itself:

- **`Thumbnail`: always populate it.** It renders picker artwork AND feeds
  thumbnail matching (AniList `bx<ID>` exact-token match, MAL poster match in
  [provider.go](/home/wraient/Projects/curd/internal/provider.go) and
  [provider_mapping.go](/home/wraient/Projects/curd/internal/provider_mapping.go)).
  Empty thumbnails (our animex audit finding) silently degrade both UX and
  accuracy. If the API field is a JSON scalar (animex `coverImage` →
  `{extraLarge, large, medium}`), parse the object; prefer the largest variant.
  When the schema is unknown, introspect it live (`__typename`, then
  `__type(name:...)`) — never guess field names.
- **`Label`: `Title · Type · Year · N eps`** (e.g. `Frieren · TV · 2023 · 28 eps`).
  The picker and the benchmark both rely on this shape.
- **`Key`: the stable provider show id** (slug path segment, never a full URL).
- **`ExtraData`: your `SearchItem`.** Then add your case to
  `providerFactsFromExtra` in [provider.go](/home/wraient/Projects/curd/internal/provider.go)
  mapping it to `{malID, episodes, typ, altTitles}` — a provider missing from
  that switch is invisible to identity matching.

Episode lists: parse flags **per episode element** (aniwaves lesson: the
`data-num`/`data-sub`/`data-dub` attributes live on the inner `<a>`, not the
`<li>` — index-shifted or shared flags silently mistag episodes). Missing-flag
elements default to absent, never to the previous episode's value.

## 3. Speed rules

- Search fan-out is parallel across the stack; keep your search to the
  minimum requests the site needs (FastSearch-style single GraphQL call good,
  search-then-N-detail-calls bad).
- Auth handshakes are cached (anilight `verifyResponse` token until
  `expiresAt-5min`). Never handshake per episode.
- **Never hold a mutex across network I/O** (anilight audit finding): check
  the cache under lock, release, fetch, re-lock to store.
- Reuse `curdhost.HTTPClient()`; never construct clients with their own
  timeouts. Spoof a browser UA + site `Referer` — several CDNs reject Go's
  default UA.
- Validate streams cheaply: HLS → `hlsverify.Validate` (playlist + one
  segment check); mp4 → single `Range: bytes=0-1` HEAD-style probe expecting
  200/206 (`probeStreamHead`). Never download full segments to "check".

## 4. Accuracy / robustness rules (audit-enforced)

- **Reject explicit error envelopes.** If the API returns a status field
  (aniwaves ajax `status`), fail on non-200 — but tolerate a *missing* field
  (some endpoints omit it on success) so you fail only on explicit errors.
- **Recover from stale auth.** If the stream endpoint rejects a cached token
  (401 / token-complaint body), clear the cache, handshake once, retry the
  lookup exactly once — then surface the error. Never fail until cache expiry.
- **Fail closed on URL mapping.** Non-http(s) tokens resolve only under the
  provider's known CDN namespace; anything else is an error, never passed to mpv.
- **Degrade channels explicitly via `substyle`:** no hardsub channel →
  hard preference degrades to soft (`substyle.Choose`), never errors.
- **Fixture fidelity (offline tests must mirror the real wire):**
  - Assert request params the real API requires (`id`/`epNum`/`type`/
    `providerId`; `slug`/`episode`/`language`; session header/cookie).
    Return 400/401/403 when they are missing so a dropped param fails the test.
  - Serve **distinct sub and dub responses** (different playlist/file/embed)
    and test the dub path end to end — a dub test that resolves the sub
    stream passes vacuously.
  - Mirror quirks: escaped JSON-in-JSON, HTML-in-JSON, cookie-setting embed
    pages (`SETCOOKIE` transport marker in the contract test).
- Movie support: verify movies resolve too (`CURD_LIVE_MOVIES=1` suite uses
  the same `edgeProviders()` list — register new providers there).

## 5. UI/UX rules (picker and errors)

- Options must be distinguishable at a glance: full `Label` (rule above) +
  real `Thumbnail`. No blank artwork, no bare titles.
- Respect `SubOrDub` end to end: dub request → dub lookup → dub stream, no
  silent sub fallback (except the explicit hard→soft substyle rule).
- Errors must name the provider, episode, and mode
  (`"aniwaves has no dub servers for episode 5"`, never `"not found"`), so the
  host can show per-provider failure notes and offer: search again / try other
  audio / change episode / back.
- Never panic on upstream garbage (HTML error pages, truncated JSON, empty
  bodies) — error or empty results, covered by `TestProviderContractEdgeCases`.

## 6. Wiring checklist (every new provider)

1. Blank import in [loadproviders/load.go](/home/wraient/Projects/curd/internal/loadproviders/load.go)
   (alphabetical).
2. Stack order: default stack lives in the provider-stack test
   ([stackcheck_test.go](/home/wraient/Projects/curd/internal/stackcheck_test.go)
   `TestDefaultStackIs...`) — fastest + most complete first; tail providers
   known-flaky stay last. Update the test name/expectation.
3. `internal/provider_disabled.go` if the provider needs a kill-switch entry.
4. Contract test: fixture in
   [contract_offline_test.go](/home/wraient/Projects/curd/internal/providers/contract_offline_test.go)
   + sub AND dub entries in `contractCases` (add a `provider` field use —
   see existing `-dub` cases).
5. Edge suite: register in `edgeProviders()` in
   [provider_edge_live_test.go](/home/wraient/Projects/curd/internal/providers/provider_edge_live_test.go).
6. Docs: `docs/providers.md` + root `README.md` provider lists.

## 7. Verification gates (all green before release)

| Gate | Command |
|---|---|
| Build + vet | `go build ./...`, `go vet ./...` (zero warnings) |
| Offline suite | `go test ./...` (includes contract + edge-case offline tests) |
| Live provider flow | `CURD_LIVE_<NAME>=1 go test ./internal/providers/<name> -run TestLive<Name>ProviderFlow -v` (sub + dub) |
| Live contract | `CURD_LIVE_CONTRACT=1 go test ./internal/providers -run TestLiveProviderContract` (infra outages in tail providers are logged, not gating, but new providers must pass) |
| Hard benchmark | `CURD_LIVE_HARD=1 go test ./internal -run TestHardAnimeBenchmark` — 27 hard titles; **0 wrong picks** required, WEAK allowed only when the correct show ranks top-3 (picker-correct) |
| Edge suite | `CURD_LIVE_EDGE=1 ... TestLiveProviderEdgeCases`, `CURD_LIVE_MOVIES=1 ... TestLiveProviderMovies` after wiring step 5 |
| Adversarial audit | delegate a review-role audit of the full diff; fix **every** finding with evidence; needs explicit `APPROVED` |
| Release | empty commit `release: vX.Y.Z`, push, CI publishes binaries (see `git log --grep=release`); never ship with a red gate |

Parked, do not re-attempt without new info: gotaku (encrypted binary
protocol), reanime (flixcloud PBKDF2+XOR crypto) — documented in
`docs/providers.md` as future work.
