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

7. **In `listview`, any dynamic per-row cell needs a matching dynamic column.** If a view's `ToRows` conditionally prepends/appends a cell (e.g. the provider glyph gated on `MixedKinds`, the project column gated on multi-project), it MUST supply a `ToColumns` driven by the *same* predicate over the *same* item slice, so column count and row-cell count never diverge. `table.renderRow` indexes `m.cols[i]`, so a cell with no column panics. _(approved 2026-06-29)_

8. **`listview` tests must render through `View()` after a `WindowSizeMsg`, not just call `ToRows`/`ToColumns` in isolation.** A unit test that invokes the row builder directly passed while the table panicked on render, because the column/cell mismatch only surfaces during `table.renderRow`. Drive at least one no-panic test through the full render path. _(approved 2026-06-29)_

9. **Config map keys arrive lowercased — look them up in lowercase snake_case.** viper lowercases all config keys on load, so `Terms` (and any future `map[string]string` config) only matches lowercase keys like `work_items`/`pull_requests`. A capitalized lookup key silently never matches the override. _(approved 2026-06-29)_

10. **Run `gofmt -l` before every commit; a non-empty result blocks the commit.** Hand-written Go (especially multi-line struct literals and table-test rows) drifts from gofmt, and the build/vet/test gates do NOT catch it, so it reaches the reviewer as a 🟡 every time. Make `gofmt -l <pkg>` printing nothing part of the commit checklist, not a post-hoc fix. _(approved 2026-06-29; Phase 3, Tasks 8 & 11 reviewer 🟡 — gofmt bounced twice.)_

11. **Guard positive-identifier inputs with `<= 0`, never `== 0`.** URL/path builders and any function keyed on a database id, issue/PR number, or thread id must reject `<= 0` — a `== 0` guard lets a negative id through and produces a malformed-but-non-empty result (e.g. `#discussion_r-5`). Test a negative input row alongside the zero row. _(approved 2026-06-29; Phase 3, Task 11 reviewer 🟡 — `PRThreadWebURL` guarded `== 0`, emitted `#discussion_r-5` for negatives.)_

12. **`.gitignore` patterns must match the exact paths the tooling writes — verify, don't assume.** A near-miss (ignoring `.go-cache/`/`.go-tmp/` while the build actually writes `.gocache/`/`.gotmp/`) silently commits thousands of cache blobs into every subsequent commit, invisibly bloating each diff. When tooling writes a build/cache dir, confirm the ignore pattern matches the literal path (`git check-ignore <path>`) rather than trusting a similar-looking entry. _(approved 2026-06-29; Phase 3 — `GOCACHE=$PWD/.gocache` mismatch committed 2927 blobs before it was caught.)_

## Proposed — awaiting approval

<!-- afk appends candidates here -->

13. **Truncation/cap rendering needs a boundary test at exactly the cap.** When a renderer shows the first N items then `+M more`, add a case at `n == cap` (all shown, no suffix) — that's the off-by-one site; testing only well-below and well-above the cap leaves it unpinned. _(Phase 2, Task 6 reviewer nit.)_

14. **Test the dynamic-column collapse path, not just the expand path.** When filtering/search can narrow a list so its column set shrinks mid-session (e.g. a mixed-Kind list filtered down to one Kind drops the glyph column), assert that transition and that the cursor/selection survives the column-count change — the expand direction passing does not prove the shrink direction. _(Phase 2, Task 3 reviewer 🟡 — filter-collapse path left unasserted.)_
