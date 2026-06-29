package desktop

import (
	"context"
	"log"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// storeChangeEvent is the frontend event name carrying changefeed hints. The
// payload is best-effort ("something changed, refetch"), mirroring the
// in-process changefeed's semantics — a UI should coalesce and refetch rather
// than treat each event as an authoritative log entry.
const storeChangeEvent = "store:change"

// Lifecycle ties the email app's runtime to the Wails application lifecycle. It
// implements ServiceStartup/ServiceShutdown so background sync starts only once
// the app is up, the store changefeed is bridged to frontend events, and the
// client (sync loops, providers, SQLite) is closed cleanly on shutdown — rather
// than relying on a lone defer in main.
type Lifecycle struct {
	app *App

	mu        sync.Mutex
	cancelSub func() // unsubscribes the changefeed bridge; nil until started
}

// NewLifecycle builds the lifecycle service over the given app.
func NewLifecycle(app *App) *Lifecycle { return &Lifecycle{app: app} }

// ServiceStartup bridges the store changefeed to the frontend and resumes sync
// for already-configured accounts. Called by Wails during App.Run, after the
// new-mail/notification handlers are wired in main.
func (l *Lifecycle) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	l.startChangefeedBridge()
	if err := l.app.StartConfiguredSyncs(ctx); err != nil {
		log.Printf("start configured sync: %v", err)
	}
	return nil
}

// ServiceShutdown stops the changefeed bridge and closes the client. Wails runs
// this after user shutdown hooks, in reverse registration order.
func (l *Lifecycle) ServiceShutdown() error {
	l.mu.Lock()
	if l.cancelSub != nil {
		l.cancelSub()
		l.cancelSub = nil
	}
	l.mu.Unlock()
	return l.app.Close()
}

// startChangefeedBridge subscribes to the client's changefeed and re-emits each
// store mutation as a Wails event so the frontend can refetch the affected view.
func (l *Lifecycle) startChangefeedBridge() {
	events, cancel := l.app.Client.Events()
	l.mu.Lock()
	l.cancelSub = cancel
	l.mu.Unlock()

	go func() {
		for e := range events {
			app := application.Get()
			if app == nil {
				continue // app torn down; drop the hint
			}
			app.Event.Emit(storeChangeEvent, map[string]any{
				"account": string(e.Account),
				"kind":    string(e.Kind),
				"ids":     e.IDs,
			})
		}
	}()
}
