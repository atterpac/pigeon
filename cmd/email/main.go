package main

import (
	"context"
	"embed"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	"github.com/atterpac/email/internal/email"
	"github.com/atterpac/email/internal/model"
	"github.com/atterpac/email/internal/provider"
)

//go:embed all:dist
var assets embed.FS

func main() {
	ctx := context.Background()

	dir, err := os.UserConfigDir()
	if err != nil {
		log.Fatalf("locate config dir: %v", err)
	}
	dir = filepath.Join(dir, "email")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		log.Fatalf("create config dir: %v", err)
	}

	mailApp, err := newApp(ctx,
		filepath.Join(dir, "mail.db"),
		filepath.Join(dir, "google-client.json"),
	)
	if err != nil {
		log.Fatalf("init email app: %v", err)
	}
	// mailApp is closed by Lifecycle.ServiceShutdown, not a defer here, so the
	// store and sync loops shut down through the Wails service lifecycle.

	// openURL is wired to the system browser once `app` exists (below).
	var openURL func(string) error
	onboarding := newOnboarding(mailApp, func(u string) error { return openURL(u) })

	// Desktop notifications service, exposed to the frontend so the test panel
	// in Settings → Notifications can drive it.
	notifs := notifications.New()

	// The email SDK client is not bound directly; instead small facade services
	// expose an intentional, domain-grouped slice of it to the frontend.
	mailboxes, messages, mutations, compose, snooze := newServices(mailApp.Client)

	app := application.New(application.Options{
		Name:        "tempwails",
		Description: "A demo of using raw HTML & CSS",
		Services: []application.Service{
			application.NewService(onboarding),
			application.NewService(notifs),
			application.NewService(&SyncSettings{app: mailApp}),
			application.NewService(mailboxes),
			application.NewService(messages),
			application.NewService(mutations),
			application.NewService(compose),
			application.NewService(snooze),
			application.NewService(&Lifecycle{app: mailApp}),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})
	openURL = app.Browser.OpenURL

	// Forward notification action responses (button taps, replies) to the
	// frontend so the test panel can show what came back.
	notifs.OnNotificationResponse(func(result notifications.NotificationResult) {
		if result.Error != nil {
			log.Printf("notification response error: %v", result.Error)
			return
		}
		app.Event.Emit("notification:action", result.Response)
	})

	// Announce newly polled mail as a desktop notification, and nudge the
	// frontend to refresh. Wired before sync starts so initial loops use it.
	mailApp.SetNewMailHandler(func(acct email.AccountID, mb provider.MailboxRef, msgs []email.Message) {
		notifyNewMail(notifs, mb, msgs)
		app.Event.Emit("mail:new", map[string]any{
			"account": string(acct),
			"mailbox": string(mb.ID),
			"count":   len(msgs),
		})
	})

	// Background sync is started by Lifecycle.ServiceStartup once the app is up.

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:  "Window 1",
		Width:  1000,
		Height: 618,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 50,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		BackgroundColour: application.NewRGB(6, 7, 15),
		URL:              "/",
	})

	if err := app.Run(); err != nil {
		log.Fatalf("application exited with error: %v", err)
	}
}

// notifyNewMail raises a desktop notification summarizing freshly synced mail.
// It only counts unread messages (a re-sync of an already-read thread shouldn't
// ping), shows the newest sender/subject, and collapses multiples into a count.
func notifyNewMail(notifs *notifications.NotificationService, mb provider.MailboxRef, msgs []email.Message) {
	var unread []email.Message
	for _, m := range msgs {
		if !slices.Contains(m.Flags, model.FlagSeen) {
			unread = append(unread, m)
		}
	}
	if len(unread) == 0 {
		return
	}

	// Newest first so the headline message is the most recent arrival.
	newest := unread[0]
	for _, m := range unread[1:] {
		if m.Date.After(newest.Date) {
			newest = m
		}
	}

	title := senderLabel(newest)
	body := newest.Subject
	if body == "" {
		body = newest.Snippet
	}
	if len(unread) > 1 {
		title = fmt.Sprintf("%d new messages", len(unread))
		body = fmt.Sprintf("%s — %s", senderLabel(newest), firstNonEmpty(newest.Subject, newest.Snippet))
	}

	if err := notifs.SendNotification(notifications.NotificationOptions{
		ID:    fmt.Sprintf("mail-%s-%d", mb.ID, newest.Date.UnixNano()),
		Title: title,
		Body:  body,
		Data:  map[string]any{"mailbox": string(mb.ID), "messageId": string(newest.ID)},
	}); err != nil {
		log.Printf("send new-mail notification: %v", err)
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
