package email

import (
	"regexp"
	"slices"
	"testing"
)

func TestEnsurePrefix(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		subject string
		prefix  string
		want    string
	}{
		{"adds when absent", "Lunch?", "Re: ", "Re: Lunch?"},
		{"keeps when present", "Re: Lunch?", "Re: ", "Re: Lunch?"},
		{"case-insensitive match", "RE: Lunch?", "Re: ", "RE: Lunch?"},
		{"empty subject", "", "Re: ", "Re: "},
		{"shorter than prefix", "Re", "Re: ", "Re: Re"},
		{"fwd prefix", "Report", "Fwd: ", "Fwd: Report"},
		{"does not match inner occurrence", "A Re: thing", "Re: ", "Re: A Re: thing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := ensurePrefix(tt.subject, tt.prefix); got != tt.want {
				t.Errorf("ensurePrefix(%q, %q) = %q, want %q", tt.subject, tt.prefix, got, tt.want)
			}
		})
	}
}

func TestReplyThreading(t *testing.T) {
	t.Parallel()
	self := Address{Addr: "me@example.com"}
	orig := Message{
		Subject:      "Lunch?",
		Thread:       "t1",
		From:         []Address{{Addr: "alice@example.com"}},
		To:           []Address{{Addr: "me@example.com"}, {Addr: "bob@example.com"}},
		Cc:           []Address{{Addr: "carol@example.com"}},
		RFCMessageID: "<msg-2@example.com>",
		References:   []string{"<msg-1@example.com>"},
	}

	t.Run("reply preserves thread and builds References chain", func(t *testing.T) {
		t.Parallel()
		out := Reply(orig, self, false)
		if out.From != self {
			t.Errorf("From = %+v, want %+v", out.From, self)
		}
		if out.Subject != "Re: Lunch?" {
			t.Errorf("Subject = %q, want %q", out.Subject, "Re: Lunch?")
		}
		if out.Thread != orig.Thread {
			t.Errorf("Thread = %q, want %q", out.Thread, orig.Thread)
		}
		if out.InReplyTo != orig.RFCMessageID {
			t.Errorf("InReplyTo = %q, want %q", out.InReplyTo, orig.RFCMessageID)
		}
		wantRefs := []string{"<msg-1@example.com>", "<msg-2@example.com>"}
		if !slices.Equal(out.References, wantRefs) {
			t.Errorf("References = %v, want %v", out.References, wantRefs)
		}
		// reply-to-sender targets the original From.
		if !slices.Equal(out.To, orig.From) {
			t.Errorf("To = %v, want %v", out.To, orig.From)
		}
		if len(out.Cc) != 0 {
			t.Errorf("Cc = %v, want empty on reply (not reply-all)", out.Cc)
		}
	})

	t.Run("does not mutate the original References", func(t *testing.T) {
		t.Parallel()
		orig := Message{RFCMessageID: "<b>", References: []string{"<a>"}}
		_ = Reply(orig, self, false)
		if !slices.Equal(orig.References, []string{"<a>"}) {
			t.Errorf("Reply mutated orig.References to %v", orig.References)
		}
	})

	t.Run("reply-all includes To/Cc minus self", func(t *testing.T) {
		t.Parallel()
		out := Reply(orig, self, true)
		// self must be excluded; bob (To) and carol (Cc) included.
		var got []string
		for _, a := range out.Cc {
			got = append(got, a.Addr)
		}
		want := []string{"bob@example.com", "carol@example.com"}
		if !slices.Equal(got, want) {
			t.Errorf("reply-all Cc = %v, want %v", got, want)
		}
		for _, a := range out.Cc {
			if a.Addr == self.Addr {
				t.Errorf("reply-all Cc must not contain self %q", self.Addr)
			}
		}
	})
}

func TestForward(t *testing.T) {
	t.Parallel()
	self := Address{Addr: "me@example.com"}
	orig := Message{
		Subject:      "Report",
		Thread:       "t1",
		RFCMessageID: "<msg-1@example.com>",
		References:   []string{"<msg-0@example.com>"},
	}
	out := Forward(orig, self)
	if out.From != self {
		t.Errorf("From = %+v, want %+v", out.From, self)
	}
	if out.Subject != "Fwd: Report" {
		t.Errorf("Subject = %q, want %q", out.Subject, "Fwd: Report")
	}
	// Forward starts a new conversation: no recipients or threading carried over.
	if out.Thread != "" {
		t.Errorf("Thread = %q, want empty", out.Thread)
	}
	if out.InReplyTo != "" || len(out.References) != 0 {
		t.Errorf("Forward should not set threading: InReplyTo=%q References=%v", out.InReplyTo, out.References)
	}
	if len(out.To) != 0 || len(out.Cc) != 0 {
		t.Errorf("Forward should not set recipients: To=%v Cc=%v", out.To, out.Cc)
	}
}

var messageIDRe = regexp.MustCompile(`^<[0-9a-f]{32}@(.+)>$`)

func TestGenMessageID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		from       string
		wantDomain string
	}{
		{"extracts domain", "alice@example.com", "example.com"},
		{"last @ wins", "weird@name@host.io", "host.io"},
		{"no domain falls back to localhost", "nobody", "localhost"},
		{"empty falls back to localhost", "", "localhost"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			id := genMessageID(tt.from)
			m := messageIDRe.FindStringSubmatch(id)
			if m == nil {
				t.Fatalf("genMessageID(%q) = %q, not a valid <hex@domain> Message-ID", tt.from, id)
			}
			if m[1] != tt.wantDomain {
				t.Errorf("domain = %q, want %q", m[1], tt.wantDomain)
			}
		})
	}

	// IDs must be unique across calls (random local part).
	seen := make(map[string]bool)
	for range 100 {
		id := genMessageID("a@b.com")
		if seen[id] {
			t.Fatalf("genMessageID produced a duplicate: %q", id)
		}
		seen[id] = true
	}
}
