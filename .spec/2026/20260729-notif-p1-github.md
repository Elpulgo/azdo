# Notifications Pane — Phase 1: GitHub

**Ticket:** N/A
**Branch:** feat/notifications-github
**Author:** Oscar Larsson
**Created:** 2026-07-29

## Goal

A default-on Notifications tab — first in the tab bar — that renders the GitHub
user inbox as a provider-neutral attention feed with read/done triage.

## Constraints

- UI has **one entrypoint**: the pane consumes `[]provider.Notification` and never
  knows which backend produced a row (same rule as the PR/work-item/pipeline lists).
- `GET /notifications` is **user-level**; the existing `github.Client` is per-`owner/repo`.
  Notifications cannot reuse the `scope`-routed fan-out (learned convention 2 does not apply).
- Azure DevOps has no inbox API. Phase 1 ships with Azure not implementing the capability.
- No new external dependencies; glob matching via `path.Match`.
- Config keys are lowercased by viper — all keys lowercase snake_case (convention 9).
- `Config.Save()` must never drop existing config. It already round-trips the file through
  `ReadInConfig()` before setting values, so unmanaged keys (`metrics`, `terms`, `github`,
  `notifications`) survive — but **no test pins this**, so task 17 adds one. Comments are
  lost by viper and that is accepted; values are not negotiable.
- **Tests must never write the real config.** `Save()` falls back to `GetPath()` when
  `configPath` is empty, so a `Config{...}` literal in a test rewrites
  `~/.config/azdo-tui/config.yaml`. Always go through `LoadFrom(<t.TempDir() path>)`.

## Scope

**In scope:**
- `provider.NotificationSource` optional capability interface + neutral `Notification` type
- User-scoped GitHub client: list, mark read, mark done, conditional requests
- Composite fan-out over backends that implement the capability
- `notifications` config block (filters, precedence) + `disabled_panes: notifications`
- New first tab, unread footer badge, help-modal section, polling integration
- Docs: ADR, README, Architecture.md, config.yaml.example, FAQ, help modal

**Out of scope:**
- Azure DevOps synthetic feed → phase 2 (`20260729-notif-p2-azdo.md`)
- Unsubscribe-from-thread, marking whole repos read, subscription-rule management
- Jump-to-tab deep-linking (browser `o` only in v1); desktop notifications
- Cross-machine sync; local read/done state (phase 1 is fully server-side)

## Approach

Model notifications as an **optional capability**, not a `Provider` method — mirroring how
Metrics stayed off the interface. `CompositeProvider` type-asserts each backend for
`NotificationSource`, fans out to those that match, merges and sorts newest-first, and
exposes `HasNotifications()` so the app can hide the tab when nothing supports it. GitHub
gets a user-scoped client alongside the per-repo ones, honouring `Last-Modified`/
`If-Modified-Since` and `X-Poll-Interval` so polling stays off the rate limit. Read and
done are server-side (`PATCH`/`DELETE /notifications/threads/{id}`), so phase 1 needs no
local state file. Filtering is a pure function over the merged feed, config-driven.

## Config shape

A `notifications:` block with keys `only_configured_repos`, `exclude_repos`
(glob), `include_repos` (glob), `exclude_reasons`, `unread_only`, `participating_only`,
`since_days`, `max_items`, `poll_interval`. Every default is the widest behaviour — the
whole inbox, nothing filtered, pane on. Task 9 owns defaults and validation; task 17
documents each key in `config.yaml.example`.

Filter precedence, most to least specific: `only_configured_repos` → `include_repos` →
`exclude_repos` → `exclude_reasons` → `unread_only`.

`exclude_reasons` filters on *why* the row reached your inbox. It exists because the repo
filters are all-or-nothing per repo, and the repos you work in hardest emit both your
`review_requested` rows and the bulk of your `subscribed` noise: blacklisting the repo would
cost you the review requests. Reason is the axis that correlates with "needs me".

The accepted values are the **neutral** enum names, not GitHub's wire strings. There are
**eleven** configurable values, verbatim and exhaustive (task 20 documents this list; do not
abbreviate it with "…"): `review_requested`, `mentioned`, `assigned`, `authored`,
`commented`, `state_changed`, `ci_activity`, `security_alert`, `approval_requested`,
`subscribed`, `other`. Five deliberately differ from GitHub's spelling — GitHub sends
`mention`, `assign`, `author`, `comment`, `state_change` — because decision 19 keeps the
config neutral for phase 2. GitHub spellings are **not** accepted as aliases; instead a value
the parser does not recognise is reported as unrecognised (decision 26) so the user sees
their typo rather than getting silently reinterpreted behaviour.

The enum's twelfth value, `unknown`, is **not configurable**. `String()` emits it for the
zero value, but `ParseNotificationReason` rejects it — the wire mapper never emits `Unknown`
(decision 18), so it could only ever match nothing. Listing it in `exclude_reasons`
therefore produces task 9's unrecognised-entry warning. Task 20 must not document it as an
accepted value.

## Decisions

| # | Question | Decision | Rationale |
|---|----------|----------|-----------|
| 1 | Extend `Provider` or add an optional interface? | Optional `NotificationSource`, discovered by type assertion | No empty Azure stubs, no `ErrUnsupported` runtime path, no typed-nil traps (convention 15). Metrics set the precedent |
| 2 | Show the whole inbox or only configured repos? | Whole inbox by default, narrowable by config | An inbox filtered to 3 repos is not a triage pane. Escape hatches cover the noise |
| 3 | Rows from unconfigured repos? | Shown; `o` opens browser, no tab jump | No client exists for those scopes; browser is the honest fallback |
| 4 | Triage actions in v1? | Read + done only | Both are server-side one-call endpoints. Unsubscribe is destructive and hard to undo from a TUI |
| 5 | Local read/done state? | None in phase 1 | GitHub owns the state. Phase 2 introduces it for Azure — do not build it early |
| 6 | Tab position? | First, before Pull Requests | It is the "what needs me now" pane, so it is the natural landing tab |
| 7 | Merged feed or per-provider sections? | One merged feed, sorted newest-first, provider glyph per row | Matches the merged-list decision from the GitHub-parity work |
| 8 | Polling cadence? | `X-Poll-Interval` as floor, else `notifications.poll_interval`, else global | Conditional requests returning 304 do not count against the rate limit |
| 9 | `state.yaml` version bump? | No bump; add `TabID "notifications"` | Additive — old files simply restore a different tab |
| 10 | `participating_only` and `exclude_reasons` overlap — keep both? | Both, and they compose: `participating_only` narrows server-side, `exclude_reasons` trims the result client-side | They are not redundant. `participating=true` is one coarse server-side bundle (roughly "everything except `subscribed`") and is cheaper — fewer pages fetched. `exclude_reasons` is per-reason and exact but only after the full fetch. Neither subsumes the other; do not drop one |
| 11 | When is the tab hidden? | On **capability**, never on emptiness. Hidden only when no configured backend implements `NotificationSource` | An empty inbox is a *result* ("you're clear"), not a reason to remove navigation. Azure-only configs hide the tab in phase 1 and it reappears by itself in phase 2 once Azure implements the interface — no app-layer change needed |
| 12 | Fetch unread-only or everything? | Always `all=true`; `unread_only` filters client-side | Default `GET /notifications` returns unread only, so marking a row read would make it vanish next poll and leave `unread_only: false` with nothing to un-filter |
| 13 | Is read reversible? | No — `u` is **mark read**, one-way, not a toggle | GitHub exposes mark-read (`PATCH`) but no mark-unread endpoint. Encode one-way; the loop has network but no token, so it can confirm the route exists and is auth-gated, not observe behaviour. Flag for manual confirmation, do not claim it verified |
| 14 | Notification identity? | Provider-qualified from day one (kind + scope + native id), never a bare id | Phase 2's Azure keys are derived strings that would collide with GitHub thread ids. This is the same trap that forced the `state.yaml` v1→v2 migration; retrofitting identity is expensive |
| 15 | `Read`/`Done` on the neutral type even though phase 1 has no local state? | Yes, both fields exist in phase 1 | GitHub populates them from the server, Azure from local state in phase 2. Keeping them means phase 2 changes only *who fills them*. Do not "simplify" them away while GitHub is the only source |
| 16 | One disable mechanism or two? | Only `disabled_panes: notifications`; no `notifications.enabled` key | Every default-on pane already disables via `disabled_panes`; `metrics.enabled` exists because metrics is opt-in. Two knobs for one behaviour invites the state where they disagree |
| 17 | Missing `notifications` token scope? | Dedicated in-**view** error state naming the required scope, plus an in-view action to disable the pane by writing `disabled_panes` via `Config.Save()` | A 403 here is a configuration problem, not a transient failure — retry-and-toast would nag forever. The user gets exactly two exits: fix the token, or turn the pane off. The disable action is offered **only** from this error state; when the pane works, `disabled_panes` in the config file is the documented route and needs no TUI affordance |

| 18 | Neutral reason vocabulary — settled here, **not** left to the implementer | `Unknown`, `ReviewRequested`, `Mentioned`, `Assigned`, `Authored`, `Commented`, `StateChanged`, `CIActivity`, `SecurityAlert`, `ApprovalRequested`, `Subscribed`, `Other`. Collapses: `team_mention`→Mentioned, `security_advisory_credit`→SecurityAlert, `manual`/`invitation`/`member_feature_requested`→Other. **Any unrecognised reason maps to `Other` and is never dropped** | Tasks 5, 10 and 11 all consume this enum, so an implementer guess would force three reworks. Collapsing is by triage action: nobody triages a team mention differently from a direct one. Dropping unknown reasons would silently hide notifications, which is the one failure a triage pane cannot have |
| 19 | `exclude_reasons` keys — GitHub strings or neutral names? | Neutral enum names, lowercase snake_case (`subscribed`, `ci_activity`, `security_alert`) | The config must survive phase 2 unchanged. Neutral names happen to read the same as GitHub's for the common cases, so nothing looks odd |
| 20 | Composite partial failure? | One backend erroring keeps the other backends' rows and surfaces a per-backend error; it never empties the feed | A blank attention pane is indistinguishable from "you're clear" — the worst possible failure mode here. Resolves a phase-2 open question early |
| 21 | Badge counts unread before or after filters? | After. Hidden entirely at zero | Counting rows the user's own config deliberately hides would nag about things they chose not to see |
| 22 | Is notifications-only a valid config? | Yes — the "at least one pane" guard must accept it | It is a genuinely useful standalone pane. The current guard only checks PR/work-items/pipelines and would reject it |
| 23 | How does `X-Poll-Interval` reach the poller? | A **separate** optional interface (`PollIntervalHinter`), discovered by the same type assertion as the capability itself. **Never a fourth method on `NotificationSource`** | Task 4 parses the header inside the GitHub client, but `List` returns only rows and an error, so there is no declared route to the poller. The reflex fix — widening `NotificationSource` — would force phase-2 Azure to implement a method it has no equivalent for, breaking the phase-2 constraint that this interface must not change. Azure simply won't implement the hinter, and the poller falls back to the configured interval |
| 24 | Who owns `NotificationReason` ⇄ config-string conversion? | Task 3, next to the enum: `String()` + `ParseNotificationReason`, following `Kind`'s existing `String()`/`ParseKind` precedent | Decision 19 makes the enum both display *and* serialized. Task 3 owns the display half and tasks 9/10 the config half; without one owner they each invent a parser and the two drift |
| 25 | How does the composite route `MarkRead`/`MarkDone`? | By `Identity.Kind`, **never** `backendFor(scope)` | `backendFor` is built from configured `Scopes()` and returns nil for unconfigured repos — which under decisions 2 and 3 is most of the inbox. Routing by scope would make mark-read silently no-op on exactly the rows the pane exists to surface |
| 26 | What does `ParseNotificationReason` do with an unrecognised string? | Returns `(Other, false)` — the value still degrades to `Other` so nothing errors, but the `bool` reports non-recognition. Callers must not ignore it: task 9 warns on an unrecognised `exclude_reasons` entry, task 10 skips it | Collapsing silently to `Other` inverts the intent on the config side. `exclude_reasons: [subscibed]` would drop every catch-all row while leaving the `subscribed` noise the user actually asked to hide — one typo, two wrong outcomes, no feedback. It also makes task 10's criterion ("an unrecognised reason is only filtered when `other` is listed explicitly") unsatisfiable, because the information needed to honour it has already been discarded. Degrade-to-`Other` is right on the **wire** side (decision 18, keeps rows visible) and wrong on the **config** side; the `bool` is what separates them. `unknown` is likewise `(Other, false)` — it is reserved and matches nothing |
| 27 | Who owns the cached feed the client hands out? | The **caller**. `NotificationsClient` guards its cache with a mutex and returns a **copy** on both the 200 and the 304 path; the cache is also invalidated whenever the request shape (`buildPath(opts)`) changes | Two measured corruptions, both silent. (a) Bubble Tea runs every `tea.Cmd` in its own goroutine, so a poll tick overlapping the refresh key puts two goroutines inside `List`; an unguarded `c.cached = all` racing a 304's `return c.cached` can tear the slice header and render rows read past the end of the array. `polling/poller.go` already guards exactly this tick-vs-refresh state with an `RWMutex` — `Client`/`MultiClient` need no lock only because they are immutable after construction, so a mutable type here breaks a package invariant silently. (b) Returning the cache by reference means a caller doing the standard in-place filter (`out := s[:0]; append(out, …)`) rewrites the cache: `[1 2 3]` filtered to `[2]` leaves the cache `[2 2 3]`, so the next 304 returns a vanished row and a duplicated one. Tasks 10 and 11 both take this slice, and task 11's `sort.Slice` reorders it in place |
| 28 | What does a 304 with nothing cached mean? | An **error**, never an empty feed | The branch exists because the code does not trust the server to send only anticipated statuses — so its fallback must not be "silently report an empty inbox", which is decision 20's worst outcome. It is not unreachable: a caching proxy or MITM appliance can revalidate against its own stored validator and synthesise a 304 nobody asked for, and `SetBaseURL` is a production-supported override (demo mode, GHE), not only a test seam. Guarding costs two lines |
| 29 | What bounds the `Link rel=next` walk? | A hard page-count constant inside the client (cycle protection, task 4) **plus** `NotifOpts.Max` honoured at the adapter boundary (task 7) | Measured: a `next` pointing at itself produced 501 requests and 500 rows with `err == nil`. Inside a `tea.Cmd` a genuinely cyclic `next` never returns — the pane hangs on "loading" with no error and no cancel. Bypassing `getAllPages` is correct (it discards the headers the 304/`X-Poll-Interval` contract needs) but it dropped the only bound the package had, and `NotifOpts.Max` shipped with no implementer. The two bounds are different failures: the constant stops a malformed server, `Max` honours the user's config |
| 30 | What must satisfy `PollIntervalHinter`? | `Adapter` — it forwards `PollInterval()` from the client (task 7), because that is the type the composite holds and task 15 asserts against | Decision 23 settled that the hint travels by a separate interface but not *which type implements it*. The header is parsed in the client, which the composite never sees. If only the client has the method, task 15's assertion against `Adapter` fails, the poller silently falls back to the configured interval, and decision 8's `max()` rule becomes dead code no test notices — a passing suite with the rate-limit protection switched off |
| 31 | How does task 7 honour `NotifOpts.Max`? | Cache the **full** walk, then truncate **on return** — never stop the walk early | Stopping early caches a truncated set under a `cachedPath` that does not encode `Max`, so a later call with a larger `Max` and the same shape hits 304 and gets the truncated cache presented as complete — the exact truncated-feed-as-complete failure decision 27's path-keying was added to prevent. `NotifOpts.Max`'s own doc ("caps the number returned across all fetched pages") is satisfied by truncate-on-return. The cycle bound (decision 29) is what protects the walk; `Max` is a presentation cap, not a fetch bound. Supersedes the earlier "page walk stops once `Max` items are collected" wording |
| 32 | Does `PollInterval()` share the fetch mutex? | No — the cadence hint gets its own guard, so the accessor never contends with an in-flight `List` | `app.go` calls `poller.StartPolling()`/`OnTick()` on the Bubble Tea **main** goroutine, and decision 30 puts `PollInterval()` on that path. Sharing the fetch mutex means a tick landing during a multi-page refresh blocks the main goroutine — worst case 50 pages × the 30s HTTP timeout, so the UI stops rendering and dropping keypresses for minutes. Holding the mutex across `List`'s I/O is still correct (it makes the validator↔cache pairing atomic); it is only the accessor that must not join it |
| 33 | How is a `Release` row's web URL built? | The repo's `/releases` **list** page — never `/releases/{id}`, which 404s. General rule: when the id segment cannot be trusted to produce a real page, fall back | Verified against live github.com (public release pages need no token): `/cli/cli/releases/348300685` → 404, `/cli/cli/releases/tag/v2.96.0` → 200, `/cli/cli/releases` → 200. The correct route needs the **tag name**, and `NotificationSubject` carries only `Title`, `URL`, `LatestCommentURL`, `Type` — so it cannot be built from a notification payload at all. Under decision 3 `o` is the *only* action on rows from unconfigured repos, so a wrong-but-well-formed URL is strictly worse than the fallback: it spends the user's one action on a dead page. Same reasoning forces shape-validation of the id segment (digits for PR/issue, hex for commit) rather than trusting whatever the last path segment happens to be |
| 34 | Which host do web URLs use? | `Repository.HTMLURL` when the wire supplies it, else `https://github.com/<full_name>` | Deferring GHE by hardcoding `github.com` was inconsistent *with itself*: measured, a GHE thread got the right host on unrecognised subject types (which route through the repo fallback and so use the wire `HTMLURL`) and the wrong one on the four handled types. The `weburl.go` precedent does not apply — `WorkItemURL`/`PRURL`/`PipelineURL` hardcode the host because they have no wire URL at all, whereas the sibling *mappers* (`mapping_pr.go`, `mapping_pipeline.go`) already populate this same neutral `WebURL` field from a wire `HTMLURL`. Notifications carry one, so they follow the mappers. Building every per-type URL on the repo-URL prefix therefore fixes the empty-`FullName` case (`https://github.com//pull/42`) and delivers GHE support in the same change. Residual: a GHE thread with an empty `HTMLURL` still falls back to github.com — task 20 documents that |
| 35 | Who defaults `ScopeDisplay`? | `MapNotification` — it **is** the adapter boundary | `provider.Notification`'s doc states `ScopeDisplay` falls back to `Scope` at the adapter boundary, and the mapper derives `scope` from the thread itself, so a caller cannot compute the fallback before calling. Leaving it to task 7's `MultiClient.DisplayNameFor` only covers *configured* scopes, and under decisions 2 and 3 most inbox rows come from unconfigured repos — every list view renders the column from `ScopeDisplay` verbatim, so those rows would render a blank repo column |
| 36 | How strictly is a numeric id segment validated? | Digits-only **and** semantically plausible: reject empty, non-digit, a leading `0`, and anything `<= 0` after parse. Hex (commit SHA) keeps digits-only-in-hex with no magnitude rule | Measured, digits-only alone still emits clickable 404s: `.../pulls/0` → `github.com/o/r/pull/0` and `.../pulls/007` → `.../pull/007`, both dead pages, which is exactly the class decision 33 exists to eliminate — GitHub PR and issue numbers start at 1. This also makes the two sibling id guards in the package consistent: convention 11 (Active) already states this rule, and task 6 restates it verbatim for thread ids, so a mapper that accepts `0` while the mutator rejects it is a trap for whoever reads one and assumes the other. Hex is deliberately exempt — a SHA legitimately begins with `0` and has no ordering |
| 37 | How do `MarkRead`/`MarkDone` invalidate the task-4 conditional-request cache? | An `atomic.Uint64` generation counter, **not** the fetch mutex. The marker calls `cacheGen.Add(1)` and never blocks. `List` **snapshots the counter once, before its first request** (`genAtStart := cacheGen.Load()`), and uses that single snapshot for all three decisions: whether the cache is valid, whether to send `If-Modified-Since`, and — critically — what to store as `cachedGen` when it commits. It must **never** re-read `cacheGen` at commit time | Without invalidation the bug is concrete: mark a thread read, the server's `Last-Modified` may be unchanged for the *collection*, `List` sends `If-Modified-Since`, gets 304, and decision 27's `cloneThreads(c.cached)` faithfully replays the row with `Unread: true` — the row the user just dismissed reappears, which is the same resurrection failure decision 14 keeps `ScopeDisplay` out of identity to avoid. Taking the fetch mutex instead would be correct but pathological: decision 27 has `List` holding `mu` across the entire page walk, so a mark issued during a 50-page fetch blocks for up to 50 × the HTTP timeout, and marks are issued from a `tea.Cmd` the user expects to feel instant. The generation counter keeps the marker lock-free and still makes the validator↔cache pairing atomic, because only `List` writes the cache. Re-reading the counter at commit time instead of snapshotting looks equivalent and is not — it was implemented that way once and reproduced the exact resurrection this decision exists to prevent, **permanently**: a mark succeeding while the walk is in flight bumps the counter, the in-flight page (generated pre-mark, still `unread: true`) is committed with `cachedGen` set to the *post*-mark value, so the cache falsely certifies that it already reflects the mark. Every later `List` then sees a valid cache, sends `If-Modified-Since`, gets 304, and replays the dismissed row — with no self-healing, since the 304 path never rewrites `cachedGen`. Because decision 27 has `List` holding `mu` across the entire walk, the exposed window is the whole fetch (up to 50 round trips), not a narrow one. Snapshotting costs at most one redundant refetch when a mark really did land mid-walk, which is the correct trade: that response may genuinely predate the mark |
| 38 | Where does task 7's `NotificationSource` conformance assertion live? | In the **default build** — a plain `_test.go` in `internal/github`, never behind `//go:build adapter` | Measured: nothing runs that tag. No Makefile, no CI workflow, no script references `-tags adapter`; the only mentions anywhere are manual invocations recorded in three closed 2026-06 specs. The tag is vestigial — it was added in `20260628-p0` so the build stayed green while `Adapter` did not yet exist. An assertion placed there is compiled by neither `go build ./...` nor `go test ./...`, which is exactly the failure decision 30 describes: the gate reads as present, and the suite passes with it switched off. `provider/composite_test.go:13` and `provider/notifications_test.go:25` are the right precedent — untagged `var _` in a normal test file. Do not remove or re-tag the existing `adapter_conformance_test.go`; that is out of scope, just do not extend it |
| 39 | How does `Adapter` obtain the user-level `NotificationsClient`? | A second constructor, `NewAdapterWithNotifications(mc, nc)`, storing an `nc *NotificationsClient` field set once at construction and never mutated. `NewAdapter(mc)` keeps its exact current signature and leaves `nc` nil | `MultiClient` holds only per-repo clients keyed by `owner/repo`; `GET /notifications` is user-level, so the client does not belong in the fan-out map (the constraint at the top of this spec). A setter was rejected: `Adapter` is read concurrently by Bubble Tea's goroutine-per-`tea.Cmd`, so a field mutable after construction is a data race with no lock to hang it on, and decision 37 exists precisely to keep locks off this path. A second constructor leaves every existing `NewAdapter` call site untouched. **Nil `nc` must return a descriptive error, never panic** — matching `NewAdapter(nil)`'s documented contract that the Adapter still satisfies the interface and methods needing a live client fail with a message. This is a reachable state, not defensive padding: decision 11 hides the tab on *capability*, and `Adapter` satisfies `NotificationSource` at compile time whether or not a client was supplied, so a GitHub config that never built one still shows the tab and calls `List`. Task 19 renders that error in-view |
| 40 | How does task 7 assert `PollIntervalHinter` before task 15 defines it? | Task 7 declares the one-method interface **locally in its own test file** and asserts `Adapter` against it. Task 15 defines the real one where it is consumed and asserts again | Decision 30 requires `Adapter` to satisfy `PollIntervalHinter`, but that interface does not exist yet and defining it in `provider` now would put an interface there with no consumer — the opposite of the capability-interface reasoning in decision 1. Go's structural typing makes the local copy sufficient and drift-proof in the direction that matters: if `Adapter` loses `PollInterval()`, task 7's assertion breaks immediately; if task 15's real interface later demands a different method set, task 15's own assertion breaks then. Two identical one-method interfaces cannot disagree silently |
| 41 | What is `total` in the notifications merge, and what does zero capable backends return? | The count of **capable** backends — never `len(cp.backends)`. Zero capable backends returns `(empty, nil)`, not an error | Copying `mergePRs`'s `total = len(cp.backends)` breaks decision 20 in precisely its worst case. The shipped dual-backend config is one Azure adapter (incapable in phase 1) plus one GitHub adapter: if GitHub fails with the 403 scope error, `len(errs) == 1 != total == 2`, so the merge takes the *partial* branch and returns `(nil, &PartialError{Failed: 1, Total: 2})` — an empty feed carrying "1 of 2 sources failed to load", which is exactly the blank-pane-indistinguishable-from-"you're clear" outcome decision 20 exists to prevent, and it buries the one error tasks 13 and 19 must render in-view. With `total` = 1 capable backend it takes the all-failed branch and surfaces a real error. Zero capable backends is separately wrong under `len(errs) == total`, which is satisfied vacuously by `0 == 0` and would report "all backends failed" for an empty set; decision 11 hides the tab on capability so this is rarely reached, but by decision 39's standard that makes it a reachable state, not dead code |
| 42 | How does the notifications merge preserve the error chain? | `errors.Join` on the all-failed path, and `PartialError` gains `Unwrap() []error`. The existing `mergePRs`/`mergeWorkItems`/`mergePipelineRuns` all-failed strings stay as they are | `fmt.Errorf("composite: all backends failed: %v", errs)` — the pattern the three sibling merges use — flattens `[]error` to text and destroys the chain. Task 19 recovers `*github.APIError` and its `RequiredScopes`/`GrantedScopes` through `errors.As` to render "add the `notifications` scope"; task 13 branches on that same recovery for its third render state. Through `%v` neither works, and the suite stays green because the error is still non-nil — the same switched-off-gate failure as decisions 30 and 38. Task 7's review pinned `errors.As` at the adapter boundary for this reason; the composite is the next link in that chain and unpinning it there wastes the guard. `Unwrap() []error` on `PartialError` is purely additive (Go 1.20+ multi-error unwrap), needed because phase 2 makes the partial path reachable with two capable backends, and it fixes `errors.As` for the three existing fan-outs as a side effect — which is why their all-failed strings are deliberately left alone: changing those is a behaviour change to shipped code that this spec has no test coverage for |
| 43 | How does the composite route `MarkRead`/`MarkDone`? | Over **capable backends only**, first match on `backend.Kind() == id.Kind`, with a descriptive error naming the kind when none matches | Decision 25 settles routing by `Identity.Kind` rather than `backendFor(scope)` but not the lookup's shape. It must filter to capable backends first: an Azure backend and a GitHub backend can both be present, and asserting the capability *after* matching kind would route a GitHub row to a type that cannot mark it. First-match-wins mirrors the documented collision rule for `routing` and `Kind()` (D3/D4); at most one GitHub adapter exists per process, since `Adapter` wraps a whole `MultiClient`. A nil/zero `id.Kind` must fall through to the not-found error rather than matching the first capable backend by accident — task 7 rejects an empty `Kind` at the adapter for the same reason. The fall-through must be an **explicit `kind == 0` guard**, not an emergent property of `KindAzure = iota + 1`: `CompositeProvider.Kind()` itself returns `0` on an empty backend list and the composite satisfies `NotificationSource` unconditionally, so an in-package counterexample already exists. Measured on the un-guarded version — a capable backend reporting `Kind() == 0`, registered alone, accepted `MarkRead(Identity{})` and returned a **nil error**, which under task 14 is an optimistic row update whose rollback path never fires |
| 44 | Does the composite re-apply `NotifOpts.Max` after merging? | Yes — truncate in `mergeNotifications` **after** the sort, on both the clean and the partial-error return. This deliberately diverges from `mergePRs`/`mergeWorkItems`/`mergePipelineRuns`, which never re-apply `top` | `NotifOpts.Max`'s own contract is "caps the number of notifications returned across all fetched pages", and `*CompositeProvider` implements the interface that contract belongs to — but `List` forwards `opts` verbatim to every capable backend and returns the merged slice uncapped, so N capable backends return up to N×`Max`. Exact in phase 1 (one capable backend), silently wrong from the first phase-2 Azure implementation, and that is also the moment task 9's `max_items` starts meaning half what it says with nothing in the codebase recording why. Truncating **after** the sort is required, not incidental: truncating before it keeps an arbitrary N rather than the newest N. The three sibling fan-outs are left alone for the same reason as decision 42 — changing shipped behaviour they have no coverage for is outside this spec |
| 45 | Is a deterministic tie order required in the merged feed? | Yes — `sort.SliceStable` plus a total order: `UpdatedAt` descending, then `Identity.Kind`, then `Identity.Scope`, then `Identity.ID` | Measured: `sort.Slice` is a deterministic function of its input (Go's pdqsort seeds `breakPatterns` from `length`, never from time or `rand`), but at realistic feed sizes the unstable result differs from the stable one (observed at n=30 and n=100) and a permuted input yields a different output. `updated_at` is second-resolution, so a CI run touching five threads in one second is routine rather than an edge case. Two input orders are already non-deterministic: with ≥2 capable backends the channel is drained in goroutine-completion order (phase 2), and with one capable backend the merge faithfully *reproduces* whatever order the server sent rather than canonicalising it — so two consecutive polls can reshuffle tied rows, which the spec's Unknowns section notes cannot be ruled out from inside the repo. The harm lands on the cursor: task 11 holds a selection by index and task 15 re-polls on a timer, so a stationary cursor selects a different row between polls and task 14's `d` marks the wrong one done. `Identity.ID` orders lexicographically over numeric strings, which is arbitrary — arbitrary is fine, *canonical* is the requirement. Sibling fan-outs share the instability and are deliberately left alone (decision 42's reasoning) |
| 46 | How does decision 26's unrecognised-`exclude_reasons` warning reach the user? | A `Warnings []string` field on `Config`, populated during load and **not** persisted (`mapstructure:"-"`). Task 9 populates it; task 13 renders it in the notifications pane. Nothing in `internal/config` prints | `internal/config` has no warning mechanism today — measured, nothing in the package writes to stderr or takes a logger, so decision 26's warning had no delivery route and would otherwise have been silently dropped, making the decision unimplementable rather than merely undone. Printing to stderr was rejected: this is a TUI, and a line written before Bubble Tea enters the alternate screen is erased by the switch, while one written after corrupts the frame — so the honest options are a struct field or a signature change, and a field keeps `LoadFrom`/`Load`'s signatures and every existing caller intact. Rendering in-view matches what decision 17 already does for the token-scope error and the user's stated preference that these belong in the view, not the footer. A warning must never become a hard error: decision 26 is explicit that an unrecognised entry cannot stop the app from starting |
| 47 | Does `notifications` count as a remaining pane for the at-least-one-pane guard? | Only when a notification-capable backend will exist — in phase 1, `HasGitHub()`. Otherwise it does not count and the guard still rejects | Decision 22 requires notifications-only to validate, but decision 11 hides the tab on **capability**, not on config, and those two combine into a hole the current guard is blind to: an Azure-only config with `disabled_panes: pullrequests,workitems,pipelines` passes a naive four-way guard, then hides the notifications tab because no backend implements `NotificationSource`, leaving zero navigable tabs — precisely what the guard exists to prevent, and the metrics tab cannot rescue it since it is separately gated on `metrics.enabled`. `Validate()` can see this without reaching into the provider layer, because in phase 1 GitHub is the only capable backend and `HasGitHub()` already decides whether that backend gets built. This coupling is phase-1-only and must carry a comment saying so: once phase 2 makes Azure capable, the condition widens to "any configured backend" and the special case disappears |
| 48 | Are the nine zero-value `notifications.*` `viper.SetDefault` calls kept, and are the unbounded `max_items`/`since_days` defaults safe? | Kept, with a comment noting they are currently no-ops. Both unbounded defaults are safe and must not be given non-zero values "for protection" | Measured: deleting all nine registrations at once leaves the suite green, because every notifications default is a Go zero value and `mapstructure.Unmarshal` leaves untouched fields at zero — so they are provably inert today. They stay anyway, because `SetDefault` is the correct mechanism and is inert here only by coincidence of the values; delete them and the first genuinely non-zero default has no landing site and gets bolted on somewhere ad hoc. On safety: `max_items` is **rate-limit-neutral by construction** — decisions 31 and 44 make it truncate-on-return (task 7 caches the full walk, the composite re-truncates after the sort), so no value of it changes the request count, and a non-zero default would buy zero protection while silently narrowing the feed. `since_days` is the only lever that does affect fetch cost (it maps to GitHub's server-side `since`), and its worst case is already bounded by decision 29's `maxNotificationPages` (50 × 100 = 50 requests per non-304 refresh) against a 5,000 req/hr PAT, with decision 27's conditional request making an unchanged inbox cost one free 304. Reaching the ceiling needs a >4,900-thread inbox that also changes every 60s; the page constant, not a default, is the real bound. Task 20 documents `since_days` as the knob for very large inboxes |
| 49 | Which package owns the config-driven filter? | `internal/ui/notifications` (task 10 creates the package, task 11 fills in the pane), as an **exported** `FilterNotifications(rows []provider.Notification, cfg *config.Config) []provider.Notification` | It cannot live in `internal/provider`: task 9 added a `config → provider` import edge, so a config-aware function there would close a cycle. `internal/config` would compile but puts feed logic in the settings layer. The pane package is where every other per-pane filter already lives (`filterPR`, `filterWorkItem`, `filterFlagsByReason`), and `internal/ui/metrics` already imports `internal/config`, so the edge is established rather than new. Exported because two callers need it: the pane applies it on receipt, and decision 21 counts the badge **after** filtering, from `app`. It takes the whole `*config.Config`, not just `NotificationsConfig`, because `only_configured_repos` resolves against `github.repos` |
| 50 | What does "filter precedence" mean — override or intersection? | The two **selection** knobs override: `only_configured_repos: true` wins and `include_repos` is ignored (with a load-time warning that it is being ignored). Everything after is **subtractive** and composes: `exclude_repos`, then `exclude_reasons`, then `unread_only` | The Config-shape section orders these "most to least specific", which is override language, and the two repo selectors genuinely conflict: intersecting them makes `only_configured_repos: true` + `include_repos: [other/*]` an empty feed, which is the worst outcome for a knob the user asked for as a convenience. Override makes the strict mode strict and keeps the failure mode "you see your configured repos", never "you see nothing". Silently-ignored config is its own trap, hence the warning — routed through decision 46's `Config.Warnings`, the same channel decision 26 already uses. The subtractive knobs cannot conflict by construction (each only removes rows), so AND is the only sensible reading there and no knob can ever widen the feed |
| 51 | Glob syntax, case, and what a malformed pattern does | `path.Match` against `Identity.Scope`, with pattern and scope both lower-cased first. A pattern `path.Match` rejects as `ErrBadPattern` is **dropped at load** with a warning, exactly as decision 26 drops an unrecognised reason; an entry that is empty after `strings.TrimSpace` is a hard config error, extending the existing empty-entry check | Dropping at load rather than failing at match time means the filter's contract is "every pattern it receives compiles", mirroring what task 9 already guarantees for `exclude_reasons`, and it fails **open** — a malformed `include_repos` reverts to the whole inbox instead of emptying the feed. Lower-casing both sides is required because GitHub owner and repo names are case-insensitive while `Scope` carries the wire's canonical casing, so a user typing `Acme/Repo` would otherwise silently match nothing; lower-casing a glob is safe since no metacharacter is affected. `"   "` is a valid pattern that matches nothing, which for `include_repos` empties the feed — it is a typo every time, and the existing check already treats `""` as an error, so trimming is the consistent fix rather than a new warning class. Task 20 must document that `*` does not cross `/` (`*/*`, not `*`, means "everything") |
| 51a | What must the filter itself do with an uncompilable pattern, given the load-time drop makes it unreachable? | **Mirror what the sanitizer would have produced**: skip uncompilable patterns, and if an `include_repos` list has no compilable pattern left, treat the list as empty — i.e. select everything — rather than selecting nothing | Measured: "treat as no match" is fail-**closed** for `include_repos` and fail-open only for `exclude_repos`. `include_repos: ["[bad"]` returns zero rows; `exclude_repos: ["[bad"]` returns all of them. Decision 51's fail-open guarantee therefore held only because `LoadFrom` drops the pattern first, and it is *not* structural: a `&config.Config{}` literal — which is exactly how tasks 11–13 build fixtures — bypasses the sanitizer and gets the empty feed. Mirroring the sanitizer makes the filter's output identical whatever the construction path, which is the property worth having; "no match" is the wrong default because for a selection list, matching nothing and matching everything are not symmetric failure modes |
| 52 | Filter purity: which knobs, and may it reuse the input's backing array? | Only the five knobs in the precedence chain. It must allocate a new slice — **never** `rows[:0]` in place — must not mutate any element, and must preserve input order | `participating_only` and `since_days` are fetch-time (`NotifOpts`) and `max_items` is applied by the composite after the sort (decisions 31, 44); re-applying any of them here would double-filter. In-place filtering is the specific trap: the pane keeps the **unfiltered** feed so it can re-apply task 11's interactive `f` reason filter without refetching, and `rows[:0]` would corrupt that feed the first time the config filter dropped a row. Order must be preserved because decision 45's total order is established upstream in the merge and re-sorting here would fight it. Defence in depth: task 9 guarantees every surviving `exclude_reasons` entry parses, but the filter still checks `ParseNotificationReason`'s bool and skips an unrecognised entry rather than letting it degrade to `Other` and silently trim rows the user never asked to hide (decision 26) |
| 52a | Is "allocates a new slice" uniform across every return path, including nil-cfg and no-knobs-set? | Yes — **every** path returns a freshly allocated slice, with no exception for `cfg == nil` or the zero-value config | Measured: two paths returned the caller's own slice or its backing array — `cfg == nil`, and the no-knobs `default:` branch if written `out = rows`. Both are safe only as long as no caller ever mutates or appends, which is an invariant living in a doc comment rather than in the code, and the `default:` branch is the **shipped default config** — the most-exercised path in the product, and the one with the weakest guarantee. A conditional contract ("aliased sometimes") is also the kind a reviewer or a later caller has to re-derive; making it unconditional costs one `append` on a path that already walks the slice. Uniform allocation is what lets task 11 hold the unfiltered feed and re-apply its interactive `f` filter without a defensive copy of its own |
| 53 | Is `f` a picker or a cycle, and in what order does it cycle? | A **cycle**, mirroring `internal/ui/metrics/list.go`'s `f`, with an "all reasons" position so the filter is always escapable. The cycle visits reasons **present in the loaded feed**, in **enum order** — never feed order, never the full enum | The `f`-cycles idiom already exists in this codebase and a picker modal for at most eleven values is more machinery than the job needs. Enum order is the load-bearing part: deriving the order from the rows means the next poll can reorder the cycle under the user's fingers, so the same number of presses lands somewhere else — the feed is sorted `UpdatedAt` descending (decision 45) and therefore reshuffles constantly. Restricting to reasons present is task 11's stated criterion; it also keeps `Unknown` out, which no mapped row can carry (decision 18) and which would otherwise be a cycle stop that matches nothing. The "all" position must be reachable by cycling alone — a filter a user cannot clear without knowing a second key is a trap |
| 54 | How does the `f` filter compose with `listview`'s own search filter without breaking convention 7? | The pane keeps the config-filtered feed in its **own** field and applies `f` by calling `SetItems(reasonFiltered(feed))`. It never feeds a second, narrower slice to `ToRows` alone | `listview.applyFilter` and `SetItems` both pass **one** slice to `effectiveColumnSpecs` *and* `ToRows`, which is exactly why convention 7 holds today. A second filter layer that narrowed only the rows would reintroduce the column/cell divergence convention 7 exists to prevent, and per convention 8 it would surface as a `table.renderRow` panic rather than a failed assertion. Routing `f` through `SetItems` keeps the invariant structural instead of a rule the pane has to remember, and it composes with `/` for free — `SetItems` re-applies the search query when one is active. Decision 52's uniform-allocation contract is what makes holding the unfiltered feed safe: the pane can re-filter from it repeatedly without a defensive copy. **Superseded in part by decision 57**: the pane sets no `FilterFunc`, so the search half of this is unreachable in phase 1 and the `IsSearching()` guard is untestable future-proofing, not a live path |
| 55 | Does the pane restore the cursor by index or by identity when `f` collapses the feed? | By **identity**: capture the selected row's `Identity` before re-filtering, then `FindIndex(SameItem)` + `SetCursor` after, falling back to `listview`'s clamp only when the item was filtered out. Fixtures must leave **≥2 rows** after the collapse with the survivor at a **non-zero, non-last** index | Measured: cursor restore in `listview.setColumnsAndRows` is purely positional — it saves `table.Cursor()` and re-applies it clamped — so the item under the cursor is not tracked at all. A 4-row probe (cursor on index 1, whose row *survives* the filter) lands on a different row after filtering. Both the validator and the reviewer independently found that task 11's collapse fixtures narrow to exactly **one** row, where index 0 is the only valid index and *any* clamp satisfies the assertion — including resetting to 0, which a mutation confirmed survives. This is the failure mode decision 45 exists to prevent: decision 45 canonicalised the *sort* to protect an index-held cursor, and an in-pane filter reintroduces the same hazard from the other direction, with task 14's `d` marking the wrong row done as the consequence. The in-range half of the criterion *is* genuinely pinned (deleting the clamp fails tests in both `notifications` and `listview`); only the same-item half needs the fix |
| 56 | How is convention 6's "named style, not a substring" asserted when the test binary strips styling? | Assert the **style object**, not rendered text: extract a `titleStyle(n, s) lipgloss.Style` and assert its `GetBold()`/`GetForeground()` against `styles.Styles`' own fields, plus that the read and unread styles differ. Forcing a color profile is the acceptable fallback, not the primary form | Measured: lipgloss resolves the `Ascii` profile in a test binary, so `s.Title.Render(x) == x` and styled/unstyled output is byte-identical. Deleting `titleCell`'s entire unread branch left the suite green — both existing assertions collapse to `"Unread title" == "Unread title"`, one of them by comparing the function under test against itself. A `GetBold()` check on `styles.Title` pins the *theme's* definition, not that the cell applies it, so it cannot fail when the cell stops using the style. Asserting the style object is theme-independent and cannot go vacuous when a palette changes, which a rendered-bytes comparison can; `internal/ui/components/table/table_test.go:97` sets the `SetColorProfile(TrueColor)` precedent for cases where only bytes are reachable. This generalises past this task and is a reflection-step candidate for `## Proposed`. **Amendment:** "read and unread must differ" must **not** be read as "give the read branch a named foreground style too". The read branch is the **empty** `lipgloss.NewStyle()`, whose `Render` emits nothing, so the cell inherits `table.renderRow`'s `Cell`/`Selected` styling — which is what every sibling pane does (`pullrequests/list.go:531` emits `pr.Title` bare). The first hardening attempt used `styles.Value` because an empty style has no foreground to compare, and that ships a real regression: `renderRow` wraps each cell in `Cell` inheriting `Selected` on the cursor row, so an inner foreground SGR overrides the selection's foreground for that one cell, and `styles.Value` is `Foreground(theme.Foreground)` — invisible on any theme where `Foreground` equals `SelectBackground`. The Matrix theme sets both to `#00ff41` (`themes.go:467`, `:474`), so a read title on the selected row would render green-on-green. Assert the read branch's *absence* of styling instead: `GetForeground() == lipgloss.NoColor{}`, `GetBold() == false`, and the rendered cell equal to the bare title even under a forced TrueColor profile. Unread rows accept the selection-override trade deliberately — persisting the emphasis is the point |
| 57 | Does the notifications pane get `listview`'s text search, given `f` is the repo-wide search key? | **No search in phase 1** — `FilterFunc` stays nil and `f` is unambiguously the reason cycle on this pane. In exchange the active cycle position must be **observable**: an exported accessor on the pane, rendered as a `Filter: <reason>` indicator, and task 17's help must state the per-pane meaning of `f` | `f` is the search key on every other `listview` pane (`listview.go:208`, gated on `FilterFunc != nil`; pipelines, pullrequests and workitems all set it) and `help.go:80` documents it globally as "Search / filter". Notifications repurposing it is a real inconsistency, but the `f`-cycle idiom is equally established (`internal/ui/metrics/list.go:432`) and decision 53 settled the key — re-keying the cycle to buy a search this pane does not have yet trades a documented exception for a worse one. What is *not* acceptable is the invisibility: measured, filtering to `Mentioned` and then polling a feed with no mentioned rows renders the **empty-inbox** text while a user-set filter hides every row. Metrics already renders `Filter: …` for exactly this reason. The accessor lands in task 11 because tasks 13 and 16 cannot tell "you're clear" from "your filter hides everything" without it, and adding it later means reopening this file |
| 58 | Is `—` the fallback for an empty `Title` too, and may `r` leave the pane spinning? | Yes to the dash — `Title` routes through the same `dashIfEmpty`, dashing *before* the unread style so an untitled unread row still shows it. And no: pressing `r` must not strand the pane on a spinner while the `Fetch` hook is still a stub | `Title` comes verbatim from `thread.Subject.Title` with no fallback, and the criterion's own justification for the `ScopeDisplay` dash — "a blank cell reads as a rendering bug rather than as missing data" — applies with more force to the 60%-width column than to the repo cell it actually names. On `r`: `listview.updateList` sets `loading = true` before batching the fetch cmd, so a `nil` cmd means nothing ever calls `HandleFetchResult` and the spinner never clears — measured as a permanent one-keypress dead end. It self-heals when task 15's poller lands, but the loop ticks task 11 before then and no later task's criteria mention `r`, so it must be closed here (intercept `r`, or have the stub emit an immediate no-change result) and task 15 removes the stopgap when the real fetch arrives |
| 59 | What is a valid "incapable backend" fixture for the decision-11 capability gate? | A capable-**shaped** but incapable provider — `provider.NewCompositeProvider(azdevops.NewAdapter(nil))`. A `nil` provider is **not** an acceptable stand-in, and `hasNotificationCapability` must route through `CompositeProvider.HasNotifications()`, never a bare `p.(provider.NotificationSource)` assertion | Measured independently by the validator and the reviewer: every "incapable" fixture passed `nil`, which fails *any* type assertion, so replacing the whole gate with the naive `_, ok := p.(provider.NotificationSource); return ok` leaves the entire `internal/app` suite green. That naive form is not merely unpinned, it is **wrong**: `*CompositeProvider` implements `NotificationSource` unconditionally, so an Azure-only composite satisfies the assertion and the tab ships to Azure-only users — the precise outcome decisions 1 and 11 exist to forbid, and the one the function's own doc comment claims to prevent. `internal/azdevops` contains zero `Notification` occurrences, so that composite is genuinely incapable and makes the distinction observable. This is the optional-capability pattern's characteristic trap: the fan-out wrapper always satisfies the interface, so capability must be asked of it as a *question* rather than inferred from its type, and a nil fixture can never tell the two apart |
| 60 | What must a tab-presence test assert — the tab-bar label or the pane body? | The **pane body**. Asserting the tab strip's `"1: Notifications"` substring is not sufficient | Measured: deleting `case TabNotifications:` from `View()`'s content switch makes the Notifications tab render the **pipelines** pane, and the suite stays green because the presence test pins only the label. Convention 8 is satisfied in letter — the test does render through `View()` after a `WindowSizeMsg` — and still misses it, because a fallthrough produces a perfectly valid render of the wrong pane. The sibling tests already do better (`TestModel_View_ShowsPullRequests_WhenActiveTab` and `..._ShowsWorkItems_...` both pin content), so this is a local regression from established practice rather than a new standard. The discriminating string for the empty pane is `"No notifications found."`, present with the case and absent without it. This generalises past task 12 and is a reflection-step candidate for `## Proposed` |
| 61 | Where does "is the notifications tab enabled" live, and must the zero-value pane be safe? | **One** place: `buildEnabledTabs`. `NewModel`'s `notifTabEnabled` and the help modal's tab-name list both **derive** from the computed `enabledTabs` and never restate the `IsPaneEnabled && capable` predicate or the tab order. And the pane is constructed **unconditionally**, so no zero value is ever reachable | Measured: the predicate is written three times and the order a third time, and dropping the `IsPaneEnabled` conjunct from `NewModel`'s copy survives, because the three `buildEnabledTabs` unit tests cover capable-plus-pane-disabled but nothing that *renders* it — leaving the help modal and the tab strip free to disagree. On the zero value: the comment claiming it is never reached is false, because the `WindowSizeMsg` handler calls `m.notificationsView.Update(contentSize)` unconditionally and `ThemeSelectedMsg` reconstructs the pane unconditionally. It is also not inert — `Init()` and `SetFeed` panic on the zero value's nil `*LoadingIndicator` (`internal/ui/components/spinner.go:40`). Nothing routes those to a disabled pane *today*, so this is latent, but tasks 15 and 16 deliver messages to this pane from the top-level switch and a future implementer will trust that comment. Constructing unconditionally is measured behaviour-preserving and removes the hazard instead of documenting it |
| 62 | What must the `main.go` wiring test pin — the callee or the argument? | Both. Asserting the callee is `NewAdapterWithNotifications` is half the mutation space; the test must also assert the second argument is not `nil` and that a `github.NewNotificationsClient` call appears at the call site | Measured: the AST walk matches only `sel.Sel.Name`, so deleting the `ghNC := github.NewNotificationsClient(token)` line and passing `nil` keeps the suite green — exactly the state the test's own doc comment says it prevents ("`a.nc` stays nil forever… every List call returns 'no notifications client configured'"). Reverting to `NewAdapter` is the easy half and *is* killed; the hard half was unguarded. Note this is a gap in the **test**, not a live bug: `NewNotificationsClient` always returns non-nil with no error, `GetGitHubToken()` has already errored out upstream, and `GitHubConfig` carries no base-URL/GHE field, so `nc == nil` is unreachable from any real config. The value of pinning the argument is that it stays unreachable |
| 63 | Task 13 names "capability-unsupported" as one of three render states — is it reachable, and what is the third state if not? | **Not reachable through the tab**, and the genuinely reachable third state is **filter-empty**. Task 13 renders four states — empty inbox, filter-empty, capability-unsupported, error — but the capability arm is asserted at the **pane** level with its unreachability stated in the doc comment, and the app-level tests cover empty / filter-empty / error. Every pair must be asserted mutually distinguishable | Measured: `CompositeProvider.HasNotifications()` (`internal/provider/composite.go:602`) is a per-backend type assertion, and `*github.Adapter` satisfies `NotificationSource` unconditionally — `NewAdapter` leaves `nc` nil on purpose and `List` returns `"no notifications client configured"` rather than failing the assertion (`internal/github/adapter.go:55-65`, `:584-587`). So the only incapable configuration is Azure-only, which decision 11 hides the tab for, and phase 2 makes even that capable. An app-level "capability-unsupported renders X" test is therefore a test that cannot fail for the right reason, and shipping one would be the same vacuity class as task 12's `nil`-provider fixture (decision 59). What *is* reachable — and measured in decision 57 — is a user-set `f` filter matching zero rows rendering the empty-inbox "you're clear" text, actively lying to the user. That is the state worth a distinct render, and it is why decision 57 put `ReasonFilter()` in task 11. The nil-client message stays reachable-in-principle and belongs to the error arm, not the capability arm: it is a failed `List`, not an absent capability |

| 64 | Does `View()`'s `!m.list.Loading()` conjunct actually protect the initial fetch? | **No.** `listview.Init` sets the spinner visible but never sets `m.loading`, so `Loading()` is false while the first fetch is in flight. The conjunct guards only the `r`-refresh path, which this pane swallows — it is dead today and correct-in-intent, so it stays, pinned by a test. **Task 15 must set `loading` on the initial fetch** (or move this pane off listview's flag) or the pane renders "you're all caught up" during startup | Measured: `listview.Init` (`internal/ui/components/listview/listview.go:151-154`) does `m.spinner.SetVisible(true)` and batches `config.Fetch()`, with no assignment to `m.loading`; only `updateList`'s `case "r"` sets it (`:202-205`). So today the only way to reach `Loading() == true` on this pane is to send `r` to the inner listview directly, because the pane intercepts `r` at its own level (decision 58's stopgap). Dropping the conjunct therefore survived the whole suite. It is kept rather than deleted because it becomes load-bearing the moment task 15 wires a real fetch, and deleting-then-restoring it is how the lie ships: an empty-inbox "you're clear" render while the first request is still outstanding is strictly worse than a spinner, and it is the same class of error decision 57 already caught once |
| 65 | On a **successful** `u`/`d`, is holding the optimistic override enough, or must the mark be written into the held feed? | It must be **written into the feed** (`commitOverride`), and the override entry **kept** for the rest of the debounce window. The two layers answer two different questions: the feed makes a confirmed mark durable for as long as that feed is held, and the override outweighs a lagging poll that replaces the feed wholesale. Neither alone is correct | Measured, and this was a live user-visible defect, not a theoretical one. With success as a no-op the override was the *sole* holder of the mark, and `visibleItems` drops an override once `!now.Before(ov.expiresAt)` and re-derives from an `m.feed` that still contained the pre-mark row. So a confirmed mark-done reappeared 30s later on any purely local re-render — an `f` cycle, a resize — with **no poll involved**: press `d`, watch the row go, press `f` half a minute later, the dismissed notification is back. Reproduced with a throwaway probe test (three same-reason rows, `d`, a success result, clock advanced past `markDebounceWindow`, one local `f`): "visible after successful result: 2 → visible after local f cycle past the window: 3". This is data resurrection, so widening or removing the window is the wrong fix — an infinite window is a second permanent source of truth, which decision 5 rules out. Committing on success and keeping the entry is the only shape that satisfies both requirements; all four mutations of it are killed (success-as-no-op, commit-unconditionally, commit-sweeps-the-whole-feed, and commit-also-drops-the-override, which resurfaces the very poll-flicker the window exists for). Note the implementer's own 6/6 "no survivors" ledger missed this because no mutation exercised success → expiry → local re-derivation |
| 66 | Is it enough for the pane to handle `MarkResultMsg` "unconditionally" inside its own `Update`? | **No — app must route it unconditionally too.** A pane-bound message whose arrival is uncorrelated with which tab is showing must be handled in app's top-level switch, *before* the delegate-to-active-tab switch. `polling.PipelineRunsUpdated` is the existing precedent. The message type is exported for this and only this; its fields stay unexported so app can forward but neither construct nor inspect one | Measured — this is decision 65's defect reachable by a second route, and it survived 65's first fix. `notificationMarkResultMsg` was unexported, so it matched no case in app's `switch msg := msg.(type)` and reached the pane only via `case TabNotifications:` (`internal/app/app.go:994`). Tab switching is handled earlier (`:786-796`) and returns early, so pressing `2` while a DELETE is in flight is trivially reachable: the result is handed to `m.pullRequestsView`, which discards it, `commitOverride` never runs, and the override is again the sole holder of the dismissal — 30s later any re-derivation resurrects a row the server already accepted as done. The pane-level comment claiming the message is "handled unconditionally, since it can land regardless of view mode or search state" was true inside the pane and false end-to-end, which is what made this invisible. Pinned by an app-level test that switches tabs mid-flight; it asserts on the **failure** path, because a dropped success and an applied success are indistinguishable inside the debounce window and the pane's clock seam is unexported from `app` — routing a failure produces a visible rollback, so "the row came back" proves delivery with no clock involved |
| 67 | What must `handleMarkResult` check before applying an outcome? | Two things beyond the key. (a) The stored override's **kind** must still match the result's — otherwise a newer action on the same row has replaced the entry and neither outcome may be applied on its behalf. (b) The re-derivation must **not** run while `listview.Err() != nil`, because `SetItems` clears `err`/`loading` as a side effect. A success with no entry left is still committed: the server accepted it, and erring the other way resurrects a row | Measured, both. (a) `u` then `d` on one row is reachable — after the `u` the row is still in `Items()` with `Read=true`, and `markDone`'s already-hidden guard only rejects a *live* `overrideHidden`, so the `d` proceeds and overwrites the same map entry with `{hidden}`. Rolling back by key alone then deleted the entry the `d` owned and un-hid a row whose DELETE was still in flight: measured `after d: visible=1` → `after the u FAILURE lands: visible=2` → `after the d SUCCESS lands: visible=1`, i.e. the user watches a dismissed notification reappear and vanish again. This also makes `dropOverride`'s "complete rollback" claim accurate — it is complete for a single action and over-broad once the key has been reused. (b) `listview.SetItems` assigns `m.err = nil` and `m.loading = false` (`internal/ui/components/listview/listview.go:379-392`), so `d` → a failed poll → the DELETE result rendered **"No notifications found. / You're all caught up."** over a failed fetch, hiding the error that explains the absence of data — strictly worse than the empty-inbox lie decisions 57 and 63 already caught, because that error is the actionable part. Both are unreachable until task 15 wires a poller that can fail; both are pinned now so task 15 cannot ship them |

| 68 | Task 16's badge counts "unread after config filters" — over which slice, and does the `f` cycle affect it? | A new exported accessor on the pane (`UnreadCount()`) counting **overrides applied over the whole `m.feed`**, deliberately **not** over `reasonFiltered()`. Two independent halves: overrides must be applied, and the `f` cycle must not be | Measured from the code as it stands. On overrides: `visibleItems` is the only place feed and overrides are combined and it is unexported, so a badge reading `n.Read` off `m.feed` would count a row the user has already cleared with `u` — the count and the rows visibly disagree until the next poll, and for `d` the badge keeps counting a notification that is no longer on screen at all. On the `f` cycle: `reasonFiltered` (`internal/ui/notifications/list.go`) is the **interactive** `f` reason cycle, not a config filter — decision 19's `exclude_reasons`/`exclude_repos`/`unread_only`/`only_configured_repos` are all applied before the feed reaches the pane, so `m.feed` is *already* the post-config-filter slice that decision 21 asks about. Counting over `reasonFiltered` would make the badge drop to a per-reason subtotal the moment the user pressed `f`, i.e. an inbox count that changes because of a local view toggle — and because decision 21 requires the badge visible from **every** tab, that wrong number would follow the user to a tab where the filter that produced it is invisible and unexplainable. So `f` is precisely the one filter the badge must ignore. Zero-count hiding then falls out of the same accessor rather than a second predicate |
| 69 | Can task 15's notifications poller reuse `polling.TickMsg`? | **No.** It needs its own tick type (`polling.NotificationsTickMsg` or equivalent) and its own top-level case. `polling.TickMsg` is a bare `struct{}` marker carrying no identity, and app's single `case polling.TickMsg:` calls `m.poller.OnTick()` — the **pipeline** poller — unconditionally | Measured from the code as it stands, and the failure compounds rather than merely doubling. `TickMsg` is `type TickMsg struct{}` (`internal/polling/events.go:16`), so two pollers emitting it are indistinguishable at the receiving end. `Poller.OnTick` (`internal/polling/poller.go:136-150`) returns `tea.Batch(p.FetchPipelineRuns(), p.StartPolling())` — it both fetches *and* schedules the next `tea.Every`. So a notifications tick reaching `app.go:970` would (a) issue a pipeline fetch that nothing asked for, and (b) start an **additional** pipeline timer chain. Since every one of those chains then emits `TickMsg` and each of those spawns another, pipeline polling frequency grows without bound for as long as the session runs — against an ADO org, over a PAT, with no visible cause in the notifications feature the user just enabled. Decision 8's `max(X-Poll-Interval, configured)` makes this worse, not better: the whole point of honouring GitHub's hint is that the notifications cadence differs from the configured pipeline one, so the two tick streams are guaranteed not to coincide. A distinct message type is the fix; a shared type discriminated by a field is not, because the existing `case` would still match it |
| 70 | Task 17's FORWARD: how does the help modal's global `f` line coexist with a pane where `f` is a reason cycle? | Scope it parenthetically, matching the pattern the modal already uses — `f` becomes "Search / filter (PRs / work items / pipelines)" with the notifications section carrying its own `f` line for the reason cycle. Also in scope for task 17: the `1/2/3` tabs line and the global `r` line | Measured from `internal/ui/components/help.go:70-94`. The parenthetical scope is not a new idea — `m` ("Toggle my items (PRs / work items)"), `A` ("(PRs)"), `T`/`s` ("(work items)"), `S` ("(pipelines)") and `o` ("(PR / work item / pipeline detail)") all already qualify themselves, so an unqualified `f` is the outlier rather than the baseline. Left alone it tells the user of the pane where `f` is a *reason cycle* that the key runs a text search, which is the one binding the notifications section most needs to teach. Two adjacent lines in the same section are wrong for the same reason and belong to task 17's edit, not a later one: `{Key: "1/2/3", Description: "PR / Work Items / Pipelines"}` names three tabs and their old order, which decision 6 has already changed (task 17's criterion covers this); and `{Key: "r", Description: "Refresh data"}` is globally true except on this pane, which swallows `r` under decision 58's stopgap — so if task 15 removes that stopgap first the `r` line becomes correct again on its own, and if it does not, `r` needs the same parenthetical treatment. Task 17 must check which of the two is true at the time it runs rather than assuming |
| 71 | A test that constructs `&config.Config{}` as a literal and then drives any code path reaching `Save()` — what does it write? | **The developer's real `~/.config/azdo-tui/config.yaml`.** Convention 17 is not only about fixture realism; a bare literal leaves `configPath` empty and `Save()` then resolves `GetPath()`. Use `config.NewWithPath(..., filepath.Join(t.TempDir(), "config.yaml"))` in any test that dispatches a message whose handler can save | Measured while closing task 14's re-check. `app.Update`'s `case components.ThemeSelectedMsg:` opens with `m.config.UpdateTheme(msg.ThemeName)`, which calls `Save()` — so a theme-switch test built on a literal `Config` writes to the real user file, which is the incident the user reported from an earlier session. It failed closed in this sandbox only because `~/.config/azdo-tui/` does not exist here; on a real machine it does. The sibling `TestModel_ThemeSwitch_*` tests already use `NewWithPath` + `t.TempDir()`, so this was a local regression, not a gap in the codebase. The second half is what makes it worth a decision rather than a fix: with `Save()` failing, `UpdateTheme` errors and the handler takes its early `return m, nil`, so **the pane rebuild further down is never reached** and a nil-marker mutation at that site survives green. A convention-17 violation and a vacuous test are the same defect here — the unsafe path is also the unobservant one, so honouring the convention is what gave the assertion teeth |

## Tasks

- [x] 1. ADR `docs/adr/0001-notifications-capability-interface.md` — decisions 1, 2, 5. → done: file exists, ≤30 lines, `Status: Accepted`, has Context/Decision/Alternatives/Consequences
- [x] 2. `provider`: `Notification` type (provider-qualified identity + `Read`/`Done` per decisions 14, 15), `NotificationReason` enum (exactly decision 18's values), `NotifOpts`, `NotificationSource` (blocked by: 1). → done: `go build ./...` clean, `gofmt -l` empty, enum values match decision 18 one-for-one
- [x] 3. `ui/display`: reason → glyph + label + style map, plus `String()`/`ParseNotificationReason` next to the enum per decisions 24 and 26 (blocked by: 2). → done: every enum value returns non-empty glyph, label and named style; an unrecognised value renders as `Other`, never empty; table test covers all values plus one unrecognised input; asserts glyph, **exact label** and named style (convention 6 — a non-emptiness check on the label does not satisfy it); `String()` emits the decision-19 lowercase snake_case names and round-trips through `ParseNotificationReason` for all 12 values; `ParseNotificationReason` returns `(NotificationReason, bool)` per decision 26 — `(Other, false)` for an unrecognised string and for `unknown`, never an error and never a dropped row; `String()`'s out-of-range fallback is `"other"`, matching the display layer, with `Unknown` an explicit case returning `"unknown"`
- [x] 4. `github`: user-scoped client — `GET /notifications` with `all=true` (decision 12), pagination, `If-Modified-Since`, `X-Poll-Interval` (blocked by: 2). → done: `httptest` tests assert `all=true` in the query, `Link rel=next` followed, `If-Modified-Since` sent when a cached timestamp exists, 304 returns the cached slice unchanged, `X-Poll-Interval` parsed. Per decisions 27–29: the cache is mutex-guarded and returned **by copy**, so mutating a returned slice cannot change what a later 304 yields — assert that by mutating the first result and re-checking the second, since comparing two aliases of one backing array is a tautology; the cache is invalidated when `buildPath(opts)` changes; a 304 with no cached validator is an error; the walk is bounded by a page constant, tested with a self-referential `next`. Tests must also pin the method and path (`GET /notifications`) and `per_page`
- [x] 5. `github`: wire → neutral mapping, reason mapping, `subject.url` → web URL resolution (blocked by: 4). → done: table test maps every reason string in decision 18 plus an invented unknown → `Other`; `subject.url` resolves for pull/issue/commit and falls back to the repo URL otherwise; no panic on absent optional fields. Per decisions 33-35: `Release` resolves to the repo's `/releases` list page and the test asserts that **from GitHub's URL scheme, not from the implementation**; the id segment is shape-validated (digits for PR/issue, hex for commit) so `.../pulls` with no id, a trailing slash, or a non-numeric id falls back rather than emitting a clickable 404; every per-type URL is built on the `Repository.HTMLURL` prefix so an empty `FullName` can never yield `https://github.com//pull/42` and a GHE host is honoured; `ScopeDisplay` falls back to `Scope` inside the mapper, with a test row
- [x] 6. `github`: mark read (`PATCH /notifications/threads/{id}`) + mark done (`DELETE`) (blocked by: 4). → done: tests assert method and path per call; ids are **strings** on the wire (`NotificationThread.ID`), so convention 11's guard is restated as: reject empty, non-numeric, and `<= 0`-after-parse — a raw `"-5"` interpolated into `/notifications/threads/-5` is the convention-11 failure shape, so keep the negative-input row; one-way read documented in the doc comment, not claimed as API-verified (decision 13)
- [x] 7. `github`: implement `NotificationSource` on `Adapter` (blocked by: 5,6). → done: compile-time `var _ provider.NotificationSource = (*Adapter)(nil)`; conformance test following `adapter_conformance_test.go`; `NotifOpts.Max` is honoured by **truncating on return, not by stopping the walk** (decision 31), with a test proving a `Max` smaller than one page truncates **and** that a later call with a larger `Max` served from cache is not stuck at the smaller one — that cache fixture must span **more than one page** (page 1 carrying a `Link ... rel="next"`, page 2 ending the walk, and `Last-Modified` echoed on every 200 so the second call really is conditional), because a single-page fixture only catches the truncate-before-caching shape: with no `rel="next"` left to skip, a `Max` pushed into `NotificationListOpts` that stops the walk at a page boundary passes green; and `Adapter` forwards `PollInterval()` so it satisfies task 15's `PollIntervalHinter` (decision 30), asserted by a compile-time `var _` against that interface; `Adapter.MarkRead`/`MarkDone` forward straight through and **must not** take any lock, least of all one shared with `List` — decision 37's entire benefit is a lock-free marker, and wrapping it here would silently reinstate the 50-page stall
- [x] 8. `provider`: composite fan-out, merge/sort, `HasNotifications()` (blocked by: 2). → done: fans out only to backends implementing the interface; `HasNotifications()` false with zero capable backends and true with ≥1; merged output sorted newest-first; per decision 20 a failing backend still returns the others' rows and never empties the feed — test that case explicitly; per decision 25 `MarkRead`/`MarkDone` route by `Identity.Kind` and **not** `backendFor(scope)` — test that a row from an unconfigured repo still routes to a backend
- [x] 9. `config`: `notifications` block with decision-19 key names, defaults, validation, `validDisabledPanes` entry, guard accepts notifications-only (decision 22) (blocked by: 2). → done: block loads with documented defaults; keys resolve lowercased (convention 9); `disabled_panes: notifications` validates; a config with only notifications enabled passes `Validate()`; per decision 26 an unrecognised `exclude_reasons` entry produces a **warning naming the bad value and the eleven accepted ones** and is then ignored — it must never silently act as `other`, and must never be a hard config error that stops the app from starting
- [x] 10. Config-driven filter as a pure function (blocked by: 8,9). → done: table tests cover each knob alone, the full precedence chain, the `participating_only` + `exclude_reasons` compose case (decision 10), and that an unrecognised reason is only filtered when `other` is listed explicitly. Task 9 drops unrecognised entries at load, so every entry this filter receives parses — assert that too. Repo globs are **not** validated at load beyond rejecting empty entries: `"   "` and `"[bad"` both load clean, and `path.Match("[bad", …)` returns `ErrBadPattern`, so this task must decide whether a malformed glob warns (consistent with decision 26's warn-don't-die) or matches nothing, and pin it either way
- [x] 11. `ui/notifications`: `listview` pane — dynamic repo column, unread emphasis, `f` reason filter (blocked by: 3,10). → done: renders through `View()` after a `WindowSizeMsg` without panic (convention 8); column count equals row-cell count in both single- and multi-repo cases (convention 7); unread rows assert a named style, not a substring (convention 6) — and per decision 56 that means asserting the **style object**, since lipgloss's `Ascii` profile in a test binary makes a rendered-bytes comparison vacuous; the filter-collapse path and cursor survival are asserted — when the `f` filter narrows the feed enough that the dynamic repo column disappears, assert that transition *and* that the cursor/selection survives the column-count change, because the expand direction passing does not prove the shrink direction, and per decision 55 the same-item half needs an identity-based restore in the pane plus a fixture leaving **≥2 rows** with the survivor at a **non-zero, non-last** index — a one-row survivor makes the assertion true whatever the cursor does (this requirement is stated inline on purpose: it matches `.spec/conventions.md`'s **proposed** convention 14, which is inert until a human promotes it, so it binds here as a spec criterion and must not be cited by convention number); a zero `UpdatedAt` renders as `—`, never as a year-0001 date (the mapper leaves it zero when the wire omits `updated_at`); an empty `ScopeDisplay` gets the same `—` treatment — decision 35 defaults it to `Scope`, but a thread whose `repository` payload is absent yields both empty, the one case the mapper cannot fix, and a blank cell reads as a rendering bug rather than as missing data; the `f` filter offers **only reasons actually present in the loaded feed**, never the full enum — otherwise it lists `Unknown`, which no mapped row can carry (decision 18), as a choice that matches nothing; an empty `Title` gets the same `—` as the other two, dashed before styling (decision 58); pressing `r` while `Fetch` is a stub must not strand the pane on a spinner (decision 58); the active cycle position is reachable through an exported accessor and rendered as a `Filter:` indicator (decision 57); and `display.MultiScope` carries its own table test beside `TestMixedKinds`, including the empty-slice case `listview.go:110` depends on
- [x] 12. `app`: register the tab first, remap number keys, `enabledTabs`, `state.TabID` (decisions 6, 9, 11) (blocked by: 11). → done: notifications is `enabledTabs[0]`; number keys map to the new order, and `cmd/azdo-tui`'s CLI help no longer advertises the old three-tab line; `TabID "notifications"` round-trips through `state.yaml` — actually serialising, not just through the in-memory store snapshot; the tab is absent when no backend implements the capability, and present-but-empty when one does — with the incapable fixture being a capable-**shaped** provider per decision 59 (a `nil` provider cannot tell the correct gate from the naive type assertion), presence asserted on **pane content** per decision 60 (the tab-bar label alone passes when the content switch falls through to a sibling pane), the enablement predicate computed **once** and the pane constructed unconditionally per decision 61, and the `main.go` wiring test pinning the **argument** as well as the callee per decision 62
- [x] 13. `app`: the render states — empty inbox, **filter-empty**, capability-unsupported, error/token-scope (decisions 11, 17, 63) (blocked by: 12). → done: each state is a distinct render asserted by its own test, and every pair is asserted **mutually distinguishable** (a shared substring is not a distinct render); the empty state reads as "you're clear", never as an error; per decision 63 the **filter-empty** state is separated from the empty inbox using decision 57's `ReasonFilter()` accessor — this is the reachable state, and conflating the two is the measured bug decision 57 documents; the capability-unsupported render is asserted at the **pane** level with its unreachability recorded in the doc comment (decision 63), not smuggled in as an app-level test that cannot fail; the error state carries the token-scope skeleton (decision 17) and the nil-client message, with task 19 owning the 403/401 differentiation; the empty and filter-empty bodies must not advertise a key the pane swallows (decision 58's `r` stopgap); and per decision 46 any `Config.Warnings` entry renders in the pane — asserted with a populated warning, and asserted absent when the slice is empty so an empty warnings list never reserves a blank line
- [x] 14. `app`: `u` mark-read (one-way, decision 13) / `d` mark-done (blocked by: 13). → done: `u` issues one mark-read and updates the row optimistically, rolling back on API failure; `d` removes the row and restores it on failure; a poll that returns stale `unread` inside the debounce window does not flicker the row back; and per decision 65 a **successful** mark is committed into the held feed while its override entry is kept, so the mark survives the window elapsing with no poll involved — asserted for both `u` and `d`, plus a failure case pinning that the commit is success-only
- [ ] 15. Polling integration honouring decisions 8 and 23 (blocked by: 12). → done: cadence is `max(X-Poll-Interval, configured)`; the hint reaches the poller via the separate `PollIntervalHinter` optional interface, **not** a new `NotificationSource` method (decision 23); a backend that does not implement the hinter falls back to the configured interval; a 304 response leaves the existing list intact rather than clearing it
- [ ] 16. Unread-count footer badge (decision 21) (blocked by: 12). → done: count is unread *after* config filters; the badge is hidden entirely at zero; visible from every tab
- [ ] 17. Help-modal section + `RemoveSection` wiring when the pane is disabled (blocked by: 12). → done: section lists `u`/`d`/`o`/`f`; disabling the pane removes it; the tabs binding line reflects the new order
- [x] 18. `config`: regression test pinning `Save()` preservation — seed a file containing `metrics:` and `notifications:`, change only the theme, assert both blocks survive with every value intact (blocked by: 9). → done: the new test fails if `ReadInConfig()` is removed from `Save()` (verify by deleting it locally, watching the test fail, restoring it); fixtures go through `LoadFrom(<t.TempDir() path>)`, never a bare `Config` literal (convention 17). Also pinned: a key the `Config` struct does not model at all survives — the general form of the requirement, and the only assertion here that can see such a key, since every other check reads the reloaded typed `Config` and is blind to sections outside it
- [ ] 19. Token-scope error state (decision 17) (blocked by: 13,18). → done: a 403 missing-scope response renders in-view naming the `notifications` scope and how to add it; 401-expired and generic failures render differently; the disable action writes `disabled_panes` via `Config.Save()` behind a confirm, on a key that is not `d` or `u`; its test uses a temp-path config (convention 17)
- [ ] 20. Docs: README (required `notifications` scope, upgrade note for existing tokens, how to disable), Architecture.md, config.yaml.example, FAQ (blocked by: 17,19). → done: every config key from decision 19 is documented; the required token scope and the upgrade path for existing tokens are stated; `exclude_reasons` values are documented as lowercase snake_case and **case-sensitive** (`Subscribed` warns and is dropped — viper lowercases config *keys*, never list values), `unknown` is documented as reserved and not accepted, and `since_days` is named as the knob for very large inboxes per decision 48. **Do not reproduce the Config-shape arrow chain as if it were a pipeline** — measured, all five knobs are independent row predicates and every stage order is observationally identical, so "precedence" describes exactly one thing: the two *selection* knobs override each other (`only_configured_repos` wins, decision 50). `exclude_repos`/`exclude_reasons`/`unread_only` are an order-independent AND and must be documented as such
- [ ] 21. Fold phase-1 answers into `20260729-notif-p2-azdo.md` "Inputs from Phase 1" (blocked by: 20). → done: no `TBD` remains in that section

## Validation: `ui/notifications` listview pane (task 11) — 2026-07-30, commit `040911d`

Gates all green: `go build ./...`, `go vet ./internal/...`, `go test -count=1 ./...` (exit 0),
`gofmt -l` clean on the three touched Go files, no diff in `internal/github`,
`internal/provider`, `internal/config`, no scope creep into tasks 12–16, `git status` clean.

Verified as satisfied: render through `View()` after a `WindowSizeMsg` in both single- and
multi-repo cases (and the render test is *real* — measured: making `toRows` prepend the repo
cell while `toColumns` stays gated makes `TestView_RendersAfterWindowSizeMsg_SingleRepo` panic
and fail); column/row-cell parity from one predicate (`multiRepo`) over the same slice in both
`toRows` and `toColumns`; zero `UpdatedAt` → `—` with an explicit no-`0001` assertion; empty
`ScopeDisplay` → `—`; `f` cycle restricted to reasons present in the feed, in enum order, never
`Unknown`, with the "all" position reachable by cycling alone; decision 54 wiring — `f` only ever
reaches `listview` through `SetItems(m.reasonFiltered())`, nothing hands a narrower slice to
`toRows`.

**Missing — one criterion:** *"unread rows assert a named style, not a substring (convention 6)"*
is not actually pinned by any test. Measured with a mutation on a scratch copy: replacing
`titleCell`'s unread branch `return s.Title.Render(n.Title)` with a bare `return n.Title` (i.e.
deleting the named style entirely) leaves **both** `TestToRows_UnreadRow_UsesNamedTitleStyle` and
`TestTitleCell_ReadVsUnread_StructurallyDifferentPaths` green. Two reasons:

1. `TestToRows_UnreadRow_UsesNamedTitleStyle` compares `rows[0][1]` against `titleCell(items[0], s)`
   — the production function under test — so it is a tautology whatever `titleCell` does.
2. `TestTitleCell_...` compares against `s.Title.Render(...)`, but the test binary has no TTY, so
   lipgloss resolves the `Ascii` profile and `Render` is the identity function — styled and
   unstyled output are byte-identical. `s.Title.GetBold()` pins `styles.Title`'s own definition,
   not that `titleCell` applies it.

Fix (in-repo precedent exists — `internal/ui/components/table/table_test.go:97`, *"Force color
output so ANSI escapes are actually emitted"*): call `lipgloss.SetColorProfile(termenv.TrueColor)`
in the style test, then assert the unread cell **differs** from the plain title and equals
`s.Title.Render(title)`, and that the read cell equals the plain title. Re-run the same mutation
(drop the style, watch it fail, restore) before ticking.

Non-blocking notes for the same pass:

- Cursor survival is asserted only as "in range". The *same-item* clause in
  `TestCycleReasonFilter_Collapse_RepoColumnDisappears_CursorSurvives` is trivially true: exactly
  one row survives the filter, so index 0 is the only valid index and `Items()[0].ID == "2"`
  regardless of cursor behaviour. `list.go` contains no `SetCursor`/`FindIndex`/`SelectedIndex`
  call, so identity preservation is entirely `listview.setColumnsAndRows`'s clamp-to-last-row and
  the pane would pass the same assertion if it reset the cursor to 0. If the same-item property is
  wanted, keep ≥2 rows after the collapse and put the cursor on a surviving non-first row.
- `TestUpdate_FKey_NoopOutsideListMode` never presses `f` outside list mode — it only asserts the
  precondition `GetViewMode() == ViewList` and pins nothing. Either drive a detail-mode/search
  state and assert `f` is forwarded rather than consumed, or drop the test.
- `display.MultiScope` is a new exported function with no direct test in `display_test.go`, whereas
  its sibling `MixedKinds` has `TestMixedKinds` (display_test.go:273). Add the mirror table test.

## Validation: app tab registration (task 12) — 2026-07-30, commit `9429c9e`

Gates all green: `go build ./...`, `go vet ./internal/... ./cmd/...`, `go test -count=1 ./...`
(exit 0), `gofmt -l` clean on all seven touched Go files, `git status --porcelain` empty.

Verified as satisfied:

- **`main.go` wiring is genuinely pinned.** Measured: reverting line 321-322 to
  `backends = append(backends, github.NewAdapter(ghMC))` makes
  `TestRunTUI_UsesGitHubAdapterWithNotifications` fail on **both** assertions
  (missing `NewAdapterWithNotifications`, present bare `NewAdapter`). The AST walk keys on
  `SelectorExpr{X: ident "github"}` so it cannot match a comment, and `azdevops.NewAdapter`
  does not trip it. Restored, md5 verified.
- **Notifications is `enabledTabs[0]`** (`buildEnabledTabs` prepends it) and `disabled_panes`
  still applies on top.
- **Number keys.** The digit set is now `"1".."5"` and the mapping is purely positional into
  `m.enabledTabs`; `TestModel_DigitKeys_MapToNewOrder_AllFiveTabs` drives all five with every
  tab enabled. No hardcoded tab index survives anywhere — every `switch m.activeTab` in
  `app.go` gained a `TabNotifications` case (Update delegation, `isActiveViewSearching`,
  `resizeActiveViewIfNeeded`, `syncStatusBarContext`, `View`, `initTabCmd`, keybindings), and
  no other package references `app.Tab*`.
- **No `CurrentVersion` bump** — `state.CurrentVersion` is still `2`; `TabNotifications` is
  additive and `state.Load` does no ID validation.
- **Degradation.** `TestApplyState_IgnoresIncapableNotificationsTab` restores a state naming
  `notifications` against an incapable model, lands on `TabPullRequests`, and calls `View()`.
- **Convention 8.** Both app-level presence tests render through `View()` after a
  `WindowSizeMsg`; mapping `tabIDForTab(TabNotifications)` to `state.TabPipelines` fails the
  round-trip test (mutation killed).
- **Scope is clean.** No `u`/`d`, no poller/`PollIntervalHinter`, no footer badge, no render-state
  branching beyond presence, no doc prose. The five new `internal/ui/notifications/list.go`
  methods are pure one-line forwarders to `m.list`.

**Missing — one criterion:** *"the tab is absent when no backend implements the capability"* is
not pinned against a production provider shape, and the gate is measurably switched off.

`TestModel_NotificationsTab_Absent_WhenIncapable` passes `NewModel(nil, …)`. A nil
`provider.Provider` fails `hasNotificationCapability`'s type assertion outright, so
`CompositeProvider.HasNotifications()` — the actual per-backend `provider.NotificationSource`
assertion — is never reached. Production always builds a `*CompositeProvider` (`main.go:296+`),
which satisfies `notificationCapableProvider` unconditionally.

Measured mutation, **SURVIVED** (whole `./internal/app/...` suite green):

```go
func hasNotificationCapability(p provider.Provider) bool {
	_, ok := p.(notificationCapableProvider)
	return ok            // HasNotifications() result discarded
}
```

Under that mutation an Azure-only config shows the notifications tab — exactly the hole
decisions 11 and 47 exist to close — with nothing failing. (`internal/azdevops` contains zero
occurrences of `Notification`, so an Azure-only composite really is incapable; the correct
behaviour is in the code today, only untested.)

Fix, measured to work: swap that one fixture to a capable-shaped-but-incapable composite —

```go
m := NewModel(provider.NewCompositeProvider(azdevops.NewAdapter(nil)), client, cfg, "dev", "")
```

Unmutated it passes; with the mutation above it fails on both assertions. `app_test.go` already
imports `azdevops` and `provider`, so no new imports are needed. Re-run the mutation (drop the
`HasNotifications()` call, watch it fail, restore) before ticking.

Non-blocking notes for the same pass:

- `tabFromID`'s `case state.TabNotifications` is **vacuously** covered: deleting the case leaves
  the suite green, because `TabNotifications == 0` is `Tab`'s zero value *and* is always
  `enabledTabs[0]` when capable, so the ignored-restore path lands on the same tab as a
  successful one. Functionally harmless; if the mapping is wanted pinned, assert
  `tabFromID(state.TabNotifications)` directly.
- The round-trip test never touches `state.yaml`: it asserts `store.State().ActiveTab`
  (in-memory) and feeds `store.State()` back into `ApplyState`, with no `store.Flush()` and no
  `state.Load`. `TabID` is a plain string with no load-time validation so the disk half is
  low-risk, but a `Flush()` + `state.Load(path)` would make the criterion literal.
- `cmd/azdo-tui/main.go:108` still prints `1/2/3  Switch tabs (Pull Requests, Work Items,
  Pipelines)` in the CLI `--help` text — stale once notifications takes slot 1. Probably task 20's
  to fix; recorded here so it is not lost.

## Review feedback: `ui/notifications` listview pane (task 11) — 2026-07-30, commit `040911d`

Opus review: REQUEST_CHANGES. 17 mutations run — all 7 the implementer reported are genuinely
KILLED, plus 10 new ones of which **6 SURVIVED**. Findings 1–3 and the `f`-key one are now
decisions 55–58; the rest are listed here with their surviving mutation.

Must fix:

1. **🔴 Same-item cursor survival does not hold** — `list.go:124`, `:153`; assertion at
   `list_test.go:388-394`. Per **decision 55**: restore by identity, and change the fixture so the
   survivor sits at a non-zero, non-last index with ≥2 rows left. Surviving mutation: clamping the
   saved cursor to `0` instead of `len(rows)-1` breaks nothing.
2. **🔴 Convention 6 is materially vacuous** — `list.go:271-276`; assertions at `list_test.go:145`,
   `:166`. Per **decision 56**: extract `titleStyle` and assert the style object. Surviving
   mutation: `titleCell` → `return n.Title` (all unread emphasis deleted).
3. **🟡 `display.MultiScope` has no test anywhere** — `display.go:161-171`. Surviving mutation:
   `MultiScope([]) → true`, even though `listview.go:110` documents `MixedKinds([]) == false` as
   load-bearing for the initial `ToColumns(nil)`. Add the mirror table beside `TestMixedKinds`:
   `nil`, `[]`, one element, all-same, all-empty, two distinct, empty-plus-nonempty.
4. **🟡 The active `f` position is invisible** — `list.go:37-40`. Per **decision 57**: add the
   exported accessor and the `Filter: <reason>` indicator.
5. **🟡 An empty `Title` renders a blank cell** — `list.go:255`, `:271-276`. Per **decision 58**,
   route it through `dashIfEmpty` before styling. (The em-dash audit is otherwise clean: the pane
   and its test contain exactly one non-ASCII punctuation codepoint, `U+2014`.)
6. **🟡 `r` strands the pane on a permanent spinner** — `list.go:78`. Per **decision 58**.
7. **🟡 The vanished-reason branch is untested** — `list.go:146`. Surviving mutation: dropping
   `idx < 0 ||` so a selected reason absent from a refreshed feed jumps to `present[0]` instead of
   "all". This is the branch finding 4's scenario runs through, and a live poller hits it routinely.
8. **🟢 Column widths are unpinned** — `list.go:229`. Surviving mutation: dropping
   `listview.NormalizeWidths(cols)`. Multi-repo specs sum to 120% unnormalised, so the table
   over-widens past the terminal with nothing failing. Assert the specs sum to 100.
9. **🟢 The gating field is unpinned** — `list.go:214`. `multiRepo` gates on `Identity.Scope` while
   the cell renders `Identity.ScopeDisplay`; surviving mutation swaps the gate, because every
   fixture sets them equal. Not a convention-7 risk (count parity holds either way), but two scopes
   sharing a display name yield a disambiguating column that disambiguates nothing. One fixture
   with `Scope != ScopeDisplay` closes it.
10. **🟢 `TestUpdate_FKey_NoopOutsideListMode` pins nothing** — `list_test.go:428-438`: it never
    presses `f` and never leaves list mode, so deleting the `!IsSearching() && ViewList` guard
    survives. Per decision 57 the `IsSearching()` half is unreachable in phase 1 — drive
    `ViewDetail` and assert `f` does not cycle, or delete the test rather than leave a name
    promising coverage it does not have.
11. **🟢 The convention-8 `defer recover()` guards the wrong statement** — `list_test.go:101-106`,
    `:118-123`. `View()` returns pre-rendered viewport content; every `table.renderRow` call happens
    inside `SetFeed → SetItems → setColumnsAndRows → SetRows → UpdateViewport`. Confirmed under
    mutation: the panic stack bottoms out at `list_test.go:113`, five lines *above* the `defer`.
    The test still fails (`tRunner` recovers), so the criterion is met — but move the `defer` above
    `SetFeed` so the failure message points at the call that can actually panic.
12. **🟢 Dead field** — `list.go:30`: `styles` is assigned at `:90` and never read (`toRows` takes
    `s` from `listview`). Remove it, or note which later task needs it.

Confirmed sound under mutation, do not re-litigate: convention 7's parity is **structural** — every
row-narrowing path in `listview` (`applyFilter` :275/:285, `SetItems` :382, `HandleFetchResult`
:400, `exitSearch` :239) passes one slice to both `effectiveColumnSpecs` and `ToRows`, and the
`WindowSizeMsg` branch (:187) derives its basis from the same set the rows came from. Decision 54's
`SetItems`-only routing is faithful. Decision 53's cycle is correct and well-pinned in all four
directions (enum-not-feed order, present-not-full-enum, never `Unknown`, "all" reachable).

Deferred, not to be fixed here: `FORWARD: task 12/15` — the `r` stopgap from decision 58 comes out
when the real fetch lands. `FORWARD: task 13` — render the `Filter:` indicator using decision 57's
accessor, and distinguish "you're clear" from "your filter hides everything". `FORWARD: task 17` —
the help section must reconcile `help.go:80`'s global "Search / filter" line with a pane where `f`
is a reason cycle and no search exists. `FORWARD: task 13` — `enter` currently costs the next
keypress (`list.go:83-86`: the `EnterDetail` stub returns `(nil, nil)`, so `updateDetail`
immediately resets to `ViewList` and swallows the message that triggered the reset); the real
detail view removes it.

## Review feedback: app tab registration (task 12) — 2026-07-30, commit `9429c9e`

Opus review: REQUEST_CHANGES. 24 mutations, **16 SURVIVED**. The production code survived every
attack on behaviour — happy path, all disabled-pane permutations, all five state-restore
degradations. What did not survive is the suite's ability to *notice* a regression: three of the
survivors reintroduce failure modes this commit's own doc comments claim to prevent. Findings 1–3
and the enablement/zero-value ones are now decisions 59–62.

Must fix:

1. **🔴 The capability gate can regress to the naive assertion undetected** — `app.go:288-294`. Per
   **decision 59**: use `provider.NewCompositeProvider(azdevops.NewAdapter(nil))` as the incapable
   fixture, not `nil`, and assert both `hasNotificationCapability(p) == false` and that `NewModel`
   renders no Notifications tab. Surviving mutation: body → `_, ok := p.(provider.NotificationSource);
   return ok`. Found independently by the validator and the reviewer.
2. **🔴 The notifications tab can render the pipelines pane with a green suite** — `app.go:1282-1287`.
   Per **decision 60**: assert pane content (`"No notifications found."`), not the tab-bar label.
   Surviving mutation: delete `case TabNotifications:` from `View()`'s content switch.
3. **🔴 The `main.go` AST test pins the callee but not the argument** — `cmd/azdo-tui/main_test.go:43-70`.
   Per **decision 62**. Surviving mutation: drop the `ghNC := github.NewNotificationsClient(token)`
   line and pass `nil` as the second argument.
4. **🟡 The enablement predicate is written three times and the tab order a third time** —
   `app.go:426` and `:448-463` versus `buildEnabledTabs` at `:273-286`. Per **decision 61**: derive
   both from the computed `enabledTabs`. Surviving mutation: `notifTabEnabled := notifCapable`
   (drop the `IsPaneEnabled` conjunct). Add one `NewModel`-level test for capable-but-pane-disabled
   asserting the tab strip *and* the help modal, which is the combination nothing renders today.
5. **🟡 The "zero value is never reached" comment is false and the zero value panics** —
   `app.go:536-542`, contradicted by `:875` and `:828`. Per **decision 61**: construct
   unconditionally (measured behaviour-preserving).
6. **🟡 Five new `case TabNotifications:` arms are entirely unpinned** — `app.go:327` (`initTabCmd`),
   `:1044` (`resizeActiveViewIfNeeded`), `:1065` (`syncStatusBarContext`), `:1315` (keybindings),
   `:875` (`WindowSizeMsg` sizing). Each deletes clean. Consequences: pipelines detail context leaks
   into the notifications footer; the pane's fetch is never dispatched (silent today, a visible bug
   at task 15); `notificationsKeybindings()` at `:1161-1170` has zero coverage; the pane is never
   sized. One table test over the five tabs asserting per-tab keybinding text and status-bar context,
   rendered through `View()`, closes all of them — the keybindings assertion is the cheapest and
   highest-value single line.
7. **🟡 Four of the five new forwarders are unpinned in both packages** —
   `internal/ui/notifications/list.go`: `GetContextItems`, `GetScrollPercent`, `GetStatusMessage`,
   `HasContextBar` each replaced with a zero value and survive. Trivial delegations, so severity is
   bounded, but they exist solely to feed the app chrome and nothing checks that they do. One table
   test seeding a feed and comparing each against the underlying `listview` value.
8. **🟡 The `state.yaml` round-trip never touches `state.yaml`** — `app_test.go`'s
   `TestModel_TabID_NotificationsRoundTripsThroughState` calls `store.Apply` then reads
   `store.State()`; `Apply` mutates memory and schedules a debounced `flushAsync`, so nothing
   serialises and renaming the on-disk literal to `"notifs"` survives. Task 12's criterion says
   *through `state.yaml`*. Either call `store.Flush()` then `state.Load(path)`, or add a
   `notifications` row to the existing disk round-trip table in `internal/state/store_test.go`.
   Mitigating: `store_test.go:75-114` covers the marshalling generically and `"pull_requests"` is
   equally unpinned, so this is pre-existing practice rather than a new regression. The convention-17
   constraint is not at issue — this is `state.Store` on a `t.TempDir()` path, not `Config.Save()`.

Genuine equivalents, do not chase: deleting `case TabNotifications:` from `isActiveViewSearching`
(`app.go:967`) cannot be killed, because decision 57 leaves `FilterFunc` nil so
`notifications.Model.IsSearching()` can never be true and both branches return `false` — keep the
vacuous-but-correct arm and write no test for it (`FORWARD: task 16`). And `CurrentVersion = 2 → 3`
survives because `Version` is written by `Save` and never read by `Load`, making decision 9's
no-bump rule unfalsifiable in-repo; the code does honour it, and `TabsState` genuinely has only
`PullRequests` and `WorkItems`, so the `state.go` comment's "mirroring Pipelines" claim checks out.

Confirmed sound under attack, do not re-litigate: notifications-first ordering and positional digit
derivation share **one** source — digits (`app.go:724-740`), `renderTabBar`'s `i+1` labels and
`enabledTabs` — so restating the order is killed by seven inherited tests, and notifications-only
(decision 47), `workitems`-disabled and notifications-disabled all render with digits agreeing;
`nextTab`/`prevTab` wrap correctly over 1..5-length slices. `main.go`'s nil semantics are unreachable
from any real config. All five state-restore degradations land on an enabled tab with no panic and no
blank screen. Decision 21 is not foreclosed: `FilterNotifications` is not referenced from
`internal/app` or `cmd` at all, so no redundant defensive copy was added at the app boundary.

Deferred: `FORWARD: task 15/16` — the zero-value pane hazard in finding 5 becomes live once those
tasks route messages to this pane from the top-level switch. `FORWARD: task 13/15` — the empty-inbox
body says "Press r to refresh" while the pane deliberately swallows `r` (decision 58's stopgap) and
the footer omits it; cosmetic today, actively misleading once the tab is discoverable. `FORWARD:
task 20` — `cmd/azdo-tui/main.go`'s `runHelp()` (~`:108`) still prints `1/2/3  Switch tabs (Pull
Requests, Work Items, Pipelines)` and its GitHub token-scope list omits `notifications`; the in-TUI
help modal *was* updated, the CLI help was not, and it will be wrong in a shipped binary.

## Validation: app tab registration (task 12) — re-check, 2026-07-30, commit `4b5abbd`

Gates (exit codes): `go build ./...` **0**, `go vet ./internal/... ./cmd/...` **0**,
`go test -count=1 ./...` **0** (all 27 packages ok), `gofmt -l` over the nine Go files touched
by task 12 (`040911d..HEAD`) → empty. `git status --porcelain` clean, no probe files left.

Mutation ledger (each mutation applied to the real file, suite run, file restored via `.probe`):

| # | Mutation | Result |
|---|----------|--------|
| 1 🔴 | `hasNotificationCapability` body → `_, ok := p.(provider.NotificationSource); return ok` (`app.go:293`) | **KILLED** — `TestHasNotificationCapability_AzureOnlyComposite_False`, `TestModel_NotificationsTab_Absent_WhenIncapable`, `TestModel_NotificationsPane_ConstructedEvenWhenTabAbsent/capability_absent`. The incapable fixture is now `provider.NewCompositeProvider(azdevops.NewAdapter(nil))` (`app_test.go` `newNotificationIncapableProvider`) and self-guards that it *is* capable-shaped, so decision 59 is met in letter |
| 2 🔴 | delete `case TabNotifications:` from `View()`'s content switch (`app.go:1323`) | **KILLED** — `TestModel_NotificationsTab_PresentButEmpty_WhenCapable`, `TestModel_PerTabChrome/notifications`, `TestModel_SwitchToNotificationsTab_ResizesPaneAndAccountsFooter`. Presence is asserted on the pane body (`notificationsPaneMarker = "No notifications found."`) plus a negative on `"No pipeline runs found."` — decision 60 satisfied |
| 3 🔴 | drop `ghNC := github.NewNotificationsClient(token)` and pass `nil` (`main.go:322-323`) | **KILLED** — `TestRunTUI_UsesGitHubAdapterWithNotifications` fails on the nil-second-arg and missing-`NewNotificationsClient` assertions; decision 62's "argument, not just callee" is pinned |
| 4 🟡 | help-modal name list restates the per-pane predicate with the `IsPaneEnabled` conjunct dropped | **KILLED** — `TestModel_NotificationsTab_Absent_WhenPaneDisabled_ButCapable` (help-modal half) |
| 5 🟡 | revert to conditional pane construction (`var nv notifications.Model; if containsTab(...)`) | **KILLED** — `TestModel_NotificationsPane_ConstructedEvenWhenTabAbsent/capability_absent` fails via an un-recovered nil-`*LoadingIndicator` panic (convention 16 respected: no recover wrapper) |
| 6a 🟡 | delete `initTabCmd`'s arm (`app.go:359`) | **KILLED** — `TestModel_InitTabCmd_Notifications` |
| 6b 🟡 | delete the `WindowSizeMsg` sizing line (`app.go:916`) | **KILLED** — `TestModel_WindowSizeMsg_SizesNotificationsPane` |
| 6c 🟡 | delete `resizeActiveViewIfNeeded`'s arm (`app.go:1085`) | **KILLED** — `TestModel_SwitchToNotificationsTab_ResizesPaneAndAccountsFooter` |
| 6d 🟡 | delete `syncStatusBarContext`'s arm (`app.go:1105`) | **KILLED** — same test (footer measured against pipelines' stale context bar → frame one row short) |
| 6e 🟡 | delete the `notificationsKeybindings()` arm (`app.go:1356`) | **KILLED** — `TestModel_PerTabChrome/notifications` |
| 8 🟡 | `state.TabNotifications` literal `"notifications"` → `"notifs"` (`state.go:43`) | **KILLED** — `TestModel_TabID_NotificationsRoundTripsThroughState`, which now `Flush()`es, `state.Load(path)`s, and greps the file for `active_tab: notifications` |
| 7 🟡 | all four chrome forwarders → constant zero (`ui/notifications/list.go:161,166,171,176`) | **SURVIVED — genuine equivalent, accepted.** `listview.go:426-455` returns the zero value unless `viewMode == ViewDetail && m.detail != nil`, and the pane's `EnterDetail` hook returns `(nil, nil)` (`list.go:84-86`), so `detail` can never be non-nil; `HasContextBar` needs `config.HasContextBar != nil`, which the pane never sets. All four are structurally constant in phase 1. `TestChromeForwarders_MatchUnderlyingListview` pins the delegation (`!= m.list.X()`, not a hand-written zero), so it becomes non-vacuous the moment task 13 gives the pane a detail view. Verified: suite green under the mutation, as the hardening report states |

Deviation judgment (finding 4 / decision 61): **accepted.** The plan asked for a `notifTabEnabled`
local derived from `enabledTabs`; the implementation deleted the local instead and derives the
help-modal line straight from `enabledTabs` via `helpTabName`, adding `containsTab` so
`isTabEnabled` queries the same slice. That is stronger than the letter of decision 61, not weaker:
with no second copy of the predicate anywhere, there is nothing left to drift.
`buildEnabledTabs` (`app.go:308`) is the sole owner, and the only other
`IsPaneEnabled("notifications")` in the tree is task 9's `Validate()` guard (`config.go:624`,
decision 47), a different question. The help-name derivation is behaviour-preserving:
`metricsEnabled` (`app.go:451`) is exactly `buildEnabledTabs`' metrics predicate, and the
label divergence (`"PR"` vs the strip's label) is documented as deliberate on `helpTabName`.

Also closed en route, beyond the eight findings: `cmd/azdo-tui/main.go:107-109`'s CLI help no
longer prints the three-tab line, satisfying task 12's own "CLI help no longer advertises the old
three-tab line" criterion literally (previously a `FORWARD: task 20` note). The CLI help's GitHub
token-scope list still omits `notifications` — that half stays `FORWARD: task 20`.

Still forwarded, unchanged: `FORWARD: task 13/15` (empty-inbox body says "Press r to refresh"
while the pane swallows `r`), `FORWARD: task 16` (the vacuous-but-correct `isActiveViewSearching`
arm), `FORWARD: task 20` (CLI token-scope list).

## Validation: notifications render states (task 13) — 2026-07-30, commit `26c2870`

Gates all green: `go build ./...` (exit 0), `go vet ./internal/... ./cmd/...` (exit 0),
`go test -count=1 ./...` (exit 0, all packages `ok`), `git status --porcelain` empty after every
probe was reverted. `gofmt -l` on the six touched Go files (`internal/app/app.go`,
`internal/app/app_test.go`, `internal/ui/components/listview/listview.go`,
`internal/ui/components/listview/listview_test.go`, `internal/ui/notifications/list.go`,
`internal/ui/notifications/list_test.go`) flags only `listview.go`, confirmed **pre-existing**
by diffing `gofmt -l` against the same file checked out from `main` — unrelated to this commit,
per convention 10's per-touched-file scope.

### Mutation ledger

| Mutation | Result |
|---|---|
| `filterEmptyBody` → `emptyInboxBody()` | KILLED — `TestView_ActiveFilter_RendersIndicator_NotBareEmptyInbox`, `TestView_FilterEmpty_DistinctFromEmptyInbox_NamesActiveFilter` (pane), `TestModel_NotificationsTab_FilterEmpty_DistinctFromEmptyInbox` (app) |
| `errorBody` → `emptyInboxBody()` | KILLED — `TestView_Error_CarriesTokenScopeSkeleton_AndTakesPriorityOverRows`, `TestView_Error_TakesPriorityOverEmptyInbox_WhenFeedIsEmpty` (pane), `TestModel_NotificationsTab_Error_RendersThroughFullView` (app) |
| `capabilityUnsupportedBody` → `emptyInboxBody()` | KILLED — `TestView_CapabilityUnsupported_DistinctFromOtherThreeStates` (pane-only, as decision 63 specifies; app suite stays green, correctly, since this state is unreachable through the tab) |
| Reorder `View()`: error check before capability check (swap adjacent pair 1↔2) | **SURVIVED** — both suites green. No test ever sets `capabilityUnsupported` and a `list.Err()` simultaneously, so the documented priority between these two specific states is unpinned. Judged non-blocking: decision 63 states the capability arm is reachable only via the pane-only `SetCapabilityUnsupported()` test seam and never co-occurs with a real error in any real configuration (an incapable backend never reaches `List`), so the two states are mutually exclusive in practice and a combined-state test would itself be the kind of vacuous fixture decision 59/63 warn against. Recorded for the record, not treated as a task-13 defect. |
| Reorder `View()`: error check after the filter-empty/empty block (swap adjacent pair 2↔3) | KILLED — `TestView_Error_TakesPriorityOverEmptyInbox_WhenFeedIsEmpty` (pane), `TestModel_NotificationsTab_Error_RendersThroughFullView` (app). This is exactly the test the implementer added to close the gap they reported finding during implementation. |
| Swap filter-empty/empty branches (pair 3↔4) | N/A as a "reorder" — these are the two arms of one `if/else` on `m.reasonFilterActive`, not a priority stack (they can never both be true), so there is nothing to reorder; swapping the `if`/`else` bodies is a no-op and both suites stayed green as expected. |
| `notificationsTabContent` → unconditionally `banner + "\n\n" + paneView` | KILLED — `TestNotificationsTabContent_EmptyWarnings_NoStrayBlankLine`, `TestModel_SwitchToNotificationsTab_ResizesPaneAndAccountsFooter` |
| `notificationsWarningsBanner` → unconditionally `""` | KILLED — `TestNotificationsTabContent_PopulatedWarnings_RendersBanner`, `TestModel_NotificationsTab_Warnings_RenderInFullView` |

### Convention checks

- **Convention 8**: every task-13 test drives `m.list.Update(tea.WindowSizeMsg{...})` before
  `SetFeed`/`HandleFetchResult`/`View()`, so `table.renderRow` runs (inside `SetItems`) on the
  correct column widths ahead of the render assertion — matching the "recover above the row
  population, not merely around `View()`" shape convention 8 requires. No dedicated
  `defer recover()` guard was needed in these specific tests because an unrecovered panic already
  fails the test; the existing panic-guard tests (`TestView_RendersAfterWindowSizeMsg_*`) remain
  from task 11 and were not touched.
- **Convention 6**: none of the four states, nor the warnings banner, introduce new styled text —
  `emptyInboxBody`, `filterEmptyBody`, `errorBody`, `capabilityUnsupportedBody` and
  `notificationsWarningsBanner` all return plain, unstyled strings (`filterIndicator` was already
  deliberately unstyled per its existing doc comment). Convention 6 is therefore not implicated by
  this commit — nothing here needs a style-object assertion.
- **Decision 58's `r` stopgap**: `emptyInboxBody()` = `"No notifications found.\n\nYou're all
  caught up."` and `filterEmptyBody()` = `"No notifications match Filter: %s.\n\nPress f to cycle
  back to all reasons."` — neither mentions `r`. `TestView_EmptyInbox_ReadsAsClear_NotError`
  explicitly asserts the absence of "Press r".
- **Additive listview accessors** (`Err()`, `Loading()`, `internal/ui/components/listview/listview.go:425-450`):
  read-only, no existing field's write path changed, no existing call site touched. Confirmed by
  running the full suite (all pre-existing `listview` and sibling-pane tests pass unmutated) and
  by inspection — both are pure getters added after existing methods, and the new
  `TestErr_ReflectsHandleFetchResult`/`TestLoading_ReflectsRefreshState` tests are additive-only.
  No other pane's behaviour changed.

### Judgment calls (per validator brief point 6)

1. **`len(m.list.Items()) == 0 && m.reasonFilterActive` vs. `len(m.feed) > 0`.** Measured directly
   (probe test, reverted): seeding a 2-row feed, activating the `f` filter to `Mentioned`, then
   calling `SetFeed(nil)` (the *whole* feed goes empty, not just the filtered view) renders
   `"No notifications match Filter: Mentioned.\n\nPress f to cycle back to all reasons."` — the
   filter-empty text — even though clearing the filter reveals nothing, because the feed itself is
   empty. No test in this commit pins this combination (confirmed by grep: none of the new
   fixtures call `SetFeed` with an empty/nil slice while a reason filter is already active). This
   is a real, unpinned edge case, but I judge it **non-blocking for task 13**: the message itself
   remains literally true (no row matches `Mentioned`) and pressing `f` — the action the message
   names — does correctly resolve to `emptyInboxBody()` on the very next cycle, because
   `cycleReasonFilter` defensively resets to "all reasons" the moment `presentReasons(m.feed)` is
   empty (list.go:328-329, 334-335). It is an imprecise-in-the-moment message, not a dead end or a
   lie the user cannot escape. Decision 63's own reachable-state definition ("a user-set `f` filter
   matching zero rows" of a *non-empty* feed) does not extend to this deeper case either. Worth a
   follow-up note for whichever task next touches this file (task 15's poller is the next writer of
   `SetFeed`), but not a task-13 criterion failure.
2. **`errorBody` appending the token-scope line to every error, including the nil-client message.**
   Acceptable as specified. Task 13's own criterion text says the error state "carries the
   token-scope skeleton (decision 17) and the nil-client message, with task 19 owning the 403/401
   differentiation" — i.e. the spec explicitly defers narrowing this message to task 19, and
   `errorBody`'s doc comment (`list.go:263-268`) states this in a `FORWARD: task 19` note naming
   exactly the `errors.As`/`*github.APIError` differentiation to add. Shipping the flat skeleton
   now, with the deferral documented, matches the letter of the task-13 criterion rather than
   overrunning into task 19's scope.

## Phase 2 notes — carry into `20260729-notif-p2-azdo.md`

Implementation questions phase 1 answered in a deliberately phase-1-shaped way. Each is
correct today and becomes wrong the moment Azure implements `NotificationSource`.

- **`only_configured_repos` resolves against `github.repos` only** (task 10, decision 50). Once
  Azure is capable, every Azure row fails that membership test and a knob the user set to tame
  GitHub noise silently empties the Azure half of the feed. Phase 2 must resolve "configured"
  per backend — Azure's equivalent is `projects`/repos, and its `Scope` is not an `owner/repo`
  slug, so decision 51's glob semantics need re-deriving rather than reusing. Concretely: an Azure
  `Scope` carries no `/`, so `*/*` — the pattern task 20 documents as "everything" — matches no
  Azure row at all, and no `owner/*` entry in `exclude_repos` can ever trim one.
- **Decision 47's at-least-one-pane guard is gated on `HasGitHub()`**. It widens to "any
  configured backend" and the special case disappears; the code carries a `PHASE-1-ONLY` comment.
- **Decision 43's mark routing** matches on `backend.Kind() == id.Kind` over capable backends
  with an explicit `kind == 0` guard. Two capable backends make this the live path rather than a
  formality — phase 2 should keep the guard and add a two-capable-backend routing test.
- **Decision 5**: no local read/done state exists in phase 1. Azure needs it, keyed on
  `Identity.SameItem` (kind + scope + id, never `ScopeDisplay` — a repo rename would otherwise
  resurrect everything dismissed).

## Unknowns

**No live API observation is possible here.** The environment reaches `api.github.com` but has
no token, so a route's existence and auth-gating can be confirmed while real payloads cannot.
Tasks 4–6 and 15 encode the *documented* shapes and unit-test against fixtures; anything below
marked _(manual)_ needs a human with a real token before merge. Do not report these verified.

- `subject.url` → browser URL for every subject type (PR, issue, release, discussion, commit,
  check suite). Task 5 implements the documented shapes and falls back to the repo URL for
  anything unrecognised; the fallback is the correctness guarantee, not the mapping. _(manual)_
- Whether `X-Poll-Interval` is in practice slow enough to feel stale against the configured
  interval. Ship the `max()` rule from decision 8 and observe later. _(manual)_
- Whether the merged feed needs pagination/virtualisation above `max_items: 100`.
- `PATCH /notifications/threads/{id}` (mark read) and `DELETE` (mark done) return 205/204 per the
  docs, and no mark-*unread* route exists (decision 13). Task 6 encodes those shapes and unit-tests
  against fixtures; the status codes and the one-way constraint need confirming with a real token.
  _(manual)_
- Read state may be eventually consistent: a poll landing right after a `PATCH` could still
  return `unread`, flickering the optimistic update back. Task 14 holds the local intent until
  the server agrees rather than trusting the first poll that contradicts it.

## Review feedback: notifications render states (task 13) — 2026-07-30, commit `26c2870`

Reviewed at opus. **Not an independent review**: three consecutive `529 Overloaded`
failures made an opus reviewer subagent unobtainable, and `.claude/skills/afk`
forbids downgrading the reviewer to sonnet, so the loop driver performed it.
Recorded here because the independence loss is a real weakening of the protocol,
not a formality — every task from 5 through 12 in this run had findings the
validator missed, so this task's review carries less assurance than theirs.

Mutation ledger — 9 mutations, 6 killed on arrival, **3 survived** and are now
closed by tests added in the follow-up commit:

| Mutation | Result |
|---|---|
| `filterEmptyBody` → `emptyInboxBody()` | KILLED (3 tests) |
| `errorBody` → `emptyInboxBody()` | KILLED (3 tests) |
| `capabilityUnsupportedBody` → `emptyInboxBody()` | KILLED |
| `View()`: error check moved after the empty/filter-empty block | KILLED (2 tests) |
| `notificationsTabContent` prepends the banner unconditionally | KILLED (2 tests) |
| `notificationsWarningsBanner` → `""` | KILLED (2 tests) |
| **`HandleFetchResult` success path → `m.list.HandleFetchResult(items, nil)`** | **SURVIVED** → now killed by `TestView_SuccessfulFeedAfterError_ClearsErrorState` |
| **`View()`: drop the `!m.list.Loading()` conjunct** | **SURVIVED** → now killed by `TestView_Loading_DoesNotClaimCaughtUp`; root cause recorded as decision 64 |
| **`View()`: swap the capability and error arms** | **SURVIVED** → now killed by `TestView_CapabilityUnsupported_OutranksError` |

Findings, in severity order:

1. 🟡 **The recovery path was held by nothing, and the pane's immunity to a
   pre-existing listview bug was incidental.** `listview.HandleFetchResult`'s
   success path never assigns `m.err = nil` (`listview.go:388-402`) while
   `viewList` short-circuits on `m.err != nil` (`:341`), so a pane recovering
   through it stays pinned to the error render permanently. This pane escapes
   only because its success path routes through `SetFeed → SetItems`, which does
   clear the field. Rewriting that one line as a forward to
   `m.list.HandleFetchResult` reads as a harmless simplification and kept the
   entire suite green. Task 15's poller is the caller. **Fixed** by a test that
   errors, recovers, and asserts the error text and token-scope skeleton are both
   gone and the new row renders.
2. 🟡 **`!m.list.Loading()` is dead code today and unpinned** — see decision 64.
   Kept and pinned rather than deleted, with a `FORWARD: task 15` on the test.
3. 🟢 **Capability-vs-error priority was unobservable.** `assertOtherStatesAbsent`
   already gives five of the six state pairs mutual distinguishability; capability
   and error were the one pair no fixture set *together*, making the arm order a
   genuine equivalent. **Fixed** by a fixture that sets both.

Verified sound, not to be re-litigated: `assertOtherStatesAbsent` is a real
six-pair distinguishability harness, not four positive assertions; the error
fixture derives its message from a real `github.NewAdapterWithNotifications(nil, nil)`
rather than a hand-typed string, so it cannot drift from the adapter; neither
empty body mentions `r` (decision 58); `Err()`/`Loading()` are additive getters
that changed no write path for the four existing panes; and decision 63's
pane-level placement of the capability assertion is honoured — there is no
app-level test claiming to exercise an unreachable state.

Judgment calls left as-specified: `errorBody`'s blanket "GitHub token scope
required: notifications" line is misleading for a transient network failure, but
task 13's criterion explicitly defers 403/401/generic differentiation to task 19,
which is where it gets narrowed. And the filter-empty message can appear when the
whole feed is empty and a filter happens to be active — unreachable by cycling
today (task 11's `f` offers only reasons present in the feed) and it becomes
reachable at task 15, where the fix is `len(m.feed) > 0` rather than
`len(m.list.Items()) == 0`. `FORWARD: task 15` for both.

## Review feedback: `u`/`d` mark-read/mark-done (task 14) — 2026-07-30, commit `3e0461c`

Reviewed by the loop driver at opus. Gates on `3e0461c` were all green — build, vet,
`go test -count=1 ./...`, `gofmt -l` on the four touched files, clean worktree — and the
implementer's ledger claimed 6/6 mutations killed. One 🔴 survived anyway.

🔴 **A successful mark did not survive its own debounce window.** `handleMarkResult` was a
no-op on success, so the override was the only thing holding the mark, and `visibleItems`
re-derives from `m.feed` once the override expires. Confirmed empirically with a throwaway
probe: `d`, a success result, the clock advanced past `markDebounceWindow`, then one purely
local `f` cycle — the dismissed row came back, no poll involved. Fixed by decision 65's
`commitOverride`: write the mark into `m.feed` on success and keep the override entry for
the rest of the window. Regression tests added for `d`, for `u`, and for the failure path
(a failed mark must never be committed). Four mutations of the new shape all die.

Why the implementer's ledger did not catch it: every mutation it ran perturbed a single
step (the rollback, the identity, the window constant). None composed
success → expiry → local re-derivation, which is the only path that reaches the bug. The
existing `PollAfterDebounceWindowExpires_TrustsPoll` test looks like it covers expiry but
does not — it advances the clock *and* delivers a contradicting poll, so it cannot tell
"the poll won" from "the feed was never updated".

Verified sound, not to be re-litigated:
- Rollback drops the override rather than re-inserting a row, so decision 45's merge order
  is never reproduced by hand — the constraint the task prompt set explicitly.
- `notificationMarkResultMsg.err` is carried untouched, so task 19 can still `errors.As`
  a `*github.APIError` out of it.
- `withOverride`/`dropOverride` both allocate fresh maps; no two Model copies share one.
- `identityKey` drops `ScopeDisplay`, matching `Identity.SameItem` — a repo rename cannot
  split one row's override in two.
- `markRead` is one-way per decision 13 and reads the row's *effective* `Read`, so an
  active `overrideRead` suppresses a second call without consulting `m.overrides`.
- `canTriage` gates on the error and capability states as well as emptiness, which is
  required rather than redundant because `listview.HandleFetchResult`'s error path leaves
  stale items in `Items()`.

🟡 **Carried forward, not fixed here:** `markDone`'s already-hidden guard is documented as
unreachable through the UI and is untested. It is a genuine double-fire guard for a future
direct caller, so it stays, but it is dead code by the same standard decision 64 applied to
the `Loading()` conjunct — worth a pin if task 16 or 19 ever drives `markDone` directly.

## Validation: `u`/`d` mark-read/mark-done (task 14) — re-check, 2026-07-30, commit `5f7a198`

**Verdict: COMPLETE.** Decision 65's fix is in place and, unlike `3e0461c`, it is pinned by
tests that can fail. All six prescribed mutations die.

Gates (all green): `go build ./...`, `go vet ./...`, `go test -count=1 ./...` (no failures),
`gofmt -l` clean on the four files this task touched (`internal/app/app.go`,
`internal/app/app_test.go`, `internal/ui/notifications/list.go`,
`internal/ui/notifications/list_test.go`). The ~23 pre-existing gofmt-dirty files inherited
from `main` are out of scope and were not counted against this task.

Mutation ledger — 6 prescribed, 6 killed, 0 survivors:

| # | Mutation | Result | Killed by |
|---|----------|--------|-----------|
| 1 | `handleMarkResult` success reverted to `if res.err == nil { return m }` (the original 🔴) | **dies** | `TestMarkDone_Success_SurvivesWindowExpiry_WithoutPoll`, `TestMarkRead_Success_SurvivesWindowExpiry_WithoutPoll` |
| 2 | `handleMarkResult` commits unconditionally, ignoring `res.err` | **dies** | `TestMarkRead_Failure_RollsBackOptimisticUpdate`, `TestMarkDone_Failure_RestoresRow`, `TestMarkDone_Failure_ThenWindowExpiry_DoesNotResurrectTwice` |
| 3 | `commitOverride` matches every feed row (`keyOf(n.Identity) == key` guard deleted) | **dies** | `TestMarkDone_Success_RemovesRowAndStaysRemoved`, both `..._SurvivesWindowExpiry_WithoutPoll` tests |
| 4 | `commitOverride` also drops the override after committing | **dies** | `TestMarkRead_PollWithinDebounceWindow_DoesNotFlickerBack`, `TestMarkDone_PollWithinDebounceWindow_RowStaysHidden` — i.e. exactly the poll-debounce pair decision 65 says must break |
| 5a | `markDebounceWindow = 0` | **dies** | 7 tests, incl. both optimistic-update paths and both poll-within-window tests |
| 5b | `markDebounceWindow = 24 * time.Hour` | **dies** | `TestMarkRead_PollAfterDebounceWindowExpires_TrustsPoll` |
| 6 | `dropOverride` made a no-op | **dies** | the three failure/rollback tests |

Note on 5b: it is killed by a single test, because the mark-done-side expiry fixtures derive
their clock advance from `markDebounceWindow + time.Second` and so move with the constant.
`TestMarkRead_PollAfterDebounceWindowExpires_TrustsPoll` hardcodes `+10 * time.Minute`, which
is what makes it a real bound. That test is also no longer the ambiguous one the review
flagged: now that success commits into `m.feed`, "the poll won" and "the feed was never
updated" have different observable outcomes, so it discriminates.

Criterion-by-criterion against the task line as amended by decision 65:

- `u` issues **exactly one** mark-read for the cursor row's own `Identity`, applied
  optimistically inside `Update` before the `tea.Cmd` runs — pinned, incl. the wrong-row
  case (`TestMarkRead_ExactIdentity_SelectsRowUnderCursor_NotFirstRow`) and one-way-ness
  from both the override and the feed (`..._AlreadyRead_IsOneWay_NoSecondCall`,
  `..._AlreadyReadInFeed_NeverCallsMarkRead`).
- Rollback on API failure for both `u` and `d`, by dropping the override rather than
  re-inserting a row — pinned; mutation 6 confirms.
- Stale-`unread` poll inside the window does not flicker the row back, for both kinds —
  pinned; mutations 4 and 5a confirm.
- Decision 65's commit-into-the-feed **and** keep-the-override: both halves pinned
  independently (mutation 1 kills the missing commit, mutation 4 kills the missing keep),
  asserted for `u` and `d` separately as the task requires, plus
  `TestMarkDone_Failure_ThenWindowExpiry_DoesNotResurrectTwice` pinning that the commit is
  success-only.
- `TestMarkRead_Success_SurvivesWindowExpiry_WithoutPoll` also asserts the *untouched*
  neighbour row stays unread, which is the in-test guard against a loose commit match —
  independent of mutation 3.
- `commitOverride` allocates a fresh `next` slice, so the caller's `SetFeed` slice and its
  backing array are never mutated (decision 52's non-aliasing rule holds through the new
  path too).
- `canTriage` gates `u`/`d` on the empty, error and capability-unsupported states, and a nil
  marker is a no-op — all four pinned.

Conventions (`## Active` only, 1–12 and 17):

- **7** satisfied structurally: every path that changes the visible row set — including the
  new `commitOverride` — routes through the single `setItemsPreservingSelection` →
  `listview.SetItems` call site, so `ToColumns` and `ToRows` always receive the same slice.
- **10** satisfied: `gofmt -l` clean on all four touched files.
- **17** trivially satisfied and confirmed, not assumed: the task-14 diff contains no
  reference to `internal/config`, no `Config.Save()`, no `GetPath()`, no `os.WriteFile`, no
  `HOME`/`TempDir` manipulation. Nothing can reach a real user config path.
- 1–6, 8, 9, 11, 12 are not engaged by this diff (no new neutral type, no `Provider` scope
  method, no mapper enumeration, no glyph/style change, no config keys, no id guard, no
  ignore patterns).

🟡 **Observations, not blockers** (neither is a task-14 acceptance criterion):

1. `app.notificationMarker` is unpinned. Mutating it to `return nil` — the exact
   nil-marker-forever state decision 62 caught for `main.go`'s wiring — leaves the whole
   suite green, because every `internal/app` notifications test constructs the pane with a
   `nil` marker and no test asserts the pane received the provider. It is a *test* gap, not
   a live bug (the wiring in `NewModel` and the `ThemeSelectedMsg` branch is correct), but
   it is the same class decision 62 documents. Worth a pin when task 15 or 16 next touches
   `app.go`.
2. Convention 8: no task-14 test drives `View()`. The mark paths share
   `setItemsPreservingSelection` with the `f` cycle, whose collapse test *does* render
   through `View()` after a `WindowSizeMsg`, so the column/cell invariant is covered by
   proxy — but a `d` that collapses a multi-repo feed to a single repo is not itself rendered.

Process note: another agent was concurrently editing `internal/config` in this worktree
during validation (`internal/config/config_save_test.go` modified, plus a transient
`config.go` + `.probe` pair). Those are task 17's in-progress work, untouched here, and one
transient `internal/config` build failure observed mid-run came from that edit, not from
task 14 — the clean full-suite run above predates it and the four packages this task can
affect (`ui/notifications`, `app`, `provider`, `github`) were re-run green afterwards.

## Validation: `Save()` preservation regression test (task 18) — 2026-07-30

Verified by the loop driver. This is the user's most emphatic constraint in the whole
spec ("it's very important that writeconfig don't destroy any config already existing…
Comments is okay, but it should NOT destroy any other configs"), so the teeth were
checked directly rather than taken on report.

Two tests in `internal/config/config_save_test.go`:

- `TestConfigSave_PreservesMetricsAndNotifications` — seeds a YAML fixture with all 8
  `metrics:` fields (including the nested `states`/`state_labels` maps) and all 9
  `notifications:` keys at non-default values, loads via `LoadFrom`, asserts the fixture
  parsed as seeded **before** mutating, changes only `Theme`, saves, reloads, and compares
  both sections with `reflect.DeepEqual` against hardcoded expected structs.
- `TestConfigSave_PreservesKeysOutsideTheConfigStruct` — seeds `some_future_section` with a
  scalar and a nested child, and re-reads the **raw YAML** after the save.

Why the second test is not redundant: every assertion in the first reads the reloaded
`*Config`, so a section with no struct field is invisible to it — it would survive or be
destroyed with the suite equally green. `Save()`'s own doc comment promises preservation for
"navigation state, future additions", and this is the only test that can observe it. It also
covers nesting specifically: viper flattens on read, so a round-trip that mishandled nesting
would keep the parent key and drop its children.

Teeth confirmed by mutation, not by inspection. Deleting the five-line `v.ReadInConfig()`
round-trip at `internal/config/config.go:744-748` fails **both** tests: metrics and
notifications come back as all-defaults (`Enabled:false IntervalDays:14 …`, `SinceDays:0
MaxItems:0 PollInterval:0`) and the unmanaged section is gone from the file entirely. Restored
and re-verified green, with `git diff internal/config/config.go` empty afterwards.

Isolation (convention 17), the user's other hard constraint — checked structurally rather
than assumed. Fixtures are written to `t.TempDir()/config.yaml` and loaded with `LoadFrom`,
which sets `cfg.configPath`. `Save()` only calls `GetPath()` — which resolves to the real
`~/.config/azdo-tui/config.yaml` — when `configPath == ""` (`config.go:725-732`). Since every
`Config` in both tests comes from `LoadFrom` with an explicit temp path, that branch is
unreachable, so neither test can read or write a real user config.

Standing gap this closes for task 19: `Save()` never calls `v.Set("notifications", …)` or
`v.Set("metrics", …)`, so both sections persist **only** via the round-trip. Task 19 writes
`disabled_panes` through `Save()`, which makes that round-trip a live path rather than a
latent one. Comments are still lost on save — viper rewrites the file rather than patching
it — which is the one loss the user accepted explicitly.

## Review feedback: `u`/`d` mark-read/mark-done (task 14) — independent review, 2026-07-30, commit `c8844ba`

Independent opus reviewer, verdict **REQUEST_CHANGES**. Eight findings. All addressed; one
forwarded rather than fixed. Task 14 unticked pending re-validation.

| # | Sev | Finding | Disposition |
|---|---|---|---|
| 1 | 🔴 | `notificationMarkResultMsg` was unexported and matched no top-level `case` in `app.Update`, so it reached the pane only through `case TabNotifications:` in the delegate-to-active-tab switch. Switch to any other tab while a mark is in flight and the result is handed to that tab's pane, which discards it — reopening decision 65's resurrection defect by a second route | **Fixed**, decision 66. Type exported as `notifications.MarkResultMsg` (fields still unexported), new unconditional case ahead of `case polling.TickMsg:`. Pinned by `TestModel_MarkResultMsg_ReachesPane_WhileAnotherTabIsActive` |
| 2 | 🟡 | `handleMarkResult` ignored `res.kind`, so a stale result rolled back whatever override held the key — including a *newer* action's | **Fixed**, decision 67(a). Kind-mismatch guard returns unchanged. Pinned by `TestMarkResult_StaleKind_DoesNotRollBackANewerAction` |
| 3 | 🟡 | A mark result re-derived the item list unconditionally, and `listview.SetItems` clears `err`, so the pane rendered "you're all caught up" over a failed fetch | **Fixed**, decision 67(b). `refreshItems` short-circuits on `m.list.Err() != nil`; `SetFeed` deliberately keeps the unguarded path as the recovery route. Pinned by `TestMarkResult_DoesNotClearAFailedFetchsErrorState`, plus a `FORWARD: task 15` note where the guard goes live |
| 4 | 🟡 | `visibleItems`' doc comment (written by the loop driver in the previous round) asserted an invariant that decision 65 had just falsified | **Fixed** — comment now states expiry is safe only for an *applied* mark |
| 5 | 🟡 | `TestMarkDone_CursorSurvives_OnPreviouslySelectedNeighbor` asserted `ID != "2"` after `d` removed row 2 — true for every possible cursor, so vacuous | **Fixed by rewrite**, not by the reviewer's suggested retarget: that path does not exist. `markDone` always removes the row *under* the cursor, so `FindIndex` always returns −1 and listview's positional clamp is the only live mechanism. Replaced with `TestMarkDone_CursorLandsOnAConcreteRow`, a table pinning measured clamp outcomes for middle/first/last-of-three and the single-row case (idx −1, `selectedItem` ok=false). The identity-restore path is genuinely pinned elsewhere — `TestCycleReasonFilter_Collapse_RepoColumnDisappears_CursorSurvives` and `TestSetFeed_PreservesSelectedItemAcrossReorder` |
| 6 | 🟡 | A failed `u`/`d` is entirely silent — the row snaps back with no message anywhere | **Forwarded to task 19**, not fixed. Task 14's criterion says "rolling back on API failure", which is met; surfacing the failure needs the error-presentation work task 19 owns, and inventing a second error channel here would be undone by it. Recorded as a `FORWARD: task 19` block above `handleMarkResult` **and** in the final report's unresolved section, so it cannot be lost if task 19 is descoped |
| 7 | 🟢 | `m.overrides` was never pruned — expired entries accumulated for the process lifetime | **Fixed** — `prunedOverrides` in `SetFeed`, returning nil when empty so `visibleItems`' fast path still fires. Pinned by both poll-debounce tests |
| 8 | 🟢 | `notificationMarker`'s doc comment claimed a nil return for an incapable provider; a **typed** nil `*CompositeProvider` passes the assertion and yields a non-nil interface wrapping a nil pointer | **Fixed** — comment narrowed to distinguish untyped from typed nil |

Also carried in from the validator's re-check: `app.notificationMarker` could be mutated to
`return nil` with the whole suite green — same class as decision 62's `main.go` gap. **Fixed**,
pinned by `TestModel_NotificationMarker_IsWiredToThePane`.

Reviewer items examined and found sound — not to be re-litigated: `commitOverride` holds in
every interleaving constructed; `notificationBackendFor` has the `kind == 0` guard
(`internal/provider/composite.go:757-771`); `markCmd` captures only `m.marker`/`id`/`key` by
value; `markRead`'s "effective state" claim is true; `markDone`'s already-hidden guard is
UI-unreachable but correctly kept (decision 64's shape); the style and colour assertions are
not vacuous; `ThemeSelectedMsg`'s pane reconstruction discards overrides and feed together,
which is consistent.

Mutation ledger for this round, all killed: stale-kind guard removed → finding 2's test;
`refreshItems` error guard removed → finding 3's test; decision 65 regressed to a no-op →
both `SurvivesWindowExpiry` tests and `StaleKind`; `prunedOverrides` inverted to drop live
entries → both poll-debounce tests; app routing case deleted → finding 1's test;
`notificationMarker` returning nil → the wiring test.

## Validation: `u`/`d` mark-read/mark-done (task 14) — independent-review re-check, 2026-07-30, commit `35466e5`

Verdict **INCOMPLETE**. Seven of the eight findings' dispositions hold. One carried-in item
(`notificationMarker` wiring) is only *narrowly* pinned and the real defect it stands for
survives mutation across the entire suite; finding 7's "pinned by" claim is factually wrong.
Task 14 stays unticked.

Full suite: `go test ./... -count=1` — **all 27 packages ok**. `gofmt -l` clean for all four
files this commit touched (`internal/ui/notifications/list.go`, `.../list_test.go`,
`internal/app/app.go`, `.../app_test.go`). `go vet` clean for both packages. Tree clean.

### Per-finding verdict

| # | Verdict | Evidence |
|---|---|---|
| 1 | **Holds** | `MarkResultMsg` exported (`list.go:165`), fields unexported; `case notifications.MarkResultMsg:` sits at `app.go:950`, ahead of `case polling.TickMsg:` and outside the delegate switch. Deleting the case fails `TestModel_MarkResultMsg_ReachesPane_WhileAnotherTabIsActive`. Asserting on the *failure* path is the right call — a dropped and an applied success are genuinely indistinguishable from `app`, which cannot reach the pane's clock seam |
| 2 | **Holds** | Kind-mismatch guard at `list.go:425`, returns `m` unchanged. Both disabling it and inverting it fail `TestMarkResult_StaleKind_DoesNotRollBackANewerAction` |
| 3 | **Holds** | `refreshItems` short-circuits on `m.list.Err() != nil` (`list.go:780-785`); `SetFeed` still routes through `setItemsPreservingSelection` directly, so recovery-from-error is preserved and stays pinned by `TestView_SuccessfulFeedAfterError_ClearsErrorState`. `FORWARD: task 15` note present. Removing the guard fails `TestMarkResult_DoesNotClearAFailedFetchsErrorState` |
| 4 | **Holds** | `visibleItems`' comment now separates applied from never-arrived marks (`list.go:831-844`) and no longer asserts the invariant decision 65 falsified |
| 5 | **Holds, and the replacement is not a tautology** | `TestMarkDone_CursorLandsOnAConcreteRow` pins `SelectedIndex()` *and* the surviving row's ID per case. Probe: forcing `SetCursor(0)` after every re-derive fails 2 of its 4 subtests ("middle of three", "last of three") — so the numbers are load-bearing, unlike the `ID != "2"` assertion it replaced. The claim that the identity restore is pinned elsewhere is verified: no-op'ing the whole `if hadSelection` block fails `TestCycleReasonFilter_Collapse_RepoColumnDisappears_CursorSurvives` and `TestSetFeed_PreservesSelectedItemAcrossReorder` |
| 6 | **Holds as a forward** | `FORWARD: task 19` block present at `list.go:405-410`; task 14's own "rolling back on API failure" criterion is met and pinned (`TestMarkRead_Failure_RollsBackOptimisticUpdate`, `TestMarkDone_Failure_RestoresRow`). Not counted as incomplete. **Note:** task 19's own spec line (`:185`) still says nothing about surfacing a failed mark, so the only in-spec record of this forward is the feedback table itself — if task 19 is descoped the item is lost. Consider amending line 185 |
| 7 | **Code holds; the "pinned by" claim is false** | `prunedOverrides` (`list.go:694-709`) is correct and returns nil when empty. But making it a **no-op** (`return overrides` unconditionally) is green across the whole suite — the poll-debounce tests kill the *inverted* form, not the *absent* form. That is expected, since the function is behaviour-neutral by design, but the table must not claim it is pinned. Severity 🟢: memory hygiene, no criterion at risk |
| 8 | **Holds** | `notificationMarker`'s comment (`app.go:301-320`) now distinguishes untyped nil (fails the assertion) from typed nil (passes it, yielding a non-nil interface over a nil pointer) |

### Mutation ledger

| Mutation | Result | Killed by |
|---|---|---|
| Delete `case notifications.MarkResultMsg:` from `app.Update` | killed | `TestModel_MarkResultMsg_ReachesPane_WhileAnotherTabIsActive` |
| `notificationMarker` → `return nil` | killed | `TestModel_NotificationMarker_IsWiredToThePane` |
| Kind-mismatch guard disabled in `handleMarkResult` | killed | `TestMarkResult_StaleKind_DoesNotRollBackANewerAction` |
| `refreshItems` skips its `m.list.Err() != nil` guard | killed | `TestMarkResult_DoesNotClearAFailedFetchsErrorState` |
| Success arm of `handleMarkResult` back to a no-op (regress decision 65) | killed | `TestMarkDone_Success_SurvivesWindowExpiry_WithoutPoll`, `TestMarkRead_Success_SurvivesWindowExpiry_WithoutPoll`, `TestMarkResult_StaleKind_DoesNotRollBackANewerAction` |
| `prunedOverrides` inverted (drops live, keeps expired) | killed | `TestMarkRead_PollWithinDebounceWindow_DoesNotFlickerBack`, `TestMarkDone_PollWithinDebounceWindow_RowStaysHidden` |
| Kind guard inverted (`==` instead of `!=`) — own | killed | 6 pane tests + `TestModel_MarkResultMsg_ReachesPane_WhileAnotherTabIsActive` |
| `SetCursor(0)` forced after every re-derive — own | killed | `TestMarkDone_CursorLandsOnAConcreteRow` (2 of 4 subtests) |
| Identity-restore block no-op'd — own | killed | `TestCycleReasonFilter_Collapse_…_CursorSurvives`, `TestSetFeed_PreservesSelectedItemAcrossReorder` |
| **`NewModel`: `NewModelWithStyles(appStyles, notificationMarker(p))` → `(appStyles, nil)`** (`app.go:604`) — own | **SURVIVOR — whole suite green** | nothing |
| **`ThemeSelectedMsg` rebuild: `notificationMarker(m.client)` → `nil`** (`app.go:892`) — own | **SURVIVOR** | nothing |
| **`prunedOverrides` never prunes** — own | **SURVIVOR** (behaviour-neutral; documentation-accuracy issue only) | nothing |

### The blocking survivor

`internal/app/app.go:604` is the only place production wires a marker into the pane. Replacing
its argument with `nil` passes `go test ./... -count=1` in full — 27/27 ok. In that state
`markRead`/`markDone` hit their `m.marker == nil` early return, so `u` and `d` are silent
no-ops: no API call, and not even an optimistic override. Task 14's criterion — "`u` issues one
mark-read and updates the row optimistically … `d` removes the row" — is therefore unmet with
the suite green. Same class as decision 62's `main.go` gap, and decision 62 already settled the
standard: the wiring test must pin the **argument**, not just the callee.

`TestModel_NotificationMarker_IsWiredToThePane` does not do that. It asserts
`notificationMarker(m.client) != nil` — which pins the helper, killing only the narrow
`return nil` mutation — and then **overwrites `m.notificationsView`** with a hand-built pane
before driving `d`, so the pane `NewModel` actually constructed is discarded and never observed.
Its own doc comment claims the opposite ("It goes through `NewModel` rather than asserting on
`notificationMarker` directly, because the defect being guarded is the pane being built
*without* the marker"); that sentence is false as written. `TestModel_MarkResultMsg_…` has the
same shape and so cannot cover the gap either.

`app.go:892` (the `ThemeSelectedMsg` pane rebuild) is the same hole by a second route: drop the
marker there and `u`/`d` die permanently after the user changes theme, with nothing failing.

### What the next iteration must do

1. Pin `NewModel`'s marker **argument**: build the model through `NewModel` with a provider that
   *is* a recording `provider.NotificationSource`, size it, `SetFeed` one unread row through the
   pane `NewModel` built (do **not** reassign `m.notificationsView`), press `d`, and require the
   recorder to have been called. Mutating `app.go:604`'s second argument to `nil` must fail it.
2. Cover `app.go:892` the same way — change theme via `ThemeSelectedMsg`, re-seed the feed, then
   press `d` and require the call.
3. Fix `TestModel_NotificationMarker_IsWiredToThePane`'s doc comment, or fold it into (1).
4. Correct finding 7's row: `prunedOverrides` is **not** pinned by the poll-debounce tests. Either
   say so plainly (acceptable — it is behaviour-neutral by construction) or add an assertion on
   `len(m.overrides)` after a `SetFeed` past the window.
5. Cosmetic, `list.go:739-785`: `setItemsPreservingSelection`'s doc comment was merged into the
   comment block that now documents `refreshItems`, so `refreshItems` carries two functions'
   docs and `setItemsPreservingSelection` (`:787`) has none. Split them.

## Review feedback closure: task 14 re-check survivors — 2026-07-30

The independent-review re-check above returned **INCOMPLETE** with two nil-marker survivors and
three lesser items. All five are now closed; task 14 re-ticked.

| Item | Fix |
|---|---|
| Blocking survivor — `NewModelWithStyles(appStyles, nil)` at the `NewModel` site stayed green | `TestModel_NotificationMarker_IsWiredToThePane` rewritten. It no longer replaces `m.notificationsView`; it feeds the pane app **built** via `SetFeed` and drives `d`, asserting the row disappears. The discriminator is the row vanishing rather than a stub being called, because on this path the marker is whatever app resolved from the real provider and the test has no handle on it — `markDone`'s nil guard returns before recording any override, so a markerless pane leaves the row on screen. Extracted as `assertPaneHasAMarker(t, m, site)` so every construction site is checked the same way |
| Second survivor — the same `nil` at the `ThemeSelectedMsg` pane rebuild | New `TestModel_ThemeChange_KeepsTheMarkerWiredToTheRebuiltPane`, same helper. See decision 71: the first attempt was itself vacuous, for the same reason it was unsafe |
| Finding 7's claim "pinned by both poll-debounce tests" was **false** — a *no-op* `prunedOverrides` was green | New `TestSetFeed_PrunesExpiredOverrides_ButKeepsLiveOnes`, reading `m.overrides` directly (same package) since the sweep has no on-screen observable. Asserts both halves: the expired entry goes, the live one stays. The no-op form and the clear-the-map-wholesale form now both die, the latter also via the two debounce tests |
| The validator's own count of what the debounce tests kill | Accepted and corrected in place rather than argued: they kill the inverted form only. The review-feedback table's finding-7 row overstated its pin |
| `setItemsPreservingSelection` left undocumented, its comment block having merged into `refreshItems`' | Comment block moved back above its function, with pointers to the tests that pin the clamp and the restore |

Validator items confirmed sound and not re-litigated: findings 1–6 and 8 hold;
`TestMarkDone_CursorLandsOnAConcreteRow` is not a tautology (forcing `SetCursor(0)` after every
re-derive fails 2 of its 4 subtests).

Mutation ledger for this closure, all killed: `NewModel` site passes nil → the wiring test **and**
the theme test; `ThemeSelectedMsg` site passes nil → the theme test; `prunedOverrides` returns
early unchanged → the new prune test; `prunedOverrides` returns the empty map → the prune test
plus both debounce tests. `gofmt -l` clean on all four touched files; `go test ./... -count=1`
fully green.

Process note: this is the second round in which an independent reviewer or validator found a real
defect in work the previous stage reported as complete with a clean mutation ledger — task 14's
implementer reported 6/6 no survivors and missed decision 65; the loop driver's own fix for the
reviewer's findings claimed a pin that did not exist. **A clean self-reported mutation ledger is
evidence, not proof**; the independent pass is where these are actually caught, and its cost has
been repaid every time in this run.
