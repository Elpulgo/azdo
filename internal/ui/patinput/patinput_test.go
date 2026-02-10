package patinput

import (
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func TestNewModel(t *testing.T) {
	model := NewModel()

	if model.textInput.EchoMode != textinput.EchoPassword {
		t.Error("Expected PAT input to be in password mode")
	}

	if model.err != "" {
		t.Errorf("Expected no error, got: %s", model.err)
	}

	if model.submitted {
		t.Error("Expected submitted to be false initially")
	}
}

func TestUpdate_EnterSubmitsPAT(t *testing.T) {
	model := NewModel()

	// Simulate typing a PAT
	testPAT := "test-pat-token-123"
	for _, char := range testPAT {
		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{char}}
		m, _ := model.Update(msg)
		model = m.(Model)
	}

	// Simulate pressing Enter
	msg := tea.KeyMsg{Type: tea.KeyEnter}
	m, cmd := model.Update(msg)
	model = m.(Model)

	if !model.submitted {
		t.Error("Expected submitted to be true after pressing Enter")
	}

	if cmd == nil {
		t.Error("Expected a command to be returned after submission")
	}

	// Verify the command returns the PAT
	if result := cmd(); result == nil {
		t.Error("Expected command to return a message")
	}
}

func TestUpdate_EscCancels(t *testing.T) {
	model := NewModel()

	msg := tea.KeyMsg{Type: tea.KeyEsc}
	_, cmd := model.Update(msg)

	if cmd == nil {
		t.Error("Expected a quit command to be returned")
	}
}

func TestUpdate_EmptyPATShowsError(t *testing.T) {
	model := NewModel()

	// Press Enter without typing anything
	msg := tea.KeyMsg{Type: tea.KeyEnter}
	m, _ := model.Update(msg)
	model = m.(Model)

	if model.err == "" {
		t.Error("Expected an error message when submitting empty PAT")
	}

	if model.submitted {
		t.Error("Expected submitted to remain false when PAT is empty")
	}
}

func TestGetPAT(t *testing.T) {
	model := NewModel()

	// Type a PAT
	testPAT := "my-secret-token"
	for _, char := range testPAT {
		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{char}}
		m, _ := model.Update(msg)
		model = m.(Model)
	}

	// Submit
	msg := tea.KeyMsg{Type: tea.KeyEnter}
	m, _ := model.Update(msg)
	model = m.(Model)

	// Get the PAT
	pat := model.GetPAT()
	if pat != testPAT {
		t.Errorf("Expected PAT to be %s, got %s", testPAT, pat)
	}
}

func TestView_ShowsPromptAndInput(t *testing.T) {
	model := NewModel()
	view := model.View()

	if view == "" {
		t.Error("Expected non-empty view")
	}

	// View should contain some indication that this is PAT input
	// (We'll verify the exact content when implementing)
}
