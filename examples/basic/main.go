// Command basic is a minimal end-to-end example of the email SDK: open a
// client, register a Gmail account, sync the inbox, and print recent threads.
// It reuses the same credential flow as cmd/email (run `email auth <addr>` and
// set EMAIL_CRED_STORE / EMAIL_CRED_PASSPHRASE first).
//
//	go run ./examples/basic michael@getgalaxy.io
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/atterpac/email/internal/auth"
	"github.com/atterpac/email/internal/model"
	"github.com/atterpac/email/internal/provider"
	gmailprov "github.com/atterpac/email/internal/provider/gmail"
	"github.com/atterpac/email/pkg/email"
)

func main() {
	if len(os.Args) != 2 {
		log.Fatal("usage: go run ./examples/basic <account-email>")
	}
	addr := os.Args[1]
	ctx := context.Background()

	client, err := email.Open(ctx, email.Config{
		DBPath:   dbPath(),
		Provider: gmailFactory(),
	})
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Bind a Session so we don't repeat the account on every call.
	s := client.Session(email.Account{
		ID: email.AccountID(addr), Kind: email.KindGmail, Email: addr,
	})

	mboxes, err := s.Register(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("registered %s — %d mailboxes\n", addr, len(mboxes))

	// Pull the first page of INBOX into the local store.
	if n, err := s.SyncOnce(ctx, "INBOX"); err != nil {
		log.Fatal(err)
	} else {
		fmt.Printf("synced %d new messages\n", n)
	}

	// Live updates while we run.
	evs, cancel := s.Events()
	defer cancel()
	go func() {
		for e := range evs {
			fmt.Printf("[event] %s %d message(s)\n", e.Kind, len(e.IDs))
		}
	}()

	// Read from the local store (instant, offline).
	threads, err := s.Threads(ctx, 10)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("\n%d recent threads:\n", len(threads))
	for _, t := range threads {
		fmt.Printf("  %-50.50s  %s\n", t.Subject, t.Last.Format("2006-01-02"))
	}

	// Background sync for a short demo window.
	_ = s.StartSync(ctx, []email.LabelID{"INBOX"}, email.SyncOptions{})
	time.Sleep(2 * time.Second)
}

func dbPath() string {
	if p := os.Getenv("EMAIL_DB"); p != "" {
		return p
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "email", "mail.db")
}

// gmailFactory builds a Gmail REST provider from the stored credential. This is
// the one piece an application owns — where credentials live.
func gmailFactory() email.ProviderFactory {
	return func(ctx context.Context, a email.Account) (provider.Provider, error) {
		credPath := os.Getenv("EMAIL_GOOGLE_CREDENTIALS")
		if credPath == "" {
			home, _ := os.UserHomeDir()
			credPath = filepath.Join(home, "creds", "google", "birdie", "client_secret.json")
		}
		cfg, err := auth.LoadGoogleConfig(credPath)
		if err != nil {
			return nil, err
		}
		store := credStore()
		cred, err := store.Get(ctx, a.Email)
		if err != nil {
			return nil, fmt.Errorf("no credential for %s (run `email auth %s`): %w", a.Email, a.Email, err)
		}
		ts := auth.TokenSource(ctx, cfg, store, a.Email, cred)
		return gmailprov.New(ctx, model.AccountID(a.ID), ts)
	}
}

func credStore() auth.CredentialStore {
	switch os.Getenv("EMAIL_CRED_STORE") {
	case "encrypted":
		path := os.Getenv("EMAIL_CRED_FILE")
		if path == "" {
			path = auth.DefaultFilePath()
		}
		return auth.NewEncrypted(path, auth.PassphraseFromEnv())
	case "file":
		path := os.Getenv("EMAIL_CRED_FILE")
		if path == "" {
			path = auth.DefaultFilePath()
		}
		return auth.NewFile(path)
	default:
		return auth.NewKeyring()
	}
}
