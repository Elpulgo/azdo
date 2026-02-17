package listview

import (
	"fmt"

	"github.com/Elpulgo/azdo/internal/ui/components"
	"github.com/Elpulgo/azdo/internal/ui/components/table"
	"github.com/Elpulgo/azdo/internal/ui/styles"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ViewMode represents the current view in a list UI.
type ViewMode int

const (
	ViewList   ViewMode = iota
	ViewDetail
)


// ColumnSpec defines a column with percentage-based width and minimum.
type ColumnSpec struct {
	Title    string
	WidthPct int
	MinWidth int
}

// DetailView is the interface that domain detail models must satisfy.
type DetailView interface {
	Update(msg tea.Msg) (DetailView, tea.Cmd)
	View() string
	SetSize(width, height int)
	GetContextItems() []components.ContextItem
	GetScrollPercent() float64
	GetStatusMessage() string
}

// Config holds domain-specific callbacks for a list view.
type Config[T any] struct {
	Columns        []ColumnSpec
	LoadingMessage string
	EntityName     string // e.g. "pipeline runs" — used in error/empty messages
	MinWidth       int    // minimum usable width for columns (default 70)
	ToRows         func(items []T, s *styles.Styles) []table.Row
	Fetch          func() tea.Cmd
	EnterDetail    func(item T, s *styles.Styles, w, h int) (DetailView, tea.Cmd)
	HasContextBar  func(mode ViewMode) bool // nil = always false
}

// Model is the generic list model.
type Model[T any] struct {
	table    table.Model
	items    []T
	loading  bool
	err      error
	width    int
	height   int
	viewMode ViewMode
	detail   DetailView
	spinner  *components.LoadingIndicator
	styles   *styles.Styles
	config   Config[T]
}

// New creates a new generic list model.
func New[T any](cfg Config[T], s *styles.Styles) Model[T] {
	minW := cfg.MinWidth
	if minW == 0 {
		minW = 70
	}

	columns := makeColumns(cfg.Columns, 80, minW)

	t := table.New(
		table.WithColumns(columns),
		table.WithFocused(true),
		table.WithHeight(10),
	)

	ts := table.DefaultStyles()
	ts.Header = s.TableHeader
	ts.Cell = s.TableCell
	ts.Selected = s.TableSelected
	t.SetStyles(ts)

	sp := components.NewLoadingIndicator(s)
	sp.SetMessage(cfg.LoadingMessage)

	return Model[T]{
		table:    t,
		items:    []T{},
		viewMode: ViewList,
		spinner:  sp,
		styles:   s,
		config:   cfg,
	}
}

// Init initializes the model, starting the fetch and spinner.
func (m Model[T]) Init() tea.Cmd {
	m.spinner.SetVisible(true)
	return tea.Batch(m.config.Fetch(), m.spinner.Init())
}

// Update handles messages.
func (m Model[T]) Update(msg tea.Msg) (Model[T], tea.Cmd) {
	if wmsg, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = wmsg.Width
		m.height = wmsg.Height
	}

	switch m.viewMode {
	case ViewDetail:
		return m.updateDetail(msg)
	default:
		return m.updateList(msg)
	}
}

func (m Model[T]) updateList(msg tea.Msg) (Model[T], tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// table.SetHeight accounts for the header internally, but not for the
		// newline between the header and the viewport in table.View().
		m.table.SetHeight(msg.Height - 1)
		minW := m.config.MinWidth
		if minW == 0 {
			minW = 70
		}
		m.table.SetColumns(makeColumns(m.config.Columns, msg.Width, minW))

	case spinner.TickMsg:
		if m.loading {
			var spinnerCmd tea.Cmd
			m.spinner, spinnerCmd = m.spinner.Update(msg)
			return m, spinnerCmd
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "r":
			m.loading = true
			m.spinner.SetVisible(true)
			return m, tea.Batch(m.config.Fetch(), m.spinner.Tick())
		case "enter":
			return m.enterDetailView()
		}
	}

	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m Model[T]) updateDetail(msg tea.Msg) (Model[T], tea.Cmd) {
	if m.detail == nil {
		m.viewMode = ViewList
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.detail.SetSize(msg.Width, msg.Height)

	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.viewMode = ViewList
			m.detail = nil
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.detail, cmd = m.detail.Update(msg)
	return m, cmd
}

func (m Model[T]) enterDetailView() (Model[T], tea.Cmd) {
	idx := m.table.Cursor()
	if idx < 0 || idx >= len(m.items) {
		return m, nil
	}

	selectedItem := m.items[idx]
	detail, cmd := m.config.EnterDetail(selectedItem, m.styles, m.width, m.height)
	m.detail = detail
	m.viewMode = ViewDetail

	return m, cmd
}

// View renders the view.
func (m Model[T]) View() string {
	if m.viewMode == ViewDetail && m.detail != nil {
		return m.detail.View()
	}
	return m.viewList()
}

var baseStyle = lipgloss.NewStyle()

func (m Model[T]) viewList() string {
	var content string

	if m.err != nil {
		content = fmt.Sprintf("Error loading %s: %v\n\nPress r to retry, q to quit", m.config.EntityName, m.err)
	} else if m.loading {
		content = m.spinner.View() + "\n\nPress q to quit"
	} else if len(m.items) == 0 {
		content = fmt.Sprintf("No %s found.\n\nPress r to refresh, q to quit", m.config.EntityName)
	} else {
		return baseStyle.Render(m.table.View())
	}

	contentStyle := lipgloss.NewStyle().
		Width(m.width)

	return contentStyle.Render(content)
}

// SetItems sets the items directly (e.g. from polling), clearing loading/error state.
func (m Model[T]) SetItems(items []T) Model[T] {
	m.loading = false
	m.spinner.SetVisible(false)
	m.err = nil
	m.items = items
	m.table.SetRows(m.config.ToRows(items, m.styles))
	return m
}

// HandleFetchResult handles a fetch response (items + error).
func (m Model[T]) HandleFetchResult(items []T, err error) Model[T] {
	m.loading = false
	m.spinner.SetVisible(false)
	if err != nil {
		m.err = err
		return m
	}
	m.items = items
	m.table.SetRows(m.config.ToRows(items, m.styles))
	return m
}

// Items returns the current items.
func (m Model[T]) Items() []T {
	return m.items
}

// SelectedIndex returns the currently selected table row index.
func (m Model[T]) SelectedIndex() int {
	return m.table.Cursor()
}

// GetViewMode returns the current view mode.
func (m Model[T]) GetViewMode() ViewMode {
	return m.viewMode
}

// GetContextItems returns context bar items, delegating to detail when in detail mode.
func (m Model[T]) GetContextItems() []components.ContextItem {
	if m.viewMode == ViewDetail && m.detail != nil {
		return m.detail.GetContextItems()
	}
	return nil
}

// GetScrollPercent returns the scroll percentage, delegating to detail when in detail mode.
func (m Model[T]) GetScrollPercent() float64 {
	if m.viewMode == ViewDetail && m.detail != nil {
		return m.detail.GetScrollPercent()
	}
	return 0
}

// GetStatusMessage returns the status message, delegating to detail when in detail mode.
func (m Model[T]) GetStatusMessage() string {
	if m.viewMode == ViewDetail && m.detail != nil {
		return m.detail.GetStatusMessage()
	}
	return ""
}

// HasContextBar returns whether the current view should show a context bar.
func (m Model[T]) HasContextBar() bool {
	if m.config.HasContextBar != nil {
		return m.config.HasContextBar(m.viewMode)
	}
	return false
}

// Table returns the underlying table model (for domain-specific access).
func (m Model[T]) Table() table.Model {
	return m.table
}

// SetTable sets the underlying table model.
func (m *Model[T]) SetTable(t table.Model) {
	m.table = t
}

// Detail returns the current detail view (may be nil).
func (m Model[T]) Detail() DetailView {
	return m.detail
}

// cellPadding is the horizontal space added by each table cell's Padding(0, 1) style.
const cellPadding = 2

// makeColumns creates table columns from specs, sized for the given width.
func makeColumns(specs []ColumnSpec, width, minWidth int) []table.Column {
	// Subtract cell padding per column so the total rendered width fits
	available := width - len(specs)*cellPadding
	if available < minWidth {
		available = minWidth
	}

	columns := make([]table.Column, len(specs))
	for i, spec := range specs {
		w := available * spec.WidthPct / 100
		if w < spec.MinWidth {
			w = spec.MinWidth
		}
		columns[i] = table.Column{Title: spec.Title, Width: w}
	}
	return columns
}
