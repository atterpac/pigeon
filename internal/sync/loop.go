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

// warmPageSize is the number of newest messages per "page" warmed at launch.
const warmPageSize = 25

// warmGraceWindow caps how long backfill waits for launch warming before
// proceeding anyway, so a slow/stuck warm never permanently blocks history.
const warmGraceWindow = 30 * time.Second

// Options tunes a background account sync loop.
type Options struct {
	// PollInterval is how often to pull mail forward when no push hint arrives.
	PollInterval time.Duration
	// BackfillPageSize is the envelope page size for history paging.
	BackfillPageSize int
	// BackfillMaxPages caps background history pages per mailbox. Zero means
	// unlimited, preserving full-backfill behavior for daemon/CLI callers.
	BackfillMaxPages int
	// BodyWarmPages is how many pages (warmPageSize each) of the primary
	// mailbox to warm — fetch bodies for — at launch, ahead of history
	// backfill, so the first screens open instantly. Zero disables warming.
	BodyWarmPages int
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
	// OnNewMail, if set, is called after each forward sync that brought in new
	// or changed messages, with the upserted messages for that mailbox. Use it
	// to surface desktop notifications. It runs on the sync goroutine, so keep
	// it quick and non-blocking.
	OnNewMail func(acct model.AccountID, mb provider.MailboxRef, msgs []model.Message)
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

	// Launch ordering: forward-sync the inbox, then warm the first pages of
	// bodies, then let history backfill run. `inboxPrimed` releases warming
	// once envelopes exist; `warmDone` releases backfill once warming finishes
	// (or its grace window elapses) so backfill never starves the warm fetch.
	inboxPrimed := make(chan struct{})
	warmDone := make(chan struct{})

	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error { return e.forwardLoop(ctx, p, acct.ID, refs, opt, wait, inboxPrimed) })
	g.Go(func() error { return e.warmLoop(ctx, p, acct.ID, refs, opt, wait, inboxPrimed, warmDone) })
	g.Go(func() error { return e.backfillLoop(ctx, p, acct.ID, refs, opt, wait, warmDone) })
	g.Go(func() error { return e.outboxLoop(ctx, p, acct.ID, opt, wait) })
	g.Go(func() error { return e.snoozeLoop(ctx, acct.ID, opt) })
	return g.Wait()
}

// snoozeLoop returns elapsed snoozes to the inbox on a timer. The relabel is
// queued to the outbox; the outbox loop delivers it, so snooze does no provider
// I/O of its own.
func (e *Engine) snoozeLoop(ctx context.Context, acct model.AccountID, opt Options) error {
	ticker := time.NewTicker(opt.SnoozeInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			n, err := e.ProcessDueSnoozes(ctx, acct, time.Now())
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
func (e *Engine) forwardLoop(ctx context.Context, p provider.Provider, acct model.AccountID, refs []provider.MailboxRef, opt Options, wait func(context.Context) error, primed chan struct{}) error {
	// Best-effort push: if Watch is unsupported, fall back to pure polling.
	var hints <-chan provider.MailboxRef
	if ch, err := p.Watch(ctx); err == nil {
		hints = ch
	}

	ticker := time.NewTicker(opt.PollInterval)
	defer ticker.Stop()

	// notify is false for the priming pass so launch doesn't fire a
	// notification for every message already in the mailbox; later polls and
	// push hints notify normally.
	syncOne := func(ref provider.MailboxRef, notify bool) {
		if err := wait(ctx); err != nil {
			return
		}
		msgs, err := e.SyncForward(ctx, p, acct, ref)
		if err != nil {
			opt.Logger.Warn("forward sync failed", "mailbox", ref.Path, "err", err)
			return
		}
		if len(msgs) > 0 {
			opt.Logger.Info("forward sync", "mailbox", ref.Path, "new", len(msgs))
			if notify && opt.OnNewMail != nil {
				opt.OnNewMail(acct, ref, msgs)
			}
		}
	}
	syncAll := func(notify bool) {
		for _, ref := range refs {
			syncOne(ref, notify)
		}
	}

	syncAll(false) // prime cursors immediately, without notifying
	// Envelopes now exist locally; release launch body-warming.
	close(primed)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			syncAll(true)
		case ref, ok := <-hints:
			// A push hint names the changed mailbox: sync just that one rather
			// than every mailbox. A closed channel (provider stopped watching)
			// is disabled so it can't spin the loop.
			if !ok {
				hints = nil
				continue
			}
			syncOne(ref, true)
		}
	}
}

// warmLoop fetches bodies for the first pages of the primary mailbox at
// launch, ahead of history backfill, then closes warmDone to release backfill.
// It waits for inboxPrimed so envelopes exist before warming, and always closes
// warmDone (even when disabled or on error) so backfill is never stuck.
func (e *Engine) warmLoop(ctx context.Context, p provider.Provider, acct model.AccountID, refs []provider.MailboxRef, opt Options, wait func(context.Context) error, primed, warmDone chan struct{}) error {
	defer close(warmDone)
	if opt.BodyWarmPages <= 0 || len(refs) == 0 {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-primed:
	}
	if err := wait(ctx); err != nil {
		return err
	}
	// Warm only the primary (first) mailbox — the inbox the user opens first.
	ref := refs[0]
	n, err := e.WarmBodies(ctx, p, acct, ref.ID, opt.BodyWarmPages*warmPageSize)
	if err != nil {
		opt.Logger.Warn("body warm failed", "mailbox", ref.Path, "err", err)
		return nil
	}
	if n > 0 {
		opt.Logger.Info("body warm", "mailbox", ref.Path, "bodies", n)
	}
	return nil
}

// backfillLoop pages history for each mailbox until complete, then exits. It
// waits for launch warming to finish first (bounded by warmGraceWindow) so the
// first screens' bodies load before history paging consumes the rate limiter.
func (e *Engine) backfillLoop(ctx context.Context, p provider.Provider, acct model.AccountID, refs []provider.MailboxRef, opt Options, wait func(context.Context) error, warmDone chan struct{}) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-warmDone:
	case <-time.After(warmGraceWindow):
		opt.Logger.Info("backfill proceeding before warm completed")
	}
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
