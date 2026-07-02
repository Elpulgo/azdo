package setupwizard

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// helper to type a string into the model
func typeString(m Model, s string) Model {
	for _, char := range s {
		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{char}}
		updated, _ := m.Update(msg)
		m = updated.(Model)
	}
	return m
}

// helper to press enter
func pressEnter(m Model) (Model, tea.Cmd) {
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	return updated.(Model), cmd
}

// helper to press a key by string
func pressKey(m Model, key string) (Model, tea.Cmd) {
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
	return updated.(Model), cmd
}

// ── Phase 3: Organization step ──────────────────────────────────

func TestNewModel_InitialState(t *testing.T) {
	m := NewModel()

	if m.step != stepOrganization {
		t.Errorf("initial step = %d, want stepOrganization (%d)", m.step, stepOrganization)
	}

	if len(m.themes) == 0 {
		t.Error("themes should be populated from styles.ListAvailableThemes()")
	}

	if m.Cancelled() {
		t.Error("should not be cancelled initially")
	}

	if m.GetConfig() != nil {
		t.Error("GetConfig() should return nil before completion")
	}
}

func TestOrg_EmptyShowsError(t *testing.T) {
	m := NewModel()

	// Press enter on empty org
	m, _ = pressEnter(m)

	if m.err == "" {
		t.Error("expected error for empty organization")
	}
	if m.step != stepOrganization {
		t.Error("should stay on org step")
	}
}

func TestOrg_ValidAdvancesToProjects(t *testing.T) {
	m := NewModel()
	m = typeString(m, "my-org")
	m, _ = pressEnter(m)

	if m.step != stepProjects {
		t.Errorf("step = %d, want stepProjects (%d)", m.step, stepProjects)
	}
	if m.organization != "my-org" {
		t.Errorf("organization = %q, want %q", m.organization, "my-org")
	}
}

// ── Phase 4: Projects step ──────────────────────────────────────

func TestProjects_EmptyShowsError(t *testing.T) {
	m := NewModel()
	m = typeString(m, "my-org")
	m, _ = pressEnter(m) // advance to projects

	m, _ = pressEnter(m) // submit empty

	if m.err == "" {
		t.Error("expected error for empty projects")
	}
	if m.step != stepProjects {
		t.Error("should stay on projects step")
	}
}

func TestProjects_CSVParsingTrimsWhitespace(t *testing.T) {
	m := NewModel()
	m = typeString(m, "my-org")
	m, _ = pressEnter(m)

	m = typeString(m, "  proj-a , proj-b  , proj-c  ")
	m, _ = pressEnter(m)

	if m.step != stepPollingInterval {
		t.Errorf("step = %d, want stepPollingInterval (%d)", m.step, stepPollingInterval)
	}
	if len(m.projects) != 3 {
		t.Fatalf("projects length = %d, want 3", len(m.projects))
	}
	if m.projects[0] != "proj-a" || m.projects[1] != "proj-b" || m.projects[2] != "proj-c" {
		t.Errorf("projects = %v, want [proj-a proj-b proj-c]", m.projects)
	}
}

// ── Phase 5: Polling interval step ──────────────────────────────

func TestPolling_DefaultAccepted(t *testing.T) {
	m := NewModel()
	m = typeString(m, "my-org")
	m, _ = pressEnter(m)
	m = typeString(m, "proj-a")
	m, _ = pressEnter(m)

	// polling input is pre-filled with "60", just press enter
	m, _ = pressEnter(m)

	if m.step != stepTheme {
		t.Errorf("step = %d, want stepTheme (%d)", m.step, stepTheme)
	}
	if m.pollingInterval != 60 {
		t.Errorf("pollingInterval = %d, want 60", m.pollingInterval)
	}
}

func TestPolling_NonNumericShowsError(t *testing.T) {
	m := NewModel()
	m = typeString(m, "my-org")
	m, _ = pressEnter(m)
	m = typeString(m, "proj-a")
	m, _ = pressEnter(m)

	// Clear the default and type non-numeric
	// Select all text by moving to start and deleting
	m = clearInput(m)
	m = typeString(m, "abc")
	m, _ = pressEnter(m)

	if m.err == "" {
		t.Error("expected error for non-numeric polling interval")
	}
	if m.step != stepPollingInterval {
		t.Error("should stay on polling interval step")
	}
}

func TestPolling_ZeroShowsError(t *testing.T) {
	m := NewModel()
	m = typeString(m, "my-org")
	m, _ = pressEnter(m)
	m = typeString(m, "proj-a")
	m, _ = pressEnter(m)

	m = clearInput(m)
	m = typeString(m, "0")
	m, _ = pressEnter(m)

	if m.err == "" {
		t.Error("expected error for zero polling interval")
	}
}

// helper to clear a text input (simulate ctrl+a then delete)
func clearInput(m Model) Model {
	// Press Home to go to start, then ctrl+k to delete to end
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyHome})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlK})
	m = updated.(Model)
	return m
}

// ── Phase 6: Theme selection ────────────────────────────────────

func TestTheme_NavigationAndSelection(t *testing.T) {
	m := NewModel()
	// Navigate to theme step
	m = typeString(m, "my-org")
	m, _ = pressEnter(m)
	m = typeString(m, "proj-a")
	m, _ = pressEnter(m)
	m, _ = pressEnter(m) // accept default polling

	if m.step != stepTheme {
		t.Fatalf("step = %d, want stepTheme", m.step)
	}

	initialCursor := m.themeCursor

	// Press down
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)
	if m.themeCursor != initialCursor+1 {
		t.Errorf("cursor = %d after down, want %d", m.themeCursor, initialCursor+1)
	}

	// Press up to go back
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = updated.(Model)
	if m.themeCursor != initialCursor {
		t.Errorf("cursor = %d after up, want %d", m.themeCursor, initialCursor)
	}

	// Move down and select with enter
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)
	m, _ = pressEnter(m)

	if m.step != stepConfirm {
		t.Errorf("step = %d, want stepConfirm (%d)", m.step, stepConfirm)
	}
	if m.theme != m.themes[1] {
		t.Errorf("theme = %q, want %q", m.theme, m.themes[1])
	}
}

// ── Phase 7: Confirm + completion ───────────────────────────────

func TestConfirm_EnterProducesQuitAndConfig(t *testing.T) {
	m := navigateToConfirm(t)

	m, cmd := pressEnter(m)

	if cmd == nil {
		t.Fatal("expected a command on confirm")
	}

	cfg := m.GetConfig()
	if cfg == nil {
		t.Fatal("GetConfig() should return non-nil after confirm")
	}
	if cfg.Organization != "my-org" {
		t.Errorf("config.Organization = %q, want %q", cfg.Organization, "my-org")
	}
	if len(cfg.Projects) != 1 || cfg.Projects[0] != "proj-a" {
		t.Errorf("config.Projects = %v, want [proj-a]", cfg.Projects)
	}
	if cfg.PollingInterval != 60 {
		t.Errorf("config.PollingInterval = %d, want 60", cfg.PollingInterval)
	}
}

func TestConfirm_BackGoesToOrganization(t *testing.T) {
	m := navigateToConfirm(t)

	m, _ = pressKey(m, "b")

	if m.step != stepOrganization {
		t.Errorf("step = %d, want stepOrganization (%d) after pressing b", m.step, stepOrganization)
	}
}

// ── Phase 8: Cancellation + View ────────────────────────────────

func TestEsc_CancelsAtAnyStep(t *testing.T) {
	steps := []struct {
		name  string
		model Model
	}{
		{"org step", NewModel()},
		{"projects step", advanceTo(stepProjects)},
		{"polling step", advanceTo(stepPollingInterval)},
		{"theme step", advanceTo(stepTheme)},
		{"confirm step", advanceTo(stepConfirm)},
	}

	for _, tt := range steps {
		t.Run(tt.name, func(t *testing.T) {
			m := tt.model
			updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
			m = updated.(Model)

			if !m.Cancelled() {
				t.Error("expected Cancelled() to be true after Esc")
			}
			if m.GetConfig() != nil {
				t.Error("expected GetConfig() to be nil after cancel")
			}
			if cmd == nil {
				t.Error("expected quit command after Esc")
			}
		})
	}
}

func TestCtrlC_CancelsAtAnyStep(t *testing.T) {
	m := NewModel()
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = updated.(Model)

	if !m.Cancelled() {
		t.Error("expected Cancelled() to be true after Ctrl+C")
	}
	if cmd == nil {
		t.Error("expected quit command after Ctrl+C")
	}
}

func TestView_ContainsExpectedLabels(t *testing.T) {
	tests := []struct {
		name     string
		model    Model
		contains []string
	}{
		{
			"org step",
			NewModel(),
			[]string{"Organization"},
		},
		{
			"projects step",
			advanceTo(stepProjects),
			[]string{"Projects"},
		},
		{
			"polling step",
			advanceTo(stepPollingInterval),
			[]string{"Polling"},
		},
		{
			"theme step",
			advanceTo(stepTheme),
			[]string{"Theme"},
		},
		{
			"confirm step",
			advanceTo(stepConfirm),
			[]string{"Confirm", "my-org", "proj-a"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			view := tt.model.View()
			for _, s := range tt.contains {
				if !strings.Contains(view, s) {
					t.Errorf("view should contain %q, got:\n%s", s, view)
				}
			}
		})
	}
}

// ── Test helpers ────────────────────────────────────────────────

func navigateToConfirm(t *testing.T) Model {
	t.Helper()
	m := NewModel()
	m = typeString(m, "my-org")
	m, _ = pressEnter(m)
	m = typeString(m, "proj-a")
	m, _ = pressEnter(m)
	m, _ = pressEnter(m) // accept default polling

	// Select first theme
	m, _ = pressEnter(m)

	if m.step != stepConfirm {
		t.Fatalf("expected to be on confirm step, got %d", m.step)
	}
	return m
}

func advanceTo(target wizardStep) Model {
	m := NewModel()
	if target >= stepProjects {
		m = typeString(m, "my-org")
		m, _ = pressEnter(m)
	}
	if target >= stepPollingInterval {
		m = typeString(m, "proj-a")
		m, _ = pressEnter(m)
	}
	if target >= stepTheme {
		m, _ = pressEnter(m) // accept default polling
	}
	if target >= stepConfirm {
		m, _ = pressEnter(m) // select first theme
	}
	return m
}
