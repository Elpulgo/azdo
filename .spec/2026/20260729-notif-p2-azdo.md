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

<!-- Phase 1 task 18 fills this in. Do not plan this phase until it is populated. -->

- **Reason enum coverage:** which reasons phase 1 defined, and which Azure sources can
  legitimately map onto them without inventing new ones — TBD
- **Composite fan-out semantics:** how partial failure was handled when one backend errors
  (drop that backend's rows, or fail the whole feed?) — TBD
- **Filter placement:** whether the config filter ended up in `provider` or `app`, and
  whether it can be reused verbatim for Azure rows — TBD
- **Subject → web URL:** what the GitHub resolver's degradation strategy was, so the Azure
  URL builders match its contract — TBD
- **Polling contract:** what interface the pane/poller settled on, given Azure has no
  `X-Poll-Interval` equivalent — TBD
- **Unread semantics:** how the footer badge counted unread, and whether that definition
  survives a locally-tracked read state — TBD

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
