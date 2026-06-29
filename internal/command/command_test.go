package command

import (
	"strings"
	"testing"
)

func TestResolveEndpoint(t *testing.T) {
	// Clear inherited overrides so the host machine's env can't skew results.
	for _, k := range []string{"EMAIL_IMAP_HOST", "EMAIL_IMAP_PORT", "EMAIL_SMTP_HOST", "EMAIL_SMTP_PORT"} {
		t.Setenv(k, "")
	}

	tests := []struct {
		name    string
		account string
		env     map[string]string
		want    imapEndpoint
		wantErr bool
	}{
		{
			name:    "known gmail domain",
			account: "user@gmail.com",
			want:    imapEndpoint{host: "imap.gmail.com", smtpHost: "smtp.gmail.com", port: 993, smtpPort: 587},
		},
		{
			name:    "googlemail alias",
			account: "user@googlemail.com",
			want:    imapEndpoint{host: "imap.gmail.com", smtpHost: "smtp.gmail.com", port: 993, smtpPort: 587},
		},
		{
			name:    "domain case-insensitive",
			account: "user@GMAIL.COM",
			want:    imapEndpoint{host: "imap.gmail.com", smtpHost: "smtp.gmail.com", port: 993, smtpPort: 587},
		},
		{
			name:    "unknown domain without env is an error",
			account: "user@example.org",
			wantErr: true,
		},
		{
			name:    "env host fills in defaults for the rest",
			account: "user@example.org",
			env:     map[string]string{"EMAIL_IMAP_HOST": "mail.example.org"},
			want:    imapEndpoint{host: "mail.example.org", smtpHost: "mail.example.org", port: 993, smtpPort: 587},
		},
		{
			name:    "env overrides every field",
			account: "user@gmail.com",
			env: map[string]string{
				"EMAIL_IMAP_HOST": "imap.local", "EMAIL_IMAP_PORT": "1993",
				"EMAIL_SMTP_HOST": "smtp.local", "EMAIL_SMTP_PORT": "2587",
			},
			want: imapEndpoint{host: "imap.local", smtpHost: "smtp.local", port: 1993, smtpPort: 2587},
		},
		{
			name:    "non-numeric port falls back to the map value",
			account: "user@gmail.com",
			env:     map[string]string{"EMAIL_IMAP_PORT": "notaport"},
			want:    imapEndpoint{host: "imap.gmail.com", smtpHost: "smtp.gmail.com", port: 993, smtpPort: 587},
		},
		{
			name:    "account without @ and no env is an error",
			account: "localuser",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			got, err := resolveEndpoint(tt.account)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("resolveEndpoint(%q) = %+v, want error", tt.account, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveEndpoint(%q) unexpected error: %v", tt.account, err)
			}
			if got != tt.want {
				t.Errorf("resolveEndpoint(%q) = %+v, want %+v", tt.account, got, tt.want)
			}
		})
	}
}

func TestSplitCSV(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"INBOX", []string{"INBOX"}},
		{"INBOX,SENT", []string{"INBOX", "SENT"}},
		{" INBOX , SENT ", []string{"INBOX", "SENT"}},
		{"INBOX,,SENT,", []string{"INBOX", "SENT"}},
		{" , , ", nil},
	}
	for _, tt := range tests {
		got := splitCSV(tt.in)
		if !equalStrings(got, tt.want) {
			t.Errorf("splitCSV(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestParseRecipients(t *testing.T) {
	got := parseRecipients("a@x.com, b@y.com ,, c@z.com")
	want := []string{"a@x.com", "b@y.com", "c@z.com"}
	if len(got) != len(want) {
		t.Fatalf("parseRecipients length = %d, want %d (%v)", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].Addr != w {
			t.Errorf("recipient[%d] = %q, want %q", i, got[i].Addr, w)
		}
	}
	if parseRecipients("") != nil && len(parseRecipients("")) != 0 {
		t.Errorf("parseRecipients(\"\") = %v, want empty", parseRecipients(""))
	}
}

func TestArgOr(t *testing.T) {
	args := []string{"a", "b"}
	if got := argOr(args, 0, "def"); got != "a" {
		t.Errorf("argOr index 0 = %q, want a", got)
	}
	if got := argOr(args, 5, "def"); got != "def" {
		t.Errorf("argOr out-of-range = %q, want def", got)
	}
}

func TestAtoiOr(t *testing.T) {
	if got := atoiOr("42", 7); got != 42 {
		t.Errorf("atoiOr(\"42\") = %d, want 42", got)
	}
	if got := atoiOr("nope", 7); got != 7 {
		t.Errorf("atoiOr(\"nope\") = %d, want 7 (default)", got)
	}
	if got := atoiOr("", 7); got != 7 {
		t.Errorf("atoiOr(\"\") = %d, want 7 (default)", got)
	}
}

func TestGenMessageID(t *testing.T) {
	id, err := genMessageID("user@example.com")
	if err != nil {
		t.Fatalf("genMessageID error: %v", err)
	}
	if !strings.HasPrefix(id, "<") || !strings.HasSuffix(id, "@example.com>") {
		t.Errorf("genMessageID = %q, want <hex@example.com>", id)
	}

	noAt, err := genMessageID("localuser")
	if err != nil {
		t.Fatalf("genMessageID(no @) error: %v", err)
	}
	if !strings.HasSuffix(noAt, "@localhost>") {
		t.Errorf("genMessageID(no @) = %q, want @localhost domain", noAt)
	}

	// IDs must be unique across calls.
	other, _ := genMessageID("user@example.com")
	if id == other {
		t.Errorf("genMessageID produced duplicate IDs: %q", id)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
