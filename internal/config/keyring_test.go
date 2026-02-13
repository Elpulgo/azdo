package config

import (
	"errors"
	"testing"
)

// mockKeyring implements the keyringProvider interface for testing
type mockKeyring struct {
	store map[string]string
	err   error
}

func newMockKeyring() *mockKeyring {
	return &mockKeyring{
		store: make(map[string]string),
	}
}

func (m *mockKeyring) Get(service, user string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	key := service + ":" + user
	val, ok := m.store[key]
	if !ok {
		return "", ErrNotFound
	}
	return val, nil
}

func (m *mockKeyring) Set(service, user, password string) error {
	if m.err != nil {
		return m.err
	}
	key := service + ":" + user
	m.store[key] = password
	return nil
}

func (m *mockKeyring) Delete(service, user string) error {
	if m.err != nil {
		return m.err
	}
	key := service + ":" + user
	delete(m.store, key)
	return nil
}

func TestSetPAT(t *testing.T) {
	mock := newMockKeyring()
	ks := &KeyringStore{provider: mock}

	err := ks.SetPAT("test-pat-token")
	if err != nil {
		t.Fatalf("SetPAT() failed: %v", err)
	}

	// Verify it was stored
	stored, err := mock.Get(serviceName, userName)
	if err != nil {
		t.Fatalf("Failed to verify stored PAT: %v", err)
	}

	if stored != "test-pat-token" {
		t.Errorf("Expected stored PAT to be 'test-pat-token', got %s", stored)
	}
}

func TestGetPAT_Success(t *testing.T) {
	mock := newMockKeyring()
	ks := &KeyringStore{provider: mock}

	// Store a PAT first
	mock.Set(serviceName, userName, "my-secret-token")

	// Retrieve it
	pat, err := ks.GetPAT()
	if err != nil {
		t.Fatalf("GetPAT() failed: %v", err)
	}

	if pat != "my-secret-token" {
		t.Errorf("Expected PAT to be 'my-secret-token', got %s", pat)
	}
}

func TestGetPAT_NotFound(t *testing.T) {
	mock := newMockKeyring()
	ks := &KeyringStore{provider: mock}

	// Try to get PAT when none exists
	pat, err := ks.GetPAT()
	if err != ErrNotFound {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}

	if pat != "" {
		t.Errorf("Expected empty PAT, got %s", pat)
	}
}

func TestDeletePAT(t *testing.T) {
	mock := newMockKeyring()
	ks := &KeyringStore{provider: mock}

	// Store a PAT first
	ks.SetPAT("token-to-delete")

	// Delete it
	err := ks.DeletePAT()
	if err != nil {
		t.Fatalf("DeletePAT() failed: %v", err)
	}

	// Verify it's deleted
	_, err = mock.Get(serviceName, userName)
	if err != ErrNotFound {
		t.Errorf("Expected ErrNotFound after deletion, got %v", err)
	}
}

func TestSetPAT_Error(t *testing.T) {
	mock := newMockKeyring()
	mock.err = errors.New("keyring access denied")
	ks := &KeyringStore{provider: mock}

	err := ks.SetPAT("test-token")
	if err == nil {
		t.Error("Expected SetPAT to fail with keyring error")
	}
}

func TestGetPAT_Error(t *testing.T) {
	mock := newMockKeyring()
	mock.err = errors.New("keyring access denied")
	ks := &KeyringStore{provider: mock}

	_, err := ks.GetPAT()
	if err == nil {
		t.Error("Expected GetPAT to fail with keyring error")
	}
}

func TestNewKeyringStore(t *testing.T) {
	ks := NewKeyringStore()
	if ks == nil {
		t.Error("NewKeyringStore() returned nil")
	}

	// Verify it has a provider
	if ks.provider == nil {
		t.Error("KeyringStore provider is nil")
	}
}

func TestSetPAT_EmptyToken(t *testing.T) {
	mock := newMockKeyring()
	ks := &KeyringStore{provider: mock}

	err := ks.SetPAT("")
	if err == nil {
		t.Error("Expected SetPAT to fail with empty token")
	}
}

func TestGetPAT_FallbackToEnvVar(t *testing.T) {
	// Set environment variable
	envPAT := "env-pat-token-123"
	t.Setenv("AZDO_PAT", envPAT)

	// Create keyring that will fail
	mock := newMockKeyring()
	mock.err = errors.New("keyring unavailable")
	ks := &KeyringStore{provider: mock}

	// Should fall back to environment variable
	pat, err := ks.GetPAT()
	if err != nil {
		t.Fatalf("GetPAT() should succeed with env fallback, got error: %v", err)
	}

	if pat != envPAT {
		t.Errorf("Expected PAT from env var '%s', got '%s'", envPAT, pat)
	}
}

func TestGetPAT_ErrorWhenNoFallback(t *testing.T) {
	// Ensure AZDO_PAT is not set
	t.Setenv("AZDO_PAT", "")

	// Create keyring that will fail
	mock := newMockKeyring()
	mock.err = errors.New("keyring unavailable")
	ks := &KeyringStore{provider: mock}

	// Should return error with helpful message
	_, err := ks.GetPAT()
	if err == nil {
		t.Error("Expected GetPAT to fail when keyring unavailable and no env var set")
	}

	// Check error message mentions both issues
	errMsg := err.Error()
	if !contains(errMsg, "keyring") {
		t.Errorf("Error message should mention keyring: %s", errMsg)
	}
	if !contains(errMsg, "AZDO_PAT") {
		t.Errorf("Error message should mention AZDO_PAT env var: %s", errMsg)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && (s[:len(substr)] == substr || contains(s[1:], substr))))
}
