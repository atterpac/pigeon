// Package sync is the reconciliation engine. It is the only component that
// talks to both the store and providers. It registers accounts, pages history
// into the store (Backfill), and pulls new mail forward (Sync), persisting
// per-mailbox cursors so both resume across runs.
package sync

import (
	"context"
	"fmt"

	"github.com/atterpac/email/internal/model"
	"github.com/atterpac/email/internal/provider"
	"github.com/atterpac/email/internal/store"
)

// Engine orchestrates sync between providers and the local store.
type Engine struct {
	store *store.Store
}

// New returns an engine bound to a store.
func New(s *store.Store) *Engine { return &Engine{store: s} }

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
// the number of messages written.
func (e *Engine) SyncForward(ctx context.Context, p provider.Provider, acct model.AccountID, mb provider.MailboxRef) (int, error) {
	curBytes, err := e.store.GetCursor(ctx, acct, mb.ID)
	if err != nil {
		return 0, err
	}
	ch, next, err := p.Sync(ctx, mb, wrap(curBytes))
	if err != nil {
		return 0, err
	}
	if err := e.applyChanges(ctx, acct, ch); err != nil {
		return 0, err
	}
	if next != nil {
		if err := e.store.SetCursor(ctx, acct, mb.ID, next.Bytes); err != nil {
			return 0, err
		}
	}
	return len(ch.Upserted), nil
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
	pageBytes, err := e.store.GetBackfill(ctx, acct, mb.ID)
	if err != nil {
		return 0, false, err
	}
	ch, next, done, err := p.Backfill(ctx, mb, wrap(pageBytes), limit)
	if err != nil {
		return 0, false, err
	}
	if err := e.store.SaveMessages(ctx, ch.Upserted); err != nil {
		return 0, false, err
	}
	// Persist paging position. When done, store an empty marker so we don't
	// restart from newest on the next call.
	var nextBytes []byte
	if next != nil {
		nextBytes = next.Bytes
	}
	if err := e.store.SetBackfill(ctx, acct, mb.ID, nextBytes); err != nil {
		return 0, false, err
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
