package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/Elpulgo/azdo/internal/app"
	"github.com/Elpulgo/azdo/internal/azdevops"
	"github.com/Elpulgo/azdo/internal/cli"
	"github.com/Elpulgo/azdo/internal/config"
	"github.com/Elpulgo/azdo/internal/ui/patinput"
	tea "github.com/charmbracelet/bubbletea"
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
	default:
		return runTUI()
	}
}

func runHelp() error {
	configPath, _ := config.GetPath()
	if configPath == "" {
		configPath = "~/.config/azdo-tui/config.yaml"
	}

	fmt.Printf(`azdo - A TUI for Azure DevOps (%s)

Usage:
  azdo              Start the TUI application
  azdo auth         Set or update your Personal Access Token (PAT)
  azdo --help       Show this help message
  azdo --version    Show version information

Configuration:
  Config file: %s
  PAT storage: System keyring (service: azdo-tui)
  PAT fallback: AZDO_PAT environment variable

Keyboard shortcuts (in TUI):
  1/2/3        Switch tabs (Pipelines, Pull Requests, Work Items)
  r            Refresh data
  f            Search / filter
  t            Select theme
  ?            Toggle help overlay
  q            Quit

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

	if isUpdate {
		fmt.Println("Azure DevOps PAT Update")
		fmt.Println("This will replace your existing Personal Access Token in the system keyring.")
	} else {
		fmt.Println("Azure DevOps PAT Setup")
		fmt.Println("This will store your Personal Access Token in the system keyring.")
	}
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
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Verify organization and projects are configured
	if cfg.Organization == "" {
		return fmt.Errorf("organization not configured\nHint: Set 'organization' in ~/.config/azdo-tui/config.yaml")
	}
	if len(cfg.Projects) == 0 {
		return fmt.Errorf("no projects configured\nHint: Set 'projects' list in ~/.config/azdo-tui/config.yaml")
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

	// Create and run the TUI application
	model := app.NewModel(client, cfg, version)
	p := tea.NewProgram(model, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("TUI application error: %w", err)
	}

	return nil
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
