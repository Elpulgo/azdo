package config

import (
	"errors"
	"fmt"
	"os"
)

// githubTokenUser is the keyring user key for the GitHub personal access
// token. It lives under the same service name ("azdo-tui") as the Azure PAT,
// distinguished by a different user key so the two credentials never collide.
const githubTokenUser = "github-token"

// GetGitHubToken returns the GitHub personal access token.
// It tries the OS keyring first (service "azdo-tui", user "github-token"),
// then falls back to the GITHUB_TOKEN environment variable. This mirrors
// GetPAT's design so that CI/CD pipelines can supply the token via env var
// without a system keyring.
//
// Returns ErrNotFound when neither the keyring nor the environment variable
// has a token configured.
func (k *KeyringStore) GetGitHubToken() (string, error) {
	token, err := k.provider.Get(serviceName, githubTokenUser)
	if err == nil {
		return token, nil
	}

	// Whether the keyring is missing the key (ErrNotFound) or unavailable
	// entirely, try the GITHUB_TOKEN env var next. This covers the common
	// CI/CD case where GITHUB_TOKEN is injected by the runner.
	if envToken := os.Getenv("GITHUB_TOKEN"); envToken != "" {
		return envToken, nil
	}

	if errors.Is(err, ErrNotFound) {
		return "", ErrNotFound
	}
	return "", fmt.Errorf(
		"failed to retrieve GitHub token from keyring and GITHUB_TOKEN not set: %w. "+
			"Please run the setup wizard or set the GITHUB_TOKEN environment variable", err)
}

// SetGitHubToken stores a GitHub personal access token in the system keyring.
func (k *KeyringStore) SetGitHubToken(token string) error {
	if token == "" {
		return errors.New("token cannot be empty")
	}
	if err := k.provider.Set(serviceName, githubTokenUser, token); err != nil {
		return fmt.Errorf("failed to store GitHub token in keyring: %w", err)
	}
	return nil
}

// DeleteGitHubToken removes the GitHub personal access token from the keyring.
func (k *KeyringStore) DeleteGitHubToken() error {
	if err := k.provider.Delete(serviceName, githubTokenUser); err != nil {
		return fmt.Errorf("failed to delete GitHub token from keyring: %w", err)
	}
	return nil
}
