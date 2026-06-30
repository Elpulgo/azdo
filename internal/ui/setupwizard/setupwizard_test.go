package setupwizard

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// ── low-level key helpers ────────────────────────────────────────

func typeString(m Model, s string) Model {
	for _, char := range s {
		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{char}}
		updated, _ := m.Update(msg)
		m = updated.(Model)
	}
	return m
}

func pressEnter(m Model) (Model, tea.Cmd) {
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	return updated.(Model), cmd
}

func pressKey(m Model, key string) (Model, tea.Cmd) {
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
	return updated.(Model), cmd
}

func pressDown(m Model) Model {
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	return updated.(Model)
}

func pressUp(m Model) Model {
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	return updated.(Model)
}

// clearInput clears the currently focused text input.
func clearInput(m Model) Model {
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyHome})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlK})
	m = updated.(Model)
	return m
}

// ── Provider selection helpers ───────────────────────────────────

// selectProvider advances past the provider step with the cursor at the given index.
func selectProvider(m Model, cursor int) Model {
	for i := 0; i < cursor; i++ {
		m = pressDown(m)
	}
	m, _ = pressEnter(m)
	return m
}

// selectAzure selects "Azure DevOps" (cursor 0).
func selectAzure(m Model) Model { return selectProvider(m, 0) }

// selectGitHub selects "GitHub" (cursor 1).
func selectGitHub(m Model) Model { return selectProvider(m, 1) }

// selectBoth selects "Both" (cursor 2).
func selectBoth(m Model) Model { return selectProvider(m, 2) }

// ── Azure-path helpers ───────────────────────────────────────────

// navigateToConfirmAzure drives Azure-only flow all the way to the confirm step.
func navigateToConfirmAzure(t *testing.T) Model {
	t.Helper()
	m := NewModel()
	m = selectAzure(m)
	m = typeString(m, "my-org")
	m, _ = pressEnter(m)
	m = typeString(m, "proj-a")
	m, _ = pressEnter(m)
	m, _ = pressEnter(m) // accept default polling
	m, _ = pressEnter(m) // select first theme
	if m.currentStep() != stepConfirm {
		t.Fatalf("expected stepConfirm, got %v", m.currentStep())
	}
	return m
}

// ── GitHub-path helpers ──────────────────────────────────────────

// navigateToConfirmGitHub drives GitHub-only flow all the way to the confirm step.
func navigateToConfirmGitHub(t *testing.T) Model {
	t.Helper()
	m := NewModel()
	m = selectGitHub(m)
	m = typeString(m, "ghp_testtoken")
	m, _ = pressEnter(m)
	m = typeString(m, "owner/repo-a")
	m, _ = pressEnter(m)
	m, _ = pressEnter(m) // accept default polling
	m, _ = pressEnter(m) // select first theme
	if m.currentStep() != stepConfirm {
		t.Fatalf("expected stepConfirm, got %v", m.currentStep())
	}
	return m
}

// navigateToConfirmBoth drives the Both flow all the way to the confirm step.
func navigateToConfirmBoth(t *testing.T) Model {
	t.Helper()
	m := NewModel()
	m = selectBoth(m)
	m = typeString(m, "my-org")
	m, _ = pressEnter(m)
	m = typeString(m, "proj-a")
	m, _ = pressEnter(m)
	m = typeString(m, "ghp_testtoken")
	m, _ = pressEnter(m)
	m = typeString(m, "owner/repo-a")
	m, _ = pressEnter(m)
	m, _ = pressEnter(m) // accept default polling
	m, _ = pressEnter(m) // select first theme
	if m.currentStep() != stepConfirm {
		t.Fatalf("expected stepConfirm, got %v", m.currentStep())
	}
	return m
}

// ── Initial state ────────────────────────────────────────────────

func TestNewModel_InitialState(t *testing.T) {
	m := NewModel()

	if m.currentStep() != stepProvider {
		t.Errorf("initial step = %v, want stepProvider", m.currentStep())
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
	if m.GitHubToken() != "" {
		t.Error("GitHubToken() should be empty initially")
	}
}

// ── Provider selection step ──────────────────────────────────────

func TestProvider_NavigationWraps(t *testing.T) {
	m := NewModel()

	// Start at Azure (index 0); up should not go below 0.
	m = pressUp(m)
	if m.providerCursor != 0 {
		t.Errorf("cursor = %d after up at top, want 0", m.providerCursor)
	}

	// Down twice → Both (index 2); another down should stay at 2.
	m = pressDown(m)
	m = pressDown(m)
	if m.providerCursor != 2 {
		t.Errorf("cursor = %d, want 2 (Both)", m.providerCursor)
	}
	m = pressDown(m)
	if m.providerCursor != 2 {
		t.Errorf("cursor = %d after down at bottom, want 2", m.providerCursor)
	}
}

func TestProvider_SelectAzureAdvancesToOrg(t *testing.T) {
	m := NewModel()
	m = selectAzure(m)
	if m.currentStep() != stepOrganization {
		t.Errorf("step = %v, want stepOrganization", m.currentStep())
	}
	if m.activeSteps == nil {
		t.Error("activeSteps should be set after provider selection")
	}
}

func TestProvider_SelectGitHubAdvancesToToken(t *testing.T) {
	m := NewModel()
	m = selectGitHub(m)
	if m.currentStep() != stepGitHubToken {
		t.Errorf("step = %v, want stepGitHubToken", m.currentStep())
	}
}

func TestProvider_SelectBothAdvancesToOrg(t *testing.T) {
	m := NewModel()
	m = selectBoth(m)
	if m.currentStep() != stepOrganization {
		t.Errorf("step = %v, want stepOrganization", m.currentStep())
	}
}

// ── Azure-only flow ──────────────────────────────────────────────

func TestAzure_OrgEmpty_ShowsError(t *testing.T) {
	m := NewModel()
	m = selectAzure(m)
	m, _ = pressEnter(m) // submit empty

	if m.err == "" {
		t.Error("expected error for empty organization")
	}
	if m.currentStep() != stepOrganization {
		t.Error("should stay on org step")
	}
}

func TestAzure_ProjectsEmpty_ShowsError(t *testing.T) {
	m := NewModel()
	m = selectAzure(m)
	m = typeString(m, "my-org")
	m, _ = pressEnter(m) // advance to projects
	m, _ = pressEnter(m) // submit empty projects

	if m.err == "" {
		t.Error("expected error for empty projects")
	}
	if m.currentStep() != stepProjects {
		t.Error("should stay on projects step")
	}
}

func TestAzure_CSVProjectsTrimsWhitespace(t *testing.T) {
	m := NewModel()
	m = selectAzure(m)
	m = typeString(m, "my-org")
	m, _ = pressEnter(m)
	m = typeString(m, "  proj-a , proj-b  , proj-c  ")
	m, _ = pressEnter(m)

	if m.currentStep() != stepPollingInterval {
		t.Errorf("step = %v, want stepPollingInterval", m.currentStep())
	}
	if len(m.projects) != 3 {
		t.Fatalf("projects = %v, want 3 items", m.projects)
	}
	if m.projects[0] != "proj-a" || m.projects[1] != "proj-b" || m.projects[2] != "proj-c" {
		t.Errorf("projects = %v, want [proj-a proj-b proj-c]", m.projects)
	}
}

func TestAzure_FullFlow_GetConfig(t *testing.T) {
	m := navigateToConfirmAzure(t)
	m, cmd := pressEnter(m)
	if cmd == nil {
		t.Fatal("expected a quit command on confirm")
	}

	cfg := m.GetConfig()
	if cfg == nil {
		t.Fatal("GetConfig() should return non-nil after confirm")
	}
	if cfg.Organization != "my-org" {
		t.Errorf("Organization = %q, want %q", cfg.Organization, "my-org")
	}
	if len(cfg.Projects) != 1 || cfg.Projects[0] != "proj-a" {
		t.Errorf("Projects = %v, want [proj-a]", cfg.Projects)
	}
	if len(cfg.GitHub.Repos) != 0 {
		t.Errorf("GitHub.Repos = %v, want empty", cfg.GitHub.Repos)
	}
	if m.GitHubToken() != "" {
		t.Errorf("GitHubToken() = %q, want empty", m.GitHubToken())
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() failed for Azure-only config: %v", err)
	}
}

func TestAzure_BackFromConfirm_GoesToOrg(t *testing.T) {
	m := navigateToConfirmAzure(t)
	m, _ = pressKey(m, "b")
	if m.currentStep() != stepOrganization {
		t.Errorf("step = %v after b, want stepOrganization", m.currentStep())
	}
}

// ── GitHub-only flow ─────────────────────────────────────────────

func TestGitHub_TokenEmpty_ShowsError(t *testing.T) {
	m := NewModel()
	m = selectGitHub(m)
	m, _ = pressEnter(m) // submit empty token

	if m.err == "" {
		t.Error("expected error for empty token")
	}
	if m.currentStep() != stepGitHubToken {
		t.Error("should stay on GitHub token step")
	}
}

func TestGitHub_FullFlow_GetConfig(t *testing.T) {
	m := navigateToConfirmGitHub(t)
	m, cmd := pressEnter(m)
	if cmd == nil {
		t.Fatal("expected a quit command on confirm")
	}

	cfg := m.GetConfig()
	if cfg == nil {
		t.Fatal("GetConfig() should return non-nil after confirm")
	}
	if cfg.Organization != "" {
		t.Errorf("Organization = %q, want empty (GitHub-only)", cfg.Organization)
	}
	if len(cfg.Projects) != 0 {
		t.Errorf("Projects = %v, want empty (GitHub-only)", cfg.Projects)
	}
	if len(cfg.GitHub.Repos) == 0 {
		t.Error("GitHub.Repos should be set")
	}
	if cfg.GitHub.Repos[0] != "owner/repo-a" {
		t.Errorf("GitHub.Repos[0] = %q, want %q", cfg.GitHub.Repos[0], "owner/repo-a")
	}
	if m.GitHubToken() != "ghp_testtoken" {
		t.Errorf("GitHubToken() = %q, want %q", m.GitHubToken(), "ghp_testtoken")
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() failed for GitHub-only config: %v", err)
	}
	// Confirm org/projects steps were skipped — they should be nil/empty.
	if m.organization != "" || len(m.projects) != 0 {
		t.Error("org/projects should be empty when GitHub-only path was taken")
	}
}

func TestGitHub_BackFromConfirm_GoesToToken(t *testing.T) {
	m := navigateToConfirmGitHub(t)
	m, _ = pressKey(m, "b")
	// Back goes to activeSteps[0] which is stepGitHubToken for GitHub-only.
	if m.currentStep() != stepGitHubToken {
		t.Errorf("step = %v after b, want stepGitHubToken", m.currentStep())
	}
}

// ── Both flow ────────────────────────────────────────────────────

func TestBoth_FullFlow_GetConfig(t *testing.T) {
	m := navigateToConfirmBoth(t)
	m, cmd := pressEnter(m)
	if cmd == nil {
		t.Fatal("expected a quit command on confirm")
	}

	cfg := m.GetConfig()
	if cfg == nil {
		t.Fatal("GetConfig() should return non-nil after confirm")
	}
	if cfg.Organization != "my-org" {
		t.Errorf("Organization = %q, want my-org", cfg.Organization)
	}
	if len(cfg.Projects) == 0 {
		t.Error("Projects should be set for Both")
	}
	if len(cfg.GitHub.Repos) == 0 {
		t.Error("GitHub.Repos should be set for Both")
	}
	if m.GitHubToken() != "ghp_testtoken" {
		t.Errorf("GitHubToken() = %q, want ghp_testtoken", m.GitHubToken())
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() failed for Both config: %v", err)
	}
}

func TestBoth_BackFromConfirm_GoesToOrg(t *testing.T) {
	m := navigateToConfirmBoth(t)
	m, _ = pressKey(m, "b")
	// activeSteps[0] for Both is stepOrganization.
	if m.currentStep() != stepOrganization {
		t.Errorf("step = %v after b, want stepOrganization", m.currentStep())
	}
}

// ── Repo slug validation ─────────────────────────────────────────

func TestGitHubRepos_InvalidSlugs_ShowError(t *testing.T) {
	bad := []string{"noslash", "owner/", "/repo", "a/b/c"}
	for _, slug := range bad {
		t.Run(slug, func(t *testing.T) {
			m := NewModel()
			m = selectGitHub(m)
			m = typeString(m, "ghp_tok")
			m, _ = pressEnter(m) // advance past token
			m = typeString(m, slug)
			m, _ = pressEnter(m)

			if m.err == "" {
				t.Errorf("expected error for invalid slug %q, got none", slug)
			}
			if m.currentStep() != stepGitHubRepos {
				t.Errorf("should stay on repos step, got %v", m.currentStep())
			}
		})
	}
}

func TestGitHubRepos_ValidSlug_Advances(t *testing.T) {
	m := NewModel()
	m = selectGitHub(m)
	m = typeString(m, "ghp_tok")
	m, _ = pressEnter(m)
	m = typeString(m, "owner/repo")
	m, _ = pressEnter(m)
	if m.currentStep() != stepPollingInterval {
		t.Errorf("step = %v, want stepPollingInterval", m.currentStep())
	}
}

// ── Step counter ─────────────────────────────────────────────────

func TestStepCount_AzureOnly(t *testing.T) {
	// Azure-only: provider + org + projects + poll + theme + confirm = 6 total.
	m := NewModel()
	_, tot := m.stepNumber()
	if tot != 1 {
		t.Errorf("provider step total = %d, want 1 (unknown until chosen)", tot)
	}

	m = selectAzure(m)
	cur, tot := m.stepNumber()
	// Now on step 2 of 6.
	if tot != 6 {
		t.Errorf("Azure-only total = %d, want 6", tot)
	}
	if cur != 2 {
		t.Errorf("Azure-only current at org = %d, want 2", cur)
	}
}

func TestStepCount_GitHubOnly(t *testing.T) {
	// GitHub-only: provider + token + repos + poll + theme + confirm = 6 total.
	m := NewModel()
	m = selectGitHub(m)
	cur, tot := m.stepNumber()
	if tot != 6 {
		t.Errorf("GitHub-only total = %d, want 6", tot)
	}
	if cur != 2 {
		t.Errorf("GitHub-only current at token = %d, want 2", cur)
	}
}

func TestStepCount_Both(t *testing.T) {
	// Both: provider + org + projects + token + repos + poll + theme + confirm = 8 total.
	m := NewModel()
	m = selectBoth(m)
	cur, tot := m.stepNumber()
	if tot != 8 {
		t.Errorf("Both total = %d, want 8", tot)
	}
	if cur != 2 {
		t.Errorf("Both current at org = %d, want 2", cur)
	}
}

// ── Polling interval validation ──────────────────────────────────

func TestPolling_DefaultAccepted(t *testing.T) {
	m := NewModel()
	m = selectAzure(m)
	m = typeString(m, "my-org")
	m, _ = pressEnter(m)
	m = typeString(m, "proj-a")
	m, _ = pressEnter(m)

	// pollInput is pre-filled with "60", just press enter.
	m, _ = pressEnter(m)

	if m.currentStep() != stepTheme {
		t.Errorf("step = %v, want stepTheme", m.currentStep())
	}
	if m.pollingInterval != 60 {
		t.Errorf("pollingInterval = %d, want 60", m.pollingInterval)
	}
}

func TestPolling_NonNumericShowsError(t *testing.T) {
	m := NewModel()
	m = selectAzure(m)
	m = typeString(m, "my-org")
	m, _ = pressEnter(m)
	m = typeString(m, "proj-a")
	m, _ = pressEnter(m)

	m = clearInput(m)
	m = typeString(m, "abc")
	m, _ = pressEnter(m)

	if m.err == "" {
		t.Error("expected error for non-numeric polling interval")
	}
	if m.currentStep() != stepPollingInterval {
		t.Error("should stay on polling interval step")
	}
}

func TestPolling_ZeroShowsError(t *testing.T) {
	m := NewModel()
	m = selectAzure(m)
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

// ── Theme selection ──────────────────────────────────────────────

func TestTheme_NavigationAndSelection(t *testing.T) {
	m := NewModel()
	m = selectAzure(m)
	m = typeString(m, "my-org")
	m, _ = pressEnter(m)
	m = typeString(m, "proj-a")
	m, _ = pressEnter(m)
	m, _ = pressEnter(m) // accept default polling

	if m.currentStep() != stepTheme {
		t.Fatalf("step = %v, want stepTheme", m.currentStep())
	}

	initial := m.themeCursor
	m = pressDown(m)
	if m.themeCursor != initial+1 {
		t.Errorf("cursor = %d after down, want %d", m.themeCursor, initial+1)
	}
	m = pressUp(m)
	if m.themeCursor != initial {
		t.Errorf("cursor = %d after up, want %d", m.themeCursor, initial)
	}
	m = pressDown(m)
	m, _ = pressEnter(m)

	if m.currentStep() != stepConfirm {
		t.Errorf("step = %v, want stepConfirm", m.currentStep())
	}
	if m.theme != m.themes[1] {
		t.Errorf("theme = %q, want %q", m.theme, m.themes[1])
	}
}

// ── Cancellation ─────────────────────────────────────────────────

func TestEsc_CancelsAtAnyStep(t *testing.T) {
	tests := []struct {
		name  string
		model Model
	}{
		{"provider step", NewModel()},
		{"org step", func() Model { m := NewModel(); return selectAzure(m) }()},
		{"github token step", func() Model { m := NewModel(); return selectGitHub(m) }()},
		{"confirm step (azure)", func() Model {
			m := NewModel()
			m = selectAzure(m)
			m = typeString(m, "my-org")
			m, _ = pressEnter(m)
			m = typeString(m, "proj-a")
			m, _ = pressEnter(m)
			m, _ = pressEnter(m)
			m, _ = pressEnter(m)
			return m
		}()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := tt.model
			updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
			m = updated.(Model)
			if !m.Cancelled() {
				t.Error("expected Cancelled() = true after Esc")
			}
			if m.GetConfig() != nil {
				t.Error("expected GetConfig() = nil after cancel")
			}
			if cmd == nil {
				t.Error("expected quit command after Esc")
			}
		})
	}
}

func TestCtrlC_Cancels(t *testing.T) {
	m := NewModel()
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = updated.(Model)
	if !m.Cancelled() {
		t.Error("expected Cancelled() = true after Ctrl+C")
	}
	if cmd == nil {
		t.Error("expected quit command after Ctrl+C")
	}
}

// ── View rendering ───────────────────────────────────────────────

func TestView_ProviderStepContainsOptions(t *testing.T) {
	m := NewModel()
	view := m.View()
	for _, label := range []string{"Azure DevOps", "GitHub", "Both"} {
		if !strings.Contains(view, label) {
			t.Errorf("provider step view missing %q", label)
		}
	}
}

func TestView_ConfirmAzure_ContainsOrgAndProjects(t *testing.T) {
	m := navigateToConfirmAzure(t)
	view := m.View()
	for _, want := range []string{"Confirm", "my-org", "proj-a"} {
		if !strings.Contains(view, want) {
			t.Errorf("confirm view missing %q; view:\n%s", want, view)
		}
	}
}

func TestView_ConfirmGitHub_ContainsRepo(t *testing.T) {
	m := navigateToConfirmGitHub(t)
	view := m.View()
	if !strings.Contains(view, "owner/repo-a") {
		t.Errorf("confirm view missing repo; view:\n%s", view)
	}
}

func TestView_StepCounter_AppearsInView(t *testing.T) {
	m := NewModel()
	view := m.View()
	if !strings.Contains(view, "Step 1 of 1") {
		t.Errorf("provider step view should show Step 1 of 1; view:\n%s", view)
	}

	m = selectAzure(m)
	view = m.View()
	if !strings.Contains(view, "Step 2 of 6") {
		t.Errorf("org step (Azure) should show Step 2 of 6; view:\n%s", view)
	}
}
