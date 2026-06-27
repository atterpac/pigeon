// Thin wrapper over the Wails SyncSettings service. Calls are no-ops (resolved
// gracefully) when running outside the desktop app, e.g. the browser preview,
// so callers don't need to branch on the runtime.
import { SyncSettings } from '../bindings/github.com/atterpac/email/cmd/email'

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
