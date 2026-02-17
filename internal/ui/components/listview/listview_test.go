package listview

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Elpulgo/azdo/internal/ui/components"
	"github.com/Elpulgo/azdo/internal/ui/components/table"
	"github.com/Elpulgo/azdo/internal/ui/styles"
	tea "github.com/charmbracelet/bubbletea"
)

// testItem is a stub type for testing the generic list model.
type testItem struct {
	ID   int
	Name string
}

// testDetailView is a stub implementing DetailView for testing.
type testDetailView struct {
	width, height int
}

func (d *testDetailView) Update(msg tea.Msg) (DetailView, tea.Cmd) {
	return d, nil
}

func (d *testDetailView) View() string {
	return "detail view content"
}

func (d *testDetailView) SetSize(width, height int) {
	d.width = width
	d.height = height
}

func (d *testDetailView) GetContextItems() []components.ContextItem {
	return []components.ContextItem{{Key: "esc", Description: "back"}}
}

func (d *testDetailView) GetScrollPercent() float64 {
	return 0.5
}

func (d *testDetailView) GetStatusMessage() string {
	return "test status"
}

func testConfig() Config[testItem] {
	return Config[testItem]{
		Columns: []ColumnSpec{
			{Title: "ID", WidthPct: 30, MinWidth: 6},
			{Title: "Name", WidthPct: 70, MinWidth: 10},
		},
		LoadingMessage: "Loading test items...",
		EntityName:     "test items",
		MinWidth:       70,
		ToRows: func(items []testItem, s *styles.Styles) []table.Row {
			rows := make([]table.Row, len(items))
			for i, item := range items {
				rows[i] = table.Row{fmt.Sprintf("%d", item.ID), item.Name}
			}
			return rows
		},
		Fetch: func() tea.Cmd {
			return func() tea.Msg { return nil }
		},
		EnterDetail: func(item testItem, s *styles.Styles, w, h int) (DetailView, tea.Cmd) {
			d := &testDetailView{width: w, height: h}
			return d, nil
		},
	}
}

func TestNew_InitialState(t *testing.T) {
	s := styles.DefaultStyles()
	m := New(testConfig(), s)

	if m.GetViewMode() != ViewList {
		t.Errorf("Expected ViewList, got %v", m.GetViewMode())
	}
	if len(m.Items()) != 0 {
		t.Errorf("Expected 0 items, got %d", len(m.Items()))
	}
	if m.styles == nil {
		t.Error("Expected styles to be set")
	}
}

func TestNew_CustomStyles(t *testing.T) {
	s := styles.NewStyles(styles.GetThemeByNameWithFallback("gruvbox"))
	m := New(testConfig(), s)

	if m.styles != s {
		t.Error("Expected model to use provided custom styles")
	}
}

func TestSetItems_ClearsLoadingAndError(t *testing.T) {
	s := styles.DefaultStyles()
	m := New(testConfig(), s)
	m.loading = true
	m.err = fmt.Errorf("previous error")

	items := []testItem{{ID: 1, Name: "Alpha"}, {ID: 2, Name: "Beta"}}
	m = m.SetItems(items)

	if m.loading {
		t.Error("Expected loading to be false after SetItems")
	}
	if m.err != nil {
		t.Errorf("Expected err to be nil after SetItems, got %v", m.err)
	}
	if len(m.Items()) != 2 {
		t.Errorf("Expected 2 items, got %d", len(m.Items()))
	}
}

func TestHandleFetchResult_Success(t *testing.T) {
	s := styles.DefaultStyles()
	m := New(testConfig(), s)
	m.loading = true

	items := []testItem{{ID: 1, Name: "Alpha"}}
	m = m.HandleFetchResult(items, nil)

	if m.loading {
		t.Error("Expected loading to be false after HandleFetchResult")
	}
	if m.err != nil {
		t.Errorf("Expected err to be nil, got %v", m.err)
	}
	if len(m.Items()) != 1 {
		t.Errorf("Expected 1 item, got %d", len(m.Items()))
	}
}

func TestHandleFetchResult_Error(t *testing.T) {
	s := styles.DefaultStyles()
	m := New(testConfig(), s)
	m.loading = true

	mockErr := fmt.Errorf("fetch failed")
	m = m.HandleFetchResult(nil, mockErr)

	if m.loading {
		t.Error("Expected loading to be false after error")
	}
	if m.err == nil {
		t.Error("Expected err to be set after error")
	}
}

func TestView_Loading(t *testing.T) {
	s := styles.DefaultStyles()
	m := New(testConfig(), s)
	m.loading = true
	m.spinner.SetVisible(true)
	m.width = 100
	m.height = 30

	view := m.View()
	if !strings.Contains(view, "test items") || !strings.Contains(view, "quit") {
		t.Errorf("Loading view should contain loading message and quit instruction, got: %q", view)
	}
}

func TestView_Error(t *testing.T) {
	s := styles.DefaultStyles()
	m := New(testConfig(), s)
	m.err = fmt.Errorf("something broke")
	m.width = 100
	m.height = 30

	view := m.View()
	if !strings.Contains(view, "Error") {
		t.Error("Error view should contain 'Error'")
	}
	if !strings.Contains(view, "test items") {
		t.Error("Error view should contain entity name")
	}
}

func TestView_Empty(t *testing.T) {
	s := styles.DefaultStyles()
	m := New(testConfig(), s)
	m.items = []testItem{}
	m.width = 100
	m.height = 30

	view := m.View()
	if !strings.Contains(view, "No test items") {
		t.Error("Empty view should contain 'No test items'")
	}
}

func TestView_WithItems(t *testing.T) {
	s := styles.DefaultStyles()
	m := New(testConfig(), s)
	m.width = 100
	m.height = 30

	items := []testItem{{ID: 1, Name: "Alpha"}}
	m = m.SetItems(items)

	// Resize so table has proper dimensions
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	view := m.View()
	// Table view should render (not error/loading/empty)
	if strings.Contains(view, "Error") || strings.Contains(view, "No test items") {
		t.Errorf("Expected table view, got: %q", view)
	}
}

func TestUpdate_EnterNavigatesToDetail(t *testing.T) {
	s := styles.DefaultStyles()
	m := New(testConfig(), s)
	m.width = 100
	m.height = 30

	items := []testItem{{ID: 1, Name: "Alpha"}}
	m = m.SetItems(items)

	// Press enter to navigate to detail
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m.GetViewMode() != ViewDetail {
		t.Errorf("Expected ViewDetail after Enter, got %v", m.GetViewMode())
	}
	if m.detail == nil {
		t.Error("Expected detail to be set")
	}
}

func TestUpdate_EscReturnsToList(t *testing.T) {
	s := styles.DefaultStyles()
	m := New(testConfig(), s)
	m.width = 100
	m.height = 30

	items := []testItem{{ID: 1, Name: "Alpha"}}
	m = m.SetItems(items)

	// Enter detail
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.GetViewMode() != ViewDetail {
		t.Fatalf("Expected ViewDetail, got %v", m.GetViewMode())
	}

	// Esc back to list
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.GetViewMode() != ViewList {
		t.Errorf("Expected ViewList after Esc, got %v", m.GetViewMode())
	}
	if m.detail != nil {
		t.Error("Expected detail to be nil after Esc")
	}
}

func TestUpdate_RefreshKey(t *testing.T) {
	s := styles.DefaultStyles()
	fetchCalled := false
	cfg := testConfig()
	cfg.Fetch = func() tea.Cmd {
		fetchCalled = true
		return func() tea.Msg { return nil }
	}
	m := New(cfg, s)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})

	if !fetchCalled {
		t.Error("Expected Fetch to be called on 'r' key")
	}
	if !m.loading {
		t.Error("Expected loading to be true after refresh")
	}
}

func TestUpdate_WindowResize(t *testing.T) {
	s := styles.DefaultStyles()
	m := New(testConfig(), s)

	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	if m.width != 120 {
		t.Errorf("Expected width 120, got %d", m.width)
	}
	if m.height != 40 {
		t.Errorf("Expected height 40, got %d", m.height)
	}
}

func TestGetContextItems_ListMode(t *testing.T) {
	s := styles.DefaultStyles()
	m := New(testConfig(), s)

	items := m.GetContextItems()
	if items != nil {
		t.Error("Expected nil context items in list mode")
	}
}

func TestGetContextItems_DetailMode(t *testing.T) {
	s := styles.DefaultStyles()
	m := New(testConfig(), s)
	m.width = 100
	m.height = 30

	m = m.SetItems([]testItem{{ID: 1, Name: "Alpha"}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	items := m.GetContextItems()
	if len(items) != 1 || items[0].Key != "esc" {
		t.Errorf("Expected detail context items, got %v", items)
	}
}

func TestGetScrollPercent_ListMode(t *testing.T) {
	s := styles.DefaultStyles()
	m := New(testConfig(), s)

	if m.GetScrollPercent() != 0 {
		t.Errorf("Expected 0 scroll percent in list mode, got %f", m.GetScrollPercent())
	}
}

func TestGetScrollPercent_DetailMode(t *testing.T) {
	s := styles.DefaultStyles()
	m := New(testConfig(), s)
	m.width = 100
	m.height = 30

	m = m.SetItems([]testItem{{ID: 1, Name: "Alpha"}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m.GetScrollPercent() != 0.5 {
		t.Errorf("Expected 0.5 scroll percent in detail mode, got %f", m.GetScrollPercent())
	}
}

func TestGetStatusMessage_ListMode(t *testing.T) {
	s := styles.DefaultStyles()
	m := New(testConfig(), s)

	if m.GetStatusMessage() != "" {
		t.Errorf("Expected empty status message, got %q", m.GetStatusMessage())
	}
}

func TestGetStatusMessage_DetailMode(t *testing.T) {
	s := styles.DefaultStyles()
	m := New(testConfig(), s)
	m.width = 100
	m.height = 30

	m = m.SetItems([]testItem{{ID: 1, Name: "Alpha"}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m.GetStatusMessage() != "test status" {
		t.Errorf("Expected 'test status', got %q", m.GetStatusMessage())
	}
}

func TestHasContextBar_Default(t *testing.T) {
	s := styles.DefaultStyles()
	m := New(testConfig(), s)

	if m.HasContextBar() {
		t.Error("Expected no context bar by default")
	}
}

func TestHasContextBar_WithCallback(t *testing.T) {
	s := styles.DefaultStyles()
	cfg := testConfig()
	cfg.HasContextBar = func(mode ViewMode) bool {
		return mode == ViewDetail
	}
	m := New(cfg, s)
	m.width = 100
	m.height = 30

	if m.HasContextBar() {
		t.Error("Expected no context bar in list mode")
	}

	m = m.SetItems([]testItem{{ID: 1, Name: "Alpha"}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if !m.HasContextBar() {
		t.Error("Expected context bar in detail mode")
	}
}

func TestSelectedIndex(t *testing.T) {
	s := styles.DefaultStyles()
	m := New(testConfig(), s)

	items := []testItem{{ID: 1, Name: "Alpha"}, {ID: 2, Name: "Beta"}}
	m = m.SetItems(items)

	idx := m.SelectedIndex()
	if idx != 0 {
		t.Errorf("Expected selected index 0, got %d", idx)
	}
}

func TestEnterDetailView_EmptyItems(t *testing.T) {
	s := styles.DefaultStyles()
	m := New(testConfig(), s)

	// With no items, enter should not navigate to detail
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m.GetViewMode() != ViewList {
		t.Error("Should stay in list mode when no items")
	}
}

func TestMakeColumns(t *testing.T) {
	specs := []ColumnSpec{
		{Title: "ID", WidthPct: 30, MinWidth: 6},
		{Title: "Name", WidthPct: 70, MinWidth: 10},
	}

	columns := makeColumns(specs, 100, 70)

	if len(columns) != 2 {
		t.Fatalf("Expected 2 columns, got %d", len(columns))
	}
	if columns[0].Title != "ID" {
		t.Errorf("Expected column 0 title 'ID', got %q", columns[0].Title)
	}
	if columns[1].Title != "Name" {
		t.Errorf("Expected column 1 title 'Name', got %q", columns[1].Title)
	}
}

func TestDetailView_ReceivesSize(t *testing.T) {
	s := styles.DefaultStyles()
	cfg := testConfig()
	sizeW, sizeH := 0, 0
	cfg.EnterDetail = func(item testItem, st *styles.Styles, w, h int) (DetailView, tea.Cmd) {
		sizeW, sizeH = w, h
		return &testDetailView{width: w, height: h}, nil
	}
	m := New(cfg, s)
	m.width = 120
	m.height = 40

	m = m.SetItems([]testItem{{ID: 1, Name: "Alpha"}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if sizeW != 120 || sizeH != 40 {
		t.Errorf("Expected detail to receive size (120, 40), got (%d, %d)", sizeW, sizeH)
	}
}

func TestView_DetailMode(t *testing.T) {
	s := styles.DefaultStyles()
	m := New(testConfig(), s)
	m.width = 100
	m.height = 30

	m = m.SetItems([]testItem{{ID: 1, Name: "Alpha"}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	view := m.View()
	if !strings.Contains(view, "detail view content") {
		t.Errorf("Expected detail view content, got: %q", view)
	}
}
