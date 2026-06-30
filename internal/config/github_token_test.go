package config

import (
	"errors"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// GetGitHubToken
// ---------------------------------------------------------------------------

func TestGetGitHubToken_FromKeyring(t *testing.T) {
	mock := newMockKeyring()
	ks := &KeyringStore{provider: mock}

	if err := mock.Set(serviceName, githubTokenUser, "ghp_keyring_token"); err != nil {
		t.Fatalf("pre-seed Set: %v", err)
	}

	tok, err := ks.GetGitHubToken()
	if err != nil {
		t.Fatalf("GetGitHubToken() error: %v", err)
	}
	if tok != "ghp_keyring_token" {
		t.Errorf("GetGitHubToken() = %q, want %q", tok, "ghp_keyring_token")
	}
}

func TestGetGitHubToken_FallbackToEnvWhenNotFound(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "ghp_env_token")

	mock := newMockKeyring() // keyring is empty → ErrNotFound
	ks := &KeyringStore{provider: mock}

	tok, err := ks.GetGitHubToken()
	if err != nil {
		t.Fatalf("GetGitHubToken() error: %v", err)
	}
	if tok != "ghp_env_token" {
		t.Errorf("GetGitHubToken() = %q, want %q", tok, "ghp_env_token")
	}
}

func TestGetGitHubToken_FallbackToEnvWhenKeyringFails(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "ghp_fallback")

	mock := newMockKeyring()
	mock.err = errors.New("keyring daemon unavailable")
	ks := &KeyringStore{provider: mock}

	tok, err := ks.GetGitHubToken()
	if err != nil {
		t.Fatalf("GetGitHubToken() error: %v", err)
	}
	if tok != "ghp_fallback" {
		t.Errorf("GetGitHubToken() = %q, want %q", tok, "ghp_fallback")
	}
}

func TestGetGitHubToken_ReturnsErrNotFoundWhenNeitherConfigured(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "") // ensure env var is unset for this test

	mock := newMockKeyring() // empty → ErrNotFound
	ks := &KeyringStore{provider: mock}

	_, err := ks.GetGitHubToken()
	if err != ErrNotFound {
		t.Errorf("GetGitHubToken() error = %v, want ErrNotFound", err)
	}
}

func TestGetGitHubToken_ReturnsWrappedErrorWhenKeyringFailsAndNoEnv(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")

	mock := newMockKeyring()
	mock.err = errors.New("secret service crash")
	ks := &KeyringStore{provider: mock}

	_, err := ks.GetGitHubToken()
	if err == nil {
		t.Fatal("GetGitHubToken() should return error when keyring fails and env not set")
	}
	if !strings.Contains(err.Error(), "GITHUB_TOKEN") {
		t.Errorf("error should mention GITHUB_TOKEN: %v", err)
	}
	if !strings.Contains(err.Error(), "keyring") {
		t.Errorf("error should mention keyring: %v", err)
	}
}

// ---------------------------------------------------------------------------
// SetGitHubToken
// ---------------------------------------------------------------------------

func TestSetGitHubToken_StoresInKeyring(t *testing.T) {
	mock := newMockKeyring()
	ks := &KeyringStore{provider: mock}

	if err := ks.SetGitHubToken("ghp_new_token"); err != nil {
		t.Fatalf("SetGitHubToken() error: %v", err)
	}

	stored, err := mock.Get(serviceName, githubTokenUser)
	if err != nil {
		t.Fatalf("verify Get: %v", err)
	}
	if stored != "ghp_new_token" {
		t.Errorf("stored = %q, want %q", stored, "ghp_new_token")
	}
}

func TestSetGitHubToken_RejectsEmptyToken(t *testing.T) {
	mock := newMockKeyring()
	ks := &KeyringStore{provider: mock}

	if err := ks.SetGitHubToken(""); err == nil {
		t.Error("SetGitHubToken(\"\") should return error for empty token")
	}
}

func TestSetGitHubToken_KeyringError(t *testing.T) {
	mock := newMockKeyring()
	mock.err = errors.New("keyring locked")
	ks := &KeyringStore{provider: mock}

	if err := ks.SetGitHubToken("ghp_tok"); err == nil {
		t.Error("SetGitHubToken should propagate keyring error")
	}
}

// ---------------------------------------------------------------------------
// DeleteGitHubToken
// ---------------------------------------------------------------------------

func TestDeleteGitHubToken_RemovesFromKeyring(t *testing.T) {
	mock := newMockKeyring()
	ks := &KeyringStore{provider: mock}

	if err := mock.Set(serviceName, githubTokenUser, "ghp_to_delete"); err != nil {
		t.Fatalf("pre-seed Set: %v", err)
	}

	if err := ks.DeleteGitHubToken(); err != nil {
		t.Fatalf("DeleteGitHubToken() error: %v", err)
	}

	_, err := mock.Get(serviceName, githubTokenUser)
	if err != ErrNotFound {
		t.Errorf("after delete, Get should return ErrNotFound, got %v", err)
	}
}

func TestDeleteGitHubToken_KeyringError(t *testing.T) {
	mock := newMockKeyring()
	mock.err = errors.New("keyring error")
	ks := &KeyringStore{provider: mock}

	if err := ks.DeleteGitHubToken(); err == nil {
		t.Error("DeleteGitHubToken should propagate keyring error")
	}
}
