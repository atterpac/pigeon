// Package desktop is the backend of the Pigeon desktop app: it owns the email
// SDK client and the provider/credential wiring, ties that runtime to the Wails
// application lifecycle, and exposes runtime sync controls. The thin cmd/email
// main only constructs it and registers the Wails services. Frontend-facing
// facade services live in the sibling onboard/ and service/ packages.
package desktop

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/atterpac/email/internal/auth"
	"github.com/atterpac/email/internal/desktop/notify"
	"github.com/atterpac/email/internal/email"
	"github.com/atterpac/email/internal/provider"
	"github.com/atterpac/email/internal/provider/imap"
)

// App owns the email SDK client and the secrets it needs to build providers.
// Hold one for the lifetime of the process; expose its methods to Wails.
type App struct {
	Client *email.Client

	creds auth.CredentialStore

	// onNewMail, if set, is invoked by the background sync loop whenever a poll
	// pulls in new mail. main wires it to the desktop notifications service.
	onNewMail func(acct email.AccountID, mb provider.MailboxRef, msgs []email.Message)

	mu sync.Mutex
	// pollInterval is how often background syncs pull mail forward when no push
	// hint arrives. Guarded by mu; mutated at runtime via SetPollInterval.
	pollInterval time.Duration
	// notify holds the user's notification preferences. Guarded by mu; pushed
	// from the frontend via SetNotifyPrefs and read by the new-mail handler.
	notify notify.Prefs
}

// defaultPollInterval matches the sync engine's own default; kept here so the
// frontend has a sensible value to show before the user changes anything.
const defaultPollInterval = 60 * time.Second

// Creds returns the credential store backing the app. The onboarding service —
// the only path that writes credentials — reaches it through here.
func (a *App) Creds() auth.CredentialStore { return a.creds }

// NotifyPrefs returns a copy of the current notification preferences.
func (a *App) NotifyPrefs() notify.Prefs {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.notify.Mode == "" {
		return notify.DefaultPrefs
	}
	return a.notify
}

// SetNotifyPrefs replaces the notification preferences (called from the frontend).
func (a *App) SetNotifyPrefs(prefs notify.Prefs) {
	if prefs.Mode == "" {
		prefs.Mode = "all"
	}
	a.mu.Lock()
	a.notify = prefs
	a.mu.Unlock()
}

// SetNewMailHandler registers the callback the sync loops use to announce new
// mail. Call it before StartConfiguredSyncs / onboarding so freshly started
// loops pick it up.
func (a *App) SetNewMailHandler(fn func(acct email.AccountID, mb provider.MailboxRef, msgs []email.Message)) {
	a.onNewMail = fn
}

// NewApp opens the store, wires the credential store (OS keyring), and builds
// the SDK client with a ProviderFactory that maps each account to an IMAP/SMTP
// backend authenticated with a password / app-password.
//
//	dbPath – SQLite path (created if absent)
func NewApp(ctx context.Context, dbPath string) (*App, error) {
	creds := auth.NewKeyring()

	factory := func(ctx context.Context, acct email.Account) (provider.Provider, error) {
		cred, err := creds.Get(ctx, string(acct.ID))
		if err != nil {
			return nil, fmt.Errorf("credentials for %s: %w", acct.Email, err)
		}

		cfg := imap.Config{
			Account:  acct.ID,
			Username: acct.Email,
			Password: cred.Password,
		}
		applyIMAPEndpoints(&cfg, acct)
		return imap.New(cfg), nil
	}

	client, err := email.Open(ctx, email.Config{
		DBPath:   dbPath,
		Provider: factory,
	})
	if err != nil {
		return nil, err
	}
	return &App{Client: client, creds: creds, pollInterval: defaultPollInterval}, nil
}

// imapEndpoint holds the IMAP + SMTP connection details for a provider.
type imapEndpoint struct {
	host, smtpHost string
	port, smtpPort int
}

// knownIMAPEndpoints maps an email domain to its well-known IMAP/SMTP servers.
// IMAP is implicit TLS (993); SMTP is STARTTLS (587).
var knownIMAPEndpoints = map[string]imapEndpoint{
	"gmail.com":      {"imap.gmail.com", "smtp.gmail.com", 993, 587},
	"googlemail.com": {"imap.gmail.com", "smtp.gmail.com", 993, 587},
}

// applyIMAPEndpoints fills cfg's Host/Port/SMTP fields. Per-account servers
// stored at onboarding win; otherwise a known-domain endpoint is used. Custom
// accounts always carry their own servers, so unknown domains still connect.
func applyIMAPEndpoints(cfg *imap.Config, acct email.Account) {
	if acct.IMAPHost != "" {
		cfg.Host = acct.IMAPHost
		cfg.Port = acct.IMAPPort
		cfg.SMTPHost = acct.SMTPHost
		cfg.SMTPPort = acct.SMTPPort
		return
	}
	at := strings.LastIndexByte(acct.Email, '@')
	if at < 0 {
		return
	}
	domain := strings.ToLower(acct.Email[at+1:])
	ep, ok := knownIMAPEndpoints[domain]
	if !ok {
		return
	}
	cfg.Host, cfg.Port = ep.host, ep.port
	cfg.SMTPHost, cfg.SMTPPort = ep.smtpHost, ep.smtpPort
}

// Close shuts down sync loops, providers, and the store.
func (a *App) Close() error {
	if a.Client == nil {
		return nil
	}
	return a.Client.Close()
}

// StartConfiguredSyncs resumes background sync for accounts already present in
// the local store. Onboarding starts sync for newly-added accounts; this covers
// subsequent app launches.
func (a *App) StartConfiguredSyncs(ctx context.Context) error {
	accounts, err := a.Client.Accounts(ctx)
	if err != nil {
		return err
	}

	var errs []error
	for _, acct := range accounts {
		mailboxes, err := a.Client.Mailboxes(ctx, acct.ID)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s mailboxes: %w", acct.Email, err))
			continue
		}
		if err := a.Client.StartSync(ctx, acct, []email.LabelID{syncMailbox(mailboxes)}, a.SyncOptions()); err != nil {
			errs = append(errs, fmt.Errorf("%s sync: %w", acct.Email, err))
		}
	}
	return errors.Join(errs...)
}

// SyncOptions builds the desktop sync options snapshot from the current poll
// interval and the registered new-mail handler.
func (a *App) SyncOptions() email.SyncOptions {
	a.mu.Lock()
	interval := a.pollInterval
	a.mu.Unlock()
	return email.SyncOptions{
		PollInterval:     interval,
		BackfillPageSize: 100,
		BackfillMaxPages: 5,
		BodyWarmPages:    3, // warm ~75 newest inbox bodies before backfill
		OnNewMail:        a.onNewMail,
	}
}

// minPollInterval guards against hammering providers with sub-5s polls.
const minPollInterval = 5 * time.Second

// PollIntervalSeconds reports the current background poll interval, in seconds.
// Exposed to the frontend so the settings UI can show the active value.
func (a *App) PollIntervalSeconds() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return int(a.pollInterval / time.Second)
}

// SetPollInterval updates the background poll interval and restarts the running
// sync loops so the change takes effect immediately. Values below
// minPollInterval are clamped. Exposed to the frontend.
func (a *App) SetPollInterval(ctx context.Context, seconds int) error {
	interval := max(time.Duration(seconds)*time.Second, minPollInterval)
	a.mu.Lock()
	a.pollInterval = interval
	a.mu.Unlock()
	// StartSync replaces any existing loop for an account, so this re-arms every
	// configured account with the new interval.
	return a.StartConfiguredSyncs(ctx)
}

func syncMailbox(mailboxes []email.Mailbox) email.LabelID {
	for _, mailbox := range mailboxes {
		if mailbox.Role == email.RoleInbox {
			return mailbox.ID
		}
	}
	for _, mailbox := range mailboxes {
		if strings.EqualFold(string(mailbox.ID), string(email.InboxLabel)) {
			return mailbox.ID
		}
	}
	return email.InboxLabel
}
