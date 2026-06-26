package sync

import (
	"context"
	"time"

	"github.com/atterpac/email/internal/model"
	"github.com/atterpac/email/internal/provider"
)

// inboxLabel is the label re-applied when a snooze elapses.
const inboxLabel model.LabelID = "INBOX"

// ProcessDueSnoozes returns snoozed messages whose timer has elapsed back to the
// inbox (re-adds the INBOX label) and clears their snooze records. Returns the
// number un-snoozed.
func (e *Engine) ProcessDueSnoozes(ctx context.Context, p provider.Provider, acct model.AccountID, now time.Time) (int, error) {
	due, err := e.store.DueSnoozes(ctx, acct, now)
	if err != nil {
		return 0, err
	}
	if len(due) == 0 {
		return 0, nil
	}
	// Re-add INBOX (optimistic local + queued provider mutation) and drain.
	if err := e.ApplyLabels(ctx, acct, due, []model.LabelID{inboxLabel}, nil); err != nil {
		return 0, err
	}
	if _, err := e.DrainOutbox(ctx, p, acct); err != nil {
		return 0, err
	}
	if err := e.store.Unsnooze(ctx, acct, due); err != nil {
		return 0, err
	}
	return len(due), nil
}
