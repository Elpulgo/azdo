# Learned Conventions

Repo-specific rules distilled from afk reviewer/validator findings, **after human approval**.
The afk implementer reads the **Active** list before every task. Proposals sit below until you
promote them — move a line up into Active to accept it, delete it to reject.

## Active

<!-- approved rules; one terse imperative line each, optionally tagged _(approved YYYY-MM-DD)_ -->

1. **Cross-check neutral types against all consumer code before finalising.** Before committing a new neutral struct, grep the view/diff layers for every field they read from the wire type — missing a field (e.g. `Thread.Line` needed by `diff.MapThreadsToLines`) causes a silent behavior regression that the type system won't catch. _(approved 2026-06-29)_

2. **Multi-project Provider methods take `scope string` first.** Any `Provider` interface method that dispatches to a per-project sub-client — including URL builders — must accept `scope string` (project API name) as its first parameter. Methods without it cannot route correctly and silently return empty or wrong data in multi-project configs. _(approved 2026-06-29)_

3. **Enumerate mapper coverage from the interface, not the types package.** When tasked with "a mapper for each domain type", derive the list from the `Provider` interface's return types — not just the types file. Sub-entity types (Iteration, IterationChange, WorkItemTypeState) are easily missed if you only scan the types package rather than tracing each interface method's return signature. _(approved 2026-06-29)_

## Proposed — awaiting approval

<!-- afk appends candidates here -->
