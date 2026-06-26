<script setup lang="ts">
// The `/` search and `:` ex-command input strip, shown above the modeline while
// a command is active. Search v-models the shell query (live results); ex
// v-models the command text and dispatches on submit.
import { nextTick, ref, watch } from 'vue'
import { useMailShell } from '../../composables/useMailShell'

const s = useMailShell()
const inputRef = ref<HTMLInputElement | null>(null)

const sigil = () => (s.command.value?.kind === 'ex' ? ':' : '/')

watch(s.command, (cmd) => {
  if (cmd) nextTick(() => { inputRef.value?.focus(); inputRef.value?.select() })
}, { immediate: true })

function onKeydown(event: KeyboardEvent) {
  if (event.key === 'Enter') { event.preventDefault(); s.submitCommand() }
  else if (event.key === 'Escape') { event.preventDefault(); s.cancelCommand() }
  else if ((event.key === 'ArrowDown' || (event.ctrlKey && event.key === 'n')) && s.command.value?.kind === 'search') { event.preventDefault(); s.moveSelection(1) }
  else if ((event.key === 'ArrowUp' || (event.ctrlKey && event.key === 'p')) && s.command.value?.kind === 'search') { event.preventDefault(); s.moveSelection(-1) }
}
</script>

<template>
  <div v-if="s.command.value" class="command-line" :class="s.command.value.kind">
    <span class="cmd-sigil">{{ sigil() }}</span>
    <input
      v-if="s.command.value.kind === 'search'"
      ref="inputRef"
      v-model="s.query.value"
      class="cmd-input"
      placeholder="from:github is:unread"
      spellcheck="false"
      @keydown="onKeydown"
    />
    <input
      v-else
      ref="inputRef"
      v-model="s.command.value.text"
      class="cmd-input"
      placeholder="archive · snooze · label work · w · q"
      spellcheck="false"
      @keydown="onKeydown"
    />
    <span class="cmd-hint">{{ s.command.value.kind === 'search' ? '↵ keep · esc cancel' : '↵ run · esc cancel' }}</span>
  </div>
</template>
