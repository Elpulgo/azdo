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
| 13 | Config shape now that a second provider exists? | **Shared keys at top level, provider-specific keys nested** under `notifications.github` and `notifications.azure`. See the block below | Of phase 1's nine keys, only six are genuinely provider-neutral (**corrected 2026-08-07 from task 10 — this read "five", which contradicts the YAML block below and does not subtract to nine; the YAML is the contract and lists six**). `participating_only` has no Azure equivalent by phase 1's own account, and `only_configured_repos` is vacuously true on Azure (the adapter only ever queries configured projects) — both sat at top level looking shared when they never were. Nesting states the truth the flat shape obscured |
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
- [x] 8. **`azdevops`: compose sources concurrently and implement `NotificationSource`** (blocked by: 4,5,6,7). → done: compile-time `var _ provider.NotificationSource = (*Adapter)(nil)` plus a conformance test following `adapter_conformance_test.go`; sources run concurrently and **one failing source degrades to the others rather than emptying the feed** — the same rule phase 1's decision 20 enforces at the composite layer, restated here because a source is to the Azure adapter what a backend is to the composite, and phase 1 lost a defect to exactly this; `Adapter` does **not** implement `PollIntervalHinter` (decision 10), asserted by a negative compile-time check; local state is folded into `Notification.Read` **at this boundary**, per phase 1's unread-semantics constraint — nothing above the adapter may learn that Azure read state is local; **and the composite will discard your rows if you hand it an error** — `composite.go:669-672` does `if r.err != nil { errs = append(errs, r.err); continue }`, and `composite.go:637-641` documents that as deliberate (a backend reporting failure cannot vouch for the completeness *or ordering* of what it returned). So propagating task 4's `*PartialError` upward from `Adapter.List` re-blanks the Azure half of the feed one layer up, re-introducing the exact defect task 4 was reopened to fix. Decide it deliberately: either `Adapter.List` absorbs partial failures and returns `(rows, nil)`, or phase 1's composite rule is revisited — do not leave it to fall out of the code (added 2026-08-06 from task 4's re-validation). **Resolved 2026-08-07: `Adapter.List` absorbs, and the composite rule stands.** The composite's rationale is right *at its own boundary* — it cannot know whether a backend's partial result is sorted or complete — but the Azure adapter is the layer that merged and sorted these rows, so it can vouch for them, and revisiting the composite rule would reopen a phase-1 defect for every backend to fix a problem local to this one. Concretely: `Adapter.List` returns `(rows, nil)` whenever **at least one source succeeded**, and propagates the error only when **every** source failed. (**Corrected 2026-08-07, same day, from task 8's review:** this first read "at least one *row*", which is not the same test and gets a live case wrong — three sources fail, the fourth succeeds, and the user has already triaged its rows away, so `Reconcile` legitimately returns zero. Keying on the row count turns that into `errors.Join` → the composite's `len(errs) == total` → a permanent `errorBody` in the pane, every poll, from a partial outage in which one source demonstrably worked. Keying on the source count says what was actually meant: an empty feed is a feed, an outage is when nothing answered.) — so a total outage still surfaces as an error while a single expired project PAT degrades to a shorter feed. The cost is real and is accepted knowingly: a partial Azure failure becomes **invisible**, since the composite renders `"%d of %d backends failed to load"` from errors alone and the adapter has no non-error channel to report on. That is the lesser harm for an attention feed — missing rows is degradation the user can recover from, an empty pane is an outage that teaches them to stop trusting the tab — but it is a gap, and widening `NotificationSource` with a warnings channel is recorded under `## Unknowns` for Oscar rather than invented here
- [x] 9. **`azdevops`: `MarkRead`/`MarkDone` write to the local store** (blocked by: 2,8). *(Re-validated 2026-08-07 against `029317c`, which fixes all seven review findings; the original `→ done:` clauses were re-checked and still hold. See the re-validation record at the end of `## Validation: task 9`.)* → done: both take a `provider.Identity` and write through `Identity.ID` as the key; marking an id the store has never seen creates the entry rather than erroring — the composite routes by `Identity.Kind` and cannot know what the store has; `MarkDone` on an already-done id is a no-op, not a double-write; writes go through the debounced store and a `Flush()` on shutdown guarantees durability
- [x] 10. **`config`: restructure `NotificationsConfig` into shared + `github` + `azure`** (decisions 13, 14) (blocked by: 8). *(Re-validated 2026-08-07 against `6047aa3`, which fixes all six review findings; the original `→ done:` clauses were re-checked against the current code and still hold. See the re-validation record at the end of `## Validation: task 10`.)* → done: the block matches decision 13's YAML exactly; `participating_only`, `only_configured_repos` and `since_days` move under `notifications.github` and the **six** shared keys stay at top level (corrected 2026-08-07 — this said "five", propagated from decision 13's rationale; the YAML lists six and nine minus three is six); keys resolve lowercased at every nesting level (convention 9 — verify the nested maps too, not just the root, since that is the untested half); **no migration shim and no deprecation warning for the old flat keys** (decision 14) — a flat `notifications.participating_only` is simply an unrecognised key, and a test pins that it is *not* silently honoured, since a half-removed shim is worse than none; per convention 25 the documented key list is derived from the struct, not restated by hand
- [x] 11. **`config`: `notifications.azure` values and source toggles** (decisions 5, 6, 10) (blocked by: 10). *(Re-validated 2026-08-07 against `0eeb10a`, which fixes all seven review findings and implements both settled decisions, plus `6556b25`, which kills the two mutants that re-validation found surviving and corrects three stale comments; the original `→ done:` clauses were re-checked against the current code and still hold. See the two re-validation records at the end of `## Validation: task 11`.)* → done: four independent source toggles, all defaulting **on**; `lookback_days` defaults to 14 and `min_poll_interval` to 300, both rejecting negatives with the same message shape as the existing `since_days` check; **`lookback_days` is additionally clamped to `orphanTTL` (30 days)** — beyond that, an item can be pruned from the triage store while still inside the query window and resurface as unread with nothing having touched it, since `assignedQueryTop` means "inside the window" and "returned by the poll" are different sets (added 2026-08-06 from task 6's review; `orphanTTL`'s doc comment states the guarantee unconditionally and must be corrected to name the condition); **zero is not "unbounded" for `lookback_days`** — it falls back to the default, and a test pins that, because the shared-key convention that zero means widest is exactly what makes this key dangerous (decision 13's closing note); disabling every source is legal and yields an empty Azure feed, **not** a config error — and must not make the adapter claim incapability, since that would silently hide the tab in an Azure-only config
- [ ] 12. **`provider`: move `max_items` truncation from adapter to composite** (decision 15) (blocked by: 8). *(Un-ticked 2026-08-07 after review — the acceptance clauses below all hold against `7040c11`, but the review found a doc bound the code does not enforce, a dropped empty-inbox assertion, and settled Decision C below. Re-tick once `## Review feedback: task 12` is addressed.)* → done: `CompositeProvider.List` applies the cap after its merge-and-sort, so `max_items: 50` yields at most 50 rows with two live backends rather than up to 100; the per-backend truncation phase 1 put in `NotifOpts.Max` handling is removed, not left in place to double-apply; a test drives two capable backends each returning more than the cap and asserts the merged length **and** that the surviving rows are the globally newest — a length-only assertion passes against a naive truncate-before-sort; phase 1's existing single-backend `Max` tests must still pass unchanged
- [ ] 13. **`config`: widen the all-panes-disabled guard** (decision 9) (blocked by: 10). → done: `config.go:614`'s `&& c.HasGitHub()` becomes "any notification-capable backend configured"; the error message at `config.go:617` no longer says the tab "needs a GitHub backend"; the stale comment at `config.go:610-613` predicting this change is removed, not left contradicting the code; tests cover Azure-only, GitHub-only, and both, each with the other three panes disabled
- [ ] 14. **`azdevops`: adapter self-throttling, and wire `notifications.azure` into the adapter** (decision 10) (blocked by: 8,11). **Wiring added to this task 2026-08-07, from task 11's implementation.** Task 11 parses, defaults, clamps and validates `lookback_days`, `min_poll_interval` and the four `sources` toggles — and nothing reads them: `cmd/azdo-tui/main.go:330-331` still calls `NewAdapterWithNotifications(client, notifStore, 0, azdevops.DefaultNotificationSourceToggles())` with a hardcoded zero lookback and hardcoded defaults. No task owned that gap, so a user setting `lookback_days: 7` or `sources.mentioned: false` today would see the key accepted, validated, and then silently ignored — the worst of the three possible outcomes, since a rejected key at least tells you. Task 14 is the right home because it already has to plumb `min_poll_interval` from the same block through the same call. → done: `main.go` passes `cfg.Notifications.Azure.LookbackDays`, the `Sources` toggles and `MinPollInterval` through, with a test proving a non-default value reaches the adapter rather than only that it parses; **and** `Adapter.List` returns its previous result unchanged when called within `min_poll_interval` of its last real query, so the single shared poller cannot price the whole feed at Azure's cost; **nothing in `polling` or `app` changes** — no second poller, no second tick message, no new interval arithmetic (phase 1 decision 69 keeps `max(hint, configured)` in app.go untouched); the cached slice is returned **by copy** under a mutex, so a caller mutating it cannot corrupt the next throttled return — phase 1 lost a defect to exactly this in its conditional-request cache, and the test must prove it by mutating the first result and re-checking the second, since comparing two aliases of one backing array is a tautology; a throttled return must not be mistaken for a failure and must not clear the feed; `MarkRead`/`MarkDone` are **never** throttled and must not block behind a poll's network work — **clarified 2026-08-07 from task 9's review**, which observed that the line as written ("take no lock shared with `List`") is already violated: marks take `TriageStore.mu`, and `list` holds that same mutex across its whole `Swap`. Reviewed and accepted as correct — that critical section runs only in-memory `Reconcile`, with no I/O and no callback back into the store, and lock order (`writeMu` → `mu`) is consistent across all of `Apply`/`ApplyIfChanged`/`Swap`/`Flush`. The constraint that was actually meant is about the **throttle** lock this task introduces: a mark must never wait on an in-flight Azure query, so the cached-result mutex `List` holds across its HTTP work must not be the mutex a mark acquires. Sharing the store's in-memory mutex is fine and is what task 8 chose deliberately to close a lost-write window
- [ ] 15. **ADR `docs/adr/000N-azure-synthetic-notification-feed.md`** — decisions 2, 3, 6, 7 (blocked by: 8). → done: follows `docs/adr/0001`'s shape (≤30 lines, `Status: Accepted`, Context/Decision/Alternatives/Consequences); the Alternatives section records the stamp-in-key design and *why* it lost, since that is the decision most likely to be re-proposed by someone reading only the original candidate
- [ ] 16. **Docs: README, Architecture.md, config.yaml.example, FAQ** (blocked by: 13,14,15). → done: the full nested config block from decision 13 documented, derived from the struct per convention 25 — including which keys are shared and which are provider-specific, since that distinction is the whole point of the restructure; `exclude_repos`/`include_repos` documented as matching an `owner/repo` on GitHub and a **project name** on Azure (decision 13's second note); `sources.ci_failed` documented as a source toggle that emits the `ci_activity` reason, so the two spellings are not read as one vocabulary; the local-state file's path, purpose and "not synced across machines" caveat stated; any PAT scope beyond the current set named explicitly, or its absence confirmed (task 1 answers this); per convention 26, grep for every place the old GitHub-only notifications requirement is stated — README, FAQ, `Architecture.md`, `cmd/azdo-tui`'s help blocks and the auth wizard all asserted it in phase 1 and each must be found and corrected, not just the first one; per convention 29 no phase/task/decision numbers appear in user-facing strings

## Validation: task 9

Verified against the current state of the code (commit `ba88a24` plus the
task-8 lineage it builds on, not just the isolated diff):

- Both `MarkRead`/`MarkDone` take a `provider.Identity`, reject wrong `Kind`
  and empty `ID`, and write through `state[id.ID]` — confirmed by reading
  `internal/azdevops/adapter_notifications.go:363-430` and by mutating the
  key lookup to a fixed string, which the existing no-op test caught.
- Unseen ids create a fresh entry rather than erroring
  (`TestAdapter_MarkRead_UpdatesLocalTriageState`,
  `TestAdapter_MarkDone_UpdatesLocalTriageState`); mutated to require a
  pre-existing entry — both the create test and
  `TestAdapter_MarkRead_UnseenID_CreatesEntryWithSaneLastActivity` failed as
  expected.
- `MarkRead`/`MarkDone` on an already-read/-done id is a true no-op via the
  new `TriageStore.ApplyIfChanged` — no `LastActivity`/`LastSeen` churn, no
  re-dirtying. Mutated out the `if entry.Read {return false}` /
  `if entry.Done {return false}` guards independently; both killed by
  `TestAdapter_MarkRead_AlreadyRead_IsNoOp` /
  `TestAdapter_MarkDone_AlreadyDone_IsNoOp`.
- The empty-`Identity.ID` guard (not named in the `→ done:` line but
  required by the loop) is present on both methods and independently
  mutation-killed by `TestAdapter_Mark{Read,Done}_EmptyID_ReturnsErrorAndCreatesNoEntry`.
- Writes go through the debounced store: `MarkRead`/`MarkDone` call
  `TriageStore.ApplyIfChanged`, which shares `markDirtyLocked`/the debounce
  timer with `Apply`; `TriageStore.Flush()` is wired on every exit path in
  `cmd/azdo-tui/main.go:394-403` against the same `*TriageStore` instance
  `NewAdapterWithNotifications` was given (`main.go:290-330`).
- `TriageEntry` gained **no** new field — still exactly `{Read, Done,
  LastActivity, LastSeen}` (`internal/azdevops/notifications_store.go:24-37`).
  The `LastCommitID` question in `## Unknowns` remains open, untouched by
  this task, as the spec's own revised note requires.
- Reasoned independently through `Reconcile` (not the implementer's
  comment): a fresh entry created by `MarkRead`/`MarkDone` for an id
  `Reconcile` has never seen carries `LastActivity = time.Now()`, captured
  strictly after the `List` call that produced the row the UI is reacting
  to. On the next poll, that key is already present in the store's map, so
  `Reconcile` takes the "seen" path, not the "!seen" (fresh-discovery)
  path — and since the row's real `UpdatedAt` (assuming no genuine new
  activity in between) is `<=` the mark's `LastActivity`, the row lands in
  the "unchanged" or "older-stamp" branch, both of which leave the stored
  `Read`/`Done` untouched rather than clearing it. This is exactly what
  `TestAdapter_MarkRead_UnseenID_CreatesEntryWithSaneLastActivity` pins by
  feeding the freshly created entry straight back through `Reconcile` with a
  row stamped before the mark, and asserting `Read` survives — verified
  passing, and verified to fail when the no-op guard is removed.
- Doc comments checked against behaviour: no "always"/"never"/"guaranteed"
  claim found unpinned by a test — the "`LastActivity` written here is
  always >= the row's own `UpdatedAt`" claim is the one load-bearing
  instance and is exactly what the invariant test above pins.
  **Superseded 2026-08-07 by the review below: that claim is false and the
  test does not pin it.** Both this validation and the implementer reasoned
  about the row that *triggered* the mark; `Reconcile` compares against the
  row as re-fetched on the **next** poll, and the forward clamp can advance
  that row's stamp with nothing having happened to the subject. See
  `## Review feedback: task 9`, finding 1.

Mutation testing: 6 targeted mutants (already-read no-op, already-done
no-op, unseen-id creation gated on pre-existing entry, empty-id guard on
`MarkRead`, empty-id guard on `MarkDone`, key-lookup swapped to a wrong
constant) — all 6 killed by the existing test suite. All files restored via
`cp` from pre-mutation backups, confirmed byte-identical (`diff` clean, `git
status` clean) before finishing.

Full suite (`CGO_ENABLED=0 go test -count=1 ./internal/... ./cmd/...`)
passes. gofmt offenders in `internal/azdevops/` are the pre-existing ones
only (`adapter_list_test.go`, `adapter_url_test.go`, `logs_test.go`,
`mapping_test.go`, `timeline_test.go`, `workitems.go`) — neither file this
task touched is on that list.

### Re-validation 2026-08-07 — `029317c` (task re-ticked)

Re-checked the **current state of the code** across `ba88a24` + `029317c` +
the task-8 lineage, not the fix commit's diff in isolation.

**A. Original `→ done:` clauses still hold.** `Reconcile`'s rewrite, the
deletion of `TriageStore.Replace` and `Apply`'s collapse to a wrapper
regressed none of them: both marks still take a `provider.Identity` and key
on `Identity.ID`; create-on-mark, the already-done no-op and the debounced
write path are all still mutation-killed; `Flush()` is still wired on every
exit path (`cmd/azdo-tui/main.go:395-405`) against the same `*TriageStore`
`NewAdapterWithNotifications` was handed (`main.go:320-331`).

**B. Findings, each mutation-tested rather than read.**

1. 🔴 **Fixed.** Every claim in the replacement text at
   `adapter_notifications.go:354-370` was checked against the code: all four
   `*ActivityStamp` helpers do clamp with `if stamp.After(now) { return now }`
   (verified by reading each body), and the `>=` claim is now scoped to "the
   row the mark was issued from". The forward-clamp case is named as an open
   question the code does **not** handle and points at `## Unknowns`.
2. 🟡 **Fixed.** Re-ran the reviewer's exact two mutants. Deleting
   `if entry.LastActivity.IsZero() {...}` from `MarkDone` now fails three
   tests (`..._UpdatesLocalTriageState`, `..._AlreadyDone_IsNoOp`,
   `..._UnseenID_CreatesEntryWithSaneLastActivity`); deleting the `LastSeen`
   equivalent fails two. `TestAdapter_MarkDone_AlreadyDone_IsNoOp` is no
   longer a tautology — it fails at its new `first.LastActivity.IsZero()`
   fatal under the first mutant.
3. 🟡 **Fixed.** The doc now forbids writing when returning `false`, and
   compliance is pinned non-accidentally: mutating each mark to write
   `entry.LastSeen` before its early return kills
   `TestAdapter_Mark{Read,Done}_Already{Read,Done}_IsNoOp` respectively.
4. 🟡 **Fixed.** No production caller of `Replace` remains (repo-wide grep;
   build + vet clean). Locking is a single `mu` acquisition —
   `Apply` → `ApplyIfChanged` → `markDirtyLocked` — and mutating the
   wrapper's `return true` to `false` kills eight tests.
5. 🟡 **Fixed, and the drop is safe.** All four sources set
   `Identity.ID = NotifKey(...)`, which is `""` only for `id <= 0`, so no
   legitimate row can be discarded. The `continue` sits *before*
   `newState[key]` is touched, so real entries' `LastSeen` bookkeeping is
   untouched: mutants removing `entry.LastSeen = now` from the unchanged-stamp
   and older-stamp branches both still die, as does flipping the orphan-TTL
   prune's `>` to `>=` (boundary row). Reverting the drop to the old
   passthrough fails the renamed table row. Grep confirms no doc comment
   still describes the passthrough except as history.
6. 🟢 / 7. 🟢 **Fixed** — the backfill-scope comment now describes the actual
   `LastActivity.IsZero()` trigger, and the debounce is `time.Hour`.

**Additional `Replace` references:** the three the implementer names
(`State()`'s doc, `Flush()`'s doc, `notifications_reconcile_test.go:252-256`)
are accurate. A **fourth** it did not name survives at
`notifications_store_test.go:692-696`, which still calls the rejected
alternative "a naive `State() -> compute -> Replace()` call chain" as though
the method existed; its production-side twin (`notifications_store.go:236-240`)
was rewritten. Cosmetic, in a test doc comment about a hypothetical — not
blocking.

**Comment hunt (the package's recurring defect class).** Re-read every comment
`029317c` changed against the code beneath it. Two residual 🟢 imprecisions,
neither an invariant the code fails to establish: `MarkRead`'s "own
`time.Now()` is **always** later" holds only absent a backwards wall-clock
step (`Reconcile` compares wall clocks, since `row.UpdatedAt` is parsed and
carries no monotonic reading) — the same stamp-direction family already
recorded under `## Unknowns`; and `ApplyIfChanged`'s "until either this store
is dropped … or an unrelated, later call to `Apply`/`ApplyIfChanged`" omits
`Swap` as a third path that marks the store dirty.

Mutation testing: 9 mutants, all killed — `MarkDone` `LastActivity` backfill,
`MarkDone` `LastSeen` backfill, `Reconcile`'s empty-key drop reverted to
passthrough, `LastSeen` in the unchanged-stamp branch, `LastSeen` in the
older-stamp branch, orphan-TTL `>` → `>=`, `MarkRead`
writes-then-returns-false, `MarkDone` writes-then-returns-false, and `Apply`'s
`return true` → `false`. Every file restored by `cp` from a scratchpad backup
(never `git checkout --`); `git status --porcelain` empty and the package green
afterwards.

Full suite (`CGO_ENABLED=0 go test -count=1 ./internal/... ./cmd/...`) passes;
`go build` and `go vet` clean. `gofmt -l internal/azdevops/` lists only the six
pre-existing offenders — none of them a file this task touched.

## Review feedback: task 9

Opus review of `ba88a24` plus its task-8 lineage, 2026-08-07. Verdict
REQUEST_CHANGES. Task 9 un-ticked.

**1. 🔴 The load-bearing "always >=" comment is false.**
`adapter_notifications.go:344-355` claims the `LastActivity` a mark writes is
always `>=` the row's own `UpdatedAt`, "which is exactly the invariant
`Reconcile`'s strictly-newer branch needs". The premise holds for the row that
*triggered* the mark. `Reconcile` compares against the row as re-fetched on the
**next** poll, and the clamp in all four `*ActivityStamp` helpers is
`if stamp.After(now) { return now }` — it caps at *each poll's own* `now`. A
subject whose raw stamp is ahead of the client clock therefore yields a clamped
stamp that advances by one poll interval every poll, with nothing having
happened to the subject, and `Reconcile` reads that as new activity and clears
the mark. Reproduced end-to-end against the real helpers. Fix the comment to
claim only what holds (`>=` the `UpdatedAt` of the row the mark was issued
from); the behaviour itself is decision A below.

**2. 🟡 `MarkDone`'s `LastActivity`/`LastSeen` backfills are unpinned — two
mutants survive the whole suite.** Deleting either
`if entry.LastActivity.IsZero()` or `if entry.LastSeen.IsZero()` from
`MarkDone` (`adapter_notifications.go:420-425`) leaves the suite green; the
same deletions in `MarkRead` are both killed. The `LastActivity` one is the
live defect `Reconcile`'s own task-9 invariant paragraph
(`notifications_reconcile.go:97-102`) warns about: a zero `LastActivity` loses
to any real `UpdatedAt`, `Done` is cleared, and the row the user dismissed is
back one poll later. `TestAdapter_MarkDone_UpdatesLocalTriageState` asserts only
`entry.Done` where its `MarkRead` twin asserts the stamps, and
`TestAdapter_MarkDone_AlreadyDone_IsNoOp` compares two zero times, which is a
tautology against this mutant.

**3. 🟡 `ApplyIfChanged`'s doc sanctions an in-memory/on-disk divergence.**
`notifications_store.go:194-196` says `mutate` "may still write into the live
map" while returning `false`, and calls the result "not persisted". `mutate`
receives `s.state` itself, so such a write is immediately visible to `State()`
and to `Swap`, and is either lost on exit or written later by an unrelated
`Apply`. What `false` skips is *scheduling*, not persistence. Tighten the
contract to "must not write when returning `false`" and pin it — it is
currently unpinned in both directions. `MarkRead`/`MarkDone` are safe only
because they return before touching the map, and nothing enforces that.

**4. 🟡 Four mutation APIs, two with no production callers.** Every
`TriageStore.Apply` and `TriageStore.Replace` call site is now in
`notifications_store_test.go` — marks moved to `ApplyIfChanged`, `Replace` lost
its caller to `Swap` in task 8. Locking is correct in all four; the problem is
surface area, and `Apply(f)` is exactly
`ApplyIfChanged(func(s) bool { f(s); return true })`. Collapse or delete before
task 14 has to choose between them.

**5. 🟡 An empty-key row is rendered but permanently un-markable.** `NotifKey`
returns `""` for `id <= 0`; all four sources emit the row anyway and
`Reconcile` (`notifications_reconcile.go:112-121`) deliberately passes it
through as unread-and-untracked. The pane renders it, the user presses `u` or
`d`, and the mark returns `azdevops: mark read: empty identity id` — unread
forever, an error banner on every attempt. The guard is right at its own
boundary; the gap is that no layer drops the row. Drop it at `Reconcile` (the
one choke point all four sources funnel through) and correct the three doc
comments that each describe this as handled.

**6. 🟢** `MarkRead`'s doc scopes the `LastActivity` backfill to "an id the
store has never seen"; the code applies it to any entry with a zero
`LastActivity`, including one `Reconcile` created from a row that had no usable
stamp. Behaviourally benign, but the comment says something narrower than the
code does.

**7. 🟢** `TestTriageStore_ApplyIfChanged_TrueDirtiesAndPersists` arms a 10 ms
debounce and then reads `store.dirty`; a scheduler stall between the two
statements fails the test spuriously. It calls `Flush()` explicitly anyway, so
the short debounce buys nothing — use `time.Hour`, as `newTestTriageStore`
already does for this reason.

**Design decision A — deferred to Oscar. The forward clamp is a resurrection
engine, and it is not a task-9 defect.** Finding 1's *behaviour* cannot be
fixed inside `MarkRead`. The four stamp helpers clamp a future raw stamp to the
current poll's `now` (tasks 4-7) and `Reconcile` reads any advance as new
activity (task 3); marks are merely where it becomes visible. It bites without
any mark at all — `Reconcile`'s `!seen` branch stores the clamped value too, so
such a row resurfaces as unread every poll regardless. Severity scales with how
far ahead the raw stamp is: a few seconds of NTP skew costs **one** spurious
resurrection and then settles, because the next poll's `now` has overtaken the
raw stamp; a genuinely far-future date (`GIT_COMMITTER_DATE`, a
badly-configured build agent) churns every poll until wall-clock catches up.
The clamp cannot simply be removed — `prActivityStamp`'s doc explains that an
unclamped future stamp "would raise the bar past anything a real push could
ever clear, permanently freezing the row". So the choice is between
permanent-freeze and permanent-churn, and the code picked churn silently.
Options: (1) have the stamp helpers report *that* they clamped and have
`Reconcile` treat a clamped stamp like a zero stamp — no usable activity
information, stored state applies unchanged; cheap, local, **no change to
`TriageEntry`'s persisted shape**, so it does not collide with the deferred
`LastCommitID` decision; (2) store the raw stamp and compare raw-to-raw, which
changes what `LastActivity` means; (3) the opaque-token design already in
`## Unknowns`, which solves this *and* the mirror regression case but changes
the persisted shape and all four sources. Option 1 looks right, but this is the
same family as the deferred unknown and the loop should not pick.

**Design decision B — resolved, task 14's wording clarified.** The reviewer
noted that task 14's "`MarkRead`/`MarkDone` … take no lock shared with `List`"
is already violated as written: marks take `TriageStore.mu`, which `list`
holds across its whole `Swap`. Reviewed and found **not** a defect — that
critical section runs only in-memory `Reconcile`, with no I/O and no callback
into the store, and lock order (`writeMu` → `mu`) is consistent everywhere.
Task 14's line is clarified below to say what was meant.

## Validation: task 10

Verified against `c013808` (subject: "Restructure NotificationsConfig into
shared + github + azure blocks"), the current tip of the branch at
`d022ade`.

- **Shape matches decision 13's YAML exactly.** `NotificationsConfig` carries
  exactly the six shared `mapstructure` tags (`exclude_reasons`,
  `unread_only`, `exclude_repos`, `include_repos`, `max_items`,
  `poll_interval`) plus the two nested blocks; `NotificationsGitHubConfig`
  carries exactly `participating_only`, `only_configured_repos`,
  `since_days`; `NotificationsAzureConfig`/`NotificationsAzureSourcesConfig`
  carry exactly `lookback_days`, `min_poll_interval`,
  `sources.{review_requested,mentioned,assigned,ci_failed}` — confirmed by
  `awk`-extracting each struct body and diffing the tag list against the
  spec's YAML block field-by-field.
- **Every caller of the three moved fields updated.** Repo-wide grep for
  `.ParticipatingOnly`/`.OnlyConfiguredRepos`/`.SinceDays` outside
  `_test.go` turns up only `internal/ui/notifications/filter.go` (now
  reading `nc.GitHub.*`), `internal/config/config.go`'s own warning/
  validation lines (now `cfg.Notifications.GitHub.*`), and
  `internal/github/adapter.go:588`'s `opts.ParticipatingOnly`, which reads
  `provider.NotifOpts.ParticipatingOnly` — confirmed a distinct type at
  `internal/provider/list_opts.go:53-59`, not the moved config field. No
  caller reads the old flat path anywhere in the tree.
- **Convention 9 at every nesting level, non-vacuously.** Three dedicated
  tests exercise mixed-case keys at the root
  (`TestLoad_NotificationsBlock_MixedCaseKeys_Convention9`), one level deep
  under `notifications.github`
  (`TestLoad_NotificationsGitHubBlock_MixedCaseKeys_Convention9`), and two
  levels deep under `notifications.azure.sources`
  (`TestLoad_NotificationsAzureSourcesBlock_MixedCaseKeys_Convention9`).
  Mutated `NotificationsAzureSourcesConfig.ReviewRequested`'s
  `mapstructure` tag to a value with no matching YAML key — the deepest
  test failed as expected (`Azure.Sources.ReviewRequested = false, want
  true`), proving the resolution is genuinely exercised by
  `viper`/`mapstructure`'s built-in case-insensitive field matching (no
  hand-rolled lowercasing code exists — `v.Unmarshal(&cfg)` is the only
  resolution path), not a test that would pass regardless.
- **Decision 14 (no shim) genuinely honoured, including in `Save`.**
  `TestLoad_FlatMovedGitHubKeys_AreNotHonoured` sets all three flat keys and
  asserts they land nowhere and produce no warning. Mutation-tested by
  reintroducing a shim in `LoadFrom` (`if v.IsSet("notifications.
  participating_only") { cfg.Notifications.GitHub.ParticipatingOnly =
  v.GetBool(...) }`) — the test failed as expected
  (`GitHub.ParticipatingOnly = true, want false`), then restored via `cp`
  from a scratchpad backup. `Config.Save()` was independently confirmed to
  set no `notifications.*` keys at all (it round-trips whatever the file
  already contains via `v.ReadInConfig()` + `v.WriteConfig()`), so there is
  no shim anywhere on the write path either — this task's diff does not
  touch `Save()`.
- **Scope respected.** `NotificationsAzureConfig`/`Sources` carry zero Go
  values with no `v.SetDefault("notifications.azure...")` registrations
  (confirmed by grep — only `notifications.github.*` and the six shared
  keys are registered) and no `c.Notifications.Azure.*` reads anywhere in
  `Validate()` (confirmed by grep — zero hits) — task 11's defaults and
  validation are untouched. `config.go:680`'s
  `notificationsCounts := c.IsPaneEnabled("notifications") &&
  c.HasGitHub()` guard is unchanged — task 13's widening is untouched.
  `config.yaml.example` was not modified by `c013808` — task 16's docs are
  untouched. No doc-comment claims a struct populated by a later task is
  already populated; each says "task 11's"/"populated by task 11"
  accurately.
- **Doc comments checked against behaviour.** Re-read every comment
  `c013808` added or changed: the `NotificationsConfig`,
  `NotificationsGitHubConfig`, `NotificationsAzureConfig`,
  `NotificationsAzureSourcesConfig` doc comments and the `LoadFrom`
  `SetDefault` block comment all describe exactly what the code does — no
  "always"/"never"/unconditional claim found that the code doesn't
  establish.

Full suite (`CGO_ENABLED=0 go build ./...`, `go vet ./...`,
`CGO_ENABLED=0 go test -count=1 ./internal/... ./cmd/...`) passes.
`gofmt -l` on every file this task touched (`internal/config/config.go`,
`internal/config/config_notifications_test.go`,
`internal/config/config_save_test.go`, `internal/ui/notifications/filter.go`,
`internal/ui/notifications/filter_test.go`,
`internal/ui/notifications/list_test.go`) is empty. Both mutants restored
via `cp` from `/tmp/claude-1001/.../scratchpad/backups/config.go.bak`
(never `git checkout --`); `git status --porcelain` clean and
`git diff --stat HEAD` empty before finishing.

### Re-validation 2026-08-07 — `6047aa3` (task re-ticked)

Re-checked the **current state of the code**, not the fix commit's diff in
isolation.

**A. Original `→ done:` clauses still hold.** The struct still matches decision
13's YAML exactly (tags re-extracted per struct body: six shared, three under
`github`, `lookback_days`/`min_poll_interval`/`sources.{review_requested,
mentioned,assigned,ci_failed}` under `azure`). The three moved fields resolve
only at their nested paths — repo-wide grep for `.ParticipatingOnly`/
`.OnlyConfiguredRepos`/`.SinceDays` outside `_test.go` finds only
`nc.GitHub.*` reads in `filter.go`, `cfg.Notifications.GitHub.*` in
`config.go`'s warning/validation, and `internal/github/adapter.go:588`'s
unrelated `provider.NotifOpts.ParticipatingOnly`; `SetDefault` registers only
the six shared keys plus `notifications.github.*`. Convention 9 holds at all
three depths and is non-vacuous: mutating
`NotificationsGitHubConfig.ParticipatingOnly`'s `mapstructure` tag killed
`TestLoad_NotificationsGitHubBlock_MixedCaseKeys_Convention9` (plus two
others), and the azure-level test asserts mixed-case `Lookback_Days`/
`MIN_POLL_INTERVAL` as well as the three-deep `sources` keys. Decision 14 still
holds end-to-end — a probe loading a config with flat
`notifications.participating_only`/`since_days` produced
`GitHub={false,false,0}` with **zero** warnings.

**B. Findings, each checked against the code.**

1. 🟡 **Fixed and pinned in both directions.** The predicate is
   `if row.Identity.Kind != provider.KindGitHub { pass through }` — stated as
   "this knob only has an opinion about GitHub rows", not as an Azure carve-out,
   so a third backend is unaffected without editing this branch. Both mutants
   re-run: (a) deleting the pass-through fails
   `TestFilterNotifications_OnlyConfiguredRepos_NonGitHubRowsPassThrough`
   (`got 1 rows [gh-configured], want 2`); (b) passing everything through
   unconditionally fails that same test on its unconfigured-GitHub row plus
   three pre-existing `OnlyConfiguredRepos` tests. The new fixture genuinely
   mixes kinds (configured GitHub + unconfigured GitHub + Azure).
   **Same-defect sweep on the other two `github` keys: clean, and for a
   different reason than the prompt assumed.** `participating_only` and
   `since_days` never reach `FilterNotifications` at all — it reads neither
   (verified) — they are fetch-time `provider.NotifOpts` fields, and
   `azdevops.Adapter.List` documents and implements ignoring both
   (`adapter_notifications.go:99-107`), narrowing server-side per source
   instead. So neither can drop an Azure row. (`opts.Max` *is* honoured
   per-backend by both adapters, which is the known, still-open task 12, not a
   new finding.) Every notification row from both backends sets
   `Identity.Kind` explicitly (all four Azure sources, `mapping_notifications.go`),
   and `Kind`'s zero value is neither constant, so no live row can fall through
   the guard by accident.
2. 🟢 **Fixed.** `NotificationsAzureConfig`'s doc now says the three declared
   fields are task 11's landing site and that what task 10 omitted is their
   defaults and validation — which matches the code (no
   `SetDefault("notifications.azure...")`, no `Azure` read in `Validate()`).
3. 🟢 **Fixed and mutation-killed.** The assertion is now the qualified
   `notifications.github.only_configured_repos`; rewriting the warning string to
   the bare `only_configured_repos` spelling fails
   `TestLoad_OnlyConfiguredRepos_WithIncludeRepos_Warns`.
4. 🟢 **Fixed and accurate.** `Validate()` really does check exactly
   `GitHub.SinceDays`, `MaxItems`, `PollInterval`; the Azure block's two numeric
   fields really are unvalidated, and the comment now names the future
   `lookback_days: 14` default as what invalidates its own "always valid"
   premise.
5. 🟢 **Fixed, verified against `app.go` itself.**
   `notificationsPollInterval` (app.go:390-400) returns `hint` when
   `hint > configured`, unconditionally — matching "max(configured, hint)" and
   "the hint is not overridden just because this field is set".
   `notificationsConfiguredInterval` (app.go:378-388) is exactly
   `poll_interval -> polling_interval -> polling.DefaultInterval`, first
   positive wins. The `polling.MinInterval` floor is in
   `NewNotificationsPoller` (`notifications_poller.go:53-57`) and `SetInterval`
   (`:84-87`), as claimed. The hint's GitHub source really is `X-Poll-Interval`
   (`github/notifications.go:381-391`).
6. 🟢 **Fixed, and the behaviour is real.** Probed directly: after
   `LoadFrom` + `Save()`, the orphaned flat `notifications.participating_only`/
   `since_days` are still in the file verbatim. Note the cited pin,
   `TestConfigSave_PreservesKeysOutsideTheConfigStruct`, exercises a *top-level*
   unmodeled section rather than an orphaned key inside the modeled
   `notifications` map — the general rule, not this instance. Cosmetic.

**Comment hunt (this loop's recurring defect class).** Re-read every comment
`6047aa3` touched against the code beneath it; the five above all check out.
Two residual 🟢 imprecisions, neither an invariant the code fails to establish:
`filter.go:19-20`'s "include_repos is ignored entirely **for those rows**"
under-states it — the `switch` skips the include_repos case for *every* row, so
a non-GitHub row is not subject to include_repos either (the next sentence does
say such a row always survives the branch); and the `Azure` *field* comment in
`NotificationsConfig` still reads "populated in full by task 11. This struct
exists here only to give that task somewhere to land its fields", which is
finding 2's wording one line away from where it was fixed, though its closing
clause ("task 10 invents neither their defaults nor their validation") says the
accurate thing.

Mutation testing: 4 mutants, all killed — Kind pass-through deleted,
`only_configured_repos` branch made unconditional, warning message
de-qualified, `participating_only`'s `mapstructure` tag broken. Every file
restored by `cp` from a scratchpad backup (never `git checkout --`), md5-checked
against the pre-mutation copy; the temporary probe test was deleted and
`git status --porcelain` is empty.

Full suite (`CGO_ENABLED=0 go test -count=1 ./internal/... ./cmd/...`) passes;
`go build ./...` and `go vet ./...` clean. `gofmt -l` on all four files
`6047aa3` touched is empty.

## Review feedback: task 10

Opus review of `c013808`, 2026-08-07. Verdict REQUEST_CHANGES. Task 10
un-ticked. Nothing here is a 🔴, but finding 1 falsifies a premise decision 13
relies on.

**A correction to the record first.** This restructure was reviewed as "a
breaking config change to a shipped release". It is not: `git tag --contains
0f90acf` is empty and the newest tag `v0.7.2` predates phase 1's merge by a
month. Decision 14's own rationale ("merged to `main` but has not been
released") is factually correct, and the silent-downgrade path is therefore
low-stakes — the maintainer's dev config plus `config.yaml.example`, which task
16 owns.

**1. 🟡 `only_configured_repos` is documented as GitHub-only and deletes the
entire Azure feed.** `FilterNotifications` runs on the **merged** feed
(`internal/app/app.go:1152`), and its `nc.GitHub.OnlyConfiguredRepos` branch
(`internal/ui/notifications/filter.go:63`) keeps only rows whose
`Identity.Scope` appears in `cfg.GitHub.Repos`. Azure rows carry
`Scope = pr.ProjectName`, never an `owner/repo` string, so the knob drops 100%
of Azure notifications. Reviewer confirmed it with a throwaway probe: one
GitHub row and one Azure row in, one GitHub row out. **Decision 13's rationale
states this key "is vacuously true on Azure (the adapter only ever queries
configured projects)" — that premise is what justified nesting it under
`github`, and the code falsifies it.** The predicate is not vacuous for Azure;
it is always false.

**Resolved 2026-08-07: make the code match decision 13's stated intent.** The
`OnlyConfiguredRepos` branch passes through any row whose `Identity.Kind` is
not GitHub, which is what "vacuously true on Azure" means when written as
code. This is not a design decision to defer — the spec already says what the
behaviour should be and the code disagrees with it; the alternative (rewrite
both doc comments to say the `github` block governs the merged feed) would
have task 10 document a defect instead of fixing it. Pin it with a test that
puts a GitHub row and an Azure row through the filter with
`only_configured_repos: true` and asserts **both** survive — a
GitHub-rows-only test passes either way. The doc comments at `config.go:92-93`
and `config.go:143-144` must also stop claiming the `github` block is
merged-feed-neutral, since that is the claim that hid this.

**2. 🟢 `NotificationsAzureConfig`'s doc contradicts the struct it sits on.**
`config.go:166-168` says "Left unpopulated by task 10 on purpose", with three
fields declared on the next four lines. What task 10 left out is the *defaults
and validation*, not the fields. Task 11's implementer reading this cannot tell
whether the existing fields are authoritative or a stub to replace.

**3. 🟢 The warning-message test cannot tell the new key path from the old.**
`config_notifications_test.go:947` asserts `Contains(msg,
"only_configured_repos")`, which the old flat spelling satisfies too. Assert
the qualified `notifications.github.only_configured_repos` — the qualification
is the point of the change.

**4. 🟢 `Validate()`'s comment counts three numeric fields where the tree now
has five.** `config.go:686-691`. `notifications.azure.lookback_days: -30` loads
clean today with no validation at all. More importantly the comment's premise
("an absent block is all-zero and therefore valid") stops holding the moment
task 11 gives `lookback_days` a default of 14 — and this comment is what task
11's implementer will read.

**5. 🟢 `PollInterval`'s fallback description doesn't match the code.**
`config.go:128-131` says the backend hint applies only when the key is zero;
`app.go:369-395` computes `max(configured, hint)` whether or not it is set,
with a third fallback (`polling.DefaultInterval`) the comment never mentions
and a `polling.MinInterval` floor. A user setting `poll_interval: 30` gets 60
against GitHub's hint. Mostly inherited from phase 1, but this commit touched
the line.

**6. 🟢** `config.go:97-99` says an orphaned flat key "is silently ignored" —
true for the read path, but `Save()` also writes it back verbatim, so it
survives in the file forever. Correct under decision 14 (a warning is
forbidden), but a maintainer reading only this comment would expect the key to
disappear on the next save.

## Validation: task 11

Verified against `ca6b3c7` ("config: notifications.azure defaults, source
toggles, and lookback clamp"), current tip of the branch at this validation.

- **Four independent source toggles, all defaulting on.** `LoadFrom`
  registers `v.SetDefault("notifications.azure.sources.{review_requested,
  mentioned,assigned,ci_failed}", true)`. This is not vacuous: mutating one
  registration to `false` fails `TestLoad_NotificationsDefaults_WhenBlockAbsent`
  and `TestConfigSave_PreservesMetricsAndNotifications` (confirmed by mutation,
  restored via `cp`). A dedicated
  `TestLoad_AzureSourceToggle_ExplicitFalse_OverridesDefaultTrue` sets exactly
  one toggle (`review_requested: false`) and leaves the other three unset,
  proving an explicit `false` is genuinely honoured and the unset three still
  default on — a fixture that only checked "all true, nothing set" would pass
  against a hardcoded-true implementation. Confirmed independently that
  viper's `Unmarshal` does materialize an explicit `false`/`0` over the
  registered default (neutralizing the zero-fallback branch below made the
  explicit-zero test fail), so the mixed-case test at
  `config_notifications_test.go` setting `MENTIONED: false`/`Ci_Failed: false`
  is a real assertion, not a coincidental pass against the true default.
- **`lookback_days` defaults to 14, `min_poll_interval` defaults to 300, both
  reject negatives with the same message shape as `since_days`.**
  `Validate()` returns `"notifications.azure.lookback_days must be >= 0, got
  %d"` / `"notifications.azure.min_poll_interval must be >= 0, got %d"`,
  matching `"notifications.github.since_days must be >= 0, got %d"`'s shape
  exactly. `TestConfig_Validate_NotificationsRejectsNegative` covers both via
  the same table-test pattern as the existing `since_days` row (a direct
  `Validate()` mutation test, not end-to-end through `LoadFrom` — the same
  pattern the pre-existing `since_days` negative row already uses, so this is
  parity, not a gap specific to this task).
- **`lookback_days` clamped to `orphanTTL`/`azureLookbackDaysMax` (30).**
  `config.go` clamps `> azureLookbackDaysMax` down to it after the
  zero-fallback. The three-row boundary test (29/30/31) genuinely pins the
  exact threshold, verified independently: mutating the comparison to `> 25`
  (a "one day early" threshold) leaves the 30 and 31 rows green — both land on
  the same clamp target (30) regardless of which threshold fired — and is
  caught only by the 29 row (`got 30, want 29`). Restored via `cp`, confirmed
  byte-identical (`diff` clean) before continuing.
- **Zero is not "unbounded" for `lookback_days`.** An explicit
  `lookback_days: 0` falls back to `DefaultAzureLookbackDays` via a dedicated
  `if cfg.Notifications.Azure.LookbackDays == 0 { ... }` branch in `LoadFrom`,
  pinned by `TestLoad_AzureLookbackDays_Zero_FallsBackToDefault`. Verified
  this branch is load-bearing, not redundant with `v.SetDefault`: neutralizing
  it (`if false { ... }`) fails the test (`LookbackDays = 0, want 14`) because
  viper's `Unmarshal` materializes an explicit `0` from the file over the
  registered default. Restored via `cp`, confirmed byte-identical.
- **`orphanTTL`'s doc comment corrected to name the condition.**
  `internal/azdevops/notifications_reconcile.go`'s comment now reads "It is
  generous relative to the default lookback only while
  notifications.azure.lookback_days stays <= 30: beyond that, ... different
  sets", replacing the prior unconditional guarantee, and cross-references
  `internal/config.azureLookbackDaysMax`. `config.go`'s `azureLookbackDaysMax`
  comment points back at `azdevops.orphanTTL` by name. Both comments read
  accurately against the code; each says "if this value changes, change the
  other," which is the best two files that cannot import each other can do —
  correctly flagged by the implementer as a duplication that cannot be
  structurally prevented from drifting, only documented against.
- **`Validate()`'s absent-block comment updated and now true.** The comment
  above the numeric checks now correctly states that `Azure.LookbackDays`/
  `Azure.MinPollInterval` default non-zero via `v.SetDefault` (unlike the
  three Go-zero-value fields), and that an absent/all-default block remains
  valid only because those defaults themselves satisfy `>= 0` — matching
  task 10 review finding 4's request.
- **Disabling every source is legal, not a config error, and does not hide
  the tab.** `TestLoad_AzureSources_AllDisabled_IsLegal` pins `LoadFrom`
  returning no error with all four toggles false.
  `NotificationsConfig.Azure`'s doc comment states this is a Go interface
  fact about `*azdevops.Adapter`, not config-driven; grepped the whole tree
  for `Sources.{ReviewRequested,Mentioned,Assigned,CIFailed}` and
  `Notifications.Azure` usage outside `_test.go` — the only production
  reader is `internal/azdevops/adapter_notifications.go` (task 8/14's
  per-source dispatch), and nothing in `internal/config` or any capability
  check reads the toggles to decide whether Azure implements
  `provider.NotificationSource`.
- **Scope respected.** `config.go:790`'s
  `c.IsPaneEnabled("notifications") && c.HasGitHub()` guard is unchanged
  (task 13 untouched). `cmd/azdo-tui/main.go:330-331` still calls
  `NewAdapterWithNotifications(client, notifStore, 0,
  azdevops.DefaultNotificationSourceToggles())` with a hardcoded zero
  lookback and default toggles — confirmed this is task 14's wiring gap, not
  a task 11 failure, per the task list's own note. `ca6b3c7` touches no
  README/Architecture.md/config.yaml.example/FAQ file (task 16 untouched).
- **Doc comments checked against behaviour.** Re-read every comment
  `ca6b3c7` added or changed in `config.go` and
  `notifications_reconcile.go` against the code beneath it — no
  "always"/"never"/unconditional claim found unpinned; the one previously
  unconditional claim (`orphanTTL`'s guarantee) is the one this task was
  required to fix, and it now names its condition correctly.

Mutation testing: 3 targeted mutants (SetDefault flipped to `false` on one
source toggle, clamp threshold shifted to `> 25`, zero-fallback branch
neutralized) — all 3 killed by the existing test suite, each verified
independently in this validation pass rather than taken on the implementer's
word. All files restored via `cp` from pre-mutation backups (never `git
checkout --`), confirmed byte-identical (`diff` clean) and `git status
--porcelain` empty before finishing.

Full suite (`CGO_ENABLED=0 go build ./...`, `go vet ./...`,
`CGO_ENABLED=0 go test -count=1 ./internal/... ./cmd/...`) passes.
`gofmt -l` on every file this task touched (`internal/config/config.go`,
`internal/config/config_notifications_test.go`,
`internal/config/config_save_test.go`,
`internal/azdevops/notifications_reconcile.go`) is empty.

### Re-validation 2026-08-07 — `0eeb10a` (INCOMPLETE, task stays un-ticked)

Re-checked the **current state of the code**, not the fix commit's diff in
isolation. Build, vet and the full suite
(`CGO_ENABLED=0 go test -count=1 ./internal/... ./cmd/...`) are green;
`gofmt -l` on all six files `0eeb10a` touched is empty.

**A. Original `→ done:` clauses still hold.** Four toggles still default on
(`v.SetDefault("notifications.azure.sources.*", true)`, mutation-killed:
flipping `mentioned` to `false` fails three tests); `lookback_days` defaults
to 14 and `min_poll_interval` to 300; both reject negatives with
`"notifications.azure.<key> must be >= 0, got %d"`, the same shape as
`notifications.github.since_days`; the 30-day clamp still holds with its
29/30/31 boundary rows; `lookback_days: 0` still falls back to 14
(mutation-killed); all four toggles false still loads clean
(`TestLoad_AzureSources_AllDisabled_IsLegal`) and cannot affect capability —
`hasNotificationCapability` is a type assertion plus
`CompositeProvider.HasNotifications`, which asserts `b.(NotificationSource)`
and reads no toggle, and `Adapter.list` returns `(rows, nil)` at
`jobCount == 0` rather than treating "no sources ran" as an outage.

**B. Every finding and both decisions genuinely fixed — each verified, not read.**

1. 🟡 **Fixed.** `config.go:203-222` now states the real post-condition set:
   "a value that reached LoadFrom's body is negative (a config error
   Validate() rejects below) or in [1, 30]" — i.e. `{n < 0} ∪ [1, 30]` — and
   names the un-normalized case explicitly ("a *Config not built via LoadFrom
   -- NewWithPath's setup-wizard constructor, or any bare struct literal").
   Checked against the code: the `== 0` fallback and the `> 30` clamp are the
   only two normalizations, and `Validate()`'s guard is `< 0`.
2. 🟡 **Fixed, verified against the four source files rather than the
   comment.** `SourceAssigned` and `SourceCIFailed` are the only two whose
   signatures take `lookbackDays`; `SourceReviewRequested(mc, userID, top,
   now)` and `SourceMentioned(mc, userID, now)` do not. Confirmed at the
   query layer too: `ListRecentlyMentionedWorkItems`' WIQL
   (`workitems.go:316-318`) is `[System.Id] IN (@RecentMentions)` with no
   date clause, while `ListRecentlyAssignedWorkItems` (`workitems.go:387-393`)
   and `ListMyFailedPipelineRuns` (`pipelines.go:55-59`) both consume it.
3. 🟡 **Fixed, and the drift detector genuinely detects.**
   (a) `NewAdapterWithNotifications` clamps independently
   (`adapter_notifications.go:86-88`); deleting that clamp fails
   `TestNewAdapterWithNotifications_LookbackDays_ClampedToMax`'s two
   above-max rows. `orphanTTL`'s comment no longer claims the gap can only
   be closed by config. (b) `MaxNotificationLookbackDays` is **derived**,
   `int(orphanTTL / (24 * time.Hour))`, not restated. Shrinking `orphanTTL`
   to `7 * 24 * time.Hour` fails `TestAzureLookbackDaysMax_ConfigAndAdapterAgree`
   with `config.AzureLookbackDaysMax = 30, azdevops.MaxNotificationLookbackDays = 7`;
   the default sibling fires too (`DefaultNotificationLookbackDays` 14 → 15
   fails `TestDefaultAzureLookbackDays_ConfigAndAdapterAgree`). No import
   cycle: `go list -deps ./internal/config` contains no `internal/azdevops`,
   and `internal/config`'s import list is `errors fmt internal/provider viper
   go-keyring os path path/filepath strings`.
4. 🟡 **Fixed — the named mutant is dead.** Changing `config.go:704`'s
   `== 0` to `<= 0` fails
   `TestLoad_AzureLookbackDaysNegative_RejectedByLoadFrom`
   ("LoadFrom() = nil error, want an error naming
   notifications.azure.lookback_days").
5. 🟢 **Fixed.** All four mixed-case source rows now assert an explicit
   `false` against a `true` default, so none can pass by coincidence.
6. 🟢 **Fixed, and the judgement call is correct.** The comment documents the
   post-change state: both numeric `SetDefault`s are redundant given their
   `== 0` fallbacks, while the four `sources.*` ones are load-bearing.
   Verified by mutation both ways — deleting **both** numeric registrations
   leaves the entire tree green (the comment's claim exactly), while flipping
   one `sources.*` registration fails three tests.
7. 🟢 **Fixed and accurate.** The disabled-source triage-history note matches
   `Reconcile`'s `now.Sub(v.LastSeen) > orphanTTL` prune on entries the
   current rows do not carry.
- **Decision A — done and pinned in both directions.** The clamp appends to
  `Config.Warnings` in the established shape
  (`"notifications.azure.lookback_days: 90 exceeds the 30-day maximum — using 30"`);
  deleting the append fails `TestLoad_AzureLookbackDays_ClampWarns`, and
  *adding* an append to the `0 → 14` branch fails
  `TestLoad_AzureLookbackDays_ZeroFallback_NoWarning`. It surfaces exactly
  like the other three: `app.go:1799` passes `m.config.Warnings` to
  `notificationsTabContent` → `notificationsWarningsBanner`, which joins the
  slice verbatim — no per-message filtering anywhere on that path.
- **Decision B — done and pinned.** `min_poll_interval: 0` falls back to 300
  (`config.go:737-739`); deleting the branch fails
  `TestLoad_AzureMinPollInterval_Zero_FallsBackToDefault`.

**C. What is still missing — two live surviving mutants, both one-line test
additions.** Neither is a behavioural defect: the code is correct today.
Both are the same class this loop keeps re-finding — a normalization whose
negative half nothing pins, next to a name or comment that claims it is
covered.

1. **`min_poll_interval`'s `== 0` → `<= 0` survives the whole tree.** This is
   the exact twin of finding 4, created by decision B *in this same commit*.
   Verified: `config.go:737`'s `if cfg.Notifications.Azure.MinPollInterval == 0`
   changed to `<= 0` leaves `./internal/... ./cmd/...` fully green — a user's
   `min_poll_interval: -1` would then silently become 300 instead of erroring.
   Finding 4's own argument carries verbatim ("`since_days` has no `LoadFrom`
   branch that can swallow a negative before `Validate()` sees it"), and
   `min_poll_interval` now has one. The risk is concrete rather than
   theoretical because the code comment at `config.go:729-736` explicitly
   frames this branch as "Symmetric with lookback_days above", inviting a
   future maintainer to edit both together — at which point one is caught and
   one is not. `TestConfig_Validate_NotificationsRejectsNegative`'s
   `min_poll_interval` row cannot substitute: it calls `Validate()` on a
   struct literal and never runs `LoadFrom`'s normalization, which is the
   whole reason finding 4 demanded a `LoadFrom`-level row.
   **Fix:** add a `TestLoad_AzureMinPollIntervalNegative_RejectedByLoadFrom`
   alongside the `lookback_days` one — a temp YAML with
   `min_poll_interval: -1`, asserting `LoadFrom` errors and the message names
   `notifications.azure.min_poll_interval`.

2. **`NewAdapterWithNotifications`' `<= 0` floor → `== 0` survives the whole
   tree, and the new test's row name claims otherwise.**
   `adapter_notifications.go:83`'s `if lookbackDays <= 0` changed to `== 0`
   leaves `./internal/... ./cmd/...` green, so a negative `lookbackDays`
   reaches `notifLookbackDays` un-floored (the WIQL builders then clamp it to
   `@Today-0`, the degenerate window the floor exists to prevent). The gap is
   pre-existing, but `0eeb10a` added the test that now covers this
   constructor and gave its only sub-30 row the name **"non-positive falls
   back to the default"** while supplying `lookbackDays: 0` — the row asserts
   less than its name claims, which is finding 5's shape one file over.
   Convention 11 also asks for this directly: "Test a negative input row
   alongside the zero row."
   **Fix:** add `{name: "negative falls back to the default", lookbackDays:
   -1, want: DefaultNotificationLookbackDays}` to
   `TestNewAdapterWithNotifications_LookbackDays_ClampedToMax`'s table, and
   either rename the existing row to "zero falls back to the default" or
   leave it once the negative row makes the "non-positive" claim true.

**Comment hunt (the recurring class), residual 🟢 only — none blocking.**
Re-read every comment `0eeb10a` changed against the code beneath it. Three
imprecisions, none an invariant the code fails to establish: `orphanTTL`'s
"If this constant's value ever changes, **both** `MaxNotificationLookbackDays`
and `internal/config.AzureLookbackDaysMax` must change with it" asks a
maintainer to hand-edit a constant that is derived and changes by itself, one
line above the comment that says so; the same comment's "that gap cannot open
through any caller of this package" is true for the constructor path but
`SourceAssigned`, `SourceCIFailed` and `Reconcile` are all exported, so a
caller composing them directly could still open it; and `Config.Warnings`'
own field comment (`config.go:42-44`) plus `notificationsWarningsBanner`'s
(`app.go:1660`) both still enumerate the warning sources as "unrecognised
exclude_reasons and malformed repo globs" — already two short before this
commit, three short now that the clamp warns. Separately,
`config_notifications_test.go:653`'s "All three rows use -1 on purpose" sits
above a five-row table (inherited from `ca6b3c7`, untouched here).

Mutation testing: 11 mutants — 9 killed (`lookback_days` `== 0` → `<= 0`;
`orphanTTL` 30d → 7d; `DefaultNotificationLookbackDays` 14 → 15; the
constructor's max clamp deleted; the clamp's `Warnings` append deleted;
`min_poll_interval`'s zero fallback deleted; a `Warnings` append added to the
`0 → 14` branch; `sources.mentioned`'s `SetDefault` flipped to false; and
both numeric `SetDefault`s deleted, which correctly changed nothing, as its
comment claims) and **2 survived** (section C). Every file restored by `cp`
from `scratchpad/valbak/` (never `git checkout --`), md5-verified identical
to the pre-mutation copy; `git status --porcelain` empty and the tree green
afterwards.

### Re-validation 2026-08-07 — `6556b25` (task re-ticked)

Narrow re-check of the delta only; `0eeb10a`'s record above still stands
unchanged. `6556b25` touches no production behaviour — its `config.go` and
`app.go` hunks are comment-only, and the two code-adjacent files are a new
test and a table row.

**Both surviving mutants are now dead, verified by applying each mutation
against the whole tree rather than by reading the report.**

1. `config.go`'s `if cfg.Notifications.Azure.MinPollInterval == 0` changed to
   `<= 0` now fails `TestLoad_AzureMinPollIntervalNegative_RejectedByLoadFrom`
   ("LoadFrom() = nil error, want an error naming
   notifications.azure.min_poll_interval"). The new test drives the full
   `LoadFrom` path from a `t.TempDir()` YAML fixture carrying
   `min_poll_interval: -1`, mirroring its `lookback_days` twin, and asserts
   the error names the qualified key.
2. `adapter_notifications.go:83`'s `if lookbackDays <= 0` changed to `== 0`
   now fails
   `TestNewAdapterWithNotifications_LookbackDays_ClampedToMax/negative_falls_back_to_the_default`
   ("notifLookbackDays = -1, want 14"). Both row names now match what they
   supply ("zero …" / "negative …"), satisfying convention 11's
   negative-row-alongside-the-zero-row requirement.

**The three rewritten comments read accurately against their code.**

- `Config.Warnings`' field comment enumerates exactly four diagnostics, and
  the file has exactly four `Warnings = append(...)` sites: the
  `exclude_reasons` sanitizer (`config.go:674`), `sanitizeRepoGlobs`
  (`config.go:335`, called for both `exclude_repos` and `include_repos` —
  one diagnostic class, which is how the comment words it),
  `only_configured_repos` overriding a non-empty `include_repos`
  (`config.go:694`) and the `lookback_days` clamp (`config.go:728`).
  Enumerated list and append sites match one-for-one.
- `notificationsWarningsBanner`'s comment (`app.go:1658-1668`) lists the same
  four and explicitly defers to `Config.Warnings`' comment as "the
  authoritative list", so the next warning added has one place to drift from
  rather than two.
- `orphanTTL`'s rewrite no longer asks for a hand-edit of the derived
  constant — it now states that `MaxNotificationLookbackDays` "updates
  itself … derived from orphanTTL, not restated" while
  `internal/config.AzureLookbackDaysMax` "does not and must be changed by
  hand", which is exactly the code. Its replacement claim about the exported
  bypass paths was checked against all three signatures rather than taken on
  trust: `SourceAssigned(mc, lookbackDays, now)` and `SourceCIFailed(mc,
  userID, lookbackDays, top, now)` both pass `lookbackDays` straight to the
  client layer with no upper bound anywhere (`ListRecentlyAssignedWorkItems`
  and `ListMyFailedPipelineRuns` clamp only *negatives* to 0, never a
  maximum), and `Reconcile(rows, stored, now)` takes no `lookbackDays` at
  all. So all three are genuinely reachable without the constructor's clamp.
  The list is illustrative rather than exhaustive — `MultiClient`/`Client`'s
  own exported `List*` methods are unbounded too — but it errs toward
  claiming the gap is *open*, which is the safe direction for this class of
  comment.

**Nothing outside scope moved.** `cmd/azdo-tui/main.go` still calls
`NewAdapterWithNotifications(client, notifStore, 0,
azdevops.DefaultNotificationSourceToggles())` with a hardcoded lookback and
default toggles (task 14's wiring gap, untouched); no `mapstructure` key was
added or renamed; `## Unknowns` and every other spec section are untouched by
`6556b25` (it contains no `.spec` file).

Full suite (`CGO_ENABLED=0 go test -count=1 ./internal/... ./cmd/...`) passes;
`go build ./...` and `go vet ./...` clean; `gofmt -l` on all five files
`6556b25` touched is empty. Both mutants restored by `cp` from
`scratchpad/valbak2/`, md5-verified byte-identical to the pre-mutation copy;
`git status --porcelain` empty afterwards.

## Review feedback: task 11

Opus review of `ca6b3c7`, 2026-08-07. Verdict REQUEST_CHANGES. Task 11
un-ticked. No 🔴 — the mechanics are right and were verified independently.

**Two things the review confirmed, recorded because they are subtle.** The
clamp genuinely *closes* the resurfaces-as-unread gap at `lookback_days <= 30`
rather than merely narrowing it: pruning needs `now - LastSeen > 30d`, and for
an unchanged subject `LastSeen >=` the field the server-side window filters on,
so by the time the TTL fires the subject is already outside any window `<= 30`.
And the `>` vs `>=` mutant at the clamp really is **equivalent** — `if x > 30 {
x = 30 }` and `if x >= 30 { x = 30 }` agree on every input because the
assignment target equals the threshold. The implementer's judgement was right;
that is not a coverage gap.

**1. 🟡 `LookbackDays`' doc states a post-condition the code does not
establish** (`config.go:211-212`). It says the field is "always either 0 (a
config error caught below) or in [1, 30]". `0` is impossible at `Validate()`
time — the fallback at `:653` already replaced it — and if it did arrive it
would be *accepted*, since the guard at `:819` is `< 0`. The set the code
establishes is `{n < 0} ∪ [1, 30]`, and negatives are exactly what the sentence
omits. Failure: task 14's implementer reads this as the contract for the value
it plumbs in, concludes non-positive cannot reach the adapter, and drops
`NewAdapterWithNotifications`'s `if lookbackDays <= 0` floor — but a `Config`
from `NewWithPath` (the setup wizard) or any struct literal has `LookbackDays
== 0`, passes `Validate()`, and reaches Azure with `@Today-0`.

**2. 🟡 `LookbackDays` is documented as bounding all four sources; it bounds
two** (`config.go:203`). Only `SourceAssigned` and `SourceCIFailed` take a
`lookbackDays`. `SourceReviewRequested` is "open PRs where I am a reviewer",
unbounded by time, and `SourceMentioned`'s stage-1 WIQL is `@RecentMentions`
with no date clause at all. `azdevops`' own `DefaultNotificationLookbackDays`
gets this right, so the two packages now contradict each other — and task 16
derives its documented key list from the struct per convention 25, so the wrong
claim would ship to the README.

**3. 🟡 The duplicated 30-day bound has no drift detector.** The commit message
says the cross-referencing comments mean the constants "cannot drift silently".
They can — comments are documentation, not a mechanism; nothing in the build,
the type system or the suite relates `config.azureLookbackDaysMax` to
`azdevops.orphanTTL`. Failure: someone shrinks `orphanTTL` to 7 days to bound
the state file, updates nothing in `internal/config`, and `lookback_days: 30`
still loads clean — reintroducing precisely the defect the clamp was added to
prevent, with the suite green. **Fix both ways, they are complementary:**
(a) apply the clamp inside `NewAdapterWithNotifications` too, where `orphanTTL`
is actually in scope and where the window is decided — this also fixes
`orphanTTL`'s new comment, which claims the gap "cannot open through ordinary
config loading" while any direct caller passing 90 opens it; and (b) export the
bound from `azdevops` and assert equality against `config`'s in a one-line test
in a package that already imports both (`cmd/azdo-tui` or `internal/app`), so
no new dependency is created for `internal/config`. The same pattern applies to
`DefaultAzureLookbackDays` ↔ `azdevops.DefaultNotificationLookbackDays`.

**4. 🟡 A live surviving mutant: nothing pins that a negative `lookback_days`
reaches `Validate()`.** `TestConfig_Validate_NotificationsRejectsNegative`
calls `Validate()` on a struct literal, which never sees `LoadFrom`'s
normalization, and no YAML fixture anywhere writes a negative. **Verified:
changing `== 0` to `<= 0` at `config.go:653` leaves the whole `internal/config`
suite green** — the user's `lookback_days: -1` silently becomes 14 and they
never learn the key was malformed. The `since_days` parity argument does not
carry: `since_days` has no `LoadFrom` branch that can swallow a negative before
`Validate()` sees it, and `lookback_days` now does. Add a `LoadFrom`-level row
writing `lookback_days: -1` to a temp config and asserting the error.

**5. 🟢** Two of the four mixed-case source assertions
(`config_notifications_test.go:314,320`) set `true` where the default is now
`true`, so they would pass even if viper stopped lowercasing keys — while their
failure messages claim to pin case resolution. The two `false` rows do
discriminate; only the claim is wrong.

**6. 🟢** `v.SetDefault("notifications.azure.lookback_days", …)`
(`config.go:535`) is fully redundant given the `== 0` fallback — deleting it
changes nothing observable and no test fails. Harmless and good symmetry, but
the block comment presents both mechanisms as load-bearing, so the next person
mutation-testing this file burns time on a survivor that is by design.

**7. 🟢** Disabling a source for >30 days silently discards its triage history:
`Reconcile` prunes on `LastSeen`, which only advances for rows a source
actually returns. Toggle `sources.mentioned: false` for a month and back on and
every mention you had read or dismissed returns as unread. Defensible, but
undocumented, and it belongs next to the toggles where the user makes that
choice.

**Decision A — settled 2026-08-07: the clamp warns.** `lookback_days: 90`
currently becomes 30 with no feedback. `Config.Warnings` exists for exactly this
class and is already used three times in the same function for notifications
config, and it renders as a banner *in the notifications tab itself* — the one
screen where a lookback surprise is noticed. The review read the silence as
having fallen out of the code rather than chosen, and the evidence supports
that: the comment at `config.go:662-664` weighs exactly two options, silent
normalization versus a validation error, and never considers the third sitting
three lines above it. Decision 14 forbids a *deprecation* warning for removed
keys, which is a different thing. So: **warn on the clamp** (shape:
`notifications.azure.lookback_days: 90 exceeds the 30-day maximum — using 30`),
and **stay silent on `0 → 14`**, because zero reads as "unset" rather than as
an expressed intent. This is settled here rather than deferred because it
applies a pattern this file already established three times over; it invents
nothing.

**Decision B — settled 2026-08-07: `min_poll_interval: 0` falls back to 300.**
`lookback_days: 0` falls back to its default while `min_poll_interval: 0` is
currently left as a literal 0, with the semantics deferred to task 14. Under
decision 10 a literal 0 means *no self-throttle* — 4+ queries per project on
every 30-second tick, which is exactly the cost decision 10 exists to bound.
That is the same footgun `lookback_days: 0` was given a fallback for, and
decision 13's closing note already states the principle: the shared-key
convention that zero means widest is what makes these keys dangerous. Applying
the spec's own stated rule to the sibling key is not a new design decision.
Both keys therefore treat 0 as unset. If a genuine "no throttle" escape hatch
is ever wanted it should be an explicit sentinel, not a value a typo produces.

## Validation: task 12

Verified against `7040c11` by reading the code (not the implementer's
commit message) and mutation-testing every load-bearing claim.

- **Cap applied after merge-and-sort, at the composite only.** Read
  `internal/provider/composite.go`'s `mergeNotifications`: the
  `sort.SliceStable` over `UpdatedAt`/`Kind`/`Scope`/`ID` runs before the
  `if maxItems > 0 && len(all) > maxItems { all = all[:maxItems:maxItems] }`
  truncation. Mutated the file (via a `cp`-saved backup, restored the same
  way afterward) to swap the two blocks so truncation ran *before* the sort.
  Three tests went red, including the new
  `TestCompositeProvider_Notifications_TwoBackendsEachExceedCap_KeepsGlobalNewest`
  and the pre-existing `TestCompositeProvider_Notifications_MaxTruncatesToNewest`
  / `..._MaxAppliedOnPartialPath` — so the required test is genuinely
  order-sensitive, not length-only. File restored via `cp`, `git status`
  clean afterward.
- **Per-backend truncation removed, not left in place.** Grepped the whole
  tree for every non-test `.Max`/`opts.Max` reference
  (`internal/provider/composite.go`, `internal/provider/list_opts.go`,
  `internal/github/adapter.go`, `internal/github/notifications.go`,
  `internal/azdevops/adapter_notifications.go`) — every hit is either the
  composite's own application of the cap or a doc comment stating that the
  backend ignores it. Confirmed there are only two production
  `NotificationSource` implementations in the repo
  (`internal/github/adapter.go`, `internal/azdevops/adapter_notifications.go`)
  and mutation-tested both directly: reintroduced a truncation block in
  `github.Adapter.List` (killed by
  `TestAdapter_List_MaxDoesNotTruncateAtThisLayer/Max_smaller_than_result`)
  and in `azdevops.Adapter.list` (killed by the same-named test's identical
  subtest) — both restored via `cp`, `git status` clean afterward.
- **Required test present and correctly shaped.**
  `TestCompositeProvider_Notifications_TwoBackendsEachExceedCap_KeepsGlobalNewest`
  drives two capable backends each returning 5 rows (Max: 4) in
  non-canonical (non-newest-first) arrival order and asserts both `len(got)
  == 4` and the exact surviving IDs span both backends and are the true
  global newest (`gh-i9,az-i8,az-i7,az-i6`) — a naive
  truncate-per-backend-then-merge bug is shown in the test's own doc comment
  to pass a length-only assertion while failing this one (confirmed above by
  mutation).
- **Convention 13 boundary rows.**
  `TestCompositeProvider_Notifications_MaxBoundary_ExactlyCapAcrossTwoBackends`
  (merged total across two backends lands exactly on Max) and
  `TestCompositeProvider_Notifications_MaxBoundary_SingleBackendSuppliesExactlyCap`
  (one backend alone supplies exactly Max rows, the other's rows are pushed
  out entirely) are both present and new to this commit.
- **Convention 11 negative row alongside zero.**
  `TestCompositeProvider_Notifications_MaxNegativeIsUncapped` (new, two
  backends, `Max: -1`) sits alongside the pre-existing single-backend
  `TestCompositeProvider_Notifications_MaxZeroIsUncapped` (`Max: 0`) —
  confirmed both present and unmodified/added respectively.
- **Partial failure + cap.** The pre-existing
  `TestCompositeProvider_Notifications_MaxAppliedOnPartialPath` already
  covers this exactly: one backend returns `broken.listErr`, the healthy
  backend returns 3 rows against `Max: 2`, and the test asserts both
  `errors.As(err, &pe)` (a `*PartialError` still surfaces) and that the cap
  held (`"new,mid"`, `cap(got) == len(got)`). This test was not touched by
  `7040c11` (confirmed by the diff below) and still passes.
- **Phase 1's existing single-backend `Max` tests unchanged.**
  `git diff 0f90acf..HEAD -- internal/provider/composite_notifications_test.go`
  shows only additions (four new `Test...` functions appended after the
  pre-existing `TestCompositeProvider_Notifications_MaxAppliedOnPartialPath`)
  — no edits to any pre-existing test body, confirmed by reading the diff
  directly rather than trusting the commit message.
- **Per-adapter replacement tests are load-bearing, not decorative.**
  `TestAdapter_List_MaxDoesNotTruncateAtThisLayer` in both
  `internal/github/adapter_notifications_test.go` and
  `internal/azdevops/adapter_notifications_test.go` were each confirmed to
  fail when truncation is reintroduced in their respective adapter (see
  above) — not tautologies.
- **Deleted `TestAdapter_List_LargerMaxLaterServedFromCache_IsNotStuckAtSmallerMax`
  (github) drops no non-Max coverage.** Its two components — multi-page
  `Link rel="next"` walk assembly, and 304-conditional cache reuse across
  sequential calls (including copy-safety, mutating the first result before
  the second call) — are both independently and thoroughly covered at the
  `NotificationsClient` level, unaffected by this deletion:
  `TestNotificationsClient_List_FollowsLinkHeader_MultipleRels` (multi-page
  assembly), `TestNotificationsClient_List_304_ReturnsCachedSliceUnchanged`
  (304 cache reuse + copy safety), `TestNotificationsClient_List_304OnSecondPage_FailsWholeList`
  (multi-page + 304 interaction), and
  `TestNotificationsClient_List_DifferentRequestShape_NeverSendsMismatchedValidator`
  (cache scoped to request shape) — all pre-dating this commit. The deleted
  test's own unique angle (a *smaller* `Max` poisoning the cache validator
  for a *later, larger* `Max` call) is moot under the new contract: `Adapter.List`
  no longer forwards `opts.Max` into `NotificationListOpts`/`buildPath` at
  all (confirmed by reading `internal/github/adapter.go:597-600`), so two
  calls with different `Max` values are now, by construction, the same
  request shape — there is no adapter-level scenario left for that test to
  guard.
- **Doc comments checked against code.** `internal/github/adapter.go`'s
  claim that `List`'s returned slice is bounded by `maxNotificationPages ×
  notifPerPageCap` was checked against `NotificationsClient.List`
  (`internal/github/notifications.go:251-352`): the page-walk loop enforces
  `pages > maxNotificationPages` as cycle protection, `buildPath` sets
  `per_page=notifPerPageCap` on every request, and `Adapter.List` maps the
  wire slice 1:1 with no additional filter or truncation — the bound holds
  as an upper bound. `internal/provider/list_opts.go`,
  `internal/provider/composite.go`, `internal/github/notifications.go` and
  `internal/azdevops/adapter_notifications.go`'s rewritten comments were all
  read against their code and match.
- **Out-of-scope check.** `cmd/azdo-tui/main.go:330-331` still passes a
  hardcoded `0` and `azdevops.DefaultNotificationSourceToggles()` (task 14's
  territory) — untouched by this commit, confirmed by reading the current
  file.

Full suite (`CGO_ENABLED=0 go build ./... && go vet ./...` then
`CGO_ENABLED=0 go test -count=1 ./internal/... ./cmd/...`) passes clean.
`gofmt -l internal/provider internal/github internal/azdevops` lists only
the six pre-existing offenders (`adapter_list_test.go`, `adapter_url_test.go`,
`logs_test.go`, `mapping_test.go`, `timeline_test.go`, `workitems.go`, all in
`internal/azdevops/`) — none of the files this task touched. All mutations
were applied to `cp`-saved backups and restored via `cp` (never `git
checkout --`), confirmed byte-identical and `git status` clean before
finishing.

## Review feedback: task 12

Reviewed at opus against `7040c11`. Verdict REQUEST_CHANGES, but narrowly: the
substance of the task is right. `mergeNotifications` sorts on the four-key
total order and *then* applies `all[:maxItems:maxItems]`; both adapters
genuinely stopped truncating; the new composite test is order-sensitive rather
than length-only; both adapters carry a `TestAdapter_List_MaxDoesNotTruncateAtThisLayer`
that fails when truncation is put back. Two findings and one design question.

**1. 🟡 `internal/github/adapter.go:578-579` states a bound the code does not
enforce.** The new sentence says the returned slice is bounded by
`maxNotificationPages × notifPerPageCap` (5000). But `notifPerPageCap` is set
only on the **first** request — `buildPath` (`notifications.go:380`) writes
`per_page=100`, and pages 2..N are fetched via `c.getPage(next, "")` where
`next` is `nextPageURL(header)`'s verbatim parse of the server's
`Link rel="next"` (`client.go:172-193`), with no per-page rewrite. So the
constant constrains exactly one of up to 50 pages. A GHES instance or caching
proxy that ignores `per_page` and returns 1000 rows per page yields 50,000
rows crossing the adapter→composite boundary — and since `opts.Max` no longer
applies there, this comment is now the *only* stated bound on that boundary.
What makes it worth fixing rather than shrugging at is the adjacency: the
constant being cited has a doc comment three lines above
(`notifications.go:21-30`) explaining that it exists *because* a server
produced "501 requests and 500 rows with err == nil". The new comment asserts
a bound premised on the well-behaved server, sitting next to the constant that
exists for the misbehaving one. Restate it as a conditional bound, not an
arithmetic one. No code change.

**2. 🟢 `internal/github/adapter_notifications_test.go` — a dropped assertion
the validation record does not mention.** The deleted `TestAdapter_List_MaxBoundaries`
carried a row `{name: "Max against an empty result", body: "[]", max: 1,
wantLen: 0}` and an assertion that the result is empty-but-non-nil. Neither
survives into the replacement test, and nothing else in `internal/github` pins
it — while the *paired* assertion for the error path is still there
(`adapter_notifications_test.go:343`, "want nil alongside the error"). Half of
a deliberately-paired contract is now unpinned: rewriting
`out := make([]provider.Notification, len(wire))` as `var out []...` + append
would return `nil, nil` on an empty inbox with no test failing. Blast radius
today is nil — `mergeNotifications` accumulates into `var all []Notification`
so a nil reaches the app on that path regardless, and `app.go:1140-1152`
documents nil-items/nil-err as "genuinely EMPTY" — which is why this is 🟢.
Restore the row rather than letting the contract go by attrition.

**Decision C — `NotifOpts.Max`'s "backends must ignore this" is prose-only,
and the prose contradicts itself. Settled 2026-08-07: zero `Max` on the
fan-out.** `list_opts.go:66-76` now ends "Backend implementations must
therefore ignore this field", but `*CompositeProvider` is itself a
`NotificationSource` (`composite.go:60`) and is the one implementation that
*does* honour `Max`. Nesting a composite inside a composite reproduces exactly
the double-apply this task removed. Not reachable today — `main.go:361` builds
one composite from concrete adapters — and enforcement is two hand-written
per-adapter tests, so a third backend gets nothing unless its author knows the
rule exists.

The review offered three options: (a) leave it, (b) `CompositeProvider.List`
passes `Max: 0` to each backend and keeps the real value for its own cap, (c) a
shared conformance helper backends' tests opt into. **(b)**, for the same
reason task 10's finding 1 went the way it did: decision 15 already states that
the cap lives at the composite, and (b) is what makes the code say that instead
of asking every future backend author to have read a comment. It is one line,
it converts prose into a machine-checked invariant — a backend that *does*
honour `Max` becomes harmless rather than wrong — and it makes the nested-composite
case correct by construction rather than by nobody having tried it. This is not
a new design decision; it is the existing one, enforced. The cost is that
`opts` is no longer forwarded verbatim, so the wording in `list_opts.go`,
`composite.go` and both adapters has to change with it. (c) is not chosen
because it catches the mistake only for an author who wires the helper up,
which is the same author who would have read the comment.

**Priced and found not to be problems** (recorded so they are not re-litigated):

- **The 5000-row slice crossing the boundary costs almost nothing.** Before,
  the adapter already allocated and mapped all 5000 before truncating; after,
  the composite's `append` copies 5000 and `all[:50:50]` keeps that array
  alive — but `FilterNotifications` (`filter.go:59`) unconditionally copies
  into a fresh slice on both the poller path (`app.go:1152`) and the pane's
  own (`list.go:369`), so it is released one message-pass later, not held for
  the poll interval. Net: one extra ~700 KB copy per poll, promptly collected,
  plus a `sort.SliceStable` over 5000 rows instead of 50. Nothing downstream is
  sized by the pre-cap slice; on the Azure side phase 1's truncation already
  ran *after* `Swap`/`Reconcile`, so the triage store's input never changed.
- **Ties and cursor stability are strictly improved by this change.** The sort
  is a total order over `(UpdatedAt desc, Kind asc, Scope asc, ID asc)`,
  entirely data-derived, so a repoll of unchanged rows produces byte-identical
  order regardless of goroutine completion order, and a tie group straddling
  the cap resolves deterministically. The pane restores the cursor by
  `Identity.SameItem`, not by index (`list.go:1231-1244`). The old per-adapter
  truncation sliced GitHub's raw wire order, which the composite had no control
  over — removing it helps.
- **Pre-existing, unchanged by this commit:** an inbox exceeding 50 pages makes
  `nc.List` return an error rather than a truncated feed, so the pane fails
  permanently for such a user. `opts.Max` never bounded that walk. Not this
  task's to fix, but it is why finding 1 matters more than a doc nit usually
  would.

## Unknowns

Resolved by the decisions above: poll cadence (10), first-run flood (6), multi-project
grouping (11), Releases-arc mirroring (12). Still genuinely open:

- **What should happen when an activity stamp is forward-clamped?** Raised by task 9's
  review, 2026-08-07, and it is the mirror of the regression question below — same root,
  opposite direction, and it applies to all four sources. Full analysis and the three
  candidate fixes are under `## Review feedback: task 9`, decision A. **Deferred to Oscar
  deliberately:** the cheapest fix still changes `Reconcile`'s contract and all four stamp
  helpers' signatures, and it should be decided together with the token question below
  rather than separately, since option 3 answers both at once.

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
  ~~Decide before task 9 hardens the store's schema.~~ **Revised 2026-08-07: the deadline was
  wrong and task 9 does not need it.** `TriageState` is a `map[string]TriageEntry` serialised by
  the store's YAML round-trip, so a later `LastCommitID string \`yaml:"last_commit_id,omitempty"\``
  is purely additive: an existing file simply loads that field as `""`, which is the same value a
  fresh entry would carry. Task 9 therefore adds **no** field to `TriageEntry` and leaves this
  open. What *does* change if Oscar adopts the token is `Reconcile`'s contract and all four
  sources' population of it — none of which task 9 touches. Decide before task 14, or after the
  loop; either is fine.
