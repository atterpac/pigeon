// Package notify turns freshly synced mail into desktop notifications, applying
// the user's preferences (mode, muted senders, quiet hours) before anything is
// raised. It owns Prefs so both the App backend and the Wails settings service
// share one definition.
package notify

import (
	"fmt"
	"log"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	"github.com/atterpac/pigeon/internal/email"
	"github.com/atterpac/pigeon/internal/model"
	"github.com/atterpac/pigeon/internal/provider"
)

// Prefs controls which new mail raises a desktop notification.
type Prefs struct {
	// Mode is "all", "inbox" (only the inbox), or "none" (suppress entirely).
	Mode string
	// MutedSenders are addresses ("a@x.io") or domains ("x.io") whose mail never
	// notifies. Matched case-insensitively.
	MutedSenders []string
	// QuietHours suppresses notifications within a daily window (local time,
	// "HH:MM"), wrapping past midnight when From > To.
	QuietEnabled bool
	QuietFrom    string
	QuietTo      string
}

// DefaultPrefs notifies for all new mail until the user narrows it.
var DefaultPrefs = Prefs{Mode: ModeAll}

// Notification modes for Prefs.Mode. Stored as plain strings because Prefs is
// shared with the Wails settings bindings, which serialize the field as a string.
const (
	ModeAll   = "all"   // notify for every mailbox
	ModeInbox = "inbox" // notify only for the inbox
	ModeNone  = "none"  // suppress all notifications
)

// Notifier is the slice of the Wails notification service that NewMail depends
// on. Declaring it here (the consumer) keeps NewMail testable with a fake;
// *notifications.NotificationService already satisfies it. Pass a true nil (not a
// nil *NotificationService) to disable notifications.
type Notifier interface {
	SendNotification(notifications.NotificationOptions) error
}

// NewMail raises a desktop notification summarizing freshly synced mail. It only
// counts unread messages (a re-sync of an already-read thread shouldn't ping),
// shows the newest sender/subject, and collapses multiples into a count. User
// preferences (mode, muted senders, quiet hours) can suppress it.
func NewMail(notifs Notifier, mb provider.MailboxRef, msgs []email.Message, prefs Prefs) {
	if notifs == nil {
		return // notifications unavailable on this build/platform
	}
	// An empty or unrecognized Mode falls through to notify-for-all, matching
	// DefaultPrefs; only ModeNone and ModeInbox actively suppress.
	switch prefs.Mode {
	case ModeNone:
		return
	case ModeInbox:
		if !isInboxRef(mb) {
			return
		}
	}
	if quietNow(prefs, time.Now()) {
		return
	}

	muted := normalizeMuted(prefs.MutedSenders)
	unread := make([]email.Message, 0, len(msgs))
	for _, m := range msgs {
		if slices.Contains(m.Flags, model.FlagSeen) {
			continue
		}
		if senderMuted(m, muted) {
			continue
		}
		unread = append(unread, m)
	}
	if len(unread) == 0 {
		return
	}

	if err := notifs.SendNotification(compose(mb, unread)); err != nil {
		log.Printf("send new-mail notification: %v", err)
	}
}

// compose builds the notification for a non-empty set of unread messages,
// headlining the most recent arrival and collapsing multiples into a count.
func compose(mb provider.MailboxRef, unread []email.Message) notifications.NotificationOptions {
	newest := unread[0]
	for _, m := range unread[1:] {
		if m.Date.After(newest.Date) {
			newest = m
		}
	}

	title := senderLabel(newest)
	body := firstNonEmpty(newest.Subject, newest.Snippet)
	if len(unread) > 1 {
		title = fmt.Sprintf("%d new messages", len(unread))
		body = fmt.Sprintf("%s — %s", senderLabel(newest), body)
	}

	return notifications.NotificationOptions{
		ID:    fmt.Sprintf("mail-%s-%d", mb.ID, newest.Date.UnixNano()),
		Title: title,
		Body:  body,
		Data:  map[string]any{"mailbox": string(mb.ID), "messageId": string(newest.ID)},
	}
}

// senderLabel prefers the display name, falling back to the bare address.
func senderLabel(m email.Message) string {
	if len(m.From) == 0 {
		return "New mail"
	}
	from := m.From[0]
	return firstNonEmpty(from.Name, from.Addr, "New mail")
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// isInboxRef reports whether a mailbox ref is the inbox (used for inbox-only
// notification mode). Background sync only follows the inbox today, but this
// keeps the check correct if that broadens.
func isInboxRef(mb provider.MailboxRef) bool {
	return strings.EqualFold(string(mb.ID), string(email.InboxLabel)) ||
		strings.EqualFold(mb.Path, string(email.InboxLabel))
}

// normalizeMuted cleans a muted-senders list once (lower-case, trim, strip a
// leading "@", drop empties) so senderMuted can compare without re-normalizing
// per message. Returns nil for an empty result.
func normalizeMuted(muted []string) []string {
	if len(muted) == 0 {
		return nil
	}
	out := make([]string, 0, len(muted))
	for _, entry := range muted {
		e := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(entry, "@")))
		if e != "" {
			out = append(out, e)
		}
	}
	return out
}

// senderMuted reports whether a message's From matches any muted entry — either
// the full address or its domain. muted must already be normalized via
// normalizeMuted.
func senderMuted(m email.Message, muted []string) bool {
	if len(muted) == 0 || len(m.From) == 0 {
		return false
	}
	addr := strings.ToLower(strings.TrimSpace(m.From[0].Addr))
	if addr == "" {
		return false
	}
	domain := addr
	if at := strings.LastIndex(addr, "@"); at >= 0 {
		domain = addr[at+1:]
	}
	for _, e := range muted {
		if e == addr || e == domain {
			return true
		}
	}
	return false
}

// quietNow reports whether `now` (local time) falls within the quiet-hours
// window. The window wraps past midnight when From > To (e.g. 22:00–07:00).
func quietNow(prefs Prefs, now time.Time) bool {
	if !prefs.QuietEnabled {
		return false
	}
	from, okFrom := parseHHMM(prefs.QuietFrom)
	to, okTo := parseHHMM(prefs.QuietTo)
	if !okFrom || !okTo || from == to {
		return false
	}
	cur := now.Hour()*60 + now.Minute()
	if from < to {
		return cur >= from && cur < to
	}
	// Wraps midnight: quiet if after `from` OR before `to`.
	return cur >= from || cur < to
}

// parseHHMM parses "HH:MM" into minutes-since-midnight.
func parseHHMM(s string) (int, bool) {
	parts := strings.SplitN(strings.TrimSpace(s), ":", 2)
	if len(parts) != 2 {
		return 0, false
	}
	h, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, false
	}
	m, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, false
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, false
	}
	return h*60 + m, true
}
