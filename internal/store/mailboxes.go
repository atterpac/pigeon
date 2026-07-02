package store

import (
	"context"

	"github.com/atterpac/pigeon/internal/model"
	gen "github.com/atterpac/pigeon/internal/store/db"
)

// UpsertMailboxes stores the mailbox topology for an account.
func (s *Store) UpsertMailboxes(ctx context.Context, mbs []model.Mailbox) error {
	return s.Tx(ctx, func(q *gen.Queries) error {
		for _, mb := range mbs {
			if err := q.UpsertMailbox(ctx, gen.UpsertMailboxParams{
				ID:      string(mb.ID),
				Account: string(mb.Account),
				Name:    mb.Name,
				Path:    mb.Path,
				Role:    int64(mb.Role),
				Unread:  int64(mb.Unread),
				Total:   int64(mb.Total),
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

// DeleteMailbox removes a single mailbox/folder row for an account.
func (s *Store) DeleteMailbox(ctx context.Context, account model.AccountID, id model.LabelID) error {
	return s.q.DeleteMailbox(ctx, gen.DeleteMailboxParams{Account: string(account), ID: string(id)})
}

// SetMailboxIcon stores the user's chosen folder icon (presentation metadata).
func (s *Store) SetMailboxIcon(ctx context.Context, account model.AccountID, id model.LabelID, icon, weight, color string) error {
	return s.q.SetMailboxIcon(ctx, gen.SetMailboxIconParams{
		Icon:       icon,
		IconWeight: weight,
		IconColor:  color,
		Account:    string(account),
		ID:         string(id),
	})
}
