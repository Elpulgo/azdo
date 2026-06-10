# Spec: Trends graph view for metrics

Status: Draft
Date: 2026-06-10

## Goal

Add a line-chart Trends view so sprint-over-sprint trends are visible at a
glance. X axis = selected sprints (from tags), Y axis = one of the four metrics
(switchable). Per-user detail is preserved via a focused user with the rest
ghosted.

This replaces the readability gap of the current text grid in
`internal/ui/metrics/trends.go`, where each user occupies four stacked text
rows (`pts`, `wip`, `stuck`, `cy`) across sprint columns — accurate but hard to
read as a trend.

## Decisions

- **User dimension:** focus + ghosts. One highlighted user (cycle with a key),
  the rest drawn dimmed. Team total always bold/distinct.
- **Layout:** single switchable chart. Keys `1`–`4` / `h` `l` pick the active
  metric. No stacked-all-four mode in v1.
- **Charts:** `github.com/NimbleMarkets/ntcharts` (Charm-native braille line
  charts, integrates with bubbletea/lipgloss).

## Non-goals

- No changes to aggregation in `internal/metrics`. The chart consumes the
  existing `[]TrendRow` / `TrendCell{Points, AvgWIP, StuckCount, CycleTime,
  OverloadedAnyDay}` produced by `internal/metrics/trends.go`.
- No changes to the Live view.
- No new metrics — the four stay `Points / AvgWIP / StuckCount / CycleTime`.
- Keep the existing Trends **table** as a sibling mode (precision fallback);
  nothing is removed in v1.

## Navigation

- The `v` toggle cycle becomes: **Live → Trends (table) → Trends (chart)**.
- In chart mode:
  - `1`–`4` / `h` `l` — select the active metric (re-scales Y).
  - `←` `→` — move the X cursor across sprints; a side readout shows that
    sprint's exact values for the focused user and team.
  - `u` / `[` `]` — cycle the focused user (focus highlighted, others ghosted).
  - `a` — show-all-users ↔ team-only toggle.
  - `T` — sprint tag multi-select (unchanged).

## Implementation

- `internal/metrics/chartdata.go` (pure, unit-tested):
  - Build per-user series from `[]TrendRow`.
  - Y auto-scale: `min = 0`, `max = niceCeil(maxVisible)` with 1/2/5×10ⁿ ticks.
    Independent per metric (the four scales differ wildly — pts ~0–20, wip ~0–5,
    stuck ~0–3, cycle in days — so they cannot share an axis).
  - Gap handling (see Edge cases).
- `internal/ui/metrics/chart.go`:
  - ntcharts wiring, focus/ghost styling from `internal/ui/styles`, X cursor +
    readout, key handling.
- `internal/ui/metrics/list.go`:
  - Wire the new mode into the `v` toggle.
- `go.mod`: add `github.com/NimbleMarkets/ntcharts`.

## Edge cases

- **< 2 sprints selected** → render a hint, no chart (a line over one point is
  not a trend).
- **User absent from a sprint, or no completed items** (cycle time undefined) →
  line break / gap, **never 0**. Missing data must not read as a real zero.
- **All-zero metric** → flat line at 0 with a sane axis range.
- **Cycle time** rendered in days.

## Testing (TDD — fail first, then green)

- `internal/metrics/chartdata_test.go`: `niceCeil`, scaling, gap-vs-zero,
  series extraction from demo-shaped fixtures (reuse `internal/demo/metrics.go`).
- UI smoke: `View()` renders without panic for 0 / 1 / many sprints and users
  (string checks in the style of the existing metrics tests).

## Rollout

Additive third mode — nothing removed. If it proves better than the table in
real use, demote the table later.

## Open risk to spike before committing

ntcharts' multi-dataset line chart styles each series by color from its palette.
The "ghost" effect (dim non-focused users) may need a custom lipgloss style per
series rather than the default palette. Do a ~30-min spike to confirm per-series
styling is controllable; fall back to hand-rolled braille if it fights us.
