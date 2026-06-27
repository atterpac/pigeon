// Reactive, localStorage-persisted user settings (singleton). Drives live theme
// application and view presets.
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
  navCollapsed: boolean
  hiddenMailboxIds: string[]
  // How often the backend polls for new mail, in seconds.
  pollIntervalSeconds: number
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
  navCollapsed: false,
  hiddenMailboxIds: [],
  pollIntervalSeconds: 60,
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
