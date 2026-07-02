package command

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/atterpac/pigeon/internal/model"
	"github.com/atterpac/pigeon/internal/provider"
	imapprov "github.com/atterpac/pigeon/internal/provider/imap"
)

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

// resolveEndpoint determines the IMAP/SMTP servers for account. Environment
// overrides win (EMAIL_IMAP_HOST, EMAIL_IMAP_PORT, EMAIL_SMTP_HOST,
// EMAIL_SMTP_PORT); otherwise the email domain is looked up in the built-in
// endpoint map. Ports default to IMAP 993 (implicit TLS) and SMTP 587 (STARTTLS).
func resolveEndpoint(account string) (imapEndpoint, error) {
	var ep imapEndpoint
	if at := strings.LastIndexByte(account, '@'); at >= 0 {
		ep = knownIMAPEndpoints[strings.ToLower(account[at+1:])]
	}
	if h := os.Getenv("EMAIL_IMAP_HOST"); h != "" {
		ep.host = h
	}
	if p := os.Getenv("EMAIL_IMAP_PORT"); p != "" {
		ep.port = atoiOr(p, ep.port)
	}
	if h := os.Getenv("EMAIL_SMTP_HOST"); h != "" {
		ep.smtpHost = h
	}
	if p := os.Getenv("EMAIL_SMTP_PORT"); p != "" {
		ep.smtpPort = atoiOr(p, ep.smtpPort)
	}
	if ep.host == "" {
		return ep, fmt.Errorf("no IMAP server for %s (set EMAIL_IMAP_HOST)", account)
	}
	if ep.port == 0 {
		ep.port = 993
	}
	if ep.smtpHost == "" {
		ep.smtpHost = ep.host
	}
	if ep.smtpPort == 0 {
		ep.smtpPort = 587
	}
	return ep, nil
}

// newProvider builds an IMAP/SMTP provider for account, authenticated with the
// stored app password. This is the single place that maps an account to a
// concrete backend; the engine and store stay backend-agnostic.
func newProvider(ctx context.Context, account string) (provider.Provider, error) {
	store := credStore()
	cred, err := store.Get(ctx, account)
	if err != nil {
		return nil, fmt.Errorf("read credential (run `email auth %s` first): %w", account, err)
	}
	if cred.Password == "" {
		return nil, fmt.Errorf("no password stored for %s (run `email auth %s`)", account, account)
	}
	ep, err := resolveEndpoint(account)
	if err != nil {
		return nil, err
	}
	return imapprov.New(imapprov.Config{
		Account:  model.AccountID(account),
		Host:     ep.host,
		Port:     ep.port,
		Username: account,
		Password: cred.Password,
		SMTPHost: ep.smtpHost,
		SMTPPort: ep.smtpPort,
	}), nil
}
