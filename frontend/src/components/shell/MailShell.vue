<script setup lang="ts">
// Mail-phase container: topbar + triple-pane (Sidebar | MessageList |
// ReadingPane) + command line + vim modeline.
import { computed, ref } from 'vue'
import { useMailShell } from '../../composables/useMailShell'
import { useMailKeyboard } from '../../composables/useMailKeyboard'
import { useSettings } from '../../composables/useSettings'
import Sidebar from './Sidebar.vue'
import MessageList from './MessageList.vue'
import ReadingPane from './ReadingPane.vue'
import CommandLine from './CommandLine.vue'
import Modeline from './Modeline.vue'
import ComposeModal from '../overlays/ComposeModal.vue'
import Cheatsheet from '../overlays/Cheatsheet.vue'
import CommandMenu from './CommandMenu.vue'
import UndoToast from './UndoToast.vue'
import ShellToast from './ShellToast.vue'
import SettingsModal from '../overlays/SettingsModal.vue'
import { PhGearSix } from '@phosphor-icons/vue'

defineProps<{ devTools?: boolean }>()
const s = useMailShell()
const settings = useSettings()
const cheatsheetOpen = ref(false)
const settingsOpen = ref(false)
const mobilePane = ref<'list' | 'thread'>('list')
const readingPane = ref<InstanceType<typeof ReadingPane> | null>(null)
const accountName = computed(() => s.account.value?.name || s.account.value?.email || 'Mail')
// Terminal-style location: ~/<profile>/<view>
const profileSlug = computed(() => {
  const a = s.account.value
  const base = a?.email?.split('@')[0] || a?.name || 'me'
  return base.toLowerCase().replace(/\s+/g, '')
})
const viewSlug = computed(() => {
  if (s.searchActive.value) return 'search'
  const mb = s.mailboxes.value.find((m) => m.id === s.activeMailbox.value)
  return (mb?.name || s.mode.value).toLowerCase().replace(/\s+/g, '-')
})

useMailKeyboard({ shell: s, readingPane, mobilePane, cheatsheetOpen, settingsOpen })

function showThread() { mobilePane.value = 'thread'; s.focusThread() }
function showList() { mobilePane.value = 'list'; s.focusList() }
</script>

<template>
  <main class="mail-shell">
    <header class="topbar">
      <button class="titlebar-icon" type="button" title="Settings" aria-label="Settings" @click="settingsOpen = true"><PhGearSix :size="15" /></button>
    </header>

    <div class="body-shell" :class="{ rail: settings.navLayout === 'rail', 'nav-collapsed': settings.navCollapsed, 'mobile-list': mobilePane === 'list', 'mobile-thread': mobilePane === 'thread' }">
      <Sidebar />
      <MessageList @open-thread="showThread" />
      <ReadingPane ref="readingPane" @back-to-list="showList" />
    </div>

    <CommandMenu v-if="s.commandMenuOpen.value" @close="s.closeCommandMenu()" />
    <ComposeModal v-if="s.composeOpen.value" />
    <Cheatsheet v-if="cheatsheetOpen" @close="cheatsheetOpen = false" />
    <SettingsModal v-if="settingsOpen" :dev-tools="devTools" @close="settingsOpen = false" />

    <UndoToast />
    <ShellToast />

    <div class="shell-foot">
      <CommandLine />
      <Modeline />
    </div>
  </main>
</template>
