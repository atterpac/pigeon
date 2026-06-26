package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/atterpac/email/internal/events"
	"github.com/atterpac/email/internal/model"
	gen "github.com/atterpac/email/internal/store/db"
)

// snippetLen caps the stored preview text.
const snippetLen = 240

// SaveBody persists a message's decoded parts (inline + attachments), marks it
// body-loaded, updates its snippet, and re-indexes FTS with the body text so it
// becomes searchable. Replaces any previously stored parts.
func (s *Store) SaveBody(ctx context.Context, account model.AccountID, id model.MessageID, parts []model.Part, text string, category model.Category) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := s.q.WithTx(tx)

	// Need subject + sender for the FTS row.
	row, err := q.GetMessage(ctx, gen.GetMessageParams{Account: string(account), ID: string(id)})
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	msg := rowToMessage(row)

	if err := q.DeleteParts(ctx, gen.DeletePartsParams{Account: string(account), Message: string(id)}); err != nil {
		return err
	}
	for i, p := range parts {
		if err := q.InsertPart(ctx, gen.InsertPartParams{
			Account: string(account), Message: string(id), Seq: int64(i),
			ContentType: p.ContentType, Charset: p.Charset, Disposition: p.Disposition,
			Filename: p.Filename, Size: p.Size, Content: p.Content, BlobRef: p.BlobRef,
		}); err != nil {
			return err
		}
	}
	if err := q.SetBodyLoaded(ctx, gen.SetBodyLoadedParams{BodyLoaded: 1, Account: string(account), ID: string(id)}); err != nil {
		return err
	}

	snippet := text
	if len(snippet) > snippetLen {
		snippet = snippet[:snippetLen]
	}
	if category == "" {
		category = msg.Category
	}
	if _, err := tx.ExecContext(ctx, `UPDATE messages SET snippet = ?, category = ? WHERE account = ? AND id = ?`,
		snippet, string(category), string(account), string(id)); err != nil {
		return err
	}

	// Re-index FTS with the body now available.
	var rowid int64
	if err := tx.QueryRowContext(ctx, `SELECT rowid FROM messages WHERE account = ? AND id = ?`,
		string(account), string(id)).Scan(&rowid); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM messages_fts WHERE rowid = ?`, rowid); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO messages_fts(rowid, subject, sender, body) VALUES (?, ?, ?, ?)`,
		rowid, msg.Subject, sender(msg), text); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	s.publish(account, events.KindUpsert, []model.MessageID{id})
	return nil
}

// IsBodyLoaded reports whether a message's body parts are cached locally.
func (s *Store) IsBodyLoaded(ctx context.Context, account model.AccountID, id model.MessageID) (bool, error) {
	m, err := s.Message(ctx, account, id)
	if err != nil {
		return false, err
	}
	return m.BodyLoaded, nil
}
