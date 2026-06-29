# Phase 1 — Seal the Azure Leaks

**Ticket:** N/A
**Branch:** feature/gh-phase-1
**Author:** Oscar Larsson
**Created:** 2026-06-28

## Goal

The three views render purely from neutral enums + provider-built URLs and
filters — no `dev.azure.com`, WIQL, `System.*`, or vote-int literals remain in
`internal/ui` (metrics excepted), with zero user-visible behavior change.

## Constraints

- **No behavior change.** Same URLs, glyphs, colors, and filter results; every existing test stays green.
- Builds on Phase 0 — the `provider.Provider` interface and neutral types exist.
- Azure stays the only backend; **no `Kind()`-based branching yet** (that's Phase 2).
- TDD: tests for URL shapes / enum mappings / filter translation first, then make green.
- After this phase, no Azure-specific string or syntax survives in `internal/ui` outside `metrics`.

## Scope

**In scope:**
- `WebURL` on the provider; retire the inline `dev.azure.com` builders in the PR and work-item views.
- Neutral semantic enums (state, type, vote, run-status, priority); UI maps enum → glyph+color+label.
- Neutral `ListOpts`; the azdevops adapter builds WIQL / `@Me` / state filters from it.

**Out of scope:**
- The metrics view — Azure-only, keeps its own `buildWorkItemURL` and concrete client.
- GitHub backend (Phase 3), per-row provider glyphs & term relabeling (Phase 2), composite (Phase 4).
- Theme/color values themselves — only *which* semantic maps to a style moves.

## Approach

Move the three `dev.azure.com` `fmt.Sprintf` builders (`workitems/detail.go`,
`pullrequests/detail.go`) behind provider `WebURL` methods that build from the
entity's `(Kind, Scope, ID)` identity. Replace the Azure-string `switch`
blocks in each list/detail view (type, state, priority, PR status, reviewer
vote, run status/result) with neutral enums supplied by the adapter, plus one
shared `enum → glyph+color+label` map in the UI so theming stays in `styles`.
Introduce a `ListOpts` filter struct so the views express *intent* (mine,
state, status, search) and the adapter owns the WIQL/`@Me` translation.

## Decisions

| # | Question | Decision | Rationale |
|---|----------|----------|-----------|
| 1 | `WebURL` shape (the Phase 0 unknown)? | Per-entity methods (`PRWebURL`, `PRThreadWebURL`, `WorkItemWebURL`) | URL shapes differ per entity; typed methods beat a `WebURL(any)` switch |
| 2 | Where do glyph/color/label live? | Provider supplies a **neutral semantic enum** + label; one UI map turns enum → glyph+theme color | Keeps theming in `styles`; removes Azure strings from views without leaking style into the provider |
| 3 | Collapse `List/ListMy/ListAsReviewer` into one `ListOpts`? | No — keep the methods, parameterize only the leaking filters (state/status/search) via `ListOpts` | Smaller, safer diff; method collapse isn't needed to seal leaks |
| 4 | Touch the metrics view? | No | Azure-only by decision; its URL builder and string maps stay |
| 5 | Is `Priority` a semantic enum? | No — keep it an optional value; the work-item column renders `-` when unset | It's an int→label render, not an Azure-syntax leak; GitHub items have no priority and simply show `-` |

## Tasks

- [x] 1. Add per-entity `WebURL` methods to `Provider` + azdevops adapter (build from identity + org); table-test the exact URL shapes against today's output. (validated: `PRThreadWebURL(scope, repositoryID string, prID int, threadID int) string` added to `provider.Provider` and `azdevops.Adapter`; table tests in `adapter_url_test.go` cover all three URL shapes — WorkItemURL, PRURL, PRThreadWebURL — plus nil-client and unknown-scope edge cases; `go vet ./...` and `go vet -tags adapter ./internal/azdevops/...` clean; `go test ./...` — all real packages pass, only the 5 pre-existing sandbox TMPDIR-cleanup failures remain)
- [x] 2. Replace the inline builders in `pullrequests/detail.go` and `workitems/detail.go` with provider `WebURL` calls. (blocked by: 1)
- [x] 3. Define neutral semantic enums (`StateCategory`, `ItemType`, `VoteKind`, `RunStatus`) in `internal/provider`; map azdevops wire → enum in the adapter; test each mapping. `Priority` stays an optional value, not an enum. (blocked by: 1)
- [x] 4. Add one shared `enum → glyph+color+label` display map (in `ui/styles` or a `ui` helper) reproducing today's glyphs/colors exactly; unit-test it. (blocked by: 3)
- [x] 5. Migrate the `workitems` view: swap the type/state string switches for enum + display map; make the priority column render `-` when unset (no priority). (blocked by: 4)
- [x] 6. Migrate the `pullrequests` view: swap the PR-status + reviewer-vote string switches for enum + display map. (blocked by: 4)
- [x] 7. Migrate the `pipelines` view: swap the status/result string switch for enum + display map. (blocked by: 4)
- [x] 8. Define `ListOpts` (mine, states, status, search, top); thread it through the provider list methods; adapter builds WIQL/`@Me`/state filters from it; test the translation. (blocked by: 3)
- [x] 9. Update the view filter call sites (my items / as reviewer / state picker / status picker / search) to pass `ListOpts`. (blocked by: 8)
- [ ] 10. Grep `internal/ui` (minus `metrics`) for `dev.azure.com`, `System.`, WIQL, and vote ints to confirm none remain; run `go test ./...`, `go vet ./...`, and a manual smoke of all three tabs. (blocked by: 2,5,6,7,9)

## Unknowns

- Whether the `ready for test`-style custom states need a dedicated category or fold into an `InProgress`/`other` bucket without losing the current color.

## Review feedback: 3. Define neutral semantic enums

- 🔴 **Add `RunStatusPending` and `RunStatusSucceededWithIssues` to `RunStatus` enum and mapper.** `internal/ui/pipelines/detail.go:592-606` renders `pending` (status) and `succeededWithIssues` (result) with distinct glyphs/colors today — existing tests at `detail_test.go:60,128` confirm this. `MapRunStatus` currently sends both to `RunStatusUnknown`. When Task 7 migrates the pipelines view to the enum+display map, those glyphs will be dropped and those tests will fail — violating the spec's "no behavior change / every existing test stays green" constraint. Add the two variants, mapper cases, and test rows.
- 🟡 **Add `RunStatusSkipped` and `RunStatusAbandoned` (or document `Unknown` reproduces their glyph intentionally).** `skipped` and `abandoned` results currently render a specific Muted `○` in the pipelines detail view — either fold them into a Canceled-equivalent variant or add a comment explaining `Unknown` produces the correct glyph for them, and add test cases.

## Review feedback: 4. Add one shared enum → glyph+color+label display map

- 🔴 **`RunStatusGlyph(RunStatusUnknown)` must return `"○"`, not `"?"`**. The pipelines detail view (`detail.go:586-607`) renders Muted `○` as its default/unknown case. The adapter maps `skipped`/`abandoned`/all unrecognised values to `RunStatusUnknown`. Existing tests at `detail_test.go:116-143` assert skipped→`○` and abandoned→`○`. Returning `"?"` is a new glyph that doesn't exist anywhere today and will break those tests when Task 7 wires it up. Either (a) make `RunStatusGlyph(RunStatusUnknown)` return `"○"` to match both view sites, or (b) have the detail view handle Unknown specially in Task 7 and document that in a comment here — but option (a) is simpler and correct.
- 🟡 **Add style tests for all four `*Style` functions.** `StateStyle`, `ItemTypeStyle`, `VoteStyle`, `RunStatusStyle` have zero tests. These are exactly where transpositions (wrong style for a variant) pass silently. Add table tests asserting each variant returns the expected named style (e.g. `styles.DefaultStyles().Warning` etc.).

## Review feedback: 6. Migrate the pullrequests view

- 🔴 **"Active" PR glyph changed `●` → `◐`.** `display.StateGlyph(StateCategoryActive)` returns `◐` (the work-item glyph). The PR list view previously rendered `● Active` (filled circle). Fix by special-casing the PR status rendering to use `●` for active PRs, rather than sourcing from `StateGlyph`.
- 🔴 **"Closed" (abandoned) PR glyph `○`→`✗` and color Muted→Error.** `MapStateCategory("abandoned")` maps to `StateCategoryRemoved`, whose glyph is `✗` with `Error` style. Old code rendered `s.Muted.Render("○ Closed")`. An abandoned PR now shows a red `✗` instead of a muted `○`. Fix: render the abandoned case with `s.Muted` and `○` explicitly, or special-case `StateCategoryRemoved` in `statusIconWithStyles` for PRs.
- 🟡 **Tests assert only the label substring, not the glyph or style.** The active and abandoned test cases in `list_test.go` only check `wantContains: "Active"` / `wantContains: "Closed"` — they don't pin the glyph or color, which is why the regressions passed the validator. Add glyph assertions (`●` for active, `○` for abandoned) to lock in behavior.
