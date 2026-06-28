<script setup lang="ts">
// Transient "Archived · Undo" affordance for the last reversible action.
// Mirrors `s.lastAction`; auto-dismisses via the shell's undo timer. `U` also
// triggers undo from the global keymap.
import { PhArrowUUpLeft } from '@phosphor-icons/vue'
import { useMailShell } from '../../composables/useMailShell'

const s = useMailShell()
</script>

<template>
  <Transition name="toast">
    <div v-if="s.lastAction.value" class="undo-toast" role="status">
      <span class="ut-label">{{ s.lastAction.value.label }}</span>
      <button class="ut-undo" type="button" @click="s.performUndo()">
        <PhArrowUUpLeft :size="13" weight="bold" />
        {{ s.lastAction.value.kind === 'send' ? 'Undo send' : 'Undo' }}
        <kbd>U</kbd>
      </button>
    </div>
  </Transition>
</template>

<style scoped>
.undo-toast {
  position: absolute;
  left: 50%;
  bottom: 14px;
  z-index: 70;
  transform: translateX(-50%);
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 8px 8px 8px 14px;
  border: 1px solid var(--border-2);
  border-radius: 10px;
  background: color-mix(in oklab, var(--surface) 96%, transparent);
  box-shadow: var(--shadow-2), var(--top-hi);
  color: var(--text);
  font-size: 12.5px;
  white-space: nowrap;
}

.ut-label { color: var(--text-dim) }

.ut-undo {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  border: 1px solid var(--accent-line);
  border-radius: 7px;
  padding: 4px 8px;
  background: var(--accent-soft);
  color: var(--accent);
  font: inherit;
  cursor: pointer;
}

.ut-undo:hover { background: color-mix(in oklab, var(--accent-soft) 70%, var(--accent)) }

.ut-undo kbd {
  display: inline-grid;
  place-items: center;
  min-width: 16px;
  height: 16px;
  padding: 0 4px;
  border: 1px solid var(--accent-line);
  border-radius: 4px;
  background: var(--surface-2);
  color: var(--accent);
  font: 10px "JetBrains Mono", ui-monospace, monospace;
}

.toast-enter-active, .toast-leave-active { transition: opacity var(--ease-fast), transform var(--ease-fast) }
.toast-enter-from, .toast-leave-to { opacity: 0; transform: translate(-50%, 8px) }
</style>
