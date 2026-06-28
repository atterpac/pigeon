package desktop

import (
	"context"

	"github.com/atterpac/email/internal/desktop/notify"
)

// SyncSettings is the thin Wails service that exposes runtime sync controls to
// the frontend, keeping App's other methods off the binding surface.
type SyncSettings struct{ app *App }

// NewSyncSettings builds the service over the given app.
func NewSyncSettings(app *App) *SyncSettings { return &SyncSettings{app: app} }

// PollIntervalSeconds reports the current background poll interval, in seconds.
func (s *SyncSettings) PollIntervalSeconds() int { return s.app.PollIntervalSeconds() }

// SetPollInterval changes the background poll interval (seconds) and restarts
// the sync loops. Values below the minimum are clamped server-side.
func (s *SyncSettings) SetPollInterval(ctx context.Context, seconds int) error {
	return s.app.SetPollInterval(ctx, seconds)
}

// NotifyPrefs reports the current notification preferences.
func (s *SyncSettings) NotifyPrefs() notify.Prefs { return s.app.NotifyPrefs() }

// SetNotifyPrefs pushes the user's notification preferences to the backend; the
// new-mail handler consults them before raising a desktop notification.
func (s *SyncSettings) SetNotifyPrefs(prefs notify.Prefs) { s.app.SetNotifyPrefs(prefs) }
