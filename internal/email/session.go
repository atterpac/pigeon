package email

import (
	"context"
	"time"
)

// Session binds a Client to a single Account so callers don't repeat the
// account on every call. It is a thin convenience wrapper; the underlying
// Client is shared and safe for concurrent use across sessions.
type Session struct {
	c    *Client
	acct Account
}

// Session returns a per-account handle. It does not register the account; call
// Register (or Client.AddAccount) once before syncing.
func (c *Client) Session(acct Account) *Session { return &Session{c: c, acct: acct} }

// Account returns the bound account.
func (s *Session) Account() Account { return s.acct }

// Register registers the account and returns its mailboxes.
func (s *Session) Register(ctx context.Context) ([]Mailbox, error) {
	return s.c.AddAccount(ctx, s.acct)
}

// ── reads ───────────────────────────────────────────────────────

func (s *Session) Mailboxes(ctx context.Context) ([]Mailbox, error) {
	return s.c.Mailboxes(ctx, s.acct.ID)
}

func (s *Session) Threads(ctx context.Context, limit int) ([]Thread, error) {
	return s.c.Threads(ctx, s.acct.ID, limit)
}

func (s *Session) ConversationList(ctx context.Context, limit int) ([]ThreadListItem, error) {
	return s.c.ConversationList(ctx, s.acct.ID, limit)
}

func (s *Session) ThreadMessages(ctx context.Context, thread ThreadID) ([]Message, error) {
	return s.c.ThreadMessages(ctx, s.acct.ID, thread)
}

func (s *Session) ThreadMessagesWithBodies(ctx context.Context, thread ThreadID) ([]Message, error) {
	return s.c.ThreadMessagesWithBodies(ctx, s.acct, thread)
}

func (s *Session) MailboxMessages(ctx context.Context, mailbox LabelID, limit int) ([]Message, error) {
	return s.c.MailboxMessages(ctx, s.acct.ID, mailbox, limit)
}

func (s *Session) Message(ctx context.Context, id MessageID) (Message, error) {
	return s.c.Message(ctx, s.acct.ID, id)
}

func (s *Session) Search(ctx context.Context, query string, limit int) ([]Message, error) {
	return s.c.Search(ctx, s.acct.ID, query, limit)
}

// ── bodies & attachments ────────────────────────────────────────

func (s *Session) MessageBody(ctx context.Context, id MessageID) ([]Part, error) {
	return s.c.MessageBody(ctx, s.acct, id)
}

func (s *Session) PreloadMailboxBodies(ctx context.Context, mailbox LabelID, limit int) (int, error) {
	return s.c.PreloadMailboxBodies(ctx, s.acct, mailbox, limit)
}

func (s *Session) PruneBodies(ctx context.Context, policy BodyRetentionPolicy) (BodyPruneResult, error) {
	return s.c.PruneBodies(ctx, s.acct.ID, policy)
}

func (s *Session) ReclassifyMailbox(ctx context.Context, mailbox LabelID, limit int) (int, error) {
	return s.c.ReclassifyMailbox(ctx, s.acct.ID, mailbox, limit)
}

func (s *Session) Attachments(ctx context.Context, id MessageID) ([]Part, error) {
	return s.c.Attachments(ctx, s.acct, id)
}

// ── sending ─────────────────────────────────────────────────────

func (s *Session) Send(ctx context.Context, out Outgoing) (bool, error) {
	return s.c.Send(ctx, s.acct, out)
}

// ── mutations ───────────────────────────────────────────────────

func (s *Session) MarkRead(ctx context.Context, ids []MessageID, read bool) error {
	return s.c.MarkRead(ctx, s.acct, ids, read)
}

func (s *Session) Star(ctx context.Context, ids []MessageID, on bool) error {
	return s.c.Star(ctx, s.acct, ids, on)
}

func (s *Session) Archive(ctx context.Context, ids []MessageID) error {
	return s.c.Archive(ctx, s.acct, ids)
}

func (s *Session) ApplyLabels(ctx context.Context, ids []MessageID, add, remove []LabelID) error {
	return s.c.ApplyLabels(ctx, s.acct, ids, add, remove)
}

func (s *Session) Move(ctx context.Context, ids []MessageID, dst LabelID) error {
	return s.c.Move(ctx, s.acct, ids, dst)
}

func (s *Session) Delete(ctx context.Context, ids []MessageID) error {
	return s.c.Delete(ctx, s.acct, ids)
}

// ── drafts ──────────────────────────────────────────────────────

func (s *Session) SaveDraft(ctx context.Context, id string, out Outgoing) (string, error) {
	return s.c.SaveDraft(ctx, s.acct.ID, id, out)
}

func (s *Session) Drafts(ctx context.Context) ([]Draft, error) {
	return s.c.Drafts(ctx, s.acct.ID)
}

func (s *Session) Draft(ctx context.Context, id string) (Draft, error) {
	return s.c.Draft(ctx, s.acct.ID, id)
}

func (s *Session) DiscardDraft(ctx context.Context, id string) error {
	return s.c.DiscardDraft(ctx, s.acct.ID, id)
}

func (s *Session) SendDraft(ctx context.Context, id string) (bool, error) {
	return s.c.SendDraft(ctx, s.acct, id)
}

// ── snooze ──────────────────────────────────────────────────────

func (s *Session) Snooze(ctx context.Context, ids []MessageID, until time.Time) error {
	return s.c.Snooze(ctx, s.acct, ids, until)
}

func (s *Session) Unsnooze(ctx context.Context, ids []MessageID) error {
	return s.c.Unsnooze(ctx, s.acct, ids)
}

func (s *Session) Snoozed(ctx context.Context) ([]Snoozed, error) {
	return s.c.Snoozed(ctx, s.acct.ID)
}

func (s *Session) DoneToday(ctx context.Context) (int, error) {
	return s.c.DoneToday(ctx, s.acct.ID)
}

// ── sync ────────────────────────────────────────────────────────

func (s *Session) StartSync(ctx context.Context, mailboxes []LabelID, opts SyncOptions) error {
	return s.c.StartSync(ctx, s.acct, mailboxes, opts)
}

func (s *Session) SyncOnce(ctx context.Context, mailbox LabelID) (int, error) {
	return s.c.SyncOnce(ctx, s.acct, mailbox)
}

func (s *Session) StopSync() { s.c.StopSync(s.acct.ID) }

// Events returns a changefeed filtered to this account, plus a cancel func.
func (s *Session) Events() (<-chan Event, func()) {
	src, cancel := s.c.Events()
	out := make(chan Event, 64)
	go func() {
		defer close(out)
		for e := range src {
			if e.Account != s.acct.ID {
				continue
			}
			select {
			case out <- e:
			default: // drop if the consumer is slow (hints, refetch anyway)
			}
		}
	}()
	return out, cancel
}
