// Reading pane: open a thread (supersession token lets a newer open cancel an
// older in-flight one), focus/expand messages, close back to the list. Pane
// *state* (selectedThread, threadMessages, focusPane, …) is owned by the shell so
// list resets can touch it; this module owns open/focus/close behaviour.
import { nextTick, type ComputedRef, type Ref } from 'vue'
import type { Conversation, MailClient, ThreadMessage } from '../mail/types'
import type { ReplyMode } from '../mail/drafts'

type Command = { kind: 'search' | 'ex' | 'find'; text: string } | null

type ThreadReaderDeps = {
  client: Ref<MailClient | null>
  status: Ref<string>
  activeMailbox: Ref<string>
  selectedThread: Ref<Conversation | null>
  threadMessages: Ref<ThreadMessage[]>
  focusedMessageId: Ref<string>
  focusPane: Ref<'list' | 'thread'>
  threadLoading: Ref<boolean>
  composeOpen: Ref<boolean>
  replyOpen: Ref<boolean>
  replyExpanded: Ref<boolean>
  command: Ref<Command>
  findExpandedSnapshot: Ref<Map<string, boolean> | null>
  threadFind: { close: () => void }
  selectedConversation: ComputedRef<Conversation | null>
  visualMode: Ref<boolean>
  findRow: (id: string) => Conversation | undefined
  patchListConversation: (id: string, patch: Partial<Conversation>) => void
  bumpMailboxUnread: (mailboxId: string, delta: number) => void
  exitVisual: () => void
  prepareReply: (replyKind: ReplyMode) => void
}

export function useThreadReader({
  client,
  status,
  activeMailbox,
  selectedThread,
  threadMessages,
  focusedMessageId,
  focusPane,
  threadLoading,
  composeOpen,
  replyOpen,
  replyExpanded,
  command,
  findExpandedSnapshot,
  threadFind,
  selectedConversation,
  visualMode,
  findRow,
  patchListConversation,
  bumpMailboxUnread,
  exitVisual,
  prepareReply,
}: ThreadReaderDeps) {
  // monotonic token so a newer openThread cancels an older in-flight one.
  let openSeq = 0

  async function openThread(threadId = selectedConversation.value?.id) {
    if (!client.value || !threadId) return
    if (visualMode.value) exitVisual()
    const wasUnread = findRow(threadId)?.unread
    // only a *newer* openThread should cancel this one. Comparing against
    // selectedConversation is wrong — background warming/reclassify can shift
    // the selection mid-await and falsely abort, flashing back to the preview.
    const seq = ++openSeq
    // a new conversation invalidates any active find session.
    if (command.value?.kind === 'find') command.value = null
    threadFind.close()
    findExpandedSnapshot.value = null
    threadLoading.value = true
    status.value = 'loading thread'
    const tStart = performance.now()
    try {
      const thread = await client.value.getThread(threadId)
      if (seq !== openSeq) return // a newer open superseded this one
      const tFetched = performance.now()
      selectedThread.value = thread.conversation
      threadMessages.value = thread.messages.map((message, index, messages) => ({
        ...message,
        expanded: message.expanded || index === messages.length - 1,
      }))
      focusedMessageId.value = threadMessages.value.at(-1)?.id ?? ''
      composeOpen.value = false
      replyOpen.value = false
      replyExpanded.value = false
      focusPane.value = 'thread'
      status.value = 'thread loaded'
      // getThread marks read server-side; mirror locally.
      if (wasUnread) {
        patchListConversation(threadId, { unread: false })
        bumpMailboxUnread(activeMailbox.value, -1)
      }
      prepareReply('reply')
      // fetch = getThread (backend+IPC+prep); render = Vue DOM patch after state
      // assignment. splits a slow open into backend vs. frontend cost.
      await nextTick()
      if (import.meta.env.DEV) {
        const tRendered = performance.now()
        console.debug(
          `[timing] openThread fetch=${(tFetched - tStart).toFixed(0)}ms render=${(tRendered - tFetched).toFixed(0)}ms total=${(tRendered - tStart).toFixed(0)}ms`,
        )
      }
    } finally {
      if (seq === openSeq) threadLoading.value = false
    }
  }
  function toggleMessageExpanded(id: string) {
    focusMessage(id)
    const message = threadMessages.value.find((item) => item.id === id)
    if (message) message.expanded = !message.expanded
  }
  function focusMessage(id: string) {
    if (threadMessages.value.some((message) => message.id === id)) focusedMessageId.value = id
  }
  // step message focus within the open thread (shift-J/K); expands the newly
  // focused message so its body shows.
  function focusAdjacentMessage(delta: number) {
    const messages = threadMessages.value
    if (!messages.length) return
    const currentIndex = messages.findIndex((message) => message.id === focusedMessageId.value)
    const nextIndex = Math.max(
      0,
      Math.min(messages.length - 1, (currentIndex < 0 ? 0 : currentIndex) + delta),
    )
    const next = messages[nextIndex]
    if (!next) return
    focusedMessageId.value = next.id
    next.expanded = true
  }
  function focusList() {
    focusPane.value = 'list'
  }
  function focusThread() {
    if (selectedThread.value) focusPane.value = 'thread'
  }
  function closeThread() {
    selectedThread.value = null
    threadMessages.value = []
    focusedMessageId.value = ''
    replyOpen.value = false
    replyExpanded.value = false
    focusPane.value = 'list'
    if (command.value?.kind === 'find') command.value = null
    threadFind.close()
    findExpandedSnapshot.value = null
  }

  return {
    openThread,
    toggleMessageExpanded,
    focusMessage,
    focusAdjacentMessage,
    focusList,
    focusThread,
    closeThread,
  }
}
