// Shared mail-shell state + actions, extracted from the old App.vue.
// Singleton: every component that calls useMailShell() gets the same instance,
// so state is shared without prop-drilling. Editor-internal concerns (caret,
// vim mode, formatting) live in components/editor/MarkdownEditor.vue, not here.
import { computed, nextTick, ref, watch } from 'vue'
import { createMailClient } from '../mail/client'
import type { Account, Category, ComposeDraft, Conversation, Label, Mailbox, MailClient, ThreadMessage } from '../mail/types'
import { createOnboardingClient, type ConfiguredAccount, type SetupMethod } from '../onboarding/client'
import { errorMessage, isToday, parseAddresses } from '../mail/format'

export type AppPhase = 'starting' | 'onboarding' | 'mail'
export type CategoryTab = Category | 'all'
export type ReplyMode = 'reply' | 'replyAll' | 'forward'

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
  const query = ref('from:github is:unread')
  const replyMode = ref<ReplyMode>('reply')
  const replyExpanded = ref(false)
  const status = ref('loading')

  // Overlay / pane modes (replaced the old `screen` enum).
  const composeOpen = ref(false)
  const searchActive = ref(false)

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
  })

  const filteredConversations = computed(() => activeCategory.value === 'all'
    ? conversations.value
    : conversations.value.filter((conversation) => conversation.category === activeCategory.value))
  const activeList = computed(() => (searchActive.value ? searchResults.value : filteredConversations.value))
  const selectedConversation = computed(() => activeList.value[selectedIndex.value] ?? null)
  const unreadCount = computed(() => filteredConversations.value.filter((conversation) => conversation.unread).length)
  const todayConversations = computed(() => filteredConversations.value.filter((conversation) => isToday(conversation.lastAt)))
  const earlierConversations = computed(() => filteredConversations.value.filter((conversation) => !isToday(conversation.lastAt)))
  const categoryCounts = computed(() => {
    const counts: Record<CategoryTab, number> = { all: conversations.value.length, primary: 0, promotions: 0, updates: 0, social: 0, forums: 0 }
    for (const conversation of conversations.value) counts[conversation.category] += 1
    return counts
  })
  const mode = computed(() => composeOpen.value ? 'COMPOSE' : searchActive.value ? 'SEARCH' : selectedThread.value ? 'THREAD' : 'NORMAL')
  const statusHints = computed(() => {
    if (composeOpen.value) return '⌘↵ send · ⌘⇧A attach · esc discard'
    if (searchActive.value) return '↑↓ navigate · ↵ open · esc clear'
    if (selectedThread.value) return 'r reply · a reply all · f forward · e archive · esc close'
    return 'j k move · e archive · s snooze · c compose · ⌘K search'
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
    appPhase.value = 'mail'
  }
  async function submitOnboarding() {
    setupError.value = ''
    const email = setup.value.email.trim()
    if (!email) { setupError.value = 'Email address is required.'; return }
    if (setup.value.method === 'appPassword' && !setup.value.appPassword.trim()) {
      setupError.value = 'App password is required.'; return
    }
    setupBusy.value = true
    setupStatus.value = setup.value.method === 'google' ? 'waiting for Google authorization' : 'verifying account'
    try {
      const added = await onboarding.addAccount(setup.value)
      configuredAccounts.value = [added, ...configuredAccounts.value.filter((item) => item.id !== added.id)]
      setup.value.appPassword = ''
      await bootMailbox(added)
    } catch (error) {
      setupError.value = errorMessage(error)
      setupStatus.value = 'setup did not finish'
    } finally {
      setupBusy.value = false
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
        selectedIndex.value = Math.min(selectedIndex.value, Math.max(0, filteredConversations.value.length - 1))
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
  }
  async function openThread(threadId = selectedConversation.value?.id) {
    if (!client.value || !threadId) return
    const thread = await client.value.getThread(threadId)
    selectedThread.value = thread.conversation
    threadMessages.value = thread.messages.map((message, index, messages) => ({ ...message, expanded: message.expanded || index === messages.length - 1 }))
    composeOpen.value = false
    status.value = 'thread loaded'
    prepareReply('reply')
  }
  function prepareReply(replyKind: ReplyMode) {
    replyMode.value = replyKind
    const latest = threadMessages.value.at(-1)
    const subject = selectedThread.value?.subject ?? ''
    draft.value = newDraft({
      threadId: selectedThread.value?.id,
      to: replyKind === 'forward' ? [] : latest?.from ? [latest.from] : [],
      subject: replyKind === 'forward' ? `Fwd: ${subject}` : subject.toLowerCase().startsWith('re:') ? subject : `Re: ${subject}`,
      inReplyTo: latest?.rfcMessageId,
      references: latest?.references ?? [],
    })
  }
  async function archiveThread() {
    if (!client.value || !selectedThread.value) return
    await client.value.archiveThread(selectedThread.value.id)
    status.value = 'archived'
    await openMailbox(activeMailbox.value)
  }
  async function snoozeThread() {
    if (!client.value || !selectedThread.value) return
    await client.value.snoozeThread(selectedThread.value.id)
    status.value = 'snoozed'
    await openMailbox(activeMailbox.value)
  }
  async function toggleStar(conversation: Conversation | null = selectedThread.value ?? selectedConversation.value) {
    if (!client.value || !conversation) return
    const next = !conversation.starred
    conversation.starred = next
    await client.value.toggleStar(conversation.id, next)
  }
  async function toggleRead() {
    if (!client.value || !selectedThread.value) return
    const read = selectedThread.value.unread
    await client.value.markThreadRead(selectedThread.value.id, read)
    selectedThread.value.unread = !read
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
  }
  function moveSelection(delta: number) {
    selectedIndex.value = Math.max(0, Math.min(activeList.value.length - 1, selectedIndex.value + delta))
  }
  function newDraft(overrides: Partial<ComposeDraft> = {}): ComposeDraft {
    return { id: `draft-${Date.now()}-${Math.random().toString(16).slice(2)}`, to: [], cc: [], bcc: [], subject: '', body: '', attachments: [], updatedAt: new Date().toISOString(), ...overrides }
  }
  function attachMock() {
    draft.value.attachments.push({ filename: `attachment-${draft.value.attachments.length + 1}.txt`, contentType: 'text/plain', content: 'Mock attachment' })
    status.value = 'attachment queued'
  }

  watch(() => draft.value.body, () => queueSave())
  watch(query, () => { if (searchActive.value) void runSearch() })

  return {
    appPhase, client, account, configuredAccounts,
    activeMailbox, activeCategory, selectedIndex, selectedThread,
    mailboxes, labels, conversations, searchResults, threadMessages,
    query, replyMode, replyExpanded, status, composeOpen, searchActive,
    draft, recipientInput, setup, setupStatus, setupError, setupBusy,
    filteredConversations, activeList, selectedConversation, unreadCount,
    todayConversations, earlierConversations, categoryCounts, mode, statusHints,
    initializeApp, bootMailbox, submitOnboarding, refreshShell, openMailbox, warmMailbox,
    selectCategory, openThread, prepareReply, archiveThread, snoozeThread, toggleStar, toggleRead,
    compose, sendDraft, discardDraft, materializeRecipients, queueSave, runSearch, openSearch, closeSearch,
    moveSelection, attachMock,
  }
}

export type MailShellApi = ReturnType<typeof createMailShell>

let instance: MailShellApi | null = null
export function useMailShell(): MailShellApi {
  if (!instance) instance = createMailShell()
  return instance
}
