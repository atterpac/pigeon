package notify

import (
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	"github.com/atterpac/pigeon/internal/email"
	"github.com/atterpac/pigeon/internal/model"
	"github.com/atterpac/pigeon/internal/provider"
)

func at(h, m int) time.Time { return time.Date(2026, 6, 27, h, m, 0, 0, time.Local) }

func TestQuietNow(t *testing.T) {
	cases := []struct {
		name  string
		prefs Prefs
		now   time.Time
		want  bool
	}{
		{"disabled", Prefs{QuietEnabled: false, QuietFrom: "22:00", QuietTo: "07:00"}, at(23, 0), false},
		{"wrap inside late", Prefs{QuietEnabled: true, QuietFrom: "22:00", QuietTo: "07:00"}, at(23, 30), true},
		{"wrap inside early", Prefs{QuietEnabled: true, QuietFrom: "22:00", QuietTo: "07:00"}, at(6, 0), true},
		{"wrap outside", Prefs{QuietEnabled: true, QuietFrom: "22:00", QuietTo: "07:00"}, at(12, 0), false},
		{"same-day inside", Prefs{QuietEnabled: true, QuietFrom: "09:00", QuietTo: "17:00"}, at(13, 0), true},
		{"same-day outside", Prefs{QuietEnabled: true, QuietFrom: "09:00", QuietTo: "17:00"}, at(18, 0), false},
		{"equal bounds noop", Prefs{QuietEnabled: true, QuietFrom: "09:00", QuietTo: "09:00"}, at(9, 0), false},
		{"bad time noop", Prefs{QuietEnabled: true, QuietFrom: "nope", QuietTo: "07:00"}, at(23, 0), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := quietNow(c.prefs, c.now); got != c.want {
				t.Fatalf("quietNow = %v, want %v", got, c.want)
			}
		})
	}
}

func TestSenderMuted(t *testing.T) {
	msg := email.Message{From: []model.Address{{Addr: "News@Mailer.Example.com"}}}
	cases := []struct {
		name  string
		muted []string
		want  bool
	}{
		{"none", nil, false},
		{"exact address", []string{"news@mailer.example.com"}, true},
		{"domain", []string{"mailer.example.com"}, true},
		{"domain with at", []string{"@mailer.example.com"}, true},
		{"other", []string{"someone@else.com"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := senderMuted(msg, normalizeMuted(c.muted)); got != c.want {
				t.Fatalf("senderMuted = %v, want %v", got, c.want)
			}
		})
	}
}

// fakeNotifier records the notifications NewMail would have raised.
type fakeNotifier struct {
	calls []notifications.NotificationOptions
	err   error
}

func (f *fakeNotifier) SendNotification(o notifications.NotificationOptions) error {
	f.calls = append(f.calls, o)
	return f.err
}

func unreadMsg(addr, subj string, when time.Time) email.Message {
	return email.Message{
		ID:      model.MessageID("id-" + addr),
		From:    []model.Address{{Addr: addr}},
		Subject: subj,
		Date:    when,
	}
}

var (
	inboxRef = provider.MailboxRef{ID: email.InboxLabel, Path: string(email.InboxLabel)}
	otherRef = provider.MailboxRef{ID: "Archive", Path: "Archive"}
)

func TestNewMailSuppression(t *testing.T) {
	seen := unreadMsg("a@x.io", "hi", at(10, 0))
	seen.Flags = []model.Flag{model.FlagSeen}

	cases := []struct {
		name  string
		ref   provider.MailboxRef
		msgs  []email.Message
		prefs Prefs
	}{
		{"mode none", inboxRef, []email.Message{unreadMsg("a@x.io", "hi", at(10, 0))}, Prefs{Mode: ModeNone}},
		{"inbox-only on non-inbox", otherRef, []email.Message{unreadMsg("a@x.io", "hi", at(10, 0))}, Prefs{Mode: ModeInbox}},
		{"all seen", inboxRef, []email.Message{seen}, Prefs{Mode: ModeAll}},
		{"all muted", inboxRef, []email.Message{unreadMsg("a@x.io", "hi", at(10, 0))}, Prefs{Mode: ModeAll, MutedSenders: []string{"x.io"}}},
		{"no messages", inboxRef, nil, Prefs{Mode: ModeAll}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := &fakeNotifier{}
			NewMail(f, c.ref, c.msgs, c.prefs)
			if len(f.calls) != 0 {
				t.Fatalf("expected no notification, got %d", len(f.calls))
			}
		})
	}
}

func TestNewMailNilNotifierIsNoop(t *testing.T) {
	// A true nil interface must not panic.
	NewMail(nil, inboxRef, []email.Message{unreadMsg("a@x.io", "hi", at(10, 0))}, Prefs{Mode: ModeAll})
}

func TestNewMailSingle(t *testing.T) {
	f := &fakeNotifier{}
	NewMail(f, inboxRef, []email.Message{unreadMsg("boss@x.io", "Lunch?", at(10, 0))}, Prefs{Mode: ModeAll})
	if len(f.calls) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(f.calls))
	}
	got := f.calls[0]
	if got.Title != "boss@x.io" {
		t.Errorf("title = %q, want sender address", got.Title)
	}
	if got.Body != "Lunch?" {
		t.Errorf("body = %q, want subject", got.Body)
	}
	if got.Data["messageId"] != "id-boss@x.io" {
		t.Errorf("messageId = %v, want newest id", got.Data["messageId"])
	}
}

func TestNewMailHeadlinesNewestAndCounts(t *testing.T) {
	f := &fakeNotifier{}
	msgs := []email.Message{
		unreadMsg("old@x.io", "old", at(9, 0)),
		unreadMsg("new@x.io", "newest", at(11, 0)),
		unreadMsg("mid@x.io", "mid", at(10, 0)),
	}
	NewMail(f, inboxRef, msgs, Prefs{Mode: ModeAll})
	if len(f.calls) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(f.calls))
	}
	got := f.calls[0]
	if got.Title != "3 new messages" {
		t.Errorf("title = %q, want count headline", got.Title)
	}
	if got.Body != "new@x.io — newest" {
		t.Errorf("body = %q, want newest sender/subject", got.Body)
	}
}
