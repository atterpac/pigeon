package command

import (
	"context"
	"fmt"

	"github.com/atterpac/email/internal/model"
	"github.com/atterpac/email/internal/provider"
)

func provRef(mailbox string) provider.MailboxRef {
	return provider.MailboxRef{ID: model.LabelID(mailbox), Path: mailbox}
}

func cmdImapList(ctx context.Context, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: email imap-list <account-email>")
	}
	p, err := newProvider(ctx, args[0])
	if err != nil {
		return err
	}
	defer p.Close()

	mbs, err := p.ListMailboxes(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("%-30s %-8s %6s %6s\n", "MAILBOX", "ROLE", "UNREAD", "TOTAL")
	for _, mb := range mbs {
		fmt.Printf("%-30s %-8d %6d %6d\n", mb.Name, mb.Role, mb.Unread, mb.Total)
	}
	return nil
}

func cmdImapSync(ctx context.Context, args []string) error {
	if len(args) < 1 || len(args) > 2 {
		return fmt.Errorf("usage: email imap-sync <account-email> [mailbox=INBOX]")
	}
	mailbox := "INBOX"
	if len(args) == 2 {
		mailbox = args[1]
	}
	p, err := newProvider(ctx, args[0])
	if err != nil {
		return err
	}
	defer p.Close()

	ch, cur, err := p.Sync(ctx, provRef(mailbox), nil)
	if err != nil {
		return err
	}
	fmt.Printf("synced %q: %d messages (cursor %d bytes)\n", mailbox, len(ch.Upserted), len(cur.Bytes))
	n := min(len(ch.Upserted), 10)
	for _, m := range ch.Upserted[:n] {
		from := ""
		if len(m.From) > 0 {
			from = m.From[0].Addr
		}
		fmt.Printf("  %-40.40s  %s\n", from, m.Subject)
	}
	return nil
}
