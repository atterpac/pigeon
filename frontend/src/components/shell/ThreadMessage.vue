<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { PhSpinnerGap } from '@phosphor-icons/vue'
import { avatarStyle, formatDate, initials, renderEmailHtml } from '../../mail/format'
import type { ThreadMessage } from '../../mail/types'

const props = withDefaults(defineProps<{
  message: ThreadMessage
  focused?: boolean
}>(), {
  focused: false,
})

// Memoize the iframe document so it only changes when the HTML body itself
// changes — not on every re-render (focus/scroll). A new srcdoc string forces
// the iframe to fully reload, which is the main source of render-time variance.
const srcdoc = computed(() => (props.message.html ? renderEmailHtml(props.message.html) : ''))

// Show a spinner over the iframe until it finishes rendering its document.
// Reset whenever the document actually changes (not on unrelated re-renders).
const frameLoading = ref(true)
watch(srcdoc, () => { frameLoading.value = true; frameHeight.value = 0 })
const emit = defineEmits<{
  (e: 'toggle-expanded', id: string): void
  (e: 'focus-message', id: string): void
}>()

// Auto-size the iframe to its content height (reported via postMessage) so the
// email body has no inner scroll — the whole thread scrolls as one region.
const frame = ref<HTMLIFrameElement | null>(null)
const frameHeight = ref(0)
function onMessage(event: MessageEvent) {
  if (event.data?.type !== 'email-frame-height') return
  if (!frame.value || event.source !== frame.value.contentWindow) return
  const h = Number(event.data.height)
  if (Number.isFinite(h) && h > 0) frameHeight.value = Math.ceil(h)
}
onMounted(() => window.addEventListener('message', onMessage))
onBeforeUnmount(() => window.removeEventListener('message', onMessage))
</script>

<template>
  <article
    :class="{ focused }"
    :data-message-id="message.id"
    tabindex="0"
    @click="emit('focus-message', message.id)"
    @focus="emit('focus-message', message.id)"
  >
    <span class="avatar" :style="avatarStyle(message.from)">{{ initials(message.from) }}</span>
    <div>
      <header>
        <strong>{{ message.from.name || message.from.addr }}</strong>
        <span>{{ message.from.addr }}</span>
        <time>{{ formatDate(message.date) }}</time>
      </header>
      <div v-if="message.expanded" class="message-body" @click.stop="emit('toggle-expanded', message.id)">
        <div v-if="message.html" class="email-html-wrap" @click.stop>
          <iframe
            ref="frame"
            class="email-html-frame"
            sandbox="allow-scripts"
            referrerpolicy="no-referrer"
            loading="lazy"
            :srcdoc="srcdoc"
            :style="frameHeight ? { height: frameHeight + 'px' } : undefined"
            @load="frameLoading = false"
          />
          <div v-if="frameLoading" class="frame-loading"><PhSpinnerGap :size="20" class="spin" /></div>
        </div>
        <div v-else class="email-text-body">
          <p v-for="paragraph in message.body" :key="paragraph">{{ paragraph }}</p>
        </div>
      </div>
      <p v-else class="snippet" @click.stop="emit('toggle-expanded', message.id)">{{ message.snippet }}</p>
    </div>
  </article>
</template>

<style scoped>
.email-html-wrap { position: relative; }
.frame-loading {
  position: absolute; inset: 0;
  display: grid; place-items: center;
  background: color-mix(in oklab, var(--surface) 60%, transparent);
  border-radius: 4px; pointer-events: none;
}
.spin { color: var(--accent); animation: frame-spin 0.8s linear infinite; }
@keyframes frame-spin { to { transform: rotate(360deg); } }
</style>
