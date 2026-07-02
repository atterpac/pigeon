package imap

import (
	"context"
	"fmt"
	"time"

	"github.com/emersion/go-imap/v2/imapclient"

	"github.com/atterpac/pigeon/internal/model"
	"github.com/atterpac/pigeon/internal/provider"
)

// idleRearm is how often to drop and re-issue IDLE. Servers typically time out
// idle connections at ~30 minutes; re-arming well before that keeps it alive.
const idleRearm = 24 * time.Minute

// Watch opens a dedicated connection and issues IMAP IDLE on INBOX, emitting a
// hint whenever the mailbox changes (new message / expunge). The engine reacts
// by running a forward sync. The channel closes when ctx is cancelled or the
// connection drops. Returns an error if the server lacks IDLE.
func (p *Provider) Watch(ctx context.Context) (<-chan provider.MailboxRef, error) {
	ref := provider.MailboxRef{ID: model.LabelID("INBOX"), Path: "INBOX"}
	hints := make(chan provider.MailboxRef, 1)
	emit := func() {
		select {
		case hints <- ref:
		default: // a hint is already pending; coalesce
		}
	}

	addr := fmt.Sprintf("%s:%d", p.cfg.Host, p.cfg.Port)
	c, err := imapclient.DialTLS(addr, &imapclient.Options{
		UnilateralDataHandler: &imapclient.UnilateralDataHandler{
			Mailbox: func(d *imapclient.UnilateralDataMailbox) {
				if d.NumMessages != nil {
					emit()
				}
			},
			Expunge: func(uint32) { emit() },
		},
	})
	if err != nil {
		return nil, fmt.Errorf("imap watch dial: %w", err)
	}
	saslClient, err := p.saslClient()
	if err != nil {
		c.Close()
		return nil, err
	}
	if err := c.Authenticate(saslClient); err != nil {
		c.Close()
		return nil, fmt.Errorf("imap watch auth: %w", err)
	}
	if !c.Caps().Has("IDLE") {
		c.Close()
		return nil, fmt.Errorf("imap: server does not support IDLE")
	}
	if _, err := c.Select("INBOX", nil).Wait(); err != nil {
		c.Close()
		return nil, fmt.Errorf("imap watch select: %w", err)
	}

	go func() {
		defer close(hints)
		defer c.Close()
		for {
			if ctx.Err() != nil {
				return
			}
			idle, err := c.Idle()
			if err != nil {
				return // connection lost; engine keeps polling as fallback
			}
			t := time.NewTimer(idleRearm)
			select {
			case <-ctx.Done():
				t.Stop()
				idle.Close()
				_ = idle.Wait()
				return
			case <-t.C:
				// Re-arm: stop this IDLE and loop to start a fresh one.
				if err := idle.Close(); err != nil {
					return
				}
				if err := idle.Wait(); err != nil {
					return
				}
			}
		}
	}()
	return hints, nil
}
