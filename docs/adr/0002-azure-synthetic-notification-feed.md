# 0002. Synthetic Notification Feed for Azure DevOps

**Date:** 2026-08-07
**Status:** Accepted

## Context

Azure DevOps has no readable notifications inbox — the Notification API only manages subscription rules — so the feed is synthesized from four polled sources: PRs awaiting review, work-item @mentions, recently assigned work items, and failed pipeline runs. No server owns read/done state, so it is persisted locally and must survive a feed recomputed from scratch on every poll; synthetic rows have no stable server ID, so their identity key must be derived and stay stable across polls.

## Decision

A local triage store keyed by `<source>/<entity>/<entity_id>` holds `{read, done, last_activity, last_seen}` beside each row rather than inside the key. On every poll, a subject whose current activity timestamp is newer than the stored one has its `read`/`done` cleared and the stamp advanced — resurrection is an explicit reconcile step, not a side effect of key churn. Auto-expiry is implicit in recomputation: a source that stops matching a subject stops returning it and the row disappears; the only explicit work is TTL-pruning orphaned state rows. No source keeps a snapshot of previously-seen ids, since a lost snapshot floods the feed on the next poll: assigned work items and failed runs are bounded by an explicit day window, mentions by the server's own recent-mentions macro, and review requests by the set of PRs still awaiting you. All four are stateless and idempotent — polling unchanged data twice yields the same rows. Mentions additionally need a two-stage query — WIQL narrows candidate work items, the Comments API confirms and timestamps each one — because WIQL's only timestamp is when the item last changed, not when the user was mentioned.

## Alternatives Considered

### Stamp-in-key identity (`kind:scope:entity_id:activity_stamp`)
Rejected: folding the activity stamp into the key makes resurrection automatic, but the identity stops naming an entity and starts naming an event — same-item comparison fails across a comment, the identity-keyed pane cursor moves mid-triage as new activity lands, and the state file grows one row per activity event forever, making TTL pruning load-bearing for correctness rather than hygiene.

## Consequences

Reconciliation is a pure function over rows and stored state, so resurrection and expiry are table-testable rather than only verifiable end-to-end. The identity key names the source and the subject together, so two sources surfacing the same entity — a work item that is both assigned to you and mentions you — remain two rows, triaged independently.
