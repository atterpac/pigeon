package imap

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"

	"github.com/atterpac/email/internal/model"
	"github.com/atterpac/email/internal/provider"
)

// messageID builds the SDK-internal id for an IMAP message. IMAP has no stable
// cross-mailbox id, so we key on (account, UID): "imap:<account>:<uid>".
func messageID(acct model.AccountID, uid imap.UID) model.MessageID {
	return model.MessageID(fmt.Sprintf("imap:%s:%d", acct, uid))
}

// uidFromMessageID extracts the UID from an id produced by messageID.
func uidFromMessageID(id model.MessageID) (imap.UID, bool) {
	s := string(id)
	i := strings.LastIndexByte(s, ':')
	if i < 0 {
		return 0, false
	}
	n, err := strconv.ParseUint(s[i+1:], 10, 32)
	if err != nil {
		return 0, false
	}
	return imap.UID(n), true
}

func hasAttr(attrs []imap.MailboxAttr, want imap.MailboxAttr) bool {
	return slices.Contains(attrs, want)
}

// roleFromAttrs maps IMAP SPECIAL-USE attributes (and common names) to a role.
func roleFromAttrs(name string, attrs []imap.MailboxAttr) model.Role {
	for _, a := range attrs {
		switch a {
		case imap.MailboxAttrSent:
			return model.RoleSent
		case imap.MailboxAttrDrafts:
			return model.RoleDrafts
		case imap.MailboxAttrTrash:
			return model.RoleTrash
		case imap.MailboxAttrJunk:
			return model.RoleSpam
		case imap.MailboxAttrArchive:
			return model.RoleArchive
		}
	}
	if strings.EqualFold(name, "INBOX") {
		return model.RoleInbox
	}
	return model.RoleNone
}

func toAddresses(in []imap.Address) []model.Address {
	if len(in) == 0 {
		return nil
	}
	out := make([]model.Address, 0, len(in))
	for _, a := range in {
		// IMAP group syntax markers carry a nil mailbox or host; skip them rather
		// than emit a malformed "@host" / "local@" address.
		if a.Mailbox == "" || a.Host == "" {
			continue
		}
		out = append(out, model.Address{
			Name: a.Name,
			Addr: a.Mailbox + "@" + a.Host,
		})
	}
	return out
}

func toFlags(in []imap.Flag) []model.Flag {
	if len(in) == 0 {
		return nil
	}
	out := make([]model.Flag, 0, len(in))
	for _, f := range in {
		out = append(out, model.Flag(f))
	}
	return out
}

// hasAttachment reports whether any body part is dispositioned as an attachment.
func hasAttachment(bs imap.BodyStructure) bool {
	if bs == nil {
		return false
	}
	found := false
	bs.Walk(func(_ []int, part imap.BodyStructure) bool {
		if d := part.Disposition(); d != nil && strings.EqualFold(d.Value, "attachment") {
			found = true
			return false
		}
		return true
	})
	return found
}

// toMessages converts a batch of IMAP fetch buffers into domain envelopes.
func toMessages(acct model.AccountID, mb provider.MailboxRef, msgs []*imapclient.FetchMessageBuffer) []model.Message {
	out := make([]model.Message, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, toMessage(acct, mb, m))
	}
	return out
}

// toMessage converts an IMAP fetch buffer into a domain Message envelope.
func toMessage(acct model.AccountID, mb provider.MailboxRef, m *imapclient.FetchMessageBuffer) model.Message {
	msg := model.Message{
		ID:      messageID(acct, m.UID),
		Account: acct,
		Flags:   toFlags(m.Flags),
		Labels:  []model.LabelID{mb.ID},
	}
	if e := m.Envelope; e != nil {
		msg.Subject = e.Subject
		msg.From = toAddresses(e.From)
		msg.To = toAddresses(e.To)
		msg.Cc = toAddresses(e.Cc)
		msg.Bcc = toAddresses(e.Bcc)
		msg.Date = e.Date
		// Canonical id is the RFC Message-ID so the same message appearing in
		// multiple Gmail folders (INBOX + All Mail) collapses to one store row
		// with merged labels. Fall back to (account,UID) when absent.
		if e.MessageID != "" {
			msg.ID = model.MessageID(e.MessageID)
		}
		// Seed threading: replies attach to their parent; roots thread on self.
		// The engine refines this with References later.
		switch {
		case len(e.InReplyTo) > 0 && e.InReplyTo[0] != "":
			msg.Thread = model.ThreadID(e.InReplyTo[0])
		case e.MessageID != "":
			msg.Thread = model.ThreadID(e.MessageID)
		}
		// Header metadata for reply/forward. IMAP ENVELOPE exposes Message-ID and
		// In-Reply-To but not the full References chain; In-Reply-To is the best
		// available approximation here.
		msg.RFCMessageID = e.MessageID
		msg.References = nil
		for _, r := range e.InReplyTo {
			if r != "" {
				msg.References = append(msg.References, r)
			}
		}
	}
	msg.HasAttachments = hasAttachment(m.BodyStructure)
	return msg
}
