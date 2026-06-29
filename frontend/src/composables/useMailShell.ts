// Shared mail-shell state + actions for the app-level mail workflow.
// Singleton: every component that calls useMailShell() gets the same instance,
// so state is shared without prop-drilling.
import { computed, ref, watch } from 'vue'
import { applyNotifyPrefs } from '../mail/syncSettings'
import { useSettings } from './useSettings'
import { useThreadFind } from './useThreadFind'
import { useShellToast, type ShellToast } from './useShellToast'
import { useUndo } from './useUndo'
import { useMailboxAdmin } from './useMailboxAdmin'
import { useMailSearch } from './useMailSearch'
import { useAccountSetup, type AppPhase } from './useAccountSetup'
import { useCompose } from './useCompose'
import { useVisualSelection } from './useVisualSelection'
import { useTriage } from './useTriage'
import { useThreadReader } from './useThreadReader'
import { useCommandLine } from './useCommandLine'
import type {
  Account,
  Category,
  Conversation,
  Label,
  Mailbox,
  MailClient,
  ThreadMessage,
} from '../mail/types'
import { errorMessage, isToday } from '../mail/format'

export type CategoryTab = Category | 'all'
export type FocusPane = 'list' | 'thread'
export type { ShellToast, AppPhase }

export const categoryTabs: Array<{ id: CategoryTab; label: string }> = [
  { id: 'all', label: 'All' },
  { id: 'primary', label: 'Primary' },
  { id: 'promotions', label: 'Promotions' },
  { id: 'updates', label: 'Updates' },
  { id: 'social', label: 'Social' },
  { id: 'forums', label: 'Forums' },
]

function createMailShell() {
  const settings = useSettings()
  const client = ref<MailClient | null>(null)
  const account = ref<Account | null>(null)

  const activeMailbox = ref('')
  const activeCategory = ref<CategoryTab>('all')
  const selectedIndex = ref(0)
  const selectedThread = ref<Conversation | null>(null)
  const mailboxes = ref<Mailbox[]>([])
  const labels = ref<Label[]>([])
  const conversations = ref<Conversation[]>([])
  const searchResults = ref<Conversation[]>([])
  // Snoozed view: a virtual mailbox of conversations hidden until their wake
  // time. Toggled like search — when active it replaces the conversation list.
  const snoozedActive = ref(false)
  const snoozedItems = ref<Conversation[]>([])
  // Visual (multi-select) mode: vim `v` enters it, j/k navigate, `space` toggles
  // the focused row in/out. selectedIds is the set acted on by batch triage.
  const visualMode = ref(false)
  const selectedIds = ref<Set<string>>(new Set())
  const { toast, showToast, clearToast } = useShellToast()
  const threadMessages = ref<ThreadMessage[]>([])
  const focusedMessageId = ref('')
  const query = ref('from:github is:unread')
  const focusPane = ref<FocusPane>('list')
  const status = ref('loading')
  // True while a thread's messages/bodies are being fetched for the reading pane.
  const threadLoading = ref(false)
  // Undo ledger (reloadList/status are hoisted/defined above their first use).
  const { lastAction, recordUndo, clearUndo, performUndo } = useUndo(status, reloadList)

  // Overlay / pane modes (replaced the old `screen` enum).
  const composeOpen = ref(false)
  const searchActive = ref(false)
  // Active command-line input: `/` search, `:` ex-command (vim layer), or
  // `find` (in-thread find when the reading pane is focused).
  const command = ref<{ kind: 'search' | 'ex' | 'find'; text: string } | null>(null)
  const threadFind = useThreadFind()
  // Expanded-state snapshot taken when find opens, so closing restores the prior
  // view. A ref (not a closure local) since the reading pane + find both use it.
  const findExpandedSnapshot = ref<Map<string, boolean> | null>(null)

  // ── Compose / draft / attachments ──────────────────────────────────────
  const {
    draft,
    recipientInput,
    ccInput,
    bccInput,
    replyMode,
    replyOpen,
    replyExpanded,
    signatureOptions,
    prepareReply,
    openReply,
    toggleReplyExpanded,
    compose,
    sendDraft,
    discardDraft,
    materializeRecipients,
    queueSave,
    setDraftSignature,
    cancelAutosave,
    pickAttachments,
    attachFiles,
    attachInlineImage,
    downloadAttachment,
    inlineImageMap,
  } = useCompose({
    client,
    account,
    status,
    settings,
    selectedThread,
    threadMessages,
    composeOpen,
    showToast,
    recordUndo,
  })

  const filteredConversations = computed(() =>
    activeCategory.value === 'all'
      ? conversations.value
      : conversations.value.filter(
          (conversation) => conversation.category === activeCategory.value,
        ),
  )
  const unreadCount = computed(
    () => filteredConversations.value.filter((conversation) => conversation.unread).length,
  )
  const todayConversations = computed(() =>
    filteredConversations.value.filter((conversation) => isToday(conversation.lastAt)),
  )
  const earlierConversations = computed(() =>
    filteredConversations.value.filter((conversation) => !isToday(conversation.lastAt)),
  )
  const groupedConversations = computed(() => [
    ...todayConversations.value,
    ...earlierConversations.value,
  ])
  const activeList = computed(() =>
    snoozedActive.value
      ? snoozedItems.value
      : searchActive.value
        ? searchResults.value
        : groupedConversations.value,
  )
  const selectedConversation = computed(() => activeList.value[selectedIndex.value] ?? null)
  // The thread targeted by triage/label commands: the open thread, else the
  // highlighted row.
  const targetThreadId = computed(() => selectedThread.value?.id ?? selectedConversation.value?.id)
  const categoryCounts = computed(() => {
    const counts: Record<CategoryTab, number> = {
      all: conversations.value.length,
      primary: 0,
      promotions: 0,
      updates: 0,
      social: 0,
      forums: 0,
    }
    for (const conversation of conversations.value) counts[conversation.category] += 1
    return counts
  })
  const mode = computed(() =>
    composeOpen.value
      ? 'COMPOSE'
      : searchActive.value
        ? 'SEARCH'
        : visualMode.value
          ? 'VISUAL'
          : snoozedActive.value
            ? 'SNOOZED'
            : selectedThread.value
              ? 'THREAD'
              : 'NORMAL',
  )
  const selectedCount = computed(() => selectedIds.value.size)
  const focusedThreadMessage = computed(
    () =>
      threadMessages.value.find((message) => message.id === focusedMessageId.value) ??
      threadMessages.value.at(-1) ??
      null,
  )
  const statusHints = computed(() => {
    if (composeOpen.value) return '⌘↵ send · ⌘⇧A attach · esc discard'
    if (visualMode.value)
      return 'j k move · space select · V all · e # s * u act · ↵ menu · esc exit'
    if (snoozedActive.value) return 'j k move · ↵ open · u unsnooze · esc back'
    if (searchActive.value) return '↑↓ navigate · ↵ open · esc clear'
    if (selectedThread.value && focusPane.value === 'thread')
      return 'j k scroll · r reply · e archive · esc list'
    if (selectedThread.value) return 'j k move · ↵ open · tab thread · e archive'
    return 'j k move · space cmd · e archive · s snooze · c compose · ⌘K search'
  })

  // ── Account lifecycle + onboarding ─────────────────────────────────────
  const {
    appPhase,
    configuredAccounts,
    setupStatus,
    setupError,
    setupBusy,
    setup,
    initializeApp,
    bootMailbox,
    submitOnboarding,
    resetSetup,
    removeAccount,
    refreshShell,
    teardownChangefeed,
  } = useAccountSetup({
    client,
    account,
    mailboxes,
    labels,
    activeMailbox,
    status,
    settings,
    openMailbox,
    reloadList,
  })
  async function openMailbox(mailboxId: string) {
    if (!client.value) return
    activeMailbox.value = mailboxId
    conversations.value = await client.value.listConversations(mailboxId)
    selectedIndex.value = 0
    selectedThread.value = null
    threadMessages.value = []
    focusedMessageId.value = ''
    replyOpen.value = false
    focusPane.value = 'list'
    composeOpen.value = false
    searchActive.value = false
    snoozedActive.value = false
    visualMode.value = false
    selectedIds.value = new Set()
    void warmMailbox(mailboxId)
  }
  async function warmMailbox(mailboxId: string) {
    if (!client.value) return
    try {
      // Warm aggressively: foreground opens use a dedicated provider connection,
      // so this background prewarm never delays an open. Backend caps at 100.
      await client.value.preloadMailboxBodies?.(mailboxId, 100)
      const changed = (await client.value.reclassifyMailbox?.(mailboxId, 100)) ?? 0
      if (changed > 0 && activeMailbox.value === mailboxId && !searchActive.value) {
        conversations.value = await client.value.listConversations(mailboxId)
        selectedIndex.value = Math.min(
          selectedIndex.value,
          Math.max(0, activeList.value.length - 1),
        )
        status.value = `categorized ${changed} conversations`
      } else if (activeMailbox.value === mailboxId && !searchActive.value) {
        status.value = 'categories checked'
      }
    } catch (error) {
      status.value = `body preload skipped: ${errorMessage(error)}`
    }
  }
  function selectCategory(category: CategoryTab) {
    activeCategory.value = category
    selectedIndex.value = 0
    focusPane.value = 'list'
    if (visualMode.value) exitVisual()
  }
  function selectConversation(conversation: Conversation) {
    const index = activeList.value.findIndex((item) => item.id === conversation.id)
    selectedIndex.value = Math.max(0, index)
    focusPane.value = 'list'
  }

  // ── Folder CRUD ────────────────────────────────────────────────────────
  const { createMailbox, renameMailbox, setMailboxIcon, deleteMailbox } = useMailboxAdmin({
    client,
    mailboxes,
    activeMailbox,
    status,
    openMailbox,
  })
  // ── Local list reconciliation ──────────────────────────────────────────
  // The client mutates server state, but the in-memory list/sidebar are
  // separate objects — patch them optimistically so the UI reflects reads,
  // archives, etc. immediately, then reload to reconcile with the backend.
  // Locate a conversation row in whichever list currently holds it.
  function findRow(id: string) {
    return (
      conversations.value.find((conversation) => conversation.id === id) ??
      searchResults.value.find((conversation) => conversation.id === id)
    )
  }
  function patchListConversation(id: string, patch: Partial<Conversation>) {
    for (const list of [conversations.value, searchResults.value]) {
      const row = list.find((conversation) => conversation.id === id)
      if (row) Object.assign(row, patch)
    }
    if (selectedThread.value?.id === id) Object.assign(selectedThread.value, patch)
  }
  function removeListConversation(id: string) {
    conversations.value = conversations.value.filter((conversation) => conversation.id !== id)
    searchResults.value = searchResults.value.filter((conversation) => conversation.id !== id)
    snoozedItems.value = snoozedItems.value.filter((conversation) => conversation.id !== id)
    if (selectedThread.value?.id === id) {
      selectedThread.value = null
      threadMessages.value = []
      focusedMessageId.value = ''
      focusPane.value = 'list'
    }
    selectedIndex.value = Math.max(0, Math.min(selectedIndex.value, activeList.value.length - 1))
  }
  function bumpMailboxUnread(mailboxId: string, delta: number) {
    const mailbox = mailboxes.value.find((item) => item.id === mailboxId)
    if (mailbox) mailbox.unread = Math.max(0, mailbox.unread + delta)
  }
  async function reloadList() {
    if (!client.value) return
    mailboxes.value = await client.value.listMailboxes()
    conversations.value = await client.value.listConversations(activeMailbox.value)
    if (searchActive.value)
      searchResults.value = await client.value.searchConversations(query.value)
    if (snoozedActive.value && client.value.listSnoozed)
      snoozedItems.value = await client.value.listSnoozed()
    selectedIndex.value = Math.max(0, Math.min(selectedIndex.value, activeList.value.length - 1))
  }

  // ── Single-thread triage ───────────────────────────────────────────────
  const {
    archiveThread,
    archiveSelected,
    snoozeThread,
    unsnoozeThread,
    moveThreadTo,
    deleteThread,
    reportSpam,
    applyLabel,
    createLabelAndApply,
    createFolderAndMove,
    toggleStar,
    toggleRead,
  } = useTriage({
    client,
    status,
    activeMailbox,
    mailboxes,
    labels,
    snoozedActive,
    selectedThread,
    conversations,
    targetThreadId,
    selectedConversation,
    findRow,
    patchListConversation,
    removeListConversation,
    bumpMailboxUnread,
    reloadList,
    recordUndo,
  })
  // Open the virtual Snoozed view (replaces the conversation list until exited).
  async function openSnoozed() {
    if (!client.value?.listSnoozed) {
      status.value = 'snooze list not supported'
      return
    }
    searchActive.value = false
    selectedThread.value = null
    threadMessages.value = []
    focusedMessageId.value = ''
    replyOpen.value = false
    composeOpen.value = false
    focusPane.value = 'list'
    snoozedActive.value = true
    status.value = 'loading snoozed'
    snoozedItems.value = await client.value.listSnoozed()
    selectedIndex.value = 0
    status.value = `${snoozedItems.value.length} snoozed`
  }
  // Leave the Snoozed view, returning to the active mailbox.
  function closeSnoozed() {
    if (!snoozedActive.value) return
    snoozedActive.value = false
    selectedIndex.value = 0
    focusPane.value = 'list'
  }
  // ── Search + saved searches ────────────────────────────────────────────
  const {
    serverSearching,
    runSearch,
    searchContacts,
    openSearch,
    closeSearch,
    searchServer,
    saveSearch,
    runSavedSearch,
    removeSavedSearch,
  } = useMailSearch({
    client,
    query,
    searchResults,
    searchActive,
    selectedIndex,
    focusPane,
    status,
    settings,
  })
  function moveSelection(delta: number) {
    focusPane.value = 'list'
    selectedIndex.value = Math.max(
      0,
      Math.min(activeList.value.length - 1, selectedIndex.value + delta),
    )
    if (selectedThread.value?.id !== selectedConversation.value?.id) {
      selectedThread.value = null
      threadMessages.value = []
      focusedMessageId.value = ''
      replyOpen.value = false
    }
  }
  function selectFirst() {
    focusPane.value = 'list'
    selectedIndex.value = 0
  }
  function selectLast() {
    focusPane.value = 'list'
    selectedIndex.value = Math.max(0, activeList.value.length - 1)
  }

  // ── Visual (multi-select) mode + batch triage ─────────────────────────
  const {
    enterVisual,
    exitVisual,
    toggleSelect,
    toggleSelectAll,
    archiveSelection,
    deleteSelection,
    snoozeSelection,
    moveSelectionTo,
    starSelection,
    toggleSelectionRead,
    labelSelection,
    createLabelAndLabelSelection,
    createFolderAndMoveSelection,
  } = useVisualSelection({
    client,
    status,
    activeMailbox,
    mailboxes,
    labels,
    focusPane,
    visualMode,
    selectedIds,
    activeList,
    selectedConversation,
    mode,
    findRow,
    patchListConversation,
    removeListConversation,
    bumpMailboxUnread,
    reloadList,
    recordUndo,
  })

  // ── Reading pane (open thread, focus, close) ───────────────────────────
  const {
    openThread,
    toggleMessageExpanded,
    focusMessage,
    focusAdjacentMessage,
    focusList,
    focusThread,
    closeThread,
  } = useThreadReader({
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
  })

  // ── Command line (`/` search, `:` ex-command, find, which-key menu) ────
  const {
    commandMenuOpen,
    openCommand,
    openFind,
    submitCommand,
    cancelCommand,
    openCommandMenu,
    closeCommandMenu,
  } = useCommandLine({
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
  })

  // Tears down the changefeed sub + pending/undo/toast timers. The singleton lives
  // for the app's lifetime, so this is mainly for tests / future multi-window.
  function dispose() {
    teardownChangefeed()
    cancelAutosave()
    clearUndo()
    clearToast()
  }

  watch(
    () => draft.value.body,
    () => queueSave(),
  )
  watch(query, () => {
    if (searchActive.value) void runSearch()
  })
  // Push notification prefs to the backend whenever they change.
  watch(
    () => settings.notify,
    () => {
      if (client.value?.source === 'wails') void applyNotifyPrefs(settings.notify)
    },
    { deep: true },
  )

  return {
    appPhase,
    client,
    account,
    configuredAccounts,
    activeMailbox,
    activeCategory,
    selectedIndex,
    selectedThread,
    mailboxes,
    labels,
    conversations,
    searchResults,
    snoozedActive,
    snoozedItems,
    visualMode,
    selectedIds,
    selectedCount,
    threadMessages,
    focusedMessageId,
    query,
    replyMode,
    replyOpen,
    replyExpanded,
    focusPane,
    status,
    threadLoading,
    composeOpen,
    searchActive,
    serverSearching,
    command,
    commandMenuOpen,
    lastAction,
    toast,
    draft,
    recipientInput,
    ccInput,
    bccInput,
    setup,
    setupStatus,
    setupError,
    setupBusy,
    filteredConversations,
    activeList,
    selectedConversation,
    unreadCount,
    todayConversations,
    earlierConversations,
    categoryCounts,
    mode,
    statusHints,
    focusedThreadMessage,
    signatureOptions,
    initializeApp,
    bootMailbox,
    submitOnboarding,
    resetSetup,
    removeAccount,
    refreshShell,
    openMailbox,
    warmMailbox,
    selectCategory,
    selectConversation,
    createMailbox,
    renameMailbox,
    setMailboxIcon,
    deleteMailbox,
    openThread,
    prepareReply,
    archiveThread,
    snoozeThread,
    openSnoozed,
    closeSnoozed,
    unsnoozeThread,
    moveThreadTo,
    deleteThread,
    reportSpam,
    applyLabel,
    createLabelAndApply,
    createFolderAndMove,
    toggleStar,
    toggleRead,
    openCommandMenu,
    closeCommandMenu,
    compose,
    sendDraft,
    discardDraft,
    materializeRecipients,
    queueSave,
    setDraftSignature,
    searchContacts,
    runSearch,
    openSearch,
    closeSearch,
    searchServer,
    saveSearch,
    runSavedSearch,
    removeSavedSearch,
    moveSelection,
    selectFirst,
    selectLast,
    archiveSelected,
    focusList,
    focusThread,
    closeThread,
    performUndo,
    clearUndo,
    showToast,
    clearToast,
    enterVisual,
    exitVisual,
    toggleSelect,
    toggleSelectAll,
    archiveSelection,
    deleteSelection,
    snoozeSelection,
    moveSelectionTo,
    starSelection,
    toggleSelectionRead,
    labelSelection,
    createLabelAndLabelSelection,
    createFolderAndMoveSelection,
    openCommand,
    openFind,
    submitCommand,
    cancelCommand,
    pickAttachments,
    attachFiles,
    attachInlineImage,
    inlineImageMap,
    downloadAttachment,
    openReply,
    toggleReplyExpanded,
    toggleMessageExpanded,
    focusMessage,
    focusAdjacentMessage,
    threadFind,
    dispose,
  }
}

export type MailShellApi = ReturnType<typeof createMailShell>

let instance: MailShellApi | null = null
export function useMailShell(): MailShellApi {
  if (!instance) instance = createMailShell()
  return instance
}
