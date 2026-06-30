# Phase 4 — Config, Setup, Wiring the Composite

**Ticket:** N/A
**Branch:** feat/p4-config-wizard-composite
**Author:** Oscar Larsson
**Created:** 2026-06-30

## Goal

A user can configure Azure DevOps, GitHub, or both; `main.go` builds the matching
backends, wraps them in a `CompositeProvider`, and the three views render a merged
list — with zero behavior change for today's Azure-only user.

## Constraints

- **Zero change for Azure-only users.** The always-wrap composite must be transparent:
  one backend in → identical results out (merge+sort is idempotent over already-sorted data).
- Builds on Phases 0–3: neutral types, `ListOpts`, `display` glyphs, `Terms`, the dormant
  `internal/github` package and `KeyringStore.GetGitHubToken`/`SetGitHubToken` all exist.
- **stdlib only**, TDD-first, `gofmt -l` clean, `CGO_ENABLED=0`; integration paths manual.
- Honor conventions: scope-first Provider methods (#2), lowercase snake_case config keys (#9),
  gofmt gate (#10), `<= 0` id guards (#11).

## Scope

**In scope:**
- `CompositeProvider` in `internal/provider` (fan-out list + scope-routed detail/mutation/URL).
- `Scopes() []string` added to `provider.Provider`; both adapters implement it.
- Config `github` section (repos, label-convention prefixes) + relaxed validation.
- `main.go` backend assembly; metrics-tab gating on a live Azure backend.
- `azdo auth` provider prompt; setup-wizard provider selection (Azure / GitHub / both).

**Out of scope:**
- Metrics for GitHub (Azure-only, ADR decision 4); Projects v2; GitHub Checks.
- Any view/render change — the merged-list UI already exists from Phase 2.

## Approach

Generalize the proven `MultiClient` fan-out one level up: a `CompositeProvider` holds N
`provider.Provider` backends, fans list calls out (goroutine per backend, merge, sort by
date, `PartialError` on partial failure), and routes detail/mutation/URL calls via a
scope→backend index built from each backend's `Scopes()`. `main.go` constructs an Azure
and/or GitHub backend from config and always wraps them. Config gains an optional `github`
section; `Validate` now requires *at least one* backend. The wizard and `azdo auth` grow a
provider choice so a GitHub-only user can complete first-run setup.

## Decisions

| # | Question | Decision | Rationale |
|---|----------|----------|-----------|
| 1 | Single-backend path? | Always wrap in `CompositeProvider` | Uniform path; transparent because single-backend merge/sort is idempotent |
| 2 | Detail-call routing? | scope→backend index from a new `Scopes()` on the Provider iface | No scope-enumeration existed; both MultiClients already hold the data (Projects/Scopes) |
| 3 | Scope collision across backends? | First-registered backend wins; documented | GitHub scopes are `owner/repo` (slash) — real collision with an Azure project is near-impossible |
| 4 | Composite `Kind()`? | Sole backend's Kind; mixed → first backend's | No consumer reads `client.Kind()` for rendering — per-row `Identity.Kind` drives glyphs |
| 5 | Validation when GitHub-only? | Require ≥1 backend (Azure org+projects OR GitHub repos); both may coexist | Azure can no longer be mandatory |
| 6 | GitHub CLI auth? | `azdo auth` gains an Azure/GitHub prompt → existing `SetGitHubToken` | Single entry point; reuses Phase 3 keyring path + `GITHUB_TOKEN` fallback |
| 7 | Label-prefix config? | Config-only (`github.type_prefix`/`priority_prefix`); wizard uses defaults | Keeps the wizard short; power users edit config |
| 8 | Wizard reach? | Front provider selector (Azure / GitHub / both); GitHub-only first-run supported | Config now supports GitHub-only, so first-run must too |
| 9 | Metrics tab when no Azure? | Gate on live Azure backend (`mc != nil`) AND `metrics.enabled` | ADR decision 4 — tab hides without Azure data |

## Tasks

- [x] 1. Add `Scopes() []string` to `provider.Provider`; implement on `azdevops.Adapter` (via `Projects()`) and `github.Adapter` (via `Scopes()`); update the conformance stub; table-test both.
- [x] 2. `CompositeProvider` in `internal/provider`: holds `[]Provider`, fans out every list method (goroutine/merge/sort-by-date/`PartialError`), routes detail/mutation/URL by a scope→backend index, `Kind()`/`IsMultiProject()`/`Scopes()` over the union. Stub-backend unit tests + `var _ Provider` gate. (blocked by: 1)
- [x] 3. Config: add `GitHubConfig{Repos, TypePrefix, PriorityPrefix}` under `github`; Load/Save round-trip (lowercase keys); relax `Validate` to require ≥1 backend, Azure optional when GitHub present. Tests incl. GitHub-only and both. (blocked by: none)
- [x] 4. `main.go`: build an Azure backend (PAT) and/or GitHub backend (token + `LabelConvention` from config) per config, wrap all in `CompositeProvider`, pass it + the concrete `*azdevops.MultiClient` (nil if no Azure) to `NewModel`. (blocked by: 2,3)
- [x] 5. Gate the metrics tab: `buildEnabledTabs` + `NewModel` add `TabMetrics`/build `metricsView` only when an Azure backend is present (`mc != nil`) AND `metrics.enabled`. Test the no-Azure path. (blocked by: 3)
- [ ] 6. `azdo auth`: add an Azure/GitHub provider prompt; GitHub branch stores via `SetGitHubToken`; bare Azure flow output unchanged. (blocked by: 3)
- [ ] 7. Setup wizard: front provider selector (Azure / GitHub / both); GitHub steps (token, repos); build a `Config` (and store the GitHub token) for any selection incl. GitHub-only; tests. (blocked by: 3)

## Unknowns

- Whether `Scopes()` should also surface display names, or stay API-name-only (lean: API-name-only;
  display routing already lives in each MultiClient's `DisplayNameFor`).
- GitHub token entry inside the wizard: store immediately to keyring on confirm vs. hand back to
  `main.go` to persist (lean: wizard stores it, mirroring how it saves config).
