// Command email runs the Pigeon desktop client.
package main

import (
	"context"
	"embed"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	"github.com/atterpac/pigeon/internal/desktop"
	"github.com/atterpac/pigeon/internal/desktop/notify"
	"github.com/atterpac/pigeon/internal/desktop/onboard"
	"github.com/atterpac/pigeon/internal/desktop/service"
	"github.com/atterpac/pigeon/internal/email"
	"github.com/atterpac/pigeon/internal/paths"
	"github.com/atterpac/pigeon/internal/provider"
)

//go:embed all:dist
var assets embed.FS

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dbPath, err := paths.DBPath()
	if err != nil {
		log.Fatalf("resolve db path: %v", err)
	}

	mailApp, err := desktop.NewApp(ctx, dbPath)
	if err != nil {
		log.Fatalf("init email app: %v", err)
	}
	// mailApp is closed by desktop.Lifecycle.ServiceShutdown, not a defer here, so
	// the store and sync loops shut down through the Wails service lifecycle.

	notifs := notifications.New()
	app := buildApp(mailApp, notifs)
	wireEvents(app, mailApp, notifs)
	newMainWindow(app)

	if err := app.Run(); err != nil {
		log.Fatalf("application exited with error: %v", err)
	}
}

// buildApp constructs the Wails application and registers the frontend-facing
// services.
func buildApp(mailApp *desktop.App, notifs *notifications.NotificationService) *application.App {
	onboarding := onboard.New(mailApp)

	// The email SDK client is not bound directly; instead small facade services
	// expose an intentional, domain-grouped slice of it to the frontend.
	svc := service.NewServices(mailApp.Client)

	return application.New(application.Options{
		Name:        "Pigeon",
		Description: "A keyboard focused email client",
		Services: []application.Service{
			application.NewService(onboarding),
			// Desktop notifications service, exposed to the frontend so the test
			// panel in Settings → Notifications can drive it.
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
}

// wireEvents connects notification responses and newly polled mail to frontend
// events. Called before sync starts so initial loops use the handler.
func wireEvents(app *application.App, mailApp *desktop.App, notifs *notifications.NotificationService) {
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
	// frontend to refresh.
	mailApp.SetNewMailHandler(func(acct email.AccountID, mb provider.MailboxRef, msgs []email.Message) {
		notify.NewMail(notifs, mb, msgs, mailApp.NotifyPrefs())
		app.Event.Emit("mail:new", map[string]any{
			"account": string(acct),
			"mailbox": string(mb.ID),
			"count":   len(msgs),
		})
	})

	// Background sync is started by desktop.Lifecycle.ServiceStartup once the app is up.
}

// newMainWindow opens the primary application window.
func newMainWindow(app *application.App) {
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
}
