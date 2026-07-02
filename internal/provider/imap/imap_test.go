package imap

import (
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"

	"github.com/atterpac/pigeon/internal/model"
	"github.com/atterpac/pigeon/internal/provider"
)

func TestMessageIDRoundTrip(t *testing.T) {
	t.Parallel()
	id := messageID("acct-1", 4242)
	if want := model.MessageID("imap:acct-1:4242"); id != want {
		t.Fatalf("messageID = %q, want %q", id, want)
	}
	uid, ok := uidFromMessageID(id)
	if !ok || uid != 4242 {
		t.Fatalf("uidFromMessageID(%q) = %d, %v; want 4242, true", id, uid, ok)
	}
}

func TestUIDFromMessageID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		id      model.MessageID
		wantUID imap.UID
		wantOK  bool
	}{
		{"canonical", "imap:acct:17", 17, true},
		{"rfc message-id", "<abc@example.com>", 0, false},
		{"no colon", "12345", 0, false},
		{"non-numeric tail", "imap:acct:nope", 0, false},
		{"overflows uint32", "imap:acct:4294967296", 0, false},
		{"max uint32", "imap:acct:4294967295", 4294967295, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			uid, ok := uidFromMessageID(tc.id)
			if uid != tc.wantUID || ok != tc.wantOK {
				t.Fatalf("uidFromMessageID(%q) = %d, %v; want %d, %v", tc.id, uid, ok, tc.wantUID, tc.wantOK)
			}
		})
	}
}

func TestCursorRoundTrip(t *testing.T) {
	t.Parallel()
	in := cursor{UIDValidity: 99, LastUID: 1500, ModSeq: 70000}
	out, ok := decodeCursor(encodeCursor(in))
	if !ok {
		t.Fatal("decodeCursor returned ok=false for a freshly encoded cursor")
	}
	if out != in {
		t.Fatalf("round-trip = %+v, want %+v", out, in)
	}
}

func TestDecodeCursor(t *testing.T) {
	t.Parallel()
	if _, ok := decodeCursor(nil); ok {
		t.Error("decodeCursor(nil) ok = true, want false")
	}
	if _, ok := decodeCursor(&provider.Cursor{}); ok {
		t.Error("decodeCursor(empty) ok = true, want false")
	}
	if _, ok := decodeCursor(&provider.Cursor{Bytes: []byte("{not json")}); ok {
		t.Error("decodeCursor(garbage) ok = true, want false")
	}
}

func TestRoleFromAttrs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		mb    string
		attrs []imap.MailboxAttr
		want  model.Role
	}{
		{"special-use sent", "[Gmail]/Sent Mail", []imap.MailboxAttr{imap.MailboxAttrSent}, model.RoleSent},
		{"special-use trash", "Bin", []imap.MailboxAttr{imap.MailboxAttrTrash}, model.RoleTrash},
		{"special-use junk", "Spam", []imap.MailboxAttr{imap.MailboxAttrJunk}, model.RoleSpam},
		{"inbox by name", "INBOX", nil, model.RoleInbox},
		{"inbox case-insensitive", "inbox", nil, model.RoleInbox},
		{"unknown", "Receipts", nil, model.RoleNone},
		{"attr wins over name", "INBOX", []imap.MailboxAttr{imap.MailboxAttrArchive}, model.RoleArchive},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := roleFromAttrs(tc.mb, tc.attrs); got != tc.want {
				t.Fatalf("roleFromAttrs(%q, %v) = %v, want %v", tc.mb, tc.attrs, got, tc.want)
			}
		})
	}
}

func TestToAddressesSkipsMalformed(t *testing.T) {
	t.Parallel()
	in := []imap.Address{
		{Name: "Real", Mailbox: "real", Host: "example.com"},
		{Name: "Group", Mailbox: "group", Host: ""}, // group marker
		{Name: "NoLocal", Mailbox: "", Host: "example.com"},
	}
	out := toAddresses(in)
	if len(out) != 1 {
		t.Fatalf("got %d addresses, want 1: %+v", len(out), out)
	}
	if out[0] != (model.Address{Name: "Real", Addr: "real@example.com"}) {
		t.Fatalf("addr = %+v", out[0])
	}
	if toAddresses(nil) != nil {
		t.Error("toAddresses(nil) should be nil")
	}
}

func TestToMessageThreading(t *testing.T) {
	t.Parallel()
	mb := provider.MailboxRef{ID: model.LabelID("INBOX"), Path: "INBOX"}

	t.Run("reply threads on in-reply-to", func(t *testing.T) {
		t.Parallel()
		m := &imapclient.FetchMessageBuffer{
			UID: 7,
			Envelope: &imap.Envelope{
				Subject:   "Re: hi",
				MessageID: "<child@x>",
				InReplyTo: []string{"<parent@x>"},
				From:      []imap.Address{{Mailbox: "a", Host: "x"}},
			},
		}
		got := toMessage("acct", mb, m)
		// Canonical id is the RFC Message-ID when present.
		if got.ID != "<child@x>" {
			t.Errorf("ID = %q, want <child@x>", got.ID)
		}
		if got.Thread != model.ThreadID("<parent@x>") {
			t.Errorf("Thread = %q, want <parent@x>", got.Thread)
		}
		if got.RFCMessageID != "<child@x>" {
			t.Errorf("RFCMessageID = %q", got.RFCMessageID)
		}
		if len(got.References) != 1 || got.References[0] != "<parent@x>" {
			t.Errorf("References = %v, want [<parent@x>]", got.References)
		}
		if want := []model.LabelID{mb.ID}; len(got.Labels) != 1 || got.Labels[0] != want[0] {
			t.Errorf("Labels = %v, want %v", got.Labels, want)
		}
	})

	t.Run("root threads on self", func(t *testing.T) {
		t.Parallel()
		m := &imapclient.FetchMessageBuffer{
			UID:      8,
			Envelope: &imap.Envelope{MessageID: "<root@x>"},
		}
		got := toMessage("acct", mb, m)
		if got.Thread != model.ThreadID("<root@x>") {
			t.Errorf("Thread = %q, want <root@x>", got.Thread)
		}
	})

	t.Run("no message-id falls back to account,uid id", func(t *testing.T) {
		t.Parallel()
		m := &imapclient.FetchMessageBuffer{UID: 9, Envelope: &imap.Envelope{Subject: "x"}}
		got := toMessage("acct", mb, m)
		if got.ID != messageID("acct", 9) {
			t.Errorf("ID = %q, want %q", got.ID, messageID("acct", 9))
		}
		if got.Thread != "" {
			t.Errorf("Thread = %q, want empty", got.Thread)
		}
	})

	t.Run("nil envelope is safe", func(t *testing.T) {
		t.Parallel()
		got := toMessage("acct", mb, &imapclient.FetchMessageBuffer{UID: 10})
		if got.ID != messageID("acct", 10) {
			t.Errorf("ID = %q", got.ID)
		}
	})
}

func TestBuildSearchCriteria(t *testing.T) {
	t.Parallel()
	mustDate := func(s string) time.Time {
		tm, err := time.Parse("2006-01-02", s)
		if err != nil {
			t.Fatalf("bad test date %q: %v", s, err)
		}
		return tm
	}

	t.Run("empty query is nil", func(t *testing.T) {
		t.Parallel()
		if buildSearchCriteria("") != nil {
			t.Error("empty query should yield nil criteria")
		}
		if buildSearchCriteria("   ") != nil {
			t.Error("whitespace query should yield nil criteria")
		}
	})

	t.Run("header operators", func(t *testing.T) {
		t.Parallel()
		c := buildSearchCriteria("from:alice subject:report")
		if c == nil || len(c.Header) != 2 {
			t.Fatalf("want 2 header criteria, got %+v", c)
		}
		if c.Header[0].Key != "From" || c.Header[0].Value != "alice" {
			t.Errorf("header[0] = %+v", c.Header[0])
		}
		if c.Header[1].Key != "Subject" || c.Header[1].Value != "report" {
			t.Errorf("header[1] = %+v", c.Header[1])
		}
	})

	t.Run("is:unread sets NotFlag Seen", func(t *testing.T) {
		t.Parallel()
		c := buildSearchCriteria("is:unread")
		if c == nil || len(c.NotFlag) != 1 || c.NotFlag[0] != imap.FlagSeen {
			t.Fatalf("want NotFlag=[Seen], got %+v", c)
		}
	})

	t.Run("is:starred aliases is:flagged", func(t *testing.T) {
		t.Parallel()
		for _, q := range []string{"is:starred", "is:flagged"} {
			c := buildSearchCriteria(q)
			if c == nil || len(c.Flag) != 1 || c.Flag[0] != imap.FlagFlagged {
				t.Fatalf("%q: want Flag=[Flagged], got %+v", q, c)
			}
		}
	})

	t.Run("date operators", func(t *testing.T) {
		t.Parallel()
		c := buildSearchCriteria("after:2024-01-01 before:2024-12-31")
		if c == nil {
			t.Fatal("nil criteria")
		}
		if !c.Since.Equal(mustDate("2024-01-01")) {
			t.Errorf("Since = %v", c.Since)
		}
		if !c.Before.Equal(mustDate("2024-12-31")) {
			t.Errorf("Before = %v", c.Before)
		}
	})

	t.Run("bad date is ignored, not an error", func(t *testing.T) {
		t.Parallel()
		// "after:" alone with a bad date contributes nothing; with no other terms
		// the whole query is empty → nil.
		if c := buildSearchCriteria("after:nope"); c != nil {
			t.Errorf("bad-date-only query should be nil, got %+v", c)
		}
	})

	t.Run("free text falls through to TEXT", func(t *testing.T) {
		t.Parallel()
		c := buildSearchCriteria("invoice from:bob quarterly")
		if c == nil {
			t.Fatal("nil criteria")
		}
		if len(c.Text) != 2 || c.Text[0] != "invoice" || c.Text[1] != "quarterly" {
			t.Errorf("Text = %v, want [invoice quarterly]", c.Text)
		}
		if len(c.Header) != 1 {
			t.Errorf("Header = %+v, want one From term", c.Header)
		}
	})

	t.Run("has:attachment is recognized but unexpressible", func(t *testing.T) {
		t.Parallel()
		// Only token is has:attachment, which contributes nothing → nil.
		if c := buildSearchCriteria("has:attachment"); c != nil {
			t.Errorf("has:attachment-only query should be nil, got %+v", c)
		}
	})
}
