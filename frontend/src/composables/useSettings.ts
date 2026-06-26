// Reactive, localStorage-persisted user settings (singleton). Drives live theme
// application and exposes the R2 view presets. The locked defaults match the
// shipped UI; other preset values persist as switch points for incremental ports.
import { reactive, watch } from 'vue'
import { applyTheme, getTheme } from '../theme/themes'

export interface Settings {
  theme: string
  compose: 'centered' | 'docked' | 'side' | 'fullscreen' | 'minimal' | 'split'
  sidebarStyle: 'flat' | 'cards' | 'compact' | 'outline' | 'header' | 'airy'
  navLayout: 'grouped' | 'plain' | 'icons' | 'counts' | 'rail' | 'accounts'
  settingsLayout: 'sidebar' | 'tabs' | 'scroll' | 'cards' | 'palette' | 'fullscreen'
  density: 'comfortable' | 'compact'
  vimMode: boolean
  relativenumber: boolean
  hiddenMailboxIds: string[]
}

const STORAGE_KEY = 'mail.settings'
const defaults: Settings = {
  theme: 'tokyonight-night',
  compose: 'centered',
  sidebarStyle: 'flat',
  navLayout: 'icons',
  settingsLayout: 'sidebar',
  density: 'compact',
  vimMode: true,
  relativenumber: true,
  hiddenMailboxIds: [],
}

function load(): Partial<Settings> {
  try { return JSON.parse(localStorage.getItem(STORAGE_KEY) || '{}') } catch { return {} }
}

const settings = reactive<Settings>({ ...defaults, ...load() })

function applyDensity() { document.documentElement.dataset.density = settings.density }

// Apply visual settings immediately, then keep them in sync + persisted.
applyTheme(getTheme(settings.theme))
applyDensity()
watch(() => settings.theme, (id) => applyTheme(getTheme(id)))
watch(() => settings.density, applyDensity)
watch(settings, () => localStorage.setItem(STORAGE_KEY, JSON.stringify(settings)), { deep: true })

export function useSettings(): Settings {
  return settings
}
