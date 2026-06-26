package imap

import (
	"context"
	"fmt"

	"github.com/emersion/go-imap/v2"

	"github.com/atterpac/email/internal/model"
	"github.com/atterpac/email/internal/provider"
)

// uidSet builds a UID set from SDK message ids (imap:<account>:<uid>).
func uidSet(ids []model.MessageID) imap.UIDSet {
	var set imap.UIDSet
	for _, id := range ids {
		if uid, ok := uidFromMessageID(id); ok {
			set.AddNum(uid)
		}
	}
	return set
}

// ensureSelected makes sure a mailbox is selected for UID-based mutations.
// IMAP mutations act on the currently selected mailbox; the SDK message id does
// not encode the mailbox, so callers must have synced/selected the relevant
// mailbox first. Defaults to INBOX.
func (p *Provider) ensureSelected(ctx context.Context) error {
	c, err := p.conn(ctx)
	if err != nil {
		return err
	}
	if p.selected != "" {
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
	set := uidSet(ids)
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
	set := uidSet(ids)
	if len(set) == 0 {
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
	set := uidSet(ids)
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

// ApplyLabels is unsupported on generic IMAP (no label model); Gmail label
// changes go through the Gmail REST provider.
func (p *Provider) ApplyLabels(context.Context, []model.MessageID, []model.LabelID, []model.LabelID) error {
	return fmt.Errorf("imap: labels unsupported (use the Gmail provider)")
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
