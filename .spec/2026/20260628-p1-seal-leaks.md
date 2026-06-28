# Phase 1 — Seal the Azure Leaks

**Ticket:** N/A
**Branch:** refact/p1-seal-leaks
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

- [ ] 1. Add per-entity `WebURL` methods to `Provider` + azdevops adapter (build from identity + org); table-test the exact URL shapes against today's output.
- [ ] 2. Replace the inline builders in `pullrequests/detail.go` and `workitems/detail.go` with provider `WebURL` calls. (blocked by: 1)
- [ ] 3. Define neutral semantic enums (`StateCategory`, `ItemType`, `VoteKind`, `RunStatus`) in `internal/provider`; map azdevops wire → enum in the adapter; test each mapping. `Priority` stays an optional value, not an enum. (blocked by: 1)
- [ ] 4. Add one shared `enum → glyph+color+label` display map (in `ui/styles` or a `ui` helper) reproducing today's glyphs/colors exactly; unit-test it. (blocked by: 3)
- [ ] 5. Migrate the `workitems` view: swap the type/state string switches for enum + display map; make the priority column render `-` when unset (no priority). (blocked by: 4)
- [ ] 6. Migrate the `pullrequests` view: swap the PR-status + reviewer-vote string switches for enum + display map. (blocked by: 4)
- [ ] 7. Migrate the `pipelines` view: swap the status/result string switch for enum + display map. (blocked by: 4)
- [ ] 8. Define `ListOpts` (mine, states, status, search, top); thread it through the provider list methods; adapter builds WIQL/`@Me`/state filters from it; test the translation. (blocked by: 3)
- [ ] 9. Update the view filter call sites (my items / as reviewer / state picker / status picker / search) to pass `ListOpts`. (blocked by: 8)
- [ ] 10. Grep `internal/ui` (minus `metrics`) for `dev.azure.com`, `System.`, WIQL, and vote ints to confirm none remain; run `go test ./...`, `go vet ./...`, and a manual smoke of all three tabs. (blocked by: 2,5,6,7,9)

## Unknowns

- Whether the `ready for test`-style custom states need a dedicated category or fold into an `InProgress`/`other` bucket without losing the current color.
