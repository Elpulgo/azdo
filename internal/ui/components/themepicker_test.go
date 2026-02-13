package components

import (
	"testing"

	"github.com/Elpulgo/azdo/internal/ui/styles"
	tea "github.com/charmbracelet/bubbletea"
)

// TestThemePickerInitialization tests that a theme picker can be created
func TestThemePickerInitialization(t *testing.T) {
	theme := styles.GetDefaultTheme()
	appStyles := styles.NewStyles(theme)
	availableThemes := []string{"dark", "gruvbox", "nord"}
	currentTheme := "dark"

	picker := NewThemePicker(appStyles, availableThemes, currentTheme)

	if picker.IsVisible() {
		t.Error("Expected theme picker to be hidden initially")
	}
}

// TestThemePickerShowHide tests visibility toggling
func TestThemePickerShowHide(t *testing.T) {
	theme := styles.GetDefaultTheme()
	appStyles := styles.NewStyles(theme)
	availableThemes := []string{"dark", "gruvbox", "nord"}
	currentTheme := "dark"

	picker := NewThemePicker(appStyles, availableThemes, currentTheme)

	// Should be hidden initially
	if picker.IsVisible() {
		t.Error("Expected theme picker to be hidden initially")
	}

	// Show it
	picker.Show()
	if !picker.IsVisible() {
		t.Error("Expected theme picker to be visible after Show()")
	}

	// Hide it
	picker.Hide()
	if picker.IsVisible() {
		t.Error("Expected theme picker to be hidden after Hide()")
	}
}

// TestThemePickerNavigation tests keyboard navigation
func TestThemePickerNavigation(t *testing.T) {
	theme := styles.GetDefaultTheme()
	appStyles := styles.NewStyles(theme)
	availableThemes := []string{"dark", "gruvbox", "nord", "dracula"}
	currentTheme := "dark"

	picker := NewThemePicker(appStyles, availableThemes, currentTheme)
	picker.Show()

	// Initial cursor should be at 0 (current theme)
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

	// Move to bottom
	picker, _ = picker.Update(tea.KeyMsg{Type: tea.KeyDown})
	picker, _ = picker.Update(tea.KeyMsg{Type: tea.KeyDown})
	if picker.GetCursor() != 3 {
		t.Errorf("Expected cursor at 3 (last item), got %d", picker.GetCursor())
	}

	// Try to move down past last item (should stay at 3)
	picker, _ = picker.Update(tea.KeyMsg{Type: tea.KeyDown})
	if picker.GetCursor() != 3 {
		t.Errorf("Expected cursor to stay at 3 when at bottom, got %d", picker.GetCursor())
	}

	// Move to top
	picker, _ = picker.Update(tea.KeyMsg{Type: tea.KeyUp})
	picker, _ = picker.Update(tea.KeyMsg{Type: tea.KeyUp})
	picker, _ = picker.Update(tea.KeyMsg{Type: tea.KeyUp})
	if picker.GetCursor() != 0 {
		t.Errorf("Expected cursor at 0 (first item), got %d", picker.GetCursor())
	}

	// Try to move up past first item (should stay at 0)
	picker, _ = picker.Update(tea.KeyMsg{Type: tea.KeyUp})
	if picker.GetCursor() != 0 {
		t.Errorf("Expected cursor to stay at 0 when at top, got %d", picker.GetCursor())
	}
}

// TestThemePickerSelection tests theme selection
func TestThemePickerSelection(t *testing.T) {
	theme := styles.GetDefaultTheme()
	appStyles := styles.NewStyles(theme)
	availableThemes := []string{"dark", "gruvbox", "nord"}
	currentTheme := "dark"

	picker := NewThemePicker(appStyles, availableThemes, currentTheme)
	picker.Show()

	// Move to second theme
	picker, _ = picker.Update(tea.KeyMsg{Type: tea.KeyDown})

	// Select it
	updatedPicker, cmd := picker.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if cmd == nil {
		t.Error("Expected command after selection")
	}

	// Should return ThemeSelectedMsg
	msg := cmd()
	if msg == nil {
		t.Fatal("Expected message from command")
	}

	themeMsg, ok := msg.(ThemeSelectedMsg)
	if !ok {
		t.Fatalf("Expected ThemeSelectedMsg, got %T", msg)
	}

	if themeMsg.ThemeName != "gruvbox" {
		t.Errorf("Expected selected theme 'gruvbox', got '%s'", themeMsg.ThemeName)
	}

	// Picker should be hidden after selection
	if updatedPicker.IsVisible() {
		t.Error("Expected picker to be hidden after selection")
	}
}

// TestThemePickerEscape tests closing with escape key
func TestThemePickerEscape(t *testing.T) {
	theme := styles.GetDefaultTheme()
	appStyles := styles.NewStyles(theme)
	availableThemes := []string{"dark", "gruvbox", "nord"}
	currentTheme := "dark"

	picker := NewThemePicker(appStyles, availableThemes, currentTheme)
	picker.Show()

	if !picker.IsVisible() {
		t.Fatal("Expected picker to be visible before escape")
	}

	// Press escape
	updatedPicker, _ := picker.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if updatedPicker.IsVisible() {
		t.Error("Expected picker to be hidden after escape")
	}
}

// TestThemePickerView tests rendering
func TestThemePickerView(t *testing.T) {
	theme := styles.GetDefaultTheme()
	appStyles := styles.NewStyles(theme)
	availableThemes := []string{"dark", "gruvbox", "nord"}
	currentTheme := "dark"

	picker := NewThemePicker(appStyles, availableThemes, currentTheme)

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

	// Should contain theme names
	if !contains(view, "dark") {
		t.Error("Expected view to contain 'dark' theme")
	}
	if !contains(view, "gruvbox") {
		t.Error("Expected view to contain 'gruvbox' theme")
	}
}

// TestThemePickerSetSize tests size setting
func TestThemePickerSetSize(t *testing.T) {
	theme := styles.GetDefaultTheme()
	appStyles := styles.NewStyles(theme)
	availableThemes := []string{"dark", "gruvbox"}
	currentTheme := "dark"

	picker := NewThemePicker(appStyles, availableThemes, currentTheme)
	picker.SetSize(100, 30)

	// Should not panic and should accept the size
	// The actual size handling is implementation-specific
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr || len(s) > 0 && (s[0:len(substr)] == substr ||
		len(s) > len(substr) && contains(s[1:], substr)))
}
