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
