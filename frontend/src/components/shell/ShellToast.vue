<script setup lang="ts">
import { computed } from 'vue'
import { PhCheckCircle, PhInfo, PhWarningCircle, PhX } from '@phosphor-icons/vue'
import { useMailShell } from '../../composables/useMailShell'

const s = useMailShell()
const icon = computed(() => {
  if (s.toast.value?.kind === 'success') return PhCheckCircle
  if (s.toast.value?.kind === 'error') return PhWarningCircle
  return PhInfo
})
</script>

<template>
  <Transition name="toast">
    <div v-if="s.toast.value" class="shell-toast" :class="`toast-${s.toast.value.kind}`" role="status">
      <component :is="icon" class="toast-icon" :size="17" weight="bold" />
      <span class="toast-copy">
        <b>{{ s.toast.value.title }}</b>
        <small v-if="s.toast.value.detail">{{ s.toast.value.detail }}</small>
      </span>
      <button type="button" class="toast-close" aria-label="Dismiss notification" @click="s.clearToast()">
        <PhX :size="12" />
      </button>
    </div>
  </Transition>
</template>

<style scoped>
.shell-toast {
  position: absolute;
  right: 18px;
  bottom: 42px;
  z-index: 72;
  display: grid;
  grid-template-columns: 18px minmax(0, 1fr) 24px;
  align-items: center;
  gap: 10px;
  width: min(360px, calc(100vw - 32px));
  padding: 10px 9px 10px 12px;
  border: 1px solid var(--border-2);
  border-radius: 11px;
  background: color-mix(in oklab, var(--surface) 96%, transparent);
  box-shadow: var(--shadow-2), var(--top-hi);
  color: var(--text);
}

.toast-icon { color: var(--accent) }
.toast-success .toast-icon { color: var(--green) }
.toast-error .toast-icon { color: var(--red) }

.toast-copy { min-width: 0; display: grid; gap: 2px }
.toast-copy b { color: var(--text); font-size: 13px; font-weight: 650 }
.toast-copy small { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text-mut); font: 11px "JetBrains Mono", ui-monospace, monospace }

.toast-close {
  display: grid;
  place-items: center;
  width: 24px;
  height: 24px;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: var(--text-mut);
  cursor: pointer;
}
.toast-close:hover { background: var(--surface-2); color: var(--text) }

.toast-enter-active, .toast-leave-active { transition: opacity var(--ease-fast), transform var(--ease-fast) }
.toast-enter-from, .toast-leave-to { opacity: 0; transform: translateY(8px) }
</style>
