# 0001. Notifications as an Optional Capability Interface

**Date:** 2026-07-29
**Status:** Accepted

## Context

GitHub's `GET /notifications` is a user-level inbox with no Azure DevOps equivalent, and the pane must present that full inbox rather than one pre-narrowed to configured repos. Phase 1 also must not invent local read/done state that phase 2's Azure work would then have to migrate.

## Decision

Notifications attach via an optional `provider.NotificationSource` interface, discovered by type assertion, never as new methods on `Provider`. The pane shows the whole GitHub inbox by default, narrowable only through config (repo/reason filters), not by restricting to configured repos. Phase 1 keeps no local read/done state — GitHub owns it server-side; Azure introduces local state only in phase 2.

## Alternatives Considered

### Add notification methods to `Provider`
Rejected: Azure has no inbox API, forcing empty stubs or a runtime `ErrUnsupported` path and risking a typed-nil boxed into the interface (convention 15). Metrics already set the precedent for keeping optional surfaces off `Provider`.

### Filter the pane to configured repos by default
Rejected: a feed narrowed to a handful of repos is not a triage pane. Config-driven filters (repo globs, reasons) are the escape hatch for noise instead.

## Consequences

Backends opt in independently — the tab hides when no backend implements the capability and reappears automatically once one does, with no app-layer change. GitHub carries no local persistence to migrate. Azure's phase-2 local state must be designed against the `Notification.Read`/`Done` fields that already exist on the neutral type, not bolted on afterward.
