<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { createMailClient } from './mail/client'
import type { Account, ComposeDraft, Conversation, Label, Mailbox, MailClient, ThreadMessage } from './mail/types'

type Screen = 'inbox' | 'thread' | 'compose' | 'search'
type EditorMode = 'INSERT' | 'NORMAL'

const accent = '#8b7cf6'
const client = ref<MailClient | null>(null)
const account = ref<Account | null>(null)
const screen = ref<Screen>('inbox')
const activeMailbox = ref('INBOX')
const selectedIndex = ref(0)
const selectedThread = ref<Conversation | null>(null)
const mailboxes = ref<Mailbox[]>([])
const labels = ref<Label[]>([])
const conversations = ref<Conversation[]>([])
const searchResults = ref<Conversation[]>([])
const threadMessages = ref<ThreadMessage[]>([])
const query = ref('from:github is:unread')
const replyMode = ref<'reply' | 'replyAll' | 'forward'>('reply')
const replyExpanded = ref(false)
const editorMode = ref<EditorMode>('INSERT')
const preview = ref(false)
const status = ref('loading')
const draft = ref(newDraft())
const recipientInput = ref('')
const searchInput = ref<HTMLInputElement | null>(null)
const editorTextarea = ref<HTMLTextAreaElement | null>(null)
const editorFocused = ref(false)
const currentLine = ref(1)
const caretStyle = ref({ transform: 'translate(58px, 20px)', height: '22px' })
let saveTimer: number | undefined
let charCanvas: HTMLCanvasElement | undefined

const activeList = computed(() => (screen.value === 'search' ? searchResults.value : conversations.value))
const selectedConversation = computed(() => activeList.value[selectedIndex.value] ?? null)
const unreadCount = computed(() => conversations.value.filter((conversation) => conversation.unread).length)
const lineNumbers = computed(() => Array.from({ length: Math.max(1, draft.value.body.split('\n').length) }, (_, index) => index + 1))
const todayConversations = computed(() => conversations.value.filter((conversation) => isToday(conversation.lastAt)))
const earlierConversations = computed(() => conversations.value.filter((conversation) => !isToday(conversation.lastAt)))
const renderedPreview = computed(() => renderMarkdown(draft.value.body))
const mode = computed(() => (screen.value === 'thread' ? 'THREAD' : screen.value === 'compose' ? 'COMPOSE' : screen.value === 'search' ? 'SEARCH' : 'NORMAL'))
const statusHints = computed(() => {
  if (screen.value === 'thread') return 'r reply · a reply all · f forward · e archive · esc back'
  if (screen.value === 'compose') return '⌘↵ send · ⌘⇧A attach · esc discard'
  if (screen.value === 'search') return '↑↓ navigate · ↵ open · esc clear'
  return 'j k move · e archive · s snooze · c compose · ⌘K palette'
})

watch(query, async () => {
  if (screen.value === 'search') await runSearch()
})
watch(() => draft.value.body, () => {
  queueSave()
  nextTick(updateCaret)
})

onMounted(async () => {
  client.value = await createMailClient()
  account.value = await client.value.getAccount()
  await refreshShell()
  window.addEventListener('keydown', handleGlobalKeydown)
})
onUnmounted(() => {
  window.removeEventListener('keydown', handleGlobalKeydown)
  if (saveTimer) window.clearTimeout(saveTimer)
})

async function refreshShell() {
  if (!client.value) return
  mailboxes.value = await client.value.listMailboxes()
  labels.value = await client.value.listLabels()
  await openMailbox(activeMailbox.value)
  status.value = 'using mock data'
}
async function openMailbox(mailboxId: string) {
  if (!client.value) return
  activeMailbox.value = mailboxId
  conversations.value = await client.value.listConversations(mailboxId)
  selectedIndex.value = 0
  screen.value = 'inbox'
}
async function openThread(threadId = selectedConversation.value?.id) {
  if (!client.value || !threadId) return
  const thread = await client.value.getThread(threadId)
  selectedThread.value = thread.conversation
  threadMessages.value = thread.messages.map((message, index, messages) => ({ ...message, expanded: message.expanded || index === messages.length - 1 }))
  screen.value = 'thread'
  status.value = 'thread loaded'
  prepareReply('reply')
}
function prepareReply(mode: 'reply' | 'replyAll' | 'forward') {
  replyMode.value = mode
  const latest = threadMessages.value.at(-1)
  const subject = selectedThread.value?.subject ?? ''
  draft.value = newDraft({
    threadId: selectedThread.value?.id,
    to: mode === 'forward' ? [] : latest?.from ? [latest.from] : [],
    subject: mode === 'forward' ? `Fwd: ${subject}` : subject.toLowerCase().startsWith('re:') ? subject : `Re: ${subject}`,
    inReplyTo: latest?.rfcMessageId,
    references: latest?.references ?? [],
  })
  preview.value = false
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
async function toggleStar(conversation = selectedThread.value ?? selectedConversation.value) {
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
  selectedThread.value = null
  screen.value = 'compose'
  preview.value = false
  nextTick(() => editorTextarea.value?.focus())
}
async function sendDraft() {
  if (!client.value) return
  const outgoing = materializeRecipients()
  if (!outgoing.to.length && screen.value === 'compose') {
    status.value = 'add at least one recipient'
    return
  }
  await client.value.sendDraft(outgoing)
  status.value = 'sent'
  screen.value = selectedThread.value ? 'thread' : 'inbox'
  draft.value = newDraft()
}
async function discardDraft() {
  if (client.value) await client.value.discardDraft(draft.value.id)
  draft.value = newDraft()
  screen.value = selectedThread.value ? 'thread' : 'inbox'
}
function materializeRecipients() {
  const parsed = parseAddresses(recipientInput.value)
  if (parsed.length) draft.value.to = [...draft.value.to, ...parsed]
  recipientInput.value = ''
  return draft.value
}
function queueSave() {
  if (!client.value || screen.value === 'inbox' || screen.value === 'search') return
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
  screen.value = 'search'
  await runSearch()
  await nextTick()
  searchInput.value?.focus()
  searchInput.value?.select()
}
function moveSelection(delta: number) {
  if (screen.value !== 'inbox' && screen.value !== 'search') return
  selectedIndex.value = Math.max(0, Math.min(activeList.value.length - 1, selectedIndex.value + delta))
}
function handleGlobalKeydown(event: KeyboardEvent) {
  if (event.defaultPrevented) return
  const target = event.target as HTMLElement | null
  const inEditor = target && ['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName)
  if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') { event.preventDefault(); void openSearch(); return }
  if ((event.metaKey || event.ctrlKey) && event.key === 'Enter') { event.preventDefault(); void sendDraft(); return }
  if (inEditor && target !== editorTextarea.value) return
  if (target === editorTextarea.value) { handleEditorKeydown(event); return }
  if (event.key === '/') { event.preventDefault(); void openSearch() }
  else if (event.key === 'c') compose()
  else if (event.key === 'j' || event.key === 'ArrowDown') moveSelection(1)
  else if (event.key === 'k' || event.key === 'ArrowUp') moveSelection(-1)
  else if (event.key === 'Enter') void openThread()
  else if (event.key === 'Escape') screen.value = selectedThread.value ? 'thread' : 'inbox'
  else if (event.key === 'e') void archiveThread()
  else if (event.key === 's') void snoozeThread()
  else if (event.key === '*') void toggleStar()
  else if (event.key === 'u') void toggleRead()
  else if (screen.value === 'thread' && event.key === 'r') prepareReply('reply')
  else if (screen.value === 'thread' && event.key === 'a') prepareReply('replyAll')
  else if (screen.value === 'thread' && event.key === 'f') prepareReply('forward')
}
function handleSearchKeydown(event: KeyboardEvent) {
  if (event.key === 'ArrowDown') { event.preventDefault(); moveSelection(1) }
  else if (event.key === 'ArrowUp') { event.preventDefault(); moveSelection(-1) }
  else if (event.key === 'Enter') { event.preventDefault(); void openThread() }
  else if (event.key === 'Escape') { event.preventDefault(); screen.value = 'inbox' }
}
function handleEditorKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') { editorMode.value = 'NORMAL'; event.preventDefault(); return true }
  if (editorMode.value === 'INSERT') return false
  const textarea = editorTextarea.value
  if (!textarea) return false
  const cursor = textarea.selectionStart
  if (event.key === 'i') editorMode.value = 'INSERT'
  else if (event.key === 'a') { textarea.setSelectionRange(cursor + 1, cursor + 1); editorMode.value = 'INSERT' }
  else if (event.key === 'x') replaceBody(textarea.value.slice(0, cursor) + textarea.value.slice(cursor + 1), cursor)
  else if (event.key === 'j' || event.key === 'ArrowDown') moveEditorLine(1)
  else if (event.key === 'k' || event.key === 'ArrowUp') moveEditorLine(-1)
  else return false
  event.preventDefault()
  updateCaret()
  return true
}
function replaceBody(body: string, cursor: number) {
  draft.value.body = body
  nextTick(() => editorTextarea.value?.setSelectionRange(cursor, cursor))
}
function moveEditorLine(delta: number) {
  const textarea = editorTextarea.value
  if (!textarea) return
  const lines = textarea.value.split('\n')
  const before = textarea.value.slice(0, textarea.selectionStart).split('\n')
  const line = before.length - 1
  const column = before.at(-1)?.length ?? 0
  const nextLine = Math.max(0, Math.min(lines.length - 1, line + delta))
  const offset = lines.slice(0, nextLine).join('\n').length + (nextLine === 0 ? 0 : 1)
  const next = offset + Math.min(column, lines[nextLine]?.length ?? 0)
  textarea.setSelectionRange(next, next)
}
function applyFormat(kind: 'bold' | 'italic' | 'code' | 'link') {
  const textarea = editorTextarea.value
  if (!textarea) return
  const start = textarea.selectionStart
  const end = textarea.selectionEnd
  const selected = draft.value.body.slice(start, end) || 'text'
  const map = {
    bold: [`**${selected}**`, 2],
    italic: [`_${selected}_`, 1],
    code: selected.includes('\n') ? [`\`\`\`\n${selected}\n\`\`\``, 4] : [`\`${selected}\``, 1],
    link: [`[${selected}](https://)`, selected.length + 3],
  } as const
  const [replacement, offset] = map[kind] as [string, number]
  draft.value.body = `${draft.value.body.slice(0, start)}${replacement}${draft.value.body.slice(end)}`
  nextTick(() => { textarea.focus(); textarea.setSelectionRange(start + offset, start + offset + selected.length); updateCaret() })
}
function attachMock() {
  draft.value.attachments.push({ filename: `attachment-${draft.value.attachments.length + 1}.txt`, contentType: 'text/plain', content: 'Mock attachment' })
  status.value = 'attachment queued'
}
function updateCaret() {
  const textarea = editorTextarea.value
  if (!textarea) return
  const style = window.getComputedStyle(textarea)
  const lineHeight = parseFloat(style.lineHeight) || 22
  const paddingTop = parseFloat(style.paddingTop) || 0
  const paddingLeft = parseFloat(style.paddingLeft) || 0
  const context = (charCanvas ??= document.createElement('canvas')).getContext('2d')
  if (context) context.font = style.font
  const charWidth = context?.measureText('M').width || 8.4
  const before = textarea.value.slice(0, textarea.selectionStart)
  const lines = before.split('\n')
  currentLine.value = lines.length
  const column = lines.at(-1)?.length ?? 0
  caretStyle.value = { transform: `translate(${paddingLeft + column * charWidth - textarea.scrollLeft}px, ${paddingTop + (currentLine.value - 1) * lineHeight - textarea.scrollTop}px)`, height: `${lineHeight}px` }
}
function onEditorFocus() { editorFocused.value = true; nextTick(updateCaret) }
function onEditorBlur() { editorFocused.value = false }
function newDraft(overrides: Partial<ComposeDraft> = {}): ComposeDraft {
  return { id: `draft-${Date.now()}-${Math.random().toString(16).slice(2)}`, to: [], cc: [], bcc: [], subject: '', body: '', attachments: [], updatedAt: new Date().toISOString(), ...overrides }
}
function parseAddresses(input: string) {
  return input.split(',').map((value) => value.trim()).filter(Boolean).map((value) => {
    const match = value.match(/^(.*)<(.+)>$/)
    return match ? { name: match[1]?.trim() ?? '', addr: match[2]?.trim() ?? '' } : { name: '', addr: value }
  })
}
function renderMarkdown(markdown: string) {
  if (!markdown.trim()) return '<div class="preview-empty">Nothing to preview yet.</div>'
  let inFence = false
  return markdown.split('\n').map((line) => {
    if (line.trim().startsWith('```')) { inFence = !inFence; return '<div class="preview-line"><code>```</code></div>' }
    const rendered = inFence ? `<code>${escapeHtml(line) || '&nbsp;'}</code>` : renderInlineMarkdown(line)
    return `<div class="preview-line">${rendered || '&nbsp;'}</div>`
  }).join('')
}
function renderInlineMarkdown(line: string) {
  return escapeHtml(line)
    .replace(/\[([^\]]+)\]\((https?:\/\/[^)\s]+)\)/g, '<a href="$2" target="_blank" rel="noreferrer">$1</a>')
    .replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>')
    .replace(/_([^_\n]+)_/g, '<em>$1</em>')
    .replace(/`([^`]+)`/g, '<code>$1</code>')
}
function escapeHtml(value: string) { return value.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;') }
function initials(address: { name: string; addr: string }) { return (address.name || address.addr).split(/\s+/).slice(0, 2).map((part) => part[0]).join('').toUpperCase() }
function participantLine(conversation: Conversation | null) { return conversation?.participants.map((p) => p.name || p.addr).join(', ') ?? '' }
function formatDate(value: string) {
  const date = new Date(value)
  return isToday(value) ? date.toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' }) : date.toLocaleDateString([], { weekday: 'short' })
}
function isToday(value: string) { return new Date(value).toDateString() === new Date().toDateString() }
function labelFor(conversation: Conversation | null) { return labels.value.find((label) => conversation?.labelIds.includes(label.id)) ?? labels.value[0] }
</script>

<template>
  <main class="mail-shell" :style="{ '--accent': accent }">
    <header class="topbar">
      <div class="path-group">
        <span class="traffic" aria-hidden="true"><span /><span /><span /></span>
        <span class="path">~/mail/<b>{{ screen }}</b></span>
      </div>
      <button class="search-affordance" type="button" @click="openSearch"><span>⌕</span> Search mail <kbd>⌘K</kbd></button>
      <button class="primary-action" type="button" @click="compose">Compose <kbd>c</kbd></button>
    </header>

    <div class="body-shell">
      <aside class="sidebar">
        <nav class="primary-nav">
          <button v-for="mailbox in mailboxes" :key="mailbox.id" :class="{ active: screen === 'inbox' && activeMailbox === mailbox.id }" type="button" @click="openMailbox(mailbox.id)">
            {{ mailbox.name }} <span v-if="mailbox.unread">{{ mailbox.unread }}</span>
          </button>
        </nav>
        <section class="bundle-nav">
          <h2>Bundles</h2>
          <button v-for="label in labels" :key="label.id" type="button" @click="query = `label:${label.name}`; openSearch()">
            <span class="swatch" :style="{ backgroundColor: label.swatch }" /> <span>{{ label.name }}</span> <span>{{ label.count }}</span>
          </button>
        </section>
      </aside>

      <section class="main-area">
        <template v-if="screen === 'inbox'">
          <header class="list-header"><p><strong>{{ conversations.length }} conversations</strong> · {{ unreadCount }} unread</p><span>sort: newest ↓</span></header>
          <div class="scroll-region">
            <p v-if="todayConversations.length" class="section-label">Today</p>
            <article v-for="conversation in todayConversations" :key="conversation.id" class="email-row" :class="{ unread: conversation.unread, selected: selectedConversation?.id === conversation.id }" @click="selectedIndex = activeList.findIndex((item) => item.id === conversation.id); openThread(conversation.id)">
              <button class="star" :class="{ active: conversation.starred }" type="button" @click.stop="toggleStar(conversation)" aria-label="Star"><svg viewBox="0 0 256 256"><path d="M128 24l31.5 63.8 70.4 10.2-50.9 49.7 12 70.1L128 184.6 65 217.8l12-70.1-50.9-49.7 70.4-10.2L128 24z" /></svg></button>
              <strong>{{ conversation.from.name || conversation.from.addr }}</strong>
              <p><span>{{ conversation.subject }}</span><small>{{ conversation.snippet }}</small></p>
              <em v-if="labelFor(conversation)" :style="{ background: labelFor(conversation)?.bg, color: labelFor(conversation)?.fg }">{{ labelFor(conversation)?.name }}</em>
              <time>{{ formatDate(conversation.lastAt) }}</time>
            </article>
            <p v-if="earlierConversations.length" class="section-label">Earlier</p>
            <article v-for="conversation in earlierConversations" :key="conversation.id" class="email-row" :class="{ unread: conversation.unread, selected: selectedConversation?.id === conversation.id }" @click="selectedIndex = activeList.findIndex((item) => item.id === conversation.id); openThread(conversation.id)">
              <button class="star" :class="{ active: conversation.starred }" type="button" @click.stop="toggleStar(conversation)" aria-label="Star"><svg viewBox="0 0 256 256"><path d="M128 24l31.5 63.8 70.4 10.2-50.9 49.7 12 70.1L128 184.6 65 217.8l12-70.1-50.9-49.7 70.4-10.2L128 24z" /></svg></button>
              <strong>{{ conversation.from.name || conversation.from.addr }}</strong>
              <p><span>{{ conversation.subject }}</span><small>{{ conversation.snippet }}</small></p>
              <em v-if="labelFor(conversation)" :style="{ background: labelFor(conversation)?.bg, color: labelFor(conversation)?.fg }">{{ labelFor(conversation)?.name }}</em>
              <time>{{ formatDate(conversation.lastAt) }}</time>
            </article>
          </div>
        </template>

        <template v-else-if="screen === 'search'">
          <header class="search-header"><label><span>⌕</span><input ref="searchInput" v-model="query" placeholder="Search - try from:github is:unread" @keydown="handleSearchKeydown" /><kbd>esc</kbd></label></header>
          <div class="results-header"><strong>{{ searchResults.length }}</strong> results <span>↑↓ navigate · ↵ open</span></div>
          <div class="scroll-region">
            <article v-for="(conversation, index) in searchResults" :key="conversation.id" class="email-row" :class="{ unread: conversation.unread, selected: index === selectedIndex }" @click="selectedIndex = index; openThread(conversation.id)">
              <button class="star" :class="{ active: conversation.starred }" type="button" @click.stop="toggleStar(conversation)" aria-label="Star"><svg viewBox="0 0 256 256"><path d="M128 24l31.5 63.8 70.4 10.2-50.9 49.7 12 70.1L128 184.6 65 217.8l12-70.1-50.9-49.7 70.4-10.2L128 24z" /></svg></button>
              <strong>{{ conversation.from.name || conversation.from.addr }}</strong><p><span>{{ conversation.subject }}</span><small>{{ conversation.snippet }}</small></p>
              <em v-if="labelFor(conversation)" :style="{ background: labelFor(conversation)?.bg, color: labelFor(conversation)?.fg }">{{ labelFor(conversation)?.name }}</em><time>{{ formatDate(conversation.lastAt) }}</time>
            </article>
          </div>
        </template>

        <template v-else-if="screen === 'thread'">
          <header class="thread-header">
            <button class="ghost-button" type="button" @click="screen = 'inbox'">← esc</button>
            <div><h1>{{ selectedThread?.subject }}</h1><p>{{ selectedThread?.messageCount }} messages · {{ participantLine(selectedThread) }}</p></div>
            <div class="thread-actions"><button class="ghost-button" type="button" @click="toggleStar()">Star</button><button class="ghost-button" type="button" @click="archiveThread">Archive <kbd>e</kbd></button><button class="ghost-button" type="button" @click="snoozeThread">Snooze <kbd>s</kbd></button><button class="ghost-button" type="button" @click="toggleRead">Unread <kbd>u</kbd></button></div>
          </header>
          <div class="thread-messages">
            <article v-for="message in threadMessages" :key="message.id"><span class="avatar">{{ initials(message.from) }}</span><div><header><strong>{{ message.from.name || message.from.addr }}</strong><span>{{ message.from.addr }}</span><time>{{ formatDate(message.date) }}</time></header><div v-if="message.expanded" class="message-body" @click="message.expanded = false"><p v-for="paragraph in message.body" :key="paragraph">{{ paragraph }}</p></div><p v-else class="snippet" @click="message.expanded = true">{{ message.snippet }}</p></div></article>
          </div>
          <footer class="reply-panel" :class="{ expanded: replyExpanded }">
            <button class="reply-expand-toggle" type="button" @click="replyExpanded = !replyExpanded">{{ replyExpanded ? '⌄' : '⌃' }}</button>
            <div class="reply-tabs"><button :class="{ active: replyMode === 'reply' }" type="button" @click="prepareReply('reply')">Reply</button><button :class="{ active: replyMode === 'replyAll' }" type="button" @click="prepareReply('replyAll')">Reply all</button><button :class="{ active: replyMode === 'forward' }" type="button" @click="prepareReply('forward')">Forward</button><span>{{ replyMode === 'forward' ? 'forward draft' : `to ${selectedThread?.from.name || selectedThread?.from.addr}` }}</span></div>
            <div class="reply-card">
              <div class="editor-shell">
                <button class="preview-toggle" :class="{ active: preview }" type="button" @click="preview = !preview">{{ preview ? 'Edit' : 'Preview' }}</button>
                <ol class="line-gutter" aria-hidden="true"><li v-for="line in lineNumbers" :key="line" :class="{ current: editorFocused && draft.body && line === currentLine }">{{ line }}</li></ol>
                <textarea v-if="!preview" ref="editorTextarea" v-model="draft.body" class="editor-input" spellcheck="true" placeholder="Write your reply...  (⌘↵ to send)" wrap="off" @keydown="handleEditorKeydown" @focus="onEditorFocus" @blur="onEditorBlur" @click="updateCaret" @keyup="updateCaret" @select="updateCaret" @scroll="updateCaret" @input="updateCaret" />
                <span v-if="!preview && editorFocused && draft.body.length" class="terminal-caret" :style="caretStyle" />
                <div v-if="preview" class="editor-preview" v-html="renderedPreview" />
              </div>
              <footer class="compose-toolbar">
                <button type="button" class="primary-action" @click="sendDraft">Send <kbd>⌘↵</kbd></button>
                <button type="button" class="format-button" @mousedown.prevent @click="applyFormat('bold')">B</button>
                <button type="button" class="format-button italic" @mousedown.prevent @click="applyFormat('italic')">I</button>
                <button type="button" class="format-button" @mousedown.prevent @click="applyFormat('code')">&lt;/&gt;</button>
                <button type="button" class="format-button" @mousedown.prevent @click="applyFormat('link')">Link</button>
                <button type="button" class="ghost-button" @click="attachMock">Attach</button>
                <span class="editor-status"><b>{{ editorMode }}</b> · {{ status }}</span>
              </footer>
            </div>
          </footer>
        </template>

        <form v-else class="compose-card" @submit.prevent="sendDraft">
          <header><h1>New message</h1><span>esc to discard</span></header>
          <label><span>to</span><em v-for="to in draft.to" :key="to.addr">{{ to.name || to.addr }}</em><input v-model="recipientInput" placeholder="Add recipients..." /></label>
          <label><span>subject</span><input v-model="draft.subject" placeholder="Subject" /></label>
          <div class="reply-card">
            <div class="editor-shell">
              <button class="preview-toggle" :class="{ active: preview }" type="button" @click="preview = !preview">{{ preview ? 'Edit' : 'Preview' }}</button>
              <ol class="line-gutter" aria-hidden="true"><li v-for="line in lineNumbers" :key="line" :class="{ current: editorFocused && draft.body && line === currentLine }">{{ line }}</li></ol>
              <textarea v-if="!preview" ref="editorTextarea" v-model="draft.body" class="editor-input" spellcheck="true" placeholder="Write a message..." wrap="off" @keydown="handleEditorKeydown" @focus="onEditorFocus" @blur="onEditorBlur" @click="updateCaret" @keyup="updateCaret" @select="updateCaret" @scroll="updateCaret" @input="updateCaret" />
              <span v-if="!preview && editorFocused && draft.body.length" class="terminal-caret" :style="caretStyle" />
              <div v-if="preview" class="editor-preview" v-html="renderedPreview" />
            </div>
            <div v-if="draft.attachments.length" class="attachment-row"><button v-for="attachment in draft.attachments" :key="attachment.filename" type="button" @click="draft.attachments = draft.attachments.filter((item) => item.filename !== attachment.filename)">{{ attachment.filename }} <small>×</small></button></div>
            <footer class="compose-toolbar">
              <button type="submit" class="primary-action">Send <kbd>⌘↵</kbd></button>
              <button type="button" class="format-button" @mousedown.prevent @click="applyFormat('bold')">B</button>
              <button type="button" class="format-button italic" @mousedown.prevent @click="applyFormat('italic')">I</button>
              <button type="button" class="format-button" @mousedown.prevent @click="applyFormat('code')">&lt;/&gt;</button>
              <button type="button" class="format-button" @mousedown.prevent @click="applyFormat('link')">Link</button>
              <button type="button" class="ghost-button" @click="attachMock">Attach</button>
              <span class="editor-status"><b>{{ editorMode }}</b> · {{ status }}</span>
              <button type="button" class="ghost-button" @click="discardDraft">Discard</button>
            </footer>
          </div>
        </form>
      </section>
    </div>
    <footer class="statusbar"><strong>{{ mode }}</strong><span>{{ statusHints }}</span><span>{{ unreadCount }} unread · {{ account?.email }}</span></footer>
  </main>
</template>

<style scoped>
:global(*){box-sizing:border-box}:global(body){margin:0;background:#0e0e13;color:#e8e8ef;font-family:"Hanken Grotesk",Inter,ui-sans-serif,system-ui,sans-serif}button,input,textarea{font:inherit}button{cursor:pointer}.mail-shell{width:100vw;height:100vh;display:grid;grid-template-rows:54px 1fr 32px;overflow:hidden;background:#0e0e13}.topbar{display:grid;grid-template-columns:minmax(248px,1fr) minmax(360px,620px) minmax(248px,1fr);align-items:center;gap:20px;padding:0 18px;background:#0b0b10;border-bottom:1px solid #1a1a22}.path-group{display:flex;align-items:center;gap:18px;color:#5a5a66;font:13px "JetBrains Mono",ui-monospace,monospace}.traffic{display:flex;gap:7px}.traffic span{width:11px;height:11px;border-radius:50%;background:#2a2a36}.path b{color:var(--accent);font-weight:500}.search-affordance{height:36px;display:flex;align-items:center;justify-content:center;gap:10px;color:#6a6a76;background:#15151c;border:1px solid #23232e;border-radius:9px;font-size:13px}kbd{color:#5a5a66;font:11px "JetBrains Mono",ui-monospace,monospace}.primary-action{background:var(--accent);color:#0e0e13;border:0;border-radius:9px;padding:9px 16px;font-size:13px;font-weight:600}.topbar>.primary-action{justify-self:end;min-width:118px}.body-shell{min-height:0;display:grid;grid-template-columns:248px minmax(0,1fr)}.sidebar{border-right:1px solid #1a1a22;background:#0b0b10;padding:22px 16px;font-size:13.5px}.primary-nav,.bundle-nav{display:grid;gap:2px}.primary-nav button,.bundle-nav button{display:grid;grid-template-columns:1fr auto;align-items:center;color:#8a8a96;background:transparent;border:0;border-radius:10px;padding:10px 13px;text-align:left}.primary-nav button.active{background:rgba(139,124,246,.13);color:var(--accent);font-weight:600}.primary-nav button.active span{border-radius:20px;padding:1px 8px;background:var(--accent);color:#0e0e13;font:11.5px "JetBrains Mono",ui-monospace,monospace}.bundle-nav h2{margin:20px 0 0;padding:22px 13px 10px;color:#4a4a55;font:11px "JetBrains Mono",ui-monospace,monospace;letter-spacing:.09em;text-transform:uppercase}.bundle-nav button{grid-template-columns:20px 1fr auto;gap:12px;padding:9px 13px}.swatch{width:8px;height:8px;border-radius:50%}.main-area{min-width:0;min-height:0;display:grid;grid-template-rows:auto 1fr auto}.list-header,.thread-header,.search-header,.results-header{display:flex;align-items:center;justify-content:space-between;gap:16px;min-height:50px;padding:0 26px;border-bottom:1px solid #1a1a22;color:#5a5a66;font:12px "JetBrains Mono",ui-monospace,monospace}.list-header p,.thread-header p{margin:0}.list-header strong{color:#c9c9d4;font:600 16px "Hanken Grotesk",Inter,sans-serif}.scroll-region{min-height:0;overflow:auto}.section-label{margin:0;padding:18px 26px 8px;color:#4a4a55;font:11px "JetBrains Mono",ui-monospace,monospace;letter-spacing:.09em;text-transform:uppercase}.email-row{position:relative;display:grid;grid-template-columns:26px 160px minmax(0,1fr) auto 60px;gap:15px;align-items:center;min-height:58px;padding:14px 26px;border-bottom:1px solid #15151d;color:#8a8a96}.email-row:hover,.email-row.selected{background:rgba(139,124,246,.1)}.email-row.selected:before{content:"";position:absolute;inset:0 auto 0 0;width:2px;background:var(--accent)}.email-row strong{overflow:hidden;text-overflow:ellipsis;white-space:nowrap;color:#8a8a96;font-size:14px;font-weight:400}.email-row.unread strong{color:#d6d0f0;font-weight:600}.email-row.unread p span{color:#eceaf6;font-weight:600}.email-row p{min-width:0;margin:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.email-row p span{color:#9a9aa6;font-size:14px}.email-row small{margin-left:10px;color:#5e5e6a;font-size:13.5px}.email-row em{font-style:normal;font:10.5px "JetBrains Mono",ui-monospace,monospace;border-radius:6px;padding:3px 9px}.email-row time{justify-self:end;color:#5a5a66;font:11.5px "JetBrains Mono",ui-monospace,monospace}.star{display:grid;place-items:center;width:22px;height:22px;border:0;background:transparent;color:#3f3c4c}.star svg{width:15px;height:15px;fill:transparent;stroke:currentColor;stroke-width:18px}.star.active{color:#f8d66d}.star.active svg{fill:currentColor}.thread-header{min-height:64px}.thread-header h1{margin:0 0 4px;color:#eceaf6;font:600 15.5px "Hanken Grotesk",Inter,sans-serif}.thread-header p{color:#6a6a76}.thread-actions{display:flex;gap:7px}.ghost-button,.format-button{color:#8a8a96;background:transparent;border:1px solid #23232e;border-radius:7px;padding:6px 11px;font:11px "JetBrains Mono",ui-monospace,monospace}.ghost-button:hover,.format-button:hover{background:#15151c}.search-header{display:block;padding:22px 26px 14px}.search-header label{display:flex;align-items:center;gap:13px;background:#15151c;border:1px solid var(--accent);border-radius:11px;padding:13px 17px}.search-header input{flex:1;color:#e8e8ef;background:transparent;border:0;outline:0}.results-header{height:42px;min-height:42px}.thread-messages{overflow:auto;padding:0 26px}.thread-messages article{display:grid;grid-template-columns:34px minmax(0,1fr);gap:14px;padding:20px 0;border-bottom:1px solid #15151d}.avatar{display:grid;place-items:center;width:34px;height:34px;border-radius:9px;background:#15151c;border:1px solid #23232e;color:#9a9aa6;font:11px "JetBrains Mono",ui-monospace,monospace}.thread-messages header{display:flex;gap:10px;color:#5a5a66;font-size:12px}.thread-messages header strong{color:#e8e8ef;font-size:14px;font-weight:600}.thread-messages header time{margin-left:auto;font:11px "JetBrains Mono",ui-monospace,monospace}.message-body,.snippet{margin:8px 0 0;color:#cfcfdb;font-size:14px;line-height:1.72}.message-body p{margin:0 0 13px}.snippet{overflow:hidden;text-overflow:ellipsis;white-space:nowrap;color:#6a6a76}.reply-panel{position:relative;border-top:1px solid #1a1a22;background:#0b0b10;padding:16px 26px}.reply-panel.expanded{padding-top:26px}.reply-expand-toggle{position:absolute;top:-15px;right:26px;width:34px;height:28px;border-radius:999px;border:1px solid #23232e;background:#15151c;color:var(--accent)}.reply-tabs{display:flex;align-items:center;gap:8px;margin-bottom:10px}.reply-tabs button{color:#8a8a96;background:transparent;border:0;border-radius:6px;padding:5px 12px;font:11.5px "JetBrains Mono",ui-monospace,monospace}.reply-tabs button.active{background:rgba(139,124,246,.13);color:var(--accent)}.reply-tabs span{margin-left:auto;color:#5a5a66;font:11.5px "JetBrains Mono",ui-monospace,monospace}.reply-card,.compose-card{display:grid;gap:12px}.compose-card{gap:0;align-self:start;justify-self:center;width:min(100% - 52px,840px);margin:26px;overflow:hidden;background:#0b0b10;border:1px solid #1f1f29;border-radius:14px}.compose-card .reply-card{gap:0}.compose-card header,.compose-card label{display:flex;align-items:center;gap:14px;min-height:54px;margin:0;padding:0 22px;border-bottom:1px solid #1a1a22;color:#5a5a66}.compose-card h1{margin:0;color:#e8e8ef;font-size:15px}.compose-card label>span{width:56px;flex:none;font:12px "JetBrains Mono",ui-monospace,monospace}.compose-card input{flex:1;color:#e8e8ef;background:transparent;border:0;outline:0}.editor-shell{position:relative;min-height:112px;border:1px solid #23232e;border-radius:11px;background:#15151c;overflow:hidden}.editor-shell:before{content:"";position:absolute;top:0;bottom:0;left:36px;width:1px;background:#23232e;z-index:1}.reply-panel.expanded .editor-shell{min-height:clamp(300px,48vh,560px)}.compose-card .editor-shell{min-height:300px;border:0;border-radius:0;background:#0b0b10}.line-gutter{position:absolute;inset:0 auto 0 0;width:36px;margin:0;padding:17px 0 0;list-style:none;color:#4a4a55;text-align:center;font:13px/22px "JetBrains Mono",ui-monospace,monospace}.line-gutter li{height:22px;line-height:22px}.compose-card .line-gutter{padding-top:20px;line-height:25.375px}.compose-card .line-gutter li{height:25.375px;line-height:25.375px}.line-gutter li.current{color:var(--accent);font-weight:600}.editor-input,.editor-preview{position:absolute;inset:0;width:100%;height:100%;padding:17px 16px 17px 48px;color:#e8e8ef;background:transparent;border:0;outline:0;resize:none;font:14px/22px "JetBrains Mono",monospace;caret-color:transparent}.compose-card .editor-input,.compose-card .editor-preview{padding:20px 22px 20px 58px;color:#d6d6e0;font-size:14.5px;line-height:1.75}.editor-preview{overflow:auto}.editor-preview :deep(.preview-line){min-height:22px;line-height:22px;white-space:pre-wrap}.compose-card .editor-preview :deep(.preview-line){min-height:25.375px;line-height:25.375px}.editor-preview :deep(code){background:#1f1d2d;border-radius:4px;padding:2px 4px}.terminal-caret{position:absolute;left:0;top:0;width:8px;background:rgba(169,157,255,.75);animation:blink 1s steps(2,start) infinite;pointer-events:none}.preview-toggle{position:absolute;top:8px;right:10px;z-index:2;color:#8a8a96;background:#111119;border:1px solid #2a2838;border-radius:7px;padding:6px 10px;font:11px "JetBrains Mono",ui-monospace,monospace}.preview-toggle:hover,.preview-toggle.active{color:var(--accent);border-color:rgba(139,124,246,.45);background:rgba(139,124,246,.13)}.compose-toolbar{display:flex;align-items:center;gap:8px;flex-wrap:wrap}.compose-card .compose-toolbar{padding:14px 22px;border-top:1px solid #1a1a22}.editor-status{margin-left:auto;color:#5a5a66;font-size:13px}.editor-status b{color:var(--accent);font:700 13px "JetBrains Mono",ui-monospace,monospace}.statusbar{display:flex;justify-content:space-between;align-items:center;height:32px;border-top:1px solid #1a1a22;background:#0b0b10;padding:0 22px;color:#5a5a66;font:11px "JetBrains Mono",ui-monospace,monospace}.statusbar strong{border-radius:5px;padding:2px 9px;background:var(--accent);color:#0e0e13}.statusbar span{color:#8a8a96}@keyframes blink{50%{opacity:0}}@media(max-width:900px){.topbar{grid-template-columns:1fr}.body-shell{grid-template-columns:1fr}.sidebar{display:none}.email-row{grid-template-columns:26px minmax(0,1fr) 60px}.email-row strong,.email-row em{display:none}}
</style>
