// vim command layer: `/` search, `:` ex-commands, in-thread find, which-key menu.
// dispatcher only — owns the menu flag and find-expansion restore; verbs delegate
// to injected actions.
import { ref, type ComputedRef, type Ref } from 'vue'
import type { Conversation, ThreadMessage } from '../mail/types'

type Command = { kind: 'search' | 'ex' | 'find'; text: string } | null

type CommandLineDeps = {
  command: Ref<Command>
  searchActive: Ref<boolean>
  query: Ref<string>
  status: Ref<string>
  selectedThread: Ref<Conversation | null>
  selectedConversation: ComputedRef<Conversation | null>
  threadMessages: Ref<ThreadMessage[]>
  findExpandedSnapshot: Ref<Map<string, boolean> | null>
  threadFind: { open: () => void; close: () => void; next: () => void }
  runSearch: () => Promise<void> | void
  openSearch: () => Promise<void>
  closeSearch: () => void
  searchServer: () => Promise<void>
  saveSearch: (name?: string) => void
  archiveSelected: () => Promise<void>
  deleteThread: () => Promise<void>
  reportSpam: () => Promise<void>
  snoozeThread: (until?: string) => Promise<void>
  queueSave: (materialize?: boolean) => void
  closeThread: () => void
}

export function useCommandLine({
  command,
  searchActive,
  query,
  status,
  selectedThread,
  selectedConversation,
  threadMessages,
  findExpandedSnapshot,
  threadFind,
  runSearch,
  openSearch,
  closeSearch,
  searchServer,
  saveSearch,
  archiveSelected,
  deleteThread,
  reportSpam,
  snoozeThread,
  queueSave,
  closeThread,
}: CommandLineDeps) {
  // which-key command menu (thread → archive/snooze/label/move).
  const commandMenuOpen = ref(false)

  function openCommand(kind: 'search' | 'ex') {
    if (kind === 'search') {
      command.value = { kind, text: query.value }
      searchActive.value = true
      void runSearch()
    } else {
      command.value = { kind, text: '' }
    }
  }
  // `/` in the thread pane finds within the open conversation, not a mailbox-wide
  // search.
  function openFind() {
    if (!selectedThread.value) return false
    // expand every message so find covers the whole conversation. snapshot first
    // so closing find restores the prior layout. new iframes are picked up by the
    // find engine as they load.
    findExpandedSnapshot.value = new Map(threadMessages.value.map((m) => [m.id, m.expanded]))
    for (const message of threadMessages.value) message.expanded = true
    command.value = { kind: 'find', text: '' }
    threadFind.open()
    return true
  }
  function restoreFindExpansion() {
    if (!findExpandedSnapshot.value) return
    for (const message of threadMessages.value) {
      const prev = findExpandedSnapshot.value.get(message.id)
      if (prev !== undefined) message.expanded = prev
    }
    findExpandedSnapshot.value = null
  }
  function submitCommand() {
    const current = command.value
    // find stays open on Enter — Enter jumps to the next match.
    if (current?.kind === 'find') {
      threadFind.next()
      return
    }
    command.value = null
    if (current?.kind === 'ex') runEx(current.text)
    // search results persist; selection moves to the list.
  }
  function cancelCommand() {
    if (command.value?.kind === 'search') closeSearch()
    if (command.value?.kind === 'find') {
      threadFind.close()
      restoreFindExpansion()
    }
    command.value = null
  }
  function runEx(text: string) {
    const cmd = text.trim().replace(/^:/, '')
    if (cmd === 'archive') void archiveSelected()
    else if (cmd === 'delete' || cmd === 'd') void deleteThread()
    else if (cmd === 'spam') void reportSpam()
    else if (cmd === 'snooze') void snoozeThread()
    else if (cmd === 'w' || cmd === 'write') {
      void queueSave(true)
      status.value = 'draft saved'
    } else if (cmd === 'q' || cmd === 'quit') closeThread()
    else if (cmd.startsWith('label ')) {
      query.value = `label:${cmd.slice(6).trim()}`
      void openSearch()
    } else if (cmd === 'server' || cmd === 'search!') void searchServer()
    else if (cmd === 'save' || cmd.startsWith('save ')) saveSearch(cmd.slice(4).trim())
    else status.value = `E492: not an editor command: ${cmd}`
  }
  function openCommandMenu() {
    if (!selectedThread.value && !selectedConversation.value) return
    commandMenuOpen.value = true
  }
  function closeCommandMenu() {
    commandMenuOpen.value = false
  }

  return {
    commandMenuOpen,
    openCommand,
    openFind,
    submitCommand,
    cancelCommand,
    openCommandMenu,
    closeCommandMenu,
  }
}
