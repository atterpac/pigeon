package mime

import (
	"bytes"
	"fmt"
	"html"
	"io"
	"net/mail"
	"net/textproto"
	"slices"
	"strings"

	_ "github.com/emersion/go-message/charset"
	gomail "github.com/emersion/go-message/mail"

	"github.com/atterpac/email/internal/model"
)

// MaxPartBytes bounds the decoded size of any single MIME part that Parse will
// buffer in memory. A part larger than this makes Parse return an error rather
// than risk OOM on a hostile or oversized message; raise it if you legitimately
// need to ingest larger attachments. The store layer is responsible for
// spooling large parts to the blob store (see model.Part.BlobRef).
var MaxPartBytes int64 = 50 << 20 // 50 MiB

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
	// Headers are best-effort: a message gomail can still stream may have a
	// header block net/mail rejects, in which case Headers stays empty.
	headers := map[string][]string{}
	if msg, err := mail.ReadMessage(bytes.NewReader(raw)); err == nil {
		headers = headerMap(msg.Header)
	}
	r, err := gomail.CreateReader(bytes.NewReader(raw))
	if err != nil {
		return Parsed{}, fmt.Errorf("mime parse: %w", err)
	}
	defer func() { _ = r.Close() }()

	out := Parsed{Headers: headers}
	for {
		part, err := r.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return Parsed{}, fmt.Errorf("mime next part: %w", err)
		}
		// Read one byte past the cap so an oversized part is detectable.
		body, err := io.ReadAll(io.LimitReader(part.Body, MaxPartBytes+1))
		if err != nil {
			return Parsed{}, fmt.Errorf("mime read part: %w", err)
		}
		if int64(len(body)) > MaxPartBytes {
			return Parsed{}, fmt.Errorf("mime: part exceeds %d-byte limit", MaxPartBytes)
		}

		p := model.Part{Size: int64(len(body)), Content: body}
		switch h := part.Header.(type) {
		case *gomail.InlineHeader:
			ct, params, _ := h.ContentType()
			p.ContentType, p.Charset, p.Disposition = ct, params["charset"], "inline"
			p.ContentID = bareID(h.Get("Content-Id"))
			if ct == "text/plain" && out.Text == "" {
				out.Text = string(body)
			}
		case *gomail.AttachmentHeader:
			ct, params, _ := h.ContentType()
			p.ContentType, p.Charset, p.Disposition = ct, params["charset"], "attachment"
			p.Filename, _ = h.Filename()
			p.ContentID = bareID(h.Get("Content-Id"))
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
		out[textproto.CanonicalMIMEHeaderKey(k)] = slices.Clone(values)
	}
	return out
}

// stripTags is a crude HTML-to-text for indexing/snippet purposes only. It
// drops markup and the bodies of <script>/<style> elements, decodes HTML
// entities, and collapses whitespace.
func stripTags(s string) string {
	var b strings.Builder
	skip := "" // non-empty while inside <script>/<style>, holds the element name
	for i := 0; i < len(s); {
		lt := strings.IndexByte(s[i:], '<')
		if lt < 0 {
			if skip == "" {
				b.WriteString(s[i:])
			}
			break
		}
		if skip == "" {
			b.WriteString(s[i : i+lt])
		}
		i += lt
		gt := strings.IndexByte(s[i:], '>')
		if gt < 0 {
			break // unterminated tag: drop the remainder
		}
		inner := s[i+1 : i+gt]
		i += gt + 1
		switch name := tagName(inner); {
		case skip != "":
			if strings.HasPrefix(inner, "/") && name == skip {
				skip = ""
			}
		case name == "script" || name == "style":
			skip = name
		}
	}
	return strings.Join(strings.Fields(html.UnescapeString(b.String())), " ")
}

// tagName extracts the lowercased element name from a tag's inner text (the
// bytes between < and >), ignoring a leading slash and any attributes.
func tagName(inner string) string {
	inner = strings.TrimPrefix(strings.TrimSpace(inner), "/")
	if i := strings.IndexAny(inner, " \t\r\n/>"); i >= 0 {
		inner = inner[:i]
	}
	return strings.ToLower(inner)
}
