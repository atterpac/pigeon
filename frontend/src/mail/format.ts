// Pure presentation helpers shared across the mail shell components.
// No reactive state — safe to import anywhere.
import type { Conversation, Label } from './types'

export function escapeHtml(value: string) {
  return value.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
}

export function initials(address: { name: string; addr: string }) {
  return (address.name || address.addr).split(/\s+/).slice(0, 2).map((part) => part[0]).join('').toUpperCase()
}

// Deterministic per-sender identity tint. Hashes the address to one of the
// theme's named hue tokens so avatars stay colorful and scannable while still
// inheriting whatever theme is active. Returns inline styles for a tinted chip.
const AVATAR_HUES = ['--accent', '--green', '--orange', '--purple', '--cyan', '--red', '--star']
export function avatarStyle(address: { name: string; addr: string }) {
  const key = (address.addr || address.name).toLowerCase()
  let hash = 0
  for (let i = 0; i < key.length; i++) hash = (hash * 31 + key.charCodeAt(i)) >>> 0
  const hue = AVATAR_HUES[hash % AVATAR_HUES.length]
  return {
    color: `var(${hue})`,
    background: `color-mix(in oklab, var(${hue}) 18%, transparent)`,
    borderColor: `color-mix(in oklab, var(${hue}) 38%, transparent)`,
  }
}

export function participantLine(conversation: Conversation | null) {
  return conversation?.participants.map((p) => p.name || p.addr).join(', ') ?? ''
}

export function isToday(value: string) {
  return new Date(value).toDateString() === new Date().toDateString()
}

export function formatDate(value: string) {
  const date = new Date(value)
  return isToday(value)
    ? date.toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' })
    : date.toLocaleDateString([], { weekday: 'short' })
}

export function labelFor(conversation: Conversation | null, labels: Label[]) {
  return labels.find((label) => conversation?.labelIds.includes(label.id))
}

export function parseAddresses(input: string) {
  return input.split(',').map((value) => value.trim()).filter(Boolean).map((value) => {
    const match = value.match(/^(.*)<(.+)>$/)
    return match ? { name: match[1]?.trim() ?? '', addr: match[2]?.trim() ?? '' } : { name: '', addr: value }
  })
}

export function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : String(error || 'Unknown error')
}

export function renderEmailHtml(html: string) {
  return `<!doctype html><html><head><base target="_blank"><meta name="referrer" content="no-referrer"><style>html,body{margin:0;padding:0;background:#fff;color:#111}body{overflow-wrap:anywhere}img{max-width:100%;height:auto}</style></head><body>${html}</body></html>`
}

export function renderInlineMarkdown(line: string) {
  return escapeHtml(line)
    .replace(/\[([^\]]+)\]\((https?:\/\/[^)\s]+)\)/g, '<a href="$2" target="_blank" rel="noreferrer">$1</a>')
    .replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>')
    .replace(/_([^_\n]+)_/g, '<em>$1</em>')
    .replace(/`([^`]+)`/g, '<code>$1</code>')
}

export function renderMarkdown(markdown: string) {
  if (!markdown.trim()) return '<div class="preview-empty">Nothing to preview yet.</div>'
  let inFence = false
  return markdown.split('\n').map((line) => {
    if (line.trim().startsWith('```')) { inFence = !inFence; return '<div class="preview-line"><code>```</code></div>' }
    const rendered = inFence ? `<code>${escapeHtml(line) || '&nbsp;'}</code>` : renderInlineMarkdown(line)
    return `<div class="preview-line">${rendered || '&nbsp;'}</div>`
  }).join('')
}
