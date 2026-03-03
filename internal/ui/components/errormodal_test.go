package components

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Elpulgo/azdo/internal/ui/styles"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestErrorModal_New(t *testing.T) {
	m := NewErrorModal(styles.DefaultStyles())

	if m == nil {
		t.Fatal("expected non-nil ErrorModal")
	}
	if m.IsVisible() {
		t.Error("error modal should be hidden by default")
	}
}

func TestErrorModal_Show(t *testing.T) {
	m := NewErrorModal(styles.DefaultStyles())
	m.Show("Test Error", "Something went wrong", "Try again")

	if !m.IsVisible() {
		t.Error("error modal should be visible after Show()")
	}
}

func TestErrorModal_Hide(t *testing.T) {
	m := NewErrorModal(styles.DefaultStyles())
	m.Show("Error", "msg", "hint")
	m.Hide()

	if m.IsVisible() {
		t.Error("error modal should be hidden after Hide()")
	}
}

func TestErrorModal_View_WhenHidden(t *testing.T) {
	m := NewErrorModal(styles.DefaultStyles())
	m.SetSize(80, 24)

	view := m.View()

	if view != "" {
		t.Error("view should be empty when hidden")
	}
}

func TestErrorModal_View_ContainsTitle(t *testing.T) {
	m := NewErrorModal(styles.DefaultStyles())
	m.SetSize(100, 30)
	m.Show("Configuration Error", "Invalid org", "Check config")

	view := m.View()

	if !strings.Contains(view, "Configuration Error") {
		t.Error("view should contain the title")
	}
}

func TestErrorModal_View_ContainsMessage(t *testing.T) {
	m := NewErrorModal(styles.DefaultStyles())
	m.SetSize(100, 30)
	m.Show("Error", "Your organization name is invalid", "Check config")

	view := m.View()

	if !strings.Contains(view, "Your organization name is invalid") {
		t.Error("view should contain the message")
	}
}

func TestErrorModal_View_ContainsHint(t *testing.T) {
	m := NewErrorModal(styles.DefaultStyles())
	m.SetSize(100, 30)
	m.Show("Error", "Something broke", "Run azdo auth to fix")

	view := m.View()

	if !strings.Contains(view, "Run azdo auth to fix") {
		t.Error("view should contain the hint")
	}
}

func TestErrorModal_View_ContainsDismissHint(t *testing.T) {
	m := NewErrorModal(styles.DefaultStyles())
	m.SetSize(100, 30)
	m.Show("Error", "msg", "hint")

	view := m.View()

	if !strings.Contains(strings.ToLower(view), "esc") {
		t.Error("view should contain dismiss hint with esc")
	}
}

func TestErrorModal_Update_EscHides(t *testing.T) {
	m := NewErrorModal(styles.DefaultStyles())
	m.Show("Error", "msg", "hint")

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if m.IsVisible() {
		t.Error("esc should hide the modal")
	}
}

func TestErrorModal_Update_QHides(t *testing.T) {
	m := NewErrorModal(styles.DefaultStyles())
	m.Show("Error", "msg", "hint")

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})

	if m.IsVisible() {
		t.Error("q should hide the modal")
	}
}

func TestErrorModal_Update_IgnoresWhenHidden(t *testing.T) {
	m := NewErrorModal(styles.DefaultStyles())
	// Not shown — should not panic or change state
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if m.IsVisible() {
		t.Error("should remain hidden")
	}
}

func TestErrorModal_View_ConstrainedByScreenWidth(t *testing.T) {
	m := NewErrorModal(styles.DefaultStyles())
	narrowWidth := 40
	m.SetSize(narrowWidth, 20)
	m.Show("Error", "This is a very long message that would normally make the modal much wider than the screen allows", "Check your config")

	view := m.View()
	lines := strings.Split(view, "\n")
	for _, line := range lines {
		// lipgloss.Width accounts for ANSI sequences
		lineWidth := lipgloss.Width(line)
		if lineWidth > narrowWidth {
			t.Errorf("Modal line width %d exceeds screen width %d", lineWidth, narrowWidth)
			break
		}
	}
}

func TestErrorModal_View_ResizesWhenScreenShrinks(t *testing.T) {
	m := NewErrorModal(styles.DefaultStyles())
	m.SetSize(120, 40)
	m.Show("Error", "A reasonably long error message for testing resize behavior", "hint")

	// Now shrink
	m.SetSize(50, 20)

	view := m.View()
	lines := strings.Split(view, "\n")
	for _, line := range lines {
		lineWidth := lipgloss.Width(line)
		if lineWidth > 50 {
			t.Errorf("After resize, modal line width %d exceeds screen width 50", lineWidth)
			break
		}
	}
}

// --- ClassifyError tests ---

func TestClassifyError_Nil(t *testing.T) {
	result := ClassifyError(nil)
	if result != nil {
		t.Error("nil error should return nil ErrorInfo")
	}
}

func TestClassifyError_404(t *testing.T) {
	err := fmt.Errorf("all projects failed: [resource not found (HTTP 404): the requested resource does not exist]")
	result := ClassifyError(err)

	if result == nil {
		t.Fatal("expected non-nil ErrorInfo for 404")
	}
	if result.Title != "Configuration Error" {
		t.Errorf("expected title 'Configuration Error', got %q", result.Title)
	}
	if !strings.Contains(result.Message, "organization") || !strings.Contains(result.Message, "project") {
		t.Error("404 message should mention organization and project names")
	}
}

func TestClassifyError_401(t *testing.T) {
	err := fmt.Errorf("authentication failed (HTTP 401): your PAT may be expired")
	result := ClassifyError(err)

	if result == nil {
		t.Fatal("expected non-nil ErrorInfo for 401")
	}
	if result.Title != "Authentication Error" {
		t.Errorf("expected title 'Authentication Error', got %q", result.Title)
	}
}

func TestClassifyError_403(t *testing.T) {
	err := fmt.Errorf("access denied (HTTP 403): insufficient permissions")
	result := ClassifyError(err)

	if result == nil {
		t.Fatal("expected non-nil ErrorInfo for 403")
	}
	if result.Title != "Authentication Error" {
		t.Errorf("expected title 'Authentication Error', got %q", result.Title)
	}
}

func TestClassifyError_TransientError(t *testing.T) {
	err := fmt.Errorf("connection timeout")
	result := ClassifyError(err)

	if result != nil {
		t.Error("transient error should return nil (not shown in modal)")
	}
}

func TestClassifyError_400(t *testing.T) {
	err := fmt.Errorf("all projects failed: [HTTP request failed with status 400]")
	result := ClassifyError(err)

	if result == nil {
		t.Fatal("HTTP 400 should return non-nil ErrorInfo")
	}
	if result.Title != "Configuration Error" {
		t.Errorf("Expected title 'Configuration Error', got %q", result.Title)
	}
}

func TestClassifyError_RateLimited(t *testing.T) {
	err := fmt.Errorf("rate limited (HTTP 429)")
	result := ClassifyError(err)

	if result != nil {
		t.Error("rate limit error should return nil (transient)")
	}
}

// --- CriticalErrorMsg / NewCriticalErrorCmd tests ---

func TestNewCriticalErrorCmd_ReturnsCmd_For404(t *testing.T) {
	err := fmt.Errorf("resource not found (HTTP 404)")
	cmd := NewCriticalErrorCmd(err)

	if cmd == nil {
		t.Fatal("expected non-nil cmd for 404 error")
	}

	msg := cmd()
	critMsg, ok := msg.(CriticalErrorMsg)
	if !ok {
		t.Fatalf("expected CriticalErrorMsg, got %T", msg)
	}
	if critMsg.Title != "Configuration Error" {
		t.Errorf("expected title 'Configuration Error', got %q", critMsg.Title)
	}
}

func TestNewCriticalErrorCmd_ReturnsNil_ForTransientError(t *testing.T) {
	err := fmt.Errorf("connection timeout")
	cmd := NewCriticalErrorCmd(err)

	if cmd != nil {
		t.Error("expected nil cmd for transient error")
	}
}

func TestNewCriticalErrorCmd_ReturnsNil_ForNilError(t *testing.T) {
	cmd := NewCriticalErrorCmd(nil)

	if cmd != nil {
		t.Error("expected nil cmd for nil error")
	}
}
