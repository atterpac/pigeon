package main

import (
	"context"
	"time"

	"github.com/atterpac/email/internal/email"
)

// The Wails binding surface is split into small, purpose-built facade services
// rather than exposing the reusable *email.Client SDK wholesale. Each service
// is a thin wrapper that delegates to the client; grouping methods by the
// frontend's mental model (mailboxes, messages, mutations, compose, snooze)
// keeps the IPC surface intentional and lets the SDK evolve independently.
//
// Account-scoping note: methods keep the client's signatures (passing an
// account or account id) so the generated bindings line up 1:1 with the SDK.

// Mailboxes exposes account listing and folder/label topology + CRUD.
type Mailboxes struct{ client *email.Client }

func (m *Mailboxes) Accounts(ctx context.Context) ([]email.Account, error) {
	return m.client.Accounts(ctx)
}

func (m *Mailboxes) Mailboxes(ctx context.Context, acct email.AccountID) ([]email.Mailbox, error) {
	return m.client.Mailboxes(ctx, acct)
}

func (m *Mailboxes) CreateMailbox(ctx context.Context, acct email.Account, name string) (email.Mailbox, error) {
	return m.client.CreateMailbox(ctx, acct, name)
}

func (m *Mailboxes) RenameMailbox(ctx context.Context, acct email.Account, id email.LabelID, newName string) (email.Mailbox, error) {
	return m.client.RenameMailbox(ctx, acct, id, newName)
}

func (m *Mailboxes) DeleteMailbox(ctx context.Context, acct email.Account, id email.LabelID) error {
	return m.client.DeleteMailbox(ctx, acct, id)
}

func (m *Mailboxes) SetMailboxIcon(ctx context.Context, acct email.AccountID, id email.LabelID, icon, weight, color string) (email.Mailbox, error) {
	return m.client.SetMailboxIcon(ctx, acct, id, icon, weight, color)
}

// Messages exposes read paths plus on-demand and opportunistic fetching.
type Messages struct{ client *email.Client }

func (m *Messages) Threads(ctx context.Context, acct email.AccountID, limit int) ([]email.Thread, error) {
	return m.client.Threads(ctx, acct, limit)
}

func (m *Messages) ConversationList(ctx context.Context, acct email.AccountID, limit int) ([]email.ThreadListItem, error) {
	return m.client.ConversationList(ctx, acct, limit)
}

func (m *Messages) ThreadMessages(ctx context.Context, acct email.AccountID, thread email.ThreadID) ([]email.Message, error) {
	return m.client.ThreadMessages(ctx, acct, thread)
}

func (m *Messages) MailboxMessages(ctx context.Context, acct email.AccountID, mailbox email.LabelID, limit int) ([]email.Message, error) {
	return m.client.MailboxMessages(ctx, acct, mailbox, limit)
}

func (m *Messages) Message(ctx context.Context, acct email.AccountID, id email.MessageID) (email.Message, error) {
	return m.client.Message(ctx, acct, id)
}

func (m *Messages) Search(ctx context.Context, acct email.AccountID, query string, limit int) ([]email.Message, error) {
	return m.client.Search(ctx, acct, query, limit)
}

func (m *Messages) MessageBody(ctx context.Context, acct email.Account, id email.MessageID) ([]email.Part, error) {
	return m.client.MessageBody(ctx, acct, id)
}

func (m *Messages) Attachments(ctx context.Context, acct email.Account, id email.MessageID) ([]email.Part, error) {
	return m.client.Attachments(ctx, acct, id)
}

func (m *Messages) PreloadMailboxBodies(ctx context.Context, acct email.Account, mailbox email.LabelID, limit int) (int, error) {
	return m.client.PreloadMailboxBodies(ctx, acct, mailbox, limit)
}

func (m *Messages) ReclassifyMailbox(ctx context.Context, acct email.AccountID, mailbox email.LabelID, limit int) (int, error) {
	return m.client.ReclassifyMailbox(ctx, acct, mailbox, limit)
}

// SyncOnce populates a never-before-opened folder on demand (forward + backfill).
func (m *Messages) SyncOnce(ctx context.Context, acct email.Account, mailbox email.LabelID) (int, error) {
	return m.client.SyncOnce(ctx, acct, mailbox)
}

// Mutations exposes optimistic message state changes (flags, labels, moves).
type Mutations struct{ client *email.Client }

func (m *Mutations) MarkRead(ctx context.Context, acct email.Account, ids []email.MessageID, read bool) error {
	return m.client.MarkRead(ctx, acct, ids, read)
}

func (m *Mutations) Star(ctx context.Context, acct email.Account, ids []email.MessageID, on bool) error {
	return m.client.Star(ctx, acct, ids, on)
}

func (m *Mutations) Archive(ctx context.Context, acct email.Account, ids []email.MessageID) error {
	return m.client.Archive(ctx, acct, ids)
}

func (m *Mutations) ApplyLabels(ctx context.Context, acct email.Account, ids []email.MessageID, add, remove []email.LabelID) error {
	return m.client.ApplyLabels(ctx, acct, ids, add, remove)
}

func (m *Mutations) Move(ctx context.Context, acct email.Account, ids []email.MessageID, dst email.LabelID) error {
	return m.client.Move(ctx, acct, ids, dst)
}

func (m *Mutations) Delete(ctx context.Context, acct email.Account, ids []email.MessageID) error {
	return m.client.Delete(ctx, acct, ids)
}

// Compose exposes draft persistence and message delivery.
type Compose struct{ client *email.Client }

func (c *Compose) SaveDraft(ctx context.Context, acct email.AccountID, id string, out email.Outgoing) (string, error) {
	return c.client.SaveDraft(ctx, acct, id, out)
}

func (c *Compose) Drafts(ctx context.Context, acct email.AccountID) ([]email.Draft, error) {
	return c.client.Drafts(ctx, acct)
}

func (c *Compose) Draft(ctx context.Context, acct email.AccountID, id string) (email.Draft, error) {
	return c.client.Draft(ctx, acct, id)
}

func (c *Compose) DiscardDraft(ctx context.Context, acct email.AccountID, id string) error {
	return c.client.DiscardDraft(ctx, acct, id)
}

func (c *Compose) Send(ctx context.Context, acct email.Account, out email.Outgoing) (bool, error) {
	return c.client.Send(ctx, acct, out)
}

func (c *Compose) SendDraft(ctx context.Context, acct email.Account, id string) (bool, error) {
	return c.client.SendDraft(ctx, acct, id)
}

// Snooze exposes snooze scheduling and the Done-today metric.
type Snooze struct{ client *email.Client }

func (s *Snooze) Snooze(ctx context.Context, acct email.Account, ids []email.MessageID, until time.Time) error {
	return s.client.Snooze(ctx, acct, ids, until)
}

func (s *Snooze) Unsnooze(ctx context.Context, acct email.Account, ids []email.MessageID) error {
	return s.client.Unsnooze(ctx, acct, ids)
}

func (s *Snooze) Snoozed(ctx context.Context, acct email.AccountID) ([]email.Snoozed, error) {
	return s.client.Snoozed(ctx, acct)
}

func (s *Snooze) DoneToday(ctx context.Context, acct email.AccountID) (int, error) {
	return s.client.DoneToday(ctx, acct)
}

// newServices builds the facade service set sharing one client.
func newServices(c *email.Client) (*Mailboxes, *Messages, *Mutations, *Compose, *Snooze) {
	return &Mailboxes{c}, &Messages{c}, &Mutations{c}, &Compose{c}, &Snooze{c}
}
