package patinput

import (
	"strings"
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

func TestUpdate_EnterQuitsProgram(t *testing.T) {
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
	_, cmd := model.Update(msg)

	if cmd == nil {
		t.Fatal("Expected a command to be returned after submission")
	}

	// Execute the command and check if it contains a quit message
	// The command should return a tea.Quit or similar message
	result := cmd()

	// Check if the result is a batch command (which would contain both PAT submission and quit)
	// We need to verify that tea.Quit is called
	batchMsg, isBatch := result.(tea.BatchMsg)
	if !isBatch {
		t.Fatal("Expected a batch command containing both PAT submission and quit")
	}

	// Execute all commands in the batch
	foundPATMsg := false
	foundQuitMsg := false
	for _, cmdFunc := range batchMsg {
		msg := cmdFunc()
		if _, ok := msg.(PATSubmittedMsg); ok {
			foundPATMsg = true
		}
		if _, ok := msg.(tea.QuitMsg); ok {
			foundQuitMsg = true
		}
	}

	if !foundPATMsg {
		t.Error("Expected batch to contain PATSubmittedMsg")
	}

	if !foundQuitMsg {
		t.Error("Expected batch to contain tea.QuitMsg to exit the program")
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

func TestNewModelForUpdate(t *testing.T) {
	model := NewModelForUpdate()

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

func TestView_ShowsPermissionRequirements(t *testing.T) {
	model := NewModel()
	view := model.View()

	requiredTexts := []string{
		"Build",
		"Read",
		"Code",
		"Write",
		"Work Items",
	}

	for _, text := range requiredTexts {
		if !containsString(view, text) {
			t.Errorf("Expected view to contain PAT permission info %q", text)
		}
	}
}

func TestView_UpdateModeShowsPermissionRequirements(t *testing.T) {
	model := NewModelForUpdate()
	view := model.View()

	requiredTexts := []string{
		"Build",
		"Read",
		"Code",
		"Write",
		"Work Items",
	}

	for _, text := range requiredTexts {
		if !containsString(view, text) {
			t.Errorf("Expected update view to contain PAT permission info %q", text)
		}
	}
}

// containsString checks if s contains substr (ANSI-aware check)
func containsString(s, substr string) bool {
	// Simple contains check; lipgloss styling adds ANSI codes
	// but the actual text content should still be present
	return strings.Contains(s, substr)
}
