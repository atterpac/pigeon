package command

import (
	"context"
	"fmt"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"golang.org/x/time/rate"

	"github.com/atterpac/email/internal/model"
	"github.com/atterpac/email/internal/provider"
	synceng "github.com/atterpac/email/internal/sync"
)

// cmdDaemon runs the continuous background sync loop for an account: forward
// polling + resumable backfill, rate-limited, until interrupted.
//
//	email daemon <account-email> [labels=INBOX,SENT] [pollSeconds=60] [pageSize=100] [rps=5]
//
// For Gmail over IMAP, backfilling the "[Gmail]/All Mail" label is recommended
// to avoid per-folder duplication — here we default to INBOX for clarity.
func cmdDaemon(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: email daemon <account-email> [labels=INBOX] [pollSeconds=60] [pageSize=100] [rps=5]")
	}
	account := args[0]
	labels := splitCSV(argOr(args, 1, "INBOX"))
	poll := time.Duration(atoiOr(argOr(args, 2, "60"), 60)) * time.Second
	pageSize := atoiOr(argOr(args, 3, "100"), 100)
	rps := atoiOr(argOr(args, 4, "5"), 5)

	p, err := newProvider(ctx, account)
	if err != nil {
		return err
	}
	defer func() { _ = p.Close() }()

	st, err := openStore(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()
	eng := synceng.New(st)

	refs := make([]provider.MailboxRef, len(labels))
	for i, l := range labels {
		refs[i] = provRef(l)
	}

	// Cancel cleanly on SIGINT/SIGTERM.
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	acct := model.Account{ID: model.AccountID(account), Kind: model.KindIMAP, Email: account}
	fmt.Printf("daemon started for %s (labels=%v, poll=%s, rps=%d)\n", account, labels, poll, rps)

	err = eng.RunAccount(ctx, p, acct, refs, synceng.Options{
		PollInterval:     poll,
		BackfillPageSize: pageSize,
		Rate:             rate.Limit(rps),
		Burst:            rps,
	})
	if ctx.Err() != nil {
		fmt.Println("\ndaemon stopped")
		return nil
	}
	return err
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
