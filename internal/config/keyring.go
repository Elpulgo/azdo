package config

import (
	"errors"
	"fmt"

	"github.com/zalando/go-keyring"
)

const (
	serviceName = "azdo-tui"
	userName    = "pat"
)

// ErrNotFound is returned when a PAT is not found in the keyring
var ErrNotFound = errors.New("PAT not found in keyring")

// keyringProvider defines the interface for keyring operations
// This allows for mocking in tests
type keyringProvider interface {
	Get(service, user string) (string, error)
	Set(service, user, password string) error
	Delete(service, user string) error
}

// systemKeyring is a wrapper around the go-keyring library
type systemKeyring struct{}

func (s *systemKeyring) Get(service, user string) (string, error) {
	secret, err := keyring.Get(service, user)
	if err != nil {
		if err == keyring.ErrNotFound {
			return "", ErrNotFound
		}
		return "", err
	}
	return secret, nil
}

func (s *systemKeyring) Set(service, user, password string) error {
	return keyring.Set(service, user, password)
}

func (s *systemKeyring) Delete(service, user string) error {
	return keyring.Delete(service, user)
}

// KeyringStore manages PAT storage in the system keyring
type KeyringStore struct {
	provider keyringProvider
}

// NewKeyringStore creates a new KeyringStore with the system keyring
func NewKeyringStore() *KeyringStore {
	return &KeyringStore{
		provider: &systemKeyring{},
	}
}

// SetPAT stores a Personal Access Token in the system keyring
func (k *KeyringStore) SetPAT(token string) error {
	if token == "" {
		return errors.New("token cannot be empty")
	}

	if err := k.provider.Set(serviceName, userName, token); err != nil {
		return fmt.Errorf("failed to store PAT in keyring: %w", err)
	}

	return nil
}

// GetPAT retrieves the Personal Access Token from the system keyring
func (k *KeyringStore) GetPAT() (string, error) {
	token, err := k.provider.Get(serviceName, userName)
	if err != nil {
		if err == ErrNotFound {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("failed to retrieve PAT from keyring: %w", err)
	}

	return token, nil
}

// DeletePAT removes the Personal Access Token from the system keyring
func (k *KeyringStore) DeletePAT() error {
	if err := k.provider.Delete(serviceName, userName); err != nil {
		return fmt.Errorf("failed to delete PAT from keyring: %w", err)
	}

	return nil
}
