# 0001. Notifications as an Optional Capability Interface

**Date:** 2026-07-29
**Status:** Accepted

## Context

GitHub's `GET /notifications` is a user-level inbox; Azure DevOps has no inbox API at all, so any shared abstraction must tolerate a backend that cannot participate. The pane's job is triage — what needs attention now — and phase 1 must not invent local read/done state that phase 2's Azure work would then have to migrate.

## Decision

Notifications attach via an optional `provider.NotificationSource` interface, never as new methods on `Provider`. `CompositeProvider` type-asserts each *backend* for the capability and exposes `HasNotifications()`; the app gates the tab on that, not on asserting the composite — the composite implements the interface unconditionally, so asserting it would leave the tab permanently visible. The pane shows the whole GitHub inbox by default, narrowable through config (repo globs, reasons, `only_configured_repos`). Phase 1 keeps no local read/done state — GitHub owns it server-side; Azure introduces local state only in phase 2.

## Alternatives Considered

### Add notification methods to `Provider`
Rejected: Azure has no inbox API, so its adapter would carry empty stubs plus a runtime `ErrUnsupported` path every caller has to branch on. Metrics already set the precedent for keeping optional surfaces off `Provider`.

### Filter the pane to configured repos by default
Rejected: a feed narrowed to a handful of repos is not a triage pane. Config-driven filters (repo globs, reasons) are the escape hatch for noise instead.

## Consequences

Backends opt in independently: once the `HasNotifications()` gate exists, phase 2's Azure adapter makes the tab appear with no further app-layer change. Visibility is capability-gated, never emptiness-gated; users turn the pane off through `disabled_panes`, since it ships default-on unlike opt-in Metrics. An optional capability value can be a non-nil interface wrapping a nil pointer, so consumers must guard the interface rather than the concrete type. GitHub carries no local persistence to migrate; Azure's phase-2 state must be designed against the `Notification.Read`/`Done` fields already on the neutral type.
