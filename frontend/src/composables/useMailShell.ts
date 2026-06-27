// Shared mail-shell state + actions for the app-level mail workflow.
// Singleton: every component that calls useMailShell() gets the same instance,
// so state is shared without prop-drilling.
import { computed, nextTick, ref, watch } from 'vue'
import { Events } from '@wailsio/runtime'
import { createMailClient } from '../mail/client'
import { applyPollInterval } from '../mail/syncSettings'
import { useSettings } from './useSettings'
import { useThreadFind } from './useThreadFind'
import type { Account, Category, Conversation, Label, Mailbox, MailClient, ThreadMessage } from '../mail/types'
import { createOnboardingClient, type ConfiguredAccount, type SetupMethod } from '../onboarding/client'
import { errorMessage, isToday, parseAddresses } from '../mail/format'
import { newDraft, replyDraft, type ReplyMode } from '../mail/drafts'

export type AppPhase = 'starting' | 'onboarding' | 'mail'
export type CategoryTab = Category | 'all'
export type FocusPane = 'list' | 'thread'

export const categoryTabs: Array<{ id: CategoryTab; label: string }> = [
  { id: 'all', label: 'All' },
  { id: 'primary', label: 'Primary' },
  { id: 'promotions', label: 'Promotions' },
  { id: 'updates', label: 'Updates' },
  { id: 'social', label: 'Social' },
  { id: 'forums', label: 'Forums' },
]

function createMailShell() {
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

  // Onboarding form state.
  const setupStatus = ref('checking accounts')
  const setupError = ref('')
  const setupBusy = ref(false)
  const setup = ref({
    method: 'google' as SetupMethod,
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
  const activeList = computed(() => (searchActive.value ? searchResults.value : groupedConversations.value))
  const selectedConversation = computed(() => activeList.value[selectedIndex.value] ?? null)
  const categoryCounts = computed(() => {
    const counts: Record<CategoryTab, number> = { all: conversations.value.length, primary: 0, promotions: 0, updates: 0, social: 0, forums: 0 }
    for (const conversation of conversations.value) counts[conversation.category] += 1
    return counts
  })
  const mode = computed(() => composeOpen.value ? 'COMPOSE' : searchActive.value ? 'SEARCH' : selectedThread.value ? 'THREAD' : 'NORMAL')
  const focusedThreadMessage = computed(() => threadMessages.value.find((message) => message.id === focusedMessageId.value) ?? threadMessages.value.at(-1) ?? null)
  const statusHints = computed(() => {
    if (composeOpen.value) return '⌘↵ send · ⌘⇧A attach · esc discard'
    if (searchActive.value) return '↑↓ navigate · ↵ open · esc clear'
    if (selectedThread.value && focusPane.value === 'thread') return 'j k scroll · r reply · e archive · esc list'
    if (selectedThread.value) return 'j k move · ↵ open · tab thread · e archive'
    return 'j k move · space cmd · e archive · s snooze · c compose · ⌘K search'
  })

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
    setupStatus.value = setup.value.method === 'google' ? 'waiting for Google authorization' : 'verifying account'
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
    setup.value = { method: 'google', email: '', displayName: '', appPassword: '', imapHost: '', imapPort: '', smtpHost: '', smtpPort: '' }
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
    void warmMailbox(mailboxId)
  }
  async function warmMailbox(mailboxId: string) {
    if (!client.value) return
    try {
      await client.value.preloadMailboxBodies?.(mailboxId, 40)
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
    selectedIndex.value = Math.max(0, Math.min(selectedIndex.value, activeList.value.length - 1))
  }

  async function openThread(threadId = selectedConversation.value?.id) {
    if (!client.value || !threadId) return
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
    try {
      const thread = await client.value.getThread(threadId)
      if (seq !== openSeq) return // a newer open superseded this one
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
    } finally {
      if (seq === openSeq) threadLoading.value = false
    }
  }
  function prepareReply(replyKind: ReplyMode) {
    replyMode.value = replyKind
    draft.value = replyDraft(replyKind, selectedThread.value, threadMessages.value)
  }
  function openReply(replyKind: ReplyMode) {
    prepareReply(replyKind)
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
  async function archiveThread() {
    await moveOut(selectedThread.value?.id, (id) => client.value!.archiveThread(id), 'archived')
  }
  async function snoozeThread(until?: string) {
    await moveOut(selectedThread.value?.id ?? selectedConversation.value?.id, (id) => client.value!.snoozeThread(id, until), 'snoozed')
  }
  // Move the selected/open thread into a folder (mailbox).
  async function moveThreadTo(mailboxId: string) {
    const id = selectedThread.value?.id ?? selectedConversation.value?.id
    if (!client.value?.moveThread) { status.value = 'move not supported'; return }
    if (!id) { status.value = 'move: nothing selected'; return }
    // Call moveThread as a method so `this` stays bound (the wails client reads
    // this.account); extracting it into a variable detaches `this`.
    await moveOut(id, (mid) => client.value!.moveThread!(mid, mailboxId), `moved → ${mailboxId}`)
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
  function compose() {
    draft.value = newDraft()
    recipientInput.value = ''
    composeOpen.value = true
  }
  async function sendDraft() {
    if (!client.value) return
    const outgoing = materializeRecipients()
    if (!outgoing.to.length && composeOpen.value) {
      status.value = 'add at least one recipient'
      return
    }
    await client.value.sendDraft(outgoing)
    status.value = 'sent'
    composeOpen.value = false
    draft.value = newDraft()
  }
  async function discardDraft() {
    if (client.value) await client.value.discardDraft(draft.value.id)
    draft.value = newDraft()
    composeOpen.value = false
  }
  function materializeRecipients() {
    const parsed = parseAddresses(recipientInput.value)
    if (parsed.length) draft.value.to = [...draft.value.to, ...parsed]
    recipientInput.value = ''
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
    await moveOut(selectedThread.value?.id ?? selectedConversation.value?.id, (id) => client.value!.archiveThread(id), 'archived')
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
    else if (cmd === 'snooze') void snoozeThread()
    else if (cmd === 'w' || cmd === 'write') { void queueSave(); status.value = 'draft saved' }
    else if (cmd === 'q' || cmd === 'quit') closeThread()
    else if (cmd.startsWith('label ')) { query.value = `label:${cmd.slice(6).trim()}`; void openSearch() }
    else status.value = `E492: not an editor command: ${cmd}`
  }
  function openCommandMenu() {
    if (!selectedThread.value && !selectedConversation.value) return
    commandMenuOpen.value = true
  }
  function closeCommandMenu() { commandMenuOpen.value = false }
  function attachMock() {
    draft.value.attachments.push({ filename: `attachment-${draft.value.attachments.length + 1}.txt`, contentType: 'text/plain', content: 'Mock attachment' })
    status.value = 'attachment queued'
  }

  watch(() => draft.value.body, () => queueSave())
  watch(query, () => { if (searchActive.value) void runSearch() })

  return {
    appPhase, client, account, configuredAccounts,
    activeMailbox, activeCategory, selectedIndex, selectedThread,
    mailboxes, labels, conversations, searchResults, threadMessages, focusedMessageId,
    query, replyMode, replyOpen, replyExpanded, focusPane, status, threadLoading, composeOpen, searchActive, command, commandMenuOpen,
    draft, recipientInput, setup, setupStatus, setupError, setupBusy,
    filteredConversations, activeList, selectedConversation, unreadCount,
    todayConversations, earlierConversations, categoryCounts, mode, statusHints, focusedThreadMessage,
    initializeApp, bootMailbox, submitOnboarding, resetSetup, removeAccount, refreshShell, openMailbox, warmMailbox,
    selectCategory, selectConversation, createMailbox, renameMailbox, setMailboxIcon, deleteMailbox,
    openThread, prepareReply, archiveThread, snoozeThread, moveThreadTo, applyLabel, createLabelAndApply, createFolderAndMove, toggleStar, toggleRead,
    openCommandMenu, closeCommandMenu,
    compose, sendDraft, discardDraft, materializeRecipients, queueSave, runSearch, openSearch, closeSearch,
    moveSelection, selectFirst, selectLast, archiveSelected, focusList, focusThread, closeThread,
    openCommand, openFind, submitCommand, cancelCommand, attachMock, openReply, toggleReplyExpanded, toggleMessageExpanded, focusMessage, focusAdjacentMessage,
    threadFind,
  }
}

export type MailShellApi = ReturnType<typeof createMailShell>

let instance: MailShellApi | null = null
export function useMailShell(): MailShellApi {
  if (!instance) instance = createMailShell()
  return instance
}
