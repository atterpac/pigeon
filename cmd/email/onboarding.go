package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/atterpac/email/internal/auth"
	"github.com/atterpac/email/internal/email"
)

// Onboarding is a Wails service for adding, listing, and removing mail
// accounts. It is the only path that writes credentials; the rest of the app
// reads them through the provider factory. Register it alongside the email
// Client so the frontend onboarding flow can call these methods.
type Onboarding struct {
	app *App

	// openURL opens a consent URL in the user's system browser. Injected from
	// main once the Wails application exists (app.Browser.OpenURL).
	openURL func(string) error
}

// newOnboarding builds the service. openURL may be nil (the OAuth flow then
// prints the consent URL to stdout as a fallback).
func newOnboarding(app *App, openURL func(string) error) *Onboarding {
	return &Onboarding{app: app, openURL: openURL}
}

// defaultMailboxes is the initial sync set for a new account. The inbox is the
// only mailbox synced eagerly; others are added as the UI requests them.
var defaultMailboxes = []email.LabelID{email.InboxLabel}

// AddGoogleAccount registers a Gmail account via the OAuth loopback flow. It
// opens the system browser for consent and blocks until the user finishes (or
// ctx is cancelled), then persists the refresh token, registers the account,
// and starts background sync. emailAddr is the Gmail address being added and
// doubles as the account id.
func (o *Onboarding) AddGoogleAccount(ctx context.Context, emailAddr, displayName string) (email.Account, error) {
	emailAddr = strings.TrimSpace(strings.ToLower(emailAddr))
	if emailAddr == "" {
		return email.Account{}, fmt.Errorf("email address is required")
	}

	cred, err := auth.InteractiveAuth(ctx, o.app.googleCfg, o.openURL)
	if err != nil {
		return email.Account{}, fmt.Errorf("google authorization: %w", err)
	}
	if err := o.app.creds.Set(ctx, emailAddr, cred); err != nil {
		return email.Account{}, fmt.Errorf("store credential: %w", err)
	}

	acct := email.Account{
		ID:    email.AccountID(emailAddr),
		Kind:  email.KindGmail,
		Email: emailAddr,
		Name:  strings.TrimSpace(displayName),
	}
	return o.register(ctx, acct)
}

// AddAppPasswordAccount registers an IMAP/SMTP account authenticated with a
// password or app password (e.g. a Gmail App Password). No OAuth, no browser.
func (o *Onboarding) AddAppPasswordAccount(ctx context.Context, emailAddr, displayName, appPassword string) (email.Account, error) {
	emailAddr = strings.TrimSpace(strings.ToLower(emailAddr))
	if emailAddr == "" {
		return email.Account{}, fmt.Errorf("email address is required")
	}
	// App passwords are shown grouped in 4s; users routinely paste the spaces.
	appPassword = strings.ReplaceAll(appPassword, " ", "")
	if appPassword == "" {
		return email.Account{}, fmt.Errorf("password is required")
	}

	if err := o.app.creds.Set(ctx, emailAddr, auth.Credential{Password: appPassword}); err != nil {
		return email.Account{}, fmt.Errorf("store credential: %w", err)
	}

	acct := email.Account{
		ID:    email.AccountID(emailAddr),
		Kind:  email.KindIMAP,
		Email: emailAddr,
		Name:  strings.TrimSpace(displayName),
	}
	return o.register(ctx, acct)
}

// AddIMAPAccount registers a custom IMAP (incoming) + SMTP (outgoing) account
// with an explicit server. Use this for providers that aren't in the built-in
// endpoint map. Ports default to IMAP 993 (implicit TLS) and SMTP 587
// (STARTTLS) when passed as 0; SMTP host defaults to the IMAP host.
func (o *Onboarding) AddIMAPAccount(ctx context.Context, emailAddr, displayName, password, imapHost string, imapPort int, smtpHost string, smtpPort int) (email.Account, error) {
	emailAddr = strings.TrimSpace(strings.ToLower(emailAddr))
	if emailAddr == "" {
		return email.Account{}, fmt.Errorf("email address is required")
	}
	imapHost = strings.TrimSpace(imapHost)
	if imapHost == "" {
		return email.Account{}, fmt.Errorf("IMAP server is required")
	}
	password = strings.ReplaceAll(password, " ", "")
	if password == "" {
		return email.Account{}, fmt.Errorf("password is required")
	}
	if imapPort == 0 {
		imapPort = 993
	}
	smtpHost = strings.TrimSpace(smtpHost)
	if smtpHost == "" {
		smtpHost = imapHost
	}
	if smtpPort == 0 {
		smtpPort = 587
	}

	if err := o.app.creds.Set(ctx, emailAddr, auth.Credential{Password: password}); err != nil {
		return email.Account{}, fmt.Errorf("store credential: %w", err)
	}

	acct := email.Account{
		ID:       email.AccountID(emailAddr),
		Kind:     email.KindIMAP,
		Email:    emailAddr,
		Name:     strings.TrimSpace(displayName),
		IMAPHost: imapHost,
		IMAPPort: imapPort,
		SMTPHost: smtpHost,
		SMTPPort: smtpPort,
	}
	return o.register(ctx, acct)
}

// register registers the account (which connects, validating the credential,
// and persists the mailbox topology) then launches background sync. On a
// registration failure the just-stored credential is rolled back so a retry
// starts clean.
func (o *Onboarding) register(ctx context.Context, acct email.Account) (email.Account, error) {
	if _, err := o.app.Client.AddAccount(ctx, acct); err != nil {
		_ = o.app.creds.Delete(ctx, string(acct.ID))
		return email.Account{}, fmt.Errorf("add account: %w", err)
	}
	if err := o.app.Client.StartSync(ctx, acct, defaultMailboxes, o.app.desktopSyncOptions()); err != nil {
		return acct, fmt.Errorf("start sync: %w", err)
	}
	return acct, nil
}

// ListAccounts returns the configured accounts for the settings UI.
func (o *Onboarding) ListAccounts(ctx context.Context) ([]email.Account, error) {
	return o.app.Client.Accounts(ctx)
}

// RemoveAccount stops sync, forgets the account's local data, and deletes its
// stored credential.
func (o *Onboarding) RemoveAccount(ctx context.Context, id email.AccountID) error {
	if err := o.app.Client.ForgetAccount(ctx, id); err != nil {
		return err
	}
	return o.app.creds.Delete(ctx, string(id))
}
