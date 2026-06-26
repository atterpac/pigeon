// Theme registry (R1). Themes are DATA, not hardcoded CSS classes: adding a
// theme is a pure data add here, and applyTheme() writes the token contract to
// :root at runtime. Settings → Appearance iterates THEMES grouped by `pack`.
export type ThemeTokens = Record<string, string>

export interface Theme {
  id: string
  name: string
  pack: string
  dark: boolean
  tokens: ThemeTokens
}

export const THEMES: Theme[] = [
  {
    id: 'tokyonight-night', name: 'Night', pack: 'TokyoNight', dark: true,
    tokens: {
      '--bg': '#16161e', '--surface': '#1a1b26', '--surface-2': '#20212e', '--border': '#202230', '--border-2': '#2a2e42',
      '--text': '#c0caf5', '--text-dim': '#a9b1d6', '--text-mut': '#565f89', '--head': '#d5d9f0',
      '--accent': '#7aa2f7', '--accent-ink': '#16161e', '--accent-soft': 'rgba(122,162,247,.16)', '--accent-line': 'rgba(122,162,247,.45)',
      '--green': '#9ece6a', '--orange': '#ff9e64', '--red': '#f7768e', '--purple': '#bb9af7', '--cyan': '#7dcfff',
      '--star': '#e0af68', '--read-bg': '#1a1b26', '--read-text': '#a9b1d6', '--grid': 'rgba(122,162,247,.06)',
    },
  },
  {
    id: 'tokyonight-storm', name: 'Storm', pack: 'TokyoNight', dark: true,
    tokens: {
      '--bg': '#1f2335', '--surface': '#24283b', '--surface-2': '#2a2e42', '--border': '#292e42', '--border-2': '#3b4261',
      '--text': '#c0caf5', '--text-dim': '#a9b1d6', '--text-mut': '#565f89', '--head': '#d5d9f0',
      '--accent': '#7aa2f7', '--accent-ink': '#1f2335', '--accent-soft': 'rgba(122,162,247,.16)', '--accent-line': 'rgba(122,162,247,.45)',
      '--green': '#9ece6a', '--orange': '#ff9e64', '--red': '#f7768e', '--purple': '#bb9af7', '--cyan': '#7dcfff',
      '--star': '#e0af68', '--read-bg': '#24283b', '--read-text': '#a9b1d6', '--grid': 'rgba(122,162,247,.06)',
    },
  },
  {
    id: 'tokyonight-moon', name: 'Moon', pack: 'TokyoNight', dark: true,
    tokens: {
      '--bg': '#1e2030', '--surface': '#222436', '--surface-2': '#2f334d', '--border': '#272a3f', '--border-2': '#3b4261',
      '--text': '#c8d3f5', '--text-dim': '#a9b8e8', '--text-mut': '#636da6', '--head': '#d5d9f0',
      '--accent': '#82aaff', '--accent-ink': '#1e2030', '--accent-soft': 'rgba(130,170,255,.16)', '--accent-line': 'rgba(130,170,255,.45)',
      '--green': '#c3e88d', '--orange': '#ff966c', '--red': '#ff757f', '--purple': '#c099ff', '--cyan': '#86e1fc',
      '--star': '#ffc777', '--read-bg': '#222436', '--read-text': '#a9b8e8', '--grid': 'rgba(130,170,255,.06)',
    },
  },
  {
    id: 'tokyonight-day', name: 'Day', pack: 'TokyoNight', dark: false,
    tokens: {
      '--bg': '#e1e2e7', '--surface': '#e9e9ed', '--surface-2': '#d5d6db', '--border': '#c4c8da', '--border-2': '#a8aecb',
      '--text': '#3760bf', '--text-dim': '#6172b0', '--text-mut': '#8990b3', '--head': '#343b58',
      '--accent': '#2e7de9', '--accent-ink': '#e1e2e7', '--accent-soft': 'rgba(46,125,233,.12)', '--accent-line': 'rgba(46,125,233,.4)',
      '--green': '#587539', '--orange': '#b15c00', '--red': '#f52a65', '--purple': '#9854f1', '--cyan': '#007197',
      '--star': '#8c6c3e', '--read-bg': '#e9e9ed', '--read-text': '#3760bf', '--grid': 'rgba(46,125,233,.05)',
    },
  },
]

export function getTheme(id: string): Theme {
  return THEMES.find((theme) => theme.id === id) ?? THEMES[0]!
}

export function applyTheme(theme: Theme) {
  const root = document.documentElement
  for (const [key, value] of Object.entries(theme.tokens)) root.style.setProperty(key, value)
}

/** THEMES grouped by pack, for the Appearance picker. */
export function themesByPack(): Array<{ pack: string; themes: Theme[] }> {
  const packs: Array<{ pack: string; themes: Theme[] }> = []
  for (const theme of THEMES) {
    let group = packs.find((entry) => entry.pack === theme.pack)
    if (!group) { group = { pack: theme.pack, themes: [] }; packs.push(group) }
    group.themes.push(theme)
  }
  return packs
}
