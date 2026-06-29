# Learned Conventions

Repo-specific rules distilled from afk reviewer/validator findings, **after human approval**.
The afk implementer reads the **Active** list before every task. Proposals sit below until you
promote them — move a line up into Active to accept it, delete it to reject.

## Active

<!-- approved rules; one terse imperative line each, optionally tagged _(approved YYYY-MM-DD)_ -->

1. **Cross-check neutral types against all consumer code before finalising.** Before committing a new neutral struct, grep the view/diff layers for every field they read from the wire type — missing a field (e.g. `Thread.Line` needed by `diff.MapThreadsToLines`) causes a silent behavior regression that the type system won't catch. _(approved 2026-06-29)_

2. **Multi-project Provider methods take `scope string` first.** Any `Provider` interface method that dispatches to a per-project sub-client — including URL builders — must accept `scope string` (project API name) as its first parameter. Methods without it cannot route correctly and silently return empty or wrong data in multi-project configs. _(approved 2026-06-29)_

3. **Enumerate mapper coverage from the interface, not the types package.** When tasked with "a mapper for each domain type", derive the list from the `Provider` interface's return types — not just the types file. Sub-entity types (Iteration, IterationChange, WorkItemTypeState) are easily missed if you only scan the types package rather than tracing each interface method's return signature. _(approved 2026-06-29)_

4. **Cross-check view-specific glyphs before wiring to a shared display map.** When migrating a view's string switch to a shared enum+display map, diff the original switch case-by-case against the display map's output for that view — different views may use different glyphs for the same semantic (e.g. PR Active=`●` vs work-item Active=`◐`). If they diverge, special-case the view rather than assuming the shared map reproduces it. _(approved 2026-06-29)_

5. **Verify filter-key functions when migrating to enum labels.** When a filter-key function (`getStatusKey`, `applyStatusFilter`) switches from raw strings to `display.RunStatusLabel`, check every value the old code left as `""` (excluded from filter) — the new enum's label for that value may now return a non-empty string, silently including items that were previously excluded. _(approved 2026-06-29)_

6. **View migration tests must assert glyph and style, not just label substring.** A test that only checks `wantContains: "Active"` does not catch a glyph change (`●`→`◐`) or a color regression (Info→Warning). Assert the full rendered token (glyph + label) and verify the style function returns the expected named style. _(approved 2026-06-29)_

## Proposed — awaiting approval

<!-- afk appends candidates here -->
