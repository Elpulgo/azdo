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

## Tasks

- [x] 1. ADR `docs/adr/0001-notifications-capability-interface.md` — decisions 1, 2, 5. → done: file exists, ≤30 lines, `Status: Accepted`, has Context/Decision/Alternatives/Consequences
- [x] 2. `provider`: `Notification` type (provider-qualified identity + `Read`/`Done` per decisions 14, 15), `NotificationReason` enum (exactly decision 18's values), `NotifOpts`, `NotificationSource` (blocked by: 1). → done: `go build ./...` clean, `gofmt -l` empty, enum values match decision 18 one-for-one
- [x] 3. `ui/display`: reason → glyph + label + style map, plus `String()`/`ParseNotificationReason` next to the enum per decisions 24 and 26 (blocked by: 2). → done: every enum value returns non-empty glyph, label and named style; an unrecognised value renders as `Other`, never empty; table test covers all values plus one unrecognised input; asserts glyph, **exact label** and named style (convention 6 — a non-emptiness check on the label does not satisfy it); `String()` emits the decision-19 lowercase snake_case names and round-trips through `ParseNotificationReason` for all 12 values; `ParseNotificationReason` returns `(NotificationReason, bool)` per decision 26 — `(Other, false)` for an unrecognised string and for `unknown`, never an error and never a dropped row; `String()`'s out-of-range fallback is `"other"`, matching the display layer, with `Unknown` an explicit case returning `"unknown"`
- [x] 4. `github`: user-scoped client — `GET /notifications` with `all=true` (decision 12), pagination, `If-Modified-Since`, `X-Poll-Interval` (blocked by: 2). → done: `httptest` tests assert `all=true` in the query, `Link rel=next` followed, `If-Modified-Since` sent when a cached timestamp exists, 304 returns the cached slice unchanged, `X-Poll-Interval` parsed. Per decisions 27–29: the cache is mutex-guarded and returned **by copy**, so mutating a returned slice cannot change what a later 304 yields — assert that by mutating the first result and re-checking the second, since comparing two aliases of one backing array is a tautology; the cache is invalidated when `buildPath(opts)` changes; a 304 with no cached validator is an error; the walk is bounded by a page constant, tested with a self-referential `next`. Tests must also pin the method and path (`GET /notifications`) and `per_page`
- [x] 5. `github`: wire → neutral mapping, reason mapping, `subject.url` → web URL resolution (blocked by: 4). → done: table test maps every reason string in decision 18 plus an invented unknown → `Other`; `subject.url` resolves for pull/issue/commit and falls back to the repo URL otherwise; no panic on absent optional fields. Per decisions 33-35: `Release` resolves to the repo's `/releases` list page and the test asserts that **from GitHub's URL scheme, not from the implementation**; the id segment is shape-validated (digits for PR/issue, hex for commit) so `.../pulls` with no id, a trailing slash, or a non-numeric id falls back rather than emitting a clickable 404; every per-type URL is built on the `Repository.HTMLURL` prefix so an empty `FullName` can never yield `https://github.com//pull/42` and a GHE host is honoured; `ScopeDisplay` falls back to `Scope` inside the mapper, with a test row
- [ ] 6. `github`: mark read (`PATCH /notifications/threads/{id}`) + mark done (`DELETE`) (blocked by: 4). → done: tests assert method and path per call; ids are **strings** on the wire (`NotificationThread.ID`), so convention 11's guard is restated as: reject empty, non-numeric, and `<= 0`-after-parse — a raw `"-5"` interpolated into `/notifications/threads/-5` is the convention-11 failure shape, so keep the negative-input row; one-way read documented in the doc comment, not claimed as API-verified (decision 13)
- [ ] 7. `github`: implement `NotificationSource` on `Adapter` (blocked by: 5,6). → done: compile-time `var _ provider.NotificationSource = (*Adapter)(nil)`; conformance test following `adapter_conformance_test.go`; `NotifOpts.Max` is honoured by **truncating on return, not by stopping the walk** (decision 31), with a test proving a `Max` smaller than one page truncates **and** that a later call with a larger `Max` served from cache is not stuck at the smaller one; and `Adapter` forwards `PollInterval()` so it satisfies task 15's `PollIntervalHinter` (decision 30), asserted by a compile-time `var _` against that interface
- [ ] 8. `provider`: composite fan-out, merge/sort, `HasNotifications()` (blocked by: 2). → done: fans out only to backends implementing the interface; `HasNotifications()` false with zero capable backends and true with ≥1; merged output sorted newest-first; per decision 20 a failing backend still returns the others' rows and never empties the feed — test that case explicitly; per decision 25 `MarkRead`/`MarkDone` route by `Identity.Kind` and **not** `backendFor(scope)` — test that a row from an unconfigured repo still routes to a backend
- [ ] 9. `config`: `notifications` block with decision-19 key names, defaults, validation, `validDisabledPanes` entry, guard accepts notifications-only (decision 22) (blocked by: 2). → done: block loads with documented defaults; keys resolve lowercased (convention 9); `disabled_panes: notifications` validates; a config with only notifications enabled passes `Validate()`; per decision 26 an unrecognised `exclude_reasons` entry produces a **warning naming the bad value and the eleven accepted ones** and is then ignored — it must never silently act as `other`, and must never be a hard config error that stops the app from starting
- [ ] 10. Config-driven filter as a pure function (blocked by: 8,9). → done: table tests cover each knob alone, the full precedence chain, the `participating_only` + `exclude_reasons` compose case (decision 10), and that an unrecognised reason is only filtered when `other` is listed explicitly
- [ ] 11. `ui/notifications`: `listview` pane — dynamic repo column, unread emphasis, `f` reason filter (blocked by: 3,10). → done: renders through `View()` after a `WindowSizeMsg` without panic (convention 8); column count equals row-cell count in both single- and multi-repo cases (convention 7); unread rows assert a named style, not a substring (convention 6); the filter-collapse path and cursor survival are asserted (convention 14); a zero `UpdatedAt` renders as `—`, never as a year-0001 date (the mapper leaves it zero when the wire omits `updated_at`); an empty `ScopeDisplay` gets the same `—` treatment — decision 35 defaults it to `Scope`, but a thread whose `repository` payload is absent yields both empty, the one case the mapper cannot fix, and a blank cell reads as a rendering bug rather than as missing data; the `f` filter offers **only reasons actually present in the loaded feed**, never the full enum — otherwise it lists `Unknown`, which no mapped row can carry (decision 18), as a choice that matches nothing
- [ ] 12. `app`: register the tab first, remap number keys, `enabledTabs`, `state.TabID` (decisions 6, 9, 11) (blocked by: 11). → done: notifications is `enabledTabs[0]`; number keys map to the new order; `TabID "notifications"` round-trips through `state.yaml`; the tab is absent when no backend implements the capability, and present-but-empty when one does
- [ ] 13. `app`: the three render states — empty inbox, capability-unsupported, token-scope error (decisions 11, 17) (blocked by: 12). → done: three distinct renders, each asserted by its own test; the empty state reads as "you're clear", never as an error
- [ ] 14. `app`: `u` mark-read (one-way, decision 13) / `d` mark-done (blocked by: 13). → done: `u` issues one mark-read and updates the row optimistically, rolling back on API failure; `d` removes the row and restores it on failure; a poll that returns stale `unread` inside the debounce window does not flicker the row back
- [ ] 15. Polling integration honouring decisions 8 and 23 (blocked by: 12). → done: cadence is `max(X-Poll-Interval, configured)`; the hint reaches the poller via the separate `PollIntervalHinter` optional interface, **not** a new `NotificationSource` method (decision 23); a backend that does not implement the hinter falls back to the configured interval; a 304 response leaves the existing list intact rather than clearing it
- [ ] 16. Unread-count footer badge (decision 21) (blocked by: 12). → done: count is unread *after* config filters; the badge is hidden entirely at zero; visible from every tab
- [ ] 17. Help-modal section + `RemoveSection` wiring when the pane is disabled (blocked by: 12). → done: section lists `u`/`d`/`o`/`f`; disabling the pane removes it; the tabs binding line reflects the new order
- [ ] 18. `config`: regression test pinning `Save()` preservation — seed a file containing `metrics:` and `notifications:`, change only the theme, assert both blocks survive with every value intact (blocked by: 9). → done: the new test fails if `ReadInConfig()` is removed from `Save()` (verify by deleting it locally, watching the test fail, restoring it); fixtures go through `LoadFrom(<t.TempDir() path>)`, never a bare `Config` literal (convention 17)
- [ ] 19. Token-scope error state (decision 17) (blocked by: 13,18). → done: a 403 missing-scope response renders in-view naming the `notifications` scope and how to add it; 401-expired and generic failures render differently; the disable action writes `disabled_panes` via `Config.Save()` behind a confirm, on a key that is not `d` or `u`; its test uses a temp-path config (convention 17)
- [ ] 20. Docs: README (required `notifications` scope, upgrade note for existing tokens, how to disable), Architecture.md, config.yaml.example, FAQ (blocked by: 17,19). → done: every config key from decision 19 is documented; the required token scope and the upgrade path for existing tokens are stated
- [ ] 21. Fold phase-1 answers into `20260729-notif-p2-azdo.md` "Inputs from Phase 1" (blocked by: 20). → done: no `TBD` remains in that section

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
- Read state may be eventually consistent: a poll landing right after a `PATCH` could still
  return `unread`, flickering the optimistic update back. Task 14 holds the local intent until
  the server agrees rather than trusting the first poll that contradicts it.
