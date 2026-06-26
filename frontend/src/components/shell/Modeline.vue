<script setup lang="ts">
// Vim-style statusline: colored mode block, buffer (mailbox), branch (account),
// hints, and a line:col / percentage position derived from the list selection.
import { computed } from 'vue'
import { useMailShell } from '../../composables/useMailShell'

const s = useMailShell()

const buffer = computed(() => {
  const mailbox = s.mailboxes.value.find((m) => m.id === s.activeMailbox.value)
  return s.searchActive.value ? 'search' : (mailbox?.name.toLowerCase() ?? 'inbox')
})
const total = computed(() => s.activeList.value.length)
const position = computed(() => `${total.value ? s.selectedIndex.value + 1 : 0}:${total.value}`)
const pct = computed(() => {
  if (!total.value) return 'All'
  if (s.selectedIndex.value <= 0) return 'Top'
  if (s.selectedIndex.value >= total.value - 1) return 'Bot'
  return `${Math.round((s.selectedIndex.value / (total.value - 1)) * 100)}%`
})
</script>

<template>
  <footer class="modeline" :class="`mode-${s.mode.value.toLowerCase()}`">
    <span class="ml-mode">{{ s.mode.value }}</span>
    <span class="ml-focus">{{ s.focusPane.value }}</span>
    <span class="ml-buffer">{{ buffer }}</span>
    <span class="ml-hints">{{ s.statusHints.value }}</span>
    <span class="ml-branch"> {{ s.account.value?.email || 'no account' }}</span>
    <span class="ml-pos">{{ position }}</span>
    <span class="ml-pct">{{ pct }}</span>
  </footer>
</template>
