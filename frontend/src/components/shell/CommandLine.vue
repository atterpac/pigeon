<script setup lang="ts">
// The `/` search and `:` ex-command input strip, shown above the modeline while
// a command is active. Search v-models the shell query (live results); ex
// v-models the command text and dispatches on submit.
import { computed, nextTick, ref, watch } from 'vue'
import { useMailShell } from '../../composables/useMailShell'

const s = useMailShell()
const find = s.threadFind
const inputRef = ref<HTMLInputElement | null>(null)

const sigil = () => (s.command.value?.kind === 'ex' ? ':' : '/')

const findCount = computed(() => {
  if (!find.query.value.trim()) return ''
  return find.total.value === 0 ? 'no matches' : `${find.current.value}/${find.total.value}`
})

watch(s.command, (cmd) => {
  if (cmd) nextTick(() => { inputRef.value?.focus(); inputRef.value?.select() })
}, { immediate: true })

// Live-highlight as the find query changes.
watch(() => s.command.value?.kind === 'find' ? s.command.value.text : null, (text) => {
  if (text != null) find.run(text)
})

function onKeydown(event: KeyboardEvent) {
  const kind = s.command.value?.kind
  if (event.key === 'Enter') {
    event.preventDefault()
    if (kind === 'find' && event.shiftKey) find.prev()
    else s.submitCommand()
  }
  else if (event.key === 'Escape') { event.preventDefault(); s.cancelCommand() }
  else if ((event.key === 'ArrowDown' || (event.ctrlKey && event.key === 'n')) && kind === 'search') { event.preventDefault(); s.moveSelection(1) }
  else if ((event.key === 'ArrowUp' || (event.ctrlKey && event.key === 'p')) && kind === 'search') { event.preventDefault(); s.moveSelection(-1) }
  else if ((event.key === 'ArrowDown' || (event.ctrlKey && event.key === 'n')) && kind === 'find') { event.preventDefault(); find.next() }
  else if ((event.key === 'ArrowUp' || (event.ctrlKey && event.key === 'p')) && kind === 'find') { event.preventDefault(); find.prev() }
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
      v-else-if="s.command.value.kind === 'find'"
      ref="inputRef"
      v-model="s.command.value.text"
      class="cmd-input"
      placeholder="find in conversation"
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
    <span v-if="s.command.value.kind === 'find' && findCount" class="cmd-count">{{ findCount }}</span>
    <span class="cmd-hint">{{ s.command.value.kind === 'search' ? '↵ keep · esc cancel' : s.command.value.kind === 'find' ? '↵ next · ⇧↵ prev · esc close' : '↵ run · esc cancel' }}</span>
  </div>
</template>
