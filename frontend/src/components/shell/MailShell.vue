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
      <div class="brand-group">
        <span class="brand-mark">chirp</span>
        <span class="brand-path" :title="accountName">
          <span class="bp-sep">~/</span><span class="bp-profile">{{ profileSlug }}</span><span class="bp-sep">/</span><span class="bp-view">{{ viewSlug }}</span>
        </span>
      </div>
      <button class="search-affordance" type="button" @click="s.openCommand('search')"><PhMagnifyingGlass :size="15" /> Search mail <kbd>⌘K</kbd></button>
      <div class="topbar-actions">
        <button class="sandbox-button" type="button" @click="cheatsheetOpen = true"><PhKeyboard :size="15" /> Keys</button>
        <button class="sandbox-button" type="button" @click="settingsOpen = true"><PhGearSix :size="15" /> Settings</button>
        <button v-if="devTools" class="sandbox-button dev-only" type="button" @click="emit('open-sandbox')"><PhFlask :size="15" /> Sandbox</button>
        <button class="primary-action" type="button" @click="s.compose()"><PhNotePencil :size="15" /> Compose <kbd>c</kbd></button>
      </div>
    </header>

    <div class="body-shell" :class="{ rail: settings.navLayout === 'rail', 'nav-collapsed': settings.navCollapsed, 'mobile-list': mobilePane === 'list', 'mobile-thread': mobilePane === 'thread' }">
      <Sidebar @open-settings="settingsOpen = true" />
      <MessageList @open-thread="showThread" />
      <ReadingPane ref="readingPane" @back-to-list="showList" />
    </div>

    <CommandMenu v-if="s.commandMenuOpen.value" @close="s.closeCommandMenu()" />
    <ComposeModal v-if="s.composeOpen.value" />
    <Cheatsheet v-if="cheatsheetOpen" @close="cheatsheetOpen = false" />
    <SettingsModal v-if="settingsOpen" :dev-tools="devTools" @close="settingsOpen = false" />

    <div class="shell-foot">
      <CommandLine />
      <Modeline />
    </div>
  </main>
</template>
