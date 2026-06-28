package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/atterpac/email/internal/model"
	gen "github.com/atterpac/email/internal/store/db"
)

// Sync cursors are stored as opaque bytes; the sync engine owns their meaning.

func (s *Store) GetCursor(ctx context.Context, account model.AccountID, mailbox model.LabelID) ([]byte, error) {
	b, err := s.q.GetCursor(ctx, gen.GetCursorParams{Account: string(account), Mailbox: string(mailbox)})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return b, err
}

func (s *Store) SetCursor(ctx context.Context, account model.AccountID, mailbox model.LabelID, cur []byte) error {
	return s.q.SetCursor(ctx, gen.SetCursorParams{Account: string(account), Mailbox: string(mailbox), Cursor: cur})
}

func (s *Store) GetBackfill(ctx context.Context, account model.AccountID, mailbox model.LabelID) ([]byte, error) {
	b, err := s.q.GetBackfill(ctx, gen.GetBackfillParams{Account: string(account), Mailbox: string(mailbox)})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return b, err
}

func (s *Store) SetBackfill(ctx context.Context, account model.AccountID, mailbox model.LabelID, cur []byte) error {
	return s.q.SetBackfill(ctx, gen.SetBackfillParams{Account: string(account), Mailbox: string(mailbox), Backfill: cur})
}

// GetBackfillState returns the saved paging cursor and whether backfill has
// already completed for this mailbox. Completion is tracked separately from the
// cursor so a finished backfill is not mistaken for "never started" (empty
// cursor) and restarted from newest on the next launch.
func (s *Store) GetBackfillState(ctx context.Context, account model.AccountID, mailbox model.LabelID) (cur []byte, done bool, err error) {
	row, err := s.q.GetBackfillState(ctx, gen.GetBackfillStateParams{Account: string(account), Mailbox: string(mailbox)})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return row.Backfill, row.BackfillDone != 0, nil
}

// MarkBackfillDone records that a mailbox is fully backfilled.
func (s *Store) MarkBackfillDone(ctx context.Context, account model.AccountID, mailbox model.LabelID) error {
	return s.q.MarkBackfillDone(ctx, gen.MarkBackfillDoneParams{Account: string(account), Mailbox: string(mailbox)})
}
