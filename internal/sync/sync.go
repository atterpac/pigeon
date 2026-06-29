// Package sync is the reconciliation engine. It is the only component that
// talks to both the store and providers. It registers accounts, pages history
// into the store (Backfill), and pulls new mail forward (Sync), persisting
// per-mailbox cursors so both resume across runs.
package sync

import (
	"context"
	"fmt"
	"sync"

	"github.com/atterpac/email/internal/model"
	"github.com/atterpac/email/internal/provider"
	"github.com/atterpac/email/internal/store"
)

// Engine orchestrates sync between providers and the local store.
type Engine struct {
	store *store.Store

	// drainMu guards drains, keyed per account, so concurrent DrainOutbox calls
	// (the outbox loop, command-triggered sends, and others sharing this Engine)
	// can't both read the same ready op and deliver it twice. There is no
	// store-level lease, so serialization lives here.
	mu     sync.Mutex
	drains map[model.AccountID]*sync.Mutex
	nudges map[model.AccountID]chan struct{}

	// bodies coalesces concurrent body fetches for the same message so a
	// foreground open and a background warm don't both hit the network for it.
	bodies *inflightBodies
}

// New returns an engine bound to a store.
func New(s *store.Store) *Engine {
	return &Engine{
		store:  s,
		drains: map[model.AccountID]*sync.Mutex{},
		nudges: map[model.AccountID]chan struct{}{},
		bodies: newInflightBodies(),
	}
}

// drainLock returns the per-account mutex that serializes outbox drains.
func (e *Engine) drainLock(acct model.AccountID) *sync.Mutex {
	e.mu.Lock()
	defer e.mu.Unlock()
	m := e.drains[acct]
	if m == nil {
		m = &sync.Mutex{}
		e.drains[acct] = m
	}
	return m
}

// drainNudge returns the per-account channel that requests a prompt background
// outbox drain. Buffered (cap 1): a pending nudge already guarantees the loop
// will drain and pick up any newly-enqueued op, so extra nudges coalesce. The
// producer (NudgeOutbox) and consumer (outboxLoop) share one channel per account.
func (e *Engine) drainNudge(acct model.AccountID) chan struct{} {
	e.mu.Lock()
	defer e.mu.Unlock()
	ch := e.nudges[acct]
	if ch == nil {
		ch = make(chan struct{}, 1)
		e.nudges[acct] = ch
	}
	return ch
}

// NudgeOutbox requests a prompt background drain for acct without blocking.
// Callers that mutate locally and enqueue a durable op then nudge here get
// near-immediate server delivery while returning as soon as the change is
// durable. If no outbox loop is running, the op still drains on the next launch.
func (e *Engine) NudgeOutbox(acct model.AccountID) {
	select {
	case e.drainNudge(acct) <- struct{}{}:
	default: // a drain is already pending; it will pick up this op too
	}
}

// RegisterAccount persists the account and its mailbox topology.
func (e *Engine) RegisterAccount(ctx context.Context, p provider.Provider, acct model.Account) ([]model.Mailbox, error) {
	if err := e.store.UpsertAccount(ctx, acct); err != nil {
		return nil, err
	}
	mbs, err := p.ListMailboxes(ctx)
	if err != nil {
		return nil, err
	}
	if err := e.store.UpsertMailboxes(ctx, mbs); err != nil {
		return nil, err
	}
	return mbs, nil
}

// SyncForward pulls mail newer than the stored cursor into the store and returns
// the messages written (new or changed envelopes).
func (e *Engine) SyncForward(ctx context.Context, p provider.Provider, acct model.AccountID, mb provider.MailboxRef) ([]model.Message, error) {
	curBytes, err := e.store.GetCursor(ctx, acct, mb.ID)
	if err != nil {
		return nil, err
	}
	ch, next, err := p.Sync(ctx, mb, wrap(curBytes))
	if err != nil {
		return nil, err
	}
	if err := e.applyChanges(ctx, acct, ch); err != nil {
		return nil, err
	}
	if next != nil {
		if err := e.store.SetCursor(ctx, acct, mb.ID, next.Bytes); err != nil {
			return nil, err
		}
	}
	return ch.Upserted, nil
}

// applyChanges persists a provider delta: upserts, flag/label changes, and
// deletions.
func (e *Engine) applyChanges(ctx context.Context, acct model.AccountID, ch provider.Changes) error {
	if err := e.store.SaveMessages(ctx, ch.Upserted); err != nil {
		return err
	}
	if len(ch.Flagged) > 0 {
		deltas := make([]store.FlagDelta, len(ch.Flagged))
		for i, f := range ch.Flagged {
			deltas[i] = store.FlagDelta{
				ID:           f.ID,
				AddFlags:     f.AddFlags,
				RemoveFlags:  f.RemoveFlags,
				AddLabels:    f.AddLabels,
				RemoveLabels: f.RemoveLabels,
			}
		}
		if err := e.store.ApplyFlagDeltas(ctx, acct, deltas); err != nil {
			return err
		}
	}
	if len(ch.Removed) > 0 {
		if err := e.store.DeleteMessages(ctx, acct, ch.Removed); err != nil {
			return err
		}
	}
	return nil
}

// BackfillPage pages one batch of older history into the store. It returns the
// count written and whether backfill is complete for this mailbox. Call it in a
// loop (or on demand, e.g. "load older") until done is true.
func (e *Engine) BackfillPage(ctx context.Context, p provider.Provider, acct model.AccountID, mb provider.MailboxRef, limit int) (n int, done bool, err error) {
	pageBytes, alreadyDone, err := e.store.GetBackfillState(ctx, acct, mb.ID)
	if err != nil {
		return 0, false, err
	}
	// History is already fully indexed for this mailbox — nothing to fetch.
	// This prevents re-paging the entire mailbox from newest on every launch.
	if alreadyDone {
		return 0, true, nil
	}
	ch, next, done, err := p.Backfill(ctx, mb, wrap(pageBytes), limit)
	if err != nil {
		return 0, false, err
	}
	if err := e.store.SaveMessages(ctx, ch.Upserted); err != nil {
		return 0, false, err
	}
	// Persist paging position, or record completion so the next launch skips
	// this mailbox entirely instead of restarting from newest.
	if done {
		if err := e.store.MarkBackfillDone(ctx, acct, mb.ID); err != nil {
			return 0, false, err
		}
	} else {
		var nextBytes []byte
		if next != nil {
			nextBytes = next.Bytes
		}
		if err := e.store.SetBackfill(ctx, acct, mb.ID, nextBytes); err != nil {
			return 0, false, err
		}
	}
	return len(ch.Upserted), done, nil
}

// BackfillAll runs BackfillPage repeatedly until the mailbox is fully indexed.
// onPage, if non-nil, is called after each page with the running total.
func (e *Engine) BackfillAll(ctx context.Context, p provider.Provider, acct model.AccountID, mb provider.MailboxRef, pageSize int, onPage func(total int)) (int, error) {
	total := 0
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		n, done, err := e.BackfillPage(ctx, p, acct, mb, pageSize)
		if err != nil {
			return total, fmt.Errorf("backfill page: %w", err)
		}
		total += n
		if onPage != nil {
			onPage(total)
		}
		if done {
			return total, nil
		}
	}
}

// wrap converts stored bytes into a provider.Cursor (nil when empty).
func wrap(b []byte) *provider.Cursor {
	if len(b) == 0 {
		return nil
	}
	return &provider.Cursor{Bytes: b}
}
