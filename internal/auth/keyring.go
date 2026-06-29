package auth

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/zalando/go-keyring"
)

// keyringService is the namespace credentials are stored under in the OS keyring.
const keyringService = "github.com/atterpac/email"

// Keyring is a CredentialStore backed by the OS keyring (Secret Service on
// Linux, Keychain on macOS, Credential Manager on Windows).
type Keyring struct {
	service string
}

// NewKeyring returns a keyring-backed store under the default service namespace.
func NewKeyring() *Keyring { return &Keyring{service: keyringService} }

// NewKeyringService returns a keyring-backed store under a custom service
// namespace, so tests (or multi-tenant hosts) can isolate their entries from the
// default.
func NewKeyringService(service string) *Keyring { return &Keyring{service: service} }

func (k *Keyring) Get(_ context.Context, account string) (Credential, error) {
	raw, err := keyring.Get(k.service, account)
	if errors.Is(err, keyring.ErrNotFound) {
		return Credential{}, ErrNotFound
	}
	if err != nil {
		return Credential{}, err
	}
	var c Credential
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return Credential{}, err
	}
	return c, nil
}

func (k *Keyring) Set(_ context.Context, account string, c Credential) error {
	raw, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return keyring.Set(k.service, account, string(raw))
}

func (k *Keyring) Delete(_ context.Context, account string) error {
	err := keyring.Delete(k.service, account)
	if errors.Is(err, keyring.ErrNotFound) {
		return ErrNotFound
	}
	return err
}
