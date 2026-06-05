# Architecture

Azure DevOps TUI built with Go and Bubble Tea.

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
│   ├── azdevops/                        # Azure DevOps API client layer
│   │   ├── client.go                    # Single-project HTTP client (auth, GET/POST/PATCH/PUT)
│   │   ├── multiclient.go              # Multi-project wrapper with concurrent fetching
│   │   ├── types.go                     # API response types + convenience methods
│   │   ├── errors.go                    # Error types (PartialError for multi-project)
│   │   ├── pipelines.go                # Pipeline/build API
│   │   ├── git.go                       # Repos, PRs, diffs API
│   │   ├── workitems.go                # Work item queries
│   │   ├── logs.go                      # Build log fetching
│   │   └── timeline.go                 # Pipeline timeline (stages/jobs/tasks)
│   │
│   ├── ui/
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
│  │  Tab Bar  [1: PRs]  [2: Work Items]  [3: Pipelines]       ││
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
                            │
                 ┌──────────┴──────────┐
                 │   MultiClient       │  Concurrent per-project
                 │   ┌─────┐ ┌─────┐  │  fetching, result merging,
                 │   │Proj1│ │Proj2│  │  enrichment
                 │   └─────┘ └─────┘  │
                 └──────────┬──────────┘
                            │
                 ┌──────────┴──────────┐
                 │   Azure DevOps      │
                 │   REST API (v7.1)   │
                 └─────────────────────┘
```

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

### 4. Multi-Project Client

The API layer uses a two-tier client pattern:

- **`Client`** — single-project HTTP client. Handles auth (Basic + PAT), request construction, and HTTP error classification. One instance per configured project.
- **`MultiClient`** — wraps multiple `Client` instances. Fetches from all projects concurrently using `sync.WaitGroup`, merges and sorts results, and enriches items with project metadata (`ProjectName`, `ProjectDisplayName`).

Multi-project failures use `PartialError` — if 1 of 3 projects fails, the UI shows data from the 2 that succeeded plus a warning. No all-or-nothing failures.

### 5. Background Polling with Graceful Degradation

The polling system has two components:

- **`Poller`** — manages fetch intervals, sends `PipelineRunsUpdated` messages via `tea.Cmd`. Supports one-shot fetches and continuous polling with configurable interval.
- **`ErrorHandler`** — tracks consecutive failures and maintains last-known-good data. If a fetch fails, the UI keeps showing stale data instead of going blank. After a configurable threshold of consecutive failures, the error is escalated to a modal.

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
- Organization name
- Project list (supports simple strings or objects with display names)
- Polling interval
- Theme selection

**Auth** uses a priority chain:
1. System keyring (Windows Credential Manager / macOS Keychain / Linux SecretService)
2. `AZDO_PAT` environment variable fallback

If neither is found, the setup wizard or PAT input modal guides the user through initial setup.

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
| Pipelines | Run list | Timeline tree (stages/jobs) | Log viewer |
| Pull Requests | PR list | Detail (description, threads) | Diff view with comments |
| Work Items | Item list | Detail (description, links) | — |

Navigation is `enter` to drill down, `esc` to go back. The `viewMode` field on each model tracks the current level.

### 10. Metrics Dashboard (opt-in tab)

The metrics tab is the only feature with persistent local state. It's gated behind `metrics.enabled` so the default install pays no cost (no extra API calls, no file on disk).

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
  → MultiClient fetches concurrently from all projects
    → Each Client makes HTTP request with PAT auth
    → Responses decoded into typed structs
  → Results merged, sorted, enriched with project metadata
  → tea.Msg sent (e.g., PipelineRunsUpdated)
    → ErrorHandler processes: success → store data, failure → return stale data
  → Root model delegates to active tab
    → listview.Model updates items + table rows via ToRows callback
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

## Azure DevOps API Reference

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

## Design Principles

- **Accept interfaces, return structs** — API client uses `PipelineClient` interface for testability; views accept `DetailView` interface for polymorphism
- **Constructor injection** — clients, styles, and config are passed via constructors, never global state
- **Graceful degradation** — partial failures show available data with warnings, not blank screens
- **Composition over inheritance** — generic `listview.Model[T]` is configured via callbacks, not subclassed
- **Message-driven async** — all I/O flows through `tea.Cmd` and `tea.Msg`, keeping the UI non-blocking
- **TDD** — table-driven tests, interface mocking, test coverage across all packages
