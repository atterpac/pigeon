package mime

import (
	"net/mail"
	"strings"
	"testing"
	"time"

	"github.com/atterpac/email/internal/model"
)

func TestBuildParses(t *testing.T) {
	out := model.Outgoing{
		From:        model.Address{Name: "Me", Addr: "me@x.io"},
		To:          []model.Address{{Addr: "a@y.io"}, {Name: "Bee", Addr: "b@y.io"}},
		Subject:     "Hello é world",
		Text:        "plain body",
		HTML:        "<p>html body</p>",
		InReplyTo:   "<parent@y.io>",
		References:  []string{"<root@y.io>", "<parent@y.io>"},
		Attachments: []model.Outfile{{Filename: "note.txt", ContentType: "text/plain", Content: []byte("hi")}},
	}
	raw, err := Build(out, time.Unix(1_700_000_000, 0), "<gen@x.io>")
	if err != nil {
		t.Fatal(err)
	}

	m, err := mail.ReadMessage(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	h := mail.Header(m.Header)
	if got := h.Get("Subject"); !strings.Contains(got, "Hello") {
		t.Fatalf("subject lost: %q", got)
	}
	if h.Get("In-Reply-To") != "<parent@y.io>" {
		t.Fatalf("in-reply-to: %q", h.Get("In-Reply-To"))
	}
	if h.Get("Message-ID") != "<gen@x.io>" {
		t.Fatalf("message-id: %q", h.Get("Message-ID"))
	}
	to, err := h.AddressList("To")
	if err != nil || len(to) != 2 {
		t.Fatalf("To list: %v %v", to, err)
	}
	if !strings.Contains(h.Get("Content-Type"), "multipart/mixed") {
		t.Fatalf("expected multipart/mixed, got %q", h.Get("Content-Type"))
	}
}

// A text-only message with no attachments is a bare text/plain message — no
// multipart wrapper (matches Build's documented structure).
func TestBuildTextOnlyIsBare(t *testing.T) {
	raw, err := Build(model.Outgoing{
		From:    model.Address{Addr: "me@x.io"},
		To:      []model.Address{{Addr: "a@y.io"}},
		Subject: "plain",
		Text:    "just text",
	}, time.Unix(1_700_000_000, 0), "")
	if err != nil {
		t.Fatal(err)
	}
	m, err := mail.ReadMessage(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	if ct := m.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("expected bare text/plain, got %q", ct)
	}
}

// An HTML-only message must not emit an empty text/plain alternative.
func TestBuildHTMLOnlyOmitsEmptyText(t *testing.T) {
	raw, err := Build(model.Outgoing{
		From:    model.Address{Addr: "me@x.io"},
		To:      []model.Address{{Addr: "a@y.io"}},
		Subject: "html only",
		HTML:    "<p>hi</p>",
	}, time.Unix(1_700_000_000, 0), "")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := Parse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	for _, p := range parsed.Parts {
		if p.ContentType == "text/plain" {
			t.Fatalf("unexpected text/plain part in HTML-only message")
		}
	}
}

// Inline images survive the build → parse round-trip: the part keeps its
// Content-ID (bare) and an inline disposition so receiving clients render it in
// place of the cid: reference.
func TestBuildParsesInlineImage(t *testing.T) {
	out := model.Outgoing{
		From:    model.Address{Addr: "me@x.io"},
		To:      []model.Address{{Addr: "a@y.io"}},
		Subject: "with image",
		Text:    "see image",
		HTML:    `<p>hi <img src="cid:logo123"></p>`,
		Attachments: []model.Outfile{
			{Filename: "logo.png", ContentType: "image/png", Content: []byte("\x89PNG..."), ContentID: "logo123"},
		},
	}
	raw, err := Build(out, time.Unix(1_700_000_000, 0), "<gen@x.io>")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := Parse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var found *model.Part
	for i := range parsed.Parts {
		if parsed.Parts[i].ContentID == "logo123" {
			found = &parsed.Parts[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("inline part with ContentID not found in %d parts", len(parsed.Parts))
	}
	if found.Disposition != "inline" {
		t.Fatalf("expected inline disposition, got %q", found.Disposition)
	}
	if found.ContentType != "image/png" {
		t.Fatalf("expected image/png, got %q", found.ContentType)
	}
}
