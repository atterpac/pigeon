package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

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
	cachedAt := time.Now().Unix()
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
			Filename: p.Filename, ContentID: p.ContentID, Size: p.Size, Content: p.Content, BlobRef: p.BlobRef, CachedAt: cachedAt,
		}); err != nil {
			return err
		}
	}
	if err := q.SetBodyLoaded(ctx, gen.SetBodyLoadedParams{BodyLoaded: 1, BodyCachedAt: cachedAt, Account: string(account), ID: string(id)}); err != nil {
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

// BodyRetentionPolicy describes which cached bodies can be evicted while
// keeping envelopes, flags, labels, and snippets.
type BodyRetentionPolicy struct {
	MaxAge             time.Duration
	MaxBytes           int64
	KeepUnread         bool
	KeepStarred        bool
	KeepRecentlyOpened time.Duration
	Limit              int
}

type BodyPruneResult struct {
	Messages int
	Bytes    int64
}

type bodyPruneCandidate struct {
	id       model.MessageID
	subject  string
	fromJSON string
	bytes    int64
}

// TouchMessagesOpened marks messages as foreground-opened. Retention uses this
// separately from message date so old-but-active threads can keep cached bodies.
func (s *Store) TouchMessagesOpened(ctx context.Context, account model.AccountID, ids []model.MessageID, at time.Time) error {
	if len(ids) == 0 {
		return nil
	}
	if at.IsZero() {
		at = time.Now()
	}
	placeholders, args := messageIDArgs(account, ids)
	args = append([]any{at.Unix()}, args...)
	_, err := s.db.ExecContext(ctx,
		`UPDATE messages SET last_opened_at = MAX(last_opened_at, ?) WHERE account = ? AND id IN (`+placeholders+`)`,
		args...,
	)
	return err
}

// PruneBodies evicts cached body parts according to policy while preserving the
// message envelope. Pruned messages can be rehydrated by foreground reads.
func (s *Store) PruneBodies(ctx context.Context, account model.AccountID, policy BodyRetentionPolicy, now time.Time) (BodyPruneResult, error) {
	if policy.MaxAge <= 0 && policy.MaxBytes <= 0 {
		return BodyPruneResult{}, nil
	}
	if now.IsZero() {
		now = time.Now()
	}
	limit := policy.Limit
	if limit <= 0 {
		limit = 500
	}

	selected := make([]bodyPruneCandidate, 0, limit)
	seen := map[model.MessageID]struct{}{}
	var ageFreed int64
	if policy.MaxAge > 0 {
		cutoff := now.Add(-policy.MaxAge).Unix()
		candidates, err := s.bodyPruneCandidates(ctx, account, policy, now, &cutoff, limit)
		if err != nil {
			return BodyPruneResult{}, err
		}
		for _, c := range candidates {
			selected = append(selected, c)
			seen[c.id] = struct{}{}
			ageFreed += c.bytes
		}
	}

	if policy.MaxBytes > 0 && len(selected) < limit {
		total, err := s.cachedBodyBytes(ctx, account)
		if err != nil {
			return BodyPruneResult{}, err
		}
		need := total - policy.MaxBytes - ageFreed
		if need > 0 {
			candidates, err := s.bodyPruneCandidates(ctx, account, policy, now, nil, limit-len(selected))
			if err != nil {
				return BodyPruneResult{}, err
			}
			var extra int64
			for _, c := range candidates {
				if _, ok := seen[c.id]; ok {
					continue
				}
				selected = append(selected, c)
				seen[c.id] = struct{}{}
				extra += c.bytes
				if extra >= need || len(selected) >= limit {
					break
				}
			}
		}
	}
	if len(selected) == 0 {
		return BodyPruneResult{}, nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return BodyPruneResult{}, err
	}
	defer tx.Rollback()
	q := s.q.WithTx(tx)
	var result BodyPruneResult
	ids := make([]model.MessageID, 0, len(selected))
	for _, c := range selected {
		if err := q.DeleteParts(ctx, gen.DeletePartsParams{Account: string(account), Message: string(c.id)}); err != nil {
			return BodyPruneResult{}, err
		}
		if err := q.SetBodyLoaded(ctx, gen.SetBodyLoadedParams{BodyLoaded: 0, BodyCachedAt: 0, Account: string(account), ID: string(c.id)}); err != nil {
			return BodyPruneResult{}, err
		}
		msg := model.Message{ID: c.id, Account: account, Subject: c.subject, From: decodeAddrs(c.fromJSON)}
		if err := indexFTS(ctx, tx, msg); err != nil {
			return BodyPruneResult{}, err
		}
		result.Messages++
		result.Bytes += c.bytes
		ids = append(ids, c.id)
	}
	if err := tx.Commit(); err != nil {
		return BodyPruneResult{}, err
	}
	s.publish(account, events.KindUpsert, ids)
	return result, nil
}

func (s *Store) bodyPruneCandidates(ctx context.Context, account model.AccountID, policy BodyRetentionPolicy, now time.Time, olderThan *int64, limit int) ([]bodyPruneCandidate, error) {
	wheres := []string{"m.account = ?", "m.body_loaded = 1"}
	args := []any{string(account)}
	if olderThan != nil {
		wheres = append(wheres, "m.body_cached_at > 0", "m.body_cached_at < ?")
		args = append(args, *olderThan)
	}
	if policy.KeepUnread {
		wheres = append(wheres, "instr(m.flags, ?) > 0")
		args = append(args, string(model.FlagSeen))
	}
	if policy.KeepStarred {
		wheres = append(wheres, "instr(m.flags, ?) = 0")
		args = append(args, string(model.FlagFlagged))
	}
	if policy.KeepRecentlyOpened > 0 {
		wheres = append(wheres, "(m.last_opened_at = 0 OR m.last_opened_at < ?)")
		args = append(args, now.Add(-policy.KeepRecentlyOpened).Unix())
	}
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, `
		SELECT m.id, m.subject, m.from_json,
		       COALESCE(SUM(CASE WHEN p.content IS NOT NULL THEN length(p.content) ELSE p.size END), 0) AS body_bytes
		FROM messages m
		LEFT JOIN parts p ON p.account = m.account AND p.message = m.id
		WHERE `+strings.Join(wheres, " AND ")+`
		GROUP BY m.account, m.id
		ORDER BY m.body_cached_at ASC, m.date ASC
		LIMIT ?`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []bodyPruneCandidate
	for rows.Next() {
		var c bodyPruneCandidate
		var id string
		if err := rows.Scan(&id, &c.subject, &c.fromJSON, &c.bytes); err != nil {
			return nil, err
		}
		c.id = model.MessageID(id)
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) cachedBodyBytes(ctx context.Context, account model.AccountID) (int64, error) {
	var bytes int64
	err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(CASE WHEN p.content IS NOT NULL THEN length(p.content) ELSE p.size END), 0)
		FROM parts p
		JOIN messages m ON m.account = p.account AND m.id = p.message
		WHERE m.account = ? AND m.body_loaded = 1`,
		string(account),
	).Scan(&bytes)
	return bytes, err
}

func messageIDArgs(account model.AccountID, ids []model.MessageID) (string, []any) {
	placeholders := make([]string, len(ids))
	args := make([]any, 0, len(ids)+1)
	args = append(args, string(account))
	for i, id := range ids {
		placeholders[i] = "?"
		args = append(args, string(id))
	}
	return strings.Join(placeholders, ","), args
}
