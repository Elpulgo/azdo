# Notifications Pane — Phase 2: Azure DevOps

**Ticket:** N/A
**Branch:** feat/notifications-azdo (worktree not yet created)
**Author:** Oscar Larsson
**Created:** 2026-07-29
**Status:** Deferred — do not start until phase 1 (`20260729-notif-p1-github.md`) has merged

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
  3. Newly assigned work items (delta since last snapshot) → `assigned`
  4. My failed pipeline runs → `ci_failed`
- Stable synthetic identity key + local read/done store, reusing `internal/state` patterns
- Snapshot/delta detection so only new or changed subjects surface as unread
- Auto-expiry rules (e.g. review completed → item auto-done)
- Config: per-source toggles under `notifications.azure`
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
| 2 | Identity key shape? | TBD during planning — candidate `kind:scope:entity_id:activity_stamp` | Load-bearing for the whole phase; must not be settled casually |
| 3 | Which @mention query? | TBD — WIQL over `System.History` vs. the mentions REST surface | Depends on what returns a usable timestamp per mention |
| 4 | Auto-expiry? | Yes, per-source rules | A completed review that stays in the feed trains the user to ignore the pane |
| 5 | Per-source config toggles? | Yes, under `notifications.azure` | Orgs differ wildly in which sources are signal vs. noise |

## Tasks

<!-- Deliberately empty. Populate after "Inputs from Phase 1" is filled and decision 2
     is resolved — task breakdown before then would be guesswork. -->

## Unknowns

- Cost: four queries per poll cycle across N projects. Does this need its own, slower
  interval than the GitHub feed?
- Can "newly assigned" be detected without a snapshot, e.g. via a changed-date filter, so a
  fresh install does not flood the feed on first run?
- Does any source need a PAT scope beyond what the app already requests?
- Multi-project: one merged Azure feed, or per-project grouping when many projects are
  configured?
- Should approvals from the Releases arc (`release-view-arc.md`) mirror into this feed as
  `approval_pending`, and if so which side owns that mapping?
