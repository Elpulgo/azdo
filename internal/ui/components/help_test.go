package components

import (
	"strings"
	"testing"

	"github.com/Elpulgo/azdo/internal/ui/styles"
	tea "github.com/charmbracelet/bubbletea"
)

func TestHelpModal_New(t *testing.T) {
	h := NewHelpModal(styles.DefaultStyles())

	if h == nil {
		t.Fatal("expected non-nil HelpModal")
	}
	if h.IsVisible() {
		t.Error("help modal should be hidden by default")
	}
}

func TestHelpModal_Show(t *testing.T) {
	h := NewHelpModal(styles.DefaultStyles())
	h.Show()

	if !h.IsVisible() {
		t.Error("help modal should be visible after Show()")
	}
}

func TestHelpModal_Hide(t *testing.T) {
	h := NewHelpModal(styles.DefaultStyles())
	h.Show()
	h.Hide()

	if h.IsVisible() {
		t.Error("help modal should be hidden after Hide()")
	}
}

func TestHelpModal_Toggle(t *testing.T) {
	h := NewHelpModal(styles.DefaultStyles())

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
	h := NewHelpModal(styles.DefaultStyles())
	h.SetSize(80, 24)

	view := h.View()

	if view != "" {
		t.Error("view should be empty when hidden")
	}
}

func TestHelpModal_View_ContainsTitle(t *testing.T) {
	h := NewHelpModal(styles.DefaultStyles())
	h.SetSize(80, 24)
	h.Show()

	view := strings.ToLower(h.View())

	if !strings.Contains(view, "keyboard shortcuts") {
		t.Error("view should contain the 'Keyboard Shortcuts' title")
	}
}

func TestHelpModal_View_ContainsKeybindings(t *testing.T) {
	h := NewHelpModal(styles.DefaultStyles())
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
	h := NewHelpModal(styles.DefaultStyles())
	h.Show()

	h, _ = h.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if h.IsVisible() {
		t.Error("esc should hide the modal")
	}
}

func TestHelpModal_Update_QuestionMarkHides(t *testing.T) {
	h := NewHelpModal(styles.DefaultStyles())
	h.Show()

	h, _ = h.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})

	if h.IsVisible() {
		t.Error("? should hide the modal when visible")
	}
}

func TestHelpModal_Update_QHides(t *testing.T) {
	h := NewHelpModal(styles.DefaultStyles())
	h.Show()

	h, _ = h.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})

	if h.IsVisible() {
		t.Error("q should hide the modal")
	}
}

func TestHelpModal_SetSize_AffectsViewCentering(t *testing.T) {
	h := NewHelpModal(styles.DefaultStyles())
	h.Show()

	// Render without size — no centering applied
	h.SetSize(0, 0)
	viewNoSize := h.View()

	// Render with a large terminal size — centering should add leading whitespace
	h.SetSize(200, 60)
	viewWithSize := h.View()

	if viewWithSize == viewNoSize {
		t.Error("setting a large terminal size should change the rendered output (centering)")
	}

	// The centered view should have leading blank lines (vertical centering)
	if !strings.HasPrefix(viewWithSize, "\n") {
		t.Error("centered view should start with blank lines for vertical padding")
	}
}

func TestHelpModal_AddSection(t *testing.T) {
	h := NewHelpModal(styles.DefaultStyles())
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

func TestHelpModal_SetConfigPath_ShowsInView(t *testing.T) {
	h := NewHelpModal(styles.DefaultStyles())
	h.SetConfigPath("/home/user/.config/azdo-tui/config.yaml")
	h.SetSize(80, 40)
	h.Show()

	view := h.View()

	if !strings.Contains(view, "config.yaml") {
		t.Error("help modal should display config path when set")
	}
}

func TestHelpModal_NoConfigPath_NotShown(t *testing.T) {
	h := NewHelpModal(styles.DefaultStyles())
	h.SetSize(80, 40)
	h.Show()

	view := h.View()

	if strings.Contains(view, "Config") {
		t.Error("help modal should not show config section when no path set")
	}
}
