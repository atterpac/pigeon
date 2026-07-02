package mime

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	gomail "github.com/emersion/go-message/mail"

	"github.com/atterpac/pigeon/internal/model"
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
	switch {
	case len(m.Attachments) == 0 && m.HTML == "":
		// text only → a bare top-level text/plain part (no multipart wrapper).
		h.SetContentType("text/plain", map[string]string{"charset": "utf-8"})
		pw, err := gomail.CreateSingleInlineWriter(&buf, h)
		if err != nil {
			return nil, fmt.Errorf("mime: create writer: %w", err)
		}
		if _, err := pw.Write([]byte(m.Text)); err != nil {
			return nil, err
		}
		if err := pw.Close(); err != nil {
			return nil, fmt.Errorf("mime: close: %w", err)
		}
	case len(m.Attachments) == 0:
		// text + html, no attachments → multipart/alternative.
		iw, err := gomail.CreateInlineWriter(&buf, h)
		if err != nil {
			return nil, fmt.Errorf("mime: create writer: %w", err)
		}
		if err := writeAltParts(iw, m); err != nil {
			return nil, err
		}
	default:
		// attachments present → multipart/mixed wrapping the body parts.
		w, err := gomail.CreateWriter(&buf, h)
		if err != nil {
			return nil, fmt.Errorf("mime: create writer: %w", err)
		}
		if err := writeMixedBody(w, m); err != nil {
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
	}
	return buf.Bytes(), nil
}

type bodyPart struct {
	ctype string
	body  string
}

// altParts returns the inline body parts in client-preference order (least to
// most preferred). text/plain is emitted whenever there's body text, or as the
// required fallback when the body would otherwise be empty; text/html follows
// when present.
func altParts(m model.Outgoing) []bodyPart {
	var parts []bodyPart
	if m.Text != "" || m.HTML == "" {
		parts = append(parts, bodyPart{"text/plain", m.Text})
	}
	if m.HTML != "" {
		parts = append(parts, bodyPart{"text/html", m.HTML})
	}
	return parts
}

// writeAltParts writes the text/plain (+ optional text/html) parts into an
// alternative section and closes it.
func writeAltParts(iw *gomail.InlineWriter, m model.Outgoing) error {
	for _, p := range altParts(m) {
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

// writeMixedBody writes the body inside a multipart/mixed container: a single
// text/plain part when there's no HTML alternative, or a multipart/alternative
// section when there is.
func writeMixedBody(w *gomail.Writer, m model.Outgoing) error {
	if m.HTML == "" {
		var ih gomail.InlineHeader
		ih.SetContentType("text/plain", map[string]string{"charset": "utf-8"})
		pw, err := w.CreateSingleInline(ih)
		if err != nil {
			return err
		}
		if _, err := pw.Write([]byte(m.Text)); err != nil {
			return err
		}
		return pw.Close()
	}
	iw, err := w.CreateInline()
	if err != nil {
		return err
	}
	return writeAltParts(iw, m)
}

func writeAttachment(w *gomail.Writer, att model.Outfile) error {
	var ah gomail.AttachmentHeader
	ah.SetContentType(ctOrOctet(att.ContentType), nil)
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
	ct := ctOrOctet(att.ContentType)
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

// ctOrOctet defaults an empty content type to application/octet-stream.
func ctOrOctet(ct string) string {
	if ct == "" {
		return "application/octet-stream"
	}
	return ct
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
		out = append(out, &gomail.Address{Name: a.Name, Address: a.Addr})
	}
	return out
}
