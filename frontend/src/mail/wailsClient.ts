import * as Client from '@/bindings/github.com/atterpac/email/pkg/email/client'
import { Address as BindingAddress, Flag, Outgoing } from '@/bindings/github.com/atterpac/email/internal/model/models'
import type {
  Account as BindingAccount,
  Mailbox as BindingMailbox,
  Message as BindingMessage,
  Part as BindingPart,
  ThreadListItem,
} from '@/bindings/github.com/atterpac/email/pkg/email/models'
import type { Account, Address, Category, ComposeDraft, Conversation, Label, Mailbox, MailClient, Thread, ThreadMessage } from './types'

const FLAG_SEEN = Flag.FlagSeen
const FLAG_FLAGGED = Flag.FlagFlagged
const ROLE_NAMES: Record<number, string | undefined> = {
  1: 'inbox',
  2: 'sent',
  3: 'drafts',
  4: 'trash',
  5: 'spam',
  6: 'archive',
}
const LABEL_COLORS = ['#5e54c0', '#c0694f', '#5b8a6f', '#787282', '#b18444', '#4f8aa8']

export async function createWailsMailClient(preferredAccountId?: string): Promise<MailClient> {
  const accounts = await Client.Accounts()
  const account = accounts.find((item) => item.ID === preferredAccountId) ?? accounts[0]
  if (!account) throw new Error('No configured email account found.')

  const mailboxes = await Client.Mailboxes(account.ID)
  return new WailsMailClient(account, mailboxes)
}

class WailsMailClient implements MailClient {
  source = 'wails' as const

  constructor(
    private readonly account: BindingAccount,
    private mailboxes: BindingMailbox[],
  ) {}

  async getAccount(): Promise<Account> {
    return normalizeAccount(this.account)
  }

  async listMailboxes(): Promise<Mailbox[]> {
    this.mailboxes = await Client.Mailboxes(this.account.ID)
    return this.mailboxes.map(normalizeMailbox)
  }

  async listLabels(): Promise<Label[]> {
    this.mailboxes = await Client.Mailboxes(this.account.ID)
    return this.mailboxes
      .filter((mailbox) => !ROLE_NAMES[mailbox.Role] || mailbox.Role === 0)
      .map((mailbox, index) => labelFromMailbox(mailbox, index))
  }

  async listConversations(mailboxId: string): Promise<Conversation[]> {
    const mailbox = this.mailboxes.find((item) => item.ID === mailboxId)
    if (mailbox?.Role === 1) {
      const items = await Client.ConversationList(this.account.ID, 100)
      if (items.length) return items.map(conversationFromThreadListItem)
    }

    let messages = await Client.MailboxMessages(this.account.ID, mailboxId, 100)
    if (!messages.length && (mailbox?.Total ?? 0) > 0) {
      messages = await waitForMailboxMessages(this.account.ID, mailboxId)
    }
    return conversationsFromMessages(messages, mailboxId)
  }

  async preloadMailboxBodies(mailboxId: string, limit = 20): Promise<number> {
    return Client.PreloadMailboxBodies(this.account, mailboxId, limit)
  }

  async reclassifyMailbox(mailboxId: string, limit = 100): Promise<number> {
    return Client.ReclassifyMailbox(this.account.ID, mailboxId, limit)
  }

  async searchConversations(query: string): Promise<Conversation[]> {
    const messages = await Client.Search(this.account.ID, query, 100)
    return conversationsFromMessages(messages)
  }

  async getThread(threadId: string): Promise<Thread> {
    const messages = await Client.ThreadMessages(this.account.ID, threadId)
    const threadMessages = await Promise.all(messages.map((message, index) => this.normalizeThreadMessage(message, index === messages.length - 1)))
    const conversation = conversationFromThreadMessages(threadId, this.account.ID, threadMessages, messages)
    if (conversation.unread) {
      await this.markThreadRead(threadId, true)
      conversation.unread = false
    }
    return { conversation, messages: threadMessages }
  }

  async archiveThread(threadId: string): Promise<void> {
    await Client.Archive(this.account, await this.threadMessageIds(threadId))
  }

  async snoozeThread(threadId: string, until?: string): Promise<void> {
    const fallback = new Date(Date.now() + 24 * 60 * 60 * 1000).toISOString()
    await Client.Snooze(this.account, await this.threadMessageIds(threadId), until ?? fallback)
  }

  async toggleStar(threadId: string, on = true): Promise<void> {
    await Client.Star(this.account, await this.threadMessageIds(threadId), on)
  }

  async markThreadRead(threadId: string, read: boolean): Promise<void> {
    await Client.MarkRead(this.account, await this.threadMessageIds(threadId), read)
  }

  async saveDraft(draft: ComposeDraft): Promise<ComposeDraft> {
    const id = await Client.SaveDraft(this.account.ID, draft.id, outgoingFromDraft(this.account, draft))
    return { ...draft, id, updatedAt: new Date().toISOString() }
  }

  async sendDraft(draft: ComposeDraft): Promise<void> {
    await Client.Send(this.account, outgoingFromDraft(this.account, draft))
    if (draft.id) await Client.DiscardDraft(this.account.ID, draft.id).catch(() => undefined)
  }

  async discardDraft(draftId: string): Promise<void> {
    await Client.DiscardDraft(this.account.ID, draftId)
  }

  private async threadMessageIds(threadId: string) {
    const messages = await Client.ThreadMessages(this.account.ID, threadId)
    return messages.map((message) => message.ID)
  }

  private async normalizeThreadMessage(message: BindingMessage, expanded: boolean): Promise<ThreadMessage> {
    const parts = await Client.MessageBody(this.account, message.ID).catch((error) => {
      console.warn('Unable to load message body', message.ID, error)
      return message.Parts?.length ? message.Parts : [{
        ContentType: 'text/plain',
        Charset: 'utf-8',
        Disposition: 'inline',
        Filename: '',
        Size: 0,
        Content: `Body failed to load: ${errorMessage(error)}`,
        BlobRef: '',
      }]
    })
    const body = bodyParagraphs(parts, message.Snippet)
    const html = htmlBody(parts)
    return {
      id: message.ID,
      threadId: message.Thread,
      from: normalizeAddress(message.From[0]),
      to: message.To.map(normalizeAddress),
      cc: message.Cc.map(normalizeAddress),
      date: dateToISO(message.Date),
      snippet: message.Snippet,
      body,
      html,
      unread: !message.Flags.includes(FLAG_SEEN),
      expanded,
      rfcMessageId: message.RFCMessageID,
      references: message.References,
    }
  }
}

async function waitForMailboxMessages(accountId: string, mailboxId: string) {
  const deadline = Date.now() + 5000
  while (Date.now() < deadline) {
    await new Promise((resolve) => window.setTimeout(resolve, 500))
    const messages = await Client.MailboxMessages(accountId, mailboxId, 100)
    if (messages.length) return messages
  }
  return []
}

function normalizeAccount(account: BindingAccount): Account {
  return { id: account.ID, email: account.Email, name: account.Name || account.Email }
}

function normalizeMailbox(mailbox: BindingMailbox): Mailbox {
  return {
    id: mailbox.ID,
    name: mailbox.Name || mailbox.ID,
    role: ROLE_NAMES[mailbox.Role],
    unread: mailbox.Unread,
    total: mailbox.Total,
  }
}

function labelFromMailbox(mailbox: BindingMailbox, index: number): Label {
  const swatch = LABEL_COLORS[index % LABEL_COLORS.length] ?? '#787282'
  return {
    id: mailbox.ID,
    name: mailbox.Name || mailbox.ID,
    count: mailbox.Total,
    swatch,
    bg: alpha(swatch, 0.18),
    fg: swatch,
  }
}

function conversationsFromMessages(messages: BindingMessage[], mailboxId?: string): Conversation[] {
  const byThread = new Map<string, BindingMessage[]>()
  for (const message of messages) {
    const threadId = message.Thread || message.ID
    const existing = byThread.get(threadId) ?? []
    existing.push(message)
    byThread.set(threadId, existing)
  }

  return [...byThread.entries()]
    .map(([threadId, threadMessages]) => conversationFromMessages(threadId, threadMessages, mailboxId))
    .sort((left, right) => Date.parse(right.lastAt) - Date.parse(left.lastAt))
}

function conversationFromThreadListItem(item: ThreadListItem): Conversation {
  return {
    id: item.ID,
    accountId: item.Account,
    mailboxIds: item.Labels,
    labelIds: item.Labels,
    subject: item.Subject || '(no subject)',
    snippet: item.Snippet,
    category: normalizeCategory(item.Category),
    lastAt: dateToISO(item.Last),
    from: normalizeAddress(item.LatestSender),
    participants: item.Participants.map(normalizeAddress),
    unread: item.Unread,
    starred: false,
    hasAttachments: item.HasAttachments,
    messageCount: item.Count,
  }
}

function conversationFromMessages(threadId: string, messages: BindingMessage[], mailboxId?: string): Conversation {
  const sorted = [...messages].sort((left, right) => Date.parse(dateToISO(left.Date)) - Date.parse(dateToISO(right.Date)))
  const latest = sorted.at(-1) ?? messages[0]
  const labelIds = unique(sorted.flatMap((message) => message.Labels))
  const participants = uniqueAddresses(sorted.flatMap((message) => message.From))
  const fallbackMailboxIds = mailboxId ? [mailboxId] : labelIds
  return {
    id: threadId,
    accountId: latest?.Account ?? '',
    mailboxIds: labelIds.length ? labelIds : fallbackMailboxIds,
    labelIds,
    subject: latest?.Subject || '(no subject)',
    snippet: latest?.Snippet ?? '',
    category: normalizeCategory(latest?.Category),
    lastAt: dateToISO(latest?.Date),
    from: normalizeAddress(latest?.From[0]),
    participants: participants.length ? participants : [normalizeAddress(latest?.From[0])],
    unread: sorted.some((message) => !message.Flags.includes(FLAG_SEEN)),
    starred: sorted.some((message) => message.Flags.includes(FLAG_FLAGGED)),
    hasAttachments: sorted.some((message) => message.HasAttachments),
    messageCount: sorted.length,
  }
}

function conversationFromThreadMessages(threadId: string, accountId: string, threadMessages: ThreadMessage[], source: BindingMessage[]): Conversation {
  const latest = threadMessages.at(-1)
  const latestSource = source.at(-1)
  const labelIds = unique(source.flatMap((message) => message.Labels))
  return {
    id: threadId,
    accountId,
    mailboxIds: labelIds,
    labelIds,
    subject: latestSource?.Subject || '(no subject)',
    snippet: latest?.snippet ?? '',
    category: normalizeCategory(latestSource?.Category),
    lastAt: latest?.date ?? new Date(0).toISOString(),
    from: latest?.from ?? { name: '', addr: '' },
    participants: uniqueAddresses(source.flatMap((message) => message.From)),
    unread: threadMessages.some((message) => message.unread),
    starred: source.some((message) => message.Flags.includes(FLAG_FLAGGED)),
    hasAttachments: source.some((message) => message.HasAttachments),
    messageCount: threadMessages.length,
  }
}

function outgoingFromDraft(account: BindingAccount, draft: ComposeDraft): Outgoing {
  return new Outgoing({
    From: new BindingAddress({ Name: account.Name, Addr: account.Email }),
    To: draft.to.map(bindingAddress),
    Cc: draft.cc.map(bindingAddress),
    Bcc: draft.bcc.map(bindingAddress),
    Subject: draft.subject,
    Text: draft.body,
    InReplyTo: draft.inReplyTo ?? '',
    References: draft.references ?? [],
    Thread: draft.threadId ?? '',
    Attachments: draft.attachments.map((attachment) => ({
      Filename: attachment.filename,
      ContentType: attachment.contentType ?? 'application/octet-stream',
      Content: attachment.content ?? '',
    })),
  })
}

function bodyParagraphs(parts: BindingPart[], fallback: string): string[] {
  const text = parts
    .filter((part) => part.Disposition !== 'attachment')
    .sort((left, right) => Number(right.ContentType.includes('text/plain')) - Number(left.ContentType.includes('text/plain')))
    .map((part) => part.ContentType.includes('html') ? stripHtml(contentToString(part.Content)) : contentToString(part.Content))
    .find((content) => content.trim())
  return splitBody(text || fallback || '(No message body loaded.)')
}

function htmlBody(parts: BindingPart[]): string | undefined {
  return parts
    .filter((part) => part.Disposition !== 'attachment')
    .map((part) => ({ contentType: part.ContentType.toLowerCase(), content: contentToString(part.Content) }))
    .find((part) => part.contentType.includes('text/html') && part.content.trim())
    ?.content
}

function normalizeCategory(category: unknown): Category {
  return category === 'promotions' || category === 'updates' || category === 'social' || category === 'forums'
    ? category
    : 'primary'
}

function splitBody(body: string) {
  return body.replace(/\r\n/g, '\n').split(/\n{2,}/).map((part) => part.trim()).filter(Boolean)
}

function contentToString(content: unknown): string {
  if (typeof content === 'string') return decodeByteSliceString(content)
  if (content instanceof Uint8Array) return new TextDecoder().decode(content)
  if (Array.isArray(content)) return new TextDecoder().decode(Uint8Array.from(content as number[]))
  return ''
}

function decodeByteSliceString(content: string): string {
  if (!content) return ''
  try {
    const bytes = Uint8Array.from(atob(content), (char) => char.charCodeAt(0))
    const decoded = new TextDecoder().decode(bytes)
    return decoded.includes('\uFFFD') ? content : decoded
  } catch {
    return content
  }
}

function stripHtml(value: string) {
  return value.replace(/<style[\s\S]*?<\/style>/gi, '').replace(/<script[\s\S]*?<\/script>/gi, '').replace(/<[^>]+>/g, ' ').replace(/\s+/g, ' ').trim()
}

function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : String(error || 'unknown error')
}

function bindingAddress(address: Address) {
  return new BindingAddress({ Name: address.name, Addr: address.addr })
}

function normalizeAddress(address?: { Name: string; Addr: string }): Address {
  return { name: address?.Name ?? '', addr: address?.Addr ?? '' }
}

function unique<T>(items: T[]) {
  return [...new Set(items.filter(Boolean))]
}

function uniqueAddresses(addresses: { Name: string; Addr: string }[]) {
  const seen = new Set<string>()
  const out: Address[] = []
  for (const address of addresses) {
    const normalized = normalizeAddress(address)
    const key = normalized.addr || normalized.name
    if (!key || seen.has(key)) continue
    seen.add(key)
    out.push(normalized)
  }
  return out
}

function dateToISO(value: unknown): string {
  if (!value) return new Date(0).toISOString()
  if (value instanceof Date) return value.toISOString()
  if (typeof value === 'string') return value
  return new Date(String(value)).toISOString()
}

function alpha(hex: string, opacity: number) {
  const clean = hex.replace('#', '')
  const r = Number.parseInt(clean.slice(0, 2), 16)
  const g = Number.parseInt(clean.slice(2, 4), 16)
  const b = Number.parseInt(clean.slice(4, 6), 16)
  return `rgba(${r},${g},${b},${opacity})`
}
