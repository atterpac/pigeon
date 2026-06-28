package notify

import (
	"testing"
	"time"

	"github.com/atterpac/email/internal/email"
	"github.com/atterpac/email/internal/model"
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
			if got := senderMuted(msg, c.muted); got != c.want {
				t.Fatalf("senderMuted = %v, want %v", got, c.want)
			}
		})
	}
}
