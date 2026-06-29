package main

import (
	"context"
	"embed"
	"log"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	"github.com/atterpac/email/internal/desktop"
	"github.com/atterpac/email/internal/desktop/notify"
	"github.com/atterpac/email/internal/desktop/onboard"
	"github.com/atterpac/email/internal/desktop/service"
	"github.com/atterpac/email/internal/email"
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

	mailApp, err := desktop.NewApp(ctx, filepath.Join(dir, "mail.db"))
	if err != nil {
		log.Fatalf("init email app: %v", err)
	}
	// mailApp is closed by desktop.Lifecycle.ServiceShutdown, not a defer here, so
	// the store and sync loops shut down through the Wails service lifecycle.

	onboarding := onboard.New(mailApp)

	// Desktop notifications service, exposed to the frontend so the test panel
	// in Settings → Notifications can drive it.
	notifs := notifications.New()

	// The email SDK client is not bound directly; instead small facade services
	// expose an intentional, domain-grouped slice of it to the frontend.
	svc := service.NewServices(mailApp.Client)

	app := application.New(application.Options{
		Name:        "Pigeon",
		Description: "A keyboard focused email client",
		Services: []application.Service{
			application.NewService(onboarding),
			application.NewService(notifs),
			application.NewService(desktop.NewSyncSettings(mailApp)),
			application.NewService(svc.Mailboxes),
			application.NewService(svc.Messages),
			application.NewService(svc.Mutations),
			application.NewService(svc.Compose),
			application.NewService(svc.Snooze),
			application.NewService(svc.Contacts),
			application.NewService(desktop.NewLifecycle(mailApp)),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

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
		notify.NewMail(notifs, mb, msgs, mailApp.NotifyPrefs())
		app.Event.Emit("mail:new", map[string]any{
			"account": string(acct),
			"mailbox": string(mb.ID),
			"count":   len(msgs),
		})
	})

	// Background sync is started by desktop.Lifecycle.ServiceStartup once the app is up.

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:  "Main",
		Width:  1280,
		Height: 800,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 50,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar: application.MacTitleBar{
				AppearsTransparent: true,
				HideTitle:          true,
				FullSizeContent:    true,
			},
		},
		BackgroundColour: application.NewRGB(6, 7, 15),
		URL:              "/",
	})

	if err := app.Run(); err != nil {
		log.Fatalf("application exited with error: %v", err)
	}
}
