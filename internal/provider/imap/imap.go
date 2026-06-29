// Package imap implements provider.Provider over IMAP4rev2 (go-imap v2) for
// fetch/sync (IDLE for push, UID-based incremental sync) and SMTP (go-smtp) for
// send. Authentication is a plain password / app-password (PLAIN SASL).
//
// This file covers the read path: connect/auth, list mailboxes, backfill and
// incremental envelope sync, and on-demand body fetch. Writes live in mutate.go,
// send/draft in send.go, and IDLE watching in watch.go.
package imap

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-sasl"

	"github.com/atterpac/email/internal/model"
	"github.com/atterpac/email/internal/provider"
)

// Config describes how to connect to an IMAP account. Authentication is a plain
// password / app-password over an implicit-TLS connection.
type Config struct {
	Account  model.AccountID
	Host     string // e.g. imap.gmail.com
	Port     int    // e.g. 993
	Username string

	// Password is the plain / app-password used for IMAP and SMTP login.
	Password string

	// SMTP delivery (Send). Defaults to the IMAP host on port 587 (STARTTLS).
	SMTPHost string
	SMTPPort int
}

// Provider is an IMAP-backed provider.Provider. Safe for sequential use; the
// engine drives one Provider per account.
type Provider struct {
	cfg Config

	opMu     sync.Mutex // serializes whole operations (one in flight at a time)
	connMu   sync.Mutex // guards c/selected during dial/reset/close
	c        *imapclient.Client
	selected string // currently selected mailbox path

	// Foreground body fetches run on a dedicated connection so an interactive
	// thread open never queues on opMu behind a bulk launch-warm or history
	// backfill holding the shared connection. These fields mirror the shared set
	// above (op serialization, connection guard, selected-folder tracking) and
	// are dialed lazily on the first interactive open.
	fgOpMu     sync.Mutex
	fgConnMu   sync.Mutex
	fgC        *imapclient.Client
	fgSelected string
}

// New returns an unconnected Provider; the first call dials lazily.
func New(cfg Config) *Provider { return &Provider{cfg: cfg} }

// cursor is the persisted sync position for a mailbox. For forward sync LastUID
// is the highest seen UID and ModSeq the CONDSTORE high-water mark; for backfill
// paging LastUID is repurposed as the next UID to page down from.
type cursor struct {
	UIDValidity uint32   `json:"uidvalidity"`
	LastUID     imap.UID `json:"last_uid"`
	ModSeq      uint64   `json:"modseq,omitempty"`
}

func encodeCursor(c cursor) *provider.Cursor {
	b, _ := json.Marshal(c) // cannot fail: cursor is a fixed struct of marshalable fields
	return &provider.Cursor{Bytes: b}
}

func decodeCursor(c *provider.Cursor) (cursor, bool) {
	if c == nil || len(c.Bytes) == 0 {
		return cursor{}, false
	}
	var out cursor
	if err := json.Unmarshal(c.Bytes, &out); err != nil {
		return cursor{}, false
	}
	return out, true
}

// conn returns a connected, authenticated client, dialing if needed. ctx bounds
// the TCP connect and TLS handshake; it does not cancel the subsequent auth.
func (p *Provider) conn(ctx context.Context) (*imapclient.Client, error) {
	p.connMu.Lock()
	defer p.connMu.Unlock()
	if p.c != nil {
		return p.c, nil
	}
	c, err := p.dial(ctx)
	if err != nil {
		return nil, err
	}
	p.c = c
	p.selected = ""
	return c, nil
}

// dial opens and authenticates a fresh IMAP connection. It locks nothing and
// touches no Provider connection state, so it backs both the shared (conn) and
// foreground (fgConn) channels.
func (p *Provider) dial(ctx context.Context) (*imapclient.Client, error) {
	addr := fmt.Sprintf("%s:%d", p.cfg.Host, p.cfg.Port)
	// Replicate imapclient.DialTLS but via a ctx-aware dialer so a hung connect
	// is bounded by the caller's deadline. ServerName is inferred from addr.
	dialer := &tls.Dialer{Config: &tls.Config{NextProtos: []string{"imap"}}}
	netConn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("imap dial %s: %w", addr, err)
	}
	c := imapclient.New(netConn, nil)
	saslClient, err := p.saslClient()
	if err != nil {
		c.Close()
		return nil, err
	}
	if err := c.Authenticate(saslClient); err != nil {
		c.Close()
		return nil, fmt.Errorf("imap auth: %w", err)
	}
	return c, nil
}

// fgConn returns the dedicated foreground connection, dialing lazily. Guarded by
// fgConnMu, mirroring conn for the shared channel.
func (p *Provider) fgConn(ctx context.Context) (*imapclient.Client, error) {
	p.fgConnMu.Lock()
	defer p.fgConnMu.Unlock()
	if p.fgC != nil {
		return p.fgC, nil
	}
	c, err := p.dial(ctx)
	if err != nil {
		return nil, err
	}
	p.fgC = c
	p.fgSelected = ""
	return c, nil
}

// fgReset drops the foreground connection so the next open reconnects (on error).
func (p *Provider) fgReset() {
	p.fgConnMu.Lock()
	defer p.fgConnMu.Unlock()
	if p.fgC != nil {
		p.fgC.Close()
		p.fgC = nil
		p.fgSelected = ""
	}
}

// saslClient builds the PLAIN SASL client from the configured password.
func (p *Provider) saslClient() (sasl.Client, error) {
	if p.cfg.Password == "" {
		return nil, errors.New("imap: no password configured")
	}
	return sasl.NewPlainClient("", p.cfg.Username, p.cfg.Password), nil
}

// reset drops the connection so the next call reconnects (used on error).
func (p *Provider) reset() {
	p.connMu.Lock()
	defer p.connMu.Unlock()
	if p.c != nil {
		p.c.Close()
		p.c = nil
		p.selected = ""
	}
}

// ListMailboxes lists selectable mailboxes with message/unread counts and roles.
func (p *Provider) ListMailboxes(ctx context.Context) ([]model.Mailbox, error) {
	p.opMu.Lock()
	defer p.opMu.Unlock()

	c, err := p.conn(ctx)
	if err != nil {
		return nil, err
	}
	opts := &imap.ListOptions{
		ReturnStatus:     &imap.StatusOptions{NumMessages: true, NumUnseen: true},
		ReturnSpecialUse: true, // ask Gmail to emit \Sent \Drafts \Trash \Junk \All
	}
	data, err := c.List("", "*", opts).Collect()
	if err != nil {
		p.reset()
		return nil, fmt.Errorf("imap list: %w", err)
	}
	out := make([]model.Mailbox, 0, len(data))
	for _, d := range data {
		if hasAttr(d.Attrs, imap.MailboxAttrNonExistent) || hasAttr(d.Attrs, imap.MailboxAttrNoSelect) {
			continue
		}
		mb := model.Mailbox{
			ID:      model.LabelID(d.Mailbox),
			Account: p.cfg.Account,
			Name:    d.Mailbox,
			Path:    d.Mailbox,
			Role:    roleFromAttrs(d.Mailbox, d.Attrs),
		}
		if d.Status != nil {
			if d.Status.NumMessages != nil {
				mb.Total = int(*d.Status.NumMessages)
			}
			if d.Status.NumUnseen != nil {
				mb.Unread = int(*d.Status.NumUnseen)
			}
		}
		out = append(out, mb)
	}
	return out, nil
}

// Sync performs an incremental forward sync from cur. With CONDSTORE it returns
// every message whose MODSEQ advanced (new + flag/label changes); without it,
// only newly arrived UIDs. A nil/stale cursor just re-establishes the position.
func (p *Provider) Sync(ctx context.Context, mb provider.MailboxRef, cur *provider.Cursor) (provider.Changes, *provider.Cursor, error) {
	p.opMu.Lock()
	defer p.opMu.Unlock()

	c, err := p.conn(ctx)
	if err != nil {
		return provider.Changes{}, nil, err
	}
	condstore := c.Caps().Has(imap.CapCondStore)
	sel, err := c.Select(mb.Path, &imap.SelectOptions{CondStore: condstore}).Wait()
	if err != nil {
		p.reset()
		return provider.Changes{}, nil, fmt.Errorf("imap select %q: %w", mb.Path, err)
	}
	p.selected = mb.Path

	// UIDNext is 1-based; guard the empty-mailbox case (UIDNext can be 1 or 0)
	// so newest doesn't underflow to a huge UID.
	var newest imap.UID
	if sel.UIDNext > 1 {
		newest = sel.UIDNext - 1
	}
	prev, ok := decodeCursor(cur)

	// First sync, or UIDVALIDITY rotated: establish the position without
	// backfilling; historical mail is pulled separately via Backfill.
	if !ok || prev.UIDValidity != sel.UIDValidity {
		return provider.Changes{}, encodeCursor(cursor{
			UIDValidity: sel.UIDValidity, LastUID: newest, ModSeq: sel.HighestModSeq,
		}), nil
	}

	ch := provider.Changes{}
	maxUID := prev.LastUID
	// collect fetches set and appends its envelopes to ch, tracking the high UID.
	collect := func(set imap.UIDSet, changedSince uint64) error {
		msgs, err := p.fetchEnvelopes(ctx, c, mb.Path, set, changedSince)
		if err != nil {
			p.reset()
			return err
		}
		for _, m := range msgs {
			ch.Upserted = append(ch.Upserted, toMessage(p.cfg.Account, mb, m))
			if m.UID > maxUID {
				maxUID = m.UID
			}
		}
		return nil
	}
	switch {
	case condstore && prev.ModSeq > 0:
		// CONDSTORE: one CHANGEDSINCE fetch returns every message whose MODSEQ
		// advanced — both newly arrived and flag-changed — so read/star/label
		// changes on existing mail are captured, not just new messages.
		var set imap.UIDSet
		set.AddRange(1, 0) // 1:*
		if err := collect(set, prev.ModSeq); err != nil {
			return provider.Changes{}, nil, err
		}
	case prev.LastUID < newest:
		// No CONDSTORE: fall back to new UIDs only.
		var set imap.UIDSet
		set.AddRange(prev.LastUID+1, 0)
		if err := collect(set, 0); err != nil {
			return provider.Changes{}, nil, err
		}
	}
	return ch, encodeCursor(cursor{
		UIDValidity: sel.UIDValidity, LastUID: maxUID, ModSeq: sel.HighestModSeq,
	}), nil
}

// Backfill pages envelopes newest-first by UID.
func (p *Provider) Backfill(ctx context.Context, mb provider.MailboxRef, page *provider.Cursor, limit int) (provider.Changes, *provider.Cursor, bool, error) {
	p.opMu.Lock()
	defer p.opMu.Unlock()

	if limit <= 0 {
		limit = 100
	}
	c, err := p.conn(ctx)
	if err != nil {
		return provider.Changes{}, nil, false, err
	}
	sel, err := c.Select(mb.Path, nil).Wait()
	if err != nil {
		p.reset()
		return provider.Changes{}, nil, false, fmt.Errorf("imap select %q: %w", mb.Path, err)
	}
	p.selected = mb.Path

	// Determine the high UID to page down from. UIDNext is 1-based; guard the
	// empty/zero case so hi doesn't underflow to a huge UID.
	var hi imap.UID
	if sel.UIDNext > 1 {
		hi = sel.UIDNext - 1 // newest
	}
	if prev, ok := decodeCursor(page); ok && prev.UIDValidity == sel.UIDValidity {
		hi = prev.LastUID // here LastUID is "next UID to fetch (inclusive), paging down"
	}
	if sel.NumMessages == 0 || hi < 1 {
		return provider.Changes{}, nil, true, nil
	}
	lo := imap.UID(1)
	if hi > imap.UID(limit) {
		lo = hi - imap.UID(limit) + 1
	}

	var set imap.UIDSet
	set.AddRange(lo, hi)
	msgs, err := p.fetchEnvelopes(ctx, c, mb.Path, set, 0)
	if err != nil {
		p.reset()
		return provider.Changes{}, nil, false, err
	}
	ch := provider.Changes{Upserted: toMessages(p.cfg.Account, mb, msgs)}

	done := lo <= 1
	if done {
		return ch, nil, true, nil
	}
	next := encodeCursor(cursor{UIDValidity: sel.UIDValidity, LastUID: lo - 1})
	return ch, next, false, nil
}

// fetchEnvelopes fetches UID+flags+envelope+structure for set. When
// changedSince > 0 it adds CONDSTORE CHANGEDSINCE, returning only messages whose
// MODSEQ advanced past it.
//
// Resilience: go-imap/v2 (beta.8) cannot parse some servers' BODYSTRUCTURE
// responses — notably Gmail multiparts with a NIL subpart, which surface as
// "imapwire: expected '(', got \"N\"". That parse error also desyncs the
// connection. When it happens we reconnect, reselect, and re-fetch the same set
// WITHOUT BODYSTRUCTURE so the server never emits the unparseable bytes. The
// envelopes still sync (attachment flags are simply left unset, later derivable
// from the body), which stops backfill from looping forever on a poison page.
func (p *Provider) fetchEnvelopes(ctx context.Context, c *imapclient.Client, mbPath string, set imap.UIDSet, changedSince uint64) ([]*imapclient.FetchMessageBuffer, error) {
	msgs, err := fetchEnvelopesOnce(c, set, changedSince, true)
	if err == nil {
		return msgs, nil
	}
	if !isBodyStructureParseError(err) {
		return nil, err
	}

	p.reset()
	c2, cerr := p.conn(ctx)
	if cerr != nil {
		p.reset()
		return nil, cerr
	}
	// Reselect with CondStore when the fetch carries CHANGEDSINCE, otherwise the
	// server may reject the re-issued CHANGEDSINCE fetch on a non-CONDSTORE SELECT.
	if _, serr := c2.Select(mbPath, &imap.SelectOptions{CondStore: changedSince > 0}).Wait(); serr != nil {
		p.reset()
		return nil, fmt.Errorf("imap reselect %q: %w", mbPath, serr)
	}
	p.selected = mbPath
	msgs, err = fetchEnvelopesOnce(c2, set, changedSince, false)
	if err != nil {
		return nil, err
	}
	return msgs, nil
}

func fetchEnvelopesOnce(c *imapclient.Client, set imap.UIDSet, changedSince uint64, withStructure bool) ([]*imapclient.FetchMessageBuffer, error) {
	opts := &imap.FetchOptions{
		UID:      true,
		Flags:    true,
		Envelope: true,
	}
	if withStructure {
		opts.BodyStructure = &imap.FetchItemBodyStructure{Extended: true}
	}
	if changedSince > 0 {
		opts.ModSeq = true
		opts.ChangedSince = changedSince
	}
	msgs, err := c.Fetch(set, opts).Collect()
	if err != nil {
		return nil, fmt.Errorf("imap fetch: %w", err)
	}
	return msgs, nil
}

// isBodyStructureParseError detects the go-imap BODYSTRUCTURE parse failure so we
// can retry without it rather than treat the page as permanently broken. go-imap
// beta.8 surfaces this two ways depending on where parsing trips: a named
// "body-type-*" production failure, or the lower-level "imapwire: expected '('"
// desync that the NIL-subpart multiparts trigger.
func isBodyStructureParseError(err error) bool {
	s := err.Error()
	return strings.Contains(s, "body-type-") || strings.Contains(s, "imapwire: expected '('")
}

// FetchBodies returns the raw RFC 5322 bytes for ids, selecting mb first so the
// UIDs resolve against the mailbox the messages actually live in.
func (p *Provider) FetchBodies(ctx context.Context, mb provider.MailboxRef, ids []model.MessageID) ([]model.RawMessage, error) {
	p.opMu.Lock()
	defer p.opMu.Unlock()
	c, err := p.conn(ctx)
	if err != nil {
		return nil, err
	}
	return p.fetchBodies(ctx, c, mb, ids, &p.selected, p.reset)
}

// FetchBodiesForeground fetches bodies on a connection reserved for interactive
// thread opens, so an open never waits on opMu behind a bulk launch-warm or
// history backfill holding the shared connection. Behaviour is otherwise
// identical to FetchBodies. The sync engine routes foreground opens here when the
// provider offers it (see sync.foregroundFetcher).
func (p *Provider) FetchBodiesForeground(ctx context.Context, mb provider.MailboxRef, ids []model.MessageID) ([]model.RawMessage, error) {
	p.fgOpMu.Lock()
	defer p.fgOpMu.Unlock()
	c, err := p.fgConn(ctx)
	if err != nil {
		return nil, err
	}
	return p.fetchBodies(ctx, c, mb, ids, &p.fgSelected, p.fgReset)
}

// fetchBodies fetches the raw RFC 5322 bytes for ids in mb over connection c,
// selecting the folder first if needed. selected tracks c's currently selected
// mailbox (read/written under the caller's op lock); reset drops c on error. It
// is shared by the foreground and shared-connection entry points above.
func (p *Provider) fetchBodies(ctx context.Context, c *imapclient.Client, mb provider.MailboxRef, ids []model.MessageID, selected *string, reset func()) ([]model.RawMessage, error) {
	// Bodies are fetched by UID/Message-ID within the selected mailbox, so we
	// must select the folder the message actually lives in (not always INBOX) —
	// otherwise non-inbox messages resolve to no UID and the body fetch is empty.
	mbPath := mb.Path
	if mbPath == "" {
		mbPath = "INBOX"
	}
	if *selected != mbPath {
		if _, err := c.Select(mbPath, nil).Wait(); err != nil {
			reset()
			return nil, fmt.Errorf("imap select %q: %w", mbPath, err)
		}
		*selected = mbPath
	}
	set, idByUID, err := p.resolveUIDs(c, ids)
	if err != nil {
		return nil, err
	}
	if len(set) == 0 {
		return nil, nil
	}
	opts := &imap.FetchOptions{
		UID:         true,
		BodySection: []*imap.FetchItemBodySection{{Peek: true}}, // BODY.PEEK[]
	}
	msgs, err := c.Fetch(set, opts).Collect()
	if err != nil {
		reset()
		return nil, fmt.Errorf("imap fetch bodies: %w", err)
	}
	out := make([]model.RawMessage, 0, len(msgs))
	for _, m := range msgs {
		if len(m.BodySection) == 0 {
			continue
		}
		id := idByUID[m.UID]
		if id == "" {
			id = messageID(p.cfg.Account, m.UID)
		}
		out = append(out, model.RawMessage{
			ID:    id,
			Bytes: m.BodySection[0].Bytes,
		})
	}
	return out, nil
}

func (p *Provider) searchMessageID(c *imapclient.Client, id model.MessageID) ([]imap.UID, error) {
	needle := strings.TrimSpace(string(id))
	if needle == "" {
		return nil, nil
	}
	data, err := c.UIDSearch(&imap.SearchCriteria{
		Header: []imap.SearchCriteriaHeaderField{{Key: "Message-ID", Value: needle}},
	}, nil).Wait()
	if err != nil {
		p.reset()
		return nil, fmt.Errorf("imap search message-id: %w", err)
	}
	return data.AllUIDs(), nil
}

// Capabilities reports the optional features this provider supports.
func (p *Provider) Capabilities() provider.Caps {
	return provider.Caps{Idle: true, ServerSearch: true}
}

// Close logs out and tears down the shared connection if one is open.
func (p *Provider) Close() error {
	// Take opMu so close can't tear the connection out from under an in-flight
	// operation, then connMu to guard the connection fields themselves.
	p.opMu.Lock()
	defer p.opMu.Unlock()
	p.connMu.Lock()
	defer p.connMu.Unlock()
	// Tear down the foreground connection too. Locks are taken shared-then-
	// foreground here; no path acquires them in the reverse order, so this can't
	// deadlock against an in-flight open (which holds only the fg locks).
	p.fgOpMu.Lock()
	defer p.fgOpMu.Unlock()
	p.fgConnMu.Lock()
	defer p.fgConnMu.Unlock()
	if p.fgC != nil {
		p.fgC.Logout().Wait()
		p.fgC.Close()
		p.fgC = nil
		p.fgSelected = ""
	}
	if p.c == nil {
		return nil
	}
	err := p.c.Logout().Wait()
	p.c.Close()
	p.c = nil
	p.selected = ""
	return err
}

// Mutations (ApplyFlags/ApplyLabels/Move/Delete) live in mutate.go;
// Send/SaveDraft in send.go; Watch in watch.go.

// compile-time check
var _ provider.Provider = (*Provider)(nil)
