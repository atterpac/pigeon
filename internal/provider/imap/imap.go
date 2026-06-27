// Package imap implements provider.Provider over IMAP4rev2 (go-imap v2) for
// fetch/sync (IDLE for push, UID-based incremental sync) and SMTP (go-smtp) for
// send. XOAUTH2 SASL where supported, password otherwise.
//
// This file covers the read path: connect/auth, list mailboxes, backfill and
// incremental envelope sync, and on-demand body fetch. Write operations and
// send are stubbed pending later milestones.
package imap

import (
	"context"
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

// Config describes how to connect to an IMAP account. Provide exactly one auth
// method: NewSASL (XOAUTH2) or Password (plain).
type Config struct {
	Account  model.AccountID
	Host     string // e.g. imap.gmail.com
	Port     int    // e.g. 993
	Username string

	// NewSASL builds a fresh SASL client per login so the access token is
	// current. Used for XOAUTH2. Mutually exclusive with Password.
	NewSASL func() (sasl.Client, error)
	// Password is the plain/app-password fallback.
	Password string

	// SMTP delivery (Send). Defaults to the IMAP host on port 587 (STARTTLS).
	SMTPHost string
	SMTPPort int
}

// Provider is an IMAP-backed provider.Provider. Safe for sequential use; the
// engine drives one Provider per account.
type Provider struct {
	cfg Config

	opMu     sync.Mutex
	mu       sync.Mutex
	c        *imapclient.Client
	selected string // currently selected mailbox path
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
	b, _ := json.Marshal(c)
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

// conn returns a connected, authenticated client, dialing if needed.
func (p *Provider) conn(ctx context.Context) (*imapclient.Client, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.c != nil {
		return p.c, nil
	}
	addr := fmt.Sprintf("%s:%d", p.cfg.Host, p.cfg.Port)
	c, err := imapclient.DialTLS(addr, nil)
	if err != nil {
		return nil, fmt.Errorf("imap dial %s: %w", addr, err)
	}
	saslClient, err := p.saslClient()
	if err != nil {
		c.Close()
		return nil, err
	}
	if err := c.Authenticate(saslClient); err != nil {
		c.Close()
		return nil, fmt.Errorf("imap auth: %w", err)
	}
	p.c = c
	p.selected = ""
	return c, nil
}

// saslClient builds the configured SASL client (XOAUTH2 or PLAIN).
func (p *Provider) saslClient() (sasl.Client, error) {
	switch {
	case p.cfg.NewSASL != nil:
		return p.cfg.NewSASL()
	case p.cfg.Password != "":
		return sasl.NewPlainClient("", p.cfg.Username, p.cfg.Password), nil
	default:
		return nil, errors.New("imap: no auth method configured")
	}
}

// reset drops the connection so the next call reconnects (used on error).
func (p *Provider) reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.c != nil {
		p.c.Close()
		p.c = nil
		p.selected = ""
	}
}

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

	newest := sel.UIDNext - 1
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
	switch {
	case condstore && prev.ModSeq > 0:
		// CONDSTORE: one CHANGEDSINCE fetch returns every message whose MODSEQ
		// advanced — both newly arrived and flag-changed — so read/star/label
		// changes on existing mail are captured, not just new messages.
		var set imap.UIDSet
		set.AddRange(1, 0) // 1:*
		msgs, err := p.fetchEnvelopes(ctx, c, mb.Path, set, prev.ModSeq)
		if err != nil {
			p.reset()
			return provider.Changes{}, nil, err
		}
		for _, m := range msgs {
			ch.Upserted = append(ch.Upserted, toMessage(p.cfg.Account, mb, m))
			if m.UID > maxUID {
				maxUID = m.UID
			}
		}
	case prev.LastUID < newest:
		// No CONDSTORE: fall back to new UIDs only.
		var set imap.UIDSet
		set.AddRange(prev.LastUID+1, 0)
		msgs, err := p.fetchEnvelopes(ctx, c, mb.Path, set, 0)
		if err != nil {
			p.reset()
			return provider.Changes{}, nil, err
		}
		for _, m := range msgs {
			ch.Upserted = append(ch.Upserted, toMessage(p.cfg.Account, mb, m))
			if m.UID > maxUID {
				maxUID = m.UID
			}
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

	// Determine the high UID to page down from.
	hi := sel.UIDNext - 1 // newest
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
	ch := provider.Changes{}
	for _, m := range msgs {
		ch.Upserted = append(ch.Upserted, toMessage(p.cfg.Account, mb, m))
	}

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
	if _, serr := c2.Select(mbPath, nil).Wait(); serr != nil {
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
// can retry without it rather than treat the page as permanently broken.
func isBodyStructureParseError(err error) bool {
	return strings.Contains(err.Error(), "body-type-")
}

func (p *Provider) FetchBodies(ctx context.Context, ids []model.MessageID) ([]model.RawMessage, error) {
	p.opMu.Lock()
	defer p.opMu.Unlock()

	// Bodies are fetched by UID within the currently selected mailbox. The
	// engine passes provider message ids that encode the UID; see messageID().
	c, err := p.conn(ctx)
	if err != nil {
		return nil, err
	}
	if p.selected != "INBOX" {
		if _, err := c.Select("INBOX", nil).Wait(); err != nil {
			p.reset()
			return nil, fmt.Errorf("imap select %q: %w", "INBOX", err)
		}
		p.selected = "INBOX"
	}
	var set imap.UIDSet
	idByUID := map[imap.UID]model.MessageID{}
	for _, id := range ids {
		if uid, ok := uidFromMessageID(id); ok {
			set.AddNum(uid)
			idByUID[uid] = id
			continue
		}
		uids, err := p.searchMessageID(c, id)
		if err != nil {
			return nil, err
		}
		for _, uid := range uids {
			set.AddNum(uid)
			idByUID[uid] = id
		}
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
		p.reset()
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

func (p *Provider) Capabilities() provider.Caps {
	return provider.Caps{Idle: true, ServerSearch: true}
}

func (p *Provider) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.c == nil {
		return nil
	}
	err := p.c.Logout().Wait()
	p.c.Close()
	p.c = nil
	return err
}

// Mutations (ApplyFlags/ApplyLabels/Move/Delete) live in mutate.go;
// Send/SaveDraft in send.go; Watch in watch.go.

// compile-time check
var _ provider.Provider = (*Provider)(nil)
