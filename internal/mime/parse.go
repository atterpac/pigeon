package mime

import (
	"bytes"
	"fmt"
	"io"
	"net/mail"
	"net/textproto"
	"strings"

	_ "github.com/emersion/go-message/charset"
	gomail "github.com/emersion/go-message/mail"

	"github.com/atterpac/email/internal/model"
)

// Parsed is the decoded content of a raw message.
type Parsed struct {
	Headers map[string][]string // RFC 5322 message headers
	Parts   []model.Part        // inline parts and attachments, in order
	Text    string              // best-effort plain-text body (for FTS / snippet)
}

// Parse decodes a raw RFC 5322 message into inline parts and attachments. Both
// inline bodies (text/plain, text/html) and attachments (any disposition with a
// filename) are returned as model.Part, distinguished by Disposition.
func Parse(raw []byte) (Parsed, error) {
	headers := map[string][]string{}
	if msg, err := mail.ReadMessage(bytes.NewReader(raw)); err == nil {
		headers = headerMap(msg.Header)
	}
	r, err := gomail.CreateReader(bytes.NewReader(raw))
	if err != nil {
		return Parsed{}, fmt.Errorf("mime parse: %w", err)
	}
	defer r.Close()

	out := Parsed{Headers: headers}
	for {
		part, err := r.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return Parsed{}, fmt.Errorf("mime next part: %w", err)
		}
		body, err := io.ReadAll(part.Body)
		if err != nil {
			return Parsed{}, fmt.Errorf("mime read part: %w", err)
		}

		p := model.Part{Size: int64(len(body)), Content: body}
		switch h := part.Header.(type) {
		case *gomail.InlineHeader:
			ct, params, _ := h.ContentType()
			p.ContentType, p.Charset, p.Disposition = ct, params["charset"], "inline"
			if ct == "text/plain" && out.Text == "" {
				out.Text = string(body)
			}
		case *gomail.AttachmentHeader:
			ct, params, _ := h.ContentType()
			p.ContentType, p.Charset, p.Disposition = ct, params["charset"], "attachment"
			p.Filename, _ = h.Filename()
		}
		out.Parts = append(out.Parts, p)
	}
	// Fall back to stripped HTML if there was no text/plain part.
	if out.Text == "" {
		for _, p := range out.Parts {
			if p.ContentType == "text/html" {
				out.Text = stripTags(string(p.Content))
				break
			}
		}
	}
	return out, nil
}

func headerMap(h mail.Header) map[string][]string {
	out := make(map[string][]string, len(h))
	for k, values := range h {
		out[textproto.CanonicalMIMEHeaderKey(k)] = append([]string(nil), values...)
	}
	return out
}

// stripTags is a crude HTML-to-text for indexing/snippet purposes only.
func stripTags(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}
