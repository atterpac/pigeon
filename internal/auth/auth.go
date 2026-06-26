// Package auth handles credentials: OAuth2 for Gmail (refresh-token flow with a
// loopback redirect), XOAUTH2 SASL for IMAP/SMTP, and app-password fallback.
// Secrets live in the OS keyring via a pluggable CredentialStore so headless
// and server deployments can swap the backend.
package auth

import (
	"context"
	"errors"
	"time"

	"golang.org/x/oauth2"
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

// Credential is either an OAuth2 token set or a password/app-password.
type Credential struct {
	// OAuth2 fields (Gmail, and any XOAUTH2-capable IMAP provider).
	AccessToken  string    `json:"access_token,omitempty"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	TokenType    string    `json:"token_type,omitempty"`
	Expiry       time.Time `json:"expiry,omitzero"`

	// Password / app-password fallback for plain IMAP/SMTP.
	Password string `json:"password,omitempty"`
}

// IsOAuth reports whether this credential carries an OAuth2 token.
func (c Credential) IsOAuth() bool { return c.RefreshToken != "" || c.AccessToken != "" }

// Token converts the credential to an oauth2.Token.
func (c Credential) Token() *oauth2.Token {
	return &oauth2.Token{
		AccessToken:  c.AccessToken,
		RefreshToken: c.RefreshToken,
		TokenType:    c.TokenType,
		Expiry:       c.Expiry,
	}
}

// FromToken builds a Credential from an oauth2.Token, preserving any existing
// password field on c.
func (c Credential) FromToken(t *oauth2.Token) Credential {
	c.AccessToken = t.AccessToken
	c.TokenType = t.TokenType
	c.Expiry = t.Expiry
	if t.RefreshToken != "" { // Google omits it on refresh; keep the old one
		c.RefreshToken = t.RefreshToken
	}
	return c
}
