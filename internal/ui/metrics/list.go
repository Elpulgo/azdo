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

// Init kicks off the initial fetch.
func (m Model) Init() tea.Cmd {
	return m.fetch()
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
				return m, nil
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
		return m, nil

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
		if m.focusedPane == paneFlags {
			m.focusedPane = paneUsers
		} else {
			m.focusedPane = paneFlags
		}
		m.updateViewportContent()
		m.scrollCursorIntoView()
		return m, nil
	case tea.KeyUp:
		m.moveCursor(-1)
		m.updateViewportContent()
		m.scrollCursorIntoView()
		return m, nil
	case tea.KeyDown:
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
		if m.activeTag != "" {
			m.activeTag = ""
			m.recompute()
		}
		return m, nil
	}

	switch keyMsg.String() {
	case "r":
		m.loading = true
		m.statusMessage = ""
		return m, m.fetch()
	case "f":
		m.flagFilter = (m.flagFilter + 1) % 3
		m.flagCursor = 0
		m.updateViewportContent()
		m.scrollCursorIntoView()
		return m, nil
	case "T":
		if m.styles == nil {
			return m, nil
		}
		tags := collectUniqueTags(m.allItems)
		m.tagPicker.SetTags(tags, m.activeTag)
		m.tagPicker.Show()
		return m, nil
	case "o":
		return m.openFocused()
	}

	return m, nil
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
	return func() tea.Msg {
		items, err := client.MetricsWorkItems(since)
		return metricsLoadedMsg{items: items, err: err, fetchedAt: now()}
	}
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
// data, focus, filters, or cursors change so the visible body stays in sync.
func (m *Model) updateViewportContent() {
	if !m.ready {
		return
	}
	body := lipgloss.JoinVertical(lipgloss.Left, m.renderFlagsPane(), "", m.renderUsersPane())
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
		item, ok := worstItemForUser(m.allItems, user, m.now(), m.config.Metrics)
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
func worstItemForUser(items []azdevops.WorkItem, user string, now time.Time, mc config.MetricsConfig) (azdevops.WorkItem, bool) {
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
		state := strings.ToLower(wi.Fields.State)
		isInFlight := state == "active" || state == "ready for test"
		if !isInFlight {
			continue
		}
		if !haveInFlight || dwell > bestInFlightDwell {
			bestInFlight = wi
			bestInFlightDwell = dwell
			haveInFlight = true
		}
		isStale := (state == "active" && dwell > activeStale) ||
			(state == "ready for test" && dwell > rftStale)
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
	if m.loading && len(m.userRows) == 0 && m.statusMessage == "" {
		return m.renderLoading()
	}

	header := m.renderHeader()
	if !m.ready {
		// No window size yet — fall back to inline rendering.
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
	if m.activeTag != "" {
		parts = append(parts, "Tag: "+m.activeTag)
	}
	parts = append(parts,
		fmt.Sprintf("Interval %dd", mc.IntervalDays),
		fmt.Sprintf("Active-stale >%dd", mc.ActiveStaleDays),
		fmt.Sprintf("RFT-stale >%dd", mc.RFTStaleDays),
		"Updated "+m.lastUpdatedLabel(),
	)
	switch m.flagFilter {
	case flagFilterActiveStale:
		parts = append(parts, "Filter: Active-stale")
	case flagFilterRFTStale:
		parts = append(parts, "Filter: RFT-stale")
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
			padCol(shortenState(f.State), flagStateW) + " " +
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

	header := padCol("  ", userCursorW) +
		padCol("User", userNameW) + " " +
		padCol("In-flight", userInFlightW) + " " +
		padCol("Active", userActiveW) + " " +
		padCol("RFT", userRFTW) + " " +
		padCol("Old-Active", userOldActiveW) + " " +
		padCol("Old-RFT", userOldRFTW) + " " +
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

// shortenState abbreviates the canonical state names for the flag rows.
func shortenState(s string) string {
	switch strings.ToLower(s) {
	case "ready for test":
		return "RFT"
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
