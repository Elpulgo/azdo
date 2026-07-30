package notifications

import (
	"time"

	"github.com/Elpulgo/azdo/internal/provider"
	"github.com/Elpulgo/azdo/internal/ui/components/listview"
	"github.com/Elpulgo/azdo/internal/ui/components/table"
	"github.com/Elpulgo/azdo/internal/ui/display"
	"github.com/Elpulgo/azdo/internal/ui/styles"
	tea "github.com/charmbracelet/bubbletea"
)

// Model is the notifications pane: a listview.Model[provider.Notification]
// plus the interactive `f` reason-filter cycle (Decisions 53, 54).
//
// The pane holds two layers over the merged feed:
//   - feed: the config-filtered rows (task 10's FilterNotifications output),
//     set wholesale by the caller via SetFeed whenever a fetch/poll lands.
//   - the `f` cycle, which narrows feed further to a single NotificationReason
//     (or "all", the default/reset position).
//
// Per decision 54, the narrowed result is always handed to listview via
// SetItems — never a second, independently-narrower slice fed to ToRows
// alone — so the column/row-cell invariant convention 7 relies on never
// diverges (convention 8: a divergence surfaces as a table.renderRow panic,
// not a failed assertion).
type Model struct {
	list   listview.Model[provider.Notification]
	styles *styles.Styles

	// feed is the config-filtered inbox, set by SetFeed. It is never itself
	// narrowed by the `f` cycle — reasonFiltered() derives a fresh slice from
	// it on every cycle step and every SetFeed call, so re-cycling never
	// needs a refetch.
	feed []provider.Notification

	// reasonFilterActive and reasonFilter together encode the `f` cycle's
	// current position. reasonFilterActive == false means "all reasons" —
	// the reset/default position reachable by cycling alone (decision 53).
	reasonFilterActive bool
	reasonFilter       provider.NotificationReason
}

// baseColumns are the notifications list's per-row column specs, excluding
// the optional dynamic Repo column. Defined once and copied inside toColumns
// to avoid mutating the package-level slice (mirrors prBaseColumns).
var baseColumns = []listview.ColumnSpec{
	{Title: "Reason", WidthPct: 20, MinWidth: 10},
	{Title: "Title", WidthPct: 60, MinWidth: 20},
	{Title: "Updated", WidthPct: 20, MinWidth: 10},
}

// repoColumn is the dynamic column prepended when the item slice spans more
// than one distinct Identity.Scope (convention 7).
var repoColumn = listview.ColumnSpec{Title: "Repo", WidthPct: 20, MinWidth: 10}

// NewModel creates a new notifications pane model with default styles.
func NewModel() Model {
	return NewModelWithStyles(styles.DefaultStyles())
}

// NewModelWithStyles creates a new notifications pane model with custom
// styles. Fetching/polling wiring (the provider.NotificationSource client,
// the tab registration, the render states) is owned by later tasks (12, 13,
// 15) — this constructor builds a self-contained pane whose data enters
// exclusively through SetFeed.
func NewModelWithStyles(s *styles.Styles) Model {
	cfg := listview.Config[provider.Notification]{
		LoadingMessage: "Loading notifications...",
		EntityName:     "notifications",
		MinWidth:       50,
		ToRows:         toRows,
		ToColumns:      toColumns,
		// Fetching is wired by a later task (12/15); this pane's data enters
		// through SetFeed, so Init's fetch is a deliberate no-op rather than
		// nil (listview.Init calls Fetch() unconditionally).
		Fetch: func() tea.Cmd { return nil },
		// No detail view in phase 1 (`o` opens the browser per decision 3;
		// there is nothing else to drill into). A harmless no-op stub avoids
		// a nil-func panic if enter is pressed, without building real
		// navigation (out of scope for this task).
		EnterDetail: func(item provider.Notification, st *styles.Styles, w, h int) (listview.DetailView, tea.Cmd) {
			return nil, nil
		},
	}

	return Model{
		list:   listview.New(cfg, s),
		styles: s,
	}
}

// Init initializes the model.
func (m Model) Init() tea.Cmd {
	return m.list.Init()
}

// Update handles messages. The `f` key cycles the reason filter (decision
// 53) when the list is in its normal browsing state; every other message is
// forwarded to the underlying listview.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		if key.String() == "f" && !m.list.IsSearching() && m.list.GetViewMode() == listview.ViewList {
			return m.cycleReasonFilter(), nil
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// View renders the view.
func (m Model) View() string {
	return m.list.View()
}

// SetFeed sets the config-filtered feed (task 10's FilterNotifications
// output) and re-applies whatever `f` reason filter is currently active on
// top of it, routing the result through listview.SetItems per decision 54.
func (m Model) SetFeed(feed []provider.Notification) Model {
	m.feed = feed
	m.list = m.list.SetItems(m.reasonFiltered())
	return m
}

// cycleReasonFilter advances the `f` cycle by one step and re-applies it.
// The cycle visits reasons present in m.feed, in enum order (decision 53) —
// never feed order (which reshuffles every poll per decision 45) and never
// the full enum (which would offer Unknown, a value no mapped row can
// carry — decision 18). The "all reasons" position is the implicit reset:
// cycling past the last present reason returns to it, and an empty feed
// (or a feed with no reasons at all) also resets to it defensively.
func (m Model) cycleReasonFilter() Model {
	present := presentReasons(m.feed)

	switch {
	case len(present) == 0:
		m.reasonFilterActive = false
	case !m.reasonFilterActive:
		m.reasonFilterActive = true
		m.reasonFilter = present[0]
	default:
		idx := indexOfReason(present, m.reasonFilter)
		if idx < 0 || idx == len(present)-1 {
			m.reasonFilterActive = false
		} else {
			m.reasonFilter = present[idx+1]
		}
	}

	m.list = m.list.SetItems(m.reasonFiltered())
	return m
}

// reasonFiltered returns the rows visible under the current `f` position: the
// whole feed when no reason filter is active, otherwise only rows whose
// Reason matches. Always a freshly allocated slice when narrowing, so the
// pane's held feed is never aliased or mutated by a subsequent filter step.
func (m Model) reasonFiltered() []provider.Notification {
	if !m.reasonFilterActive {
		return m.feed
	}
	out := make([]provider.Notification, 0, len(m.feed))
	for _, n := range m.feed {
		if n.Reason == m.reasonFilter {
			out = append(out, n)
		}
	}
	return out
}

// presentReasons returns the NotificationReason values that appear in feed,
// in enum declaration order. NotificationReasonUnknown (the zero value) is
// always excluded: the wire mapper never emits it (Decision 18), and
// including it would offer a cycle stop that can never match a real row.
// Iterating provider.NotificationReasonCount() rather than a hardcoded slice
// means the cycle stays in sync automatically if the enum ever grows.
func presentReasons(feed []provider.Notification) []provider.NotificationReason {
	seen := make(map[provider.NotificationReason]bool, len(feed))
	for _, n := range feed {
		seen[n.Reason] = true
	}

	out := make([]provider.NotificationReason, 0, provider.NotificationReasonCount())
	for i := 1; i < provider.NotificationReasonCount(); i++ { // start at 1: skip Unknown (0)
		r := provider.NotificationReason(i)
		if seen[r] {
			out = append(out, r)
		}
	}
	return out
}

// indexOfReason returns the index of target in reasons, or -1 if absent.
func indexOfReason(reasons []provider.NotificationReason, target provider.NotificationReason) int {
	for i, r := range reasons {
		if r == target {
			return i
		}
	}
	return -1
}

// multiRepo reports whether items span more than one distinct
// Identity.Scope, via display.MultiScope. Both toRows and toColumns compute
// it from the exact same slice they're each called with, so the dynamic
// Repo column's presence and the per-row Repo cell can never diverge
// (convention 7).
func multiRepo(items []provider.Notification) bool {
	scopes := make([]string, len(items))
	for i, n := range items {
		scopes[i] = n.Identity.Scope
	}
	return display.MultiScope(scopes)
}

// toColumns derives the notifications list's column specs from the current
// items: [Repo?] [Reason] [Title] [Updated].
func toColumns(items []provider.Notification) []listview.ColumnSpec {
	cols := make([]listview.ColumnSpec, len(baseColumns))
	copy(cols, baseColumns)

	if multiRepo(items) {
		cols = append([]listview.ColumnSpec{repoColumn}, cols...)
	}

	listview.NormalizeWidths(cols)
	return cols
}

// toRows converts notifications to table rows, mirroring toColumns's layout
// and gating predicate exactly: [Repo?] [Reason] [Title] [Updated].
//
//   - The Repo cell (when present) falls back to "—" for an empty
//     ScopeDisplay: decision 35 defaults it to Scope at the adapter
//     boundary, but a thread whose repository payload is absent leaves both
//     empty, the one case the mapper cannot fix.
//   - The Title cell is rendered with the styles.Styles.Title named style
//     (bold) for unread rows, so unread emphasis is a named style rather
//     than an inline lipgloss.NewStyle() (convention 6).
//   - The Updated cell renders "—" for a zero UpdatedAt (the mapper leaves it
//     zero when the wire omits updated_at), never a year-0001 date.
func toRows(items []provider.Notification, s *styles.Styles) []table.Row {
	multi := multiRepo(items)

	rows := make([]table.Row, len(items))
	for i, n := range items {
		reasonCell := display.NotificationReasonStyle(n.Reason, s).
			Render(display.NotificationReasonGlyph(n.Reason) + " " + display.NotificationReasonLabel(n.Reason))

		cells := table.Row{
			reasonCell,
			titleCell(n, s),
			formatUpdatedAt(n.UpdatedAt),
		}
		if multi {
			cells = append(table.Row{dashIfEmpty(n.Identity.ScopeDisplay)}, cells...)
		}
		rows[i] = cells
	}
	return rows
}

// titleCell returns the Title column's cell text for a notification. Unread
// rows are rendered through styles.Styles.Title — a named style, never an
// inline lipgloss.NewStyle() (convention 6) — so unread emphasis is
// assertable against that exact named style rather than a substring of the
// rendered output. Read rows are plain, unstyled text.
func titleCell(n provider.Notification, s *styles.Styles) string {
	if !n.Read {
		return s.Title.Render(n.Title)
	}
	return n.Title
}

// dashIfEmpty returns "—" for an empty string, val otherwise.
func dashIfEmpty(val string) string {
	if val == "" {
		return "—"
	}
	return val
}

// formatUpdatedAt renders a notification's UpdatedAt for display. A zero
// time (the mapper's sentinel for "the wire omitted updated_at") renders as
// "—", never as a year-0001 date.
func formatUpdatedAt(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Format("2006-01-02 15:04")
}
