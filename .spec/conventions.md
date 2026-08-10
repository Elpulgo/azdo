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

17. **Never call `Config.Save()` in a test on a config that did not come from `LoadFrom(<t.TempDir() path>)`.** `Save()` falls back to `GetPath()` when `configPath` is empty, so a `Config{...}` struct literal — or anything built without an explicit path — writes straight to the developer's real `~/.config/azdo-tui/config.yaml` and destroys it. Build every save-path fixture by writing a temp YAML file and loading it with `LoadFrom`; never a bare struct literal, never `GetPath()`. _(approved 2026-07-29; a live session overwrote the maintainer's actual config this way.)_

## Proposed — awaiting approval

<!-- afk appends candidates here -->

13. **Truncation/cap rendering needs a boundary test at exactly the cap.** When a renderer shows the first N items then `+M more`, add a case at `n == cap` (all shown, no suffix) — that's the off-by-one site; testing only well-below and well-above the cap leaves it unpinned. _(Phase 2, Task 6 reviewer nit.)_

14. **Test the dynamic-column collapse path, not just the expand path.** When filtering/search can narrow a list so its column set shrinks mid-session (e.g. a mixed-Kind list filtered down to one Kind drops the glyph column), assert that transition and that the cursor/selection survives the column-count change — the expand direction passing does not prove the shrink direction. _(Phase 2, Task 3 reviewer 🟡 — filter-collapse path left unasserted.)_

15. **Never pass a concrete typed-nil pointer where an interface parameter is expected — assign through an interface variable first.** A `var c *Concrete = nil` boxed into an interface arg yields a non-nil interface value, so the callee's `if x == nil` guard is false and the first method call derefs the nil pointer and panics. When a dependency is optional, declare `var iface IfaceType; if c != nil { iface = c }` and pass `iface`, then guard `iface == nil` in the consumer. _(Phase 4, Task 5 review 🔴 → Task 8 — GitHub-only run boxed a nil `*azdevops.MultiClient` into `polling.PipelineClient` and panicked at startup.)_

16. **A test that must pin a panic/deref regression has to execute the offending path WITHOUT a recover wrapper.** Driving the suspect command through a helper that `defer recover()`s (e.g. a `collectMsgTypes`-style harness) silently swallows the very panic the test claims to guard, so it passes against the unfixed code too. Invoke the command directly and assert its observable result (nil/no-op cmd), or let the panic propagate and fail the test. _(Phase 4, Task 8 review 🟡 — nil-Azure smoke test routed through a recovering helper and would not have caught the original panic.)_

17. **When a test asserts "X clears Y", check that nothing between the two steps also clears Y.** A setup call placed between the failure and the success — a refetch, a reset, a re-render — often clears the same field, so the assertion passes against code with the clear deleted. Drive the two steps back-to-back, which is usually the realistic user path anyway. _(Phase 1 notifications, Task 19 — `TestMarkResult_Success_ClearsAnyPriorFailureMessage` called `HandleFetchResult` in between; `HandleFetchResult` resets `statusMessage` unconditionally, so deleting `handleMarkResult`'s own clear left the test green.)_

18. **Confirm the package still compiles before reading a mutation's result.** A mutation that breaks the build produces no test output, which greps for `FAIL` or `--- FAIL` report as "no failures" — indistinguishable from a survivor. Run `go build`/`go vet` on the package first and treat a build error as *inconclusive*, not as a passing mutant. _(Phase 1 notifications, Task 17 — removing an `if` guard left a variable declared-and-unused; the false survivor was chased before the build error was noticed.)_

19. **A status field is not a render surface until something renders it.** Before adding a `statusMessage`-style field, find the code that actually paints it and confirm the condition guarding that paint can be true for your pane. Assert visibility through the top-level `View()`, never through the getter. _(Phase 1 notifications, Tasks 17/19 — the pane's `GetStatusMessage()` was gated on `HasContextBar()`, permanently false for that pane, so every `o` and mark outcome was computed and rendered nowhere. `internal/ui/metrics/list.go` still has the identical dead path.)_

20. **A shared status-bar field may be asserted freely but retracted only by its own writer.** Capture the value you wrote, and clear only if the field still holds it — an unconditional clear deletes whatever another subsystem wrote in the meantime. _(Phase 1 notifications, Task 17 — the pane's clear wiped a concurrent partial-load warning; measured `present before=true after=false`.)_

21. **A handler that changes a status-bar field and returns early must re-measure the footer.** Footer height is cached; a handler that widens the bar and returns without re-measuring leaves the cached row count stale and the view renders one row over the terminal height. _(Phase 1 notifications, Task 16 — an unread badge appearing pushed the render to 41 lines in a 40-row terminal at widths 116–128.)_

22. **Calibrate a width sweep against the actual message, don't inherit one.** Wrapping depends on the string's own length, so a band that discriminates one message's footer-row change often does not discriminate another's — the stale cached count coincidentally matches at those widths and the test passes against the mutation. Instrument directly to find the widths where the row count really changes. _(Phase 1 notifications, Task 17 — the task-16 band 100–120 did not discriminate the `o`-failure or mark-failure messages; 128/130/150 did.)_

23. **A background poller must be gated on the same predicate as the pane it feeds, and the gate belongs on the timer, not only the initial fetch.** `OnTick` re-arms itself, so one ungated start is a permanent chain. _(Phase 1 notifications, Task 15 — a disabled notifications pane still issued `GET /notifications` every 30s for the life of the process, defeating the documented "disable the pane" remedy for a missing token scope.)_

24. **`RemoveBindingsByDescription`-style helpers match on substring — treat every help description as a match target.** Text added to one binding's description can make it a match for an unrelated removal call elsewhere in the file. Check the existing call sites before wording a new description. _(Phase 1 notifications, Task 17 — a proposed `f` description would have been deleted whenever work items or pipelines were disabled.)_

25. **Derive a documented enumeration from its source of truth, never restate it.** A hand-copied list of enum values, config keys or scopes in docs or a second package drifts silently. Generate it, or verify it against the declaring type as a review step. _(Phase 1 notifications, Task 20 — the spec's own prose named six config keys where the struct had nine; the docs were right only because the implementer read `NotificationsConfig` instead of the spec.)_

26. **When a requirement is stated in a reference doc, grep for every other place that states it.** A setup wizard, a `--help` block and a README often carry the same list; updating one and not the others leaves the flow users actually follow teaching the old requirement. _(Phase 1 notifications, Task 20 — `runHelp()` was fixed to name the `notifications` scope while `runAuthGitHub()`, the interactive token-creation wizard, still omitted it.)_

27. **`*CompositeProvider` implements every optional capability interface unconditionally — ask, never type-assert.** A `provider.X` type assertion against the composite always succeeds regardless of whether any backend is actually capable. Call the composite's own `HasX()`. _(Phase 1 notifications, Decision 30.)_

28. **Verify a claim about an external API against that API's own docs before it reaches user-facing text.** Plausible-by-symmetry is not verification: an API that supports one auth flavor does not thereby support the other, and an error message that names a nonexistent remedy sends the user hunting for something that cannot be found. Cite the doc page in the spec decision. _(Phase 1 notifications, Decision 87 — README, FAQ, Architecture.md, both `main.go` scope blocks and the pane's 403 body all documented a fine-grained "Notifications" account permission that GitHub does not have; the notifications API is classic-PAT-only.)_

29. **Internal roadmap vocabulary ("phase 1", "task 12", decision numbers) must not appear in user-facing strings.** It names the project's internal structure instead of the user's next action. Keep it in the comment above the string, where maintainers read it. _(Phase 1 notifications, Decision 91 — a config validation error told users notifications "requires GitHub in phase 1".)_

30. **A conditional column that hides information a filter was applied to reveal is a bug, not a space saving.** Gate dynamic columns on whether the dimension is *incidental* to the pane, not on whether the current slice happens to vary along it. _(Phase 1 notifications, Decision 88 — the Repo column vanished exactly when `only_configured_repos` narrowed the feed to one repo.)_

31. **State conveyed only by font weight or colour is invisible to some users and some themes — give it a glyph.** If the sole feedback for an action is a style change, the action looks like a no-op when that style is subtle. _(Phase 1 notifications, Decision 89 — unread was bold-only, so `u` appeared to do nothing.)_

32. **Index row cells by named constant, never by literal position.** Tests that carry their own `const fooCol = 1` keep passing against a shifted layout while asserting the wrong cell, and the failure reads as a data bug. _(Phase 1 notifications, Decision 90.)_

33. **Mutation-test with a widening mutant, not only narrowing ones.** A set of mutants that only makes a predicate stricter cannot tell a load-bearing guard from a dead one — the tests pass either way because the guard was never the thing being exercised. For every predicate you claim is pinned, also flip it the *loose* way (`if x` → `if true`, `>=` → `>`, drop the clause entirely) and confirm something fails.

34. **Treat "this mutant is equivalent" as a claim to disprove, not a conclusion.** An equivalence argument that reasons only about the value a branch computes misses the side effects it also performs — appending a warning, advancing a timestamp, writing to a store. Read the whole branch body before declaring a mutant unkillable; a passing test and an equivalence claim deserve exactly the same scepticism.

35. **Every behavioural sentence in user-facing docs must name the code line that makes it true.** A docs task is a verification task, not a writing task: README/FAQ/ADR prose asserting how a feature behaves is as falsifiable as an assertion, and this repo has repeatedly shipped plausible-sounding sentences the code contradicts. Trace each claim to its implementation before committing it, and re-trace it after any edit that touches the surrounding paragraph.

36. **Never report a file as already-correct without opening it.** "That file already says the right thing" is a claim about content, and inferring it from a task description, a filename or a previous pass's summary has produced false clears more than once. Open it, or say you did not check it.

37. **When a helper's last real caller goes away, delete it — do not keep it "for safety".** A defensive copy or guard that nothing reaches is not insurance; it is a claim the code no longer makes, and the next reader will build on it. If the guarantee moved elsewhere, point the doc comment at the new home and remove the old code.

38. **Trace a config knob to every code path the docs claim it bounds.** A key that reaches two of a feature's four sources will be documented as bounding all four unless someone follows the value through each call site. Grep for the field name and read every use before writing "narrowing X helps here".

39. **Never `git checkout --` a file inside the loop's worktree.** It discards uncommitted work with no recovery and no prompt. To undo a temporary edit, `cp` a backup first and restore from it, then `diff` to confirm the restore was exact.
