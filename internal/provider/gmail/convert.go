package gmail

import (
	"encoding/base64"
	"net/mail"
	"net/textproto"
	"strings"
	"time"

	gmailapi "google.golang.org/api/gmail/v1"

	"github.com/atterpac/email/internal/classify"
	"github.com/atterpac/email/internal/model"
)

// messageID namespaces the Gmail message id: "gmail:<id>".
func messageID(id string) model.MessageID { return model.MessageID("gmail:" + id) }

// gmailID extracts the raw Gmail id from a namespaced MessageID.
func gmailID(id model.MessageID) string { return strings.TrimPrefix(string(id), "gmail:") }

// roleFromLabel maps Gmail system label ids to semantic roles.
func roleFromLabel(id string) model.Role {
	switch id {
	case "INBOX":
		return model.RoleInbox
	case "SENT":
		return model.RoleSent
	case "DRAFT":
		return model.RoleDrafts
	case "TRASH":
		return model.RoleTrash
	case "SPAM":
		return model.RoleSpam
	default:
		return model.RoleNone
	}
}

func header(hs []*gmailapi.MessagePartHeader, name string) string {
	for _, h := range hs {
		if strings.EqualFold(h.Name, name) {
			return h.Value
		}
	}
	return ""
}

func parseAddrs(s string) []model.Address {
	if s == "" {
		return nil
	}
	list, err := mail.ParseAddressList(s)
	if err != nil {
		return []model.Address{{Addr: s}} // keep raw on parse failure
	}
	out := make([]model.Address, len(list))
	for i, a := range list {
		out[i] = model.Address{Name: a.Name, Addr: a.Address}
	}
	return out
}

// toMessage builds a domain envelope from a metadata-format Gmail message.
func toMessage(acct model.AccountID, m *gmailapi.Message) model.Message {
	var hs []*gmailapi.MessagePartHeader
	if m.Payload != nil {
		hs = m.Payload.Headers
	}
	date := time.UnixMilli(m.InternalDate)
	if d := header(hs, "Date"); d != "" {
		if t, err := mail.ParseDate(d); err == nil {
			date = t
		}
	}
	labels := make([]model.LabelID, len(m.LabelIds))
	for i, l := range m.LabelIds {
		labels[i] = model.LabelID(l)
	}
	msg := model.Message{
		ID:           messageID(m.Id),
		Thread:       model.ThreadID(m.ThreadId), // native Gmail threading
		Account:      acct,
		Subject:      header(hs, "Subject"),
		From:         parseAddrs(header(hs, "From")),
		To:           parseAddrs(header(hs, "To")),
		Cc:           parseAddrs(header(hs, "Cc")),
		Bcc:          parseAddrs(header(hs, "Bcc")),
		Date:         date,
		Snippet:      m.Snippet,
		Flags:        flagsFromLabels(m.LabelIds),
		Labels:       labels,
		RFCMessageID: header(hs, "Message-ID"),
		References:   parseRefs(header(hs, "References"), header(hs, "In-Reply-To")),
	}
	msg.Category = classify.Classify(classify.Input{
		Subject: msg.Subject,
		Snippet: msg.Snippet,
		From:    msg.From,
		To:      msg.To,
		Cc:      msg.Cc,
		Labels:  msg.Labels,
		Headers: gmailHeaders(hs),
	})
	return msg
}

func gmailHeaders(hs []*gmailapi.MessagePartHeader) map[string][]string {
	out := make(map[string][]string, len(hs))
	for _, h := range hs {
		key := textproto.CanonicalMIMEHeaderKey(h.Name)
		out[key] = append(out[key], h.Value)
	}
	return out
}

// parseRefs builds the References chain from the References header (falling back
// to In-Reply-To), de-duplicated and in order.
func parseRefs(references, inReplyTo string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		for _, id := range strings.Fields(s) {
			if id != "" && !seen[id] {
				seen[id] = true
				out = append(out, id)
			}
		}
	}
	add(references)
	add(inReplyTo)
	return out
}

// flagsFromLabels derives IMAP-style flags from Gmail labels: a message is
// "seen" unless it carries UNREAD.
func flagsFromLabels(labels []string) []model.Flag {
	var flags []model.Flag
	unread, starred := false, false
	for _, l := range labels {
		switch l {
		case "UNREAD":
			unread = true
		case "STARRED":
			starred = true
		}
	}
	if !unread {
		flags = append(flags, model.FlagSeen)
	}
	if starred {
		flags = append(flags, model.FlagFlagged)
	}
	return flags
}

// decodeRaw decodes Gmail's web-safe base64 message body, tolerating missing
// padding (Gmail omits it).
func decodeRaw(s string) ([]byte, error) {
	if b, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(s, "=")); err == nil {
		return b, nil
	}
	return base64.URLEncoding.DecodeString(s)
}
