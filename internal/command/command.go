// Package command implements the reference CLI for the email SDK. It is a thin
// harness over the same internals the public email.Client uses; cmd/email just
// calls Run. Keeping it here (rather than in package main) lets a different
// front-end — e.g. a Wails app — own main without dragging the CLI along.
package command

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

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

// googleCredPath resolves the client_secret.json location.
func googleCredPath() string {
	if p := os.Getenv("EMAIL_GOOGLE_CREDENTIALS"); p != "" {
		return p
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "creds", "google", "birdie", "client_secret.json")
}

// cmdAuth runs the Gmail OAuth loopback flow and stores the credential.
func cmdAuth(ctx context.Context, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: email auth <account-email>")
	}
	account := args[0]
	cfg, err := auth.LoadGoogleConfig(googleCredPath())
	if err != nil {
		return err
	}
	cred, err := auth.InteractiveAuth(ctx, cfg, openBrowser)
	if err != nil {
		return err
	}
	store := credStore()
	if err := store.Set(ctx, account, cred); err != nil {
		return fmt.Errorf("store credential: %w", err)
	}
	fmt.Printf("authorized %s — refresh token stored\n", account)
	return nil
}

// cmdAuthStatus reads the stored credential and forces a token refresh to prove
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
	fmt.Printf("stored: oauth=%v hasRefresh=%v expiry=%s\n",
		cred.IsOAuth(), cred.RefreshToken != "", cred.Expiry.Format("2006-01-02 15:04:05"))

	cfg, err := auth.LoadGoogleConfig(googleCredPath())
	if err != nil {
		return err
	}
	ts := auth.TokenSource(ctx, cfg, store, account, cred)
	tok, err := ts.Token()
	if err != nil {
		return fmt.Errorf("refresh: %w", err)
	}
	fmt.Printf("refresh OK — access token valid until %s\n", tok.Expiry.Format("2006-01-02 15:04:05"))
	return nil
}

func openBrowser(url string) error {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
	case "windows":
		cmd, args = "rundll32", []string{"url.dll,FileProtocolHandler"}
	default:
		cmd = "xdg-open"
	}
	return exec.Command(cmd, append(args, url)...).Start()
}
