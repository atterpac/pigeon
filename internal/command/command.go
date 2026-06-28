// Package command implements the reference CLI for the email SDK. It is a thin
// harness over the same internals the public email.Client uses; cmd/email just
// calls Run. Keeping it here (rather than in package main) lets a different
// front-end — e.g. a Wails app — own main without dragging the CLI along.
package command

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/atterpac/email/internal/auth"
)

// Run dispatches a CLI invocation. args is the argument list without the program
// name (i.e. os.Args[1:]). It returns an error for the caller to report; usage
// problems return a non-nil error too.
func Run(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return usageErr()
	}
	rest := args[1:]
	switch args[0] {
	case "auth":
		return cmdAuth(ctx, rest)
	case "auth-status":
		return cmdAuthStatus(ctx, rest)
	case "imap-list":
		return cmdImapList(ctx, rest)
	case "imap-sync":
		return cmdImapSync(ctx, rest)
	case "sync":
		return cmdSync(ctx, rest)
	case "search":
		return cmdSearch(ctx, rest)
	case "daemon":
		return cmdDaemon(ctx, rest)
	case "send":
		return cmdSend(ctx, rest)
	default:
		return usageErr()
	}
}

func usageErr() error {
	return fmt.Errorf("usage:\n" +
		"  email auth <account-email>\n" +
		"  email auth-status <account-email>\n" +
		"  email imap-list <account-email>\n" +
		"  email imap-sync <account-email> [mailbox]\n" +
		"  email sync <account-email> [mailbox] [pageSize] [pages]\n" +
		"  email search <account-email> <query>\n" +
		"  email daemon <account-email> [labels] [pollSeconds] [pageSize] [rps]\n" +
		"  email send <account-email> <to> <subject> <body...>")
}

// credStore selects the credential backend via EMAIL_CRED_STORE:
//
//	keyring (default) — OS Secret Service / Keychain / WinCred
//	encrypted         — argon2id + AES-GCM file; passphrase via
//	                    EMAIL_CRED_PASSPHRASE or EMAIL_CRED_KEYFILE
//	file              — plaintext JSON file (dev only)
func credStore() auth.CredentialStore {
	credPath := func() string {
		if p := os.Getenv("EMAIL_CRED_FILE"); p != "" {
			return p
		}
		return auth.DefaultFilePath()
	}
	switch os.Getenv("EMAIL_CRED_STORE") {
	case "encrypted":
		return auth.NewEncrypted(credPath(), auth.PassphraseFromEnv())
	case "file":
		return auth.NewFile(credPath())
	default:
		return auth.NewKeyring()
	}
}

// cmdAuth stores an IMAP/SMTP app password for account. The password is read
// from EMAIL_APP_PASSWORD, or prompted on stdin when that is unset. Spaces (as
// shown in grouped app passwords) are stripped.
func cmdAuth(ctx context.Context, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: email auth <account-email>")
	}
	account := args[0]
	password := os.Getenv("EMAIL_APP_PASSWORD")
	if password == "" {
		fmt.Print("App password: ")
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && line == "" {
			return fmt.Errorf("read password: %w", err)
		}
		password = line
	}
	password = strings.ReplaceAll(strings.TrimSpace(password), " ", "")
	if password == "" {
		return fmt.Errorf("password is required")
	}
	store := credStore()
	if err := store.Set(ctx, account, auth.Credential{Password: password}); err != nil {
		return fmt.Errorf("store credential: %w", err)
	}
	fmt.Printf("stored app password for %s\n", account)
	return nil
}

// cmdAuthStatus reads the stored credential and opens an IMAP session to prove
// it is valid end-to-end.
func cmdAuthStatus(ctx context.Context, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: email auth-status <account-email>")
	}
	account := args[0]
	store := credStore()
	cred, err := store.Get(ctx, account)
	if err != nil {
		return fmt.Errorf("read credential: %w", err)
	}
	if cred.Password == "" {
		return fmt.Errorf("no password stored for %s (run `email auth %s`)", account, account)
	}
	ep, err := resolveEndpoint(account)
	if err != nil {
		return err
	}
	p, err := newProvider(ctx, account)
	if err != nil {
		return err
	}
	defer p.Close()
	if _, err := p.ListMailboxes(ctx); err != nil {
		return fmt.Errorf("login failed: %w", err)
	}
	fmt.Printf("login OK for %s (imap %s:%d)\n", account, ep.host, ep.port)
	return nil
}
