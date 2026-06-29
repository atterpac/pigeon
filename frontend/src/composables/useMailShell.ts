// Shared mail-shell state + actions for the app-level mail workflow.
// Singleton: every component that calls useMailShell() gets the same instance,
// so state is shared without prop-drilling.
import { computed, nextTick, ref, watch } from 'vue'
import { Events } from '@wailsio/runtime'
import { createMailClient } from '../mail/client'
import { applyNotifyPrefs, applyPollInterval } from '../mail/syncSettings'
import { useSettings } from './useSettings'
import { useThreadFind } from './useThreadFind'
import type { Account, Category, Conversation, Label, Mailbox, MailClient, ThreadMessage } from '../mail/types'
import { createOnboardingClient, type ConfiguredAccount, type SetupMethod } from '../onboarding/client'
import { errorMessage, isToday, parseAddresses } from '../mail/format'
import { newDraft, replaceSignature, type ReplyMode, replyDraft, withSignature } from '../mail/drafts'
import { defaultSignatureId, signatureBody, signatureHTML, signaturesFor } from '../mail/signatures'

export type AppPhase = 'starting' | 'onboarding' | 'mail'
export type CategoryTab = Category | 'all'
export type FocusPane = 'list' | 'thread'
export type ShellToast = { id: number; kind: 'success' | 'info' | 'error'; title: string; detail?: string }

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
  const onboarding = createOnboardingClient()
  const appPhase = ref<AppPhase>('starting')
  const client = ref<MailClient | null>(null)
  const account = ref<Account | null>(null)
  const configuredAccounts = ref<ConfiguredAccount[]>([])

  const activeMailbox = ref('')
  const activeCategory = ref<CategoryTab>('all')
  const selectedIndex = ref(0)
  const selectedThread = ref<Conversation | null>(null)
  const mailboxes = ref<Mailbox[]>([])
  const labels = ref<Label[]>([])
  const conversations = ref<Conversation[]>([])
  const searchResults = ref<Conversation[]>([])
  // True while an explicit server-side search round-trip is in flight.
  const serverSearching = ref(false)
  // Snoozed view: a virtual mailbox of conversations hidden until their wake
  // time. Toggled like search — when active it replaces the conversation list.
  const snoozedActive = ref(false)
  const snoozedItems = ref<Conversation[]>([])
  // Visual (multi-select) mode: vim `v` enters it, j/k navigate, `space` toggles
  // the focused row in/out. selectedIds is the set acted on by batch triage.
  const visualMode = ref(false)
  const selectedIds = ref<Set<string>>(new Set())
  // Last reversible triage action, surfaced as a toast + `U`. `kind: 'send'` is a
  // pending outbox hold (undo cancels delivery); everything else is an inverse
  // mutation. Auto-expires after `undoTimer`.
  const lastAction = ref<{ label: string; kind: 'triage' | 'send'; undo: () => Promise<void> } | null>(null)
  let undoTimer: number | undefined
  const toast = ref<ShellToast | null>(null)
  let toastTimer: number | undefined
  const threadMessages = ref<ThreadMessage[]>([])
  const focusedMessageId = ref('')
  const query = ref('from:github is:unread')
  const replyMode = ref<ReplyMode>('reply')
  const replyOpen = ref(false)
  const replyExpanded = ref(false)
  const focusPane = ref<FocusPane>('list')
  const status = ref('loading')
  // True while a thread's messages/bodies are being fetched for the reading pane.
  const threadLoading = ref(false)
  // Monotonic token so a newer openThread cancels an older in-flight one.
  let openSeq = 0

  // Overlay / pane modes (replaced the old `screen` enum).
  const composeOpen = ref(false)
  const searchActive = ref(false)
  // Which-key command menu (assign thread → archive/snooze/label/move).
  const commandMenuOpen = ref(false)
  // Active command-line input: `/` search, `:` ex-command (vim layer), or
  // `find` (in-thread find when the reading pane is focused).
  const command = ref<{ kind: 'search' | 'ex' | 'find'; text: string } | null>(null)
  const threadFind = useThreadFind()
  // Expanded-state snapshot taken when find opens, so closing find restores the
  // conversation to exactly how it looked before.
  let findExpandedSnapshot: Map<string, boolean> | null = null
  let changefeedOff: (() => void) | null = null

  const draft = ref(newDraft())
  const recipientInput = ref('')
  const ccInput = ref('')
  const bccInput = ref('')

  // Onboarding form state.
  const setupStatus = ref('checking accounts')
  const setupError = ref('')
  const setupBusy = ref(false)
  const setup = ref({
    method: 'appPassword' as SetupMethod,
    email: '',
    displayName: '',
    appPassword: '',
    imapHost: '',
    imapPort: '',
    smtpHost: '',
    smtpPort: '',
  })

  const filteredConversations = computed(() => activeCategory.value === 'all'
    ? conversations.value
    : conversations.value.filter((conversation) => conversation.category === activeCategory.value))
  const unreadCount = computed(() => filteredConversations.value.filter((conversation) => conversation.unread).length)
  const todayConversations = computed(() => filteredConversations.value.filter((conversation) => isToday(conversation.lastAt)))
  const earlierConversations = computed(() => filteredConversations.value.filter((conversation) => !isToday(conversation.lastAt)))
  const groupedConversations = computed(() => [...todayConversations.value, ...earlierConversations.value])
  const activeList = computed(() => (snoozedActive.value ? snoozedItems.value : searchActive.value ? searchResults.value : groupedConversations.value))
  const selectedConversation = computed(() => activeList.value[selectedIndex.value] ?? null)
  const categoryCounts = computed(() => {
    const counts: Record<CategoryTab, number> = { all: conversations.value.length, primary: 0, promotions: 0, updates: 0, social: 0, forums: 0 }
    for (const conversation of conversations.value) counts[conversation.category] += 1
    return counts
  })
  const mode = computed(() => composeOpen.value ? 'COMPOSE' : searchActive.value ? 'SEARCH' : visualMode.value ? 'VISUAL' : snoozedActive.value ? 'SNOOZED' : selectedThread.value ? 'THREAD' : 'NORMAL')
  const selectedCount = computed(() => selectedIds.value.size)
  const focusedThreadMessage = computed(() => threadMessages.value.find((message) => message.id === focusedMessageId.value) ?? threadMessages.value.at(-1) ?? null)
  const statusHints = computed(() => {
    if (composeOpen.value) return '⌘↵ send · ⌘⇧A attach · esc discard'
    if (visualMode.value) return 'j k move · space select · V all · e # s * u act · ↵ menu · esc exit'
    if (snoozedActive.value) return 'j k move · ↵ open · u unsnooze · esc back'
    if (searchActive.value) return '↑↓ navigate · ↵ open · esc clear'
    if (selectedThread.value && focusPane.value === 'thread') return 'j k scroll · r reply · e archive · esc list'
    if (selectedThread.value) return 'j k move · ↵ open · tab thread · e archive'
    return 'j k move · space cmd · e archive · s snooze · c compose · ⌘K search'
  })
  const signatureOptions = computed(() => signaturesFor(settings, account.value?.id))

  async function initializeApp() {
    setupStatus.value = 'checking accounts'
    try {
      configuredAccounts.value = await onboarding.listAccounts()
    } catch (error) {
      appPhase.value = 'onboarding'
      setupStatus.value = 'account setup required'
      setupError.value = errorMessage(error)
      return
    }
    if (!configuredAccounts.value.length) {
      appPhase.value = 'onboarding'
      setupStatus.value = 'account setup required'
      return
    }
    await bootMailbox(configuredAccounts.value[0])
  }
  async function bootMailbox(configuredAccount?: ConfiguredAccount) {
    client.value = await createMailClient(configuredAccount?.id)
    account.value = configuredAccount ? accountFromConfigured(configuredAccount) : await client.value.getAccount()
    await refreshShell()
    // Push the user's saved poll interval to the backend now that sync loops
    // are running; the backend default applies until this lands.
    if (client.value.source === 'wails') {
      void applyPollInterval(useSettings().pollIntervalSeconds)
      void applyNotifyPrefs(settings.notify)
      subscribeChangefeed()
    }
    appPhase.value = 'mail'
  }
  // subscribeChangefeed reconciles the active view whenever the backend store
  // changes (background sync pulling mail, flag/label/delete mutations). Events
  // are best-effort hints, so we coalesce bursts and refetch rather than apply
  // individual ids. Registered once; survives account switches.
  function subscribeChangefeed() {
    if (changefeedOff) return
    let pending: ReturnType<typeof setTimeout> | null = null
    changefeedOff = Events.On('store:change', (ev: { data?: { account?: string } }) => {
      // Ignore changes for other accounts than the one on screen.
      if (ev.data?.account && account.value && ev.data.account !== account.value.id) return
      if (pending) return // already scheduled; coalesce the burst
      pending = setTimeout(() => { pending = null; void reloadList() }, 250)
    })
  }
  async function submitOnboarding(): Promise<boolean> {
    setupError.value = ''
    const email = setup.value.email.trim()
    if (!email) { setupError.value = 'Email address is required.'; return false }
    if (setup.value.method === 'appPassword' && !setup.value.appPassword.trim()) {
      setupError.value = 'App password is required.'; return false
    }
    if (setup.value.method === 'imap') {
      if (!setup.value.imapHost.trim()) { setupError.value = 'IMAP server is required.'; return false }
      if (!setup.value.appPassword.trim()) { setupError.value = 'Password is required.'; return false }
    }
    setupBusy.value = true
    setupStatus.value = 'verifying account'
    try {
      const added = await onboarding.addAccount(setup.value)
      configuredAccounts.value = [added, ...configuredAccounts.value.filter((item) => item.id !== added.id)]
      setup.value.appPassword = ''
      await bootMailbox(added)
      return true
    } catch (error) {
      setupError.value = errorMessage(error)
      setupStatus.value = 'setup did not finish'
      return false
    } finally {
      setupBusy.value = false
    }
  }
  // Clears the setup form before re-using it to add another account mid-session.
  function resetSetup() {
    setup.value = { method: 'appPassword', email: '', displayName: '', appPassword: '', imapHost: '', imapPort: '', smtpHost: '', smtpPort: '' }
    setupError.value = ''
    setupStatus.value = ''
  }
  // RemoveAccount forgets an account (credentials + local store via the backend)
  // and re-homes the active view if the current account was removed.
  async function removeAccount(id: string) {
    await onboarding.removeAccount(id)
    configuredAccounts.value = await onboarding.listAccounts()
    if (account.value?.id !== id) return
    if (configuredAccounts.value.length) {
      await bootMailbox(configuredAccounts.value[0])
    } else {
      client.value = null
      account.value = null
      appPhase.value = 'onboarding'
      setupStatus.value = 'account setup required'
    }
  }
  function accountFromConfigured(configuredAccount: ConfiguredAccount): Account {
    return { id: configuredAccount.id, email: configuredAccount.email, name: configuredAccount.name || configuredAccount.email }
  }
  async function refreshShell() {
    if (!client.value) return
    mailboxes.value = await client.value.listMailboxes()
    labels.value = await client.value.listLabels()
    const nextMailbox = mailboxes.value.find((mailbox) => mailbox.id === activeMailbox.value)?.id
      ?? mailboxes.value.find((mailbox) => mailbox.role === 'inbox')?.id
      ?? mailboxes.value[0]?.id
      ?? ''
    if (nextMailbox) await openMailbox(nextMailbox)
    status.value = client.value.source === 'wails' ? 'synced from local store' : 'using mock data'
  }
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
      const changed = await client.value.reclassifyMailbox?.(mailboxId, 100) ?? 0
      if (changed > 0 && activeMailbox.value === mailboxId && !searchActive.value) {
        conversations.value = await client.value.listConversations(mailboxId)
        selectedIndex.value = Math.min(selectedIndex.value, Math.max(0, activeList.value.length - 1))
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
  async function createMailbox(name: string) {
    if (!client.value?.createMailbox) return
    const created = await client.value.createMailbox(name.trim())
    mailboxes.value = await client.value.listMailboxes()
    status.value = `created ${created.name}`
    await openMailbox(created.id)
  }
  async function renameMailbox(id: string, newName: string) {
    if (!client.value?.renameMailbox) return
    const renamed = await client.value.renameMailbox(id, newName.trim())
    mailboxes.value = await client.value.listMailboxes()
    status.value = `renamed to ${renamed.name}`
    if (activeMailbox.value === id) await openMailbox(renamed.id)
  }
  async function setMailboxIcon(id: string, icon: string, weight: string, color: string) {
    if (!client.value?.setMailboxIcon) return
    await client.value.setMailboxIcon(id, icon, weight, color)
    mailboxes.value = await client.value.listMailboxes()
  }
  async function deleteMailbox(id: string) {
    if (!client.value?.deleteMailbox) return
    await client.value.deleteMailbox(id)
    mailboxes.value = await client.value.listMailboxes()
    status.value = 'folder deleted'
    if (activeMailbox.value === id) {
      const fallback = mailboxes.value.find((mailbox) => mailbox.role === 'inbox')?.id ?? mailboxes.value[0]?.id
      if (fallback) await openMailbox(fallback)
    }
  }
  // ── Local list reconciliation ──────────────────────────────────────────
  // The client mutates server state, but the in-memory list/sidebar are
  // separate objects — patch them optimistically so the UI reflects reads,
  // archives, etc. immediately, then reload to reconcile with the backend.
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
    if (selectedThread.value?.id === id) { selectedThread.value = null; threadMessages.value = []; focusedMessageId.value = ''; focusPane.value = 'list' }
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
    if (searchActive.value) searchResults.value = await client.value.searchConversations(query.value)
    if (snoozedActive.value && client.value.listSnoozed) snoozedItems.value = await client.value.listSnoozed()
    selectedIndex.value = Math.max(0, Math.min(selectedIndex.value, activeList.value.length - 1))
  }

  async function openThread(threadId = selectedConversation.value?.id) {
    if (!client.value || !threadId) return
    if (visualMode.value) exitVisual()
    const wasUnread = conversations.value.find((c) => c.id === threadId)?.unread
      ?? searchResults.value.find((c) => c.id === threadId)?.unread
    // Supersession token: only a *newer* openThread call should cancel this
    // one. Comparing against selectedConversation here is wrong — background
    // warming/reclassify can shift the selection mid-await and would falsely
    // abort a legitimate open, flashing the reader back to the preview.
    const seq = ++openSeq
    // A new conversation invalidates any active find session.
    if (command.value?.kind === 'find') command.value = null
    threadFind.close()
    findExpandedSnapshot = null
    threadLoading.value = true
    status.value = 'loading thread'
    const tStart = performance.now()
    try {
      const thread = await client.value.getThread(threadId)
      if (seq !== openSeq) return // a newer open superseded this one
      const tFetched = performance.now()
      selectedThread.value = thread.conversation
      threadMessages.value = thread.messages.map((message, index, messages) => ({ ...message, expanded: message.expanded || index === messages.length - 1 }))
      focusedMessageId.value = threadMessages.value.at(-1)?.id ?? ''
      composeOpen.value = false
      replyOpen.value = false
      replyExpanded.value = false
      focusPane.value = 'thread'
      status.value = 'thread loaded'
      // getThread marks the thread read server-side; mirror that locally.
      if (wasUnread) { patchListConversation(threadId, { unread: false }); bumpMailboxUnread(activeMailbox.value, -1) }
      prepareReply('reply')
      // fetch = getThread (backend+IPC+prep); render = Vue DOM patch after state
      // assignment. Splits a slow open into backend vs. frontend render cost.
      await nextTick()
      const tRendered = performance.now()
      console.debug(`[timing] openThread fetch=${(tFetched - tStart).toFixed(0)}ms render=${(tRendered - tFetched).toFixed(0)}ms total=${(tRendered - tStart).toFixed(0)}ms`)
    } finally {
      if (seq === openSeq) threadLoading.value = false
    }
  }
  function prepareReply(replyKind: ReplyMode) {
    replyMode.value = replyKind
    draft.value = replyDraft(replyKind, selectedThread.value, threadMessages.value, account.value?.email ?? '')
    draft.value.signatureId = defaultSignatureId(settings, account.value?.id)
  }
  function openReply(replyKind: ReplyMode) {
    prepareReply(replyKind)
    // Add the signature only when the user actually opens a reply, not on the
    // auto-prep that runs for every opened thread.
    draft.value.body = withSignature(draft.value.body, signatureFor(draft.value.signatureId))
    replyOpen.value = true
  }
  function toggleReplyExpanded() {
    replyExpanded.value = !replyExpanded.value
  }
  function toggleMessageExpanded(id: string) {
    focusMessage(id)
    const message = threadMessages.value.find((item) => item.id === id)
    if (message) message.expanded = !message.expanded
  }
  function focusMessage(id: string) {
    if (threadMessages.value.some((message) => message.id === id)) focusedMessageId.value = id
  }
  // Step message focus within the open thread (shift-J/K). Expands the newly
  // focused message so its body is visible.
  function focusAdjacentMessage(delta: number) {
    const messages = threadMessages.value
    if (!messages.length) return
    const currentIndex = messages.findIndex((message) => message.id === focusedMessageId.value)
    const nextIndex = Math.max(0, Math.min(messages.length - 1, (currentIndex < 0 ? 0 : currentIndex) + delta))
    const next = messages[nextIndex]
    if (!next) return
    focusedMessageId.value = next.id
    next.expanded = true
  }
  async function moveOut(id: string | undefined, op: (id: string) => Promise<void>, label: string) {
    if (!client.value || !id) return
    const wasUnread = conversations.value.find((c) => c.id === id)?.unread
    removeListConversation(id)
    if (wasUnread) bumpMailboxUnread(activeMailbox.value, -1)
    status.value = label
    try { await op(id) } finally { await reloadList() }
  }
  // ── Undo ───────────────────────────────────────────────────────────────
  // Consecutive triage actions of the same verb (e.g. archiving a few in a row)
  // collapse into one undo entry, so a single `U` reverses the whole recent run
  // rather than just the last item.
  let undoChain: Array<() => Promise<void>> = []
  let undoVerb = ''
  let undoCount = 0
  // Registers the inverse of the action just taken; the toast + `U` invoke it.
  // `count` is how many conversations the action covered (batch ops pass >1).
  function recordUndo(verb: string, undo: () => Promise<void>, kind: 'triage' | 'send' = 'triage', count = 1, ttl = 8000) {
    if (kind === 'triage' && undoVerb === verb && lastAction.value?.kind === 'triage') {
      undoChain.push(undo)
      undoCount += count
    } else {
      undoChain = [undo]
      undoVerb = verb
      undoCount = count
    }
    const chain = undoChain.slice()
    const label = kind === 'triage' && undoCount > 1 ? `${verb} ${undoCount}` : verb
    // Reverse so the most recent action is undone first.
    lastAction.value = { label, kind, undo: async () => { for (const fn of chain.slice().reverse()) await fn() } }
    if (undoTimer) window.clearTimeout(undoTimer)
    undoTimer = window.setTimeout(clearUndo, ttl)
  }
  function clearUndo() {
    lastAction.value = null
    undoChain = []
    undoVerb = ''
    undoCount = 0
    if (undoTimer) window.clearTimeout(undoTimer)
  }
  function showToast(next: Omit<ShellToast, 'id'>, timeout = 3200) {
    if (toastTimer) window.clearTimeout(toastTimer)
    toast.value = { ...next, id: Date.now() }
    toastTimer = window.setTimeout(() => {
      toast.value = null
      toastTimer = undefined
    }, timeout)
  }
  function clearToast() {
    if (toastTimer) window.clearTimeout(toastTimer)
    toastTimer = undefined
    toast.value = null
  }
  async function performUndo() {
    const action = lastAction.value
    if (!action) return
    clearUndo()
    status.value = 'undoing…'
    try {
      await action.undo()
      status.value = 'undone'
    } catch (error) {
      status.value = `undo failed: ${errorMessage(error)}`
    } finally {
      if (action.kind === 'triage') await reloadList()
    }
  }
  // Inverse helper: move a thread (back) into a mailbox, if supported.
  function moveBack(id: string, dst: string) {
    return async () => { if (client.value?.moveThread) await client.value.moveThread(id, dst) }
  }
  async function archiveThread() {
    const id = selectedThread.value?.id
    const origin = activeMailbox.value
    await moveOut(id, (i) => client.value!.archiveThread(i), 'archived')
    if (id) recordUndo('Archived', moveBack(id, origin))
  }
  async function snoozeThread(until?: string) {
    const id = selectedThread.value?.id ?? selectedConversation.value?.id
    await moveOut(id, (i) => client.value!.snoozeThread(i, until), 'snoozed')
    if (id && client.value?.unsnooze) recordUndo('Snoozed', () => client.value!.unsnooze!(id))
  }
  // Open the virtual Snoozed view (replaces the conversation list until exited).
  async function openSnoozed() {
    if (!client.value?.listSnoozed) { status.value = 'snooze list not supported'; return }
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
  // Wake a snoozed thread now (returns it to the inbox). Defaults to the
  // selected row in the Snoozed view.
  async function unsnoozeThread(threadId = selectedConversation.value?.id) {
    if (!client.value?.unsnooze || !threadId) return
    removeListConversation(threadId)
    status.value = 'unsnoozed'
    try { await client.value.unsnooze(threadId) } finally { if (snoozedActive.value) await reloadList() }
  }
  // Move the selected/open thread into a folder (mailbox).
  async function moveThreadTo(mailboxId: string) {
    const id = selectedThread.value?.id ?? selectedConversation.value?.id
    const origin = activeMailbox.value
    if (!client.value?.moveThread) { status.value = 'move not supported'; return }
    if (!id) { status.value = 'move: nothing selected'; return }
    // Call moveThread as a method so `this` stays bound (the wails client reads
    // this.account); extracting it into a variable detaches `this`.
    await moveOut(id, (mid) => client.value!.moveThread!(mid, mailboxId), `moved → ${mailboxId}`)
    recordUndo('Moved', moveBack(id, origin))
  }
  // Delete the selected/open thread (moves it to Trash — reversible).
  async function deleteThread() {
    const id = selectedThread.value?.id ?? selectedConversation.value?.id
    const origin = activeMailbox.value
    if (!client.value?.deleteThread) { status.value = 'delete not supported'; return }
    await moveOut(id, (tid) => client.value!.deleteThread!(tid), 'deleted')
    if (id) recordUndo('Deleted', moveBack(id, origin))
  }
  // Report the selected/open thread as spam by moving it to the Spam mailbox.
  async function reportSpam() {
    const id = selectedThread.value?.id ?? selectedConversation.value?.id
    const origin = activeMailbox.value
    const spam = mailboxes.value.find((mailbox) => mailbox.role === 'spam')
    if (!client.value?.moveThread) { status.value = 'move not supported'; return }
    if (!spam) { status.value = 'no spam folder'; return }
    if (!id) { status.value = 'spam: nothing selected'; return }
    await moveOut(id, (tid) => client.value!.moveThread!(tid, spam.id), 'reported spam')
    recordUndo('Reported spam', moveBack(id, origin))
  }
  // Apply a label without removing the thread from its mailbox.
  async function applyLabel(labelId: string) {
    const id = selectedThread.value?.id ?? selectedConversation.value?.id
    if (!client.value?.applyLabel || !id) return
    const row = conversations.value.find((c) => c.id === id) ?? searchResults.value.find((c) => c.id === id)
    if (row && !row.labelIds.includes(labelId)) row.labelIds = [...row.labelIds, labelId]
    status.value = 'labelled'
    try { await client.value.applyLabel(id, labelId) } finally { await reloadList() }
  }
  async function createLabelAndApply(name: string) {
    if (!client.value?.createLabel) { status.value = 'labels not supported'; return }
    const label = await client.value.createLabel(name)
    labels.value = await client.value.listLabels()
    await applyLabel(label.id)
  }
  // Create a folder and move the selected thread into it in one step.
  async function createFolderAndMove(name: string) {
    if (!client.value?.createMailbox) { status.value = 'folders not supported'; return }
    const created = await client.value.createMailbox(name.trim())
    mailboxes.value = await client.value.listMailboxes()
    await moveThreadTo(created.id)
  }
  async function toggleStar(conversation: Conversation | null = selectedThread.value ?? selectedConversation.value) {
    if (!client.value || !conversation) return
    const next = !conversation.starred
    conversation.starred = next
    patchListConversation(conversation.id, { starred: next })
    await client.value.toggleStar(conversation.id, next)
  }
  async function toggleRead() {
    if (!client.value || !selectedThread.value) return
    const id = selectedThread.value.id
    const read = selectedThread.value.unread
    patchListConversation(id, { unread: !read })
    bumpMailboxUnread(activeMailbox.value, read ? -1 : 1)
    await client.value.markThreadRead(id, read)
  }
  // The active account's signature (empty when none configured).
  function signatureFor(id = defaultSignatureId(settings, account.value?.id)) {
    return signatureBody(settings, account.value?.id, id)
  }
  function signatureHtmlFor(id = defaultSignatureId(settings, account.value?.id)) {
    return signatureHTML(settings, account.value?.id, id)
  }
  function draftWithDefaultSignature() {
    const signatureId = defaultSignatureId(settings, account.value?.id)
    return newDraft({ signatureId, signatureHtml: signatureHtmlFor(signatureId), body: withSignature('', signatureFor(signatureId)) })
  }
  function setDraftSignature(id: string) {
    draft.value.signatureId = id
    draft.value.signatureHtml = signatureHtmlFor(id)
    draft.value.body = replaceSignature(draft.value.body, signatureFor(id))
  }
  function compose() {
    draft.value = draftWithDefaultSignature()
    recipientInput.value = ''
    ccInput.value = ''
    bccInput.value = ''
    composeOpen.value = true
  }
  async function sendDraft() {
    if (!client.value) return
    const outgoing = materializeRecipients()
    if (!outgoing.to.length && composeOpen.value) {
      status.value = 'add at least one recipient'
      return
    }
    const hold = Math.max(0, settings.sendUndoSeconds | 0)
    // Snapshot the draft so "Undo send" can reopen it for editing after cancel.
    const restore = { ...draft.value }
    const opId = await client.value.sendDraft(outgoing, hold)
    composeOpen.value = false
    draft.value = draftWithDefaultSignature()
    if (opId && hold > 0 && client.value.cancelSend) {
      status.value = 'sending…'
      recordUndo('Sending…', async () => {
        await client.value!.cancelSend!(opId)
        draft.value = restore
        composeOpen.value = true
        status.value = 'send cancelled'
      }, 'send', 1, hold * 1000)
    } else {
      status.value = 'sent'
    }
  }
  async function discardDraft() {
    if (client.value) await client.value.discardDraft(draft.value.id)
    draft.value = draftWithDefaultSignature()
    composeOpen.value = false
  }
  // Fold any unconfirmed text in the To/Cc/Bcc inputs into the draft's address
  // lists before saving or sending, so a recipient typed but not "chipped" isn't
  // dropped.
  function materializeRecipients() {
    const to = parseAddresses(recipientInput.value)
    if (to.length) draft.value.to = [...draft.value.to, ...to]
    const cc = parseAddresses(ccInput.value)
    if (cc.length) draft.value.cc = [...draft.value.cc, ...cc]
    const bcc = parseAddresses(bccInput.value)
    if (bcc.length) draft.value.bcc = [...draft.value.bcc, ...bcc]
    recipientInput.value = ''
    ccInput.value = ''
    bccInput.value = ''
    return draft.value
  }
  let saveTimer: number | undefined
  function queueSave() {
    if (!client.value || (!composeOpen.value && !selectedThread.value)) return
    status.value = 'saving...'
    if (saveTimer) window.clearTimeout(saveTimer)
    saveTimer = window.setTimeout(async () => {
      if (!client.value) return
      draft.value = await client.value.saveDraft(materializeRecipients())
      status.value = 'draft saved'
    }, 350)
  }
  async function runSearch() {
    if (!client.value) return
    searchResults.value = await client.value.searchConversations(query.value)
    selectedIndex.value = 0
  }
  // Recipient autocomplete: ranked address-book matches for a typed prefix.
  // Returns [] when the client predates the feature or the query is empty.
  async function searchContacts(prefix: string) {
    const trimmed = prefix.trim()
    if (!trimmed || !client.value?.searchContacts) return []
    try {
      return await client.value.searchContacts(trimmed)
    } catch (error) {
      console.warn('contact search failed', error)
      return []
    }
  }
  async function openSearch() {
    searchActive.value = true
    await runSearch()
    await nextTick()
  }
  function closeSearch() {
    searchActive.value = false
    selectedIndex.value = 0
    focusPane.value = 'list'
  }
  // Reach mail not synced locally: run the same query on the server and merge any
  // new hits into the (local) results. Explicit — one network round-trip.
  async function searchServer() {
    if (!client.value?.searchServer) { status.value = 'server search not supported'; return }
    if (!searchActive.value) await openSearch()
    if (!query.value.trim()) { status.value = 'type a query first'; return }
    serverSearching.value = true
    status.value = 'searching server…'
    try {
      const hits = await client.value.searchServer(query.value)
      const seen = new Set(searchResults.value.map((conversation) => conversation.id))
      const fresh = hits.filter((conversation) => !seen.has(conversation.id))
      searchResults.value = [...searchResults.value, ...fresh].sort((left, right) => Date.parse(right.lastAt) - Date.parse(left.lastAt))
      status.value = fresh.length ? `found ${fresh.length} more on server` : 'no additional results on server'
    } catch (error) {
      status.value = `server search failed: ${errorMessage(error)}`
    } finally {
      serverSearching.value = false
    }
  }
  // ── Saved searches ─────────────────────────────────────────────────────
  function saveSearch(name?: string) {
    const q = query.value.trim()
    if (!q) { status.value = 'nothing to save'; return }
    const label = (name || q).trim()
    const entry = { name: label, query: q }
    const index = settings.savedSearches.findIndex((item) => item.name === label)
    if (index >= 0) settings.savedSearches[index] = entry
    else settings.savedSearches.push(entry)
    status.value = `saved search “${label}”`
  }
  function runSavedSearch(savedQuery: string) {
    query.value = savedQuery
    void openSearch()
  }
  function removeSavedSearch(name: string) {
    settings.savedSearches = settings.savedSearches.filter((item) => item.name !== name)
  }
  function moveSelection(delta: number) {
    focusPane.value = 'list'
    selectedIndex.value = Math.max(0, Math.min(activeList.value.length - 1, selectedIndex.value + delta))
    if (selectedThread.value?.id !== selectedConversation.value?.id) {
      selectedThread.value = null
      threadMessages.value = []
      focusedMessageId.value = ''
      replyOpen.value = false
    }
  }
  function selectFirst() { focusPane.value = 'list'; selectedIndex.value = 0 }
  function selectLast() { focusPane.value = 'list'; selectedIndex.value = Math.max(0, activeList.value.length - 1) }
  function focusList() { focusPane.value = 'list' }
  function focusThread() { if (selectedThread.value) focusPane.value = 'thread' }
  function closeThread() {
    selectedThread.value = null
    threadMessages.value = []
    focusedMessageId.value = ''
    replyOpen.value = false
    replyExpanded.value = false
    focusPane.value = 'list'
    if (command.value?.kind === 'find') command.value = null
    threadFind.close()
    findExpandedSnapshot = null
  }
  async function archiveSelected() {
    const id = selectedThread.value?.id ?? selectedConversation.value?.id
    const origin = activeMailbox.value
    await moveOut(id, (i) => client.value!.archiveThread(i), 'archived')
    if (id) recordUndo('Archived', moveBack(id, origin))
  }

  // ── Visual (multi-select) mode ─────────────────────────────────────────
  // Entering Visual mode selects nothing — j/k just navigate, and the user opts
  // rows in with space (cherry-pick).
  function enterVisual() {
    if (!activeList.value.length) return
    visualMode.value = true
    focusPane.value = 'list'
    selectedIds.value = new Set()
    status.value = 'VISUAL'
  }
  function exitVisual() {
    if (!visualMode.value) return
    visualMode.value = false
    selectedIds.value = new Set()
    status.value = mode.value.toLowerCase()
  }
  // Toggle the focused row's membership (the spacebar action).
  function toggleSelect(id = selectedConversation.value?.id) {
    if (!id) return
    const next = new Set(selectedIds.value)
    if (next.has(id)) next.delete(id)
    else next.add(id)
    selectedIds.value = next
  }
  function toggleSelectAll() {
    const ids = activeList.value.map((conversation) => conversation.id)
    selectedIds.value = selectedIds.value.size >= ids.length && ids.length > 0 ? new Set() : new Set(ids)
  }
  function selectionConversations() {
    return activeList.value.filter((conversation) => selectedIds.value.has(conversation.id))
  }
  // Runs a triage op over the selection: optimistically drops the rows (for
  // moves) then calls the batch client method, falling back to per-thread calls.
  async function runSelectionMoveOut(label: string, batch: ((ids: string[]) => Promise<void>) | undefined, single: (id: string) => Promise<void>, undoLabel?: string, undo?: (ids: string[]) => () => Promise<void>) {
    const ids = [...selectedIds.value]
    if (!client.value || !ids.length) return
    for (const id of ids) removeListConversation(id)
    status.value = `${label} ${ids.length}`
    exitVisual()
    try {
      if (batch) await batch(ids)
      else for (const id of ids) await single(id)
    } finally { await reloadList() }
    if (undoLabel && undo) recordUndo(undoLabel, undo(ids), 'triage', ids.length)
  }
  // Inverse for a batch move-out: send every thread back to `dst`, using the
  // batch client method when present.
  function moveBackAll(dst: string) {
    return (ids: string[]) => async () => {
      if (client.value?.moveThreads) await client.value.moveThreads(ids, dst)
      else if (client.value?.moveThread) for (const id of ids) await client.value.moveThread(id, dst)
    }
  }
  async function archiveSelection() {
    const origin = activeMailbox.value
    await runSelectionMoveOut('archived', client.value?.archiveThreads && ((ids) => client.value!.archiveThreads!(ids)), (id) => client.value!.archiveThread(id), 'Archived', moveBackAll(origin))
  }
  async function deleteSelection() {
    if (!client.value?.deleteThread && !client.value?.deleteThreads) { status.value = 'delete not supported'; return }
    const origin = activeMailbox.value
    await runSelectionMoveOut('deleted', client.value?.deleteThreads && ((ids) => client.value!.deleteThreads!(ids)), (id) => client.value!.deleteThread!(id), 'Deleted', moveBackAll(origin))
  }
  async function snoozeSelection(until?: string) {
    const undo = client.value?.unsnooze
      ? (ids: string[]) => async () => { for (const id of ids) await client.value!.unsnooze!(id) }
      : undefined
    await runSelectionMoveOut('snoozed', client.value?.snoozeThreads && ((ids) => client.value!.snoozeThreads!(ids, until)), (id) => client.value!.snoozeThread(id, until), undo ? 'Snoozed' : undefined, undo)
  }
  async function moveSelectionTo(mailboxId: string) {
    if (!client.value?.moveThread && !client.value?.moveThreads) { status.value = 'move not supported'; return }
    const origin = activeMailbox.value
    await runSelectionMoveOut(`moved → ${mailboxId}`, client.value?.moveThreads && ((ids) => client.value!.moveThreads!(ids, mailboxId)), (id) => client.value!.moveThread!(id, mailboxId), 'Moved', moveBackAll(origin))
  }
  // Star/read don't remove rows — patch in place, then call the batch method.
  async function starSelection() {
    const rows = selectionConversations()
    if (!client.value || !rows.length) return
    const on = !rows.every((conversation) => conversation.starred)
    const ids = rows.map((conversation) => conversation.id)
    for (const id of ids) patchListConversation(id, { starred: on })
    status.value = `${on ? 'starred' : 'unstarred'} ${ids.length}`
    try {
      if (client.value.starThreads) await client.value.starThreads(ids, on)
      else for (const id of ids) await client.value.toggleStar(id, on)
    } finally { exitVisual() }
  }
  async function toggleSelectionRead() {
    const rows = selectionConversations()
    if (!client.value || !rows.length) return
    // If every selected row is already read, mark unread; otherwise mark read.
    const read = rows.some((conversation) => conversation.unread)
    const ids = rows.map((conversation) => conversation.id)
    for (const id of ids) {
      const wasUnread = (conversations.value.find((c) => c.id === id) ?? searchResults.value.find((c) => c.id === id))?.unread
      patchListConversation(id, { unread: !read })
      if (wasUnread !== !read) bumpMailboxUnread(activeMailbox.value, read ? -1 : 1)
    }
    status.value = `marked ${ids.length} ${read ? 'read' : 'unread'}`
    try {
      if (client.value.markThreadsRead) await client.value.markThreadsRead(ids, read)
      else for (const id of ids) await client.value.markThreadRead(id, read)
    } finally { exitVisual() }
  }
  async function createLabelAndLabelSelection(name: string) {
    if (!client.value?.createLabel) { status.value = 'labels not supported'; return }
    const label = await client.value.createLabel(name)
    labels.value = await client.value.listLabels()
    await labelSelection(label.id)
  }
  async function createFolderAndMoveSelection(name: string) {
    if (!client.value?.createMailbox) { status.value = 'folders not supported'; return }
    const created = await client.value.createMailbox(name.trim())
    mailboxes.value = await client.value.listMailboxes()
    await moveSelectionTo(created.id)
  }
  async function labelSelection(labelId: string) {
    const ids = [...selectedIds.value]
    if (!client.value || !ids.length) return
    for (const id of ids) {
      const row = conversations.value.find((c) => c.id === id) ?? searchResults.value.find((c) => c.id === id)
      if (row && !row.labelIds.includes(labelId)) row.labelIds = [...row.labelIds, labelId]
    }
    status.value = `labelled ${ids.length}`
    try {
      if (client.value.labelThreads) await client.value.labelThreads(ids, labelId)
      else if (client.value.applyLabel) for (const id of ids) await client.value.applyLabel(id, labelId)
    } finally { exitVisual(); await reloadList() }
  }

  // ── Command line (`/` search, `:` ex-command) ──────────────────────────
  function openCommand(kind: 'search' | 'ex') {
    if (kind === 'search') {
      command.value = { kind, text: query.value }
      searchActive.value = true
      void runSearch()
    } else {
      command.value = { kind, text: '' }
    }
  }
  // `/` in the thread pane finds within the open conversation instead of
  // running a mailbox-wide search.
  function openFind() {
    if (!selectedThread.value) return false
    // Expand every message so find covers the whole conversation, not just the
    // messages the user happened to have open. Snapshot first so closing find
    // restores the prior collapsed/expanded layout. Newly-rendered iframes are
    // picked up by the find engine as they load.
    findExpandedSnapshot = new Map(threadMessages.value.map((m) => [m.id, m.expanded]))
    for (const message of threadMessages.value) message.expanded = true
    command.value = { kind: 'find', text: '' }
    threadFind.open()
    return true
  }
  function restoreFindExpansion() {
    if (!findExpandedSnapshot) return
    for (const message of threadMessages.value) {
      const prev = findExpandedSnapshot.get(message.id)
      if (prev !== undefined) message.expanded = prev
    }
    findExpandedSnapshot = null
  }
  function submitCommand() {
    const current = command.value
    // Find stays open on Enter — Enter just jumps to the next match.
    if (current?.kind === 'find') { threadFind.next(); return }
    command.value = null
    if (current?.kind === 'ex') runEx(current.text)
    // search results persist; selection moves to the list.
  }
  function cancelCommand() {
    if (command.value?.kind === 'search') closeSearch()
    if (command.value?.kind === 'find') { threadFind.close(); restoreFindExpansion() }
    command.value = null
  }
  function runEx(text: string) {
    const cmd = text.trim().replace(/^:/, '')
    if (cmd === 'archive') void archiveSelected()
    else if (cmd === 'delete' || cmd === 'd') void deleteThread()
    else if (cmd === 'spam') void reportSpam()
    else if (cmd === 'snooze') void snoozeThread()
    else if (cmd === 'w' || cmd === 'write') { void queueSave(); status.value = 'draft saved' }
    else if (cmd === 'q' || cmd === 'quit') closeThread()
    else if (cmd.startsWith('label ')) { query.value = `label:${cmd.slice(6).trim()}`; void openSearch() }
    else if (cmd === 'server' || cmd === 'search!') void searchServer()
    else if (cmd === 'save' || cmd.startsWith('save ')) saveSearch(cmd.slice(4).trim())
    else status.value = `E492: not an editor command: ${cmd}`
  }
  function openCommandMenu() {
    if (!selectedThread.value && !selectedConversation.value) return
    commandMenuOpen.value = true
  }
  function closeCommandMenu() { commandMenuOpen.value = false }
  // Total attachment payload cap per message. SMTP servers commonly reject
  // anything much larger once base64 expansion (~33%) is accounted for.
  const MAX_ATTACHMENT_BYTES = 25 * 1024 * 1024
  // Opens the OS file picker and queues the chosen files onto the active draft.
  // Works from both compose surfaces (modal + reply panel) without a DOM ref by
  // creating a transient input element.
  function pickAttachments() {
    const input = document.createElement('input')
    input.type = 'file'
    input.multiple = true
    input.addEventListener('change', () => { void attachFiles(input.files) })
    input.click()
  }
  async function attachFiles(files: FileList | File[] | null) {
    const list = files ? Array.from(files) : []
    if (!list.length) return
    let total = draft.value.attachments.reduce((sum, item) => sum + (item.size ?? 0), 0)
    for (const file of list) {
      if (total + file.size > MAX_ATTACHMENT_BYTES) {
        status.value = `attachment skipped: ${file.name} exceeds 25 MB limit`
        continue
      }
      try {
        const content = await readFileBase64(file)
        draft.value.attachments.push({ filename: file.name, contentType: file.type || 'application/octet-stream', content, size: file.size })
        total += file.size
      } catch (error) {
        status.value = `attachment failed: ${errorMessage(error)}`
      }
    }
    const count = draft.value.attachments.length
    status.value = `${count} attachment${count === 1 ? '' : 's'} queued`
    queueSave()
  }
  // Saves a received attachment via the backend. prompt=false drops it straight
  // into Downloads; prompt=true opens a native "Save as" dialog.
  async function downloadAttachment(messageId: string, index: number, prompt = false, filename = 'Attachment') {
    if (!client.value?.saveAttachment) {
      status.value = 'download not supported'
      showToast({ kind: 'error', title: 'Download not supported', detail: filename })
      return
    }
    status.value = prompt ? 'choose where to save…' : 'saving attachment…'
    try {
      const path = await client.value.saveAttachment(messageId, index, prompt)
      if (path) {
        status.value = `saved → ${path}`
        showToast({ kind: 'success', title: 'File downloaded', detail: filename })
      } else {
        status.value = 'save cancelled'
        showToast({ kind: 'info', title: 'Download cancelled', detail: filename })
      }
    } catch (error) {
      const message = errorMessage(error)
      status.value = `save failed: ${message}`
      showToast({ kind: 'error', title: 'Download failed', detail: message })
    }
  }
  // Embeds an image into the draft as an inline part (referenced from the body
  // via cid:<contentId>), distinct from a file attachment. The editor inserts the
  // matching ![](cid:…) markdown; this stores the bytes.
  async function attachInlineImage(file: File, contentId: string) {
    try {
      const content = await readFileBase64(file)
      draft.value.attachments.push({ filename: file.name || `${contentId}.png`, contentType: file.type || 'image/png', content, size: file.size, contentId })
      queueSave()
    } catch (error) {
      status.value = `inline image failed: ${errorMessage(error)}`
    }
  }
  // cid → inline image (base64) for the current draft, so the compose preview can
  // resolve its own cid: refs to data URLs.
  const inlineImageMap = computed<Record<string, { contentType: string; content: string }>>(() => {
    const map: Record<string, { contentType: string; content: string }> = {}
    for (const attachment of draft.value.attachments) {
      if (attachment.contentId && attachment.content) map[attachment.contentId] = { contentType: attachment.contentType || 'image/png', content: attachment.content }
    }
    return map
  })
  // Reads a File into base64 (the encoding the Outfile binding decodes back to
  // []byte), stripping the data-URL prefix readAsDataURL adds.
  function readFileBase64(file: File): Promise<string> {
    return new Promise((resolve, reject) => {
      const reader = new FileReader()
      reader.onerror = () => reject(reader.error ?? new Error('read failed'))
      reader.onload = () => resolve(String(reader.result).split(',')[1] ?? '')
      reader.readAsDataURL(file)
    })
  }

  watch(() => draft.value.body, () => queueSave())
  watch(query, () => { if (searchActive.value) void runSearch() })
  // Push notification prefs to the backend whenever they change.
  watch(() => settings.notify, () => { if (client.value?.source === 'wails') void applyNotifyPrefs(settings.notify) }, { deep: true })

  return {
    appPhase, client, account, configuredAccounts,
    activeMailbox, activeCategory, selectedIndex, selectedThread,
    mailboxes, labels, conversations, searchResults, snoozedActive, snoozedItems, visualMode, selectedIds, selectedCount, threadMessages, focusedMessageId,
    query, replyMode, replyOpen, replyExpanded, focusPane, status, threadLoading, composeOpen, searchActive, serverSearching, command, commandMenuOpen, lastAction, toast,
    draft, recipientInput, ccInput, bccInput, setup, setupStatus, setupError, setupBusy,
    filteredConversations, activeList, selectedConversation, unreadCount,
    todayConversations, earlierConversations, categoryCounts, mode, statusHints, focusedThreadMessage, signatureOptions,
    initializeApp, bootMailbox, submitOnboarding, resetSetup, removeAccount, refreshShell, openMailbox, warmMailbox,
    selectCategory, selectConversation, createMailbox, renameMailbox, setMailboxIcon, deleteMailbox,
    openThread, prepareReply, archiveThread, snoozeThread, openSnoozed, closeSnoozed, unsnoozeThread, moveThreadTo, deleteThread, reportSpam, applyLabel, createLabelAndApply, createFolderAndMove, toggleStar, toggleRead,
    openCommandMenu, closeCommandMenu,
    compose, sendDraft, discardDraft, materializeRecipients, queueSave, setDraftSignature, searchContacts, runSearch, openSearch, closeSearch, searchServer, saveSearch, runSavedSearch, removeSavedSearch,
    moveSelection, selectFirst, selectLast, archiveSelected, focusList, focusThread, closeThread,
    performUndo, clearUndo, showToast, clearToast,
    enterVisual, exitVisual, toggleSelect, toggleSelectAll,
    archiveSelection, deleteSelection, snoozeSelection, moveSelectionTo, starSelection, toggleSelectionRead, labelSelection, createLabelAndLabelSelection, createFolderAndMoveSelection,
    openCommand, openFind, submitCommand, cancelCommand, pickAttachments, attachFiles, attachInlineImage, inlineImageMap, downloadAttachment, openReply, toggleReplyExpanded, toggleMessageExpanded, focusMessage, focusAdjacentMessage,
    threadFind,
  }
}

export type MailShellApi = ReturnType<typeof createMailShell>

let instance: MailShellApi | null = null
export function useMailShell(): MailShellApi {
  if (!instance) instance = createMailShell()
  return instance
}
