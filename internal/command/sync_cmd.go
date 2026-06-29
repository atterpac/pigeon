package command

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/atterpac/email/internal/blob"
	"github.com/atterpac/email/internal/model"
	"github.com/atterpac/email/internal/store"
	synceng "github.com/atterpac/email/internal/sync"
)

// dbPath resolves the local store location (EMAIL_DB or the XDG data default).
func dbPath() (string, error) {
	if p := os.Getenv("EMAIL_DB"); p != "" {
		return p, nil
	}
	dir := os.Getenv("XDG_DATA_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("locate home dir (set EMAIL_DB or XDG_DATA_HOME): %w", err)
		}
		dir = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(dir, "email", "mail.db"), nil
}

func openStore(ctx context.Context) (*store.Store, error) {
	path, err := dbPath()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	blobDir := filepath.Join(filepath.Dir(path), "blobs")
	return store.Open(ctx, path, store.WithBlobStore(blob.NewFS(blobDir)))
}

// cmdSync: register account + mailboxes, backfill N pages of history, then pull
// new mail forward — all into the local SQLite store. Envelopes only.
//
//	email sync <account-email> [mailbox=INBOX] [pageSize=100] [pages=1]
//	(pages=0 means backfill the entire mailbox)
func cmdSync(ctx context.Context, args []string) (err error) {
	if len(args) < 1 {
		return fmt.Errorf("usage: email sync <account-email> [mailbox=INBOX] [pageSize=100] [pages=1]")
	}
	account := args[0]
	mailbox := argOr(args, 1, "INBOX")
	pageSize := atoiOr(argOr(args, 2, "100"), 100)
	pages := atoiOr(argOr(args, 3, "1"), 1)

	p, err := newProvider(ctx, account)
	if err != nil {
		return err
	}
	defer func() { _ = p.Close() }()

	st, err := openStore(ctx)
	if err != nil {
		return err
	}
	// Backfill writes envelopes through st; surface a close/flush error if nothing else failed.
	defer func() {
		if cerr := st.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()
	eng := synceng.New(st)

	acct := model.Account{ID: model.AccountID(account), Kind: model.KindIMAP, Email: account}
	mbs, err := eng.RegisterAccount(ctx, p, acct)
	if err != nil {
		return err
	}
	fmt.Printf("registered %s: %d mailboxes\n", account, len(mbs))

	ref := provRef(mailbox)

	if pages == 0 {
		total, err := eng.BackfillAll(ctx, p, acct.ID, ref, pageSize, func(t int) {
			fmt.Printf("\rbackfill %q: %d messages...", mailbox, t)
		})
		fmt.Println()
		if err != nil {
			return err
		}
		fmt.Printf("backfill complete: %d messages\n", total)
	} else {
		written := 0
		for i := 0; i < pages; i++ {
			n, done, err := eng.BackfillPage(ctx, p, acct.ID, ref, pageSize)
			if err != nil {
				return err
			}
			written += n
			fmt.Printf("backfill page %d: +%d (total %d)\n", i+1, n, written)
			if done {
				fmt.Println("backfill complete (reached oldest)")
				break
			}
		}
	}

	// Pull anything newer than our forward cursor.
	msgs, err := eng.SyncForward(ctx, p, acct.ID, ref)
	if err != nil {
		return err
	}
	fmt.Printf("forward sync: +%d new\n", len(msgs))
	return nil
}

// cmdSearch runs a local FTS query — no network.
func cmdSearch(ctx context.Context, args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("usage: email search <account-email> <query>")
	}
	st, err := openStore(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	msgs, err := st.Search(ctx, model.AccountID(args[0]), args[1], 25)
	if err != nil {
		return err
	}
	fmt.Printf("%d results for %q:\n", len(msgs), args[1])
	for _, m := range msgs {
		fmt.Printf("  %-30.30s  %-50.50s  %s\n", firstFrom(m), m.Subject, m.Date.Format("2006-01-02"))
	}
	return nil
}

func argOr(args []string, i int, def string) string {
	if i < len(args) {
		return args[i]
	}
	return def
}

func atoiOr(s string, def int) int {
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return def
}
