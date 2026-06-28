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
| 6 | Right shape for `WebURL`? | Per-entity methods with `scope string` first param: `WorkItemURL(scope string, id int)`, `PRURL(scope, repositoryID string, prID int)`, `PipelineURL(scope string, id int)` | Avoids type-unsafe `any` parameter; each entity has different identifying fields; `scope` routes to the correct per-project sub-client so multi-project configs produce correct URLs; empty string return signals "cannot construct URL" |

## Tasks

- [x] 1. Create `internal/provider` with neutral domain types (WorkItem, PullRequest, PipelineRun, Thread, Comment, Timeline, BuildLog, Identity) carrying `Kind`/`Scope`/`ID`; no JSON tags. Map `Kind` = constant `ProviderAzure`, `Scope` from the existing `ProjectName`/`ProjectDisplayName`, `ID` from the wire ID.
- [x] 2. Define the `Provider` interface (PR, work-item, pipeline, log surface + `Kind()` and `WebURL()`) covering every method the views call today. (validated: both review must-fixes resolved — `scope string` added to per-project entity methods, `ScopeDisplay` added to `Identity`; build + provider tests green)
- [x] 3. Add a compile-time conformance test asserting the azdevops adapter satisfies `provider.Provider` (fails until task 5). (validated: `internal/azdevops/adapter_conformance_test.go` gated by `//go:build adapter`; normal build/vet clean, `go test/vet -tags adapter` yields the expected `undefined: azdevops.Adapter`)
- [x] 4. Write mapping tests (wire struct → neutral type) for each domain type; assert as an invariant that every mapped entity has non-zero `Kind`, `Scope`, and `ID`. (blocked by: 1)
- [x] 5. Implement the azdevops adapter: map wire→neutral, delegate to `MultiClient`, satisfy `Provider`. (blocked by: 2,4) (validated: URL methods take `scope string` first param and use `ClientFor(scope)` to build project-specific URLs; build/vet/tests green)
- [x] 6. Re-type `app.Model.client` and `app.NewModel` to `provider.Provider`, keeping a separate nullable `*azdevops.MultiClient` field for the metrics view; `main.go` passes both. (blocked by: 5) (validated: `Model.client provider.Provider` + nullable `Model.metricsClient *azdevops.MultiClient`; `NewModel(p provider.Provider, mc *azdevops.MultiClient, ...)`; metrics view wired to concrete client; main.go/demo.go pass both (nil provider until task 10); no view files touched; build clean + app tests green)
- [x] 7. Migrate the `pullrequests` view and its `NewModelWithStyles` to consume neutral types (list, detail, diff, threads/votes). (blocked by: 6)
- [ ] 8. Migrate the `workitems` view and its `NewModelWithStyles` to consume neutral types (list, detail, state/tag pickers, comments). (blocked by: 6)
- [ ] 9. Migrate the `pipelines` view and its `NewModelWithStyles` to consume neutral types (list, timeline detail, log viewer). (blocked by: 6)
- [ ] 10. Wire `main.go` to build the azdevops adapter and pass it as `provider.Provider`. (blocked by: 7,8,9)
- [ ] 11. Run `go test ./...`, `go vet ./...`, and a manual smoke of all three tabs; confirm no behavior change. (blocked by: 10)

## Review feedback: 5. Implement the azdevops adapter

- 🔴 **URL methods silently break multi-project configurations.** `WorkItemURL`, `PRURL`, and `PipelineURL` return `""` whenever the multi-client holds more than one project, because they have no `scope` param to know which project's URL to build. The views today build URLs from the *selected item's own project* (e.g. `workitems/detail.go:252` uses `m.client.GetProject()` on the item's per-project client). Once views migrate to the adapter, these links become empty strings in any multi-project setup — a user-visible regression. Fix: add `scope string` as the first parameter to all three URL methods in `provider.Provider` (updating Decision 6 in the spec) and implement them in the adapter using `ClientFor(scope)` to build the project-specific URL.

## Review feedback: 1. Create internal/provider with neutral domain types

- ✅ Resolved: `Thread` now carries `Line int` (maps from wire `RightFileStart.Line`), so the adapter can reconstruct inline diff placement that `diff.MapThreadsToLines` needs. Covered by `TestThreadLineField`.

## Review feedback: 4. Write mapping tests

- 🔴 **Missing mappers for Iteration, IterationChange, and WorkItemTypeState.** All three neutral types are returned by the `Provider` interface (`GetPRIterations`, `GetPRIterationChanges`, `GetWorkItemTypeStates`) and consumed by the views (`diffview.go`, `detail.go`, `statepicker`). No `MapIteration`, `MapIterationChange`, or `MapWorkItemTypeState` functions were added to `mapping.go`, so task 5's adapter has nothing to call for these three types. Add the mapping functions and tests for each.

## Review feedback: 2. Define the Provider interface

- 🔴 **Project scoping missing on entity methods.** `MultiClient` is keyed by project name (`clients map[string]*Client`); views resolve the right sub-client with `client.ClientFor(item.ProjectName)` before calling entity ops. The flat interface passes only `repositoryID`/`prID`/`buildID` — never a scope — so the task-5 adapter cannot route to the correct sub-client. Fix: add a `scope string` (project name) parameter to every entity method that needs to dispatch per-project (e.g. `GetPRThreads(scope, repositoryID, prID string)`), or expose a `ClientFor`-style `For(scope string) Provider` method. Settle this now — it changes the interface shape and all view migrations (tasks 7-9) depend on it.
- 🔴 **`Identity.ScopeDisplay` missing.** List views render and filter on both `ProjectName` and `ProjectDisplayName` (pullrequests/list.go:593,624-625, workitems/list.go:515,556-557). `Identity.Scope` carries only the project name; there is no display-name field. Add `ScopeDisplay string` to `Identity`, populated at the adapter boundary.

## Unknowns

- Right shape for `WebURL` — a single `WebURL(ref any)` vs per-entity methods (`PRWebURL`, `WorkItemWebURL`).
- How deeply views lean on azdevops helper methods (`StateIcon`, `EffectiveDescription`, `BranchShortName`) — some may need to ride along on neutral types to avoid touching Phase 1 concerns.
