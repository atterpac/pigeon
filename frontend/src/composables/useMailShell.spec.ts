// tests the mail shell's framework-invisible logic: undo chaining, optimistic
// patch/reconcile, openThread supersession token, recipient materialization,
// attachment size cap.
import { nextTick, ref } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { Conversation, MailClient, Thread, ThreadMessage } from '../mail/types'
import { useUndo } from './useUndo'
import { useShellToast } from './useShellToast'
import { useAttachments } from './useAttachments'
import { newDraft } from '../mail/drafts'

// stub the wails-coupled modules so the singleton builds in jsdom without a backend.
vi.mock('@wailsio/runtime', () => ({ Events: { On: () => () => {} } }))
vi.mock('../mail/syncSettings', () => ({ applyPollInterval: vi.fn(), applyNotifyPrefs: vi.fn() }))
vi.mock('../mail/client', () => ({ createMailClient: vi.fn() }))
vi.mock('../onboarding/client', () => ({
  createOnboardingClient: () => ({
    listAccounts: vi.fn(async () => []),
    addAccount: vi.fn(),
    removeAccount: vi.fn(),
  }),
}))

function conv(id: string, over: Partial<Conversation> = {}): Conversation {
  return {
    id,
    accountId: 'acc',
    mailboxIds: ['inbox'],
    labelIds: [],
    subject: `Subject ${id}`,
    snippet: '',
    category: 'primary',
    lastAt: '2026-06-29T12:00:00.000Z',
    from: { name: 'A', addr: 'a@example.com' },
    participants: [],
    unread: false,
    starred: false,
    hasAttachments: false,
    messageCount: 1,
    ...over,
  }
}

function msg(id: string, over: Partial<ThreadMessage> = {}): ThreadMessage {
  return {
    id,
    threadId: 't',
    from: { name: 'Sender', addr: 'sender@example.com' },
    to: [],
    cc: [],
    date: '2026-06-29T12:00:00.000Z',
    snippet: '',
    body: [],
    unread: false,
    expanded: false,
    ...over,
  }
}

const thread = (id: string): Thread => ({ conversation: conv(id), messages: [] })

// fresh singleton per test — useMailShell caches a module-level instance, so
// resetModules + dynamic import isolates state.
async function freshShell() {
  vi.resetModules()
  const mod = await import('./useMailShell')
  return mod.useMailShell()
}

describe('useUndo', () => {
  it('collapses consecutive same-verb triage actions into one reversed chain', async () => {
    const status = ref('')
    const reloadList = vi.fn(async () => {})
    const { lastAction, recordUndo } = useUndo(status, reloadList)
    const order: number[] = []

    recordUndo('Archived', async () => {
      order.push(1)
    })
    recordUndo('Archived', async () => {
      order.push(2)
    })

    // One entry, labelled with the running count.
    expect(lastAction.value?.label).toBe('Archived 2')
    await lastAction.value!.undo()
    // Most-recent-first: action 2 reverses before action 1.
    expect(order).toEqual([2, 1])
  })

  it('starts a new entry when the verb changes', () => {
    const { lastAction, recordUndo } = useUndo(
      ref(''),
      vi.fn(async () => {}),
    )
    recordUndo('Archived', async () => {})
    recordUndo('Deleted', async () => {})
    expect(lastAction.value?.label).toBe('Deleted')
  })

  it('performUndo reconciles the list for triage but not for send', async () => {
    const reloadTriage = vi.fn(async () => {})
    const triage = useUndo(ref(''), reloadTriage)
    triage.recordUndo('Archived', async () => {})
    await triage.performUndo()
    expect(reloadTriage).toHaveBeenCalledOnce()
    expect(triage.lastAction.value).toBeNull()

    const reloadSend = vi.fn(async () => {})
    const send = useUndo(ref(''), reloadSend)
    send.recordUndo('Sending…', async () => {}, 'send')
    await send.performUndo()
    expect(reloadSend).not.toHaveBeenCalled()
  })
})

describe('useShellToast', () => {
  it('replaces the current toast and auto-dismisses after the timeout', () => {
    vi.useFakeTimers()
    try {
      const { toast, showToast, clearToast } = useShellToast()
      showToast({ kind: 'info', title: 'first' })
      expect(toast.value?.title).toBe('first')
      showToast({ kind: 'success', title: 'second' }, 1000)
      expect(toast.value?.title).toBe('second')
      vi.advanceTimersByTime(1000)
      expect(toast.value).toBeNull()

      showToast({ kind: 'error', title: 'third' })
      clearToast()
      expect(toast.value).toBeNull()
    } finally {
      vi.useRealTimers()
    }
  })
})

describe('useAttachments', () => {
  function harness() {
    const draft = ref(newDraft())
    const status = ref('')
    const client = ref<MailClient | null>(null)
    const queueSave = vi.fn()
    const showToast = vi.fn()
    const api = useAttachments({ draft, status, client, queueSave, showToast })
    return { draft, status, client, queueSave, showToast, api }
  }

  it('queues files under the cap and skips ones that would blow the 25 MB budget', async () => {
    const h = harness()
    const small = new File([new Uint8Array(10)], 'small.txt', { type: 'text/plain' })
    const huge = new File([''], 'huge.bin')
    Object.defineProperty(huge, 'size', { value: 26 * 1024 * 1024 })

    await h.api.attachFiles([small, huge])

    // The oversized file is dropped; only the small one survives the budget check.
    expect(h.draft.value.attachments.map((a) => a.filename)).toEqual(['small.txt'])
    expect(h.status.value).toBe('1 attachment queued')
    expect(h.queueSave).toHaveBeenCalled()
  })

  it('exposes inline images keyed by contentId', async () => {
    const h = harness()
    const img = new File([new Uint8Array([1, 2, 3])], 'pic.png', { type: 'image/png' })
    await h.api.attachInlineImage(img, 'cid-1')
    expect(h.api.inlineImageMap.value['cid-1']).toMatchObject({ contentType: 'image/png' })
  })
})

describe('useMailShell integration', () => {
  afterEach(() => vi.useRealTimers())

  it('openThread: a newer open supersedes an in-flight older one', async () => {
    const shell = await freshShell()
    let resolveA!: () => void
    let resolveB!: () => void
    const client = {
      source: 'mock',
      getThread: vi.fn((id: string) =>
        id === 'a'
          ? new Promise<Thread>((r) => {
              resolveA = () => r(thread('a'))
            })
          : new Promise<Thread>((r) => {
              resolveB = () => r(thread('b'))
            }),
      ),
    } as unknown as MailClient
    shell.client.value = client
    shell.conversations.value = [conv('a'), conv('b')]

    const p1 = shell.openThread('a')
    const p2 = shell.openThread('b')
    // Resolve the stale one last to prove ordering, not timing, decides the winner.
    resolveB()
    resolveA()
    await Promise.all([p1, p2])

    expect(shell.selectedThread.value?.id).toBe('b')
  })

  it('archiveThread optimistically drops the row, reconciles, and records undo', async () => {
    const shell = await freshShell()
    const archiveThread = vi.fn(async () => {})
    const listConversations = vi.fn(async () => [conv('b')])
    const client = {
      source: 'mock',
      archiveThread,
      listMailboxes: vi.fn(async () => []),
      listConversations,
    } as unknown as MailClient
    shell.client.value = client
    shell.activeMailbox.value = 'inbox'
    shell.conversations.value = [conv('a', { unread: true }), conv('b')]
    shell.selectedThread.value = conv('a')

    await shell.archiveThread()

    expect(archiveThread).toHaveBeenCalledWith('a')
    expect(listConversations).toHaveBeenCalledWith('inbox') // reconciled
    expect(shell.conversations.value.find((c) => c.id === 'a')).toBeUndefined()
    expect(shell.selectedThread.value).toBeNull()
    expect(shell.lastAction.value?.label).toBe('Archived')
  })

  it('materializeRecipients folds typed input into the draft and clears the field', async () => {
    const shell = await freshShell()
    shell.draft.value = newDraft()
    shell.recipientInput.value = 'a@b.com, c@d.com'
    const result = shell.materializeRecipients()
    expect(result.to.map((addr) => addr.addr)).toEqual(['a@b.com', 'c@d.com'])
    expect(shell.recipientInput.value).toBe('')
  })

  it('body autosave persists without chipping a half-typed recipient', async () => {
    vi.useFakeTimers()
    const shell = await freshShell()
    const saveDraft = vi.fn(async (d) => d)
    shell.client.value = { source: 'mock', saveDraft } as unknown as MailClient
    shell.composeOpen.value = true
    shell.recipientInput.value = 'john@example.com' // typed, not yet confirmed

    shell.draft.value.body = 'hello'
    await nextTick() // flush the body watcher → queueSave()
    await vi.advanceTimersByTimeAsync(400) // fire the debounced save

    expect(saveDraft).toHaveBeenCalledOnce()
    // The recipient input is untouched by an autosave triggered from the body.
    expect(shell.recipientInput.value).toBe('john@example.com')
    const savedDraft = saveDraft.mock.calls[0]![0]
    expect(savedDraft.to).toEqual([])
  })

  it('searchServer merges fresh hits, drops dupes, and sorts newest-first', async () => {
    const shell = await freshShell()
    shell.query.value = 'x'
    await nextTick() // let the async query watcher fire as a no-op (searchActive still false)
    shell.searchActive.value = true // skip the openSearch round-trip
    shell.searchResults.value = [conv('a', { lastAt: '2026-06-20T00:00:00.000Z' })]
    const searchServer = vi.fn(async () => [
      conv('a', { lastAt: '2026-06-20T00:00:00.000Z' }), // dupe of existing
      conv('b', { lastAt: '2026-06-25T00:00:00.000Z' }),
      conv('c', { lastAt: '2026-06-10T00:00:00.000Z' }),
    ])
    shell.client.value = { source: 'mock', searchServer } as unknown as MailClient

    await shell.searchServer()

    expect(shell.searchResults.value.map((c) => c.id)).toEqual(['b', 'a', 'c'])
    expect(shell.serverSearching.value).toBe(false)
  })

  it('openFind expands all messages; cancelCommand restores the prior layout', async () => {
    const shell = await freshShell()
    shell.selectedThread.value = conv('t')
    shell.threadMessages.value = [msg('m1', { expanded: false }), msg('m2', { expanded: true })]

    expect(shell.openFind()).toBe(true)
    expect(shell.threadMessages.value.every((m) => m.expanded)).toBe(true)
    expect(shell.command.value?.kind).toBe('find')

    shell.cancelCommand()
    expect(shell.threadMessages.value.map((m) => m.expanded)).toEqual([false, true])
    expect(shell.command.value).toBeNull()
  })

  it('ex command :q dispatches to closeThread', async () => {
    const shell = await freshShell()
    shell.selectedThread.value = conv('t')
    shell.threadMessages.value = [msg('m1')]
    shell.command.value = { kind: 'ex', text: 'q' }

    shell.submitCommand()

    expect(shell.selectedThread.value).toBeNull()
    expect(shell.threadMessages.value).toEqual([])
    expect(shell.command.value).toBeNull()
  })

  it('deleteThread optimistically drops the row, reconciles, and records a Deleted undo', async () => {
    const shell = await freshShell()
    const deleteThread = vi.fn(async (_id: string) => {})
    const moveThread = vi.fn(async (_id: string, _dst: string) => {})
    const client = {
      source: 'mock',
      deleteThread,
      moveThread, // the undo inverse moves it back
      listMailboxes: vi.fn(async () => []),
      listConversations: vi.fn(async () => []),
    } as unknown as MailClient
    shell.client.value = client
    shell.activeMailbox.value = 'inbox'
    shell.conversations.value = [conv('a'), conv('b')]
    shell.selectedThread.value = conv('a')

    await shell.deleteThread()

    expect(deleteThread).toHaveBeenCalledWith('a')
    expect(shell.conversations.value.find((c) => c.id === 'a')).toBeUndefined()
    expect(shell.lastAction.value?.label).toBe('Deleted')

    await shell.lastAction.value!.undo()
    expect(moveThread).toHaveBeenCalledWith('a', 'inbox') // restored to origin
  })

  it('archiveSelection falls back to per-id calls when no batch method exists', async () => {
    const shell = await freshShell()
    const archiveThread = vi.fn(async (_id: string) => {})
    const client = {
      source: 'mock',
      archiveThread, // note: no archiveThreads batch method
      listMailboxes: vi.fn(async () => []),
      listConversations: vi.fn(async () => []),
    } as unknown as MailClient
    shell.client.value = client
    shell.activeMailbox.value = 'inbox'
    shell.conversations.value = [conv('a'), conv('b'), conv('c')]
    shell.enterVisual()
    shell.toggleSelect('a')
    shell.toggleSelect('c')

    await shell.archiveSelection()

    // one call per selected id, and visual mode exits
    expect(archiveThread.mock.calls.map((c) => c[0]).sort()).toEqual(['a', 'c'])
    expect(shell.visualMode.value).toBe(false)
    expect(shell.lastAction.value?.label).toBe('Archived 2')
  })

  it('prepareReply builds a reply draft addressed to the sender', async () => {
    const shell = await freshShell()
    shell.selectedThread.value = conv('t', { subject: 'Hello' })
    shell.threadMessages.value = [msg('m1', { from: { name: 'X', addr: 'x@example.com' } })]
    shell.prepareReply('reply')
    expect(shell.draft.value.to.map((a) => a.addr)).toEqual(['x@example.com'])
    expect(shell.draft.value.subject).toBe('Re: Hello')
  })
})
