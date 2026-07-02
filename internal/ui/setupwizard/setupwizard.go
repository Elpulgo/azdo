package setupwizard

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Elpulgo/azdo/internal/config"
	"github.com/Elpulgo/azdo/internal/ui/components"
	"github.com/Elpulgo/azdo/internal/ui/styles"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// providerChoice identifies which backend(s) the user wants to configure.
type providerChoice int

const (
	providerAzure  providerChoice = iota // Azure DevOps only
	providerGitHub                       // GitHub only
	providerBoth                         // Both Azure and GitHub
)

// wizardStep names all possible screens. Not every step is active in every
// provider selection; activeSteps holds only the ones reachable in the
// current session.
type wizardStep int

const (
	stepProvider        wizardStep = iota // 3-way provider selector (new first step)
	stepOrganization                      // Azure org name
	stepProjects                          // Azure projects (comma-separated)
	stepGitHubToken                       // GitHub PAT (masked)
	stepGitHubRepos                       // GitHub "owner/repo" slugs (comma-separated)
	stepPollingInterval                   // polling cadence (pre-filled with default)
	stepTheme                             // cursor-based theme selection
	stepConfirm                           // summary + save
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("99"))

	stepStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("99"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)

	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("99")).
			Bold(true)

	labelStyle = lipgloss.NewStyle().
			Bold(true)
)

// Model is the Bubbletea model for the setup wizard.
type Model struct {
	// Provider selection (step 0).
	providerCursor int
	provider       providerChoice

	// activeSteps is built once the provider is chosen and drives navigation.
	// Before the provider step completes it is nil.
	activeSteps     []wizardStep
	activeStepIndex int // index into activeSteps

	// Text inputs.
	orgInput   textinput.Model
	projInput  textinput.Model
	pollInput  textinput.Model
	tokenInput textinput.Model
	reposInput textinput.Model

	// Theme selection.
	themes      []string
	themeCursor int

	err       string
	cancelled bool
	done      bool

	// Collected values.
	organization    string
	projects        []string
	pollingInterval int
	theme           string
	githubToken     string
	repos           []string
}

var providerLabels = []string{"Azure DevOps", "GitHub", "Both"}

// NewModel creates a new setup wizard model.
func NewModel() Model {
	orgInput := textinput.New()
	orgInput.Placeholder = "e.g. my-organization"
	orgInput.CharLimit = 200
	orgInput.Width = 60

	projInput := textinput.New()
	projInput.Placeholder = "e.g. project-a, project-b"
	projInput.CharLimit = 500
	projInput.Width = 60

	pollInput := textinput.New()
	pollInput.Placeholder = "seconds"
	pollInput.CharLimit = 10
	pollInput.Width = 20
	pollInput.SetValue(strconv.Itoa(config.DefaultPollingInterval))

	tokenInput := textinput.New()
	tokenInput.Placeholder = "ghp_..."
	tokenInput.EchoMode = textinput.EchoPassword
	tokenInput.EchoCharacter = '•'
	tokenInput.CharLimit = 200
	tokenInput.Width = 60

	reposInput := textinput.New()
	reposInput.Placeholder = "e.g. owner/repo-a, owner/repo-b"
	reposInput.CharLimit = 500
	reposInput.Width = 60

	return Model{
		orgInput:   orgInput,
		projInput:  projInput,
		pollInput:  pollInput,
		tokenInput: tokenInput,
		reposInput: reposInput,
		themes:     styles.ListAvailableThemes(),
		// activeSteps is nil until the provider is chosen.
	}
}

// currentStep returns the wizardStep for the current position in the active
// sequence, or stepProvider when we haven't chosen a provider yet.
func (m Model) currentStep() wizardStep {
	if m.activeSteps == nil {
		return stepProvider
	}
	return m.activeSteps[m.activeStepIndex]
}

// buildActiveSteps returns the ordered slice of steps that are active for
// the given provider choice.
func buildActiveSteps(p providerChoice) []wizardStep {
	steps := []wizardStep{}
	if p == providerAzure || p == providerBoth {
		steps = append(steps, stepOrganization, stepProjects)
	}
	if p == providerGitHub || p == providerBoth {
		steps = append(steps, stepGitHubToken, stepGitHubRepos)
	}
	steps = append(steps, stepPollingInterval, stepTheme, stepConfirm)
	return steps
}

// Init starts the text cursor blink.
func (m Model) Init() tea.Cmd {
	return textinput.Blink
}

// Update handles key messages and drives the wizard state machine.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Global cancel.
		if msg.Type == tea.KeyEsc || msg.Type == tea.KeyCtrlC {
			m.cancelled = true
			return m, tea.Quit
		}

		switch m.currentStep() {
		case stepProvider:
			return m.updateProvider(msg)
		case stepOrganization:
			return m.updateOrganization(msg)
		case stepProjects:
			return m.updateProjects(msg)
		case stepGitHubToken:
			return m.updateGitHubToken(msg)
		case stepGitHubRepos:
			return m.updateGitHubRepos(msg)
		case stepPollingInterval:
			return m.updatePollingInterval(msg)
		case stepTheme:
			return m.updateTheme(msg)
		case stepConfirm:
			return m.updateConfirm(msg)
		}
	}

	return m, nil
}

func (m Model) updateProvider(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyUp:
		if m.providerCursor > 0 {
			m.providerCursor--
		}
	case tea.KeyDown:
		if m.providerCursor < len(providerLabels)-1 {
			m.providerCursor++
		}
	case tea.KeyEnter:
		m.provider = providerChoice(m.providerCursor)
		m.activeSteps = buildActiveSteps(m.provider)
		m.activeStepIndex = 0
		// Focus the first input in the active sequence.
		return m.focusCurrentStep()
	}
	return m, nil
}

// focusCurrentStep focuses the appropriate text input for the step we just
// moved to. Cursor-based steps (theme, confirm) and the provider step itself
// need no focus.
func (m Model) focusCurrentStep() (tea.Model, tea.Cmd) {
	if m.activeSteps == nil {
		return m, nil
	}
	switch m.activeSteps[m.activeStepIndex] {
	case stepOrganization:
		m.orgInput.Focus()
		return m, textinput.Blink
	case stepProjects:
		m.projInput.Focus()
		return m, textinput.Blink
	case stepGitHubToken:
		m.tokenInput.Focus()
		return m, textinput.Blink
	case stepGitHubRepos:
		m.reposInput.Focus()
		return m, textinput.Blink
	case stepPollingInterval:
		m.pollInput.Focus()
		return m, textinput.Blink
	}
	return m, nil
}

func (m Model) advance() (tea.Model, tea.Cmd) {
	m.err = ""
	m.activeStepIndex++
	return m.focusCurrentStep()
}

func (m Model) updateOrganization(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyEnter {
		val := strings.TrimSpace(m.orgInput.Value())
		if val == "" {
			m.err = "Organization cannot be empty"
			return m, nil
		}
		m.organization = val
		m.orgInput.Blur()
		return m.advance()
	}

	var cmd tea.Cmd
	m.orgInput, cmd = m.orgInput.Update(msg)
	return m, cmd
}

func (m Model) updateProjects(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyEnter {
		val := strings.TrimSpace(m.projInput.Value())
		if val == "" {
			m.err = "Projects cannot be empty"
			return m, nil
		}
		parts := strings.Split(val, ",")
		projects := make([]string, 0, len(parts))
		for _, p := range parts {
			trimmed := strings.TrimSpace(p)
			if trimmed != "" {
				projects = append(projects, trimmed)
			}
		}
		if len(projects) == 0 {
			m.err = "At least one project is required"
			return m, nil
		}
		m.projects = projects
		m.projInput.Blur()
		return m.advance()
	}

	var cmd tea.Cmd
	m.projInput, cmd = m.projInput.Update(msg)
	return m, cmd
}

func (m Model) updateGitHubToken(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyEnter {
		val := strings.TrimSpace(m.tokenInput.Value())
		if val == "" {
			m.err = "Token cannot be empty"
			return m, nil
		}
		m.githubToken = val
		m.tokenInput.Blur()
		return m.advance()
	}

	var cmd tea.Cmd
	m.tokenInput, cmd = m.tokenInput.Update(msg)
	return m, cmd
}

// validateRepoSlug returns an error message when the slug is not in the
// "owner/repo" format (exactly one slash, both halves non-empty). This mirrors
// config.Validate()'s slug rule exactly.
func validateRepoSlug(slug string) string {
	parts := strings.SplitN(slug, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" || strings.Contains(parts[1], "/") {
		return fmt.Sprintf("%q is not a valid owner/repo slug", slug)
	}
	return ""
}

func (m Model) updateGitHubRepos(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyEnter {
		val := strings.TrimSpace(m.reposInput.Value())
		if val == "" {
			m.err = "At least one repository is required"
			return m, nil
		}
		parts := strings.Split(val, ",")
		repos := make([]string, 0, len(parts))
		for _, r := range parts {
			trimmed := strings.TrimSpace(r)
			if trimmed != "" {
				repos = append(repos, trimmed)
			}
		}
		if len(repos) == 0 {
			m.err = "At least one repository is required"
			return m, nil
		}
		for _, r := range repos {
			if errMsg := validateRepoSlug(r); errMsg != "" {
				m.err = errMsg
				return m, nil
			}
		}
		m.repos = repos
		m.reposInput.Blur()
		return m.advance()
	}

	var cmd tea.Cmd
	m.reposInput, cmd = m.reposInput.Update(msg)
	return m, cmd
}

func (m Model) updatePollingInterval(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyEnter {
		val := strings.TrimSpace(m.pollInput.Value())
		n, err := strconv.Atoi(val)
		if err != nil {
			m.err = "Polling interval must be a number"
			return m, nil
		}
		if n <= 0 {
			m.err = "Polling interval must be greater than 0"
			return m, nil
		}
		m.pollingInterval = n
		m.pollInput.Blur()
		return m.advance()
	}

	var cmd tea.Cmd
	m.pollInput, cmd = m.pollInput.Update(msg)
	return m, cmd
}

func (m Model) updateTheme(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyUp:
		if m.themeCursor > 0 {
			m.themeCursor--
		}
	case tea.KeyDown:
		if m.themeCursor < len(m.themes)-1 {
			m.themeCursor++
		}
	case tea.KeyEnter:
		m.theme = m.themes[m.themeCursor]
		return m.advance()
	}
	return m, nil
}

func (m Model) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		m.done = true
		return m, tea.Quit
	case tea.KeyRunes:
		if string(msg.Runes) == "b" {
			// Go back to the first active input step (index 0).
			m.activeStepIndex = 0
			return m.focusCurrentStep()
		}
	}
	return m, nil
}

// stepNumber returns the 1-based position in the active sequence for rendering
// "Step N of M". The provider step is always step 1; the active-sequence steps
// follow from step 2.
func (m Model) stepNumber() (current, total int) {
	if m.activeSteps == nil {
		// On the provider step itself.
		total = 1 // we show "Step 1 of ?" — we don't know M yet; show 1/1 as a placeholder
		return 1, total
	}
	// Provider step = 1; active steps begin at 2.
	total = 1 + len(m.activeSteps)
	current = 1 + m.activeStepIndex + 1
	return current, total
}

// View renders the current wizard step.
func (m Model) View() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render(strings.Join(components.LogoArt, "\n")))
	b.WriteString("\n\n")
	b.WriteString(titleStyle.Render("Setup Wizard"))
	b.WriteString("\n")

	cur, tot := m.stepNumber()
	b.WriteString(stepStyle.Render(fmt.Sprintf("Step %d of %d", cur, tot)))
	b.WriteString("\n\n")

	switch m.currentStep() {
	case stepProvider:
		b.WriteString(labelStyle.Render("Provider"))
		b.WriteString("\n")
		b.WriteString("Select which provider(s) you want to configure:\n\n")
		for i, label := range providerLabels {
			if i == m.providerCursor {
				b.WriteString(selectedStyle.Render("▸ " + label))
			} else {
				b.WriteString("  " + label)
			}
			b.WriteString("\n")
		}
	case stepOrganization:
		b.WriteString(labelStyle.Render("Organization"))
		b.WriteString("\n")
		b.WriteString("Enter your Azure DevOps organization name:\n\n")
		b.WriteString(m.orgInput.View())
	case stepProjects:
		b.WriteString(labelStyle.Render("Projects"))
		b.WriteString("\n")
		b.WriteString("Enter project names (comma-separated):\n\n")
		b.WriteString(m.projInput.View())
	case stepGitHubToken:
		b.WriteString(labelStyle.Render("GitHub Token"))
		b.WriteString("\n")
		b.WriteString("Enter your GitHub personal access token:\n\n")
		b.WriteString(m.tokenInput.View())
	case stepGitHubRepos:
		b.WriteString(labelStyle.Render("GitHub Repositories"))
		b.WriteString("\n")
		b.WriteString("Enter repositories in owner/repo format (comma-separated):\n\n")
		b.WriteString(m.reposInput.View())
	case stepPollingInterval:
		b.WriteString(labelStyle.Render("Polling Interval"))
		b.WriteString("\n")
		b.WriteString("How often to refresh data (in seconds):\n\n")
		b.WriteString(m.pollInput.View())
	case stepTheme:
		b.WriteString(labelStyle.Render("Theme"))
		b.WriteString("\n")
		b.WriteString("Select a color theme:\n\n")
		for i, t := range m.themes {
			if i == m.themeCursor {
				b.WriteString(selectedStyle.Render("▸ " + t))
			} else {
				b.WriteString("  " + t)
			}
			b.WriteString("\n")
		}
	case stepConfirm:
		b.WriteString(labelStyle.Render("Confirm"))
		b.WriteString("\n\n")
		if m.organization != "" {
			b.WriteString(fmt.Sprintf("  Organization:     %s\n", m.organization))
		}
		if len(m.projects) > 0 {
			b.WriteString(fmt.Sprintf("  Projects:         %s\n", strings.Join(m.projects, ", ")))
		}
		if len(m.repos) > 0 {
			b.WriteString(fmt.Sprintf("  GitHub repos:     %s\n", strings.Join(m.repos, ", ")))
		}
		b.WriteString(fmt.Sprintf("  Polling interval: %ds\n", m.pollingInterval))
		b.WriteString(fmt.Sprintf("  Theme:            %s\n", m.theme))
	}

	b.WriteString("\n\n")

	if m.err != "" {
		b.WriteString(errorStyle.Render("Error: " + m.err))
		b.WriteString("\n\n")
	}

	switch m.currentStep() {
	case stepProvider, stepTheme:
		b.WriteString(helpStyle.Render("↑/↓ to navigate • Enter to select • Esc to cancel"))
	case stepConfirm:
		b.WriteString(helpStyle.Render("Enter to save • b to go back • Esc to cancel"))
	default:
		b.WriteString(helpStyle.Render("Enter to continue • Esc to cancel"))
	}

	return b.String()
}

// GetConfig returns the collected configuration after the wizard completes.
// Returns nil if the wizard was cancelled or hasn't completed yet.
// This is a pure getter — no keyring side-effects. The caller is responsible
// for persisting the GitHub token (see GitHubToken()).
func (m Model) GetConfig() *config.Config {
	if m.cancelled || !m.done {
		return nil
	}

	configPath, err := config.GetPath()
	if err != nil {
		return nil
	}

	cfg := config.NewWithPath(
		m.organization,
		m.projects,
		m.pollingInterval,
		m.theme,
		configPath,
	)

	if len(m.repos) > 0 {
		cfg.GitHub = config.GitHubConfig{Repos: m.repos}
	}

	return cfg
}

// GitHubToken returns the GitHub personal access token entered during the
// wizard, or "" when the GitHub steps were skipped.
func (m Model) GitHubToken() string {
	return m.githubToken
}

// Cancelled returns true if the user cancelled the wizard.
func (m Model) Cancelled() bool {
	return m.cancelled
}
