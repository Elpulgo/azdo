package components

import (
	"testing"

	"github.com/Elpulgo/azdo/internal/azdevops"
	"github.com/Elpulgo/azdo/internal/ui/styles"
	tea "github.com/charmbracelet/bubbletea"
)

func newTestVotePicker() VotePicker {
	theme := styles.GetDefaultTheme()
	appStyles := styles.NewStyles(theme)
	return NewVotePicker(appStyles)
}

func TestVotePickerInitialization(t *testing.T) {
	picker := newTestVotePicker()

	if picker.IsVisible() {
		t.Error("Expected vote picker to be hidden initially")
	}

	if picker.GetCursor() != 0 {
		t.Errorf("Expected initial cursor at 0, got %d", picker.GetCursor())
	}
}

func TestVotePickerShowHide(t *testing.T) {
	picker := newTestVotePicker()

	if picker.IsVisible() {
		t.Error("Expected vote picker to be hidden initially")
	}

	picker.Show()
	if !picker.IsVisible() {
		t.Error("Expected vote picker to be visible after Show()")
	}

	picker.Hide()
	if picker.IsVisible() {
		t.Error("Expected vote picker to be hidden after Hide()")
	}
}

func TestVotePickerHasFiveOptions(t *testing.T) {
	picker := newTestVotePicker()

	// Should have 5 vote options
	if len(picker.options) != 5 {
		t.Errorf("Expected 5 vote options, got %d", len(picker.options))
	}

	// Verify the vote values match the azdevops constants
	expectedVotes := []int{
		azdevops.VoteApprove,
		azdevops.VoteApproveWithSuggestions,
		azdevops.VoteWaitForAuthor,
		azdevops.VoteReject,
		azdevops.VoteNoVote,
	}

	for i, expected := range expectedVotes {
		if picker.options[i].Vote != expected {
			t.Errorf("Option %d vote = %d, want %d", i, picker.options[i].Vote, expected)
		}
	}
}

func TestVotePickerNavigation(t *testing.T) {
	picker := newTestVotePicker()
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

	// Navigate to bottom
	picker, _ = picker.Update(tea.KeyMsg{Type: tea.KeyDown})
	picker, _ = picker.Update(tea.KeyMsg{Type: tea.KeyDown})
	picker, _ = picker.Update(tea.KeyMsg{Type: tea.KeyDown})
	if picker.GetCursor() != 4 {
		t.Errorf("Expected cursor at 4 (last item), got %d", picker.GetCursor())
	}

	// Can't go past last item
	picker, _ = picker.Update(tea.KeyMsg{Type: tea.KeyDown})
	if picker.GetCursor() != 4 {
		t.Errorf("Expected cursor to stay at 4 when at bottom, got %d", picker.GetCursor())
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

func TestVotePickerNavigationJK(t *testing.T) {
	picker := newTestVotePicker()
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

func TestVotePickerSelection(t *testing.T) {
	picker := newTestVotePicker()
	picker.Show()

	// Move to second option (Approve with suggestions)
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

	voteMsg, ok := msg.(VoteSelectedMsg)
	if !ok {
		t.Fatalf("Expected VoteSelectedMsg, got %T", msg)
	}

	if voteMsg.Vote != azdevops.VoteApproveWithSuggestions {
		t.Errorf("Expected vote %d (ApproveWithSuggestions), got %d", azdevops.VoteApproveWithSuggestions, voteMsg.Vote)
	}

	// Picker should be hidden after selection
	if updatedPicker.IsVisible() {
		t.Error("Expected picker to be hidden after selection")
	}
}

func TestVotePickerSelectionAllOptions(t *testing.T) {
	expectedVotes := []int{
		azdevops.VoteApprove,
		azdevops.VoteApproveWithSuggestions,
		azdevops.VoteWaitForAuthor,
		azdevops.VoteReject,
		azdevops.VoteNoVote,
	}

	for i, expectedVote := range expectedVotes {
		picker := newTestVotePicker()
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
		voteMsg, ok := msg.(VoteSelectedMsg)
		if !ok {
			t.Errorf("Option %d: expected VoteSelectedMsg, got %T", i, msg)
			continue
		}

		if voteMsg.Vote != expectedVote {
			t.Errorf("Option %d: expected vote %d, got %d", i, expectedVote, voteMsg.Vote)
		}
	}
}

func TestVotePickerEscape(t *testing.T) {
	picker := newTestVotePicker()
	picker.Show()

	if !picker.IsVisible() {
		t.Fatal("Expected picker to be visible before escape")
	}

	updatedPicker, cmd := picker.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if updatedPicker.IsVisible() {
		t.Error("Expected picker to be hidden after escape")
	}

	// Escape should not produce a command
	if cmd != nil {
		t.Error("Expected no command after escape")
	}
}

func TestVotePickerQuitKey(t *testing.T) {
	picker := newTestVotePicker()
	picker.Show()

	updatedPicker, _ := picker.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})

	if updatedPicker.IsVisible() {
		t.Error("Expected picker to be hidden after q")
	}
}

func TestVotePickerIgnoresInputWhenHidden(t *testing.T) {
	picker := newTestVotePicker()
	// Don't show it

	picker, cmd := picker.Update(tea.KeyMsg{Type: tea.KeyDown})
	if cmd != nil {
		t.Error("Expected no command when picker is hidden")
	}

	if picker.GetCursor() != 0 {
		t.Errorf("Expected cursor to stay at 0 when hidden, got %d", picker.GetCursor())
	}
}

func TestVotePickerView(t *testing.T) {
	picker := newTestVotePicker()

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

	// Should contain vote option labels
	if !contains(view, "Approve") {
		t.Error("Expected view to contain 'Approve'")
	}
	if !contains(view, "Reject") {
		t.Error("Expected view to contain 'Reject'")
	}
	if !contains(view, "Reset feedback") {
		t.Error("Expected view to contain 'Reset feedback'")
	}
}

func TestVotePickerSetSize(t *testing.T) {
	picker := newTestVotePicker()
	picker.SetSize(100, 30)
	// Should not panic
}
