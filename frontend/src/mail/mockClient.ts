import type { Account, Address, ComposeDraft, Conversation, Label, Mailbox, MailClient, ThreadMessage } from './types'

const me: Account = { id: 'dev', email: 'dev@example.com', name: 'You' }
const a = (name: string, addr: string): Address => ({ name, addr })

let mailboxes: Mailbox[] = [
  { id: 'INBOX', name: 'Inbox', role: 'inbox', unread: 3, total: 7 },
  { id: 'SNOOZED', name: 'Snoozed', unread: 0, total: 0 },
  { id: 'DONE', name: 'Done', role: 'archive', unread: 0, total: 0 },
  { id: 'SENT', name: 'Sent', role: 'sent', unread: 0, total: 0 },
  { id: 'DRAFTS', name: 'Drafts', role: 'drafts', unread: 0, total: 0 },
]

const labels: Label[] = [
  { id: 'team', name: 'Team', count: 1, swatch: '#5e54c0', bg: 'rgba(94,84,192,.16)', fg: '#b0a6f0' },
  { id: 'alerts', name: 'Alerts', count: 3, swatch: '#c0694f', bg: 'rgba(192,105,79,.18)', fg: '#e0a18f' },
  { id: 'receipts', name: 'Receipts', count: 2, swatch: '#5b8a6f', bg: 'rgba(63,122,92,.18)', fg: '#8fcaa6' },
  { id: 'reads', name: 'Reads', count: 1, swatch: '#787282', bg: 'rgba(120,114,130,.18)', fg: '#a8a2b2' },
]

let conversations: Conversation[] = [
  row('t-rate-limit', 'Marina Chen', 'marina@example.com', 'Re: API rate limiting design', 'Looks good - one concern about burst handling...', '2026-06-25T09:24:00', ['team'], true, false, 3),
  row('t-github', 'GitHub', 'noreply@github.com', '[atterpac/email] CI failed', 'Type-check failed on generated bindings', '2026-06-25T08:40:00', ['alerts'], true, false, 1, true, 'updates'),
  row('t-sentry', 'Sentry', 'alerts@sentry.io', 'New issue in mail bridge', 'Cannot resolve @wailsio/runtime in frontend build', '2026-06-25T08:15:00', ['alerts'], true, false, 1, true, 'updates'),
  row('t-npm', 'npm', 'npm@npmjs.com', 'Security advisory: lodash', 'High severity - upgrade to 4.17.21', '2026-06-24T11:05:00', ['alerts'], false, false, 1, false, 'updates'),
  row('t-stripe', 'Stripe', 'receipts@stripe.com', 'Payment received - $2,400.00', 'Invoice INV-2043 paid by Acme Inc', '2026-06-24T09:31:00', ['receipts'], false, true, 1, false, 'updates'),
  row('t-aws', 'AWS', 'no-reply@amazon.com', 'Your June cost estimate', '$1,847.22 - 12% above last month', '2026-06-22T13:15:00', ['receipts'], false, false, 1, false, 'updates'),
  row('t-tldr', 'TLDR', 'newsletter@tldr.dev', 'Bun 2.0, Postgres 17, and more', 'Top dev news in 5 minutes', '2026-06-22T07:02:00', ['reads'], false, false, 1, false, 'promotions'),
]

const threads: Record<string, ThreadMessage[]> = {
  't-rate-limit': [
    msg('m-1', 't-rate-limit', a('Marina Chen', 'marina@example.com'), '2026-06-23T14:17:00', ['Sketched the rate limiting approach for the public API.', 'Token bucket per-key, 5k/min steady rate with a short burst window.']),
    msg('m-2', 't-rate-limit', a(me.name, me.email), '2026-06-24T08:40:00', ['Agreed on token bucket. One concern at the burst boundary.']),
    { ...msg('m-3', 't-rate-limit', a('Marina Chen', 'marina@example.com'), '2026-06-25T09:24:00', ['Good catch on the boundary spike.', 'Per-250ms refill sounds right.']), expanded: true },
  ],
}
let drafts: ComposeDraft[] = []

export function createMockMailClient(): MailClient {
  return {
    source: 'mock',
    async getAccount() { return me },
    async listMailboxes() { return refreshMailboxes() },
    async listLabels() { return labels.map((label) => ({ ...label, count: conversations.filter((c) => c.labelIds.includes(label.id)).length })) },
    async listConversations(mailboxId) { return sort(conversations.filter((c) => c.mailboxIds.includes(mailboxId) || c.labelIds.includes(mailboxId))) },
    async preloadMailboxBodies() { return 0 },
    async reclassifyMailbox() { return 0 },
    async searchConversations(query) { return sort(conversations.filter((c) => matches(c, query))) },
    async getThread(threadId) {
      const conversation = get(threadId)
      conversation.unread = false
      threads[threadId] ||= [{ ...msg(`${threadId}-latest`, threadId, conversation.from, conversation.lastAt, [conversation.snippet]), expanded: true }]
      return { conversation: { ...conversation }, messages: threads[threadId].map((message) => ({ ...message })) }
    },
    async archiveThread(threadId) { mutate(threadId, (c) => { c.mailboxIds = c.mailboxIds.filter((id) => id !== 'INBOX'); c.mailboxIds.push('DONE') }) },
    async snoozeThread(threadId) { mutate(threadId, (c) => { c.mailboxIds = c.mailboxIds.filter((id) => id !== 'INBOX'); c.mailboxIds.push('SNOOZED') }) },
    async moveThread(threadId, mailboxId) { mutate(threadId, (c) => { c.mailboxIds = c.mailboxIds.filter((id) => id !== 'INBOX'); if (!c.mailboxIds.includes(mailboxId)) c.mailboxIds.push(mailboxId) }) },
    async applyLabel(threadId, labelId) { mutate(threadId, (c) => { if (!c.labelIds.includes(labelId)) c.labelIds.push(labelId) }) },
    async createLabel(name) { const id = name.trim().toLowerCase().replace(/\s+/g, '-'); const existing = labels.find((l) => l.id === id); if (existing) return existing; const label: Label = { id, name: name.trim(), count: 0, swatch: '#7aa2f7', bg: 'rgba(122,162,247,.16)', fg: '#7aa2f7' }; labels.push(label); return label },
    async createMailbox(name) { const id = name.trim().toUpperCase().replace(/\s+/g, '_'); const existing = mailboxes.find((m) => m.id === id); if (existing) return existing; const mailbox: Mailbox = { id, name: name.trim(), unread: 0, total: 0 }; mailboxes.push(mailbox); return mailbox },
    async renameMailbox(id, newName) { const mailbox = mailboxes.find((m) => m.id === id); if (mailbox) mailbox.name = newName.trim(); return mailbox ?? { id, name: newName.trim(), unread: 0, total: 0 } },
    async setMailboxIcon(id, icon, weight, color) { const mailbox = mailboxes.find((m) => m.id === id); if (mailbox) { mailbox.icon = icon || undefined; mailbox.iconWeight = weight || undefined; mailbox.iconColor = color || undefined } return mailbox ?? { id, name: id, unread: 0, total: 0 } },
    async toggleStar(threadId, on) { mutate(threadId, (c) => { c.starred = on ?? !c.starred }) },
    async markThreadRead(threadId, read) { mutate(threadId, (c) => { c.unread = !read }) },
    async saveDraft(draft) { const saved = { ...draft, updatedAt: new Date().toISOString() }; drafts = drafts.filter((d) => d.id !== saved.id).concat(saved); return saved },
    async sendDraft(draft) { const sent = row(`sent-${Date.now()}`, 'You', me.email, draft.subject || '(no subject)', draft.body.split('\n').find(Boolean) || '', new Date().toISOString(), ['team'], false); sent.mailboxIds = ['SENT']; conversations = [sent, ...conversations]; drafts = drafts.filter((d) => d.id !== draft.id) },
    async discardDraft(draftId) { drafts = drafts.filter((d) => d.id !== draftId) },
  }
}

function row(id: string, name: string, email: string, subject: string, snippet: string, lastAt: string, labelIds: string[], unread: boolean, hasAttachments = false, messageCount = 1, alert = false, category: Conversation['category'] = 'primary'): Conversation {
  const from = a(name, email)
  return { id, accountId: me.id, mailboxIds: ['INBOX'], labelIds, subject, snippet, category, lastAt, from, participants: [from, a(me.name, me.email)], unread, starred: false, hasAttachments, messageCount, alert }
}
function msg(id: string, threadId: string, from: Address, date: string, body: string[]): ThreadMessage {
  return { id, threadId, from, to: [a(me.name, me.email)], cc: [], date, snippet: body[0] ?? '', body, unread: false, expanded: false, rfcMessageId: `<${id}@example.com>`, references: [] }
}
function sort(items: Conversation[]) { return [...items].sort((left, right) => Date.parse(right.lastAt) - Date.parse(left.lastAt)) }
function get(id: string) { const found = conversations.find((c) => c.id === id); if (!found) throw new Error(`unknown thread ${id}`); return found }
function mutate(id: string, apply: (conversation: Conversation) => void) { apply(get(id)) }
function refreshMailboxes() { mailboxes = mailboxes.map((m) => { const rows = conversations.filter((c) => c.mailboxIds.includes(m.id)); return { ...m, total: rows.length, unread: rows.filter((c) => c.unread).length } }); return mailboxes }
function matches(c: Conversation, query: string) {
  const labelText = c.labelIds.map((id) => labels.find((l) => l.id === id)?.name ?? id).join(' ').toLowerCase()
  const haystack = `${c.from.name} ${c.from.addr} ${c.subject} ${c.snippet} ${labelText}`.toLowerCase()
  return query.toLowerCase().trim().split(/\s+/).filter(Boolean).every((token) => {
    if (token.startsWith('from:')) return `${c.from.name} ${c.from.addr}`.toLowerCase().includes(token.slice(5))
    if (token.startsWith('label:')) return labelText.includes(token.slice(6))
    if (token === 'is:unread') return c.unread
    if (token === 'is:read') return !c.unread
    if (token === 'has:attachment') return c.hasAttachments
    if (token.startsWith('after:') || token.startsWith('before:')) return true
    return haystack.includes(token)
  })
}
