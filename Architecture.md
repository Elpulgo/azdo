# Architecture

A terminal UI for reviewing pull requests, work items, and pipelines, built
with Go and Bubble Tea.

Originally an Azure-DevOps-only client, the app is now **provider-agnostic**: a
backend-neutral `provider.Provider` interface sits between the UI and the
concrete backends (Azure DevOps and GitHub). Views depend only on neutral
domain types and never import a backend package. A single install can talk to
Azure DevOps, GitHub, or both at once — entities from different backends are
merged into the same lists and tagged with their origin. (The product is still
named `azdo` for historical reasons.)

## Project Structure

```
azdo/
├── cmd/azdo-tui/
│   └── main.go                          # Entry point, CLI dispatch, bootstrap
│
├── internal/
│   ├── app/
│   │   ├── app.go                       # Root Bubble Tea model, tab navigation, layout
│   │   └── app_test.go
│   │
│   ├── provider/                        # Backend-neutral core (no HTTP, no UI)
│   │   ├── provider.go                  # Provider interface every view depends on
│   │   ├── types.go                     # Neutral domain types (+ Identity, Kind)
│   │   ├── enums.go                     # Neutral semantic enums (StateCategory, ItemType, VoteKind, RunStatus)
│   │   ├── notifications.go            # NotificationSource + PollIntervalHinter (optional capability interfaces)
│   │   ├── composite.go                # CompositeProvider: fan-out list calls, scope-route detail calls, notifications fan-out
│   │   ├── list_opts.go                # ListOpts — neutral filter intent passed to list calls
│   │   └── errors.go                    # PartialError (shared across backends)
│   │
│   ├── azdevops/                        # Azure DevOps backend
│   │   ├── client.go                    # Single-project HTTP client (auth, GET/POST/PATCH/PUT)
│   │   ├── multiclient.go              # Multi-project wrapper with concurrent fetching
│   │   ├── adapter.go                  # Wraps MultiClient as a provider.Provider
│   │   ├── mapping.go                  # Wire types → neutral domain types
│   │   ├── mapper_enums.go             # Wire strings → neutral enums (MapStateCategory, etc.)
│   │   ├── types.go                     # API response types + convenience methods
│   │   ├── errors.go                    # Error types (PartialError for multi-project)
│   │   ├── pipelines.go                # Pipeline/build API
│   │   ├── git.go                       # Repos, PRs, diffs API
│   │   ├── workitems.go                # Work item queries
│   │   ├── logs.go                      # Build log fetching
│   │   └── timeline.go                 # Pipeline timeline (stages/jobs/tasks)
│   │
│   ├── github/                          # GitHub backend
│   │   ├── client.go                    # Single-repo HTTP client (REST + GraphQL)
│   │   ├── multiclient.go              # Multi-repo wrapper, keyed by "owner/repo"
│   │   ├── adapter.go                  # Wraps MultiClient as a provider.Provider (+ NotificationSource, PollIntervalHinter)
│   │   ├── notifications.go            # User-scoped GET /notifications client: pagination, If-Modified-Since, X-Poll-Interval, mark read/done
│   │   ├── mapping.go                  # Issues/PRs → neutral domain types
│   │   ├── mapping_pr.go               # PR + review/vote mapping
│   │   ├── mapping_pipeline.go         # Actions runs → neutral PipelineRun/Timeline
│   │   ├── mapping_notifications.go    # Notification thread → neutral Notification, reason mapping, web-URL resolution
│   │   ├── mapper_enums.go             # GitHub states → neutral enums
│   │   ├── labels.go                   # LabelConvention: labels → ItemType/Priority/Tags
│   │   ├── pullrequests.go             # Pulls, reviews, files, comments API
│   │   ├── workitems.go                # Issues API (issues == work items)
│   │   ├── pipelines.go                # Actions runs/jobs/logs API
│   │   ├── weburl.go                   # Browser URL builders
│   │   └── types.go                     # GitHub API response types
│   │
│   ├── ui/
│   │   ├── display/
│   │   │   └── display.go              # Neutral enum → glyph/label/style + Kind glyph helpers
│   │   │
│   │   ├── styles/
│   │   │   ├── styles.go               # Lipgloss style struct & factories
│   │   │   ├── theme.go                # Theme type definition
│   │   │   └── themes.go              # Built-in themes (dark, light, dracula, etc.)
│   │   │
│   │   ├── components/                 # Reusable UI building blocks
│   │   │   ├── listview/
│   │   │   │   └── listview.go         # Generic list view (list/detail toggle, search)
│   │   │   ├── table/
│   │   │   │   └── table.go           # Custom table (ANSI-aware truncation)
│   │   │   ├── statusbar.go           # Footer (org, project, connection state)
│   │   │   ├── errormodal.go          # Error overlay modal
│   │   │   ├── help.go                # Help overlay with keybindings
│   │   │   ├── tagpicker.go           # Work item tag filter
│   │   │   ├── spinner.go             # Loading indicator
│   │   │   ├── themepicker.go         # Theme selector
│   │   │   ├── votepicker.go          # PR vote/approval picker
│   │   │   ├── statepicker.go         # Work item state picker
│   │   │   ├── logo.go                # ASCII art logo
│   │   │   └── contextitem.go         # Context-aware keybinding items
│   │   │
│   │   ├── notifications/
│   │   │   ├── list.go                 # Notifications listview pane: reason filter, mark read/done, open in browser, error/disable states
│   │   │   └── filter.go              # Config-driven filter (pure function over the merged feed)
│   │   │
│   │   ├── pipelines/
│   │   │   ├── list.go                 # Pipeline runs list
│   │   │   ├── detail.go              # Timeline detail (expandable tree)
│   │   │   └── logviewer.go           # Log viewer with scrolling & search
│   │   │
│   │   ├── pullrequests/
│   │   │   ├── list.go                 # PR list view
│   │   │   ├── detail.go              # PR description, threads, voting
│   │   │   └── diffview.go            # File diff viewer with inline comments
│   │   │
│   │   ├── workitems/
│   │   │   ├── list.go                 # Work item list with filtering
│   │   │   └── detail.go              # Work item detail & state changes
│   │   │
│   │   ├── metrics/                    # Metrics dashboard tab (opt-in)
│   │   │   ├── list.go                 # Live view: per-user roll-up + stuck pane
│   │   │   ├── trends.go               # Trends sub-view: sprint × user grid
│   │   │   ├── snapshot.go             # Daily snapshot writer command + gap fallback
│   │   │   └── backfill.go             # One-shot /updates backfill orchestrator
│   │   │
│   │   ├── patinput/
│   │   │   └── patinput.go            # PAT input modal for auth setup
│   │   │
│   │   ├── providerselect/
│   │   │   └── providerselect.go      # Provider picker shown during 'azdo auth' (Azure / GitHub)
│   │   │
│   │   └── setupwizard/
│   │       └── setupwizard.go         # Interactive first-run config wizard
│   │
│   ├── metrics/                        # Pure metrics core (no UI, no I/O on hot path)
│   │   ├── aggregate.go                # Live aggregation: WIP, stuck, closed pts
│   │   ├── snapshot.go                 # JSONL read/write, dedup, prune, mutex
│   │   ├── transitions.go              # /updates fold + gap-fallback classifier
│   │   ├── trends.go                   # Sprint windowing + TrendAggregate
│   │   ├── selection.go                # Persisted sprint-picker selection
│   │   └── backfill.go                 # Marker-file helpers for one-shot backfill
│   │
│   ├── config/
│   │   ├── config.go                   # YAML config loading (viper)
│   │   └── keyring.go                 # PAT storage via system keyring
│   │
│   ├── state/
│   │   ├── state.go                    # Persistent navigation state (active tab, last detail IDs)
│   │   └── store.go                    # Debounced, atomic, thread-safe state writer
│   │
│   ├── polling/
│   │   ├── poller.go                   # Background polling manager
│   │   ├── notifications_poller.go    # Separate poller for the notifications feed (own tick type, own cadence)
│   │   ├── errorhandler.go            # Error recovery & graceful degradation
│   │   └── events.go                  # tea.Msg types for polling events
│   │
│   ├── cli/
│   │   └── cli.go                      # CLI argument parsing (no cobra)
│   │
│   ├── diff/
│   │   └── diff.go                     # Diff parsing & formatting
│   │
│   └── version/
│       └── version.go                  # Version checking & update notifications
│
├── go.mod
├── go.sum
└── .goreleaser.yaml
```

## System Overview

```
┌────────────────────────────────────────────────────────────────┐
│                        Terminal (TUI)                           │
│  ┌────────────────────────────────────────────────────────────┐│
│  │  Tab Bar  [1: Notifications]  [2: PRs]  [3: Work Items]   ││
│  │           [4: Pipelines]                                  ││
│  └────────────────────────────────────────────────────────────┘│
│  ┌────────────────────────────────────────────────────────────┐│
│  │                                                            ││
│  │           Active View (list → detail → sub-view)           ││
│  │                                                            ││
│  │  Modals overlay: Error | Help | Theme | Pickers            ││
│  │                                                            ││
│  └────────────────────────────────────────────────────────────┘│
│  ┌────────────────────────────────────────────────────────────┐│
│  │  Footer: org/project · connection state · context keys     ││
│  └────────────────────────────────────────────────────────────┘│
└───────────────────────────┬────────────────────────────────────┘
                            │
                 ┌──────────┴──────────┐
                 │   Polling Manager   │  Background goroutines
                 │   + Error Handler   │  send tea.Msg updates
                 └──────────┬──────────┘
                            │  depends only on provider.Provider
                 ┌──────────┴──────────┐
                 │ CompositeProvider   │  fan-out list calls across
                 │                     │  backends, merge + sort;
                 │                     │  route detail calls by scope
                 └─────┬─────────┬─────┘
                       │         │
            ┌──────────┴──┐   ┌──┴──────────┐
            │ azdevops    │   │ github      │  adapters: wire types →
            │ .Adapter    │   │ .Adapter    │  neutral domain types
            └──────┬──────┘   └──────┬──────┘
                   │                 │
            ┌──────┴──────┐   ┌──────┴──────┐
            │ MultiClient │   │ MultiClient │  concurrent per-scope
            │ (per project│   │ (per "owner/│  fetching + enrichment
            │  client)    │   │  repo")     │
            └──────┬──────┘   └──────┬──────┘
                   │                 │
            ┌──────┴──────┐   ┌──────┴──────┐
            │ Azure DevOps│   │ GitHub      │
            │ REST v7.1   │   │ REST + GraphQL│
            └─────────────┘   └─────────────┘
```

A backend is built only when its config is present, so an Azure-only user gets a
single-backend `CompositeProvider` that behaves transparently (no glyph column,
no extra calls). The metrics tab is the one exception that still reaches a
concrete `*azdevops.MultiClient` directly (see Decision 5 below).

## Core Dependencies

| Dependency | Purpose |
|------------|---------|
| `charmbracelet/bubbletea` | TUI framework (Elm architecture) |
| `charmbracelet/lipgloss` | Terminal styling and layout |
| `charmbracelet/bubbles` | Pre-built TUI components (textinput, viewport, etc.) |
| `spf13/viper` | YAML config loading |
| `zalando/go-keyring` | System keyring for PAT storage |

No CLI framework (cobra/urfave) — uses lightweight custom CLI parsing in `internal/cli`.

## Architectural Patterns

### 1. Nested Model Hierarchy (Elm Architecture)

The app follows Bubble Tea's model-update-view pattern with a strict nesting hierarchy:

```
App Model (root)
├── Tab views (one active at a time)
│   ├── pullrequests.Model  → listview.Model[PullRequest]
│   ├── workitems.Model     → listview.Model[WorkItem]
│   └── pipelines.Model     → listview.Model[PipelineRun]
├── Overlay modals
│   ├── ErrorModal
│   ├── HelpModal
│   └── ThemePicker
├── StatusBar
└── Logo
```

The root model handles message routing with **priority-based dispatch**: modals consume messages first (error → help → theme), then global keybindings, then delegation to the active tab. This prevents key presses from leaking through overlays.

### 2. Generic List View (`listview.Model[T]`)

All three tabs share a generic, type-parameterized list view that provides:
- Scrollable table display
- Inline search/filter (press `f`)
- List ↔ detail view toggling (enter/esc)
- Loading state with spinner
- Error display

Domain-specific behavior is injected via a **configuration callback struct**:

| Callback | Purpose |
|----------|---------|
| `ToRows` | Format domain items into table rows |
| `Fetch` | Return a `tea.Cmd` to load items from the API |
| `EnterDetail` | Create a detail view for the selected item |
| `FilterFunc` | Determine if an item matches a search query |
| `HasContextBar` | Whether to show context-aware keybindings |

This avoids duplicating list/detail/search logic across tabs while keeping each tab's rendering and data handling domain-specific.

### 3. DetailView Interface

Detail views implement a common interface so the generic list view can manage them uniformly:

| Method | Purpose |
|--------|---------|
| `Update(msg) (DetailView, Cmd)` | Handle messages |
| `View() string` | Render content |
| `SetSize(w, h)` | Respond to window resize |
| `GetContextItems()` | Context-aware keybindings for footer |
| `GetScrollPercent()` | Scroll position for status bar |
| `GetStatusMessage()` | Status text for footer |

Implemented by pipeline detail (timeline tree), PR detail (threads, voting, diff), and work item detail (state management).

### 4. Provider Abstraction (backend-neutral core)

The single most important structural change from the Azure-only origin: the UI
no longer knows what a backend is. Everything above the adapter boundary speaks
in neutral types defined in `internal/provider`.

**The `Provider` interface** (`internal/provider/provider.go`) is what every view
depends on. It covers four surfaces — pull requests, work items, pipelines, and
build logs — plus per-entity web-URL helpers and two multi-project helpers
(`IsMultiProject()`, `Scopes()`). It deliberately excludes the metrics surface
(Decision 5): metrics stay on the concrete `*azdevops.MultiClient`.

List methods (`ListPullRequests`, `ListWorkItems`, `ListPipelineRuns`, and the
"my"/"as-reviewer" variants) take a `top int` and a neutral `ListOpts`; detail
and mutation methods take a `scope` string as their first argument so calls can
be routed to the backend that owns that scope.

**Neutral domain types** (`internal/provider/types.go`) — `PullRequest`,
`WorkItem`, `Thread`, `Comment`, `Reviewer`, `PipelineRun`, `Timeline`,
`Iteration`, `IterationChange`, etc. Every entity carries an **`Identity`**:

| Field | Meaning |
|-------|---------|
| `Kind` | which backend produced it (`KindAzure`, `KindGitHub`) |
| `Scope` | the API-name scope: Azure project name, or GitHub `"owner/repo"` |
| `ScopeDisplay` | human-readable scope (falls back to `Scope`) |
| `ID` | the wire ID, stringified |

`Scope` is the routing key — the same string a backend reports from `Scopes()`
and that the views thread back into detail/mutation calls.

**Neutral semantic enums** (`internal/provider/enums.go`) let views pick a glyph,
label, and color without inspecting backend-specific strings:

| Enum | Drives |
|------|--------|
| `StateCategory` | work-item / PR state (New, Active, Resolved, ReadyForTest, ClosedDone, Removed) |
| `ItemType` | work-item type (Bug, Task, UserStory, Feature, Epic, Issue) |
| `VoteKind` | reviewer vote (Approved, ApprovedWithSuggestions, WaitingForAuthor, Rejected, NoVote) |
| `RunStatus` | combined pipeline status+result (Running, Queued, Succeeded, Failed, …) |

Each enum is populated **at the adapter mapping boundary** — never in the UI.

#### Backend adapters

Each backend keeps its original two-tier client and gets a thin adapter that
implements `provider.Provider`:

- **`Client`** — single-scope HTTP client (auth, request construction, error
  classification). Azure: one per project. GitHub: one per `"owner/repo"`.
- **`MultiClient`** — wraps multiple `Client`s, fetches concurrently with
  `sync.WaitGroup`, merges/sorts, and enriches with scope metadata. Multi-scope
  failures use `PartialError` — if 1 of 3 scopes fails, the UI shows the 2 that
  succeeded plus a warning. No all-or-nothing failures.
- **`Adapter`** (`azdevops/adapter.go`, `github/adapter.go`) — wraps the
  `MultiClient`, maps wire types → neutral types (see the `mapping*.go` and
  `mapper_enums.go` files), and stamps `Identity` on every returned entity.

#### CompositeProvider

`CompositeProvider` (`internal/provider/composite.go`) is itself a
`provider.Provider` that wraps one or more backend adapters and is what the app
actually hands to the views. Two routing strategies:

- **List calls fan out.** `ListPullRequests` / `ListWorkItems` /
  `ListPipelineRuns` call every backend concurrently, merge the results, and sort
  (PRs by `CreationDate` desc, work items by `ChangedDate` desc, pipeline runs by
  `QueueTime` desc). All-backends-fail returns a plain error; a partial failure
  returns the surviving data wrapped in `PartialError`.
- **Detail / mutation / URL calls route by scope.** A `scope → backend` index is
  built once at construction from each backend's `Scopes()`. An unknown scope
  returns a descriptive routing error (URL helpers return `""`).
- **Notifications fan out over capable backends only** (see below) — a third
  strategy, neither of the two above, because `GET /notifications` is
  user-level and has no `scope` to route by.

Design decisions baked in: a single-backend composite is **transparent** (D1);
scope collisions resolve **first-registered-wins** (D3); `Kind()` returns the
first backend's kind and is *not* used for per-row rendering — `Identity.Kind`
is (D4).

#### Notifications: an optional capability, not a `Provider` method

Notifications are modeled the same way Metrics is (Decision 5's precedent,
carried forward as Decision 1 of the notifications spec): as an **optional
capability interface**, not a method added to `provider.Provider`. Azure
DevOps has no inbox API, so extending `Provider` would force it to carry
empty stub methods or a runtime "unsupported" error path — both of which
convention 15 rules out, along with the typed-nil trap they invite.

**`provider.NotificationSource`** (`internal/provider/notifications.go`) is
the capability interface:

```go
type NotificationSource interface {
    List(opts NotifOpts) ([]Notification, error)
    MarkRead(id Identity) error
    MarkDone(id Identity) error
}
```

A backend implements it or it doesn't; nothing asserts it into existence. The
cadence hint (GitHub's `X-Poll-Interval` header) is a *second*, independent
optional interface, `PollIntervalHinter` — deliberately not a fourth method on
`NotificationSource`, because widening that interface later would force every
implementer (including a future Azure one) to also implement a hint it has no
equivalent for.

**`CompositeProvider` discovers support by type-asserting each backend it
wraps**, never by asserting itself — `*CompositeProvider` satisfies
`NotificationSource` unconditionally (it has `List`/`MarkRead`/`MarkDone`
methods of its own that fan out), so asserting the composite would always
report "capable" even when every backend inside it is Azure-only. The
correct capability check is `HasNotifications()`, which loops over
`cp.backends` and asserts each one individually. This is what lets the app
decide whether to show the tab at all (Decision 11): hidden on *capability*,
never on the feed being empty, and it flips back on by itself the moment a
future backend implements the interface — no app-layer change required.

`List` fans out concurrently to every capable backend, merges the results,
sorts newest-first, and — per Decision 20 — never lets one backend's failure
empty the feed: the other backends' rows still come back, wrapped alongside
a `*PartialError` naming what failed. `MarkRead`/`MarkDone` route by the
notification's own `Identity.Kind` over capable backends only — never by
`backendFor(scope)`, which is built from configured scopes and would return
nil for the (very common) case of a notification from a repo nobody
configured a client for.

**The neutral `Notification` type** (`internal/provider/types.go`) carries a
provider-qualified `Identity` (kind + scope + native id — never a bare id,
since a future Azure identity scheme could otherwise collide with a GitHub
thread id) plus `Read`/`Done` fields. GitHub populates `Read`/`Done` from the
server; a future Azure implementation would populate them from local state
instead — the same fields, filled by a different source, so adding Azure
support later changes *who* fills them, not the shape the UI consumes.

**Single entrypoint, same rule as every other list.** The `internal/ui/notifications`
pane consumes `[]provider.Notification` and nothing else — it never imports
`internal/github` or knows which backend produced a given row, exactly the
same convention the PR/work-item/pipeline lists already follow. Provider
normalization happens once, at the adapter mapping boundary (`mapping_notifications.go`
maps a GitHub notification thread to the neutral type); the pane, the filter
(`internal/ui/notifications/filter.go`), and the footer's unread badge all
operate purely on the merged, neutral feed.

#### Provider-aware rendering

All theming for neutral enums lives in `internal/ui/display` (`display.go`). View
code passes an enum value and gets back a ready-to-render glyph, label, and
lipgloss style — it never branches on backend strings. Helpers exist for
`StateCategory`, `ItemType`, `VoteKind`, `RunStatus`, and `Kind`.

The `Kind` helpers drive the only visible cross-provider affordance: when a list
spans more than one backend, row builders prepend a **provider-origin glyph
column**. `MixedKinds(kinds)` decides whether the column appears at all, so a
single-backend list (the common case) shows no glyph and keeps its original
column layout. When shown, `KindGlyph` renders `⬢` for Azure and `⎇` for GitHub,
styled muted via `KindStyle` so it reads as secondary metadata, not status.

Because the trigger is per-list (not per-config), an Azure-only or GitHub-only
user never sees the column; a both-backends user sees it only on lists that
actually mix origins.

### 5. Background Polling with Graceful Degradation

The polling system has these components:

- **`Poller`** — manages fetch intervals, sends `PipelineRunsUpdated` messages via `tea.Cmd`. Supports one-shot fetches and continuous polling with configurable interval.
- **`ErrorHandler`** — tracks consecutive failures and maintains last-known-good data. If a fetch fails, the UI keeps showing stale data instead of going blank. After a configurable threshold of consecutive failures, the error is escalated to a modal.
- **`NotificationsPoller`** (`internal/polling/notifications_poller.go`) — a
  separate poller for the notifications feed, with its own tick message type
  (`NotificationsTickMsg`, never the shared `TickMsg`, since two pollers
  emitting the same bare marker would be indistinguishable at the receiving
  end and one arming the other's fetch loop is a runaway-timer hazard, not a
  convenience). Its cadence is `max(configured X-Poll-Interval-derived hint,
  notifications.poll_interval or the global polling_interval)`, re-derived on
  every fetch so a live `X-Poll-Interval` response can only ever raise the
  interval, never lower it below what the user configured. Gated on the same
  predicate that decides whether the tab exists at all (capability **and**
  the pane being enabled) — an ungated timer would keep polling GitHub every
  cycle for the life of the process even with the tab disabled or hidden.

### 6. Styles and Theming

All UI components receive a `*styles.Styles` struct via constructor injection. This struct contains pre-built lipgloss styles derived from the active theme.

Theme switching works by:
1. User selects a theme via the theme picker
2. A new `Styles` struct is created from the selected theme
3. All views are recreated with the new styles
4. Config is persisted so the theme survives restarts

Built-in themes include dark, light, dracula, catppuccin, and others.

### 7. Configuration and Auth

**Config** is YAML-based (`~/.config/azdo-tui/config.yaml`) loaded via viper:
- Azure: organization name + project list (simple strings or objects with display names)
- GitHub: `github.repos` (`owner/repo` slugs), plus optional `github.type_prefix`
  and `github.priority_prefix`
- Polling interval
- Theme selection

**Which backends are enabled** is derived from config, not a flag:
- `HasAzure()` — true when both `organization` and `projects` are set.
- `HasGitHub()` — true when `github.repos` has at least one entry.
- `Validate()` requires at least one backend (Decision D5). A half-configured
  Azure stanza (org without projects, or vice-versa) is a fatal error only when
  GitHub is not configured; otherwise it is skipped so GitHub can carry the app.

**GitHub label convention.** `type_prefix` / `priority_prefix` configure how
GitHub issue labels map to a neutral `ItemType` and priority. With the default
`type:` / `priority:`, a label `type:bug` becomes `ItemTypeBug` and is consumed;
labels that don't match (or match but carry an unrecognised value) are surfaced
as tags. Empty/absent prefixes fall back to `DefaultLabelConvention()`.

**Notifications config** (`NotificationsConfig`, `internal/config/config.go`)
holds nine filter/cadence keys (`only_configured_repos`, `include_repos`,
`exclude_repos`, `exclude_reasons`, `unread_only`, `participating_only`,
`since_days`, `max_items`, `poll_interval`), every one defaulting to the
widest possible behaviour. There is deliberately **no `notifications.enabled`
key** — every other default-on pane disables via `disabled_panes`, and the
Notifications tab follows the same single mechanism rather than adding a
second knob that could disagree with it. An unrecognised `exclude_reasons`
value or a malformed `exclude_repos`/`include_repos` glob is dropped at load
with a warning (`Config.Warnings`, rendered in the pane) rather than treated
as a hard config error — the same "warn and degrade, never crash the app"
posture the rest of `internal/config` already has for optional features. See
`README.md`'s Notifications Configuration section for the full key reference.

**Auth** is per-backend, each with a keyring-first priority chain:
- Azure PAT: system keyring → `AZDO_PAT` env fallback.
- GitHub token: system keyring → `GITHUB_TOKEN` env fallback. The Notifications
  tab additionally requires the `notifications` scope on top of whatever the
  rest of the GitHub backend already needs — and specifically a **classic**
  PAT, since GitHub's notifications endpoints support no other token flavor.
  That asymmetry is why the pane's 403 handling treats *absent*
  `X-Accepted-OAuth-Scopes` headers as a positive signal ("this is a
  fine-grained token") rather than as missing information.

System keyring is Windows Credential Manager / macOS Keychain / Linux
SecretService. If a required credential is missing, `azdo auth` (which uses the
`providerselect` picker to choose Azure or GitHub) or the first-run setup wizard
guides the user through setup.

### 8. Navigation State Persistence

A lightweight `internal/state` package persists the last active tab and the most recently opened PR / work item detail to `$XDG_STATE_HOME/azdo-tui/state.yaml` (falling back to `~/.local/state/azdo-tui/state.yaml`). Pipeline detail is intentionally not persisted.

- **`State`** — YAML-tagged struct (`ActiveTab`, `Tabs.PullRequests.LastDetailID`, `Tabs.WorkItems.LastDetailID`) with a `Version` field for forward-compatible schema changes.
- **`Store`** — thread-safe wrapper around `State`. `Apply(mutate)` schedules a debounced write (default 500ms) so rapid tab switches coalesce into a single disk write. `Flush()` is synchronous and called on shutdown.
- **Atomic writes** — `Store` writes to a temp file in the same directory, `fsync`s, then renames over the target, so a crash mid-write never leaves a half-written file.
- **Restore on startup** — the root model calls `ApplyState` once at boot. Disabled or unknown tabs are ignored. For the PR / work item tabs, a one-shot `pendingDetailID` is consumed on the first populate after launch; if the persisted ID isn't found in the loaded data, the app stays on the list (graceful fallback) and the intent is cleared so polling refreshes can't hijack the user back into a stale detail.
- **Shutdown flush** — `cmd/azdo-tui/main.go` forwards SIGINT / SIGTERM / SIGHUP to `tea.QuitMsg{}` and `Flush()`es in a `defer` so debounced writes land before exit. SIGKILL / power loss is unrecoverable; the debounce window bounds the loss.

### 9. CLI Action Dispatch

The entry point uses a simple action enum pattern (no framework). CLI args are parsed into an action (`Help`, `Version`, `Auth`, or default `RunTUI`), and a switch dispatches to the appropriate handler. The `Auth` action runs an interactive PAT setup flow; the default action boots the full TUI.

### 10. View Navigation

Each tab implements a drill-down navigation pattern:

| Tab | Level 1 | Level 2 | Level 3 |
|-----|---------|---------|---------|
| Notifications | Merged feed (list only) | — | — |
| Pipelines | Run list | Timeline tree (stages/jobs) | Log viewer |
| Pull Requests | PR list | Detail (description, threads) | Diff view with comments |
| Work Items | Item list | Detail (description, links) | — |

Navigation is `enter` to drill down, `esc` to go back. The `viewMode` field on each model tracks the current level. Notifications has no detail view in phase 1 (Decision 3: `o` opens the browser, there is nothing to drill into) — `enter`/`esc` on that pane are a harmless round-trip through `listview`'s detail mode with nothing to show.

### 11. Metrics Dashboard (opt-in tab)

The metrics tab is the only feature with persistent local state. It's gated behind `metrics.enabled` so the default install pays no cost (no extra API calls, no file on disk).

It is also the one feature that bypasses the provider abstraction (**Decision 5**): it talks to a concrete `*azdevops.MultiClient` rather than `provider.Provider`, because it relies on Azure-specific surfaces (WIQL, the `/updates` revision endpoint, Azure custom-state semantics) that have no GitHub equivalent. `main.go` therefore keeps the Azure `MultiClient` and passes it to the app alongside the composite. Metrics are unavailable on a GitHub-only install.

**Two-package split.** Logic is divided to keep the core pure and table-testable:

- **`internal/metrics`** — pure aggregation, snapshot I/O, transition algebra. No bubbletea, no HTTP, no UI types. Heavy table-driven tests.
- **`internal/ui/metrics`** — bubbletea model, `tea.Cmd` orchestrators, rendering. Wraps the core with HTTP via `MultiClient` and writes via the snapshot writer.

**Three data tiers.** The tab combines three sources of work-item state, each filling a different role:

| Tier | Field / Source | Cost | Used for |
|------|----------------|------|----------|
| 1 | `Microsoft.VSTS.Common.StateChangeDate` (current-state dwell) | One field on existing fetch | Live: "who's stuck right now" |
| 2 | `/updates` REST endpoint (per-item revision history) | One call per item | One-shot 90-day backfill + gap-fallback when a state was skipped between days |
| 3 | Local 90-day JSONL snapshot file | Free — reuses Tier 1 fetch | Trends: sprint-on-sprint comparison |

Tier 3 is the only persistent state. Tier 2 is bounded (only used in two specific paths, never on every poll).

**Snapshot file.** `~/.config/azdo-tui/metrics.jsonl`, one row per (work item, day, observed state). Written once per calendar day on first metrics-tab open. The writer reads existing rows, dedups by `(TS, ID)` latest-wins, prunes anything older than 90 days, and atomically renames a temp file. A package-level `sync.Mutex` serializes calls so the daily writer and the one-shot backfill cannot race on the read-merge-rename sequence.

**Gap fallback.** If today's observed state can't be reconciled with the previous snapshot row in a single legal transition (e.g. Active → Closed with no RFT row in between), the writer fires `/updates` for that item and synthesizes the missing intermediate rows. Bounded concurrency (cap 4) and per-item failures don't fail the whole snapshot save.

**One-shot backfill.** Opt-in via `metrics.run_one_shot_backfill: true`. On launch, walks every in-flight or recently-closed item across all configured projects, fans `/updates` calls with the same bounded-concurrency pattern, and synthesizes 90 days of snapshot rows tagged `Source="updates"`. A marker file at `~/.config/azdo-tui/.metrics-backfill-done` prevents re-runs; delete it to re-seed. Reuses the same `SynthesizeGapRows` helper as gap-fallback.

**Trends sub-view.** Toggled with `v`. Reads exclusively from the snapshot file — no live fetch. The user picks sprint tags through a multi-select picker (`T`); selection is persisted to `~/.config/azdo-tui/metrics-selection.json`. Sprint windows are derived purely from the snapshot rows (`Start` = earliest observation of the tag, `End` = latest non-Closed observation, or `now` if the sprint is still in flight). `TrendAggregate` then produces a users × sprints grid with points closed, average WIP, stuck count, cycle time, and an overloaded-any-day flag — each computed from the daily rows within the window.

**TUI error surfacing.** All metrics paths use the standard `PartialError` (multi-project partial fetch) + `GetStatusMessage()` (footer status) conventions. Nothing in the metrics layer writes to `log`/stderr — that would corrupt the rendered grid.

## Data Flow

### Fetch → Display

```
Poller tick / manual refresh
  → CompositeProvider fans the list call out to every backend concurrently
    → Each backend's MultiClient fetches concurrently from all its scopes
      → Each Client makes an HTTP request with its backend's auth
      → Responses decoded into wire structs
    → Adapter maps wire → neutral types, stamping Identity (Kind, Scope, …)
  → Composite merges all backends' results, sorts (e.g. CreationDate desc)
    → partial failure → surviving data wrapped in PartialError
  → tea.Msg sent (e.g., PipelineRunsUpdated)
    → ErrorHandler processes: success → store data, failure → return stale data
  → Root model delegates to active tab
    → listview.Model updates items + table rows via ToRows callback
      → row builder prepends a Kind glyph column iff the list mixes backends
    → View() renders table
```

### Search/Filter

```
User presses 'f' → search mode enabled, text input focused
  → Keystrokes update search query
  → FilterFunc(item, query) applied to all items
  → Filtered items rendered via ToRows
  → Esc exits search mode, restores full list
```

### Theme Change

```
User presses 't' → theme picker shown
  → Selection sends ThemeSelectedMsg
  → New Styles created from theme
  → All views recreated with new styles
  → Config persisted to disk
```

## Backend API Reference

### Azure DevOps

All endpoints use base URL `https://dev.azure.com/{organization}/` with Basic auth (empty username, PAT as password).

| Feature | Endpoint | Version |
|---------|----------|---------|
| List builds | `GET {project}/_apis/build/builds` | 7.1 |
| Build timeline | `GET {project}/_apis/build/builds/{id}/timeline` | 7.1 |
| Build logs | `GET {project}/_apis/build/builds/{id}/logs/{logId}` | 7.1 |
| List PRs | `GET {project}/_apis/git/repositories/{repo}/pullrequests` | 7.1 |
| PR threads | `GET {project}/_apis/git/repositories/{repo}/pullrequests/{id}/threads` | 7.1 |
| Update PR | `PATCH {project}/_apis/git/repositories/{repo}/pullrequests/{id}` | 7.1 |
| Work items (WIQL) | `POST {project}/_apis/wit/wiql` | 7.1 |
| Work item by ID | `GET {project}/_apis/wit/workitems/{id}` | 7.1 |

### GitHub

REST endpoints use base URL `https://api.github.com` with `Authorization: Bearer <token>`, `Accept: application/vnd.github+json`, and `X-GitHub-Api-Version: 2022-11-28`. A few list paths use the GraphQL endpoint.

| Feature | Endpoint |
|---------|----------|
| List PRs | `GET /repos/{owner}/{repo}/pulls` |
| PR changed files | `GET /repos/{owner}/{repo}/pulls/{n}/files` |
| PR review comments | `GET /repos/{owner}/{repo}/pulls/{n}/comments` |
| Submit review (vote) | `POST /repos/{owner}/{repo}/pulls/{n}/reviews` |
| File content | `GET /repos/{owner}/{repo}/contents/{path}` |
| Work items (issues) | `GET /repos/{owner}/{repo}/issues`, `/issues/{n}`, `/issues/{n}/comments` |
| "My" / reviewer lists | `GET /search/issues` |
| Actions runs | `GET /repos/{owner}/{repo}/actions/runs`, `/actions/runs/{id}/jobs` |
| Actions job logs | `GET /repos/{owner}/{repo}/actions/jobs/{id}/logs` |
| Current user | `GET /user` |
| Combined queries | `POST /graphql` |

**Backend mapping highlights.** GitHub *issues* are the neutral work-item type;
issue labels become `ItemType` / priority / tags via the `LabelConvention`.
GitHub *Actions* runs map to neutral pipeline runs and timelines. GitHub PRs have
no per-push iterations, so the adapter returns a single synthetic iteration
covering the whole PR and serves changed files (with ready-made unified-diff
patches in `IterationChange.Patch`) from the PR files API. PR reviews map to
votes — `APPROVED` → approved, `CHANGES_REQUESTED` → rejected — which is why the
vote picker offers only Approve / Request changes for GitHub PRs.

## Design Principles

- **Backend neutrality** — the UI depends only on `provider.Provider` and neutral
  domain types; backend specifics (HTTP, wire shapes, vendor strings) stay behind
  adapters and are translated to neutral enums at the mapping boundary
- **Adapter-boundary mapping** — `Identity` and every semantic enum are populated
  exactly once, where wire types become neutral types; views never re-derive them
- **Accept interfaces, return structs** — API client uses `PipelineClient` interface for testability; views accept `DetailView` interface for polymorphism
- **Constructor injection** — clients, styles, and config are passed via constructors, never global state
- **Graceful degradation** — partial failures show available data with warnings, not blank screens
- **Composition over inheritance** — generic `listview.Model[T]` is configured via callbacks, not subclassed
- **Message-driven async** — all I/O flows through `tea.Cmd` and `tea.Msg`, keeping the UI non-blocking
- **TDD** — table-driven tests, interface mocking, test coverage across all packages
