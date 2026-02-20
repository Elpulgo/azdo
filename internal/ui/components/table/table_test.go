package table

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestFKeyDoesNotTriggerPageDown(t *testing.T) {
	rows := make([]Row, 50)
	for i := range rows {
		rows[i] = Row{"col1", "col2"}
	}

	m := New(
		WithColumns([]Column{{Title: "A", Width: 10}, {Title: "B", Width: 10}}),
		WithRows(rows),
		WithHeight(20),
		WithFocused(true),
	)

	// Cursor should start at 0
	if m.Cursor() != 0 {
		t.Fatalf("Expected cursor at 0, got %d", m.Cursor())
	}

	// Press 'f' — should NOT move cursor (no longer bound to page down)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})

	if m.Cursor() != 0 {
		t.Errorf("Pressing 'f' should not move cursor, but cursor is at %d", m.Cursor())
	}
}

func TestPageDownStillWorksWithPgDnKey(t *testing.T) {
	rows := make([]Row, 50)
	for i := range rows {
		rows[i] = Row{"col1", "col2"}
	}

	m := New(
		WithColumns([]Column{{Title: "A", Width: 10}, {Title: "B", Width: 10}}),
		WithRows(rows),
		WithHeight(20),
		WithFocused(true),
	)

	// Press pgdown — should move cursor
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyPgDown})

	if m.Cursor() == 0 {
		t.Error("Pressing pgdown should move cursor, but it stayed at 0")
	}
}

func TestPageDownStillWorksWithSpace(t *testing.T) {
	rows := make([]Row, 50)
	for i := range rows {
		rows[i] = Row{"col1", "col2"}
	}

	m := New(
		WithColumns([]Column{{Title: "A", Width: 10}, {Title: "B", Width: 10}}),
		WithRows(rows),
		WithHeight(20),
		WithFocused(true),
	)

	// Press space — should move cursor
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})

	if m.Cursor() == 0 {
		t.Error("Pressing space should move cursor, but it stayed at 0")
	}
}
