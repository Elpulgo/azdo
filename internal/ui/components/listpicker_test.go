package components

import (
	"strings"
	"testing"

	"github.com/Elpulgo/azdo/internal/ui/styles"
	tea "github.com/charmbracelet/bubbletea"
)

func newTestListPicker() ListPicker {
	theme := styles.GetDefaultTheme()
	appStyles := styles.NewStyles(theme)
	return NewListPicker(appStyles)
}

func testListPickerOptions() []ListPickerOption {
	return []ListPickerOption{
		{Name: "New", Icon: "○"},
		{Name: "Active", Icon: "●"},
		{Name: "Closed", Icon: "✓"},
	}
}

func TestListPickerInitialization(t *testing.T) {
	picker := newTestListPicker()

	if picker.IsVisible() {
		t.Error("Expected list picker to be hidden initially")
	}

	if picker.GetCursor() != 0 {
		t.Errorf("Expected initial cursor at 0, got %d", picker.GetCursor())
	}
}

func TestListPickerShowHide(t *testing.T) {
	picker := newTestListPicker()

	picker.Show()
	if !picker.IsVisible() {
		t.Error("Expected list picker to be visible after Show()")
	}

	picker.Hide()
	if picker.IsVisible() {
		t.Error("Expected list picker to be hidden after Hide()")
	}
}

func TestListPickerSetConfig_NoActiveValue(t *testing.T) {
	picker := newTestListPicker()
	picker.SetConfig("Filter by State", testListPickerOptions(), "", true)

	if len(picker.options) != 3 {
		t.Errorf("Expected 3 options, got %d", len(picker.options))
	}

	if picker.GetCursor() != 0 {
		t.Errorf("Expected cursor at 0, got %d", picker.GetCursor())
	}
}

func TestListPickerSetConfig_WithActiveValueAndClear(t *testing.T) {
	picker := newTestListPicker()
	picker.SetConfig("Filter by State", testListPickerOptions(), "Active", true)

	if len(picker.options) != 4 {
		t.Errorf("Expected 4 options including clear, got %d", len(picker.options))
	}

	if picker.options[0].Name != "Clear filter" {
		t.Errorf("Expected first option to be Clear filter, got %q", picker.options[0].Name)
	}

	if picker.GetCursor() != 2 {
		t.Errorf("Expected cursor at 2 (Active with clear offset), got %d", picker.GetCursor())
	}
}

func TestListPickerSetConfig_WithActiveValueWithoutClear(t *testing.T) {
	picker := newTestListPicker()
	picker.SetConfig("Filter by State", testListPickerOptions(), "Active", false)

	if len(picker.options) != 3 {
		t.Errorf("Expected 3 options without clear, got %d", len(picker.options))
	}

	if picker.options[0].Name == "Clear filter" {
		t.Error("Did not expect Clear filter option when allowClear is false")
	}

	if picker.GetCursor() != 1 {
		t.Errorf("Expected cursor at 1 (Active), got %d", picker.GetCursor())
	}
}

func TestListPickerNavigation(t *testing.T) {
	picker := newTestListPicker()
	picker.SetConfig("Filter by State", testListPickerOptions(), "", true)
	picker.Show()

	picker, _ = picker.Update(tea.KeyMsg{Type: tea.KeyDown})
	if picker.GetCursor() != 1 {
		t.Errorf("Expected cursor at 1 after down, got %d", picker.GetCursor())
	}

	picker, _ = picker.Update(tea.KeyMsg{Type: tea.KeyDown})
	picker, _ = picker.Update(tea.KeyMsg{Type: tea.KeyDown})
	if picker.GetCursor() != 2 {
		t.Errorf("Expected cursor at last item (2), got %d", picker.GetCursor())
	}

	picker, _ = picker.Update(tea.KeyMsg{Type: tea.KeyUp})
	if picker.GetCursor() != 1 {
		t.Errorf("Expected cursor at 1 after up, got %d", picker.GetCursor())
	}

	picker, _ = picker.Update(tea.KeyMsg{Type: tea.KeyUp})
	picker, _ = picker.Update(tea.KeyMsg{Type: tea.KeyUp})
	if picker.GetCursor() != 0 {
		t.Errorf("Expected cursor at 0 when at top, got %d", picker.GetCursor())
	}
}

func TestListPickerNavigationJK(t *testing.T) {
	picker := newTestListPicker()
	picker.SetConfig("Filter by State", testListPickerOptions(), "", true)
	picker.Show()

	picker, _ = picker.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if picker.GetCursor() != 1 {
		t.Errorf("Expected cursor at 1 after j, got %d", picker.GetCursor())
	}

	picker, _ = picker.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if picker.GetCursor() != 0 {
		t.Errorf("Expected cursor at 0 after k, got %d", picker.GetCursor())
	}
}

func TestListPickerSelection_NormalOption(t *testing.T) {
	picker := newTestListPicker()
	picker.SetConfig("Filter by State", testListPickerOptions(), "", true)
	picker.Show()

	picker, _ = picker.Update(tea.KeyMsg{Type: tea.KeyDown})
	picker, cmd := picker.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if cmd == nil {
		t.Fatal("Expected command after selection")
	}

	msg := cmd()
	selectedMsg, ok := msg.(ListPickerSelectedMsg)
	if !ok {
		t.Fatalf("Expected ListPickerSelectedMsg, got %T", msg)
	}

	if selectedMsg.Value != "Active" {
		t.Errorf("Expected selected value Active, got %q", selectedMsg.Value)
	}

	if picker.IsVisible() {
		t.Error("Expected picker to be hidden after selection")
	}
}

func TestListPickerSelection_ClearFilter(t *testing.T) {
	picker := newTestListPicker()
	picker.SetConfig("Filter by State", testListPickerOptions(), "Active", true)
	picker.Show()

	for i := 0; i < 5; i++ {
		picker, _ = picker.Update(tea.KeyMsg{Type: tea.KeyUp})
	}

	_, cmd := picker.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Expected command after clear selection")
	}

	msg := cmd()
	selectedMsg, ok := msg.(ListPickerSelectedMsg)
	if !ok {
		t.Fatalf("Expected ListPickerSelectedMsg, got %T", msg)
	}

	if selectedMsg.Value != "" {
		t.Errorf("Expected empty selected value for clear filter, got %q", selectedMsg.Value)
	}
}

func TestListPickerEscapeAndQuit(t *testing.T) {
	picker := newTestListPicker()
	picker.SetConfig("Filter by State", testListPickerOptions(), "", true)
	picker.Show()

	picker, cmd := picker.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if picker.IsVisible() {
		t.Error("Expected picker to hide on esc")
	}
	if cmd != nil {
		t.Error("Expected no command on esc")
	}

	picker.Show()
	picker, _ = picker.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if picker.IsVisible() {
		t.Error("Expected picker to hide on q")
	}
}

func TestListPickerIgnoresInputWhenHidden(t *testing.T) {
	picker := newTestListPicker()
	picker.SetConfig("Filter by State", testListPickerOptions(), "", true)

	picker, cmd := picker.Update(tea.KeyMsg{Type: tea.KeyDown})
	if cmd != nil {
		t.Error("Expected no command when picker is hidden")
	}

	if picker.GetCursor() != 0 {
		t.Errorf("Expected cursor to stay at 0 when hidden, got %d", picker.GetCursor())
	}
}

func TestListPickerSelectionWithNoOptions(t *testing.T) {
	picker := newTestListPicker()
	picker.SetConfig("Filter by State", nil, "", true)
	picker.Show()

	_, cmd := picker.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Error("Expected no command when selecting with empty options")
	}
}

func TestListPickerView(t *testing.T) {
	picker := newTestListPicker()
	picker.SetConfig("Filter by State", testListPickerOptions(), "Active", true)

	if picker.View() != "" {
		t.Error("Expected empty view when picker is hidden")
	}

	picker.Show()
	picker.SetSize(80, 24)
	view := picker.View()

	if view == "" {
		t.Fatal("Expected non-empty view when picker is visible")
	}

	if !strings.Contains(view, "Filter by State") {
		t.Error("Expected view to contain title")
	}
	if !strings.Contains(view, "Clear filter") {
		t.Error("Expected view to contain clear option")
	}
	if !strings.Contains(view, "Active (current)") {
		t.Error("Expected view to contain current indicator")
	}
	if !strings.Contains(view, "enter: select") {
		t.Error("Expected view to contain help text")
	}
}
