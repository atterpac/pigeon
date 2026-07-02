package sync

import (
	"context"
	"time"

	"github.com/atterpac/pigeon/internal/model"
)

// inboxLabel is the label re-applied when a snooze elapses.
const inboxLabel model.LabelID = "INBOX"

// ProcessDueSnoozes returns snoozed messages whose timer has elapsed back to the
// inbox (re-adds the INBOX label) and clears their snooze records. Returns the
// number un-snoozed. The provider relabel is left queued in the outbox; the
// rate-limited outbox loop delivers it, so this neither bypasses the limiter nor
// drains concurrently with it.
func (e *Engine) ProcessDueSnoozes(ctx context.Context, acct model.AccountID, now time.Time) (int, error) {
	due, err := e.store.DueSnoozes(ctx, acct, now)
	if err != nil {
		return 0, err
	}
	if len(due) == 0 {
		return 0, nil
	}
	// Re-add INBOX optimistically and queue the provider mutation.
	if err := e.ApplyLabels(ctx, acct, due, []model.LabelID{inboxLabel}, nil); err != nil {
		return 0, err
	}
	if err := e.store.Unsnooze(ctx, acct, due); err != nil {
		return 0, err
	}
	return len(due), nil
}
