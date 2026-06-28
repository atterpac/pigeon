package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"

	"github.com/atterpac/email/internal/classify"
	"github.com/atterpac/email/internal/events"
	"github.com/atterpac/email/internal/model"
	gen "github.com/atterpac/email/internal/store/db"
)

// publish emits a changefeed event (no-op if the bus is unset or ids empty).
func (s *Store) publish(account model.AccountID, kind events.Kind, ids []model.MessageID) {
	s.bus.Publish(events.Event{Account: account, Kind: kind, IDs: ids})
}

func msgIDs(msgs []model.Message) []model.MessageID {
	ids := make([]model.MessageID, len(msgs))
	for i, m := range msgs {
		ids[i] = m.ID
	}
	return ids
}

// SaveMessages upserts a batch of message envelopes with their labels, thread
// rows, and FTS index entries — all in a single transaction. Every message in
// the batch must belong to the same account.
func (s *Store) SaveMessages(ctx context.Context, msgs []model.Message) error {
	if len(msgs) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("save messages: begin: %w", err)
	}
	defer tx.Rollback()
	q := s.q.WithTx(tx)
	touchedThreads := map[model.ThreadID]struct{}{}

	for _, m := range msgs {
		if m.Category == "" {
			m.Category = classify.Message(m)
		}
		if err := q.UpsertMessage(ctx, toUpsertParams(m)); err != nil {
			return err
		}
		for _, lbl := range m.Labels {
			if err := q.AddLabel(ctx, gen.AddLabelParams{
				Account: string(m.Account), Message: string(m.ID), Label: string(lbl),
			}); err != nil {
				return err
			}
		}
		if m.Thread != "" {
			touchedThreads[m.Thread] = struct{}{}
			if err := q.UpsertThread(ctx, gen.UpsertThreadParams{
				ID:      string(m.Thread),
				Account: string(m.Account),
				Subject: m.Subject,
				Last:    m.Date.Unix(),
				Unread:  boolToInt(!slices.Contains(m.Flags, model.FlagSeen)),
			}); err != nil {
				return err
			}
		}
		if err := indexFTS(ctx, tx, m, ""); err != nil {
			return err
		}
		if err := harvestContacts(ctx, q, m.Account, m); err != nil {
			return err
		}
	}
	for thread := range touchedThreads {
		if err := recalcThreadUnread(ctx, tx, msgs[0].Account, thread); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("save messages: commit: %w", err)
	}
	s.publish(msgs[0].Account, events.KindUpsert, msgIDs(msgs))
	return nil
}

// FlagDelta is an incremental change to a single message's flags and labels,
// produced by provider history sync (read/star toggles, label moves).
type FlagDelta struct {
	ID           model.MessageID
	AddFlags     []model.Flag
	RemoveFlags  []model.Flag
	AddLabels    []model.LabelID
	RemoveLabels []model.LabelID
}

// ApplyFlagDeltas applies flag/label changes to existing messages in one
// transaction (read-modify-write on the flags set; label add/remove rows).
// Messages not present locally are skipped — backfill will pick them up.
func (s *Store) ApplyFlagDeltas(ctx context.Context, account model.AccountID, deltas []FlagDelta) error {
	if len(deltas) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("apply flag deltas: begin: %w", err)
	}
	defer tx.Rollback()
	q := s.q.WithTx(tx)
	touchedThreads := map[model.ThreadID]struct{}{}
	for _, d := range deltas {
		msg, err := q.GetMessage(ctx, gen.GetMessageParams{Account: string(account), ID: string(d.ID)})
		if errors.Is(err, sql.ErrNoRows) {
			continue // not synced yet
		}
		if err != nil {
			return err
		}
		touchedThreads[model.ThreadID(msg.Thread)] = struct{}{}
		set := flagSet(splitFlags(msg.Flags))
		for _, f := range d.AddFlags {
			set[f] = struct{}{}
		}
		for _, f := range d.RemoveFlags {
			delete(set, f)
		}
		if err := q.SetFlags(ctx, gen.SetFlagsParams{
			Flags: joinFlags(flagSlice(set)), Account: string(account), ID: string(d.ID),
		}); err != nil {
			return err
		}
		for _, l := range d.AddLabels {
			if err := q.AddLabel(ctx, gen.AddLabelParams{Account: string(account), Message: string(d.ID), Label: string(l)}); err != nil {
				return err
			}
		}
		for _, l := range d.RemoveLabels {
			if err := q.RemoveLabel(ctx, gen.RemoveLabelParams{Account: string(account), Message: string(d.ID), Label: string(l)}); err != nil {
				return err
			}
		}
	}
	for thread := range touchedThreads {
		if err := recalcThreadUnread(ctx, tx, account, thread); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("apply flag deltas: commit: %w", err)
	}
	ids := make([]model.MessageID, len(deltas))
	for i, d := range deltas {
		ids[i] = d.ID
	}
	s.publish(account, events.KindFlag, ids)
	return nil
}

type execer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func recalcThreadUnread(ctx context.Context, db execer, account model.AccountID, thread model.ThreadID) error {
	if thread == "" {
		return nil
	}
	_, err := db.ExecContext(ctx, `
		UPDATE threads
		SET unread = EXISTS (
			SELECT 1 FROM messages
			WHERE account = ? AND thread = ? AND instr(flags, ?) = 0
		)
		WHERE account = ? AND id = ?`,
		string(account), string(thread), string(model.FlagSeen), string(account), string(thread),
	)
	return err
}

// DeleteMessages removes messages and their FTS entries in one transaction.
func (s *Store) DeleteMessages(ctx context.Context, account model.AccountID, ids []model.MessageID) error {
	if len(ids) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("delete messages: begin: %w", err)
	}
	defer tx.Rollback()
	q := s.q.WithTx(tx)
	for _, id := range ids {
		rowid, err := messageRowID(ctx, tx, account, id)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM messages_fts WHERE rowid = ?`, rowid); err != nil {
			return err
		}
		if err := q.DeleteMessage(ctx, gen.DeleteMessageParams{Account: string(account), ID: string(id)}); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("delete messages: commit: %w", err)
	}
	s.publish(account, events.KindDelete, ids)
	return nil
}

// messageRowID looks up a message's SQLite rowid, used to address the external
// FTS table. Returns sql.ErrNoRows if the message is absent.
func messageRowID(ctx context.Context, tx *sql.Tx, account model.AccountID, id model.MessageID) (int64, error) {
	var rowid int64
	err := tx.QueryRowContext(ctx, `SELECT rowid FROM messages WHERE account = ? AND id = ?`,
		string(account), string(id)).Scan(&rowid)
	return rowid, err
}

// indexFTS keeps the own-content FTS table in step with a message row:
// delete-by-rowid then insert. Pass the decoded body text once it is available;
// envelope-only writes pass "".
func indexFTS(ctx context.Context, tx *sql.Tx, m model.Message, body string) error {
	rowid, err := messageRowID(ctx, tx, m.Account, m.ID)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM messages_fts WHERE rowid = ?`, rowid); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx,
		`INSERT INTO messages_fts(rowid, subject, sender, body) VALUES (?, ?, ?, ?)`,
		rowid, m.Subject, sender(m), body)
	return err
}
