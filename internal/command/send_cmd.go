package command

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/atterpac/email/internal/mime"
	"github.com/atterpac/email/internal/model"
	"github.com/atterpac/email/internal/provider"
	synceng "github.com/atterpac/email/internal/sync"
)

// cmdSend composes a message, queues it in the outbox, and drains once so it is
// delivered immediately (the daemon would otherwise drain on its timer).
//
//	email send <account-email> <to> <subject> <body...>
func cmdSend(ctx context.Context, args []string) error {
	if len(args) < 4 {
		return fmt.Errorf("usage: email send <account-email> <to> <subject> <body...>")
	}
	account := args[0]
	to := args[1]
	subject := args[2]
	body := strings.Join(args[3:], " ")

	out := model.Outgoing{
		From:    model.Address{Addr: account},
		To:      parseRecipients(to),
		Subject: subject,
		Text:    body,
	}
	raw, err := mime.Build(out, time.Now(), genMessageID(account))
	if err != nil {
		return err
	}

	p, err := newProvider(ctx, account)
	if err != nil {
		return err
	}
	defer p.Close()

	st, err := openStore(ctx)
	if err != nil {
		return err
	}
	defer st.Close()
	eng := synceng.New(st)

	acct := model.AccountID(account)
	if err := eng.EnqueueSend(ctx, acct, model.RawMessage{Bytes: raw}, provider.SendOpts{Thread: out.Thread}); err != nil {
		return err
	}
	fmt.Println("queued in outbox; sending...")

	n, err := eng.DrainOutbox(ctx, p, acct)
	if err != nil {
		return fmt.Errorf("send failed (left in outbox for retry): %w", err)
	}
	if n == 0 {
		fmt.Println("not sent yet — left in outbox (will retry on daemon tick)")
	} else {
		fmt.Printf("sent %d message(s) to %s\n", n, to)
	}
	return nil
}

func parseRecipients(csv string) []model.Address {
	var out []model.Address
	for _, p := range strings.Split(csv, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, model.Address{Addr: p})
		}
	}
	return out
}

// genMessageID builds a unique RFC 5322 Message-ID using the sender's domain.
func genMessageID(from string) string {
	domain := "localhost"
	if i := strings.LastIndexByte(from, '@'); i >= 0 {
		domain = from[i+1:]
	}
	var b [16]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("<%s@%s>", hex.EncodeToString(b[:]), domain)
}
