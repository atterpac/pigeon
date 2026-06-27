import type { ComposeDraft, Conversation, ThreadMessage } from './types'

export type ReplyMode = 'reply' | 'replyAll' | 'forward'

export function newDraft(overrides: Partial<ComposeDraft> = {}): ComposeDraft {
  return {
    id: `draft-${Date.now()}-${Math.random().toString(16).slice(2)}`,
    to: [],
    cc: [],
    bcc: [],
    subject: '',
    body: '',
    attachments: [],
    updatedAt: new Date().toISOString(),
    ...overrides,
  }
}

export function replyDraft(replyKind: ReplyMode, selectedThread: Conversation | null, messages: ThreadMessage[]) {
  const latest = messages.at(-1)
  const subject = selectedThread?.subject ?? ''
  return newDraft({
    threadId: selectedThread?.id,
    to: replyKind === 'forward' ? [] : latest?.from ? [latest.from] : [],
    subject: replyKind === 'forward' ? `Fwd: ${subject}` : replySubject(subject),
    inReplyTo: latest?.rfcMessageId,
    references: latest?.references ?? [],
  })
}

function replySubject(subject: string) {
  return subject.toLowerCase().startsWith('re:') ? subject : `Re: ${subject}`
}
