# Notifications Pane — Phase 2: Azure DevOps

**Ticket:** N/A
**Branch:** feat/notifications-azdo
**Author:** Oscar Larsson
**Created:** 2026-07-29
**Status:** Ready. Phase 1 (`20260729-notif-p1-github.md`) merged 2026-08-06 as `0f90acf`, 21/21 tasks.
Decisions 2 and 3 resolved 2026-08-06 and the task list is populated.

## Probe results

Task 1's spike, run 2026-08-06 against a live org.

**(a) `[System.Id] IN (@RecentMentions)` via `POST /_apis/wit/wiql`** — **WORKS.**
HTTP 200, 3 work items returned. The doc's blanket "macros are web-portal-only" claim is
wrong for this macro over REST, exactly as it is for `@Me`. Decision 3 stage 1 ships as
written; **the `System.History CONTAINS WORDS` fallback is not needed and must not be built.**

**(b) `mentions[]` on `GET /wit/workItems/{id}/comments`** — **populated by default.**
Same count of mention-bearing comments with and without `$expand=all` (2 either way), so
`mentions` is returned despite being absent from `CommentExpandOptions`. **The client must
not send `$expand=all`** — it would cost rendered-text payload for nothing.

**(c) `CommentMention.targetId` vs `GetCurrentUserID()`** — **exact match, no normalising.**
The comment carried mentions of three distinct users; the authenticated user's GUID matched
one verbatim, byte-for-byte, same casing and hyphenation. A plain `==` is correct — no
lowercasing, no GUID parsing. The two non-matching ids confirm the filter actually
discriminates rather than trivially accepting everything.

**Consequence:** decision 3 stands unchanged and task 5 is unblocked. No PAT scope beyond
the current set was needed — all three calls succeeded with the existing token (task 16's
"any PAT scope beyond the current set" question is answered: none).

## Goal

Azure DevOps implements `provider.NotificationSource` by synthesizing an attention feed
from queries we already run, with read/done state persisted locally.

## Constraints

- The UI must not change. If this phase touches `ui/notifications`, the phase-1
  abstraction was wrong — fix the abstraction, not the pane.
- Azure-only configs gain the tab **automatically** the moment the adapter implements
  `NotificationSource`: phase 1 gates tab visibility on capability, not on provider
  (phase-1 decision 11). Needing an app-layer change here means phase 1 got it wrong.
- Azure has **no readable inbox**. The Notification API only manages subscription *rules*.
- No server-side read/done state exists → local persistence is mandatory, and it must
  survive a feed that is recomputed from scratch on every poll.
- Synthetic items have no stable server ID. The identity key must be derived and stable
  across polls, or every poll resurrects everything the user already dismissed.
- Additional PAT scope may be required; document whatever the queries need.

## Scope

**In scope:**
- `azdevops`: `NotificationSource` implementation composing these sources
  1. PRs awaiting my review → `review_requested`
  2. @mentions in work-item discussions → `mentioned`
  3. Recently assigned work items (bounded lookback, decision 6) → `assigned`
  4. My failed pipeline runs → `ci_activity` — **not `ci_failed`**, which is not a member of
     the phase-1 enum; see decision 8
- Stable synthetic identity key + local read/done store, reusing `internal/state` patterns
- Activity-stamp reconciliation so only new or changed subjects surface as unread (decision 2)
- Auto-expiry rules (e.g. review completed → item auto-done) — mostly free, see decision 7
- Config: restructure `notifications` into shared + `github` + `azure` blocks, with per-source
  toggles under `notifications.azure.sources` (decision 13)
- Docs: ADR for the synthetic-feed and local-state design, README, Architecture.md, FAQ

**Out of scope:**
- Managing Azure subscription rules
- Comment replies on my PR threads (needs per-thread polling — fast-follow)
- Cross-machine sync of local read/done state
- Any change to the GitHub path beyond shared-code refactors

## Approach

Each source is an independent query returning `[]provider.Notification`; a composer runs
them concurrently, merges, and applies local read/done state before returning. Because the
feed is recomputed rather than fetched, correctness hinges on the identity key: it must be a
function of the subject (kind + scope + id) plus the activity that made it interesting, so
new activity on a dismissed subject resurrects it while a re-poll of unchanged data does not.
Local state holds `{key: {read, done, last_seen_activity}}` and is pruned on a TTL so the
file does not grow without bound.

## Inputs from Phase 1

Populated 2026-07-30 from the shipped phase-1 implementation (spec
`20260729-notif-p1-github.md`, tasks 1-20). Every answer below was read out of the source,
not out of phase 1's prose — where the two disagreed, the source won.

- **Reason enum coverage:** eleven configurable reasons, in `provider.NotificationReason`
  declaration order: `review_requested`, `mentioned`, `assigned`, `authored`, `commented`,
  `state_changed`, `ci_activity`, `security_alert`, `approval_requested`, `subscribed`,
  `other`. A twelfth, `unknown`, is the reserved zero value: it is never emitted by a
  mapper and is rejected in `exclude_reasons`. **`provider` is the single source of truth**
  — `config.acceptedNotificationReasons()` generates its list from the enum rather than
  restating it, and Azure mappers must do the same. `approval_requested` and
  `ci_activity` were defined with Azure in mind and need no new members; the phase-2
  sources most at risk of inventing one are work-item assignment (use `assigned`) and PR
  comment replies (use `commented`). Adding a member is allowed but is a **phase-2
  decision, not an implementation detail** — it widens a user-facing config vocabulary
  that phase 1 has now documented as closed.
- **Identity shape (input to decision 2, does not settle it):** phase 1 shipped and
  exercised `provider.Identity{Kind, Scope, ScopeDisplay, ID}`, with
  `SameItem` comparing **`Kind`, `Scope`, `ID` only** — `ScopeDisplay` is presentation and
  is deliberately excluded, since it varies by config. `CompositeProvider` already routes
  mark-read/mark-done by `Identity.Kind`, so that field is load-bearing for phase 2's
  triage routing before phase 2 writes a line. What phase 1 did **not** need is decision
  2's `activity_stamp` component: GitHub's server owns read state, so an identity only has
  to name the *entity*. Azure's locally-tracked state is what makes a stamp a live
  question — "this work item, as of this comment" is a different row from "this work item,
  as of the previous one". Decision 2 therefore remains open; phase 1 constrains it only in
  that whatever key is chosen must remain derivable from an `Identity`, because the pane,
  the override layer and the composite's routing all key off that type today.
- **Composite fan-out semantics:** a failing backend **never empties the feed** (phase-1
  decision 20). `CompositeProvider.List` fans out concurrently to capable backends only,
  merges, sorts by `UpdatedAt` descending, and returns the surviving rows *alongside* a
  `*provider.PartialError{Failed, Total, Errors}`. Two traps phase 2 must not re-introduce,
  both of which cost phase 1 a defect: **`Total` counts capable backends, not
  `len(backends)`** (decision 41) — otherwise an incapable backend makes a single GitHub
  403 satisfy "all backends failed" and empties the feed; and **zero capable backends
  returns `(nil, nil)`**, because `len(errs) == total` is vacuously true at `0 == 0`.
  A backend returning rows *and* an error has its rows discarded deliberately: a backend
  reporting failure cannot vouch for what it did return. Once Azure is capable, both
  backends are live simultaneously and this path stops being theoretical.
- **Filter placement:** **neither `provider` nor `app` — it lives in the UI package**, as
  two pure functions in `internal/ui/notifications/filter.go`:
  `NotifOptsFromConfig(cfg) provider.NotifOpts` (the server-side narrowing handed to the
  fetch) and `FilterNotifications(rows, cfg) []provider.Notification` (the client-side row
  predicates). **`FilterNotifications` is provider-agnostic and reusable verbatim** — it
  matches on `Notification` fields, not on anything GitHub-shaped. `NotifOptsFromConfig`
  is **not** reusable as-is: `participating_only` is a GitHub inbox concept with no Azure
  equivalent, and `Since` is truncated to the day specifically so `buildPath` stays stable
  and GitHub's conditional-request cache keeps hitting (phase-1 decision 75). Note the five
  knobs are **independent row predicates, not a pipeline** — the only precedence is that
  `only_configured_repos` beats `include_repos`/`exclude_repos` selection (decision 50);
  the rest are an order-independent AND. Do not document or implement them as a chain.
- **Subject → web URL:** `Notification.WebURL` is **best-effort, resolved at the adapter
  boundary, and may legitimately be `""`**. The degradation ladder Azure builders must
  match: unrecognised subject type, subject carrying no URL, or a trailing id segment
  failing shape validation all fall back to the **repository/project page** rather than
  guessing a deep link — *a wrong-but-clickable 404 is worse than a landing page*
  (decisions 33/36). `""` means "nothing to open", and consumers are contractually required
  to treat it as such rather than opening it; the pane already guards this and shows
  "No URL for this notification". Azure's analogue of the `Release` special case (which
  resolves to `/releases` because the payload lacks a tag name) is any entity whose deep
  link needs an id the notification source does not carry.
- **Polling contract:** two separate pieces. (1) `polling.NotificationsPoller` is a
  **distinct type from `polling.Poller`, not a generalisation of it** (decision 69) — it
  emits its own `NotificationsTickMsg` because app.go's single `case polling.TickMsg`
  already drives the pipeline poller unconditionally. (2) The cadence hint is
  `provider.PollIntervalHinter`, a **second optional capability interface** separate from
  `NotificationSource`. **Azure simply does not implement it**, and that is the designed-for
  case: `CompositeProvider` takes the max over capable backends that implement it, and a
  backend that does not falls back to the configured interval. The `max(hint, configured)`
  arithmetic lives in **app.go, not the poller** — the poller has no dependency on the
  hinter interface at all. Two phase-1 rules carry over: the interval floor is
  `MinInterval` (so a misconfigured `poll_interval: 1` cannot produce sub-minimum polling),
  and **the poller's timer must be gated on the same predicate as the pane it feeds**
  (decision 73) — gating only the initial fetch is insufficient, because `OnTick` re-arms
  itself, so one ungated start is a permanent chain.
- **Unread semantics:** the badge counts rows in the pane's held feed where `!Read`, with
  active `u`/`d` overrides folded on top so a just-cleared row is reflected before the next
  poll lands. It counts the **config-filtered** inbox (the decision-19 filters are applied
  before the feed reaches the pane) but **deliberately ignores the `f` reason filter**
  (decision 68): `f` is an interactive local narrowing, and since the badge is visible from
  every tab, a per-reason subtotal would follow the user to a tab where the filter
  producing it is invisible. **This definition survives a locally-tracked read state, but
  only if Azure's local state is folded into `Notification.Read` at or below the adapter
  boundary** — `UnreadCount` reads that one field and must not grow a second, Azure-specific
  notion of "read". The override layer is a *finite debounce buffer*, not a store: it exists
  to bridge the round trip to a server that owns the truth. Azure has no such server, so
  phase-2 decision 1's local state file is the owner and must be authoritative at read time,
  not an override laid on top of a feed that disagrees with it.

## Decisions

| # | Question | Decision | Rationale |
|---|----------|----------|-----------|
| 1 | Where does local read/done state live? | New file under the `internal/state` dir, separate from `state.yaml` | Navigation state is disposable; triage state is not. Different lifetimes, different files |
| 2 | Identity key shape? | `Identity{Kind: KindAzure, Scope: <project API name>, ID: "<source>/<entity>/<entity_id>"}`. The activity stamp is a **stored value, not a key component** | See "Decision 2 in full" below |
| 3 | Which @mention query? | Two stages: WIQL **narrows** candidate work-item ids, the Comments API **confirms and timestamps** each mention via `mentions[].targetId` | See "Decision 3 in full" below. WIQL cannot answer this alone — it has no per-mention timestamp |
| 4 | Auto-expiry? | Yes, per-source rules | A completed review that stays in the feed trains the user to ignore the pane |
| 5 | Per-source config toggles? | Yes, four booleans under `notifications.azure.sources`, all defaulting on | Orgs differ wildly in which sources are signal vs. noise |
| 6 | Snapshot/delta, or a lookback window? | **Bounded lookback window**, no snapshot. Each source queries "changed within the last N days" and local read state does the rest | A snapshot is a second piece of durable state that can be lost, and losing it floods the feed. A lookback is stateless and idempotent — the same poll twice yields the same rows. Also answers the phase's own "does a fresh install flood?" unknown: it cannot, the window bounds it |
| 7 | How does auto-expiry actually work? | It is **implicit in recomputation**. A source that stops matching a subject stops returning it, and the row is gone. The only explicit work is pruning the now-orphaned local state row on a TTL | Decision 4 asked for per-source expiry rules; recomputing the feed from scratch already provides them. Writing explicit "review completed → mark done" rules would be a second, drift-prone encoding of what the query already says |
| 8 | Does `ci_failed` need a new reason enum member? | **No.** Failed pipeline runs map to the existing `ci_activity` | Phase 1 documented the eleven-reason vocabulary as closed and `ci_activity` as defined with Azure in mind. `ci_failed` appears in this spec's own prose and in `gh-notifications-arc.md`, but never existed in `provider.NotificationReason` — adding it would widen a user-facing config vocabulary for no triage benefit |
| 9 | Does config validation need changing? | **Yes** — `config.go:614`'s `notificationsCounts := c.IsPaneEnabled("notifications") && c.HasGitHub()` must widen to "any notification-capable backend" | This is the one app-layer change the phase's constraints do **not** forbid: the constraint bans changes to *tab visibility* and to `ui/notifications`, and this is neither — it is the all-panes-disabled startup guard. The existing comment at `config.go:610-613` predicts this exact edit ("will need revisiting when a second notification-capable backend arrives") |
| 10 | Azure poll cadence? | **The adapter self-throttles.** One poller and one interval stay as phase 1 shipped them; `notifications.azure.min_poll_interval` makes `Adapter.List` return its previous result when called sooner than that. Azure still does not implement `PollIntervalHinter` | Azure costs 4+ queries per project per cycle against GitHub's one conditional request, so a shared interval prices the whole feed at Azure's cost. But there is no per-backend cadence to configure: phase 1 shipped exactly one `NotificationsPoller` emitting one `NotificationsTickMsg`, and `CompositeProvider.List` fans out to every capable backend on that single tick. A second poller and tick message would contradict this phase's "the UI must not change" constraint. Self-throttling puts the cost control where the cost is, and the poller keeps no knowledge of it. **An earlier draft of this decision specified `notifications.azure.poll_interval` as a second poller interval — that is not implementable; do not reinstate it** |
| 11 | One merged feed or per-project sections? | **One merged feed.** `Scope` already carries the project and the pane already renders a scope column | The pane is provider-agnostic by constraint; per-project sectioning would be a UI change, which this phase forbids. `MultiClient` fan-out already merges everywhere else in the app |
| 12 | Do Releases-arc approvals mirror into this feed? | **No** — out of scope for phase 2, revisit when `release-view-arc.md` is spec'd | Mirroring needs the release arc to exist first. Deciding the ownership question now would be designing against an unwritten spec |
| 13 | Config shape now that a second provider exists? | **Shared keys at top level, provider-specific keys nested** under `notifications.github` and `notifications.azure`. See the block below | Of phase 1's nine keys, only five are genuinely provider-neutral. `participating_only` has no Azure equivalent by phase 1's own account, and `only_configured_repos` is vacuously true on Azure (the adapter only ever queries configured projects) — both sat at top level looking shared when they never were. Nesting states the truth the flat shape obscured |
| 14 | Migration for the moved keys? | **None. No shim, no deprecation warning, no compatibility path.** The nested shape is simply the shape | Phase 1 merged to `main` but has **not been released** — these key names have never reached a public build, so there is no installed config anywhere that uses the flat form. Writing a migration would be compatibility code for a version that never existed. Confirmed by Oscar 2026-08-06 |
| 15 | `max_items` with two live backends? | Cap the **merged feed** after the composite's sort, not per-backend | Phase 1 applies it inside each fetch, which was unambiguous with one backend and silently becomes "up to 2×N rows" with two. A cap the user writes once should mean what it says. Requires moving the truncation from the adapter to the composite — a phase-1 refactor, which the phase's constraints permit (they forbid UI changes, not shared-code refactors) |

The approved config shape:

```yaml
notifications:
  # shared — apply to the merged feed regardless of backend
  exclude_reasons: []
  unread_only: false
  exclude_repos: []
  include_repos: []
  max_items: 0
  poll_interval: 0

  github:
    participating_only: false
    only_configured_repos: false
    since_days: 0

  azure:
    lookback_days: 14
    min_poll_interval: 300
    sources:
      review_requested: true
      mentioned: true
      assigned: true
      ci_failed: true
```

Two notes on names that are deliberate, not oversights:

- **`sources.ci_failed` names a *source*, not a reason.** Decision 8 established that `ci_failed`
  is not a member of `provider.NotificationReason` and that this source emits `ci_activity`. The
  config key keeps the `ci_failed` spelling because it describes what the source *queries* (failed
  runs) and reads better than `ci_activity: true`, which would suggest toggling a reason. Any test
  or doc touching both must not treat the two spellings as the same vocabulary.
- **`exclude_repos`/`include_repos` stay at top level under their phase-1 names**, even though they
  glob over `Identity.Scope` — an `owner/repo` on GitHub but a project name on Azure. Renaming them
  to `*_scopes` was considered and rejected: "scope" is internal vocabulary that appears nowhere in
  the user-facing config today, and the keys do work correctly against both. Document the Azure
  meaning rather than renaming.

`since_days` moving under `github` also resolves a collision this spec would otherwise have
carried: it and Azure's `lookback_days` are the same idea with incompatible zero values. `0` means
"no bound" — correct for a GitHub inbox, catastrophic for "work items assigned to me", where
unbounded means every work item ever assigned. Separate keys under separate providers, each with a
zero value that is safe in its own context.

### Decision 2 in full — the identity key

Rejected: putting the activity stamp **in** the key (`kind:scope:entity_id:activity_stamp`, the
original candidate). It makes resurrection automatic — new activity yields a key nothing has seen,
so the row is unread by construction — but it pays for that three times over:

- `Identity` stops naming an entity and starts naming an *event*. `SameItem` on the same work item
  returns false across a comment, and phase 1's inputs record `SameItem` and `Identity.Kind` as
  load-bearing for the composite's mark-read/mark-done routing.
- The pane's cursor is keyed on identity, so a comment landing mid-triage moves the selection.
- The state file grows one row per activity event forever, making TTL pruning load-bearing for
  correctness rather than for hygiene.

Chosen instead: the key names the **subject**, and the stamp is a value stored beside it.

```
Identity.ID = "<source>/<entity>/<entity_id>"     e.g. "review/pr/1234", "mention/wi/5678"
local state  = { key: {read, done, last_activity, last_seen} }
```

- `<source>` rather than `<reason>` so a later remap of a source to a different reason does not
  churn every key and resurrect the user's whole triaged backlog.
- `<entity>` is required: work item 42 and PR 42 in one project would otherwise collide. `Kind` is
  the *backend* (`azure`), not the entity type.
- Two sources surfacing the same entity stay two rows — "PR 123 awaiting your review" and "PR 123
  where you were mentioned" are genuinely separate attention items and are triaged separately.

Resurrection becomes an explicit reconcile step instead of a side effect of key churn: on each
poll, if a subject's current activity timestamp is newer than the stored `last_activity`, clear
`read`/`done` and advance the stamp. That is a pure function over (rows, state) → rows, which is
the whole point — it can be table-tested exhaustively, where key-churn semantics can only be
tested end-to-end.

Per phase 1's constraint, the key stays derivable from an `Identity`: it *is* one.

### Decision 3 in full — the @mention query

Neither candidate in the original framing works alone. The deciding constraint was the one the
question itself named — a usable timestamp per mention — and **WIQL cannot supply it**: a WIQL
query returns work items, so the newest timestamp available is `System.ChangedDate`, which says
when the item last changed, not when *you* were mentioned. Any WIQL-only design has to treat "item
changed" as "you were mentioned", which resurfaces a dismissed mention on every unrelated edit.

The Comments API does supply it. Each `Comment` carries `createdDate` **and** a structured
`mentions: CommentMention[]`, where `CommentMention.targetId` is "the resolved target of the
mention… an example of this could be a user's tfid". So mentions are matched by identity, not by
scraping display names out of HTML — and `Client.GetCurrentUserID()` (`client.go:198`) already
resolves the id to match against. `GetWorkItemComments` already exists (`comments.go:35`).

So: **WIQL narrows, Comments confirms and timestamps.**

Stage 1 narrows to candidate work-item ids. Preferred form is the `@RecentMentions` macro:

```sql
SELECT [System.Id] FROM WorkItems
WHERE [System.TeamProject] = @project AND [System.Id] IN (@RecentMentions)
```

**This is unverified over REST and must be probed before it is relied on** (task 1). Microsoft's
macro reference explicitly lists `@RecentMentions` among macros "only supported from the web
portal", alongside `@CurrentIteration` — which genuinely does fail over REST. The list is not
reliable as stated, since it also implies `@Me` is portal-only while `ListMyWorkItems`
(`workitems.go:255`) ships `@Me` over the REST WIQL endpoint today and works. That makes
`@RecentMentions` a plausible-but-unconfirmed capability, which is exactly the shape convention 28
was written about. Probe it; do not reason from the `@Me` precedent.

Fallback if the probe fails — full-text over the History field, bounded by the lookback window:

```sql
SELECT [System.Id] FROM WorkItems
WHERE [System.TeamProject] = @project
  AND [System.History] CONTAINS WORDS '<display name>'
  AND [System.ChangedDate] >= @Today-<N>
```

The fallback is strictly worse and its weaknesses are load-bearing, not cosmetic: it needs the
user's display name (not the id), it matches anyone who typed that name as plain text, and
`CONTAINS WORDS` requires a full-text index that some deployments do not have. All three are
survivable **only because stage 2 re-filters by `targetId`** — stage 1 is allowed to over-match,
never to under-match. Under either form, stage 2 is what decides.

Sources: [Query fields, operators, macros, and variables](https://learn.microsoft.com/en-us/azure/devops/boards/queries/query-operators-variables?view=azure-devops) ·
[Comments — Get Comments Batch (7.1-preview.4)](https://learn.microsoft.com/en-us/rest/api/azure/devops/wit/comments/get-comments-batch?view=azure-devops-rest-7.1) ·
[Use @mentions in work items and pull requests](https://learn.microsoft.com/en-us/azure/devops/organizations/notifications/at-mentions?view=azure-devops)

## Tasks

Task 1 is a spike and gates task 5 only; everything else can start immediately.

- [x] 1. **Spike: probe `@RecentMentions` and `CommentMention` over REST against a real org.** Not a code task — a throwaway script plus a finding recorded in this spec. → done: this spec gains a "Probe results" section stating (a) whether `[System.Id] IN (@RecentMentions)` returns rows via `POST /_apis/wit/wiql`, or the exact error if not; (b) whether `GET /wit/workItems/{id}/comments` populates `mentions[]` **by default** — `mentions` is absent from `CommentExpandOptions` (`none|reactions|renderedText|renderedTextOnly|all`), so if it arrives only under `$expand=all`, that is what the client must send; (c) whether `CommentMention.targetId` string-equals `GetCurrentUserID()`'s value verbatim or needs normalising. Decision 3's fallback is adopted only if (a) fails. If (b) or (c) fails there is **no fallback** and decision 3 must be reopened — say so rather than working around it
- [x] 2. **`azdevops`: local triage store** — `notifications.yaml` beside `state.yaml`, own `state.Store` instance (decision 1). → done: `map[string]TriageEntry` with `{Read, Done bool; LastActivity, LastSeen time.Time}`; round-trips through the store's atomic write; a missing file loads as empty, not an error; **the file path is derived the same way `state.yaml`'s is** and a test asserts the two are different paths in the same dir; convention 17 applies — every fixture goes through a `t.TempDir()` path, never a bare struct literal
- [x] 3. **`azdevops`: identity key + reconcile function** (decision 2). → done: `NotifKey(source, entity, id) string` producing `<source>/<entity>/<id>`, guarded per convention 11 (reject `<= 0` ids, not `== 0`, with a negative-input test row); `Reconcile(rows []provider.Notification, state map[string]TriageEntry, now time.Time) ([]provider.Notification, map[string]TriageEntry)` as a **pure function**, table-tested for: unseen subject → unread; seen subject, unchanged stamp → stored `read`/`done` applied; seen subject, **newer** stamp → `read`/`done` cleared and stamp advanced; seen subject, **older** stamp (clock skew / reordered poll) → **triage** state (`Read`/`Done`/`LastActivity`) left untouched, *not* cleared, but `LastSeen` **is** advanced like every other branch — `LastSeen` is presence bookkeeping, not triage, and freezing it lets a row present in every poll be TTL-pruned and resurrect (amended 2026-08-06 after review; decision 2's prose only ever discusses clearing `read`/`done`) — the equal and older cases must be separate rows, since `>` and `>=` differ only on the equal case and that is the every-poll case; `done` rows dropped from the returned slice; orphaned entries older than the TTL pruned, with a boundary row exactly at the TTL (convention 13's shape)
- [x] 4. **`azdevops`: source — PRs awaiting my review** → `review_requested` (blocked by: 3). → done: reuses `MultiClient.ListPullRequestsAsReviewer` (`multiclient.go:248`), no new client method; key is `review/pr/<id>`; activity stamp is the PR's last-update timestamp so a new push resurrects a dismissed row; `WebURL` follows phase 1's degradation ladder — a PR whose repo/id cannot be resolved falls back to the project page, never to a guessed deep link, and `""` is a legal result
- [x] 5. **`azdevops`: source — @mentions in work-item discussions** → `mentioned` (blocked by: 1,3). → done: stage 1 narrows via the form task 1 confirmed; stage 2 fetches comments for candidates and keeps only those with a `mentions[].targetId` equal to `GetCurrentUserID()`; key is `mention/wi/<id>`; activity stamp is the **newest matching comment's `createdDate`**, not the work item's `ChangedDate` — a test must pin that an unrelated edit after the mention does not advance the stamp, which is the entire reason stage 2 exists; candidate fan-out is bounded by a constant, and when the bound truncates it **keeps the most recent candidates, not the oldest** — work-item ids are sequential and never reused, so truncating toward low ids systematically discards exactly the mentions the feed exists to surface; the truncation is **surfaced to the caller as structured data on the source's return**, never silently swallowed and never written to stderr — this repo has no logger, nothing calls `slog.SetDefault`, and an unredirected write mid-poll lands on top of Bubble Tea's alt-screen render (amended 2026-08-06 after validation; the line originally said "logged", which assumed a logging facility that does not exist here — task 8 decides how to surface it)
- [x] 6. **`azdevops`: source — recently assigned work items** → `assigned` (blocked by: 3). → done: WIQL over `[System.AssignedTo] = @Me AND [System.ChangedDate] >= @Today-N AND [System.State] <> 'Closed' AND [System.State] <> 'Removed'` (decision 6 — no snapshot, no delta state); **the state clause is not optional** — closing a work item is a revision, so it advances `ChangedDate`, and without the clause the one action that completes your work is also the one that guarantees the row returns as unread, inverting decision 7's auto-expiry (amended 2026-08-06 after review; `ListMyWorkItems` at `workitems.go:261-266` already excludes both states); key is `assigned/wi/<id>`; a test proves the same poll run twice yields identical rows and identical state (idempotence is the property that replaces the snapshot); a fresh install with an empty state file surfaces at most the window's worth of items, asserted with a fixture spanning items inside and outside the window
- [x] 7. **`azdevops`: source — my failed pipeline runs** → `ci_activity` (blocked by: 3). → done: queries runs triggered by me with a failed result; key is `cifail/run/<id>`; reason is `NotificationReasonCIActivity` per decision 8, asserted by name so a future `ci_failed` member cannot be silently swapped in. **The query must narrow server-side, not in Go over `ListPipelineRuns`'s output** (amended 2026-08-06 after review): `ListPipelineRuns` (`pipelines.go:12`) fetches the N most recent builds in the project across all pipelines, all users and all results, so a client-side filter spends `$top` on other people's builds — this is the only one of the four sources whose narrowing is not server-side (review uses `reviewerId`, assigned and mentioned use WIQL), and on a busy project your failure ages out of the window and **disappears from the feed while still unread**, with the failure mode worsening exactly as team activity rises. Add a dedicated client method passing `statusFilter=completed&resultFilter=failed&requestedFor=<id>&minTime=<now-lookbackDays>&queryOrder=finishTimeDescending`; leave `ListPipelineRuns` untouched for the pipelines pane, and **keep the Go-side `requestedFor`/result check as a belt-and-braces re-check** so an ignored or mis-typed server parameter degrades to over-fetching rather than to attributing someone else's build to you. `minTime` also makes this source honour decision 6's lookback — it is the only source that currently does not, which would leave task 11's `lookback_days` silently governing three of four sources. **The subject is not permanently stable:** Azure's *rerun failed jobs* / *rerun stage* re-executes inside the **same run id** on YAML pipelines (classic queues a new id), recomputing `result` and `finishTime` — so `FinishTime` is load-bearing, a re-failure correctly resurfaces the row as unread, and a green rerun expires it per decision 7; that behaviour is wanted, but it must be documented as what it is and pinned by a test that advances `FinishTime` on the same id, not only by the equal-stamp repoll case `Reconcile` already covers
- [ ] 8. **`azdevops`: compose sources concurrently and implement `NotificationSource`** (blocked by: 4,5,6,7). → done: compile-time `var _ provider.NotificationSource = (*Adapter)(nil)` plus a conformance test following `adapter_conformance_test.go`; sources run concurrently and **one failing source degrades to the others rather than emptying the feed** — the same rule phase 1's decision 20 enforces at the composite layer, restated here because a source is to the Azure adapter what a backend is to the composite, and phase 1 lost a defect to exactly this; `Adapter` does **not** implement `PollIntervalHinter` (decision 10), asserted by a negative compile-time check; local state is folded into `Notification.Read` **at this boundary**, per phase 1's unread-semantics constraint — nothing above the adapter may learn that Azure read state is local; **and the composite will discard your rows if you hand it an error** — `composite.go:669-672` does `if r.err != nil { errs = append(errs, r.err); continue }`, and `composite.go:637-641` documents that as deliberate (a backend reporting failure cannot vouch for the completeness *or ordering* of what it returned). So propagating task 4's `*PartialError` upward from `Adapter.List` re-blanks the Azure half of the feed one layer up, re-introducing the exact defect task 4 was reopened to fix. Decide it deliberately: either `Adapter.List` absorbs partial failures and returns `(rows, nil)`, or phase 1's composite rule is revisited — do not leave it to fall out of the code (added 2026-08-06 from task 4's re-validation). **Resolved 2026-08-07: `Adapter.List` absorbs, and the composite rule stands.** The composite's rationale is right *at its own boundary* — it cannot know whether a backend's partial result is sorted or complete — but the Azure adapter is the layer that merged and sorted these rows, so it can vouch for them, and revisiting the composite rule would reopen a phase-1 defect for every backend to fix a problem local to this one. Concretely: `Adapter.List` returns `(rows, nil)` whenever it has **at least one** row, and propagates the error only when it has **none** — so a total outage still surfaces as an error while a single expired project PAT degrades to a shorter feed. The cost is real and is accepted knowingly: a partial Azure failure becomes **invisible**, since the composite renders `"%d of %d backends failed to load"` from errors alone and the adapter has no non-error channel to report on. That is the lesser harm for an attention feed — missing rows is degradation the user can recover from, an empty pane is an outage that teaches them to stop trusting the tab — but it is a gap, and widening `NotificationSource` with a warnings channel is recorded under `## Unknowns` for Oscar rather than invented here
- [ ] 9. **`azdevops`: `MarkRead`/`MarkDone` write to the local store** (blocked by: 2,8). → done: both take a `provider.Identity` and write through `Identity.ID` as the key; marking an id the store has never seen creates the entry rather than erroring — the composite routes by `Identity.Kind` and cannot know what the store has; `MarkDone` on an already-done id is a no-op, not a double-write; writes go through the debounced store and a `Flush()` on shutdown guarantees durability
- [ ] 10. **`config`: restructure `NotificationsConfig` into shared + `github` + `azure`** (decisions 13, 14) (blocked by: 8). → done: the block matches decision 13's YAML exactly; `participating_only`, `only_configured_repos` and `since_days` move under `notifications.github` and the five shared keys stay at top level; keys resolve lowercased at every nesting level (convention 9 — verify the nested maps too, not just the root, since that is the untested half); **no migration shim and no deprecation warning for the old flat keys** (decision 14) — a flat `notifications.participating_only` is simply an unrecognised key, and a test pins that it is *not* silently honoured, since a half-removed shim is worse than none; per convention 25 the documented key list is derived from the struct, not restated by hand
- [ ] 11. **`config`: `notifications.azure` values and source toggles** (decisions 5, 6, 10) (blocked by: 10). → done: four independent source toggles, all defaulting **on**; `lookback_days` defaults to 14 and `min_poll_interval` to 300, both rejecting negatives with the same message shape as the existing `since_days` check; **`lookback_days` is additionally clamped to `orphanTTL` (30 days)** — beyond that, an item can be pruned from the triage store while still inside the query window and resurface as unread with nothing having touched it, since `assignedQueryTop` means "inside the window" and "returned by the poll" are different sets (added 2026-08-06 from task 6's review; `orphanTTL`'s doc comment states the guarantee unconditionally and must be corrected to name the condition); **zero is not "unbounded" for `lookback_days`** — it falls back to the default, and a test pins that, because the shared-key convention that zero means widest is exactly what makes this key dangerous (decision 13's closing note); disabling every source is legal and yields an empty Azure feed, **not** a config error — and must not make the adapter claim incapability, since that would silently hide the tab in an Azure-only config
- [ ] 12. **`provider`: move `max_items` truncation from adapter to composite** (decision 15) (blocked by: 8). → done: `CompositeProvider.List` applies the cap after its merge-and-sort, so `max_items: 50` yields at most 50 rows with two live backends rather than up to 100; the per-backend truncation phase 1 put in `NotifOpts.Max` handling is removed, not left in place to double-apply; a test drives two capable backends each returning more than the cap and asserts the merged length **and** that the surviving rows are the globally newest — a length-only assertion passes against a naive truncate-before-sort; phase 1's existing single-backend `Max` tests must still pass unchanged
- [ ] 13. **`config`: widen the all-panes-disabled guard** (decision 9) (blocked by: 10). → done: `config.go:614`'s `&& c.HasGitHub()` becomes "any notification-capable backend configured"; the error message at `config.go:617` no longer says the tab "needs a GitHub backend"; the stale comment at `config.go:610-613` predicting this change is removed, not left contradicting the code; tests cover Azure-only, GitHub-only, and both, each with the other three panes disabled
- [ ] 14. **`azdevops`: adapter self-throttling** (decision 10) (blocked by: 8,11). → done: `Adapter.List` returns its previous result unchanged when called within `min_poll_interval` of its last real query, so the single shared poller cannot price the whole feed at Azure's cost; **nothing in `polling` or `app` changes** — no second poller, no second tick message, no new interval arithmetic (phase 1 decision 69 keeps `max(hint, configured)` in app.go untouched); the cached slice is returned **by copy** under a mutex, so a caller mutating it cannot corrupt the next throttled return — phase 1 lost a defect to exactly this in its conditional-request cache, and the test must prove it by mutating the first result and re-checking the second, since comparing two aliases of one backing array is a tautology; a throttled return must not be mistaken for a failure and must not clear the feed; `MarkRead`/`MarkDone` are **never** throttled and take no lock shared with `List`
- [ ] 15. **ADR `docs/adr/000N-azure-synthetic-notification-feed.md`** — decisions 2, 3, 6, 7 (blocked by: 8). → done: follows `docs/adr/0001`'s shape (≤30 lines, `Status: Accepted`, Context/Decision/Alternatives/Consequences); the Alternatives section records the stamp-in-key design and *why* it lost, since that is the decision most likely to be re-proposed by someone reading only the original candidate
- [ ] 16. **Docs: README, Architecture.md, config.yaml.example, FAQ** (blocked by: 13,14,15). → done: the full nested config block from decision 13 documented, derived from the struct per convention 25 — including which keys are shared and which are provider-specific, since that distinction is the whole point of the restructure; `exclude_repos`/`include_repos` documented as matching an `owner/repo` on GitHub and a **project name** on Azure (decision 13's second note); `sources.ci_failed` documented as a source toggle that emits the `ci_activity` reason, so the two spellings are not read as one vocabulary; the local-state file's path, purpose and "not synced across machines" caveat stated; any PAT scope beyond the current set named explicitly, or its absence confirmed (task 1 answers this); per convention 26, grep for every place the old GitHub-only notifications requirement is stated — README, FAQ, `Architecture.md`, `cmd/azdo-tui`'s help blocks and the auth wizard all asserted it in phase 1 and each must be found and corrected, not just the first one; per convention 29 no phase/task/decision numbers appear in user-facing strings

## Unknowns

Resolved by the decisions above: poll cadence (10), first-run flood (6), multi-project
grouping (11), Releases-arc mirroring (12). Still genuinely open:

- **Should `provider.NotificationSource` gain a non-error warnings channel?** Task 8 resolves
  the partial-failure question by having `Adapter.List` absorb a `*PartialError` and return
  `(rows, nil)` whenever it has rows, because the composite discards the rows of any backend
  that reports an error (`composite.go:669-672`) and propagating would blank the Azure half of
  the feed. The consequence is that a partial failure — one project's PAT expired, one project
  403s — is now **silent**: the user sees a shorter feed and nothing tells them why, because
  the only channel for "something went wrong" is the error return the adapter just swallowed.
  A second return value, or a `Warnings []error` field the pane could render as a subdued
  status line, would close it. **Deferred to Oscar deliberately:** it widens an interface
  phase 1 shipped and that GitHub also implements, so it is a cross-provider design decision,
  not a fix for this loop to make on its own.

- **Does any source need a PAT scope beyond what the app already requests?** The comments
  endpoint documents `vso.work`, which the work-item pane already needs, so the likely answer
  is no — but "likely" is what convention 28 exists to catch. Task 1 confirms it against a real
  org before it reaches the README.
- **Cost of the mention source specifically.** Sources 1, 3 and 4 are one query per project per
  cycle. Source 2 is one WIQL plus one comments call *per candidate work item*, so its cost
  scales with how much a user is mentioned. Decision 10's slower interval bounds the damage and
  task 5 bounds the fan-out, but the constant is a guess until someone runs it against a busy
  org. Revisit after the first real-world use.
- **Should resurrection key off a changed activity *token* rather than a newer timestamp?**
  Raised by task 4's review, 2026-08-06, and it applies to every source, not just PRs. Every
  stamp this design uses is attacker- or accident-settable and can move backwards: a PR's
  committer date (`GIT_COMMITTER_DATE`, `rebase --committer-date-is-author-date`, force-push to
  an older commit) and task 5's newest-matching-comment `createdDate` (deleting that comment
  regresses it permanently). `Reconcile`'s `After` test then reads real new activity as "older"
  and leaves a dismissed row stranded. Clamping future stamps to `now` — done in task 4 — closes
  the permanent-strand variant but not the regression one. The alternative is a `LastCommitID`
  or generic opaque-token field on `TriageEntry`, where *any* change means new activity
  regardless of clock direction; `CommitID` is already decoded and unused. **Deferred to Oscar
  deliberately:** it changes `TriageEntry`'s persisted shape and `Reconcile`'s contract across
  all four sources, which is a design decision, not a fix an implementer should make mid-loop.
  Decide before task 9 hardens the store's schema.
