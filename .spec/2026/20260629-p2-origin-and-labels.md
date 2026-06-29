# Phase 2 — Per-Row Provider Origin + Configurable Labels

**Ticket:** N/A
**Branch:** feature/gh-phase-2
**Author:** Oscar Larsson
**Created:** 2026-06-29

## Goal

List rows carry their origin `Kind` and show a provider glyph **only when a list
mixes Kinds**; tab/term labels flow from a new user-configurable `Terms` config
(provider-blind, never branched on `Kind()`); and the status bar shows **all**
active scopes together — with no visible change for an Azure-only user beyond the
multi-scope status bar.

## Constraints

- Azure stays the only backend at runtime. The per-row glyph never appears in a
  single-provider config, so row rendering is unchanged for today's users.
- **No `Kind()`-based branching of tab names** (ADR decision 3). Labels resolve
  from config + defaults, identically regardless of provider.
- Builds on Phase 0/1 — `Identity` already carries `Kind`/`Scope`/`ScopeDisplay`;
  `display` already owns the glyph maps; `config` already has the project
  `DisplayNames` pattern to mirror.
- TDD: glyph map, mixed-kind detection, config parsing, and label/status-bar
  rendering get tests first, then made green.
- Build/test with `CGO_ENABLED=0` (sandbox blocks cgo + `/tmp`).

## Scope

**In scope:**
- `KindGlyph`/`KindLabel` in `internal/ui/display`; a leading glyph column added to
  the row builders, gated on a list spanning >1 distinct `Kind`.
- A new `Terms map[string]string` config field (+ parser, `TermFor()` accessor,
  defaults); tab bar reads labels from it instead of the hard-coded map.
- Status bar renders all active scopes together (not one project / a count).

**Out of scope:**
- GitHub backend (Phase 3) and `CompositeProvider` (Phase 4) — mixed-kind paths are
  tested with synthetic data only; no second backend ships here.
- Setup-wizard prompts for `Terms` (Phase 4) — config field + parsing only now.
- Column-header / in-view term overrides — tab labels only this phase.
- Metrics view rendering — untouched.

## Approach

Add `KindGlyph`/`KindLabel` beside the existing display maps. Derive "mixed" from
the rendered items' `Identity.Kind` set (not config — survives the Phase 4
composite); when mixed, prepend a small glyph column in each view's row builder.
Add a `Terms` map to config mirroring the `DisplayNames` parse/accessor pattern but
in its own namespace; route it into `app` so `renderTabBar` resolves each tab label
via `TermFor(key, default)`. Replace the status bar's single `project` field with a
scopes slice rendered as a joined, truncated list.

## Decisions

| # | Question | Decision | Rationale |
|---|----------|----------|-----------|
| 1 | When does the per-row glyph show? | Only when a list spans >1 distinct `Kind` | Mirrors the "Project" column (multi-project only); zero noise in Azure-only |
| 2 | How is "mixed" determined? | From the rendered items' `Identity.Kind` set | Works once the Phase 4 composite merges backends; no config coupling |
| 3 | Where does `KindGlyph` live? | `internal/ui/display` | Beside the other enum→glyph maps; theming stays in the UI |
| 4 | Reuse `DisplayNames` for tab labels? | No — new `Terms map[string]string` | Avoids key collision between a project name and a tab override; same pattern, own namespace |
| 5 | How far does Phase 2 take labels? | Config field + parsing + config-driven rendering now; wizard prompts deferred to Phase 4 | Labels are genuinely overridable now without enlarging the wizard scope |
| 6 | Multi-scope status bar format when many scopes? | Join scope names, truncate past a small cap with `+N more` | Keeps the bar single-line; exact cap/separator is an implementation call |

## Tasks

- [x] 1. Add `KindGlyph(Kind)` and `KindLabel(Kind)` to `internal/ui/display` (Azure mark for `KindAzure`, placeholder for a future `KindGitHub`); table-test both.
- [x] 2. Add a `mixedKinds(items)` helper (true iff >1 distinct `Identity.Kind`); unit-test single-kind→false, multi-kind→true, empty→false. (blocked by: 1)
- [x] 3. Wire a leading glyph column into the PR/work-item/pipeline row builders, shown only when `mixedKinds` is true; test with synthetic multi-kind rows that the glyph column appears, and is absent for Azure-only rows. (blocked by: 2)
- [ ] 4. Add `Terms map[string]string` to config: parse it, add `TermFor(key, fallback string) string`, and round-trip it in `Save()`; test parse + fallback + persistence.
- [ ] 5. Route `Terms` into `app`; make `renderTabBar` resolve each tab label via `TermFor` with the current strings as defaults (no `Kind()` branch); test that defaults render unchanged and an override replaces the label. (blocked by: 4)
- [ ] 6. Replace the status bar's single `project` with active **scopes**: add `SetScopes([]string)`, render them joined+truncated, and update the app init + theme-change call sites; test single-scope, multi-scope, and truncation. (blocked by: none)
- [ ] 7. Verify: grep `internal/app` confirms no tab label is branched on `Kind()`; `CGO_ENABLED=0 go test ./...` and `go vet ./...` clean; manual smoke of all three tabs (glyph absent, labels/status-bar correct). (blocked by: 3,5,6)

## Unknowns

- Status-bar scope cap/separator, and whether org renders once or per scope.
- Final `Terms` key spelling (`work_items` / `pipelines` / …) and case — settle in task 5.
- Glyph as its own column vs a prefix on the status cell — decide visually in task 3.
