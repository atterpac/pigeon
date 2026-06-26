// Package provider defines the abstraction over concrete mail backends
// (Gmail REST in provider/gmail, IMAP+SMTP in provider/imap). The sync engine
// is the only consumer; it never imports a concrete implementation directly but
// receives one via a Factory.
package provider

import (
	"context"

	"github.com/atterpac/email/internal/model"
)

// Cursor is an opaque, provider-specific sync position. For Gmail it wraps a
// historyId; for IMAP it wraps UIDVALIDITY + the highest seen MODSEQ. Callers
// persist the Bytes verbatim and hand them back on the next Sync.
type Cursor struct {
	Bytes []byte
}

// MailboxRef identifies a mailbox to a provider in its own terms.
type MailboxRef struct {
	ID   model.LabelID
	Path string
}

// Changes is the delta returned by Sync. The engine applies it transactionally.
type Changes struct {
	Upserted []model.Message // new or changed envelopes
	Flagged  []FlagChange    // flag/label-only changes
	Removed  []model.MessageID
}

// FlagChange records flag/label mutations without a full envelope reload.
type FlagChange struct {
	ID           model.MessageID
	AddFlags     []model.Flag
	RemoveFlags  []model.Flag
	AddLabels    []model.LabelID
	RemoveLabels []model.LabelID
}

// SendOpts carries delivery options (e.g. thread to attach to).
type SendOpts struct {
	Thread model.ThreadID
}

// Caps reports server features so the engine can pick the optimal strategy.
type Caps struct {
	Idle         bool // IMAP IDLE / push available
	QResync      bool // IMAP QRESYNC for efficient resync
	History      bool // Gmail history API
	ServerSearch bool
}

// Provider is one configured backend connection.
type Provider interface {
	ListMailboxes(ctx context.Context) ([]model.Mailbox, error)

	// Sync fetches mail newer than cur (incremental, forward). cur==nil returns
	// the newest position without backfilling history — use Backfill for that.
	Sync(ctx context.Context, mb MailboxRef, cur *Cursor) (Changes, *Cursor, error)

	// Backfill pages envelopes newest-first. page==nil starts at the newest
	// message; pass the returned cursor to fetch the next (older) page. done is
	// true once no older messages remain.
	Backfill(ctx context.Context, mb MailboxRef, page *Cursor, limit int) (changes Changes, next *Cursor, done bool, err error)

	FetchBodies(ctx context.Context, ids []model.MessageID) ([]model.RawMessage, error)

	ApplyFlags(ctx context.Context, ids []model.MessageID, add, remove []model.Flag) error
	ApplyLabels(ctx context.Context, ids []model.MessageID, add, remove []model.LabelID) error
	Move(ctx context.Context, ids []model.MessageID, dst MailboxRef) error
	Delete(ctx context.Context, ids []model.MessageID) error

	// CreateMailbox creates a new mailbox/folder and returns its normalized form.
	CreateMailbox(ctx context.Context, name string) (model.Mailbox, error)
	// RenameMailbox renames an existing mailbox and returns its new form.
	RenameMailbox(ctx context.Context, mb MailboxRef, newName string) (model.Mailbox, error)
	// DeleteMailbox removes a mailbox.
	DeleteMailbox(ctx context.Context, mb MailboxRef) error

	Send(ctx context.Context, raw model.RawMessage, opts SendOpts) (model.MessageID, error)
	SaveDraft(ctx context.Context, raw model.RawMessage) (model.MessageID, error)

	// Watch emits a hint whenever a mailbox may have changed (IDLE / Gmail watch).
	Watch(ctx context.Context) (<-chan MailboxRef, error)

	Capabilities() Caps
	Close() error
}

// Factory builds a Provider for an account, resolving credentials via auth.
type Factory func(ctx context.Context, acct model.Account) (Provider, error)
