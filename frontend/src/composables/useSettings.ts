// Reactive, localStorage-persisted user settings. Phase 4 only needs a few
// keys; phase 5 (Settings modal) extends this with the full view-preset set and
// live theme application. Kept as a singleton reactive object.
import { reactive, watch } from 'vue'

export interface Settings {
  theme: string
  relativenumber: boolean
  vimMode: boolean
}

const STORAGE_KEY = 'mail.settings'
const defaults: Settings = { theme: 'tokyonight-night', relativenumber: true, vimMode: true }

function load(): Partial<Settings> {
  try { return JSON.parse(localStorage.getItem(STORAGE_KEY) || '{}') } catch { return {} }
}

const settings = reactive<Settings>({ ...defaults, ...load() })
watch(settings, () => localStorage.setItem(STORAGE_KEY, JSON.stringify(settings)), { deep: true })

export function useSettings(): Settings {
  return settings
}
