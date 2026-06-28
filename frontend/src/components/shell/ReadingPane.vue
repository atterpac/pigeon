<script setup lang="ts">
// Right pane (persistent): compose form when composing, otherwise the selected
// thread with its reply panel, otherwise an empty placeholder.
import { computed, nextTick, ref, watch } from 'vue'
import { useExternalEmailLinks } from '../../composables/useExternalEmailLinks'
import { useMailShell } from '../../composables/useMailShell'
import { useThreadFind } from '../../composables/useThreadFind'
import { formatDate, labelFor } from '../../mail/format'
import ReplyPanel from './ReplyPanel.vue'
import ThreadMessage from './ThreadMessage.vue'
import { PhArchive, PhClock, PhEnvelopeOpen, PhArrowBendUpLeft, PhArrowLeft, PhArrowSquareOut, PhTray, PhSpinnerGap } from '@phosphor-icons/vue'

const emit = defineEmits<{ (e: 'back-to-list'): void }>()
const s = useMailShell()
const previewLabel = computed(() => labelFor(s.selectedConversation.value, s.labels.value))
const threadScroll = ref<HTMLElement | null>(null)
const replyPanel = ref<InstanceType<typeof ReplyPanel> | null>(null)
const activeMailboxName = computed(() => s.mailboxes.value.find((mailbox) => mailbox.id === s.activeMailbox.value)?.name ?? 'mail')
const focusedSenderName = computed(() => s.focusedThreadMessage.value?.from.name || s.focusedThreadMessage.value?.from.addr || 'message')
const focusedSenderEmail = computed(() => s.focusedThreadMessage.value?.from.addr ?? '')
const focusedSenderPath = computed(() => {
  const base = focusedSenderEmail.value.split('@')[0] || focusedSenderName.value
  return base.toLowerCase().replace(/[^a-z0-9._-]+/g, '-').replace(/^-+|-+$/g, '') || 'message'
})

useExternalEmailLinks()

// Let in-thread find (`/`) scroll this region to each match.
const threadFind = useThreadFind()
watch(threadScroll, (el) => threadFind.register(el), { immediate: true })

// Keep the focused message (shift-J/K) in view.
watch(() => s.focusedMessageId.value, (id) => {
  if (!id) return
  nextTick(() => {
    const row = threadScroll.value?.querySelector<HTMLElement>(`[data-message-id="${id}"]`)
    row?.scrollIntoView({ block: 'nearest', behavior: 'auto' })
  })
})

function scrollThread(delta: number | 'top' | 'bottom') {
  const target = threadScroll.value
  if (!target) return
  if (delta === 'top') { target.scrollTo({ top: 0 }); return }
  if (delta === 'bottom') { target.scrollTo({ top: target.scrollHeight }); return }
  target.scrollBy({ top: delta * 72, behavior: 'auto' })
}
function focusReply() {
  replyPanel.value?.focusReply()
}
function openReply() {
  s.openReply('reply')
  focusReply()
}

defineExpose({ scrollThread, focusReply })
</script>

<template>
  <section class="reading-pane" :class="{ focused: s.focusPane.value === 'thread' }" @pointerdown="s.focusThread()">
    <!-- Thread -->
    <template v-if="s.selectedThread.value">
      <header class="thread-header">
        <button class="mobile-back" type="button" @click.stop="emit('back-to-list')"><PhArrowLeft :size="15" /> Inbox</button>
        <div class="thread-title">
          <div class="thread-path">
            <PhTray :size="14" />
            <span>~/{{ activeMailboxName.toLowerCase() }}/{{ focusedSenderPath }}</span>
          </div>
          <h1>{{ s.selectedThread.value.subject }}</h1>
          <p>focused on <b>{{ focusedSenderName }}</b><span v-if="focusedSenderEmail"> · {{ focusedSenderEmail }}</span></p>
        </div>
        <div class="thread-actions">
          <button class="thread-action" type="button" @click="s.archiveThread()"><PhArchive :size="14" /> Archive <kbd>e</kbd></button>
          <button class="thread-action" type="button" @click="s.snoozeThread()"><PhClock :size="14" /> Snooze <kbd>s</kbd></button>
          <button class="thread-action" type="button" @click="s.toggleRead()"><PhEnvelopeOpen :size="14" /> Unread <kbd>u</kbd></button>
          <button class="thread-action primary" type="button" @click="openReply"><PhArrowBendUpLeft :size="15" /> Reply <kbd>r</kbd></button>
        </div>
      </header>
      <div ref="threadScroll" class="thread-messages">
        <ThreadMessage
          v-for="message in s.threadMessages.value"
          :key="message.id"
          :message="message"
          :focused="s.focusedMessageId.value === message.id"
          @toggle-expanded="s.toggleMessageExpanded"
          @focus-message="s.focusMessage"
        />
      </div>
      <ReplyPanel ref="replyPanel" />
    </template>

    <!-- Empty -->
    <div v-else-if="s.selectedConversation.value" class="reading-preview">
      <button class="mobile-back" type="button" @click.stop="emit('back-to-list')"><PhArrowLeft :size="15" /> Inbox</button>
      <span class="preview-eyebrow">{{ s.selectedConversation.value.from.name || s.selectedConversation.value.from.addr }}</span>
      <h1>{{ s.selectedConversation.value.subject }}</h1>
      <p>{{ s.selectedConversation.value.snippet }}</p>
      <div class="preview-meta">
        <span>{{ formatDate(s.selectedConversation.value.lastAt) }}</span>
        <span v-if="previewLabel" :style="{ background: previewLabel.bg, color: previewLabel.fg }">{{ previewLabel.name }}</span>
        <span>{{ s.selectedConversation.value.messageCount }} messages</span>
      </div>
      <button class="primary-action" type="button" @click="s.openThread()"><PhArrowSquareOut :size="15" /> Open conversation</button>
    </div>
    <div v-else class="reading-empty">
      <p>No conversation selected.</p>
    </div>

    <!-- Thread fetch in flight (bodies loading) — overlay, outside the v-if chain -->
    <div v-if="s.threadLoading.value" class="thread-loading">
      <PhSpinnerGap :size="22" class="spin" />
      <span>Loading conversation…</span>
    </div>
  </section>
</template>

<style scoped>
.reading-pane { position: relative; }
.thread-loading {
  position: absolute; inset: 0; z-index: 5;
  display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 10px;
  background: color-mix(in oklab, var(--surface) 72%, transparent);
  backdrop-filter: blur(2px);
  color: var(--text-mut); font: 12px "JetBrains Mono", ui-monospace, monospace;
}
.spin { color: var(--accent); animation: reading-spin 0.8s linear infinite; }
@keyframes reading-spin { to { transform: rotate(360deg); } }
</style>
