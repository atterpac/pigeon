// Package onboard is the Wails service for adding, listing, and removing mail
// accounts. It is the only path that writes credentials; the rest of the app
// reads them through the App's provider factory.
package onboard

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/atterpac/pigeon/internal/auth"
	"github.com/atterpac/pigeon/internal/desktop"
	"github.com/atterpac/pigeon/internal/email"
)

// Onboarding is a Wails service for adding, listing, and removing mail
// accounts. Register it alongside the email Client so the frontend onboarding
// flow can call these methods.
type Onboarding struct {
	app    host
	client mailClient
}

// host is the subset of *desktop.App the onboarding flow needs: the credential
// store it writes to and the sync options for the initial sync. Narrowing to an
// interface (rather than holding *desktop.App directly) lets tests drive the
// validation and rollback logic with fakes instead of a live App.
type host interface {
	Creds() auth.CredentialStore
	SyncOptions() email.SyncOptions
}

// mailClient is the subset of *email.Client the onboarding flow drives.
type mailClient interface {
	AddAccount(ctx context.Context, acct email.Account) ([]email.Mailbox, error)
	ForgetAccount(ctx context.Context, id email.AccountID) error
	Accounts(ctx context.Context) ([]email.Account, error)
	StartSync(ctx context.Context, acct email.Account, mailboxes []email.LabelID, opts email.SyncOptions) error
}

// New builds the service over the given app.
func New(app *desktop.App) *Onboarding {
	return &Onboarding{app: app, client: app.Client}
}

// defaultMailboxes is the initial sync set for a new account. The inbox is the
// only mailbox synced eagerly; others are added as the UI requests them.
var defaultMailboxes = []email.LabelID{email.InboxLabel}

// AddAppPasswordAccount registers an IMAP/SMTP account authenticated with a
// password or app password (e.g. a Gmail App Password). No OAuth, no browser.
func (o *Onboarding) AddAppPasswordAccount(ctx context.Context, emailAddr, displayName, appPassword string) (email.Account, error) {
	emailAddr = strings.TrimSpace(strings.ToLower(emailAddr))
	if emailAddr == "" {
		return email.Account{}, errors.New("email address is required")
	}
	// App passwords are shown grouped in 4s; users routinely paste the spaces.
	appPassword = strings.ReplaceAll(appPassword, " ", "")
	if appPassword == "" {
		return email.Account{}, errors.New("password is required")
	}

	acct := email.Account{
		ID:    email.AccountID(emailAddr),
		Kind:  email.KindIMAP,
		Email: emailAddr,
		Name:  strings.TrimSpace(displayName),
	}
	return o.storeAndRegister(ctx, acct, appPassword)
}

// IMAPAccountRequest describes a custom IMAP (incoming) + SMTP (outgoing)
// account to add. Ports default to IMAP 993 (implicit TLS) and SMTP 587
// (STARTTLS) when zero; SMTPHost defaults to IMAPHost when empty. A struct
// keeps the four host/port fields from being transposed at the call site.
type IMAPAccountRequest struct {
	Email       string
	DisplayName string
	Password    string
	IMAPHost    string
	IMAPPort    int
	SMTPHost    string
	SMTPPort    int
}

// AddIMAPAccount registers a custom IMAP (incoming) + SMTP (outgoing) account
// with an explicit server. Use this for providers that aren't in the built-in
// endpoint map. See IMAPAccountRequest for the port/host defaulting rules.
func (o *Onboarding) AddIMAPAccount(ctx context.Context, req IMAPAccountRequest) (email.Account, error) {
	emailAddr := strings.TrimSpace(strings.ToLower(req.Email))
	if emailAddr == "" {
		return email.Account{}, errors.New("email address is required")
	}
	imapHost := strings.TrimSpace(req.IMAPHost)
	if imapHost == "" {
		return email.Account{}, errors.New("IMAP server is required")
	}
	// Unlike a Gmail App Password, a custom server's password may legitimately
	// contain spaces, so validate emptiness without mangling the input.
	if strings.TrimSpace(req.Password) == "" {
		return email.Account{}, errors.New("password is required")
	}
	imapPort := req.IMAPPort
	if imapPort == 0 {
		imapPort = 993
	}
	smtpHost := strings.TrimSpace(req.SMTPHost)
	if smtpHost == "" {
		smtpHost = imapHost
	}
	smtpPort := req.SMTPPort
	if smtpPort == 0 {
		smtpPort = 587
	}

	acct := email.Account{
		ID:       email.AccountID(emailAddr),
		Kind:     email.KindIMAP,
		Email:    emailAddr,
		Name:     strings.TrimSpace(req.DisplayName),
		IMAPHost: imapHost,
		IMAPPort: imapPort,
		SMTPHost: smtpHost,
		SMTPPort: smtpPort,
	}
	return o.storeAndRegister(ctx, acct, req.Password)
}

// storeAndRegister persists acct's credential then registers the account,
// shared by the Add* entry points. The credential is rolled back if
// registration fails (see register).
func (o *Onboarding) storeAndRegister(ctx context.Context, acct email.Account, password string) (email.Account, error) {
	if err := o.app.Creds().Set(ctx, acct.Email, auth.Credential{Password: password}); err != nil {
		return email.Account{}, fmt.Errorf("store credential: %w", err)
	}
	return o.register(ctx, acct)
}

// register registers the account (which connects, validating the credential,
// and persists the mailbox topology) then launches background sync. On a
// registration failure the just-stored credential is rolled back so a retry
// starts clean. Once the account is added the call is a success: a failure to
// launch the initial sync is logged rather than returned, since the account is
// persisted and the sync loop will retry on its own — returning both a valid
// account and an error would leave callers guessing which to believe.
func (o *Onboarding) register(ctx context.Context, acct email.Account) (email.Account, error) {
	if _, err := o.client.AddAccount(ctx, acct); err != nil {
		_ = o.app.Creds().Delete(ctx, string(acct.ID))
		return email.Account{}, fmt.Errorf("add account: %w", err)
	}
	if err := o.client.StartSync(ctx, acct, defaultMailboxes, o.app.SyncOptions()); err != nil {
		log.Printf("onboard: start sync for %s after add: %v", acct.Email, err)
	}
	return acct, nil
}

// ListAccounts returns the configured accounts for the settings UI.
func (o *Onboarding) ListAccounts(ctx context.Context) ([]email.Account, error) {
	return o.client.Accounts(ctx)
}

// RemoveAccount stops sync, forgets the account's local data, and deletes its
// stored credential. Both steps are attempted even if the first fails: leaving
// a stored credential behind is the worse outcome for the package that owns
// credential writes, so the delete is never skipped.
func (o *Onboarding) RemoveAccount(ctx context.Context, id email.AccountID) error {
	forget := o.client.ForgetAccount(ctx, id)
	del := o.app.Creds().Delete(ctx, string(id))
	return errors.Join(forget, del)
}
