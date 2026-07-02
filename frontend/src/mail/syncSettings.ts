// Thin wrapper over the Wails SyncSettings service. Calls are no-ops (resolved
// gracefully) when running outside the desktop app, e.g. the browser preview,
// so callers don't need to branch on the runtime.
import { SyncSettings } from '../bindings/github.com/atterpac/pigeon/internal/desktop'
import { Prefs as NotifyPrefs } from '../bindings/github.com/atterpac/pigeon/internal/desktop/notify/models'

export type NotifyPrefsInput = {
  mode: string
  mutedSenders: string[]
  quietHours: { enabled: boolean; from: string; to: string }
}

// applyNotifyPrefs pushes the user's notification preferences to the backend's
// new-mail handler. No-op (resolved gracefully) outside the desktop runtime.
export async function applyNotifyPrefs(prefs: NotifyPrefsInput): Promise<void> {
  try {
    await SyncSettings.SetNotifyPrefs(new NotifyPrefs({
      Mode: prefs.mode,
      MutedSenders: [...prefs.mutedSenders],
      QuietEnabled: prefs.quietHours.enabled,
      QuietFrom: prefs.quietHours.from,
      QuietTo: prefs.quietHours.to,
    }))
  } catch (err) {
    console.warn('applyNotifyPrefs failed', err)
  }
}

// applyPollInterval pushes the background mail-poll interval (seconds) to the
// backend, which restarts the sync loops. The backend clamps to its minimum.
export async function applyPollInterval(seconds: number): Promise<void> {
  try {
    await SyncSettings.SetPollInterval(Math.round(seconds))
  } catch (err) {
    // Non-fatal: likely no Wails runtime (browser preview) or no accounts yet.
    console.warn('applyPollInterval failed', err)
  }
}

// pollInterval reads the backend's current interval (seconds), or null when the
// runtime is unavailable.
export async function pollInterval(): Promise<number | null> {
  try {
    return await SyncSettings.PollIntervalSeconds()
  } catch {
    return null
  }
}
