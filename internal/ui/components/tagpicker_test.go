package components

import (
	"strings"
	"testing"

	"github.com/Elpulgo/azdo/internal/ui/styles"
	tea "github.com/charmbracelet/bubbletea"
)

func newTestTagPicker() TagPicker {
	theme := styles.GetDefaultTheme()
	appStyles := styles.NewStyles(theme)
	return NewTagPicker(appStyles)
}

func TestTagPickerInitialization(t *testing.T) {
	picker := newTestTagPicker()

	if picker.IsVisible() {
		t.Error("Expected tag picker to be hidden initially")
	}

	if picker.GetCursor() != 0 {
		t.Errorf("Expected initial cursor at 0, got %d", picker.GetCursor())
	}
}

func TestTagPickerShowHide(t *testing.T) {
	picker := newTestTagPicker()

	if picker.IsVisible() {
		t.Error("Expected tag picker to be hidden initially")
	}

	picker.Show()
	if !picker.IsVisible() {
		t.Error("Expected tag picker to be visible after Show()")
	}

	picker.Hide()
	if picker.IsVisible() {
		t.Error("Expected tag picker to be hidden after Hide()")
	}
}

func TestTagPickerSetTags(t *testing.T) {
	picker := newTestTagPicker()
	tags := []string{"Sprint 1", "Backend", "Frontend", "Bug Fix"}

	picker.SetTags(tags, "")

	if picker.GetCursor() != 0 {
		t.Errorf("Expected cursor at 0 (no active tag), got %d", picker.GetCursor())
	}
}

func TestTagPickerSetTags_WithActiveTag(t *testing.T) {
	picker := newTestTagPicker()
	tags := []string{"Sprint 1", "Backend", "Frontend"}

	picker.SetTags(tags, "Backend")

	// First option is "Clear filter", so Backend should be at index 2
	// Cursor should be positioned on the active tag
	if picker.GetCursor() != 2 {
		t.Errorf("Expected cursor at 2 (Backend), got %d", picker.GetCursor())
	}
}

func TestTagPickerNavigation(t *testing.T) {
	picker := newTestTagPicker()
	picker.SetTags([]string{"Sprint 1", "Backend", "Frontend"}, "")
	picker.Show()

	// Initial cursor at 0
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

	// Can't go past first item
	picker, _ = picker.Update(tea.KeyMsg{Type: tea.KeyUp})
	picker, _ = picker.Update(tea.KeyMsg{Type: tea.KeyUp})
	if picker.GetCursor() != 0 {
		t.Errorf("Expected cursor to stay at 0 when at top, got %d", picker.GetCursor())
	}
}

func TestTagPickerNavigation_JK(t *testing.T) {
	picker := newTestTagPicker()
	picker.SetTags([]string{"Sprint 1", "Backend"}, "")
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

func TestTagPickerSelection_Tag(t *testing.T) {
	picker := newTestTagPicker()
	picker.SetTags([]string{"Sprint 1", "Backend"}, "")
	picker.Show()

	// Select first tag "Sprint 1"
	picker, cmd := picker.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if cmd == nil {
		t.Fatal("Expected command after selection")
	}

	msg := cmd()
	tagMsg, ok := msg.(TagSelectedMsg)
	if !ok {
		t.Fatalf("Expected TagSelectedMsg, got %T", msg)
	}

	if tagMsg.Tag != "Sprint 1" {
		t.Errorf("Expected tag 'Sprint 1', got %q", tagMsg.Tag)
	}

	// Picker should be hidden after selection
	if picker.IsVisible() {
		t.Error("Expected picker to be hidden after selection")
	}
}

func TestTagPickerSelection_ClearFilter(t *testing.T) {
	picker := newTestTagPicker()
	tags := []string{"Sprint 1", "Backend"}

	// Set with an active tag - "Clear filter" appears as first option
	picker.SetTags(tags, "Sprint 1")
	picker.Show()

	// Navigate to first option (Clear filter) - cursor should be on active tag,
	// so move to top first
	for i := 0; i < 5; i++ {
		picker, _ = picker.Update(tea.KeyMsg{Type: tea.KeyUp})
	}

	// Select "Clear filter"
	picker, cmd := picker.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if cmd == nil {
		t.Fatal("Expected command after clear selection")
	}

	msg := cmd()
	tagMsg, ok := msg.(TagSelectedMsg)
	if !ok {
		t.Fatalf("Expected TagSelectedMsg, got %T", msg)
	}

	if tagMsg.Tag != "" {
		t.Errorf("Expected empty tag for clear filter, got %q", tagMsg.Tag)
	}
}

func TestTagPickerEscape(t *testing.T) {
	picker := newTestTagPicker()
	picker.SetTags([]string{"Sprint 1"}, "")
	picker.Show()

	if !picker.IsVisible() {
		t.Fatal("Expected picker to be visible before escape")
	}

	picker, cmd := picker.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if picker.IsVisible() {
		t.Error("Expected picker to be hidden after escape")
	}

	if cmd != nil {
		t.Error("Expected no command after escape (no selection)")
	}
}

func TestTagPickerQuitKey(t *testing.T) {
	picker := newTestTagPicker()
	picker.SetTags([]string{"Sprint 1"}, "")
	picker.Show()

	picker, _ = picker.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})

	if picker.IsVisible() {
		t.Error("Expected picker to be hidden after q")
	}
}

func TestTagPickerIgnoresInputWhenHidden(t *testing.T) {
	picker := newTestTagPicker()
	picker.SetTags([]string{"Sprint 1", "Backend"}, "")
	// Don't show it

	picker, cmd := picker.Update(tea.KeyMsg{Type: tea.KeyDown})
	if cmd != nil {
		t.Error("Expected no command when picker is hidden")
	}

	if picker.GetCursor() != 0 {
		t.Errorf("Expected cursor to stay at 0 when hidden, got %d", picker.GetCursor())
	}
}

func TestTagPickerView_Hidden(t *testing.T) {
	picker := newTestTagPicker()
	picker.SetTags([]string{"Sprint 1"}, "")

	view := picker.View()
	if view != "" {
		t.Error("Expected empty view when picker is hidden")
	}
}

func TestTagPickerView_Visible(t *testing.T) {
	picker := newTestTagPicker()
	picker.SetTags([]string{"Sprint 1", "Backend", "Frontend"}, "")
	picker.Show()
	picker.SetSize(80, 24)

	view := picker.View()
	if view == "" {
		t.Error("Expected non-empty view when picker is visible")
	}

	// Should contain tag names
	if !strings.Contains(view, "Sprint 1") {
		t.Error("Expected view to contain 'Sprint 1'")
	}
	if !strings.Contains(view, "Backend") {
		t.Error("Expected view to contain 'Backend'")
	}
	if !strings.Contains(view, "Frontend") {
		t.Error("Expected view to contain 'Frontend'")
	}
}

func TestTagPickerView_ShowsClearFilter(t *testing.T) {
	picker := newTestTagPicker()
	picker.SetTags([]string{"Sprint 1"}, "Sprint 1")
	picker.Show()
	picker.SetSize(80, 24)

	view := picker.View()

	if !strings.Contains(view, "Clear filter") {
		t.Error("Expected view to show 'Clear filter' when a tag is active")
	}
}

func TestTagPickerView_NoClearFilterWhenInactive(t *testing.T) {
	picker := newTestTagPicker()
	picker.SetTags([]string{"Sprint 1"}, "")
	picker.Show()
	picker.SetSize(80, 24)

	view := picker.View()

	if strings.Contains(view, "Clear filter") {
		t.Error("Expected no 'Clear filter' when no tag is active")
	}
}

func TestTagPickerEmptySelection(t *testing.T) {
	picker := newTestTagPicker()
	// No tags set
	picker.Show()

	picker, cmd := picker.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if cmd != nil {
		t.Error("Expected no command when selecting with empty options")
	}
}
