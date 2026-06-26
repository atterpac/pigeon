package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/emersion/go-sasl"
	"golang.org/x/oauth2"
	gmailapi "google.golang.org/api/gmail/v1"

	"github.com/atterpac/email/internal/auth"
	"github.com/atterpac/email/internal/provider"
	"github.com/atterpac/email/internal/provider/gmail"
	"github.com/atterpac/email/internal/provider/imap"
	"github.com/atterpac/email/pkg/email"
)

// App owns the email SDK client and the secrets it needs to build providers.
// Hold one for the lifetime of the process; expose its methods to Wails.
type App struct {
	Client *email.Client

	creds     auth.CredentialStore
	googleCfg *oauth2.Config
}

// newApp opens the store, wires the credential store (OS keyring), and builds
// the SDK client with a ProviderFactory that maps each account to a Gmail or
// IMAP backend.
//
//	dbPath           – SQLite path (created if absent)
//	googleClientJSON – Google OAuth client-secrets file (used by Gmail accounts)
func newApp(ctx context.Context, dbPath, googleClientJSON string) (*App, error) {
	creds := auth.NewKeyring()

	googleCfg, err := auth.LoadGoogleConfig(googleClientJSON, gmailapi.GmailModifyScope)
	if err != nil {
		return nil, fmt.Errorf("load google oauth config: %w", err)
	}

	factory := func(ctx context.Context, acct email.Account) (provider.Provider, error) {
		cred, err := creds.Get(ctx, string(acct.ID))
		if err != nil {
			return nil, fmt.Errorf("credentials for %s: %w", acct.Email, err)
		}

		switch acct.Kind {
		case email.KindGmail:
			ts := auth.TokenSource(ctx, googleCfg, creds, string(acct.ID), cred)
			return gmail.New(ctx, acct.ID, ts)

		case email.KindIMAP:
			cfg := imap.Config{
				Account:  acct.ID,
				Username: acct.Email,
			}
			applyIMAPEndpoints(&cfg, acct)
			if cred.IsOAuth() {
				ts := auth.TokenSource(ctx, googleCfg, creds, string(acct.ID), cred)
				cfg.NewSASL = func() (sasl.Client, error) {
					return auth.XOAuth2Client(acct.Email, ts)
				}
			} else {
				cfg.Password = cred.Password
			}
			return imap.New(cfg), nil

		default:
			return nil, fmt.Errorf("unknown account kind %v for %s", acct.Kind, acct.Email)
		}
	}

	client, err := email.Open(ctx, email.Config{
		DBPath:   dbPath,
		Provider: factory,
	})
	if err != nil {
		return nil, err
	}
	return &App{Client: client, creds: creds, googleCfg: googleCfg}, nil
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
		if err := a.Client.StartSync(ctx, acct, []email.LabelID{syncMailbox(mailboxes)}, desktopSyncOptions()); err != nil {
			errs = append(errs, fmt.Errorf("%s sync: %w", acct.Email, err))
		}
	}
	return errors.Join(errs...)
}

func desktopSyncOptions() email.SyncOptions {
	return email.SyncOptions{
		BackfillPageSize: 100,
		BackfillMaxPages: 5,
	}
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
