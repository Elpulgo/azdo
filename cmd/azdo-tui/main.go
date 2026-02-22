package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/Elpulgo/azdo/internal/app"
	"github.com/Elpulgo/azdo/internal/azdevops"
	"github.com/Elpulgo/azdo/internal/config"
	"github.com/Elpulgo/azdo/internal/ui/patinput"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
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
	client, err := azdevops.NewMultiClient(cfg.Organization, cfg.Projects, pat)
	if err != nil {
		return fmt.Errorf("failed to create Azure DevOps client: %w", err)
	}

	// Create and run the TUI application
	model := app.NewModel(client, cfg)
	p := tea.NewProgram(model, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("TUI application error: %w", err)
	}

	return nil
}

// promptForPAT displays a TUI to prompt the user for their PAT and saves it to the keyring
func promptForPAT(store *config.KeyringStore) (string, error) {
	model := patinput.NewModel()
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
