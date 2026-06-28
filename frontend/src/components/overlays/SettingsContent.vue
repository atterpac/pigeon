<script setup lang="ts">
// The body of one settings category. Split out of SettingsModal so the `scroll`
// settings layout can stack every category, while the other layouts render one.
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { type EmailSignature, useSettings } from '../../composables/useSettings'
import { useMailShell } from '../../composables/useMailShell'
import { getTheme, themesByPack } from '../../theme/themes'
import { Events } from '@wailsio/runtime'
import { NotificationService } from '../../bindings/github.com/wailsapp/wails/v3/pkg/services/notifications'
import { applyPollInterval } from '../../mail/syncSettings'
import { deleteSignature, emptySignature, sanitizeSignatureHtml, saveSignature, signaturesFor } from '../../mail/signatures'
import { PhX } from '@phosphor-icons/vue'

defineProps<{ category: string }>()
const settings = useSettings()
const s = useMailShell()
const packs = themesByPack()
const activeTheme = computed(() => getTheme(settings.theme))
const hiddenMailboxes = computed(() => s.mailboxes.value.filter((mailbox) => settings.hiddenMailboxIds.includes(mailbox.id)))
function unhideMailbox(id: string) {
  settings.hiddenMailboxIds = settings.hiddenMailboxIds.filter((mailboxId) => mailboxId !== id)
}

const accountId = computed(() => s.account.value?.id ?? '')
const accountSignatures = computed(() => signaturesFor(settings, accountId.value))
const selectedSignatureId = ref('')
const richEditor = ref<HTMLElement | null>(null)
const selectedSignature = computed(() => accountSignatures.value.find((signature) => signature.id === selectedSignatureId.value) ?? null)
const defaultSignatureId = computed(() => settings.defaultSignatureIds[accountId.value] ?? '')

watch(accountSignatures, (list) => {
  if (!list.length) {
    selectedSignatureId.value = ''
    return
  }
  if (!list.some((signature) => signature.id === selectedSignatureId.value)) {
    selectedSignatureId.value = defaultSignatureId.value || list[0]?.id || ''
  }
}, { immediate: true })

watch(selectedSignature, () => {
  nextTick(syncRichEditor)
}, { immediate: true })

function addSignature() {
  if (!accountId.value) return
  const next = emptySignature()
  next.name = accountSignatures.value.length ? `Signature ${accountSignatures.value.length + 1}` : 'Default'
  saveSignature(settings, accountId.value, next)
  selectedSignatureId.value = next.id
}

function updateSignature(patch: Partial<EmailSignature>) {
  if (!accountId.value || !selectedSignature.value) return
  saveSignature(settings, accountId.value, { ...selectedSignature.value, ...patch })
}

function syncRichEditor() {
  if (!richEditor.value || !selectedSignature.value) return
  const html = selectedSignature.value.html || textToHtml(selectedSignature.value.body)
  if (richEditor.value.innerHTML !== html) richEditor.value.innerHTML = html
}

function textToHtml(value: string) {
  const escaped = value.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
  return escaped.split('\n').join('<br>')
}

function saveRichSignature() {
  if (!richEditor.value) return
  const html = sanitizeSignatureHtml(richEditor.value.innerHTML)
  updateSignature({ html, body: richEditor.value.innerText.trim() })
}

async function pasteRichSignature(event: ClipboardEvent) {
  event.preventDefault()
  const items = Array.from(event.clipboardData?.items ?? [])
  const imageItems = items.filter((item) => item.type.startsWith('image/'))
  if (imageItems.length) {
    for (const item of imageItems) {
      const file = item.getAsFile()
      if (!file) continue
      const dataUrl = await fileToDataURL(file)
      insertHtmlAtCursor(`<img src="${dataUrl}" alt="${file.name.replace(/"/g, '&quot;')}" style="max-width:220px;height:auto">`)
    }
    saveRichSignature()
    return
  }
  const html = event.clipboardData?.getData('text/html')
  const text = event.clipboardData?.getData('text/plain')
  insertHtmlAtCursor(sanitizeSignatureHtml(html || textToHtml(text || '')))
  saveRichSignature()
}

function insertHtmlAtCursor(html: string) {
  richEditor.value?.focus()
  document.execCommand('insertHTML', false, html)
}

function fileToDataURL(file: File) {
  return new Promise<string>((resolve, reject) => {
    const reader = new FileReader()
    reader.onerror = () => reject(reader.error ?? new Error('read failed'))
    reader.onload = () => resolve(String(reader.result || ''))
    reader.readAsDataURL(file)
  })
}

function removeSignature(id: string) {
  if (!accountId.value) return
  deleteSignature(settings, accountId.value, id)
}

function setDefaultSignature(id: string) {
  if (!accountId.value) return
  settings.defaultSignatureIds = { ...settings.defaultSignatureIds, [accountId.value]: id }
}

const composeOptions = ['centered', 'docked', 'side', 'fullscreen', 'minimal', 'split'] as const
const sidebarOptions = ['flat', 'cards', 'compact', 'outline', 'header', 'airy'] as const
const navOptions = ['grouped', 'plain', 'icons', 'counts', 'rail', 'accounts'] as const
const settingsLayoutOptions = [
  'sidebar',
  'tabs',
  'scroll',
  'cards',
  'palette',
  'fullscreen',
] as const
const densityOptions = ['comfortable', 'compact'] as const

// --- Poll interval ----------------------------------------------------------
const pollOptions = [
  { label: '15 seconds', value: 15 },
  { label: '30 seconds', value: 30 },
  { label: '1 minute', value: 60 },
  { label: '5 minutes', value: 300 },
  { label: '15 minutes', value: 900 },
] as const

function setPollInterval(seconds: number) {
  settings.pollIntervalSeconds = seconds
  void applyPollInterval(seconds)
}

const sendUndoOptions = [
  { label: 'Off', value: 0 },
  { label: '5 seconds', value: 5 },
  { label: '10 seconds', value: 10 },
  { label: '20 seconds', value: 20 },
  { label: '30 seconds', value: 30 },
] as const

const notifyModes = [
  { value: 'all', label: 'All mail' },
  { value: 'inbox', label: 'Inbox only' },
  { value: 'none', label: 'Off' },
] as const

const mutedInput = ref('')
function addMutedSender() {
  const value = mutedInput.value.trim().toLowerCase().replace(/^@/, '')
  if (value && !settings.notify.mutedSenders.includes(value)) settings.notify.mutedSenders.push(value)
  mutedInput.value = ''
}
function removeMutedSender(sender: string) {
  settings.notify.mutedSenders = settings.notify.mutedSenders.filter((item) => item !== sender)
}

// --- Notifications test harness ---------------------------------------------
// Drives the wails v3 notifications service directly so we can poke at it
// without waiting for real mail to arrive.
const notifLog = ref<string[]>([])
const notifAuth = ref<boolean | null>(null)
let notifOff: (() => void) | undefined

function logNotif(msg: string) {
  notifLog.value = [msg, ...notifLog.value].slice(0, 12)
}

let notifSeq = 0
function notifId() {
  notifSeq += 1
  return `test-${notifSeq}-${Math.floor(performance.now())}`
}

async function requestAuth() {
  try {
    notifAuth.value = await NotificationService.RequestNotificationAuthorization()
    logNotif(`RequestAuthorization → ${notifAuth.value}`)
  } catch (err) {
    logNotif(`RequestAuthorization error: ${String(err)}`)
  }
}

async function checkAuth() {
  try {
    notifAuth.value = await NotificationService.CheckNotificationAuthorization()
    logNotif(`CheckAuthorization → ${notifAuth.value}`)
  } catch (err) {
    logNotif(`CheckAuthorization error: ${String(err)}`)
  }
}

async function sendBasic() {
  const id = notifId()
  try {
    await NotificationService.SendNotification({
      id,
      title: 'New message',
      subtitle: 'inbox',
      body: 'This is a basic test notification.',
    } as any)
    logNotif(`SendNotification(${id}) sent`)
  } catch (err) {
    logNotif(`SendNotification error: ${String(err)}`)
  }
}

async function sendWithActions() {
  const categoryId = 'test-actions'
  const id = notifId()
  try {
    await NotificationService.RegisterNotificationCategory({
      id: categoryId,
      actions: [
        { id: 'archive', title: 'Archive' },
        { id: 'reply', title: 'Reply' },
      ],
      hasReplyField: true,
      replyPlaceholder: 'Type a reply…',
      replyButtonTitle: 'Send',
    } as any)
    await NotificationService.SendNotificationWithActions({
      id,
      title: 'Message with actions',
      subtitle: 'inbox',
      body: 'Tap an action button (desktop only).',
      categoryId,
    } as any)
    logNotif(`SendNotificationWithActions(${id}) sent`)
  } catch (err) {
    logNotif(`SendNotificationWithActions error: ${String(err)}`)
  }
}

async function clearAll() {
  try {
    await NotificationService.RemoveAllDeliveredNotifications()
    await NotificationService.RemoveAllPendingNotifications()
    logNotif('Cleared delivered + pending notifications')
  } catch (err) {
    logNotif(`Clear error: ${String(err)}`)
  }
}

let mailOff: (() => void) | undefined
onMounted(() => {
  notifOff = Events.On('notification:action', (ev: { data?: unknown }) => {
    logNotif(`action response: ${JSON.stringify(ev?.data)}`)
  })
  // Backend sync loop emits this whenever a poll pulls in new mail.
  mailOff = Events.On('mail:new', (ev: { data?: unknown }) => {
    logNotif(`poll → new mail: ${JSON.stringify(ev?.data)}`)
  })
})
onUnmounted(() => {
  notifOff?.()
  mailOff?.()
})
</script>

<template>
  <!-- Appearance -->
  <template v-if="category === 'appearance'">
    <p class="set-section">Theme</p>
    <label class="set-row theme-select-row">
      <span>
        <b>{{ activeTheme.pack }} · {{ activeTheme.name }}</b>
        <small>{{ activeTheme.dark ? 'dark' : 'light' }}</small>
      </span>
      <select class="select theme-select" v-model="settings.theme">
        <optgroup v-for="group in packs" :key="group.pack" :label="group.pack">
          <option v-for="theme in group.themes" :key="theme.id" :value="theme.id">
            {{ theme.name }} · {{ theme.dark ? 'dark' : 'light' }}
          </option>
        </optgroup>
      </select>
    </label>
    <div
      class="theme-preview"
      :style="{
        background: activeTheme.tokens['--bg'],
        borderColor: activeTheme.tokens['--border-2'],
      }"
    >
      <span class="theme-swatches">
        <i :style="{ background: activeTheme.tokens['--accent'] }" />
        <i :style="{ background: activeTheme.tokens['--green'] }" />
        <i :style="{ background: activeTheme.tokens['--orange'] }" />
        <i :style="{ background: activeTheme.tokens['--purple'] }" />
        <i :style="{ background: activeTheme.tokens['--cyan'] }" />
        <i :style="{ background: activeTheme.tokens['--red'] }" />
      </span>
      <span :style="{ color: activeTheme.tokens['--text'] }">Aa</span>
      <small :style="{ color: activeTheme.tokens['--text-mut'] }">{{ activeTheme.id }}</small>
    </div>

    <p class="set-section">Display</p>
    <label class="set-row">
      <span><b>Relative line numbers</b><small>vim-style gutter in the message list</small></span>
      <button
        type="button"
        class="toggle"
        :class="{ on: settings.relativenumber }"
        @click="settings.relativenumber = !settings.relativenumber"
      >
        <i />
      </button>
    </label>
    <label class="set-row">
      <span><b>Vim mode</b><small>NORMAL/INSERT editing in the composer</small></span>
      <button
        type="button"
        class="toggle"
        :class="{ on: settings.vimMode }"
        @click="settings.vimMode = !settings.vimMode"
      >
        <i />
      </button>
    </label>
    <label class="set-row">
      <span><b>Density</b><small>row + padding scale</small></span>
      <select class="select" v-model="settings.density">
        <option v-for="opt in densityOptions" :key="opt" :value="opt">{{ opt }}</option>
      </select>
    </label>
    <label class="set-row">
      <span><b>Block remote images</b><small>stop tracking pixels until you load images per message</small></span>
      <button
        type="button"
        class="toggle"
        :class="{ on: settings.blockRemoteImages }"
        @click="settings.blockRemoteImages = !settings.blockRemoteImages"
      >
        <i />
      </button>
    </label>
  </template>

  <!-- Layout & Views -->
  <template v-else-if="category === 'layout'">
    <p class="set-section">View presets</p>
    <p class="set-note">All presets are live — pick one and the UI updates immediately.</p>
    <label class="set-row"
      ><span><b>Compose surface</b><small>where new messages open</small></span
      ><select class="select" v-model="settings.compose">
        <option v-for="opt in composeOptions" :key="opt" :value="opt">{{ opt }}</option>
      </select></label
    >
    <label class="set-row"
      ><span><b>Sidebar style</b></span
      ><select class="select" v-model="settings.sidebarStyle">
        <option v-for="opt in sidebarOptions" :key="opt" :value="opt">{{ opt }}</option>
      </select></label
    >
    <label class="set-row"
      ><span><b>Nav layout</b></span
      ><select class="select" v-model="settings.navLayout">
        <option v-for="opt in navOptions" :key="opt" :value="opt">{{ opt }}</option>
      </select></label
    >
    <label class="set-row"
      ><span><b>Settings layout</b><small>this modal's own shape</small></span
      ><select class="select" v-model="settings.settingsLayout">
        <option v-for="opt in settingsLayoutOptions" :key="opt" :value="opt">{{ opt }}</option>
      </select></label
    >
  </template>

  <!-- Accounts -->
  <template v-else-if="category === 'accounts'">
    <p class="set-section">Connected</p>
    <div class="set-row">
      <span
        ><b>{{ s.account.value?.name || 'Account' }}</b
        ><small>{{ s.account.value?.email }}</small></span
      ><span class="set-tag">active</span>
    </div>
    <p class="set-note">
      {{ s.configuredAccounts.value.length }} account(s) configured · stored in the local SQLite
      store + keyring.
    </p>

    <p class="set-section">Signatures</p>
    <p class="set-note">Saved per account and appended below new messages and replies for {{ s.account.value?.email || 'this account' }}.</p>
    <div class="signature-manager">
      <nav class="signature-list" aria-label="Saved signatures">
        <button
          v-for="signature in accountSignatures"
          :key="signature.id"
          type="button"
          class="signature-list-item"
          :class="{ active: selectedSignatureId === signature.id }"
          @click="selectedSignatureId = signature.id"
        >
          <b>{{ signature.name || 'Untitled' }}</b>
          <small>{{ defaultSignatureId === signature.id ? 'default' : signature.body ? `${signature.body.length} chars` : 'empty' }}</small>
        </button>
        <button class="signature-add" type="button" @click="addSignature">New signature</button>
      </nav>
      <section v-if="selectedSignature" class="signature-editor">
        <div class="signature-editor-row">
          <input
            class="signature-name"
            :value="selectedSignature.name"
            placeholder="Signature name"
            @input="updateSignature({ name: ($event.target as HTMLInputElement).value })"
          />
          <button
            class="set-btn"
            :class="{ active: defaultSignatureId === selectedSignature.id }"
            type="button"
            @click="setDefaultSignature(selectedSignature.id)"
          >Default</button>
        </div>
        <div class="signature-rich-preview">
          <span>Paste signature</span>
          <div
            ref="richEditor"
            class="signature-paste-target"
            contenteditable="true"
            spellcheck="true"
            @input="saveRichSignature"
            @paste="pasteRichSignature"
          />
        </div>
        <p class="set-note">Paste from a signature generator or web page. HTML, links, remote images, and pasted image files are preserved.</p>
        <div class="signature-editor-actions">
          <button class="ghost-button" type="button" @click="removeSignature(selectedSignature.id)">Delete</button>
        </div>
      </section>
      <section v-else class="signature-empty">
        <button class="set-btn" type="button" @click="addSignature">Create your first signature</button>
      </section>
    </div>

    <p class="set-section">Hidden folders</p>
    <p v-if="!hiddenMailboxes.length" class="set-note">No hidden folders. Hide one from the sidebar (eye icon) to declutter the list.</p>
    <div v-for="mailbox in hiddenMailboxes" :key="mailbox.id" class="set-row">
      <span><b>{{ mailbox.name }}</b><small>hidden from the sidebar</small></span>
      <button class="ghost-button" type="button" @click="unhideMailbox(mailbox.id)">Show</button>
    </div>
  </template>

  <!-- Keybindings -->
  <template v-else-if="category === 'keybindings'">
    <p class="set-section">Motions</p>
    <p class="set-note">
      Press <b>?</b> anywhere for the full cheatsheet. Vim mode:
      {{ settings.vimMode ? 'on' : 'off' }}.
    </p>
  </template>

  <template v-else-if="category === 'notifications'">
    <p class="set-section">Mail polling</p>
    <p class="set-note">
      How often to check for new mail in the background. Lower is snappier but
      hits your provider more often; push-capable accounts update sooner regardless.
    </p>
    <div class="notif-actions">
      <button
        v-for="opt in pollOptions"
        :key="opt.value"
        class="set-btn"
        :class="{ active: settings.pollIntervalSeconds === opt.value }"
        type="button"
        @click="setPollInterval(opt.value)"
      >{{ opt.label }}</button>
    </div>

    <p class="set-section">Desktop notifications</p>
    <p class="set-note">Which new mail raises a desktop notification.</p>
    <div class="notif-actions">
      <button
        v-for="opt in notifyModes"
        :key="opt.value"
        class="set-btn"
        :class="{ active: settings.notify.mode === opt.value }"
        type="button"
        @click="settings.notify.mode = opt.value"
      >{{ opt.label }}</button>
    </div>

    <label class="set-row">
      <span><b>Quiet hours</b><small>silence notifications during a daily window</small></span>
      <button
        type="button"
        class="toggle"
        :class="{ on: settings.notify.quietHours.enabled }"
        @click="settings.notify.quietHours.enabled = !settings.notify.quietHours.enabled"
      ><i /></button>
    </label>
    <label v-if="settings.notify.quietHours.enabled" class="set-row">
      <span><b>From → To</b><small>local time; wraps past midnight</small></span>
      <span class="quiet-times">
        <input type="time" v-model="settings.notify.quietHours.from" />
        <input type="time" v-model="settings.notify.quietHours.to" />
      </span>
    </label>

    <p class="set-section">Muted senders</p>
    <p class="set-note">Mail from these addresses or domains never notifies.</p>
    <form class="muted-add" @submit.prevent="addMutedSender">
      <input v-model="mutedInput" placeholder="someone@example.com or example.com" />
      <button type="submit" class="set-btn">Add</button>
    </form>
    <div v-if="settings.notify.mutedSenders.length" class="muted-list">
      <button v-for="sender in settings.notify.mutedSenders" :key="sender" type="button" class="muted-chip" @click="removeMutedSender(sender)">{{ sender }} <PhX :size="11" /></button>
    </div>

    <p class="set-section">Undo send</p>
    <p class="set-note">
      Hold outgoing mail this long before delivery so you can recall it from the
      "Sending…" toast or with <kbd>U</kbd>. Off sends immediately.
    </p>
    <div class="notif-actions">
      <button
        v-for="opt in sendUndoOptions"
        :key="opt.value"
        class="set-btn"
        :class="{ active: settings.sendUndoSeconds === opt.value }"
        type="button"
        @click="settings.sendUndoSeconds = opt.value"
      >{{ opt.label }}</button>
    </div>

    <p class="set-section">Test harness</p>
    <p class="set-note">
      Drives the Wails notifications service directly.
      Authorization:
      <b>{{ notifAuth === null ? 'unknown' : notifAuth ? 'granted' : 'denied' }}</b>.
      Desktop only — actions/replies won't fire in the browser preview.
    </p>
    <div class="notif-actions">
      <button class="set-btn" type="button" @click="requestAuth">Request authorization</button>
      <button class="set-btn" type="button" @click="checkAuth">Check authorization</button>
      <button class="set-btn" type="button" @click="sendBasic">Send basic</button>
      <button class="set-btn" type="button" @click="sendWithActions">Send with actions</button>
      <button class="set-btn" type="button" @click="clearAll">Clear all</button>
    </div>
    <p class="set-section">Log</p>
    <ul class="notif-log">
      <li v-if="!notifLog.length" class="set-note">No events yet.</li>
      <li v-for="(line, i) in notifLog" :key="i">{{ line }}</li>
    </ul>
  </template>

  <!-- Placeholder tabs -->
  <template v-else>
    <p class="set-note">Nothing to configure here yet.</p>
  </template>
</template>
