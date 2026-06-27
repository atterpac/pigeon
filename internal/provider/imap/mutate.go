package imap

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"

	"github.com/atterpac/email/internal/model"
	"github.com/atterpac/email/internal/provider"
)

// resolveUIDSet maps store message ids to UIDs in the currently selected
// mailbox. Store ids are canonically RFC Message-IDs (so the same mail across
// folders dedups to one row), which carry no UID — so we resolve those via
// UID SEARCH HEADER Message-ID. Ids still in "imap:<account>:<uid>" form are
// parsed directly. Without this, mutations get RFC ids, resolve to zero UIDs,
// and silently no-op.
func (p *Provider) resolveUIDSet(c *imapclient.Client, ids []model.MessageID) (imap.UIDSet, error) {
	var set imap.UIDSet
	for _, id := range ids {
		if uid, ok := uidFromMessageID(id); ok {
			set.AddNum(uid)
			continue
		}
		uids, err := p.searchMessageID(c, id)
		if err != nil {
			return nil, err
		}
		if len(uids) == 0 {
			slog.Debug("imap: message-id not found in selected mailbox", "selected", p.selected, "id", id)
		}
		for _, uid := range uids {
			set.AddNum(uid)
		}
	}
	return set, nil
}

// ensureSelected selects INBOX for UID-based mutations. IMAP mutations act on
// the currently selected mailbox and the SDK message id does not encode its
// source mailbox — and background folder pulls leave other mailboxes selected.
// In this app the desktop acts on INBOX messages, so we explicitly (re)select
// INBOX rather than trusting whatever was last selected (which would make a
// COPY/MOVE/STORE silently target the wrong mailbox's UIDs).
func (p *Provider) ensureSelected(ctx context.Context) error {
	c, err := p.conn(ctx)
	if err != nil {
		return err
	}
	if p.selected == "INBOX" {
		return nil
	}
	if _, err := c.Select("INBOX", nil).Wait(); err != nil {
		return err
	}
	p.selected = "INBOX"
	return nil
}

// ApplyFlags issues STORE +FLAGS / -FLAGS on the selected mailbox.
func (p *Provider) ApplyFlags(ctx context.Context, ids []model.MessageID, add, remove []model.Flag) error {
	p.opMu.Lock()
	defer p.opMu.Unlock()

	if err := p.ensureSelected(ctx); err != nil {
		return err
	}
	c, _ := p.conn(ctx)
	set, err := p.resolveUIDSet(c, ids)
	if err != nil {
		return err
	}
	if len(set) == 0 {
		return nil
	}
	if len(add) > 0 {
		if err := c.Store(set, &imap.StoreFlags{Op: imap.StoreFlagsAdd, Silent: true, Flags: toIMAPFlags(add)}, nil).Close(); err != nil {
			return fmt.Errorf("imap store +flags: %w", err)
		}
	}
	if len(remove) > 0 {
		if err := c.Store(set, &imap.StoreFlags{Op: imap.StoreFlagsDel, Silent: true, Flags: toIMAPFlags(remove)}, nil).Close(); err != nil {
			return fmt.Errorf("imap store -flags: %w", err)
		}
	}
	return nil
}

// Move relocates messages to dst (IMAP MOVE).
func (p *Provider) Move(ctx context.Context, ids []model.MessageID, dst provider.MailboxRef) error {
	p.opMu.Lock()
	defer p.opMu.Unlock()

	if err := p.ensureSelected(ctx); err != nil {
		return err
	}
	c, _ := p.conn(ctx)
	set, err := p.resolveUIDSet(c, ids)
	if err != nil {
		return err
	}
	// IMAP UID operations act on the *currently selected* mailbox (INBOX here).
	slog.Debug("imap.move", "selected", p.selected, "dst", dst.Path, "ids", len(ids), "uids", len(set))
	if len(set) == 0 {
		slog.Debug("imap.move: no message ids resolved to UIDs", "ids", ids)
		return nil
	}
	if _, err := c.Move(set, dst.Path).Wait(); err != nil {
		return fmt.Errorf("imap move: %w", err)
	}
	return nil
}

// Delete flags messages \Deleted and expunges them.
func (p *Provider) Delete(ctx context.Context, ids []model.MessageID) error {
	p.opMu.Lock()
	defer p.opMu.Unlock()

	if err := p.ensureSelected(ctx); err != nil {
		return err
	}
	c, _ := p.conn(ctx)
	set, err := p.resolveUIDSet(c, ids)
	if err != nil {
		return err
	}
	if len(set) == 0 {
		return nil
	}
	if err := c.Store(set, &imap.StoreFlags{Op: imap.StoreFlagsAdd, Silent: true, Flags: []imap.Flag{imap.FlagDeleted}}, nil).Close(); err != nil {
		return fmt.Errorf("imap store \\Deleted: %w", err)
	}
	if err := c.UIDExpunge(set).Close(); err != nil {
		return fmt.Errorf("imap expunge: %w", err)
	}
	return nil
}

// ApplyLabels maps the app's label model onto IMAP folders: IMAP has no labels,
// but in this app a "label" is just a mailbox, so adding label X means COPY the
// message into folder X (the original stays in place). Removing INBOX (the
// archive gesture) expunges from the source; other removals have no IMAP analog
// and are ignored. COPY/STORE act on the currently selected mailbox, so the
// caller must have the source mailbox selected (defaults to INBOX).
func (p *Provider) ApplyLabels(ctx context.Context, ids []model.MessageID, add, remove []model.LabelID) error {
	p.opMu.Lock()
	defer p.opMu.Unlock()

	if err := p.ensureSelected(ctx); err != nil {
		return err
	}
	c, _ := p.conn(ctx)
	set, err := p.resolveUIDSet(c, ids)
	if err != nil {
		return err
	}
	slog.Debug("imap.applyLabels", "selected", p.selected, "add", add, "remove", remove,
		"ids", len(ids), "uids", len(set))
	if len(set) == 0 {
		return nil
	}
	for _, lbl := range add {
		if string(lbl) == p.selected {
			continue // already in this mailbox
		}
		if _, err := c.Copy(set, string(lbl)).Wait(); err != nil {
			return fmt.Errorf("imap copy to %q: %w", lbl, err)
		}
	}
	// Removing INBOX is the archive gesture: drop the source copy.
	for _, lbl := range remove {
		if string(lbl) != string(p.selected) {
			continue
		}
		if err := c.Store(set, &imap.StoreFlags{Op: imap.StoreFlagsAdd, Silent: true, Flags: []imap.Flag{imap.FlagDeleted}}, nil).Close(); err != nil {
			return fmt.Errorf("imap store \\Deleted: %w", err)
		}
		if err := c.UIDExpunge(set).Close(); err != nil {
			return fmt.Errorf("imap expunge: %w", err)
		}
	}
	return nil
}

// CreateMailbox issues IMAP CREATE and returns the new mailbox.
func (p *Provider) CreateMailbox(ctx context.Context, name string) (model.Mailbox, error) {
	p.opMu.Lock()
	defer p.opMu.Unlock()

	c, err := p.conn(ctx)
	if err != nil {
		return model.Mailbox{}, err
	}
	if err := c.Create(name, nil).Wait(); err != nil {
		return model.Mailbox{}, fmt.Errorf("imap create mailbox: %w", err)
	}
	return p.mailbox(name), nil
}

// RenameMailbox issues IMAP RENAME and returns the renamed mailbox.
func (p *Provider) RenameMailbox(ctx context.Context, mb provider.MailboxRef, newName string) (model.Mailbox, error) {
	p.opMu.Lock()
	defer p.opMu.Unlock()

	c, err := p.conn(ctx)
	if err != nil {
		return model.Mailbox{}, err
	}
	old := mailboxPath(mb)
	if err := c.Rename(old, newName, nil).Wait(); err != nil {
		return model.Mailbox{}, fmt.Errorf("imap rename mailbox: %w", err)
	}
	if p.selected == old {
		p.selected = ""
	}
	return p.mailbox(newName), nil
}

// DeleteMailbox issues IMAP DELETE.
func (p *Provider) DeleteMailbox(ctx context.Context, mb provider.MailboxRef) error {
	p.opMu.Lock()
	defer p.opMu.Unlock()

	c, err := p.conn(ctx)
	if err != nil {
		return err
	}
	path := mailboxPath(mb)
	if p.selected == path {
		p.selected = ""
	}
	if err := c.Delete(path).Wait(); err != nil {
		return fmt.Errorf("imap delete mailbox: %w", err)
	}
	return nil
}

// mailbox normalizes a server mailbox path into the SDK model.
func (p *Provider) mailbox(path string) model.Mailbox {
	return model.Mailbox{
		ID:      model.LabelID(path),
		Account: p.cfg.Account,
		Name:    path,
		Path:    path,
		Role:    model.RoleNone,
	}
}

func mailboxPath(mb provider.MailboxRef) string {
	if mb.Path != "" {
		return mb.Path
	}
	return string(mb.ID)
}

func toIMAPFlags(fs []model.Flag) []imap.Flag {
	out := make([]imap.Flag, len(fs))
	for i, f := range fs {
		out[i] = imap.Flag(f)
	}
	return out
}
