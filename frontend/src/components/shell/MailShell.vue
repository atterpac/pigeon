<script setup lang="ts">
// Mail-phase container: topbar + triple-pane (Sidebar | MessageList |
// ReadingPane) + vim-ish statusbar. Owns the global keyboard handling.
import { onMounted, onUnmounted } from 'vue'
import { useMailShell } from '../../composables/useMailShell'
import Sidebar from './Sidebar.vue'
import MessageList from './MessageList.vue'
import ReadingPane from './ReadingPane.vue'
import ComposeModal from '../overlays/ComposeModal.vue'

const emit = defineEmits<{ (e: 'open-sandbox'): void }>()
const s = useMailShell()

onMounted(() => window.addEventListener('keydown', handleGlobalKeydown))
onUnmounted(() => window.removeEventListener('keydown', handleGlobalKeydown))

function handleGlobalKeydown(event: KeyboardEvent) {
  if (event.defaultPrevented) return
  const target = event.target as HTMLElement | null
  const inField = !!target && ['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName)
  if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') { event.preventDefault(); void s.openSearch(); return }
  if ((event.metaKey || event.ctrlKey) && event.key === 'Enter') { event.preventDefault(); void s.sendDraft(); return }
  if (inField) return
  if (event.key === '/') { event.preventDefault(); void s.openSearch() }
  else if (event.key === 'c') s.compose()
  else if (event.key === 'j' || event.key === 'ArrowDown') s.moveSelection(1)
  else if (event.key === 'k' || event.key === 'ArrowUp') s.moveSelection(-1)
  else if (event.key === 'Enter') void s.openThread()
  else if (event.key === 'Escape') onEscape()
  else if (event.key === 'e') void s.archiveThread()
  else if (event.key === 's') void s.snoozeThread()
  else if (event.key === '*') void s.toggleStar()
  else if (event.key === 'u') void s.toggleRead()
  else if (s.selectedThread.value && event.key === 'r') s.prepareReply('reply')
  else if (s.selectedThread.value && event.key === 'a') s.prepareReply('replyAll')
  else if (s.selectedThread.value && event.key === 'f') s.prepareReply('forward')
}
function onEscape() {
  if (s.composeOpen.value) { void s.discardDraft(); return }
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
      <button class="search-affordance" type="button" @click="s.openSearch()"><span>⌕</span> Search mail <kbd>⌘K</kbd></button>
      <div class="topbar-actions">
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


    <footer class="statusbar">
      <strong>{{ s.mode.value }}</strong>
      <span>{{ s.statusHints.value }}</span>
      <span>{{ s.unreadCount.value }} unread · {{ s.account.value?.email }}</span>
    </footer>
  </main>
</template>
