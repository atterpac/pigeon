// Package auth handles credentials: passwords / app-passwords for IMAP and SMTP.
// Secrets live in the OS keyring via a pluggable CredentialStore so headless
// and server deployments can swap the backend.
package auth

import (
	"context"
	"errors"
)

// ErrNotFound is returned by a CredentialStore when no credential is stored.
var ErrNotFound = errors.New("auth: credential not found")

// CredentialStore persists and retrieves per-account secrets. Secrets never
// touch the SQLite store. Implementations must be safe for concurrent use.
type CredentialStore interface {
	Get(ctx context.Context, account string) (Credential, error)
	Set(ctx context.Context, account string, c Credential) error
	Delete(ctx context.Context, account string) error
}

// Credential is a password / app-password for plain IMAP/SMTP.
type Credential struct {
	Password string `json:"password,omitempty"`
}
