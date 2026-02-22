package table

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func newTestTable() Model {
	rows := make([]Row, 50)
	for i := range rows {
		rows[i] = Row{"col1", "col2"}
	}
	return New(
		WithColumns([]Column{{Title: "A", Width: 10}, {Title: "B", Width: 10}}),
		WithRows(rows),
		WithHeight(20),
		WithFocused(true),
	)
}

func TestFKeyDoesNotTriggerPageDown(t *testing.T) {
	m := newTestTable()

	if m.Cursor() != 0 {
		t.Fatalf("Expected cursor at 0, got %d", m.Cursor())
	}

	// Press 'f' — should NOT move cursor
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})

	if m.Cursor() != 0 {
		t.Errorf("Pressing 'f' should not move cursor, but cursor is at %d", m.Cursor())
	}
}

func TestPageDownStillWorksWithPgDnKey(t *testing.T) {
	m := newTestTable()

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyPgDown})

	if m.Cursor() == 0 {
		t.Error("Pressing pgdown should move cursor, but it stayed at 0")
	}
}

func TestUndocumentedKeysDoNotNavigate(t *testing.T) {
	tests := []struct {
		name string
		msg  tea.KeyMsg
	}{
		{"space does not page down", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}}},
		{"b does not page up", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}}},
		{"u does not half page up", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}}},
		{"d does not half page down", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}}},
		{"g does not go to top", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}}},
		{"G does not go to bottom", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestTable()

			// Move cursor to middle first so we can detect both up and down movement
			for i := 0; i < 25; i++ {
				m.MoveDown(1)
			}
			pos := m.Cursor()

			m, _ = m.Update(tt.msg)

			if m.Cursor() != pos {
				t.Errorf("Key should not move cursor, was at %d now at %d", pos, m.Cursor())
			}
		})
	}
}

func TestDocumentedKeysStillWork(t *testing.T) {
	tests := []struct {
		name    string
		msg     tea.KeyMsg
		movesUp bool
	}{
		{"up arrow", tea.KeyMsg{Type: tea.KeyUp}, true},
		{"k moves up", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}}, true},
		{"down arrow", tea.KeyMsg{Type: tea.KeyDown}, false},
		{"j moves down", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}, false},
		{"pgup", tea.KeyMsg{Type: tea.KeyPgUp}, true},
		{"pgdown", tea.KeyMsg{Type: tea.KeyPgDown}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestTable()

			// Move to middle so both directions work
			for i := 0; i < 25; i++ {
				m.MoveDown(1)
			}
			pos := m.Cursor()

			m, _ = m.Update(tt.msg)

			if tt.movesUp && m.Cursor() >= pos {
				t.Errorf("Key should move cursor up from %d, but cursor is at %d", pos, m.Cursor())
			}
			if !tt.movesUp && m.Cursor() <= pos {
				t.Errorf("Key should move cursor down from %d, but cursor is at %d", pos, m.Cursor())
			}
		})
	}
}

func TestDefaultKeyMapHasNoHiddenBindings(t *testing.T) {
	km := DefaultKeyMap()

	// PageUp should only have pgup
	if len(km.PageUp.Keys()) != 1 || km.PageUp.Keys()[0] != "pgup" {
		t.Errorf("PageUp should only bind 'pgup', got %v", km.PageUp.Keys())
	}

	// PageDown should only have pgdown
	if len(km.PageDown.Keys()) != 1 || km.PageDown.Keys()[0] != "pgdown" {
		t.Errorf("PageDown should only bind 'pgdown', got %v", km.PageDown.Keys())
	}
}
