<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { PhSpinnerGap, PhDownloadSimple, PhPaperclip, PhFolderOpen } from '@phosphor-icons/vue'
import { avatarStyle, formatBytes, formatDate, initials, renderEmailHtml } from '../../mail/format'
import { useMailShell } from '../../composables/useMailShell'
import { useSettings } from '../../composables/useSettings'
import type { ThreadMessage } from '../../mail/types'

const shell = useMailShell()
const settings = useSettings()

const props = withDefaults(defineProps<{
  message: ThreadMessage
  focused?: boolean
}>(), {
  focused: false,
})

// Memoize the iframe document so it only changes when the HTML body itself
// changes — not on every re-render (focus/scroll). A new srcdoc string forces
// the iframe to fully reload, which is the main source of render-time variance.
const srcdoc = computed(() => (props.message.html ? renderEmailHtml(props.message.html, settings.blockRemoteImages, props.message.inlineImages) : ''))

// Show a spinner over the iframe until it finishes rendering its document.
// Reset whenever the document actually changes (not on unrelated re-renders).
const frameLoading = ref(true)
// Count of remote images the iframe blocked; drives the "Load images" banner.
const blockedImages = ref(0)
watch(srcdoc, () => { frameLoading.value = true; frameHeight.value = 0; blockedImages.value = 0 })
const emit = defineEmits<{
  (e: 'toggle-expanded', id: string): void
  (e: 'focus-message', id: string): void
}>()

// Auto-size the iframe to its content height (reported via postMessage) so the
// email body has no inner scroll — the whole thread scrolls as one region.
const frame = ref<HTMLIFrameElement | null>(null)
const frameHeight = ref(0)
function onMessage(event: MessageEvent) {
  if (!frame.value || event.source !== frame.value.contentWindow) return
  const data = event.data || {}
  if (data.type === 'email-frame-height') {
    const h = Number(data.height)
    if (Number.isFinite(h) && h > 0) frameHeight.value = Math.ceil(h)
  } else if (data.type === 'email-images-blocked') {
    blockedImages.value = Number(data.count) || 0
  }
}
// Ask the iframe to restore the real image sources for this message only.
function loadImages() {
  frame.value?.contentWindow?.postMessage({ type: 'email-load-images' }, '*')
  blockedImages.value = 0
}
onMounted(() => window.addEventListener('message', onMessage))
onBeforeUnmount(() => window.removeEventListener('message', onMessage))

</script>

<template>
  <article
    :class="{ focused }"
    :data-message-id="message.id"
    tabindex="0"
    :aria-expanded="message.expanded"
    :aria-label="`Message from ${message.from.name || message.from.addr}`"
    @click="emit('focus-message', message.id)"
    @focus="emit('focus-message', message.id)"
  >
    <span class="avatar" :style="avatarStyle(message.from)" aria-hidden="true">{{ initials(message.from) }}</span>
    <div>
      <header>
        <strong>{{ message.from.name || message.from.addr }}</strong>
        <span>{{ message.from.addr }}</span>
        <time>{{ formatDate(message.date) }}</time>
      </header>
      <div v-if="message.expanded && (message.to.length || message.cc.length)" class="recipients">
        <span v-if="message.to.length">to {{ message.to.map((addr) => addr.name || addr.addr).join(', ') }}</span>
        <span v-if="message.cc.length">cc {{ message.cc.map((addr) => addr.name || addr.addr).join(', ') }}</span>
      </div>
      <div v-if="message.expanded" class="message-body" @click.stop="emit('toggle-expanded', message.id)">
        <div v-if="message.html" class="email-html-wrap" @click.stop>
          <div v-if="blockedImages" class="img-banner">
            <span>{{ blockedImages }} remote image{{ blockedImages === 1 ? '' : 's' }} blocked</span>
            <button type="button" @click="loadImages">Load images</button>
          </div>
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
        <ul v-if="message.attachments?.length" class="attachment-list" @click.stop>
          <li v-for="attachment in message.attachments" :key="attachment.index">
            <button type="button" class="attachment-main" @click="shell.downloadAttachment(message.id, attachment.index, false, attachment.filename)" :title="`Download ${attachment.filename} to Downloads`">
              <PhPaperclip :size="14" />
              <span class="attachment-name">{{ attachment.filename }}</span>
              <span class="attachment-size">{{ formatBytes(attachment.size) }}</span>
              <PhDownloadSimple :size="14" class="attachment-dl" />
            </button>
            <button type="button" class="attachment-saveas" @click="shell.downloadAttachment(message.id, attachment.index, true, attachment.filename)" title="Save as…">
              <PhFolderOpen :size="14" />
            </button>
          </li>
        </ul>
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

.img-banner {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  margin-bottom: 6px;
  padding: 5px 10px;
  border: 1px solid color-mix(in oklab, var(--orange, var(--accent)) 40%, transparent);
  border-radius: 6px;
  background: color-mix(in oklab, var(--orange, var(--accent)) 12%, transparent);
  color: var(--text);
  font-size: 12px;
}
.img-banner button {
  flex-shrink: 0;
  padding: 3px 10px;
  border: 1px solid var(--accent);
  border-radius: 5px;
  background: transparent;
  color: var(--accent);
  font: inherit;
  font-size: 12px;
  cursor: pointer;
}
.img-banner button:hover { background: color-mix(in oklab, var(--accent) 16%, transparent); }
.recipients {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 14px;
  margin: 2px 0 6px;
  color: color-mix(in oklab, var(--text) 55%, transparent);
  font-size: 11.5px;
}
.attachment-list {
  list-style: none;
  margin: 10px 0 0;
  padding: 0;
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.attachment-list li {
  display: inline-flex;
  align-items: stretch;
  border: 1px solid var(--border, color-mix(in oklab, var(--text) 18%, transparent));
  border-radius: 6px;
  overflow: hidden;
}
.attachment-list button {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 5px 9px;
  border: 0;
  background: color-mix(in oklab, var(--surface) 70%, transparent);
  color: var(--text);
  font: inherit;
  font-size: 12px;
  cursor: pointer;
}
.attachment-main { max-width: 320px; }
.attachment-list button:hover { background: color-mix(in oklab, var(--accent) 16%, transparent); }
.attachment-saveas {
  border-left: 1px solid var(--border, color-mix(in oklab, var(--text) 18%, transparent));
  color: color-mix(in oklab, var(--text) 70%, transparent);
}
.attachment-name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.attachment-size { color: color-mix(in oklab, var(--text) 55%, transparent); flex-shrink: 0; }
.attachment-dl { color: var(--accent); flex-shrink: 0; }
</style>
