<script setup lang="ts">
// `?` keybinding cheatsheet overlay.
import { PhX } from '@phosphor-icons/vue'

const emit = defineEmits<{ (e: 'close'): void }>()

const groups: Array<{ title: string; keys: Array<[string, string]> }> = [
  { title: 'Navigation', keys: [['j / k', 'move down / up'], ['{count}j', 'move by count'], ['gg / G', 'first / last'], ['↵', 'open thread'], ['esc', 'close / cancel']] },
  { title: 'Actions', keys: [['space', 'command menu'], ['c', 'compose'], ['e / dd', 'archive'], ['s', 'snooze'], ['#', 'delete'], ['!', 'report spam'], ['*', 'star'], ['u', 'toggle unread'], ['U', 'undo last action'], ['r / a / f', 'reply / all / forward']] },
  { title: 'Command line', keys: [['/', 'search'], [':', 'ex-command'], [': archive', 'archive selected'], [': label x', 'open label x'], ['⌘K', 'search'], ['⌘↵', 'send']] },
  { title: 'Snoozed view', keys: [['u', 'unsnooze (wake now)'], ['↵', 'open thread'], ['esc', 'back to mailbox']] },
  { title: 'Visual (multi-select)', keys: [['v', 'enter / exit visual'], ['j / k', 'move cursor'], ['space', 'toggle row'], ['V', 'select all / none'], ['e # s * u', 'archive · del · snooze · star · read'], ['↵', 'command menu (selection)'], ['esc', 'exit']] },
]
</script>

<template>
  <div class="modal-backdrop" @click.self="emit('close')">
    <section class="cheatsheet">
      <header><h2>Keybindings</h2><button class="modal-close" type="button" @click="emit('close')">? <PhX :size="12" /></button></header>
      <div class="cheat-grid">
        <div v-for="group in groups" :key="group.title" class="cheat-group">
          <h3>{{ group.title }}</h3>
          <dl><template v-for="[key, desc] in group.keys" :key="key"><dt>{{ key }}</dt><dd>{{ desc }}</dd></template></dl>
        </div>
      </div>
    </section>
  </div>
</template>
