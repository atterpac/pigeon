export type Address = { name: string; addr: string }
// Address-book entry for recipient autocomplete, ranked server-side by
// frequency/recency. Harvested from message envelopes during sync.
export type Contact = { name: string; addr: string }
export type Account = { id: string; email: string; name: string }
export type Mailbox = { id: string; name: string; role?: string; unread: number; total: number; icon?: string; iconWeight?: string; iconColor?: string }
export type Label = { id: string; name: string; count: number; swatch: string; bg: string; fg: string }
export type Category = 'primary' | 'promotions' | 'updates' | 'social' | 'forums'
export type Conversation = {
  id: string
  accountId: string
  mailboxIds: string[]
  labelIds: string[]
  subject: string
  snippet: string
  category: Category
  lastAt: string
  from: Address
  participants: Address[]
  unread: boolean
  starred: boolean
  hasAttachments: boolean
  alert?: boolean
  messageCount: number
  // Set only for rows in the Snoozed view: ISO wake time.
  snoozedUntil?: string
}
// MessageAttachment is a received attachment surfaced for display + download.
// `index` is its position among the message's attachment parts; the backend
// SaveAttachment binding rebuilds the bytes from the same ordering.
export type MessageAttachment = { filename: string; contentType: string; size: number; index: number }
export type ThreadMessage = {
  id: string
  threadId: string
  from: Address
  to: Address[]
  cc: Address[]
  date: string
  snippet: string
  body: string[]
  html?: string
  unread: boolean
  expanded: boolean
  rfcMessageId?: string
  references?: string[]
  attachments?: MessageAttachment[]
  // cid → inline image (base64), for resolving cid: refs in the HTML body.
  inlineImages?: Record<string, { contentType: string; content: string }>
}
// `content` is base64 of the file bytes; `size` is the original byte length.
// `contentId` marks an inline image embedded in the HTML body via cid:<contentId>
// (rendered in the preview, sent as an inline part) rather than a file attachment.
export type AttachmentDraft = { filename: string; contentType?: string; content?: string; size?: number; contentId?: string }
export type ComposeDraft = {
  id: string
  threadId?: string
  to: Address[]
  cc: Address[]
  bcc: Address[]
  subject: string
  body: string
  attachments: AttachmentDraft[]
  updatedAt: string
  signatureId?: string
  signatureHtml?: string
  inReplyTo?: string
  references?: string[]
}
export type Thread = { conversation: Conversation; messages: ThreadMessage[] }
export type MailClient = {
  source: 'mock' | 'wails'
  getAccount(): Promise<Account>
  listMailboxes(): Promise<Mailbox[]>
  createMailbox?(name: string): Promise<Mailbox>
  renameMailbox?(id: string, newName: string): Promise<Mailbox>
  setMailboxIcon?(id: string, icon: string, weight: string, color: string): Promise<Mailbox>
  deleteMailbox?(id: string): Promise<void>
  listLabels(): Promise<Label[]>
  listConversations(mailboxId: string): Promise<Conversation[]>
  preloadMailboxBodies?(mailboxId: string, limit?: number): Promise<number>
  reclassifyMailbox?(mailboxId: string, limit?: number): Promise<number>
  searchConversations(query: string): Promise<Conversation[]>
  // Server-side search reaching mail not synced locally (slower; one round-trip).
  searchServer?(query: string): Promise<Conversation[]>
  getThread(threadId: string): Promise<Thread>
  archiveThread(threadId: string): Promise<void>
  snoozeThread(threadId: string, until?: string): Promise<void>
  // Lists currently-snoozed conversations (each carries `snoozedUntil`).
  listSnoozed?(): Promise<Conversation[]>
  // Wakes a snoozed thread immediately (returns it to the inbox).
  unsnooze?(threadId: string): Promise<void>
  moveThread?(threadId: string, mailboxId: string): Promise<void>
  // Moves the thread to Trash (reversible — not a permanent delete).
  deleteThread?(threadId: string): Promise<void>
  applyLabel?(threadId: string, labelId: string): Promise<void>
  createLabel?(name: string): Promise<Label>
  toggleStar(threadId: string, on?: boolean): Promise<void>
  markThreadRead(threadId: string, read: boolean): Promise<void>
  // Recipient autocomplete: ranked address-book matches for a typed prefix.
  searchContacts?(query: string): Promise<Contact[]>
  saveDraft(draft: ComposeDraft): Promise<ComposeDraft>
  // Sends the draft. With holdSeconds > 0 the message is parked in the outbox
  // (undo-send window) and the returned op id can be passed to cancelSend;
  // resolves to '' when there's nothing to cancel (immediate send).
  sendDraft(draft: ComposeDraft, holdSeconds?: number): Promise<string>
  // Recalls a held send by op id before its window elapses.
  cancelSend?(opId: string): Promise<void>
  discardDraft(draftId: string): Promise<void>
  // Writes an attachment to disk. prompt=false drops it in Downloads; prompt=true
  // opens a native "Save as" dialog. Resolves to the saved path, or '' if cancelled.
  saveAttachment?(messageId: string, index: number, prompt: boolean): Promise<string>
  // Batch triage over multiple threads — used by Visual (multi-select) mode.
  // Each gathers the member message ids across the threads and issues one
  // mutation. The shell falls back to looping the single-thread methods when a
  // batch method is absent.
  archiveThreads?(threadIds: string[]): Promise<void>
  deleteThreads?(threadIds: string[]): Promise<void>
  moveThreads?(threadIds: string[], mailboxId: string): Promise<void>
  labelThreads?(threadIds: string[], labelId: string): Promise<void>
  starThreads?(threadIds: string[], on: boolean): Promise<void>
  markThreadsRead?(threadIds: string[], read: boolean): Promise<void>
  snoozeThreads?(threadIds: string[], until?: string): Promise<void>
}
