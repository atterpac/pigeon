<script setup lang="ts">
// Mail-phase container: topbar + triple-pane (Sidebar | MessageList |
// ReadingPane) + command line + vim modeline. Owns global keyboard handling
// including vim motions (counts, gg/G, dd) and the `?` cheatsheet.
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useMailShell } from '../../composables/useMailShell'
import { useSettings } from '../../composables/useSettings'
import Sidebar from './Sidebar.vue'
import MessageList from './MessageList.vue'
import ReadingPane from './ReadingPane.vue'
import CommandLine from './CommandLine.vue'
import Modeline from './Modeline.vue'
import ComposeModal from '../overlays/ComposeModal.vue'
import Cheatsheet from '../overlays/Cheatsheet.vue'
import SettingsModal from '../overlays/SettingsModal.vue'
import { PhMagnifyingGlass, PhKeyboard, PhGearSix, PhFlask, PhNotePencil } from '@phosphor-icons/vue'

defineProps<{ devTools?: boolean }>()
const emit = defineEmits<{ (e: 'open-sandbox'): void }>()
const s = useMailShell()
const settings = useSettings()
const cheatsheetOpen = ref(false)
const settingsOpen = ref(false)
const mobilePane = ref<'list' | 'thread'>('list')
const readingPane = ref<InstanceType<typeof ReadingPane> | null>(null)
const accountName = computed(() => s.account.value?.name || s.account.value?.email || 'Mail')

let countBuffer = ''
let gPending = false
let dPending = false
function resetPending() { countBuffer = ''; gPending = false; dPending = false }

onMounted(() => window.addEventListener('keydown', handleGlobalKeydown))
onUnmounted(() => window.removeEventListener('keydown', handleGlobalKeydown))

function handleGlobalKeydown(event: KeyboardEvent) {
  if (event.defaultPrevented) return
  const target = event.target as HTMLElement | null
  const inField = !!target && ['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName)
  if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') { event.preventDefault(); s.openCommand('search'); return }
  if ((event.metaKey || event.ctrlKey) && event.key === 'Enter') { event.preventDefault(); void s.sendDraft(); return }
  if (inField) return

  const key = event.key
  if (key === '?') { event.preventDefault(); cheatsheetOpen.value = !cheatsheetOpen.value; return }
  if (key === 'Escape') { if (settingsOpen.value) { settingsOpen.value = false } else if (cheatsheetOpen.value) { cheatsheetOpen.value = false } else { onEscape() } resetPending(); return }

  // Count prefix (1-9, then any digit).
  if (/[0-9]/.test(key) && !(key === '0' && !countBuffer)) { countBuffer += key; return }
  const count = parseInt(countBuffer || '1', 10)

  if (key === 'g') {
    if (gPending) {
      if (s.focusPane.value === 'thread') readingPane.value?.scrollThread('top')
      else s.selectFirst()
      resetPending()
    } else { gPending = true }
    return
  }
  gPending = false
  if (key === 'd') { if (dPending) { void s.archiveSelected(); resetPending() } else { dPending = true } return }
  dPending = false

  if (key === 'G') { if (s.focusPane.value === 'thread') readingPane.value?.scrollThread('bottom'); else s.selectLast() }
  else if (key === '/') { event.preventDefault(); s.openCommand('search') }
  else if (key === ':') { event.preventDefault(); s.openCommand('ex') }
  else if (key === 'c') s.compose()
  else if (key === 'j' || key === 'ArrowDown') {
    if (s.focusPane.value === 'thread') readingPane.value?.scrollThread(count)
    else s.moveSelection(count)
  }
  else if (key === 'k' || key === 'ArrowUp') {
    if (s.focusPane.value === 'thread') readingPane.value?.scrollThread(-count)
    else s.moveSelection(-count)
  }
  else if (key === 'Tab' && s.selectedThread.value) { event.preventDefault(); s.focusPane.value === 'thread' ? s.focusList() : s.focusThread() }
  else if (key === 'Enter') { void s.openThread(); mobilePane.value = 'thread' }
  else if (key === 'e') void s.archiveSelected()
  else if (key === 's') void s.snoozeThread()
  else if (key === '*') void s.toggleStar()
  else if (key === 'u') void s.toggleRead()
  else if (s.selectedThread.value && key === 'r') { s.openReply('reply'); readingPane.value?.focusReply() }
  else if (s.selectedThread.value && key === 'a') { s.openReply('replyAll'); readingPane.value?.focusReply() }
  else if (s.selectedThread.value && key === 'f') { s.openReply('forward'); readingPane.value?.focusReply() }
  resetPending()
}
function onEscape() {
  if (s.composeOpen.value) { void s.discardDraft(); return }
  if (s.command.value) { s.cancelCommand(); return }
  if (s.searchActive.value) { s.closeSearch(); return }
  if (s.focusPane.value === 'thread' || s.selectedThread.value) {
    s.closeThread()
    mobilePane.value = 'list'
  }
}
function showThread() { mobilePane.value = 'thread'; s.focusThread() }
function showList() { mobilePane.value = 'list'; s.focusList() }
</script>

<template>
  <main class="mail-shell">
    <header class="topbar">
      <div class="brand-group">
        <span class="brand-mark">mail</span>
        <span class="brand-copy"><b>{{ accountName }}</b><small>{{ s.mode.value.toLowerCase() }}</small></span>
      </div>
      <button class="search-affordance" type="button" @click="s.openCommand('search')"><PhMagnifyingGlass :size="15" /> Search mail <kbd>⌘K</kbd></button>
      <div class="topbar-actions">
        <button class="sandbox-button" type="button" @click="cheatsheetOpen = true"><PhKeyboard :size="15" /> Keys</button>
        <button class="sandbox-button" type="button" @click="settingsOpen = true"><PhGearSix :size="15" /> Settings</button>
        <button v-if="devTools" class="sandbox-button dev-only" type="button" @click="emit('open-sandbox')"><PhFlask :size="15" /> Sandbox</button>
        <button class="primary-action" type="button" @click="s.compose()"><PhNotePencil :size="15" /> Compose <kbd>c</kbd></button>
      </div>
    </header>

    <div class="body-shell" :class="{ rail: settings.navLayout === 'rail', 'mobile-list': mobilePane === 'list', 'mobile-thread': mobilePane === 'thread' }">
      <Sidebar />
      <MessageList @open-thread="showThread" />
      <ReadingPane ref="readingPane" @back-to-list="showList" />
    </div>

    <ComposeModal v-if="s.composeOpen.value" />
    <Cheatsheet v-if="cheatsheetOpen" @close="cheatsheetOpen = false" />
    <SettingsModal v-if="settingsOpen" :dev-tools="devTools" @close="settingsOpen = false" />

    <div class="shell-foot">
      <CommandLine />
      <Modeline />
    </div>
  </main>
</template>
