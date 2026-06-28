# Phase 0 — Neutral Domain + Provider Interface

**Ticket:** N/A
**Branch:** refact/p0-provider-iface
**Author:** Oscar Larsson
**Created:** 2026-06-28

## Goal

The whole app depends on a `provider.Provider` interface instead of the
concrete `*azdevops.MultiClient`, with zero user-visible behavior change.

## Constraints

- **No behavior change.** Every existing test stays green; the TUI looks and acts identically.
- Azure remains the only backend at runtime — this phase ships no GitHub code.
- TDD: write the conformance/mapping tests first, watch them fail, then make them green.
- Keep wire-format concerns (`System.*` tags, WIQL, vote ints) inside `internal/azdevops`.
- **Identity is provider-stamped, always.** `(Kind, Scope, ID)` is populated *only* at the adapter mapping boundary; no app/UI/view code ever constructs, defaults, or mutates it. Every neutral entity returned from the interface must carry all three as non-zero values.

## Scope

**In scope:**
- New `internal/provider` package: neutral domain types + `Provider` interface.
- An azdevops adapter that maps wire types → neutral types and satisfies `Provider`.
- Re-typing `app.Model` and all views to depend on `provider.Provider`.

**Out of scope:**
- The metrics view — stays on a concrete `*azdevops.MultiClient` (see decision 5), untouched.
- `CompositeProvider` / merged backends (Phase 4), GitHub backend (Phase 3).
- Sealing remaining leaks: URL builders, enum display metadata, `ListOpts` (Phase 1).
- Any config, wizard, or auth changes.

## Approach

Define neutral types carrying `(Kind, Scope, ID)` identity from the start so the
later GitHub merge needs no retrofit. Build an adapter in `internal/azdevops`
(or `internal/azdevops/provider`) that translates existing wire structs to
neutral types and implements the interface — keeping `MultiClient` untouched.
Re-point `app.NewModel` and each view's `NewModelWithStyles` at the interface,
migrating view code from concrete azdevops types to neutral ones. Validate by
running the full suite plus a manual smoke of all three tabs.

## Decisions

| # | Question | Decision | Rationale |
|---|----------|----------|-----------|
| 1 | Adapter vs rename to satisfy interface? | Adapter that maps wire→neutral | Isolates wire concerns; GitHub backend mirrors the same shape |
| 2 | Bake `(Kind, Scope, ID)` identity now? | Yes | Avoids a painful retrofit when the merged list lands |
| 3 | Build `CompositeProvider` here? | No, defer to Phase 4 | Keep this phase a pure, shippable refactor |
| 4 | Move display helpers (StateIcon, etc.)? | No, leave in place for now | Display-metadata cleanup is Phase 1 — keep scope tight |
| 5 | Does metrics move to the interface? | No — keep it on a nullable concrete `*azdevops.MultiClient` | Its calls (`MetricsWorkItems`, `WorkItemUpdates`, `GetOrg`) are Azure- and metrics-only; `nil` client already hides the tab, matching "metrics is Azure-only" |
| 6 | Right shape for `WebURL`? | Per-entity methods: `WorkItemURL(id int)`, `PRURL(repositoryID string, prID int)`, `PipelineURL(id int)` | Avoids type-unsafe `any` parameter; each entity has different identifying fields; empty string return signals "cannot construct URL" |

## Tasks

- [x] 1. Create `internal/provider` with neutral domain types (WorkItem, PullRequest, PipelineRun, Thread, Comment, Timeline, BuildLog, Identity) carrying `Kind`/`Scope`/`ID`; no JSON tags. Map `Kind` = constant `ProviderAzure`, `Scope` from the existing `ProjectName`/`ProjectDisplayName`, `ID` from the wire ID.
- [x] 2. Define the `Provider` interface (PR, work-item, pipeline, log surface + `Kind()` and `WebURL()`) covering every method the views call today.
- [ ] 3. Add a compile-time conformance test asserting the azdevops adapter satisfies `provider.Provider` (fails until task 5).
- [ ] 4. Write mapping tests (wire struct → neutral type) for each domain type; assert as an invariant that every mapped entity has non-zero `Kind`, `Scope`, and `ID`. (blocked by: 1)
- [ ] 5. Implement the azdevops adapter: map wire→neutral, delegate to `MultiClient`, satisfy `Provider`. (blocked by: 2,4)
- [ ] 6. Re-type `app.Model.client` and `app.NewModel` to `provider.Provider`, keeping a separate nullable `*azdevops.MultiClient` field for the metrics view; `main.go` passes both. (blocked by: 5)
- [ ] 7. Migrate the `pullrequests` view and its `NewModelWithStyles` to consume neutral types (list, detail, diff, threads/votes). (blocked by: 6)
- [ ] 8. Migrate the `workitems` view and its `NewModelWithStyles` to consume neutral types (list, detail, state/tag pickers, comments). (blocked by: 6)
- [ ] 9. Migrate the `pipelines` view and its `NewModelWithStyles` to consume neutral types (list, timeline detail, log viewer). (blocked by: 6)
- [ ] 10. Wire `main.go` to build the azdevops adapter and pass it as `provider.Provider`. (blocked by: 7,8,9)
- [ ] 11. Run `go test ./...`, `go vet ./...`, and a manual smoke of all three tabs; confirm no behavior change. (blocked by: 10)

## Review feedback: 1. Create internal/provider with neutral domain types

- ✅ Resolved: `Thread` now carries `Line int` (maps from wire `RightFileStart.Line`), so the adapter can reconstruct inline diff placement that `diff.MapThreadsToLines` needs. Covered by `TestThreadLineField`.

## Unknowns

- Right shape for `WebURL` — a single `WebURL(ref any)` vs per-entity methods (`PRWebURL`, `WorkItemWebURL`).
- How deeply views lean on azdevops helper methods (`StateIcon`, `EffectiveDescription`, `BranchShortName`) — some may need to ride along on neutral types to avoid touching Phase 1 concerns.
