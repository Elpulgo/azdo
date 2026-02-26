package components

import (
	"strings"
	"testing"

	"github.com/Elpulgo/azdo/internal/azdevops"
	"github.com/Elpulgo/azdo/internal/ui/styles"
	tea "github.com/charmbracelet/bubbletea"
)

func newTestStatePicker() StatePicker {
	theme := styles.GetDefaultTheme()
	appStyles := styles.NewStyles(theme)
	return NewStatePicker(appStyles)
}

func testStates() []azdevops.WorkItemTypeState {
	return []azdevops.WorkItemTypeState{
		{Name: "New", Color: "b2b2b2", Category: "Proposed"},
		{Name: "Active", Color: "007acc", Category: "InProgress"},
		{Name: "Resolved", Color: "ff9d00", Category: "Resolved"},
		{Name: "Closed", Color: "339933", Category: "Completed"},
	}
}

func TestStatePickerInitialization(t *testing.T) {
	picker := newTestStatePicker()

	if picker.IsVisible() {
		t.Error("Expected state picker to be hidden initially")
	}

	if picker.GetCursor() != 0 {
		t.Errorf("Expected initial cursor at 0, got %d", picker.GetCursor())
	}
}

func TestStatePickerShowHide(t *testing.T) {
	picker := newTestStatePicker()

	if picker.IsVisible() {
		t.Error("Expected state picker to be hidden initially")
	}

	picker.Show()
	if !picker.IsVisible() {
		t.Error("Expected state picker to be visible after Show()")
	}

	picker.Hide()
	if picker.IsVisible() {
		t.Error("Expected state picker to be hidden after Hide()")
	}
}

func TestStatePickerSetStates(t *testing.T) {
	picker := newTestStatePicker()
	states := testStates()

	picker.SetStates(states, "Active")

	if len(picker.options) != 4 {
		t.Errorf("Expected 4 options, got %d", len(picker.options))
	}

	// Cursor should be on the current state
	if picker.GetCursor() != 1 {
		t.Errorf("Expected cursor at 1 (Active), got %d", picker.GetCursor())
	}
}

func TestStatePickerSetStates_CurrentStateNotFound(t *testing.T) {
	picker := newTestStatePicker()
	states := testStates()

	picker.SetStates(states, "Unknown")

	// Cursor should be at 0 when current state not found
	if picker.GetCursor() != 0 {
		t.Errorf("Expected cursor at 0, got %d", picker.GetCursor())
	}
}

func TestStatePickerNavigation(t *testing.T) {
	picker := newTestStatePicker()
	picker.SetStates(testStates(), "New")
	picker.Show()

	// Initial cursor at 0 (New)
	if picker.GetCursor() != 0 {
		t.Errorf("Expected initial cursor at 0, got %d", picker.GetCursor())
	}

	// Move down
	picker, _ = picker.Update(tea.KeyMsg{Type: tea.KeyDown})
	if picker.GetCursor() != 1 {
		t.Errorf("Expected cursor at 1 after down, got %d", picker.GetCursor())
	}

	// Move down again
	picker, _ = picker.Update(tea.KeyMsg{Type: tea.KeyDown})
	if picker.GetCursor() != 2 {
		t.Errorf("Expected cursor at 2 after second down, got %d", picker.GetCursor())
	}

	// Move up
	picker, _ = picker.Update(tea.KeyMsg{Type: tea.KeyUp})
	if picker.GetCursor() != 1 {
		t.Errorf("Expected cursor at 1 after up, got %d", picker.GetCursor())
	}

	// Navigate to bottom
	picker, _ = picker.Update(tea.KeyMsg{Type: tea.KeyDown})
	picker, _ = picker.Update(tea.KeyMsg{Type: tea.KeyDown})
	if picker.GetCursor() != 3 {
		t.Errorf("Expected cursor at 3 (last item), got %d", picker.GetCursor())
	}

	// Can't go past last item
	picker, _ = picker.Update(tea.KeyMsg{Type: tea.KeyDown})
	if picker.GetCursor() != 3 {
		t.Errorf("Expected cursor to stay at 3 when at bottom, got %d", picker.GetCursor())
	}

	// Navigate to top
	for i := 0; i < 5; i++ {
		picker, _ = picker.Update(tea.KeyMsg{Type: tea.KeyUp})
	}
	if picker.GetCursor() != 0 {
		t.Errorf("Expected cursor at 0 after navigating up, got %d", picker.GetCursor())
	}

	// Can't go past first item
	picker, _ = picker.Update(tea.KeyMsg{Type: tea.KeyUp})
	if picker.GetCursor() != 0 {
		t.Errorf("Expected cursor to stay at 0 when at top, got %d", picker.GetCursor())
	}
}

func TestStatePickerNavigationJK(t *testing.T) {
	picker := newTestStatePicker()
	picker.SetStates(testStates(), "New")
	picker.Show()

	// j moves down
	picker, _ = picker.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if picker.GetCursor() != 1 {
		t.Errorf("Expected cursor at 1 after j, got %d", picker.GetCursor())
	}

	// k moves up
	picker, _ = picker.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if picker.GetCursor() != 0 {
		t.Errorf("Expected cursor at 0 after k, got %d", picker.GetCursor())
	}
}

func TestStatePickerSelection(t *testing.T) {
	picker := newTestStatePicker()
	picker.SetStates(testStates(), "New")
	picker.Show()

	// Move to "Active" (index 1)
	picker, _ = picker.Update(tea.KeyMsg{Type: tea.KeyDown})

	// Select it
	updatedPicker, cmd := picker.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if cmd == nil {
		t.Error("Expected command after selection")
	}

	msg := cmd()
	if msg == nil {
		t.Fatal("Expected message from command")
	}

	stateMsg, ok := msg.(StateSelectedMsg)
	if !ok {
		t.Fatalf("Expected StateSelectedMsg, got %T", msg)
	}

	if stateMsg.State != "Active" {
		t.Errorf("Expected state 'Active', got %q", stateMsg.State)
	}

	// Picker should be hidden after selection
	if updatedPicker.IsVisible() {
		t.Error("Expected picker to be hidden after selection")
	}
}

func TestStatePickerSelectionAllOptions(t *testing.T) {
	states := testStates()

	for i, expectedState := range states {
		picker := newTestStatePicker()
		picker.SetStates(states, "New")
		picker.Show()

		// Navigate to the option
		for j := 0; j < i; j++ {
			picker, _ = picker.Update(tea.KeyMsg{Type: tea.KeyDown})
		}

		// Select it
		_, cmd := picker.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if cmd == nil {
			t.Errorf("Option %d: expected command after selection", i)
			continue
		}

		msg := cmd()
		stateMsg, ok := msg.(StateSelectedMsg)
		if !ok {
			t.Errorf("Option %d: expected StateSelectedMsg, got %T", i, msg)
			continue
		}

		if stateMsg.State != expectedState.Name {
			t.Errorf("Option %d: expected state %q, got %q", i, expectedState.Name, stateMsg.State)
		}
	}
}

func TestStatePickerEscape(t *testing.T) {
	picker := newTestStatePicker()
	picker.SetStates(testStates(), "New")
	picker.Show()

	if !picker.IsVisible() {
		t.Fatal("Expected picker to be visible before escape")
	}

	updatedPicker, cmd := picker.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if updatedPicker.IsVisible() {
		t.Error("Expected picker to be hidden after escape")
	}

	if cmd != nil {
		t.Error("Expected no command after escape")
	}
}

func TestStatePickerQuitKey(t *testing.T) {
	picker := newTestStatePicker()
	picker.SetStates(testStates(), "New")
	picker.Show()

	updatedPicker, _ := picker.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})

	if updatedPicker.IsVisible() {
		t.Error("Expected picker to be hidden after q")
	}
}

func TestStatePickerIgnoresInputWhenHidden(t *testing.T) {
	picker := newTestStatePicker()
	picker.SetStates(testStates(), "New")
	// Don't show it

	picker, cmd := picker.Update(tea.KeyMsg{Type: tea.KeyDown})
	if cmd != nil {
		t.Error("Expected no command when picker is hidden")
	}

	if picker.GetCursor() != 0 {
		t.Errorf("Expected cursor to stay at 0 when hidden, got %d", picker.GetCursor())
	}
}

func TestStatePickerView(t *testing.T) {
	picker := newTestStatePicker()
	picker.SetStates(testStates(), "Active")

	// Hidden picker should return empty string
	view := picker.View()
	if view != "" {
		t.Error("Expected empty view when picker is hidden")
	}

	// Visible picker should render content
	picker.Show()
	picker.SetSize(80, 24)
	view = picker.View()
	if view == "" {
		t.Error("Expected non-empty view when picker is visible")
	}

	// Should contain state names
	if !strings.Contains(view, "New") {
		t.Error("Expected view to contain 'New'")
	}
	if !strings.Contains(view, "Active") {
		t.Error("Expected view to contain 'Active'")
	}
	if !strings.Contains(view, "Resolved") {
		t.Error("Expected view to contain 'Resolved'")
	}
	if !strings.Contains(view, "Closed") {
		t.Error("Expected view to contain 'Closed'")
	}
}

func TestStatePickerViewShowsCurrentIndicator(t *testing.T) {
	picker := newTestStatePicker()
	picker.SetStates(testStates(), "Active")
	picker.Show()
	picker.SetSize(80, 24)

	view := picker.View()

	// The current state should be marked with a special indicator
	if !strings.Contains(view, "current") {
		t.Error("Expected view to indicate current state")
	}
}

func TestStatePickerSetSize(t *testing.T) {
	picker := newTestStatePicker()
	picker.SetSize(100, 30)
	// Should not panic
}
