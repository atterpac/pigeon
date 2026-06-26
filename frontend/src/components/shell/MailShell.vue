<script setup lang="ts">
// Mail-phase container: topbar + triple-pane (Sidebar | MessageList |
// ReadingPane) + command line + vim modeline. Owns global keyboard handling
// including vim motions (counts, gg/G, dd) and the `?` cheatsheet.
import { onMounted, onUnmounted, ref } from 'vue'
import { useMailShell } from '../../composables/useMailShell'
import Sidebar from './Sidebar.vue'
import MessageList from './MessageList.vue'
import ReadingPane from './ReadingPane.vue'
import CommandLine from './CommandLine.vue'
import Modeline from './Modeline.vue'
import ComposeModal from '../overlays/ComposeModal.vue'
import Cheatsheet from '../overlays/Cheatsheet.vue'
import SettingsModal from '../overlays/SettingsModal.vue'

const emit = defineEmits<{ (e: 'open-sandbox'): void }>()
const s = useMailShell()
const cheatsheetOpen = ref(false)
const settingsOpen = ref(false)

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

  if (key === 'g') { if (gPending) { s.selectFirst(); resetPending() } else { gPending = true } return }
  gPending = false
  if (key === 'd') { if (dPending) { void s.archiveSelected(); resetPending() } else { dPending = true } return }
  dPending = false

  if (key === 'G') { s.selectLast() }
  else if (key === '/') { event.preventDefault(); s.openCommand('search') }
  else if (key === ':') { event.preventDefault(); s.openCommand('ex') }
  else if (key === 'c') s.compose()
  else if (key === 'j' || key === 'ArrowDown') s.moveSelection(count)
  else if (key === 'k' || key === 'ArrowUp') s.moveSelection(-count)
  else if (key === 'Enter') void s.openThread()
  else if (key === 'e') void s.archiveSelected()
  else if (key === 's') void s.snoozeThread()
  else if (key === '*') void s.toggleStar()
  else if (key === 'u') void s.toggleRead()
  else if (s.selectedThread.value && key === 'r') s.prepareReply('reply')
  else if (s.selectedThread.value && key === 'a') s.prepareReply('replyAll')
  else if (s.selectedThread.value && key === 'f') s.prepareReply('forward')
  resetPending()
}
function onEscape() {
  if (s.composeOpen.value) { void s.discardDraft(); return }
  if (s.command.value) { s.cancelCommand(); return }
  if (s.searchActive.value) { s.closeSearch(); return }
  s.selectedThread.value = null
  s.threadMessages.value = []
}
</script>

<template>
  <main class="mail-shell">
    <header class="topbar">
      <div class="path-group">
        <span class="traffic" aria-hidden="true"><span /><span /><span /></span>
        <span class="path">~/mail/<b>{{ s.mode.value.toLowerCase() }}</b></span>
      </div>
      <button class="search-affordance" type="button" @click="s.openCommand('search')"><span>⌕</span> Search mail <kbd>⌘K</kbd></button>
      <div class="topbar-actions">
        <button class="sandbox-button" type="button" @click="cheatsheetOpen = true">? Keys</button>
        <button class="sandbox-button" type="button" @click="settingsOpen = true">⚙ Settings</button>
        <button class="sandbox-button" type="button" @click="emit('open-sandbox')">Sandbox</button>
        <button class="primary-action" type="button" @click="s.compose()">Compose <kbd>c</kbd></button>
      </div>
    </header>

    <div class="body-shell">
      <Sidebar />
      <MessageList />
      <ReadingPane />
    </div>

    <ComposeModal v-if="s.composeOpen.value" />
    <Cheatsheet v-if="cheatsheetOpen" @close="cheatsheetOpen = false" />
    <SettingsModal v-if="settingsOpen" @close="settingsOpen = false" />

    <div class="shell-foot">
      <CommandLine />
      <Modeline />
    </div>
  </main>
</template>
