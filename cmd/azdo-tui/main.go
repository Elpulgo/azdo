package main

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"strings"

	"github.com/Elpulgo/azdo/internal/app"
	"github.com/Elpulgo/azdo/internal/azdevops"
	"github.com/Elpulgo/azdo/internal/cli"
	"github.com/Elpulgo/azdo/internal/config"
	"github.com/Elpulgo/azdo/internal/demo"
	"github.com/Elpulgo/azdo/internal/state"
	"github.com/Elpulgo/azdo/internal/ui/components"
	"github.com/Elpulgo/azdo/internal/ui/patinput"
	"github.com/Elpulgo/azdo/internal/ui/setupwizard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Build-time variables injected via ldflags by goreleaser.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if err := run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	action := cli.ParseArgs(args)

	switch action {
	case cli.ActionHelp:
		return runHelp()
	case cli.ActionVersion:
		return runVersion()
	case cli.ActionAuth:
		return runAuth()
	case cli.ActionDemo:
		return demo.Run(version, commit)
	default:
		return runTUI()
	}
}

func runHelp() error {
	configPath, _ := config.GetPath()
	if configPath == "" {
		configPath = "~/.config/azdo-tui/config.yaml"
	}

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("99"))
	fmt.Println(titleStyle.Render(strings.Join(components.LogoArt, "\n")))

	fmt.Printf(`azdo - A TUI for Azure DevOps (%s)

Usage:
  azdo              Start the TUI application
  azdo auth         Set or update your Personal Access Token (PAT)
  azdo demo         Launch with mock data (for screenshots/demos)
  azdo --help       Show this help message
  azdo --version    Show version information

Configuration:
  Config file: %s
  PAT storage: System keyring (service: azdo-tui)
  PAT fallback: AZDO_PAT environment variable

Required PAT permissions:
  Build        (Read)         - pipelines, build logs
  Code         (Read & Write) - pull requests, voting, comments
  Work Items   (Read & Write) - queries, comments, state changes

Keyboard shortcuts (in TUI):
  Navigation:
    ↑/k          Move up
    ↓/j          Move down
    pgup/pgdn    Page up / down
    enter        View details / expand
    esc          Go back

  Tabs:
    1/2/3        Switch tabs (Pull Requests, Work Items, Pipelines)
    ←/→          Previous / next tab

  Actions:
    f            Search / filter
    m            Toggle my items (PRs / work items)
    A            Toggle as reviewer (PRs)
    T            Filter by tag (work items)
    r            Refresh data
    v            Vote on PR (detail view)
    s            Change work item state (detail view)
    c            Add comment (work item detail)
    o            Open in browser (PR / work item detail)
    t            Select theme
    ?            Toggle help overlay
    q            Quit

  Code Review (PR diff):
    c            Create new comment
    p            Reply to nearest thread
    x            Resolve nearest thread
    n / N        Jump to next / previous comment

  Log Viewer (pipelines):
    g            Go to top
    G            Go to bottom

For more information, visit: https://github.com/Elpulgo/azdo
`, version, configPath)

	return nil
}

func runVersion() error {
	fmt.Printf("azdo version %s (commit: %s, built: %s)\n", version, commit, date)
	return nil
}

func runAuth() error {
	store := config.NewKeyringStore()

	// Check if PAT already exists to show appropriate message
	_, err := store.GetPAT()
	isUpdate := err == nil

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("99"))
	fmt.Println(titleStyle.Render(strings.Join(components.LogoArt, "\n")))
	fmt.Println()
	if isUpdate {
		fmt.Println("Azure DevOps PAT Update")
		fmt.Println("This will replace your existing Personal Access Token in the system keyring.")
	} else {
		fmt.Println("Azure DevOps PAT Setup")
		fmt.Println("This will store your Personal Access Token in the system keyring.")
	}
	fmt.Println()
	fmt.Println(patinput.PermissionInfoPlain())
	fmt.Println()

	pat, err := promptForPATWithMode(store, isUpdate)
	if err != nil {
		return fmt.Errorf("failed to set PAT: %w", err)
	}

	if pat != "" {
		fmt.Println("\nPAT saved successfully to system keyring.")
	}

	return nil
}

func runTUI() error {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		if errors.Is(err, config.ErrConfigNotFound) {
			cfg, err = runSetupWizard()
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}

	// Get PAT from keyring
	store := config.NewKeyringStore()
	pat, err := store.GetPAT()
	if err != nil {
		// If PAT not found, prompt user to enter it
		if errors.Is(err, config.ErrNotFound) {
			pat, err = promptForPAT(store)
			if err != nil {
				return fmt.Errorf("failed to set PAT: %w", err)
			}
		} else {
			return fmt.Errorf("failed to get PAT: %w", err)
		}
	}

	// Create multi-project Azure DevOps client
	client, err := azdevops.NewMultiClient(cfg.Organization, cfg.Projects, pat, cfg.DisplayNames)
	if err != nil {
		return fmt.Errorf("failed to create Azure DevOps client: %w", err)
	}

	// Load persisted state (last active tab, last-viewed PR/work item).
	// A missing file is treated as a clean slate — not an error.
	statePath, err := state.Path()
	if err != nil {
		return fmt.Errorf("resolve state path: %w", err)
	}
	stateStore, err := state.NewStore(statePath)
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	// Create and run the TUI application
	model := app.NewModel(client, cfg, version, commit)
	model.SetStateStore(stateStore)
	model.ApplyState(stateStore.State())
	p := tea.NewProgram(model, tea.WithAltScreen())

	// Forward OS termination signals to Bubble Tea so the program unwinds
	// cleanly (alt-screen restored, state flushed) instead of being killed
	// mid-write. SIGINT is also handled by the in-app 'q'/Ctrl+C binding;
	// SIGTERM and SIGHUP are the ones that matter here.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		<-sigCh
		p.Send(tea.QuitMsg{})
	}()

	// Best-effort flush on any exit path — normal quit, signal-driven
	// quit, or a panic propagating up from the Tea program. A SIGKILL or
	// power loss is unrecoverable; the debounced writes during the
	// session bound the loss window.
	defer func() {
		signal.Stop(sigCh)
		if flushErr := stateStore.Flush(); flushErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to persist state: %v\n", flushErr)
		}
	}()

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("TUI application error: %w", err)
	}

	return nil
}

// runSetupWizard launches the interactive setup wizard and saves the config.
func runSetupWizard() (*config.Config, error) {
	model := setupwizard.NewModel()
	p := tea.NewProgram(model)

	m, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("setup wizard error: %w", err)
	}

	finalModel, ok := m.(setupwizard.Model)
	if !ok {
		return nil, fmt.Errorf("unexpected model type from setup wizard")
	}

	if finalModel.Cancelled() {
		return nil, fmt.Errorf("setup cancelled")
	}

	cfg := finalModel.GetConfig()
	if cfg == nil {
		return nil, fmt.Errorf("setup wizard did not produce a configuration")
	}

	if err := cfg.Save(); err != nil {
		return nil, fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Println("\nConfiguration saved successfully!")
	return cfg, nil
}

// promptForPAT displays a TUI to prompt the user for their PAT (first-time setup)
func promptForPAT(store *config.KeyringStore) (string, error) {
	return promptForPATWithMode(store, false)
}

// promptForPATWithMode displays a TUI to prompt the user for their PAT.
// If isUpdate is true, shows an "update" message instead of "first-time setup".
func promptForPATWithMode(store *config.KeyringStore, isUpdate bool) (string, error) {
	var model patinput.Model
	if isUpdate {
		model = patinput.NewModelForUpdate()
	} else {
		model = patinput.NewModel()
	}
	p := tea.NewProgram(model)

	m, err := p.Run()
	if err != nil {
		return "", fmt.Errorf("failed to run PAT input: %w", err)
	}

	// Extract the final model
	finalModel, ok := m.(patinput.Model)
	if !ok {
		return "", fmt.Errorf("unexpected model type")
	}

	pat := finalModel.GetPAT()
	if pat == "" {
		return "", fmt.Errorf("PAT input cancelled or empty")
	}

	// Save PAT to keyring
	if err := store.SetPAT(pat); err != nil {
		return "", fmt.Errorf("failed to save PAT to keyring: %w", err)
	}

	return pat, nil
}
