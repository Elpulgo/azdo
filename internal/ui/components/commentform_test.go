package components

import (
	"strings"
	"testing"

	"github.com/Elpulgo/azdo/internal/ui/styles"
	tea "github.com/charmbracelet/bubbletea"
)

func typeRunes(f CommentForm, s string) CommentForm {
	f, _ = f.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)})
	return f
}

func TestCommentForm_CtrlSSubmitsNonEmpty(t *testing.T) {
	f := NewCommentForm(styles.DefaultStyles())
	f.Show()
	f.Focus()
	f = typeRunes(f, "hello world")

	f, cmd := f.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd == nil {
		t.Fatal("Expected a command from Ctrl+S, got nil")
	}
	msg := cmd()
	submitted, ok := msg.(CommentSubmittedMsg)
	if !ok {
		t.Fatalf("Expected CommentSubmittedMsg, got %T", msg)
	}
	if submitted.Text != "hello world" {
		t.Errorf("submitted.Text = %q, want %q", submitted.Text, "hello world")
	}
}

func TestCommentForm_CtrlSIgnoredWhenEmpty(t *testing.T) {
	f := NewCommentForm(styles.DefaultStyles())
	f.Show()
	f.Focus()
	// no text typed, and whitespace-only should also be ignored
	f = typeRunes(f, "   ")

	_, cmd := f.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if cmd != nil {
		if msg := cmd(); msg != nil {
			if _, ok := msg.(CommentSubmittedMsg); ok {
				t.Fatal("Expected no submission for empty/whitespace text")
			}
		}
	}
}

func TestCommentForm_EscCancels(t *testing.T) {
	f := NewCommentForm(styles.DefaultStyles())
	f.Show()
	f.Focus()
	f = typeRunes(f, "draft")

	_, cmd := f.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("Expected a command from Esc, got nil")
	}
	if _, ok := cmd().(CommentFormCancelledMsg); !ok {
		t.Fatalf("Expected CommentFormCancelledMsg, got %T", cmd())
	}
}

func TestCommentForm_EnterInsertsNewline(t *testing.T) {
	f := NewCommentForm(styles.DefaultStyles())
	f.Show()
	f.Focus()
	f = typeRunes(f, "line1")
	f, cmd := f.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		if _, ok := cmd().(CommentSubmittedMsg); ok {
			t.Fatal("Enter should not submit the form")
		}
	}
	f = typeRunes(f, "line2")

	if !strings.Contains(f.Value(), "\n") {
		t.Errorf("Expected value to contain a newline, got %q", f.Value())
	}
	if !strings.Contains(f.Value(), "line1") || !strings.Contains(f.Value(), "line2") {
		t.Errorf("Expected value to contain both lines, got %q", f.Value())
	}
}

func TestCommentForm_ShowHideReset(t *testing.T) {
	f := NewCommentForm(styles.DefaultStyles())
	if f.IsVisible() {
		t.Error("Expected form hidden by default")
	}
	f.Show()
	if !f.IsVisible() {
		t.Error("Expected form visible after Show()")
	}
	f.Focus()
	f = typeRunes(f, "stuff")
	f.Reset()
	if f.Value() != "" {
		t.Errorf("Expected empty value after Reset(), got %q", f.Value())
	}
	f.Hide()
	if f.IsVisible() {
		t.Error("Expected form hidden after Hide()")
	}
}
