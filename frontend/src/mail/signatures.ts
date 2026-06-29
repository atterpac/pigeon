import type { EmailSignature, Settings } from '../composables/useSettings'
import { escapeHtml } from './format'

function signatureId() {
  return `sig-${Date.now()}-${Math.random().toString(16).slice(2)}`
}

export function emptySignature(): EmailSignature {
  return { id: signatureId(), name: 'New signature', body: '', html: '' }
}

// An account's signatures.
export function signaturesFor(settings: Settings, accountId?: string): EmailSignature[] {
  if (!accountId) return []
  return settings.signatureBooks[accountId] ?? []
}

export function defaultSignatureId(settings: Settings, accountId?: string) {
  if (!accountId) return ''
  const list = signaturesFor(settings, accountId)
  const saved = settings.defaultSignatureIds[accountId]
  return list.some((signature) => signature.id === saved) ? saved : (list[0]?.id ?? '')
}

export function signatureBody(settings: Settings, accountId?: string, signatureId?: string) {
  if (!accountId || !signatureId) return ''
  return (
    signaturesFor(settings, accountId).find((signature) => signature.id === signatureId)?.body ?? ''
  )
}

export function signatureHTML(settings: Settings, accountId?: string, signatureId?: string) {
  if (!accountId || !signatureId) return ''
  const signature = signaturesFor(settings, accountId).find((item) => item.id === signatureId)
  return signature?.html || plainSignatureHtml(signature?.body ?? '')
}

export function plainSignatureHtml(body: string) {
  return body
    .split('\n')
    .map((line) => escapeHtml(line) || '<br>')
    .join('<br>')
}

export function sanitizeSignatureHtml(html: string) {
  return html
    .replace(/<script\b[^>]*>[\s\S]*?<\/script>/gi, '')
    .replace(/\son\w+\s*=\s*"[^"]*"/gi, '')
    .replace(/\son\w+\s*=\s*'[^']*'/gi, '')
    .replace(/javascript:/gi, '')
}

export function saveSignature(settings: Settings, accountId: string, signature: EmailSignature) {
  const list = signaturesFor(settings, accountId)
  const next = list.some((item) => item.id === signature.id)
    ? list.map((item) => (item.id === signature.id ? signature : item))
    : [...list, signature]
  settings.signatureBooks = { ...settings.signatureBooks, [accountId]: next }
  if (!settings.defaultSignatureIds[accountId]) {
    settings.defaultSignatureIds = { ...settings.defaultSignatureIds, [accountId]: signature.id }
  }
}

export function deleteSignature(settings: Settings, accountId: string, id: string) {
  const next = signaturesFor(settings, accountId).filter((signature) => signature.id !== id)
  settings.signatureBooks = { ...settings.signatureBooks, [accountId]: next }
  if (settings.defaultSignatureIds[accountId] === id) {
    settings.defaultSignatureIds = {
      ...settings.defaultSignatureIds,
      [accountId]: next[0]?.id ?? '',
    }
  }
}
