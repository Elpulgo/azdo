package components

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestHelpModal_New(t *testing.T) {
	h := NewHelpModal()

	if h == nil {
		t.Fatal("expected non-nil HelpModal")
	}
	if h.IsVisible() {
		t.Error("help modal should be hidden by default")
	}
}

func TestHelpModal_Show(t *testing.T) {
	h := NewHelpModal()
	h.Show()

	if !h.IsVisible() {
		t.Error("help modal should be visible after Show()")
	}
}

func TestHelpModal_Hide(t *testing.T) {
	h := NewHelpModal()
	h.Show()
	h.Hide()

	if h.IsVisible() {
		t.Error("help modal should be hidden after Hide()")
	}
}

func TestHelpModal_Toggle(t *testing.T) {
	h := NewHelpModal()

	h.Toggle()
	if !h.IsVisible() {
		t.Error("should be visible after first toggle")
	}

	h.Toggle()
	if h.IsVisible() {
		t.Error("should be hidden after second toggle")
	}
}

func TestHelpModal_View_WhenHidden(t *testing.T) {
	h := NewHelpModal()
	h.SetSize(80, 24)

	view := h.View()

	if view != "" {
		t.Error("view should be empty when hidden")
	}
}

func TestHelpModal_View_WhenVisible(t *testing.T) {
	h := NewHelpModal()
	h.SetSize(80, 24)
	h.Show()

	view := h.View()

	if view == "" {
		t.Error("view should not be empty when visible")
	}
}

func TestHelpModal_View_ContainsTitle(t *testing.T) {
	h := NewHelpModal()
	h.SetSize(80, 24)
	h.Show()

	view := h.View()

	if !strings.Contains(strings.ToLower(view), "help") {
		t.Error("view should contain help title")
	}
}

func TestHelpModal_View_ContainsKeybindings(t *testing.T) {
	h := NewHelpModal()
	h.SetSize(80, 24)
	h.Show()

	view := h.View()

	// Should contain common keybindings
	keybindings := []string{"quit", "refresh", "move", "esc"}
	for _, kb := range keybindings {
		if !strings.Contains(strings.ToLower(view), kb) {
			t.Errorf("view should contain '%s' keybinding", kb)
		}
	}
}

func TestHelpModal_Update_EscHides(t *testing.T) {
	h := NewHelpModal()
	h.Show()

	h, _ = h.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if h.IsVisible() {
		t.Error("esc should hide the modal")
	}
}

func TestHelpModal_Update_QuestionMarkHides(t *testing.T) {
	h := NewHelpModal()
	h.Show()

	h, _ = h.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})

	if h.IsVisible() {
		t.Error("? should hide the modal when visible")
	}
}

func TestHelpModal_Update_QHides(t *testing.T) {
	h := NewHelpModal()
	h.Show()

	h, _ = h.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})

	if h.IsVisible() {
		t.Error("q should hide the modal")
	}
}

func TestHelpModal_SetSize(t *testing.T) {
	h := NewHelpModal()
	h.SetSize(100, 50)

	if h.width != 100 || h.height != 50 {
		t.Errorf("expected size 100x50, got %dx%d", h.width, h.height)
	}
}

func TestHelpModal_AddSection(t *testing.T) {
	h := NewHelpModal()
	h.AddSection("Custom", []HelpBinding{
		{Key: "x", Description: "do something"},
	})
	h.SetSize(80, 24)
	h.Show()

	view := h.View()

	if !strings.Contains(view, "Custom") {
		t.Error("view should contain custom section")
	}
	if !strings.Contains(view, "do something") {
		t.Error("view should contain custom binding description")
	}
}
