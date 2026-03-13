package table

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
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

func TestUndocumentedKeysDoNotNavigate(t *testing.T) {
	tests := []struct {
		name string
		msg  tea.KeyMsg
	}{
		{"f does not page down", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}}},
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

func TestSelectedRowHighlightsAllColumns(t *testing.T) {
	// Force color output so ANSI escapes are actually emitted.
	lipgloss.SetColorProfile(termenv.TrueColor)

	m := New(
		WithColumns([]Column{{Title: "A", Width: 10}, {Title: "B", Width: 10}}),
		WithRows([]Row{{"hello", "world"}}),
		WithHeight(5),
		WithFocused(true),
	)

	viewportWidth := 80
	m.SetWidth(viewportWidth)

	// Use a background color so we can detect it in the ANSI output.
	styles := DefaultStyles()
	styles.Selected = lipgloss.NewStyle().
		Background(lipgloss.Color("62"))
	styles.Cell = lipgloss.NewStyle().Padding(0, 1)
	m.SetStyles(styles)

	rendered := m.renderRow(0)

	// The rendered row should fill the full viewport width.
	printableWidth := ansi.StringWidth(rendered)
	if printableWidth != viewportWidth {
		t.Errorf("selected row width = %d, want %d (should fill full viewport width)", printableWidth, viewportWidth)
	}

	// The background escape sequence for color 62 should appear around
	// BOTH cell values, not just the first one.
	helloIdx := strings.Index(rendered, "hello")
	worldIdx := strings.Index(rendered, "world")
	if helloIdx < 0 || worldIdx < 0 {
		t.Fatal("expected both 'hello' and 'world' in rendered row")
	}

	// After the first cell is rendered, lipgloss resets styles. If the
	// selected style is only applied as an outer wrapper, the background
	// won't be re-emitted before the second cell. We check that the
	// background escape is present in the segment between the two values.
	betweenCells := rendered[helloIdx+len("hello") : worldIdx]
	bgEscape := "48;5;62" // SGR parameter for 256-color background 62
	if !strings.Contains(betweenCells, bgEscape) {
		t.Errorf("background color not active before second column; selected style must apply to every cell, not just the row wrapper\nbetween cells: %q", betweenCells)
	}
}

func TestUnselectedRowDoesNotFillFullWidth(t *testing.T) {
	m := New(
		WithColumns([]Column{{Title: "A", Width: 10}, {Title: "B", Width: 10}}),
		WithRows([]Row{{"hello", "world"}, {"foo", "bar"}}),
		WithHeight(5),
		WithFocused(true),
	)

	viewportWidth := 80
	m.SetWidth(viewportWidth)

	styles := DefaultStyles()
	styles.Cell = lipgloss.NewStyle().Padding(0, 1)
	m.SetStyles(styles)

	rendered := m.renderRow(1) // row 1 is not selected (cursor defaults to 0)
	printableWidth := ansi.StringWidth(rendered)

	// Unselected row should NOT be padded to full width
	if printableWidth >= viewportWidth {
		t.Errorf("unselected row width = %d, should be less than viewport width %d", printableWidth, viewportWidth)
	}
}

func TestDefaultKeyMapHasNoHiddenBindings(t *testing.T) {
	km := DefaultKeyMap()

	tests := []struct {
		name     string
		keys     []string
		expected []string
	}{
		{"LineUp", km.LineUp.Keys(), []string{"up", "k"}},
		{"LineDown", km.LineDown.Keys(), []string{"down", "j"}},
		{"PageUp", km.PageUp.Keys(), []string{"pgup"}},
		{"PageDown", km.PageDown.Keys(), []string{"pgdown"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if len(tt.keys) != len(tt.expected) {
				t.Errorf("%s should bind %v, got %v", tt.name, tt.expected, tt.keys)
				return
			}
			for i, k := range tt.keys {
				if k != tt.expected[i] {
					t.Errorf("%s should bind %v, got %v", tt.name, tt.expected, tt.keys)
					return
				}
			}
		})
	}
}
