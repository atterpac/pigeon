package main

import (
	"context"
	"embed"
	"log"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v3/pkg/application"
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
	defer mailApp.Close()

	// openURL is wired to the system browser once `app` exists (below).
	var openURL func(string) error
	onboarding := newOnboarding(mailApp, func(u string) error { return openURL(u) })

	app := application.New(application.Options{
		Name:        "tempwails",
		Description: "A demo of using raw HTML & CSS",
		Services: []application.Service{
			application.NewService(mailApp.Client),
			application.NewService(onboarding),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})
	openURL = app.Browser.OpenURL
	if err := mailApp.StartConfiguredSyncs(ctx); err != nil {
		log.Printf("start configured sync: %v", err)
	}

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
