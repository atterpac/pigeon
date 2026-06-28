package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/atterpac/email/internal/model"
	gen "github.com/atterpac/email/internal/store/db"
)

// ErrNotFound is returned when a requested row does not exist.
var ErrNotFound = errors.New("store: not found")

// defaultListLimit caps list/search reads when the caller passes limit <= 0.
const defaultListLimit = 50

// ListAccounts returns all configured accounts.
func (s *Store) ListAccounts(ctx context.Context) ([]model.Account, error) {
	rows, err := s.q.ListAccounts(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]model.Account, len(rows))
	for i, r := range rows {
		out[i] = model.Account{
			ID: model.AccountID(r.ID), Kind: model.Kind(r.Kind), Email: r.Email, Name: r.Name,
			IMAPHost: r.ImapHost, IMAPPort: int(r.ImapPort), SMTPHost: r.SmtpHost, SMTPPort: int(r.SmtpPort),
		}
	}
	return out, nil
}

// Mailboxes returns the mailbox topology for an account.
func (s *Store) Mailboxes(ctx context.Context, account model.AccountID) ([]model.Mailbox, error) {
	rows, err := s.q.ListMailboxes(ctx, string(account))
	if err != nil {
		return nil, err
	}
	out := make([]model.Mailbox, len(rows))
	for i, r := range rows {
		out[i] = model.Mailbox{
			ID: model.LabelID(r.ID), Account: model.AccountID(r.Account), Name: r.Name,
			Path: r.Path, Role: model.Role(r.Role), Unread: int(r.Unread), Total: int(r.Total),
			Icon: r.Icon, IconWeight: r.IconWeight, IconColor: r.IconColor,
		}
	}
	return out, nil
}

// Threads lists conversations for an account, newest activity first.
func (s *Store) Threads(ctx context.Context, account model.AccountID, limit int) ([]model.Thread, error) {
	if limit <= 0 {
		limit = defaultListLimit
	}
	rows, err := s.q.ListThreads(ctx, gen.ListThreadsParams{Account: string(account), Limit: int64(limit)})
	if err != nil {
		return nil, err
	}
	out := make([]model.Thread, len(rows))
	for i, r := range rows {
		out[i] = model.Thread{
			ID: model.ThreadID(r.ID), Account: model.AccountID(r.Account), Subject: r.Subject,
			Last: time.Unix(r.Last, 0), Unread: r.Unread != 0,
		}
	}
	return out, nil
}

// ThreadMessages returns all messages in a thread, oldest first.
func (s *Store) ThreadMessages(ctx context.Context, account model.AccountID, thread model.ThreadID) ([]model.Message, error) {
	rows, err := s.q.ListThreadMessages(ctx, gen.ListThreadMessagesParams{Account: string(account), Thread: string(thread)})
	if err != nil {
		return nil, err
	}
	msgs := mapMessages(rows)
	if err := s.loadLabels(ctx, account, msgs); err != nil {
		return nil, err
	}
	return msgs, nil
}

// MailboxMessages returns messages carrying a label/mailbox, newest first.
func (s *Store) MailboxMessages(ctx context.Context, account model.AccountID, mailbox model.LabelID, limit int) ([]model.Message, error) {
	if limit <= 0 {
		limit = defaultListLimit
	}
	rows, err := s.q.ListMailboxMessages(ctx, gen.ListMailboxMessagesParams{
		Account: string(account), Label: string(mailbox), Limit: int64(limit),
	})
	if err != nil {
		return nil, err
	}
	msgs := mapMessages(rows)
	if err := s.loadLabels(ctx, account, msgs); err != nil {
		return nil, err
	}
	return msgs, nil
}

// Message returns a single message envelope (without body parts).
func (s *Store) Message(ctx context.Context, account model.AccountID, id model.MessageID) (model.Message, error) {
	r, err := s.q.GetMessage(ctx, gen.GetMessageParams{Account: string(account), ID: string(id)})
	if errors.Is(err, sql.ErrNoRows) {
		return model.Message{}, ErrNotFound
	}
	if err != nil {
		return model.Message{}, err
	}
	m := rowToMessage(r)
	one := []model.Message{m}
	if err := s.loadLabels(ctx, account, one); err != nil {
		return model.Message{}, err
	}
	return one[0], nil
}

// Parts returns the stored body parts for a message (empty if not yet fetched).
func (s *Store) Parts(ctx context.Context, account model.AccountID, id model.MessageID) ([]model.Part, error) {
	rows, err := s.q.ListParts(ctx, gen.ListPartsParams{Account: string(account), Message: string(id)})
	if err != nil {
		return nil, err
	}
	out := make([]model.Part, len(rows))
	for i, r := range rows {
		out[i] = model.Part{
			ContentType: r.ContentType, Charset: r.Charset, Disposition: r.Disposition,
			Filename: r.Filename, ContentID: r.ContentID, Size: r.Size, Content: r.Content, BlobRef: r.BlobRef,
		}
	}
	return out, nil
}

// loadLabels populates Labels on each message in place with a single query.
func (s *Store) loadLabels(ctx context.Context, account model.AccountID, msgs []model.Message) error {
	if len(msgs) == 0 {
		return nil
	}
	ids := make([]model.MessageID, len(msgs))
	for i, m := range msgs {
		ids[i] = m.ID
	}
	byID, err := s.labelsByMessage(ctx, account, ids)
	if err != nil {
		return err
	}
	for i := range msgs {
		msgs[i].Labels = byID[msgs[i].ID]
	}
	return nil
}

// labelsByMessage returns the labels attached to each message id, batched into
// one query.
func (s *Store) labelsByMessage(ctx context.Context, account model.AccountID, ids []model.MessageID) (map[model.MessageID][]model.LabelID, error) {
	byID := map[model.MessageID][]model.LabelID{}
	if len(ids) == 0 {
		return byID, nil
	}
	placeholders, args := idArgs(account, ids)
	rows, err := s.db.QueryContext(ctx,
		`SELECT message, label FROM message_labels WHERE account = ? AND message IN (`+placeholders+`)`,
		args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var msg, label string
		if err := rows.Scan(&msg, &label); err != nil {
			return nil, err
		}
		byID[model.MessageID(msg)] = append(byID[model.MessageID(msg)], model.LabelID(label))
	}
	return byID, rows.Err()
}
