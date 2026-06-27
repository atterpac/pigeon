package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"strings"

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

// SaveMessages upserts a batch of message envelopes with their labels, thread
// rows, and FTS index entries — all in a single transaction.
func (s *Store) SaveMessages(ctx context.Context, msgs []model.Message) error {
	if len(msgs) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
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
		if err := indexFTS(ctx, tx, m); err != nil {
			return err
		}
	}
	for thread := range touchedThreads {
		if err := recalcThreadUnread(ctx, tx, msgs[0].Account, thread); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
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
		return err
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
	err = tx.Commit()
	if err != nil {
		return err
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
		return err
	}
	defer tx.Rollback()
	q := s.q.WithTx(tx)
	for _, id := range ids {
		var rowid int64
		err := tx.QueryRowContext(ctx, `SELECT rowid FROM messages WHERE account = ? AND id = ?`,
			string(account), string(id)).Scan(&rowid)
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
		return err
	}
	s.publish(account, events.KindDelete, ids)
	return nil
}

func flagSet(fs []model.Flag) map[model.Flag]struct{} {
	m := make(map[model.Flag]struct{}, len(fs))
	for _, f := range fs {
		m[f] = struct{}{}
	}
	return m
}

func flagSlice(m map[model.Flag]struct{}) []model.Flag {
	out := make([]model.Flag, 0, len(m))
	for f := range m {
		out = append(out, f)
	}
	slices.Sort(out)
	return out
}

// indexFTS keeps the own-content FTS table in step: delete-by-rowid then insert.
func indexFTS(ctx context.Context, tx *sql.Tx, m model.Message) error {
	var rowid int64
	err := tx.QueryRowContext(ctx,
		`SELECT rowid FROM messages WHERE account = ? AND id = ?`,
		string(m.Account), string(m.ID)).Scan(&rowid)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM messages_fts WHERE rowid = ?`, rowid); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx,
		`INSERT INTO messages_fts(rowid, subject, sender, body) VALUES (?, ?, ?, ?)`,
		rowid, m.Subject, sender(m), "" /* body filled when bodies are synced */)
	return err
}

// --- sync cursors (stored as opaque bytes; the engine owns their meaning) ---

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

// --- conversions ---

func toUpsertParams(m model.Message) gen.UpsertMessageParams {
	return gen.UpsertMessageParams{
		ID:             string(m.ID),
		Account:        string(m.Account),
		Thread:         string(m.Thread),
		Subject:        m.Subject,
		FromJson:       mustJSON(m.From),
		ToJson:         mustJSON(m.To),
		CcJson:         mustJSON(m.Cc),
		BccJson:        mustJSON(m.Bcc),
		Date:           m.Date.Unix(),
		Snippet:        m.Snippet,
		Category:       string(m.Category),
		Flags:          joinFlags(m.Flags),
		HasAttachments: boolToInt(m.HasAttachments),
		BodyLoaded:     boolToInt(m.BodyLoaded),
		RfcMessageID:   m.RFCMessageID,
		Refs:           strings.Join(m.References, " "),
	}
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func joinFlags(fs []model.Flag) string {
	parts := make([]string, len(fs))
	for i, f := range fs {
		parts[i] = string(f)
	}
	return strings.Join(parts, " ")
}

func splitFlags(s string) []model.Flag {
	if s == "" {
		return nil
	}
	parts := strings.Fields(s)
	out := make([]model.Flag, len(parts))
	for i, p := range parts {
		out[i] = model.Flag(p)
	}
	return out
}

func sender(m model.Message) string {
	if len(m.From) == 0 {
		return ""
	}
	return m.From[0].Name + " " + m.From[0].Addr
}

func boolToInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
