package sync

import (
	"context"
	"log/slog"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"

	"github.com/atterpac/email/internal/model"
	"github.com/atterpac/email/internal/provider"
)

// Options tunes a background account sync loop.
type Options struct {
	// PollInterval is how often to pull mail forward when no push hint arrives.
	PollInterval time.Duration
	// BackfillPageSize is the envelope page size for history paging.
	BackfillPageSize int
	// BackfillMaxPages caps background history pages per mailbox. Zero means
	// unlimited, preserving full-backfill behavior for daemon/CLI callers.
	BackfillMaxPages int
	// OutboxInterval is how often to drain queued outbound sends.
	OutboxInterval time.Duration
	// SnoozeInterval is how often to check for elapsed snoozes.
	SnoozeInterval time.Duration
	// Rate caps provider requests/sec; Burst is the bucket size. Zero Rate
	// means unlimited.
	Rate  rate.Limit
	Burst int
	// Logger receives progress/error events; defaults to slog.Default().
	Logger *slog.Logger
}

func (o Options) withDefaults() Options {
	if o.PollInterval <= 0 {
		o.PollInterval = 60 * time.Second
	}
	if o.OutboxInterval <= 0 {
		o.OutboxInterval = 15 * time.Second
	}
	if o.SnoozeInterval <= 0 {
		o.SnoozeInterval = 60 * time.Second
	}
	if o.BackfillPageSize <= 0 {
		o.BackfillPageSize = 100
	}
	if o.Burst <= 0 {
		o.Burst = 5
	}
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	return o
}

// RunAccount drives a single account until ctx is cancelled. It runs two
// cooperating loops over the given mailboxes:
//
//   - forward: establishes a sync cursor, then polls (and reacts to provider
//     push hints, where supported) for new mail.
//   - backfill: pages history newest→oldest in the background, resumable across
//     restarts, until each mailbox is fully indexed.
//
// Both share a rate limiter so a large backfill never starves foreground sync
// or trips provider quotas. RunAccount blocks; call it in its own goroutine.
func (e *Engine) RunAccount(ctx context.Context, p provider.Provider, acct model.Account, refs []provider.MailboxRef, opt Options) error {
	opt = opt.withDefaults()
	var lim *rate.Limiter
	if opt.Rate > 0 {
		lim = rate.NewLimiter(opt.Rate, opt.Burst)
	}
	wait := func(ctx context.Context) error {
		if lim == nil {
			return ctx.Err()
		}
		return lim.Wait(ctx)
	}

	if _, err := e.RegisterAccount(ctx, p, acct); err != nil {
		return err
	}

	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error { return e.forwardLoop(ctx, p, acct.ID, refs, opt, wait) })
	g.Go(func() error { return e.backfillLoop(ctx, p, acct.ID, refs, opt, wait) })
	g.Go(func() error { return e.outboxLoop(ctx, p, acct.ID, opt, wait) })
	g.Go(func() error { return e.snoozeLoop(ctx, p, acct.ID, opt) })
	return g.Wait()
}

// snoozeLoop returns elapsed snoozes to the inbox on a timer.
func (e *Engine) snoozeLoop(ctx context.Context, p provider.Provider, acct model.AccountID, opt Options) error {
	ticker := time.NewTicker(opt.SnoozeInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			n, err := e.ProcessDueSnoozes(ctx, p, acct, time.Now())
			if err != nil {
				opt.Logger.Warn("snooze processing failed", "err", err)
				continue
			}
			if n > 0 {
				opt.Logger.Info("unsnoozed", "count", n)
			}
		}
	}
}

// outboxLoop drains queued outbound sends on a timer.
func (e *Engine) outboxLoop(ctx context.Context, p provider.Provider, acct model.AccountID, opt Options, wait func(context.Context) error) error {
	ticker := time.NewTicker(opt.OutboxInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := wait(ctx); err != nil {
				return err
			}
			n, err := e.DrainOutbox(ctx, p, acct)
			if err != nil {
				opt.Logger.Warn("outbox drain failed", "err", err)
				continue
			}
			if n > 0 {
				opt.Logger.Info("outbox", "sent", n)
			}
		}
	}
}

// forwardLoop polls for new mail, reacting to push hints when the provider
// exposes a Watch channel.
func (e *Engine) forwardLoop(ctx context.Context, p provider.Provider, acct model.AccountID, refs []provider.MailboxRef, opt Options, wait func(context.Context) error) error {
	// Best-effort push: if Watch is unsupported, fall back to pure polling.
	var hints <-chan provider.MailboxRef
	if ch, err := p.Watch(ctx); err == nil {
		hints = ch
	}

	ticker := time.NewTicker(opt.PollInterval)
	defer ticker.Stop()

	syncAll := func() {
		for _, ref := range refs {
			if err := wait(ctx); err != nil {
				return
			}
			n, err := e.SyncForward(ctx, p, acct, ref)
			if err != nil {
				opt.Logger.Warn("forward sync failed", "mailbox", ref.Path, "err", err)
				continue
			}
			if n > 0 {
				opt.Logger.Info("forward sync", "mailbox", ref.Path, "new", n)
			}
		}
	}

	syncAll() // prime cursors immediately
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			syncAll()
		case <-hints:
			syncAll()
		}
	}
}

// backfillLoop pages history for each mailbox until complete, then exits.
func (e *Engine) backfillLoop(ctx context.Context, p provider.Provider, acct model.AccountID, refs []provider.MailboxRef, opt Options, wait func(context.Context) error) error {
	for _, ref := range refs {
		pages := 0
		for {
			if opt.BackfillMaxPages > 0 && pages >= opt.BackfillMaxPages {
				opt.Logger.Info("backfill paused", "mailbox", ref.Path, "pages", pages)
				break
			}
			if err := wait(ctx); err != nil {
				return err
			}
			n, done, err := e.BackfillPage(ctx, p, acct, ref, opt.BackfillPageSize)
			if err != nil {
				opt.Logger.Warn("backfill page failed", "mailbox", ref.Path, "err", err)
				// Brief pause before retrying so transient errors don't spin.
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(5 * time.Second):
				}
				continue
			}
			if n > 0 {
				opt.Logger.Info("backfill", "mailbox", ref.Path, "page", n)
			}
			pages++
			if done {
				opt.Logger.Info("backfill complete", "mailbox", ref.Path)
				break
			}
		}
	}
	return nil
}
