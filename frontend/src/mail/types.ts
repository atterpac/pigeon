export type Address = { name: string; addr: string }
export type Account = { id: string; email: string; name: string }
export type Mailbox = { id: string; name: string; role?: string; unread: number; total: number }
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
}
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
}
export type AttachmentDraft = { filename: string; contentType?: string; content?: string }
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
  deleteMailbox?(id: string): Promise<void>
  listLabels(): Promise<Label[]>
  listConversations(mailboxId: string): Promise<Conversation[]>
  preloadMailboxBodies?(mailboxId: string, limit?: number): Promise<number>
  reclassifyMailbox?(mailboxId: string, limit?: number): Promise<number>
  searchConversations(query: string): Promise<Conversation[]>
  getThread(threadId: string): Promise<Thread>
  archiveThread(threadId: string): Promise<void>
  snoozeThread(threadId: string, until?: string): Promise<void>
  toggleStar(threadId: string, on?: boolean): Promise<void>
  markThreadRead(threadId: string, read: boolean): Promise<void>
  saveDraft(draft: ComposeDraft): Promise<ComposeDraft>
  sendDraft(draft: ComposeDraft): Promise<void>
  discardDraft(draftId: string): Promise<void>
}
