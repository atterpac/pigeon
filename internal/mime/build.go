package mime

import (
	"bytes"
	"fmt"
	"net/mail"
	"strings"
	"time"

	gomail "github.com/emersion/go-message/mail"

	"github.com/atterpac/email/internal/model"
)

// Build renders an Outgoing message to RFC 5322 bytes. Structure:
//
//   - text only            → text/plain
//   - text + html          → multipart/alternative
//   - with attachments     → multipart/mixed wrapping the above
//
// now is injected so callers control the Date header (and tests stay
// deterministic); pass time.Now() in production.
func Build(m model.Outgoing, now time.Time, messageID string) ([]byte, error) {
	var h gomail.Header
	h.SetDate(now)
	h.SetSubject(m.Subject)
	if messageID != "" {
		h.SetMessageID(bareID(messageID))
	}
	h.SetAddressList("From", addrs([]model.Address{m.From}))
	if len(m.To) > 0 {
		h.SetAddressList("To", addrs(m.To))
	}
	if len(m.Cc) > 0 {
		h.SetAddressList("Cc", addrs(m.Cc))
	}
	if len(m.Bcc) > 0 {
		h.SetAddressList("Bcc", addrs(m.Bcc))
	}
	if m.InReplyTo != "" {
		h.SetMsgIDList("In-Reply-To", []string{bareID(m.InReplyTo)})
	}
	if len(m.References) > 0 {
		h.SetMsgIDList("References", bareIDs(m.References))
	}

	var buf bytes.Buffer
	w, err := gomail.CreateWriter(&buf, h)
	if err != nil {
		return nil, fmt.Errorf("mime: create writer: %w", err)
	}
	if err := writeBody(w, m); err != nil {
		return nil, err
	}
	for _, att := range m.Attachments {
		write := writeAttachment
		if att.ContentID != "" {
			write = writeInlineImage // referenced from the HTML body via cid:
		}
		if err := write(w, att); err != nil {
			return nil, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("mime: close: %w", err)
	}
	return buf.Bytes(), nil
}

func writeBody(w *gomail.Writer, m model.Outgoing) error {
	iw, err := w.CreateInline()
	if err != nil {
		return err
	}
	parts := []struct {
		ctype string
		body  string
	}{}
	parts = append(parts, struct {
		ctype string
		body  string
	}{"text/plain", m.Text})
	if m.HTML != "" {
		parts = append(parts, struct {
			ctype string
			body  string
		}{"text/html", m.HTML})
	}
	for _, p := range parts {
		var ih gomail.InlineHeader
		ih.SetContentType(p.ctype, map[string]string{"charset": "utf-8"})
		pw, err := iw.CreatePart(ih)
		if err != nil {
			return err
		}
		if _, err := pw.Write([]byte(p.body)); err != nil {
			return err
		}
		if err := pw.Close(); err != nil {
			return err
		}
	}
	return iw.Close()
}

func writeAttachment(w *gomail.Writer, att model.Outfile) error {
	var ah gomail.AttachmentHeader
	ct := att.ContentType
	if ct == "" {
		ct = "application/octet-stream"
	}
	ah.SetContentType(ct, nil)
	ah.SetFilename(att.Filename)
	pw, err := w.CreateAttachment(ah)
	if err != nil {
		return err
	}
	if _, err := pw.Write(att.Content); err != nil {
		return err
	}
	return pw.Close()
}

// writeInlineImage writes an inline part carrying a Content-ID so the HTML body
// can reference it via cid:. It uses the inline-part path (Content-Disposition:
// inline, base64) so receiving clients render it in place rather than listing it
// as a downloadable attachment. CreateAttachment can't be used here — it forces
// Content-Disposition: attachment.
func writeInlineImage(w *gomail.Writer, att model.Outfile) error {
	var ih gomail.InlineHeader
	ct := att.ContentType
	if ct == "" {
		ct = "application/octet-stream"
	}
	if att.Filename != "" {
		ih.SetContentType(ct, map[string]string{"name": att.Filename})
	} else {
		ih.SetContentType(ct, nil)
	}
	ih.Set("Content-Id", "<"+bareID(att.ContentID)+">")
	pw, err := w.CreateSingleInline(ih)
	if err != nil {
		return err
	}
	if _, err := pw.Write(att.Content); err != nil {
		return err
	}
	return pw.Close()
}

// bareID strips surrounding angle brackets; SetMsgIDList re-adds them.
func bareID(id string) string {
	return strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(id), "<"), ">")
}

func bareIDs(ids []string) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = bareID(id)
	}
	return out
}

func addrs(in []model.Address) []*gomail.Address {
	out := make([]*gomail.Address, 0, len(in))
	for _, a := range in {
		out = append(out, &mail.Address{Name: a.Name, Address: a.Addr})
	}
	return out
}
