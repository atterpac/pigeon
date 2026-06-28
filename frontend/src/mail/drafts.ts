import type { Address, ComposeDraft, Conversation, ThreadMessage } from './types'

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
    signatureId: '',
    signatureHtml: '',
    ...overrides,
  }
}

// selfAddr is the active account's address, excluded from reply-all Cc so you
// don't copy yourself.
export function replyDraft(replyKind: ReplyMode, selectedThread: Conversation | null, messages: ThreadMessage[], selfAddr = '') {
  const latest = messages.at(-1)
  const subject = selectedThread?.subject ?? ''
  const sender = latest?.from
  let cc: Address[] = []
  if (replyKind === 'replyAll' && latest) {
    // Carry the original To + Cc, dropping yourself and the sender (already in To).
    const seen = new Set([selfAddr.toLowerCase(), sender?.addr.toLowerCase() ?? ''].filter(Boolean))
    cc = [...(latest.to ?? []), ...(latest.cc ?? [])].filter((addr) => {
      const key = addr.addr.toLowerCase()
      if (!key || seen.has(key)) return false
      seen.add(key)
      return true
    })
  }
  return newDraft({
    threadId: selectedThread?.id,
    to: replyKind === 'forward' ? [] : sender ? [sender] : [],
    cc,
    subject: replyKind === 'forward' ? `Fwd: ${subject}` : replySubject(subject),
    inReplyTo: latest?.rfcMessageId,
    references: latest?.references ?? [],
  })
}

function replySubject(subject: string) {
  return subject.toLowerCase().startsWith('re:') ? subject : `Re: ${subject}`
}

// Appends a signature below the body using the conventional "-- " delimiter.
// No-op when the signature is empty.
export function withSignature(body: string, signature: string) {
  const sig = signature.trim()
  if (!sig) return body
  return `${body}\n\n-- \n${sig}`
}

export function stripSignature(body: string) {
  return body.replace(/\n\n-- \n[\s\S]*$/, '')
}

export function replaceSignature(body: string, signature: string) {
  return withSignature(stripSignature(body), signature)
}
