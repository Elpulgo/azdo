package providerselect

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestNewModel_InitialCursorIsAzure(t *testing.T) {
	m := NewModel()
	if m.cursor != 0 {
		t.Errorf("want cursor 0 (Azure), got %d", m.cursor)
	}
	if m.Selected() != ProviderAzure {
		t.Errorf("want initial Selected() == ProviderAzure, got %v", m.Selected())
	}
	if m.Cancelled() {
		t.Error("want Cancelled() == false initially")
	}
}

func TestUpdate_KeyHandling(t *testing.T) {
	type step struct {
		key         string
		wantCursor  int
		wantChosen  bool
		wantCancel  bool
		wantQuit    bool
		wantSelects Provider
	}

	tests := []struct {
		name  string
		steps []step
	}{
		{
			name: "down moves cursor to GitHub",
			steps: []step{
				{key: "j", wantCursor: 1},
			},
		},
		{
			name: "down then up returns to Azure",
			steps: []step{
				{key: "down", wantCursor: 1},
				{key: "up", wantCursor: 0},
			},
		},
		{
			name: "down does not go past last item",
			steps: []step{
				{key: "j", wantCursor: 1},
				{key: "j", wantCursor: 1},
			},
		},
		{
			name: "up does not go below zero",
			steps: []step{
				{key: "k", wantCursor: 0},
				{key: "k", wantCursor: 0},
			},
		},
		{
			name: "enter on Azure selects ProviderAzure and quits",
			steps: []step{
				{key: "enter", wantCursor: 0, wantChosen: true, wantQuit: true, wantSelects: ProviderAzure},
			},
		},
		{
			name: "down then enter selects ProviderGitHub and quits",
			steps: []step{
				{key: "j", wantCursor: 1},
				{key: "enter", wantCursor: 1, wantChosen: true, wantQuit: true, wantSelects: ProviderGitHub},
			},
		},
		{
			name: "esc sets cancelled and quits",
			steps: []step{
				{key: "esc", wantCancel: true, wantQuit: true},
			},
		},
		{
			name: "ctrl+c sets cancelled and quits",
			steps: []step{
				{key: "ctrl+c", wantCancel: true, wantQuit: true},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModel()
			for i, s := range tt.steps {
				msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s.key)}
				// Use the String() form for special keys.
				switch s.key {
				case "enter":
					msg = tea.KeyMsg{Type: tea.KeyEnter}
				case "esc":
					msg = tea.KeyMsg{Type: tea.KeyEsc}
				case "ctrl+c":
					msg = tea.KeyMsg{Type: tea.KeyCtrlC}
				case "up":
					msg = tea.KeyMsg{Type: tea.KeyUp}
				case "down":
					msg = tea.KeyMsg{Type: tea.KeyDown}
				}

				updated, cmd := m.Update(msg)
				m = updated.(Model)

				if m.cursor != s.wantCursor {
					t.Errorf("step %d: cursor: want %d, got %d", i, s.wantCursor, m.cursor)
				}
				if m.chosen != s.wantChosen {
					t.Errorf("step %d: chosen: want %v, got %v", i, s.wantChosen, m.chosen)
				}
				if m.cancelled != s.wantCancel {
					t.Errorf("step %d: cancelled: want %v, got %v", i, s.wantCancel, m.cancelled)
				}
				if s.wantChosen && m.Selected() != s.wantSelects {
					t.Errorf("step %d: Selected(): want %v, got %v", i, s.wantSelects, m.Selected())
				}
				if s.wantQuit {
					if cmd == nil {
						t.Errorf("step %d: want quit command, got nil", i)
					}
				}
			}
		})
	}
}
