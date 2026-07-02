package desktop

import (
	"testing"

	"github.com/atterpac/pigeon/internal/email"
	"github.com/atterpac/pigeon/internal/provider/imap"
)

func TestApplyIMAPEndpoints(t *testing.T) {
	cases := []struct {
		name string
		acct email.Account
		want imap.Config
	}{
		{
			name: "explicit per-account servers win",
			acct: email.Account{
				Email:    "user@custom.example",
				IMAPHost: "mail.custom.example", IMAPPort: 1993,
				SMTPHost: "smtp.custom.example", SMTPPort: 1587,
			},
			want: imap.Config{
				Host: "mail.custom.example", Port: 1993,
				SMTPHost: "smtp.custom.example", SMTPPort: 1587,
			},
		},
		{
			name: "known domain resolves to built-in endpoint",
			acct: email.Account{Email: "person@gmail.com"},
			want: imap.Config{
				Host: "imap.gmail.com", Port: 993,
				SMTPHost: "smtp.gmail.com", SMTPPort: 587,
			},
		},
		{
			name: "known domain is case-insensitive",
			acct: email.Account{Email: "Person@GoogleMail.com"},
			want: imap.Config{
				Host: "imap.gmail.com", Port: 993,
				SMTPHost: "smtp.gmail.com", SMTPPort: 587,
			},
		},
		{
			name: "unknown domain leaves config untouched",
			acct: email.Account{Email: "user@unheard-of.example"},
			want: imap.Config{},
		},
		{
			name: "malformed address (no @) leaves config untouched",
			acct: email.Account{Email: "not-an-address"},
			want: imap.Config{},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var cfg imap.Config
			applyIMAPEndpoints(&cfg, c.acct)
			if cfg != c.want {
				t.Fatalf("applyIMAPEndpoints = %+v, want %+v", cfg, c.want)
			}
		})
	}
}

func TestSyncMailbox(t *testing.T) {
	cases := []struct {
		name      string
		mailboxes []email.Mailbox
		want      email.LabelID
	}{
		{
			name: "prefers the mailbox tagged with the inbox role",
			mailboxes: []email.Mailbox{
				{ID: "Archive"},
				{ID: "All Mail", Role: email.RoleInbox},
				{ID: "INBOX"},
			},
			want: "All Mail",
		},
		{
			name: "falls back to an INBOX-named mailbox when no role is set",
			mailboxes: []email.Mailbox{
				{ID: "Archive"},
				{ID: "inbox"}, // case-insensitive match
			},
			want: "inbox",
		},
		{
			name:      "defaults to the canonical inbox label when nothing matches",
			mailboxes: []email.Mailbox{{ID: "Archive"}, {ID: "Sent"}},
			want:      email.InboxLabel,
		},
		{
			name:      "empty topology defaults to the canonical inbox label",
			mailboxes: nil,
			want:      email.InboxLabel,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := syncMailbox(c.mailboxes); got != c.want {
				t.Fatalf("syncMailbox = %q, want %q", got, c.want)
			}
		})
	}
}
