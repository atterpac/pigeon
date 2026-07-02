package store

import (
	"context"

	"github.com/atterpac/pigeon/internal/model"
	gen "github.com/atterpac/pigeon/internal/store/db"
)

// UpsertAccount stores or updates an account row.
func (s *Store) UpsertAccount(ctx context.Context, a model.Account) error {
	return s.q.UpsertAccount(ctx, gen.UpsertAccountParams{
		ID:       string(a.ID),
		Kind:     int64(a.Kind),
		Email:    a.Email,
		Name:     a.Name,
		ImapHost: a.IMAPHost,
		ImapPort: int64(a.IMAPPort),
		SmtpHost: a.SMTPHost,
		SmtpPort: int64(a.SMTPPort),
	})
}

// DeleteAccount removes an account and its associated rows (mailboxes,
// messages, etc. cascade via foreign keys).
func (s *Store) DeleteAccount(ctx context.Context, account model.AccountID) error {
	return s.q.DeleteAccount(ctx, string(account))
}
