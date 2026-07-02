// Package email owns the application email client: account management, local
// store reads, send, and background sync. All reads hit SQLite; the network is
// never on the read path.
package email

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/atterpac/pigeon/internal/blob"
	"github.com/atterpac/pigeon/internal/events"
	"github.com/atterpac/pigeon/internal/mime"
	"github.com/atterpac/pigeon/internal/model"
	"github.com/atterpac/pigeon/internal/provider"
	"github.com/atterpac/pigeon/internal/store"
	synceng "github.com/atterpac/pigeon/internal/sync"
)

// Re-exported domain types so callers depend only on this package.
type (
	Account             = model.Account
	AccountID           = model.AccountID
	Mailbox             = model.Mailbox
	Message             = model.Message
	Thread              = model.Thread
	ThreadID            = model.ThreadID
	MessageID           = model.MessageID
	Address             = model.Address
	Outgoing            = model.Outgoing
	Outfile             = model.Outfile
	Part                = model.Part
	ThreadListItem      = model.ThreadListItem
	Draft               = model.Draft
	Snoozed             = model.Snoozed
	Contact             = model.Contact
	BodyRetentionPolicy = store.BodyRetentionPolicy
	BodyPruneResult     = store.BodyPruneResult
	Flag                = model.Flag
	LabelID             = model.LabelID
	Role                = model.Role

	// Event is a changefeed notification; subscribe via Client.Events.
	Event = events.Event
)

// Kind constants for accounts.
const (
	KindIMAP = model.KindIMAP
)

// InboxLabel is the label/mailbox treated as "the inbox" for Archive.
const InboxLabel LabelID = "INBOX"

// Role constants for normalized mailboxes.
const (
	RoleNone    = model.RoleNone
	RoleInbox   = model.RoleInbox
	RoleSent    = model.RoleSent
	RoleDrafts  = model.RoleDrafts
	RoleTrash   = model.RoleTrash
	RoleSpam    = model.RoleSpam
	RoleArchive = model.RoleArchive
)

// SyncOptions configures a background sync loop. See sync.Options.
type SyncOptions = synceng.Options

// ProviderFactory builds a backend for an account (resolving credentials,
// choosing Gmail REST vs IMAP, etc.). Defined by the application layer.
type ProviderFactory func(ctx context.Context, acct Account) (provider.Provider, error)

// Config configures a Client.
type Config struct {
	DBPath   string          // SQLite path; created if absent
	Provider ProviderFactory // required: builds backends for accounts
}

// Client is the SDK entry point. Safe for concurrent use.
type Client struct {
	store   *store.Store
	eng     *synceng.Engine
	factory ProviderFactory

	mu        sync.Mutex
	providers map[AccountID]provider.Provider
	daemons   map[AccountID]context.CancelFunc
	drains    map[*time.Timer]struct{} // pending held-send drains, cancelled on Close
	closed    bool
}

// Open initializes the store (running migrations) and the sync engine.
func Open(ctx context.Context, cfg Config) (*Client, error) {
	if cfg.Provider == nil {
		return nil, fmt.Errorf("email: Config.Provider is required")
	}
	// Spool large attachments to a blobs/ dir alongside the database so they
	// don't bloat SQLite rows and can be loaded lazily on open.
	blobDir := filepath.Join(filepath.Dir(cfg.DBPath), "blobs")
	st, err := store.Open(ctx, cfg.DBPath, store.WithBlobStore(blob.NewFS(blobDir)))
	if err != nil {
		return nil, err
	}
	return &Client{
		store:     st,
		eng:       synceng.New(st),
		factory:   cfg.Provider,
		providers: map[AccountID]provider.Provider{},
		daemons:   map[AccountID]context.CancelFunc{},
		drains:    map[*time.Timer]struct{}{},
	}, nil
}

// provider returns the cached backend for acct, building it on first use. The
// factory (credential resolution, IMAP/REST selection) can block on I/O, so it
// runs without c.mu held — otherwise one account's cold start would serialize
// every other Client operation.
func (c *Client) provider(ctx context.Context, acct Account) (provider.Provider, error) {
	c.mu.Lock()
	if p, ok := c.providers[acct.ID]; ok {
		c.mu.Unlock()
		return p, nil
	}
	c.mu.Unlock()

	p, err := c.factory(ctx, acct)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed { // Client shut down while we were building; don't leak the provider
		_ = p.Close()
		return nil, fmt.Errorf("email: client is closed")
	}
	if existing, ok := c.providers[acct.ID]; ok { // lost a concurrent build; keep the winner
		_ = p.Close()
		return existing, nil
	}
	c.providers[acct.ID] = p
	return p, nil
}

// AddAccount registers an account and its mailbox topology, then returns the
// mailboxes. Call StartSync to begin pulling mail.
func (c *Client) AddAccount(ctx context.Context, acct Account) ([]Mailbox, error) {
	p, err := c.provider(ctx, acct)
	if err != nil {
		return nil, err
	}
	return c.eng.RegisterAccount(ctx, p, acct)
}

// ForgetAccount stops the account's sync loop, closes its provider, removes it
// from the local store (mailboxes/messages cascade), and drops the cached
// provider. Credentials live outside the store and must be deleted by the
// caller. Safe to call for an account that was never fully registered.
func (c *Client) ForgetAccount(ctx context.Context, acct AccountID) error {
	c.StopSync(acct)
	c.mu.Lock()
	if p, ok := c.providers[acct]; ok {
		_ = p.Close()
		delete(c.providers, acct)
	}
	c.mu.Unlock()
	return c.store.DeleteAccount(ctx, acct)
}

// Accounts lists configured accounts.
func (c *Client) Accounts(ctx context.Context) ([]Account, error) {
	return c.store.ListAccounts(ctx)
}

// Mailboxes returns the mailbox/label topology for an account.
func (c *Client) Mailboxes(ctx context.Context, acct AccountID) ([]Mailbox, error) {
	return c.store.Mailboxes(ctx, acct)
}

// CreateMailbox creates a folder on the server and records it in the local
// store, returning the new mailbox.
func (c *Client) CreateMailbox(ctx context.Context, acct Account, name string) (Mailbox, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Mailbox{}, fmt.Errorf("mailbox name is required")
	}
	p, err := c.provider(ctx, acct)
	if err != nil {
		return Mailbox{}, err
	}
	mb, err := p.CreateMailbox(ctx, name)
	if err != nil {
		return Mailbox{}, fmt.Errorf("create mailbox %q: %w", name, err)
	}
	mb.Account = acct.ID
	if err := c.store.UpsertMailboxes(ctx, []Mailbox{mb}); err != nil {
		return Mailbox{}, fmt.Errorf("create mailbox %q: %w", name, err)
	}
	return mb, nil
}

// RenameMailbox renames a folder on the server and updates the local store.
// System folders (inbox/sent/drafts/archive/…) cannot be renamed.
func (c *Client) RenameMailbox(ctx context.Context, acct Account, id LabelID, newName string) (Mailbox, error) {
	newName = strings.TrimSpace(newName)
	if newName == "" {
		return Mailbox{}, fmt.Errorf("mailbox name is required")
	}
	if err := c.guardUserMailbox(ctx, acct.ID, id); err != nil {
		return Mailbox{}, err
	}
	// Capture the user's chosen icon so it follows the folder even when the
	// provider rename changes the mailbox id (which creates a fresh row).
	var icon, iconWeight, iconColor string
	if mbs, err := c.store.Mailboxes(ctx, acct.ID); err == nil {
		for _, mb := range mbs {
			if mb.ID == id {
				icon, iconWeight, iconColor = mb.Icon, mb.IconWeight, mb.IconColor
				break
			}
		}
	}
	p, err := c.provider(ctx, acct)
	if err != nil {
		return Mailbox{}, err
	}
	mb, err := p.RenameMailbox(ctx, provider.MailboxRef{ID: id, Path: string(id)}, newName)
	if err != nil {
		return Mailbox{}, fmt.Errorf("rename mailbox %q to %q: %w", id, newName, err)
	}
	mb.Account = acct.ID
	if err := c.store.UpsertMailboxes(ctx, []Mailbox{mb}); err != nil {
		return Mailbox{}, fmt.Errorf("rename mailbox %q to %q: %w", id, newName, err)
	}
	if mb.ID != id {
		_ = c.store.DeleteMailbox(ctx, acct.ID, id)
		// New row defaults to no icon; reapply the captured choice.
		if icon != "" {
			_ = c.store.SetMailboxIcon(ctx, acct.ID, mb.ID, icon, iconWeight, iconColor)
		}
	}
	mb.Icon, mb.IconWeight, mb.IconColor = icon, iconWeight, iconColor
	return mb, nil
}

// DeleteMailbox removes a folder on the server and locally. System folders
// cannot be deleted.
func (c *Client) DeleteMailbox(ctx context.Context, acct Account, id LabelID) error {
	if err := c.guardUserMailbox(ctx, acct.ID, id); err != nil {
		return err
	}
	p, err := c.provider(ctx, acct)
	if err != nil {
		return err
	}
	if err := p.DeleteMailbox(ctx, provider.MailboxRef{ID: id, Path: string(id)}); err != nil {
		return fmt.Errorf("delete mailbox %q: %w", id, err)
	}
	return c.store.DeleteMailbox(ctx, acct.ID, id)
}

// SetMailboxIcon records the user's chosen icon (registry id + Phosphor weight +
// theme color) for a folder and returns the updated mailbox. This is local
// presentation metadata; it is preserved across topology sync.
func (c *Client) SetMailboxIcon(ctx context.Context, acct AccountID, id LabelID, icon, weight, color string) (Mailbox, error) {
	if err := c.store.SetMailboxIcon(ctx, acct, id, icon, weight, color); err != nil {
		return Mailbox{}, err
	}
	mbs, err := c.store.Mailboxes(ctx, acct)
	if err != nil {
		return Mailbox{}, err
	}
	for _, mb := range mbs {
		if mb.ID == id {
			return mb, nil
		}
	}
	return Mailbox{}, fmt.Errorf("mailbox %q not found", id)
}

// guardUserMailbox rejects mutations targeting a system-role mailbox.
func (c *Client) guardUserMailbox(ctx context.Context, acct AccountID, id LabelID) error {
	mbs, err := c.store.Mailboxes(ctx, acct)
	if err != nil {
		return err
	}
	for _, mb := range mbs {
		if mb.ID == id && mb.Role != RoleNone {
			return fmt.Errorf("cannot modify system mailbox %q", id)
		}
	}
	return nil
}

// Threads lists conversations for an account, newest activity first.
func (c *Client) Threads(ctx context.Context, acct AccountID, limit int) ([]Thread, error) {
	return c.store.Threads(ctx, acct, limit)
}

// ConversationList returns denormalized conversation-list rows (participants,
// snippet, count, latest sender, labels) ready to render an inbox view.
func (c *Client) ConversationList(ctx context.Context, acct AccountID, limit int) ([]ThreadListItem, error) {
	return c.store.ThreadListItems(ctx, acct, limit)
}

// ThreadMessages returns all messages in a thread, oldest first.
func (c *Client) ThreadMessages(ctx context.Context, acct AccountID, thread ThreadID) ([]Message, error) {
	return c.store.ThreadMessages(ctx, acct, thread)
}

// ThreadMessagesWithBodies returns all messages in a thread after fetching and
// caching any bodies missing from the local store. Use this for foreground
// thread opens, where a local cache miss should be satisfied immediately rather
// than showing snippets or issuing one request per message.
func (c *Client) ThreadMessagesWithBodies(ctx context.Context, acct Account, thread ThreadID) ([]Message, error) {
	start := time.Now()
	msgs, err := c.store.ThreadMessages(ctx, acct.ID, thread)
	if err != nil {
		return nil, err
	}
	opened := make([]MessageID, 0, len(msgs))
	ids := make([]MessageID, 0, len(msgs))
	for _, msg := range msgs {
		opened = append(opened, msg.ID)
		if !msg.BodyLoaded {
			ids = append(ids, msg.ID)
		}
	}
	if len(ids) == 0 {
		if err := c.store.TouchMessagesOpened(ctx, acct.ID, opened, time.Now()); err != nil {
			return nil, err
		}
		// All bodies already cached: this open did no provider I/O.
		slog.Info("thread open", "thread", thread, "messages", len(msgs), "cache", "hit", "fetched", 0, "dur", time.Since(start))
		return msgs, nil
	}
	p, err := c.provider(ctx, acct)
	if err != nil {
		return nil, err
	}
	// Best-effort: surface the error only when nothing loaded. A partial load
	// (n>0 with err) still cached some bodies, so render what we got rather than
	// failing the whole thread open.
	fetchStart := time.Now()
	if n, err := c.eng.LoadBodiesForeground(ctx, p, acct.ID, ids); err != nil && n == 0 {
		return nil, err
	}
	fetchDur := time.Since(fetchStart)
	if err := c.store.TouchMessagesOpened(ctx, acct.ID, opened, time.Now()); err != nil {
		return nil, err
	}
	out, err := c.store.ThreadMessages(ctx, acct.ID, thread)
	// Cache miss: `fetched` bodies came over the network; `fetch` isolates that
	// provider time from the surrounding store reads (`dur` is the whole call).
	slog.Info("thread open", "thread", thread, "messages", len(msgs), "cache", "miss",
		"fetched", len(ids), "fetch", fetchDur, "dur", time.Since(start))
	return out, err
}

// MailboxMessages returns messages in a mailbox/label, newest first.
func (c *Client) MailboxMessages(ctx context.Context, acct AccountID, mailbox LabelID, limit int) ([]Message, error) {
	return c.store.MailboxMessages(ctx, acct, mailbox, limit)
}

// Message returns a single message envelope from the local store.
func (c *Client) Message(ctx context.Context, acct AccountID, id MessageID) (Message, error) {
	return c.store.Message(ctx, acct, id)
}

// Search runs a local full-text query (subject/sender; body once body-sync
// lands), newest first.
func (c *Client) Search(ctx context.Context, acct AccountID, query string, limit int) ([]Message, error) {
	return c.store.Search(ctx, acct, query, limit)
}

// SearchServer runs a server-side search over the account's inbox, reaching mail
// not yet synced locally. Hits are cached into the local store (so they're
// openable like any other message) and returned newest-first. Returns nil when
// the provider doesn't support server search.
func (c *Client) SearchServer(ctx context.Context, acct Account, query string, limit int) ([]Message, error) {
	p, err := c.provider(ctx, acct)
	if err != nil {
		return nil, err
	}
	if !p.Capabilities().ServerSearch {
		return nil, nil
	}
	mb := provider.MailboxRef{ID: InboxLabel, Path: string(InboxLabel)}
	msgs, err := p.Search(ctx, mb, query, limit)
	if err != nil {
		return nil, err
	}
	if len(msgs) > 0 {
		// Cache envelopes so results open without another round-trip and dedup
		// against locally-synced mail by id.
		if err := c.store.SaveMessages(ctx, msgs); err != nil {
			return nil, err
		}
	}
	return msgs, nil
}

// MessageBody returns a message's decoded parts (inline bodies + attachments).
// The first call fetches from the provider, parses the MIME, persists the parts,
// and makes the body searchable; subsequent calls are served from the local
// store with no network.
func (c *Client) MessageBody(ctx context.Context, acct Account, id MessageID) ([]Part, error) {
	start := time.Now()
	p, err := c.provider(ctx, acct)
	if err != nil {
		return nil, err
	}
	parts, err := c.eng.LoadBodyForeground(ctx, p, acct.ID, id)
	if err != nil {
		return nil, err
	}
	if err := c.store.TouchMessagesOpened(ctx, acct.ID, []MessageID{id}, time.Now()); err != nil {
		return nil, err
	}
	// Time the backend portion and report the payload size: subtracting this
	// `dur` from the frontend's body-rpc timing isolates the Wails IPC transfer
	// of the part bytes (`bytes`), which dominates for image-heavy mail.
	var bytes int
	for _, pt := range parts {
		bytes += len(pt.Content)
	}
	slog.Info("message body", "id", id, "parts", len(parts), "bytes", bytes, "dur", time.Since(start))
	return parts, nil
}

func (c *Client) PruneBodies(ctx context.Context, acct AccountID, policy BodyRetentionPolicy) (BodyPruneResult, error) {
	return c.store.PruneBodies(ctx, acct, policy, time.Now())
}

// Bounds for PreloadMailboxBodies' opportunistic prewarm.
const (
	defaultPreloadBodies = 20
	maxPreloadBodies     = 100
)

// PreloadMailboxBodies fetches and caches bodies for the newest messages in a
// mailbox. It is intended for opportunistic UI prewarming after a list view has
// rendered; already-loaded bodies are skipped.
func (c *Client) PreloadMailboxBodies(ctx context.Context, acct Account, mailbox LabelID, limit int) (int, error) {
	if limit <= 0 {
		limit = defaultPreloadBodies
	}
	if limit > maxPreloadBodies {
		limit = maxPreloadBodies
	}
	msgs, err := c.store.MailboxMessages(ctx, acct.ID, mailbox, limit)
	if err != nil {
		return 0, err
	}
	ids := make([]MessageID, 0, len(msgs))
	for _, msg := range msgs {
		if msg.BodyLoaded {
			continue
		}
		ids = append(ids, msg.ID)
	}
	if len(ids) == 0 {
		return 0, nil
	}
	p, err := c.provider(ctx, acct)
	if err != nil {
		return 0, err
	}
	return c.eng.LoadBodies(ctx, p, acct.ID, ids)
}

// ReclassifyMailbox recalculates Gmail-like categories for recent messages in a
// mailbox, using any cached body text as extra signal.
func (c *Client) ReclassifyMailbox(ctx context.Context, acct AccountID, mailbox LabelID, limit int) (int, error) {
	return c.store.ReclassifyMailbox(ctx, acct, mailbox, limit)
}

// Attachments returns just the attachment parts of a message (loading the body
// if needed).
func (c *Client) Attachments(ctx context.Context, acct Account, id MessageID) ([]Part, error) {
	parts, err := c.MessageBody(ctx, acct, id)
	if err != nil {
		return nil, err
	}
	var atts []Part
	for _, p := range parts {
		if p.Disposition == "attachment" {
			atts = append(atts, p)
		}
	}
	return atts, nil
}

// PartContent returns a part's bytes, hydrating from the blob store when the
// part was spooled (empty Content, non-empty BlobRef). Inline and small parts
// carry their bytes directly and are returned as-is.
func (c *Client) PartContent(ctx context.Context, p Part) ([]byte, error) {
	if len(p.Content) > 0 || p.BlobRef == "" {
		return p.Content, nil
	}
	rc, err := c.store.BlobContent(ctx, p.BlobRef)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

// Events returns a changefeed channel of store mutations and a cancel func.
// Events are hints — coalesce and refetch rather than treating them as a log.
func (c *Client) Events() (<-chan Event, func()) {
	return c.store.Events().Subscribe()
}

// Send composes, queues, and immediately delivers a message. On delivery
// failure it remains in the durable outbox for retry (e.g. by a running sync
// loop). Returns true if it went out now.
func (c *Client) Send(ctx context.Context, acct Account, out Outgoing) (sent bool, err error) {
	if out.From.Addr == "" {
		out.From = Address{Addr: acct.Email}
	}
	raw, err := mime.Build(out, time.Now(), genMessageID(acct.Email))
	if err != nil {
		return false, err
	}
	if err := c.eng.EnqueueSend(ctx, acct.ID, model.RawMessage{Bytes: raw}, provider.SendOpts{Thread: out.Thread}); err != nil {
		return false, err
	}
	p, err := c.provider(ctx, acct)
	if err != nil {
		return false, err
	}
	n, err := c.eng.DrainOutbox(ctx, p, acct.ID)
	return n > 0, err
}

// SendHeld delivers a message with an optional pre-delivery hold (the undo-send
// window). With hold<=0 it behaves like Send — delivers immediately, returns 0.
// Otherwise it parks the message in the outbox until now+hold and returns the
// op id, so CancelSend can recall it within the window; the background outbox
// loop delivers it once the hold elapses.
func (c *Client) SendHeld(ctx context.Context, acct Account, out Outgoing, hold time.Duration) (int64, error) {
	if hold <= 0 { // no hold window: identical to Send (build → enqueue → drain)
		_, err := c.Send(ctx, acct, out)
		return 0, err
	}
	if out.From.Addr == "" {
		out.From = Address{Addr: acct.Email}
	}
	raw, err := mime.Build(out, time.Now(), genMessageID(acct.Email))
	if err != nil {
		return 0, err
	}
	id, err := c.eng.EnqueueSendAt(ctx, acct.ID, model.RawMessage{Bytes: raw}, provider.SendOpts{Thread: out.Thread}, time.Now().Add(hold))
	if err == nil {
		// Deliver right when the hold elapses instead of waiting for the next
		// outbox tick. Best-effort: if the send was cancelled, the op is gone and
		// this drain is a no-op; the background loop remains the durable fallback.
		c.scheduleDrain(acct, hold)
	}
	return id, err
}

// scheduleDrain fires a one-shot outbox drain for acct after the given delay.
// The timer is tracked so Close can stop it; without that a held send's drain
// would outlive the Client and rebuild a torn-down provider.
func (c *Client) scheduleDrain(acct Account, after time.Duration) {
	var t *time.Timer
	t = time.AfterFunc(after, func() {
		c.mu.Lock()
		_, live := c.drains[t]
		delete(c.drains, t)
		c.mu.Unlock()
		if !live { // stopped by Close between firing and acquiring the lock
			return
		}
		ctx := context.Background()
		p, err := c.provider(ctx, acct)
		if err != nil {
			return
		}
		_, _ = c.eng.DrainOutbox(ctx, p, acct.ID)
	})

	c.mu.Lock()
	if c.closed { // already shutting down; don't schedule
		c.mu.Unlock()
		t.Stop()
		return
	}
	c.drains[t] = struct{}{}
	c.mu.Unlock()
}

// CancelSend recalls a held send by op id before its hold elapses. Returns false
// if the message was already delivered.
func (c *Client) CancelSend(ctx context.Context, acct Account, opID int64) (bool, error) {
	return c.eng.CancelSend(ctx, acct.ID, opID)
}

// mutate applies a local optimistic change (fn) then flushes the outbox so the
// provider mutation goes out immediately. The local change has already published
// a changefeed event, so a UI updates instantly even if the network is slow.
func (c *Client) mutate(_ context.Context, acct Account, fn func(AccountID) error) error {
	// fn applies the change to the local store and enqueues a durable outbox op —
	// that is the part the caller must see succeed. The server write is performed
	// by the background outbox loop, nudged here for prompt delivery, so
	// interactive mutations don't block on a network round-trip. Delivery failures
	// are retried with backoff by the drain rather than surfaced synchronously.
	if err := fn(acct.ID); err != nil {
		return err
	}
	c.eng.NudgeOutbox(acct.ID)
	return nil
}

// MarkRead marks messages read (read=true) or unread (read=false).
func (c *Client) MarkRead(ctx context.Context, acct Account, ids []MessageID, read bool) error {
	add, remove := []Flag{model.FlagSeen}, []Flag(nil)
	if !read {
		add, remove = nil, []Flag{model.FlagSeen}
	}
	return c.mutate(ctx, acct, func(id AccountID) error { return c.eng.SetFlags(ctx, id, ids, add, remove) })
}

// Star flags/unflags messages.
func (c *Client) Star(ctx context.Context, acct Account, ids []MessageID, on bool) error {
	add, remove := []Flag{model.FlagFlagged}, []Flag(nil)
	if !on {
		add, remove = nil, []Flag{model.FlagFlagged}
	}
	return c.mutate(ctx, acct, func(id AccountID) error { return c.eng.SetFlags(ctx, id, ids, add, remove) })
}

// Archive ("Done") removes messages from the inbox and records completion for
// the Done-today metric.
func (c *Client) Archive(ctx context.Context, acct Account, ids []MessageID) error {
	_ = c.store.RecordDone(ctx, acct.ID, ids, time.Now())
	if archive, ok := c.mailboxByRole(ctx, acct.ID, RoleArchive); ok {
		return c.mutate(ctx, acct, func(id AccountID) error {
			return c.eng.Move(ctx, id, ids, archive)
		})
	}
	return c.mutate(ctx, acct, func(id AccountID) error {
		return c.eng.ApplyLabels(ctx, id, ids, nil, []LabelID{InboxLabel})
	})
}

// mailboxByRole returns the id of the account's mailbox carrying the given
// system role, if one exists.
func (c *Client) mailboxByRole(ctx context.Context, acct AccountID, role Role) (LabelID, bool) {
	mailboxes, err := c.store.Mailboxes(ctx, acct)
	if err != nil {
		return "", false
	}
	for _, mailbox := range mailboxes {
		if mailbox.Role == role {
			return mailbox.ID, true
		}
	}
	return "", false
}

// ApplyLabels adds and/or removes labels on messages.
func (c *Client) ApplyLabels(ctx context.Context, acct Account, ids []MessageID, add, remove []LabelID) error {
	return c.mutate(ctx, acct, func(id AccountID) error { return c.eng.ApplyLabels(ctx, id, ids, add, remove) })
}

// Move relocates messages to a destination mailbox/label.
func (c *Client) Move(ctx context.Context, acct Account, ids []MessageID, dst LabelID) error {
	slog.Debug("client.move", "account", acct.ID, "dst", dst, "ids", len(ids))
	if err := c.mutate(ctx, acct, func(id AccountID) error { return c.eng.Move(ctx, id, ids, dst) }); err != nil {
		slog.Error("move failed", "dst", dst, "err", err)
		return err
	}
	// Over IMAP a move expunges the source UID and recreates the message under a
	// new UID in dst. The destination only reflects the move once it is synced,
	// and dst is usually not in the background sync set — so pull it now. Without
	// this the moved mail vanishes from INBOX and never appears in its folder.
	p, err := c.provider(ctx, acct)
	if err != nil {
		return err
	}
	pulled, err := c.pullMailbox(ctx, p, acct.ID, dst, 1)
	if err != nil {
		return fmt.Errorf("sync destination %q after move: %w", dst, err)
	}
	slog.Debug("client.move: done", "dst", dst, "dstMessagesPulled", pulled)
	return nil
}

// Delete moves messages to Trash. When the account has a Trash mailbox this is a
// reversible IMAP move (so it can be undone, and the message stays in the local
// store under the Trash folder); only without one does it fall back to a hard
// \Deleted + expunge.
func (c *Client) Delete(ctx context.Context, acct Account, ids []MessageID) error {
	if trash, ok := c.mailboxByRole(ctx, acct.ID, RoleTrash); ok {
		return c.mutate(ctx, acct, func(id AccountID) error {
			return c.eng.Move(ctx, id, ids, trash)
		})
	}
	return c.mutate(ctx, acct, func(id AccountID) error { return c.eng.Delete(ctx, id, ids) })
}

// ── drafts (local, autosaved) ───────────────────────────────────

// SaveDraft upserts a compose draft locally and returns its id (generated if
// empty). Cheap — safe to call on every keystroke for autosave.
func (c *Client) SaveDraft(ctx context.Context, acct AccountID, id string, out Outgoing) (string, error) {
	return c.store.SaveDraft(ctx, acct, id, out)
}

// Drafts lists an account's drafts, most recently updated first.
func (c *Client) Drafts(ctx context.Context, acct AccountID) ([]Draft, error) {
	return c.store.ListDrafts(ctx, acct)
}

// Draft returns a single draft.
func (c *Client) Draft(ctx context.Context, acct AccountID, id string) (Draft, error) {
	return c.store.GetDraft(ctx, acct, id)
}

// DiscardDraft deletes a draft.
func (c *Client) DiscardDraft(ctx context.Context, acct AccountID, id string) error {
	return c.store.DeleteDraft(ctx, acct, id)
}

// SendDraft sends a saved draft, then discards it on success.
func (c *Client) SendDraft(ctx context.Context, acct Account, id string) (bool, error) {
	d, err := c.store.GetDraft(ctx, acct.ID, id)
	if err != nil {
		return false, err
	}
	sent, err := c.Send(ctx, acct, d.Message)
	if err != nil {
		return false, err
	}
	if sent {
		_ = c.store.DeleteDraft(ctx, acct.ID, id)
	}
	return sent, nil
}

// ── snooze ──────────────────────────────────────────────────────

// Snooze hides messages from the inbox until `until`; a running sync loop
// returns them automatically when the time elapses.
func (c *Client) Snooze(ctx context.Context, acct Account, ids []MessageID, until time.Time) error {
	if err := c.store.Snooze(ctx, acct.ID, ids, until); err != nil {
		return err
	}
	return c.mutate(ctx, acct, func(id AccountID) error {
		return c.eng.ApplyLabels(ctx, id, ids, nil, []LabelID{InboxLabel})
	})
}

// Unsnooze returns snoozed messages to the inbox immediately.
func (c *Client) Unsnooze(ctx context.Context, acct Account, ids []MessageID) error {
	if err := c.store.Unsnooze(ctx, acct.ID, ids); err != nil {
		return err
	}
	return c.mutate(ctx, acct, func(id AccountID) error {
		return c.eng.ApplyLabels(ctx, id, ids, []LabelID{InboxLabel}, nil)
	})
}

// Snoozed lists current snoozes for an account.
func (c *Client) Snoozed(ctx context.Context, acct AccountID) ([]Snoozed, error) {
	return c.store.ListSnoozes(ctx, acct)
}

// ── contacts ────────────────────────────────────────────────────

// SearchContacts returns address-book entries (harvested from message envelopes
// during sync) matching query, ranked by frequency then recency. Drives
// recipient autocomplete in compose.
func (c *Client) SearchContacts(ctx context.Context, acct AccountID, query string, limit int) ([]Contact, error) {
	return c.store.SearchContacts(ctx, acct, query, limit)
}

// DoneToday counts messages archived ("done") since local midnight.
func (c *Client) DoneToday(ctx context.Context, acct AccountID) (int, error) {
	now := time.Now()
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return c.store.DoneSince(ctx, acct, midnight)
}

// SyncOnce populates a mailbox on demand: it forward-syncs (to advance the
// cursor and catch newly-arrived mail) then backfills up to two pages of
// history. A first forward sync of a fresh mailbox only sets the baseline and
// returns nothing — the actual messages come from backfill — so a plain
// forward sync leaves never-before-synced folders looking empty.
func (c *Client) SyncOnce(ctx context.Context, acct Account, mailbox LabelID) (int, error) {
	p, err := c.provider(ctx, acct)
	if err != nil {
		return 0, err
	}
	n, err := c.pullMailbox(ctx, p, acct.ID, mailbox, 2)
	if err != nil {
		slog.Error("sync mailbox failed", "mailbox", mailbox, "err", err)
		return 0, err
	}
	slog.Debug("client.syncOnce", "mailbox", mailbox, "pulled", n)
	return n, nil
}

// pullMailbox forward-syncs then backfills up to `pages` pages of a mailbox,
// returning the number of messages written. Forward sync of a fresh mailbox
// only establishes the cursor (0 messages); backfill pulls the real history.
func (c *Client) pullMailbox(ctx context.Context, p provider.Provider, acct AccountID, mailbox LabelID, pages int) (int, error) {
	ref := provider.MailboxRef{ID: mailbox, Path: string(mailbox)}
	fwd, err := c.eng.SyncForward(ctx, p, acct, ref)
	if err != nil {
		return 0, err
	}
	total := len(fwd)
	for range pages {
		n, done, berr := c.eng.BackfillPage(ctx, p, acct, ref, 100)
		if berr != nil {
			return total, berr
		}
		total += n
		if done {
			break
		}
	}
	return total, nil
}

// StartSync launches the background sync loop for an account (forward poll +
// IDLE/push where supported, resumable backfill, outbox drain). It returns
// immediately; call StopSync or Close to stop. Re-calling for the same account
// replaces the previous loop.
func (c *Client) StartSync(ctx context.Context, acct Account, mailboxes []LabelID, opts SyncOptions) error {
	p, err := c.provider(ctx, acct)
	if err != nil {
		return err
	}
	refs := make([]provider.MailboxRef, len(mailboxes))
	for i, m := range mailboxes {
		refs[i] = provider.MailboxRef{ID: m, Path: string(m)}
	}

	c.mu.Lock()
	if cancel, ok := c.daemons[acct.ID]; ok {
		cancel()
	}
	loopCtx, cancel := context.WithCancel(context.Background())
	c.daemons[acct.ID] = cancel
	c.mu.Unlock()

	go func() { _ = c.eng.RunAccount(loopCtx, p, acct, refs, opts) }()
	return nil
}

// StopSync stops the background loop for an account, if running.
func (c *Client) StopSync(acct AccountID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if cancel, ok := c.daemons[acct]; ok {
		cancel()
		delete(c.daemons, acct)
	}
}

// Close stops all sync loops, cancels pending held-send drains, closes
// providers, and closes the store. It is idempotent.
func (c *Client) Close() error {
	c.mu.Lock()
	c.closed = true
	for t := range c.drains {
		t.Stop()
		delete(c.drains, t)
	}
	for id, cancel := range c.daemons {
		cancel()
		delete(c.daemons, id)
	}
	for id, p := range c.providers {
		_ = p.Close()
		delete(c.providers, id)
	}
	c.mu.Unlock()
	return c.store.Close()
}

// Reply builds an Outgoing pre-wired for correct threading: recipient and
// References/In-Reply-To are derived from orig. replyAll includes the original
// To/Cc recipients (minus self). Caller fills Text/HTML/Attachments and passes
// it to Send.
func Reply(orig Message, self Address, replyAll bool) Outgoing {
	out := Outgoing{
		From:      self,
		Subject:   ensurePrefix(orig.Subject, "Re: "),
		Thread:    orig.Thread,
		InReplyTo: orig.RFCMessageID,
		// References = original chain + the message being replied to.
		References: append(slices.Clone(orig.References), orig.RFCMessageID),
	}
	out.To = orig.From
	if replyAll {
		for _, a := range append(orig.To, orig.Cc...) {
			if a.Addr != self.Addr {
				out.Cc = append(out.Cc, a)
			}
		}
	}
	return out
}

// Forward builds an Outgoing for forwarding orig (subject prefixed, no
// recipients or threading set).
func Forward(orig Message, self Address) Outgoing {
	return Outgoing{From: self, Subject: ensurePrefix(orig.Subject, "Fwd: ")}
}

func ensurePrefix(subject, prefix string) string {
	if len(subject) >= len(prefix) && strings.EqualFold(subject[:len(prefix)], prefix) {
		return subject
	}
	return prefix + subject
}

// genMessageID builds a unique RFC 5322 Message-ID from the sender's domain.
func genMessageID(from string) string {
	domain := "localhost"
	if i := strings.LastIndexByte(from, '@'); i >= 0 {
		domain = from[i+1:]
	}
	var b [16]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("<%s@%s>", hex.EncodeToString(b[:]), domain)
}
