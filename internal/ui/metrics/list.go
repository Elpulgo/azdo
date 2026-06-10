// Package metrics implements the metrics-tab UI: a per-developer dashboard
// fed by internal/metrics.Aggregate. It is built directly on lipgloss + the
// shared table component, not the list/detail listview, because the screen is
// a stacked dashboard with no drill-down.
package metrics

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Elpulgo/azdo/internal/azdevops"
	"github.com/Elpulgo/azdo/internal/browser"
	"github.com/Elpulgo/azdo/internal/config"
	coremetrics "github.com/Elpulgo/azdo/internal/metrics"
	"github.com/Elpulgo/azdo/internal/ui/components"
	"github.com/Elpulgo/azdo/internal/ui/styles"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// openURL is a package-level seam so tests can intercept browser launches.
var openURL = browser.Open

// Fixed column widths used by renderFlagsPane and renderUsersPane. The header
// row and data rows reference these so columns stay aligned regardless of the
// content's natural width.
const (
	// Flags-pane columns
	flagCursorW  = 2  // "  " or "> "
	flagIDW      = 8  // "#1234567"
	flagStateW   = 7  // "Active" / "RFT"
	flagDwellW   = 6  // "12d"
	flagUserW    = 14
	flagProjectW = 18
	flagTitleW   = 50

	// Users-pane columns
	userCursorW    = 2
	userNameW      = 18
	userInFlightW  = 10 // values can include " ⚠"
	userActiveW    = 7
	userRFTW       = 5
	userOldActiveW = 11
	userOldRFTW    = 9
	userClosedPtsW = 11
	userStalledW   = 3
)

// padCol pads or truncates s so its display width equals n. Truncation uses
// "…"; padding uses ASCII spaces. ANSI-aware via lipgloss.Width / ansi.Truncate.
func padCol(s string, n int) string {
	w := lipgloss.Width(s)
	if w == n {
		return s
	}
	if w > n {
		return ansi.Truncate(s, n, "…")
	}
	return s + strings.Repeat(" ", n-w)
}

// focusedPane identifies which sub-pane (flags vs users) owns the cursor.
type focusedPane int

const (
	paneFlags focusedPane = iota
	paneUsers
)

// viewMode toggles between the Live dashboard and the Trends sprint-on-sprint
// comparison. Swapped with the `v` key.
type viewMode int

const (
	viewLive viewMode = iota
	viewTrends
	viewTrendsChart
)

// isTrendsLike reports whether the mode is one of the sprint-on-sprint views
// (table or chart), both of which share the Trends data and several key guards.
func (v viewMode) isTrendsLike() bool {
	return v == viewTrends || v == viewTrendsChart
}

// flagFilter is the f-key cycle position.
type flagFilter int

const (
	flagFilterAll flagFilter = iota
	flagFilterActiveStale
	flagFilterRFTStale
)

// metricsLoadedMsg is the fetch-completion message for the metrics tab.
type metricsLoadedMsg struct {
	items     []azdevops.WorkItem
	err       error
	fetchedAt time.Time
}

// openURLResultMsg is sent when an attempt to open a URL completes.
type openURLResultMsg struct {
	err error
}

// Model is the metrics dashboard model.
type Model struct {
	client      *azdevops.MultiClient
	config      *config.Config
	styles      *styles.Styles

	allItems    []azdevops.WorkItem
	userRows    []coremetrics.UserMetrics
	flags       []coremetrics.ItemFlag

	activeTag   string
	flagFilter  flagFilter
	focusedPane focusedPane
	userCursor  int
	flagCursor  int

	mode             viewMode
	snapshots        []coremetrics.Snapshot
	selectedSprints  []string
	sprintWindows    []coremetrics.SprintWindow
	trendRows        []coremetrics.TrendRow
	availableSprints []string

	// Trends chart state.
	chartMetric  coremetrics.MetricKind
	sprintCursor int // index into sprintWindows for the readout column

	loading       bool
	lastUpdated   time.Time
	statusMessage string

	width, height int
	viewport      viewport.Model
	ready         bool

	tagPicker components.TagPicker

	// now lets tests replace time.Now for deterministic dwell calculations.
	now func() time.Time
}

// NewModel returns a metrics model with default styles.
func NewModel(client *azdevops.MultiClient, cfg *config.Config) Model {
	return NewModelWithStyles(client, cfg, styles.DefaultStyles())
}

// NewModelWithStyles returns a metrics model with the provided styles. Pass
// nil styles to skip the picker creation (used by tests).
func NewModelWithStyles(client *azdevops.MultiClient, cfg *config.Config, s *styles.Styles) Model {
	m := Model{
		client: client,
		config: cfg,
		styles: s,
		now:    time.Now,
	}
	if s != nil {
		m.tagPicker = components.NewTagPicker(s)
	}
	return m
}

// SetStyles swaps the active styles in place, preserving all loaded data and
// view state (snapshots, selected sprints, fetched rows, current mode). The app
// calls this on theme change instead of reconstructing the model, which would
// erase the metrics section. The tag picker is rebuilt with the new styles (its
// tags/selection are repopulated from the model whenever it's shown).
func (m *Model) SetStyles(s *styles.Styles) {
	m.styles = s
	if s != nil {
		m.tagPicker = components.NewTagPicker(s)
		if m.width > 0 && m.height > 0 {
			m.tagPicker.SetSize(m.width, m.height)
		}
	}
	if m.ready {
		m.updateViewportContent()
	}
}

// stateConfig pulls the configured workflow state names out of the model's
// config and produces a coremetrics.StateConfig. Falls back to defaults if
// the config block hasn't been populated (test fixtures, no-config paths).
func (m Model) stateConfig() coremetrics.StateConfig {
	if m.config == nil {
		return coremetrics.DefaultStates()
	}
	st := m.config.Metrics.States
	if st.Active == "" || st.ReadyForTest == "" || st.Closed == "" {
		return coremetrics.DefaultStates()
	}
	return coremetrics.StateConfig{
		Active:       st.Active,
		ReadyForTest: st.ReadyForTest,
		Closed:       st.Closed,
	}
}

// Init kicks off the initial live fetch and loads any persisted snapshot data
// + sprint selection in parallel. If the user has opted into the one-shot
// /updates backfill, that cmd is dispatched alongside (a marker file inside
// the cmd short-circuits it on subsequent launches).
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.fetch(), loadSnapshotsCmd()}
	if m.config != nil && m.config.Metrics.RunOneShotBackfill {
		cmds = append(cmds, runBackfillCmd(m.client, m.now(), m.stateConfig()))
	}
	return tea.Batch(cmds...)
}

// Update handles incoming messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resizeViewport()
		m.updateViewportContent()
		return m, nil

	case metricsLoadedMsg:
		m.loading = false
		m.lastUpdated = msg.fetchedAt
		if msg.err != nil {
			var pe *azdevops.PartialError
			if errors.As(msg.err, &pe) {
				m.allItems = msg.items
				m.recompute()
				m.statusMessage = fmt.Sprintf(
					"%d of %d projects failed — partial data shown",
					pe.Failed, pe.Total,
				)
				return m, saveSnapshotCmd(m.client, msg.items, m.now(), m.stateConfig())
			}
			m.allItems = nil
			m.userRows = nil
			m.flags = nil
			m.statusMessage = "Failed to load metrics: " + msg.err.Error()
			return m, nil
		}
		m.allItems = msg.items
		m.statusMessage = ""
		m.recompute()
		return m, saveSnapshotCmd(m.client, msg.items, m.now(), m.stateConfig())

	case snapshotSavedMsg:
		switch {
		case msg.err != nil:
			m.statusMessage = "Snapshot save failed: " + msg.err.Error()
			return m, nil
		case msg.alreadyToday:
			// Quiet: don't overwrite a successful "updated" message.
			if m.statusMessage == "" {
				m.statusMessage = "Snapshot skipped (already saved today)"
			}
			return m, nil
		case msg.skipped > 0:
			m.statusMessage = fmt.Sprintf("Snapshot saved · %d rows, %d items couldn't be backfilled", msg.saved, msg.skipped)
		default:
			m.statusMessage = fmt.Sprintf("Snapshot saved · %d rows appended", msg.saved)
		}
		// Reload from disk so the Trends view reflects the new rows.
		return m, loadSnapshotsCmd()

	case backfillDoneMsg:
		switch {
		case msg.err != nil:
			m.statusMessage = "Backfill failed: " + msg.err.Error()
			return m, nil
		case msg.alreadyDone:
			// Quiet: backfill already ran on a previous launch.
			return m, nil
		default:
			m.statusMessage = fmt.Sprintf(
				"Backfill complete · %d rows from %d items, %d skipped — set run_one_shot_backfill: false to stop seeing this",
				msg.saved, msg.total, msg.skipped,
			)
			// Reload so Trends picks up the synthesized rows.
			return m, loadSnapshotsCmd()
		}

	case snapshotsLoadedMsg:
		if msg.err != nil {
			// Don't disrupt the live view — trends just stays empty.
			return m, nil
		}
		m.snapshots = msg.snaps
		m.availableSprints = collectUniqueTagsFromSnaps(msg.snaps)
		// Only adopt the persisted selection on first load (when the
		// in-session selection is still empty). After that, user picks win.
		if m.selectedSprints == nil {
			m.selectedSprints = coremetrics.FilterAvailable(msg.selected, m.availableSprints)
		} else {
			// Existing in-session selection might reference tags newly added
			// or removed; refilter against the freshly loaded snapshot.
			m.selectedSprints = coremetrics.FilterAvailable(m.selectedSprints, m.availableSprints)
		}
		m.recomputeTrends()
		if m.mode.isTrendsLike() {
			m.updateViewportContent()
		}
		return m, nil

	case components.TagsSelectedMsg:
		m.selectedSprints = append([]string(nil), msg.Tags...)
		m.tagPicker.Hide()
		m.recomputeTrends()
		if m.mode.isTrendsLike() {
			m.updateViewportContent()
		}
		return m, saveSelectionCmd(m.selectedSprints)

	case openURLResultMsg:
		if msg.err != nil {
			m.statusMessage = "Open in browser failed: " + msg.err.Error()
		}
		return m, nil

	case components.TagSelectedMsg:
		m.activeTag = msg.Tag
		if m.tagPicker.IsVisible() {
			m.tagPicker.Hide()
		}
		m.recompute()
		return m, nil
	}

	// Tag picker swallows key events while visible.
	if m.tagPicker.IsVisible() {
		if kmsg, ok := msg.(tea.KeyMsg); ok {
			var cmd tea.Cmd
			m.tagPicker, cmd = m.tagPicker.Update(kmsg)
			return m, cmd
		}
		return m, nil
	}

	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch keyMsg.Type {
	case tea.KeyTab:
		// Pane focus only matters in Live mode.
		if m.mode.isTrendsLike() {
			return m, nil
		}
		if m.focusedPane == paneFlags {
			m.focusedPane = paneUsers
		} else {
			m.focusedPane = paneFlags
		}
		m.updateViewportContent()
		m.scrollCursorIntoView()
		return m, nil
	case tea.KeyUp:
		if m.mode.isTrendsLike() {
			if m.ready {
				m.viewport.LineUp(1)
			}
			return m, nil
		}
		m.moveCursor(-1)
		m.updateViewportContent()
		m.scrollCursorIntoView()
		return m, nil
	case tea.KeyDown:
		if m.mode.isTrendsLike() {
			if m.ready {
				m.viewport.LineDown(1)
			}
			return m, nil
		}
		m.moveCursor(1)
		m.updateViewportContent()
		m.scrollCursorIntoView()
		return m, nil
	case tea.KeyPgUp:
		if m.ready {
			m.viewport.LineUp(m.viewport.Height)
		}
		return m, nil
	case tea.KeyPgDown:
		if m.ready {
			m.viewport.LineDown(m.viewport.Height)
		}
		return m, nil
	case tea.KeyEsc:
		// Esc in Live mode clears the tag filter. In Trends mode it's a no-op
		// — clearing the sprint selection should be deliberate (open picker).
		if m.mode == viewLive && m.activeTag != "" {
			m.activeTag = ""
			m.recompute()
		}
		return m, nil
	}

	switch keyMsg.String() {
	case "r":
		m.loading = true
		m.statusMessage = ""
		return m, tea.Batch(m.fetch(), loadSnapshotsCmd())
	case "v":
		// Cycle Live → Trends (table) → Trends (chart) → Live.
		switch m.mode {
		case viewLive:
			m.mode = viewTrends
		case viewTrends:
			m.mode = viewTrendsChart
		default:
			m.mode = viewLive
		}
		m.updateViewportContent()
		if m.ready {
			m.viewport.SetYOffset(0)
		}
		return m, nil
	case "f":
		// Flag filter is Live-only.
		if m.mode.isTrendsLike() {
			return m, nil
		}
		m.flagFilter = (m.flagFilter + 1) % 3
		m.flagCursor = 0
		m.updateViewportContent()
		m.scrollCursorIntoView()
		return m, nil
	case "T":
		if m.styles == nil {
			return m, nil
		}
		if m.mode.isTrendsLike() {
			m.tagPicker.SetTagsMulti(m.availableSprints, m.selectedSprints)
		} else {
			tags := collectUniqueTags(m.allItems)
			m.tagPicker.SetTags(tags, m.activeTag)
		}
		m.tagPicker.Show()
		return m, nil
	case "o":
		// Open-in-browser is Live-only (no focused item in Trends).
		if m.mode.isTrendsLike() {
			return m, nil
		}
		return m.openFocused()
	case "h", "l", ",", ".", "n", "p", "a":
		// Chart-only navigation. The arrow/number keys are intercepted by the
		// app shell for tab switching, so the chart uses letters/punctuation.
		if m.mode != viewTrendsChart {
			return m, nil
		}
		m.handleChartKey(keyMsg.String())
		m.updateViewportContent()
		return m, nil
	}

	return m, nil
}

// handleChartKey applies a chart-mode navigation key, mutating the chart cursor
// state in place. Callers re-render afterwards.
func (m *Model) handleChartKey(k string) {
	switch k {
	case "l":
		m.chartMetric = nextMetric(m.chartMetric, 1)
	case "h":
		m.chartMetric = nextMetric(m.chartMetric, -1)
	case ".":
		if n := len(m.sprintWindows); n > 0 {
			m.sprintCursor = clamp(m.sprintCursor+1, 0, n-1)
		}
	case ",":
		if n := len(m.sprintWindows); n > 0 {
			m.sprintCursor = clamp(m.sprintCursor-1, 0, n-1)
		}
	}
}

// nextMetric advances the metric selection by delta, wrapping around the four
// metrics.
func nextMetric(cur coremetrics.MetricKind, delta int) coremetrics.MetricKind {
	n := len(coremetrics.AllMetricKinds)
	idx := (int(cur) + delta + n) % n
	return coremetrics.AllMetricKinds[idx]
}

// fetch returns a tea.Cmd that performs the metrics fetch.
func (m Model) fetch() tea.Cmd {
	if m.client == nil {
		return nil
	}
	intervalDays := m.config.Metrics.IntervalDays
	if intervalDays <= 0 {
		intervalDays = config.DefaultMetricsIntervalDays
	}
	since := m.now().AddDate(0, 0, -intervalDays)
	client := m.client
	now := m.now
	states := toMetricsStateNames(m.stateConfig())
	return func() tea.Msg {
		items, err := client.MetricsWorkItems(since, states)
		return metricsLoadedMsg{items: items, err: err, fetchedAt: now()}
	}
}

// toMetricsStateNames converts the core StateConfig into the azdevops layer's
// state-name struct. The two structs are intentionally separate so the API
// client doesn't import the metrics package.
func toMetricsStateNames(sc coremetrics.StateConfig) azdevops.MetricsStateNames {
	return azdevops.MetricsStateNames{
		Active:       sc.Active,
		ReadyForTest: sc.ReadyForTest,
		Closed:       sc.Closed,
	}
}

// recomputeTrends rebuilds sprintWindows + trendRows from the current
// selection. Called whenever snapshots or selection change.
func (m *Model) recomputeTrends() {
	if len(m.selectedSprints) == 0 {
		m.sprintWindows = nil
		m.trendRows = nil
		m.sprintCursor = 0
		return
	}
	var windows []coremetrics.SprintWindow
	for _, tag := range m.selectedSprints {
		w, ok := coremetrics.DeriveSprintWindow(m.snapshots, tag, m.now(), m.stateConfig())
		if !ok {
			continue
		}
		windows = append(windows, w)
	}
	m.sprintWindows = windows
	m.trendRows = coremetrics.TrendAggregate(m.snapshots, windows, coremetrics.Thresholds{
		ActiveStaleDays: m.config.Metrics.ActiveStaleDays,
		RFTStaleDays:    m.config.Metrics.RFTStaleDays,
		WIPLimit:        m.config.Metrics.WIPLimit,
		States:          m.stateConfig(),
	}, m.now())

	// Keep the chart cursor within the new bounds.
	if m.sprintCursor >= len(m.sprintWindows) {
		m.sprintCursor = 0
	}
}

// collectUniqueTagsFromSnaps returns the sorted set of tags across the
// snapshot file. Differs from collectUniqueTags(items) — that one only
// surfaces tags on currently in-flight items, which would prevent the
// user from picking retired sprints.
func collectUniqueTagsFromSnaps(snaps []coremetrics.Snapshot) []string {
	seen := make(map[string]struct{})
	for _, s := range snaps {
		for _, t := range s.Tags {
			seen[t] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for t := range seen {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// recompute re-runs Aggregate on the currently filtered base set.
func (m *Model) recompute() {
	now := m.now()
	intervalStart := now.AddDate(0, 0, -m.config.Metrics.IntervalDays)
	filtered := applyTagFilter(m.allItems, m.activeTag)
	rows, flags := coremetrics.Aggregate(filtered, intervalStart, now, coremetrics.Thresholds{
		ActiveStaleDays: m.config.Metrics.ActiveStaleDays,
		RFTStaleDays:    m.config.Metrics.RFTStaleDays,
		WIPLimit:        m.config.Metrics.WIPLimit,
		States:          m.stateConfig(),
	})
	m.userRows = rows
	m.flags = flags
	if m.userCursor >= len(rows) {
		m.userCursor = 0
	}
	if m.flagCursor >= len(flags) {
		m.flagCursor = 0
	}
	m.updateViewportContent()
}

// resizeViewport rebuilds the viewport whenever the available area changes.
// The header (1 line) and the blank between header and body (1 line) live
// outside the viewport so they stay anchored.
func (m *Model) resizeViewport() {
	const reservedRows = 2 // header + blank
	h := m.height - reservedRows
	if h < 1 {
		h = 1
	}
	w := m.width
	if w < 1 {
		w = 1
	}
	if !m.ready {
		m.viewport = viewport.New(w, h)
		m.ready = true
		return
	}
	m.viewport.Width = w
	m.viewport.Height = h
}

// updateViewportContent rebuilds the viewport's rendered body. Called whenever
// data, focus, filters, mode, or cursors change so the visible body stays in
// sync.
func (m *Model) updateViewportContent() {
	if !m.ready {
		return
	}
	var body string
	switch m.mode {
	case viewTrends:
		body = m.renderTrends()
	case viewTrendsChart:
		body = m.renderTrendsChart()
	default:
		body = lipgloss.JoinVertical(lipgloss.Left, m.renderFlagsPane(), "", m.renderUsersPane())
	}
	m.viewport.SetContent(body)
}

// scrollCursorIntoView nudges the viewport so the focused row stays visible.
// Called after every cursor move.
func (m *Model) scrollCursorIntoView() {
	if !m.ready {
		return
	}
	line := m.cursorLineInBody()
	top := m.viewport.YOffset
	bottom := top + m.viewport.Height - 1
	switch {
	case line < top:
		m.viewport.SetYOffset(line)
	case line > bottom:
		m.viewport.SetYOffset(line - m.viewport.Height + 1)
	}
}

// cursorLineInBody returns the 0-indexed line number (within the viewport
// content) of the currently focused row. Mirrors the layout produced by
// updateViewportContent: flags pane, blank, users pane.
func (m Model) cursorLineInBody() int {
	flagsRows := len(m.visibleFlags())
	if flagsRows == 0 {
		flagsRows = 1 // "(no flagged items)"
	}
	switch m.focusedPane {
	case paneFlags:
		// flags pane: line 0 = title, line 1..N = rows
		return 1 + m.flagCursor
	case paneUsers:
		// flags pane height = 1 (title) + flagsRows
		// + 1 blank between panes
		// users pane: line 0 = title, line 1 = column header, line 2..M = rows
		usersStart := 1 + flagsRows + 1
		return usersStart + 2 + m.userCursor
	}
	return 0
}

// visibleFlags returns the flag slice filtered by the active flag filter.
func (m Model) visibleFlags() []coremetrics.ItemFlag {
	switch m.flagFilter {
	case flagFilterActiveStale:
		return filterFlagsByReason(m.flags, "active-stale")
	case flagFilterRFTStale:
		return filterFlagsByReason(m.flags, "rft-stale")
	default:
		return m.flags
	}
}

func filterFlagsByReason(flags []coremetrics.ItemFlag, reason string) []coremetrics.ItemFlag {
	out := flags[:0:0]
	for _, f := range flags {
		if f.Reason == reason {
			out = append(out, f)
		}
	}
	return out
}

func (m *Model) moveCursor(delta int) {
	switch m.focusedPane {
	case paneFlags:
		n := len(m.visibleFlags())
		if n == 0 {
			m.flagCursor = 0
			return
		}
		m.flagCursor = clamp(m.flagCursor+delta, 0, n-1)
	case paneUsers:
		n := len(m.userRows)
		if n == 0 {
			m.userCursor = 0
			return
		}
		m.userCursor = clamp(m.userCursor+delta, 0, n-1)
	}
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// openFocused opens the URL for the focused row in the browser.
func (m Model) openFocused() (Model, tea.Cmd) {
	if m.client == nil {
		m.statusMessage = "Cannot open: no Azure DevOps client"
		return m, nil
	}
	org := m.client.GetOrg()

	var url string
	switch m.focusedPane {
	case paneFlags:
		vis := m.visibleFlags()
		if len(vis) == 0 || m.flagCursor >= len(vis) {
			return m, nil
		}
		f := vis[m.flagCursor]
		project := projectAPINameFor(m.allItems, f.ID, f.Project)
		url = buildWorkItemURL(org, project, f.ID)
	case paneUsers:
		if len(m.userRows) == 0 || m.userCursor >= len(m.userRows) {
			return m, nil
		}
		user := m.userRows[m.userCursor].User
		// Pick the focused user's worst stalled item; fall back to any in-flight.
		item, ok := worstItemForUser(m.allItems, user, m.now(), m.config.Metrics, m.stateConfig())
		if !ok {
			m.statusMessage = "No openable item for " + user
			return m, nil
		}
		url = buildWorkItemURL(org, item.ProjectName, item.ID)
	}
	if url == "" {
		m.statusMessage = "Cannot open: missing organization or project"
		return m, nil
	}
	return m, func() tea.Msg {
		return openURLResultMsg{err: openURL(url)}
	}
}

// projectAPINameFor finds the API project name for a given work item ID.
// Falls back to the supplied display name if no match is found.
func projectAPINameFor(items []azdevops.WorkItem, id int, fallback string) string {
	for i := range items {
		if items[i].ID == id {
			return items[i].ProjectName
		}
	}
	return fallback
}

// worstItemForUser returns the worst-stalled (highest dwell) item belonging to
// `user`. Prefers items past the configured thresholds; falls back to any
// in-flight item.
func worstItemForUser(items []azdevops.WorkItem, user string, now time.Time, mc config.MetricsConfig, states coremetrics.StateConfig) (azdevops.WorkItem, bool) {
	activeStale := time.Duration(mc.ActiveStaleDays) * 24 * time.Hour
	rftStale := time.Duration(mc.RFTStaleDays) * 24 * time.Hour
	var bestStale, bestInFlight azdevops.WorkItem
	var bestStaleDwell, bestInFlightDwell time.Duration
	haveStale, haveInFlight := false, false
	for _, wi := range items {
		if wi.AssignedToName() != user {
			continue
		}
		dwell := wi.TimeInCurrentState(now)
		isActive := states.IsActive(wi.Fields.State)
		isRFT := states.IsRFT(wi.Fields.State)
		if !isActive && !isRFT {
			continue
		}
		if !haveInFlight || dwell > bestInFlightDwell {
			bestInFlight = wi
			bestInFlightDwell = dwell
			haveInFlight = true
		}
		isStale := (isActive && dwell > activeStale) || (isRFT && dwell > rftStale)
		if isStale && (!haveStale || dwell > bestStaleDwell) {
			bestStale = wi
			bestStaleDwell = dwell
			haveStale = true
		}
	}
	if haveStale {
		return bestStale, true
	}
	if haveInFlight {
		return bestInFlight, true
	}
	return azdevops.WorkItem{}, false
}

// applyTagFilter mirrors the work-items pane filter — exact-match on
// individual tags (parsed via TagList()).
func applyTagFilter(items []azdevops.WorkItem, tag string) []azdevops.WorkItem {
	if tag == "" {
		return items
	}
	var filtered []azdevops.WorkItem
	for _, wi := range items {
		for _, t := range wi.TagList() {
			if t == tag {
				filtered = append(filtered, wi)
				break
			}
		}
	}
	return filtered
}

// collectUniqueTags returns the sorted set of tags across the items.
func collectUniqueTags(items []azdevops.WorkItem) []string {
	seen := make(map[string]struct{})
	for i := range items {
		for _, tag := range items[i].TagList() {
			seen[tag] = struct{}{}
		}
	}
	tags := make([]string, 0, len(seen))
	for tag := range seen {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	return tags
}

// buildWorkItemURL constructs the Azure DevOps URL to view a work item.
func buildWorkItemURL(org, project string, id int) string {
	if org == "" || project == "" {
		return ""
	}
	return fmt.Sprintf("https://dev.azure.com/%s/%s/_workitems/edit/%d", org, project, id)
}

// View renders the metrics dashboard.
func (m Model) View() string {
	if m.loading && len(m.userRows) == 0 && m.statusMessage == "" && m.mode == viewLive {
		return m.renderLoading()
	}

	header := m.renderHeader()
	if !m.ready {
		// No window size yet — fall back to inline rendering.
		switch m.mode {
		case viewTrends:
			return header + "\n\n" + m.renderTrends()
		case viewTrendsChart:
			return header + "\n\n" + m.renderTrendsChart()
		}
		flagsPane := m.renderFlagsPane()
		userPane := m.renderUsersPane()
		parts := []string{header, "", flagsPane, "", userPane}
		return lipgloss.JoinVertical(lipgloss.Left, parts...)
	}
	return header + "\n\n" + m.viewport.View()
}

func (m Model) renderLoading() string {
	msg := "Loading metrics…"
	if m.styles != nil {
		return m.styles.Muted.Render(msg)
	}
	return msg
}

func (m Model) renderHeader() string {
	mc := m.config.Metrics
	parts := []string{"Metrics"}
	if m.mode == viewTrendsChart {
		parts = append(parts, "Trends (chart)")
	} else if m.mode == viewTrends {
		parts = append(parts, "Trends")
	} else {
		parts = append(parts, "Live")
		if m.activeTag != "" {
			parts = append(parts, "Tag: "+m.activeTag)
		}
		_, rftLbl, _ := m.stateLabels()
		parts = append(parts,
			fmt.Sprintf("Interval %dd", mc.IntervalDays),
			fmt.Sprintf("Active-stale >%dd", mc.ActiveStaleDays),
			fmt.Sprintf("%s-stale >%dd", titleCase(rftLbl), mc.RFTStaleDays),
		)
	}
	parts = append(parts, "Updated "+m.lastUpdatedLabel())
	if m.mode == viewLive {
		_, rftLbl, _ := m.stateLabels()
		switch m.flagFilter {
		case flagFilterActiveStale:
			parts = append(parts, "Filter: Active-stale")
		case flagFilterRFTStale:
			parts = append(parts, "Filter: "+titleCase(rftLbl)+"-stale")
		}
	}
	line := strings.Join(parts, " · ")
	if m.styles != nil {
		return m.styles.Header.Render(line)
	}
	return line
}

func (m Model) lastUpdatedLabel() string {
	if m.lastUpdated.IsZero() {
		return "never"
	}
	d := m.now().Sub(m.lastUpdated)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

func (m Model) renderFlagsPane() string {
	vis := m.visibleFlags()
	title := fmt.Sprintf("⚠  Stuck items (%d)", len(vis))
	if m.styles != nil && m.focusedPane == paneFlags {
		title = m.styles.Warning.Render(title) + m.styles.Muted.Render("  [focused]")
	} else if m.styles != nil {
		title = m.styles.Warning.Render(title)
	}

	if len(vis) == 0 {
		body := padCol("  (no flagged items)", flagCursorW+flagIDW+1+flagStateW+1+flagDwellW+1+flagUserW+1+flagProjectW+1+flagTitleW)
		if m.styles != nil {
			body = m.styles.Muted.Render(body)
		}
		return title + "\n" + body
	}

	var b strings.Builder
	b.WriteString(title)
	b.WriteString("\n")
	for i, f := range vis {
		cursor := padCol("  ", flagCursorW)
		if m.focusedPane == paneFlags && i == m.flagCursor {
			cursor = padCol("> ", flagCursorW)
		}
		row := cursor +
			padCol(fmt.Sprintf("#%d", f.ID), flagIDW) + " " +
			padCol(m.shortenState(f.State), flagStateW) + " " +
			padCol(fmtDwell(f.Dwell), flagDwellW) + " " +
			padCol(f.User, flagUserW) + " " +
			padCol(f.Project, flagProjectW) + " " +
			padCol(f.Title, flagTitleW)
		if m.styles != nil && m.focusedPane == paneFlags && i == m.flagCursor {
			row = m.styles.Selected.Render(row)
		} else if m.styles != nil {
			row = m.styles.Error.Render(row)
		}
		b.WriteString(row)
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) renderUsersPane() string {
	title := fmt.Sprintf("Per developer (sorted by stalled, then in-flight)  —  %d", len(m.userRows))
	if m.styles != nil && m.focusedPane == paneUsers {
		title = m.styles.Header.Render(title) + m.styles.Muted.Render("  [focused]")
	} else if m.styles != nil {
		title = m.styles.Header.Render(title)
	}

	totalW := userCursorW + userNameW + 1 + userInFlightW + 1 + userActiveW + 1 + userRFTW + 1 +
		userOldActiveW + 1 + userOldRFTW + 1 + userClosedPtsW + 1 + userStalledW

	if len(m.userRows) == 0 {
		body := padCol("  (no in-flight items)", totalW)
		if m.styles != nil {
			body = m.styles.Muted.Render(body)
		}
		return title + "\n" + body
	}

	activeLbl, rftLbl, _ := m.stateLabels()
	activeTitle := titleCase(activeLbl)
	rftTitle := titleCase(rftLbl)
	header := padCol("  ", userCursorW) +
		padCol("User", userNameW) + " " +
		padCol("In-flight", userInFlightW) + " " +
		padCol(activeTitle, userActiveW) + " " +
		padCol(rftTitle, userRFTW) + " " +
		padCol("Old-"+activeTitle, userOldActiveW) + " " +
		padCol("Old-"+rftTitle, userOldRFTW) + " " +
		padCol("Closed-pts", userClosedPtsW) + " " +
		padCol("⚠", userStalledW)
	if m.styles != nil {
		header = m.styles.Muted.Render(header)
	}

	var b strings.Builder
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(header)
	b.WriteString("\n")

	for i, r := range m.userRows {
		cursor := padCol("  ", userCursorW)
		if m.focusedPane == paneUsers && i == m.userCursor {
			cursor = padCol("> ", userCursorW)
		}
		inFlight := strconv.Itoa(r.InFlight)
		if r.Overloaded {
			inFlight += " ⚠"
		}
		row := cursor +
			padCol(r.User, userNameW) + " " +
			padCol(inFlight, userInFlightW) + " " +
			padCol(strconv.Itoa(r.ActiveCount), userActiveW) + " " +
			padCol(strconv.Itoa(r.RFTCount), userRFTW) + " " +
			padCol(fmtDwell(r.OldestActive), userOldActiveW) + " " +
			padCol(fmtDwell(r.OldestRFT), userOldRFTW) + " " +
			padCol(fmtPoints(r.PointsClosed), userClosedPtsW) + " " +
			padCol(strconv.Itoa(r.Stalled), userStalledW)
		if m.styles != nil && m.focusedPane == paneUsers && i == m.userCursor {
			row = m.styles.Selected.Render(row)
		} else if m.styles != nil && r.Stalled > 0 {
			row = m.styles.Warning.Render(row)
		} else if m.styles != nil {
			row = m.styles.Value.Render(row)
		}
		b.WriteString(row)
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// shortenState abbreviates a state name for the flag rows so the
// flagStateW column ("Active" / "RFT" sized) is enough to fit any
// configured label. Uses the configured Active/RFT/Closed labels with
// title-case applied.
func (m Model) shortenState(s string) string {
	sc := m.stateConfig()
	activeLbl, rftLbl, closedLbl := m.stateLabels()
	switch {
	case sc.IsActive(s):
		return titleCase(activeLbl)
	case sc.IsRFT(s):
		return titleCase(rftLbl)
	case sc.IsClosed(s):
		return titleCase(closedLbl)
	default:
		return s
	}
}

// fmtDwell turns a duration into a compact "Nd" / "Nh" / "Nm" string.
func fmtDwell(d time.Duration) string {
	if d <= 0 {
		return "—"
	}
	switch {
	case d >= 24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	case d >= time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
}

func fmtPoints(p float64) string {
	if p == 0 {
		return "—"
	}
	if p == float64(int64(p)) {
		return strconv.FormatInt(int64(p), 10)
	}
	return strconv.FormatFloat(p, 'f', 1, 64)
}

// IsSearching satisfies the active-view-capturing-input contract — metrics
// has no search input today, but tag-picker visibility counts.
func (m Model) IsSearching() bool {
	return false
}

// HasContextBar tells the app shell whether to surface a context bar instead
// of the default keybindings line. The metrics tab is a single screen with
// fixed key hints, so we don't claim the context bar.
func (m Model) HasContextBar() bool {
	return false
}

// GetContextItems is required by the parent shell, but never used (see
// HasContextBar above).
func (m Model) GetContextItems() []components.ContextItem {
	return nil
}

// GetScrollPercent returns the body viewport scroll percentage.
func (m Model) GetScrollPercent() float64 {
	if !m.ready {
		return 0
	}
	return m.viewport.ScrollPercent() * 100
}

// GetStatusMessage surfaces the most recent transient message.
func (m Model) GetStatusMessage() string {
	return m.statusMessage
}

// Tag-picker glue — mirrors the work-items tab API.

// IsTagPickerVisible reports whether the tag picker overlay is open.
func (m Model) IsTagPickerVisible() bool {
	return m.tagPicker.IsVisible()
}

// TagPickerView returns the rendered tag picker overlay.
func (m Model) TagPickerView() string {
	return m.tagPicker.View()
}

// SetTagPickerSize sets the dimensions for the tag picker overlay.
func (m *Model) SetTagPickerSize(width, height int) {
	m.tagPicker.SetSize(width, height)
}

// IsTagFilterActive reports whether a tag filter is currently applied.
func (m Model) IsTagFilterActive() bool {
	return m.activeTag != ""
}

// ActiveTag returns the currently active tag filter, or "" if none.
func (m Model) ActiveTag() string {
	return m.activeTag
}
