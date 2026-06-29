package store

import (
	"context"
	"time"

	"github.com/atterpac/email/internal/model"
	gen "github.com/atterpac/email/internal/store/db"
)

// Snooze records messages as snoozed until `until`. The caller is responsible
// for hiding them from the inbox (e.g. removing the INBOX label).
func (s *Store) Snooze(ctx context.Context, account model.AccountID, ids []model.MessageID, until time.Time) error {
	now := time.Now().Unix()
	return s.Tx(ctx, func(q *gen.Queries) error {
		for _, id := range ids {
			if err := q.UpsertSnooze(ctx, gen.UpsertSnoozeParams{
				Account: string(account), Message: string(id), Until: until.Unix(), Created: now,
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

// Unsnooze clears snooze records for messages.
func (s *Store) Unsnooze(ctx context.Context, account model.AccountID, ids []model.MessageID) error {
	return s.Tx(ctx, func(q *gen.Queries) error {
		for _, id := range ids {
			if err := q.DeleteSnooze(ctx, gen.DeleteSnoozeParams{Account: string(account), Message: string(id)}); err != nil {
				return err
			}
		}
		return nil
	})
}

// DueSnoozes returns message ids whose snooze has elapsed as of now.
func (s *Store) DueSnoozes(ctx context.Context, account model.AccountID, now time.Time) ([]model.MessageID, error) {
	rows, err := s.q.DueSnoozes(ctx, gen.DueSnoozesParams{Account: string(account), Until: now.Unix()})
	if err != nil {
		return nil, err
	}
	out := make([]model.MessageID, len(rows))
	for i, r := range rows {
		out[i] = model.MessageID(r)
	}
	return out, nil
}

// ListSnoozes returns all current snoozes for an account.
func (s *Store) ListSnoozes(ctx context.Context, account model.AccountID) ([]model.Snoozed, error) {
	rows, err := s.q.ListSnoozes(ctx, string(account))
	if err != nil {
		return nil, err
	}
	out := make([]model.Snoozed, len(rows))
	for i, r := range rows {
		out[i] = model.Snoozed{MessageID: model.MessageID(r.Message), Until: time.Unix(r.Until, 0)}
	}
	return out, nil
}

// RecordDone logs messages as completed (archived) at the given time.
func (s *Store) RecordDone(ctx context.Context, account model.AccountID, ids []model.MessageID, at time.Time) error {
	return s.Tx(ctx, func(q *gen.Queries) error {
		for _, id := range ids {
			if err := q.RecordDone(ctx, gen.RecordDoneParams{Account: string(account), Message: string(id), At: at.Unix()}); err != nil {
				return err
			}
		}
		return nil
	})
}

// DoneSince counts messages marked done at or after `since`.
func (s *Store) DoneSince(ctx context.Context, account model.AccountID, since time.Time) (int, error) {
	n, err := s.q.CountDoneSince(ctx, gen.CountDoneSinceParams{Account: string(account), At: since.Unix()})
	return int(n), err
}
