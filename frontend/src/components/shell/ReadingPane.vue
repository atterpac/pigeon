<script setup lang="ts">
// Right pane (persistent): compose form when composing, otherwise the selected
// thread with its reply panel, otherwise an empty placeholder.
import { computed, nextTick, ref } from 'vue'
import { useMailShell } from '../../composables/useMailShell'
import { useSettings } from '../../composables/useSettings'
import { avatarStyle, formatDate, initials, labelFor, participantLine, renderEmailHtml } from '../../mail/format'
import MarkdownEditor from '../editor/MarkdownEditor.vue'
import { PhStar, PhArchive, PhClock, PhEnvelopeOpen, PhCaretUp, PhCaretDown, PhArrowLeft, PhArrowSquareOut } from '@phosphor-icons/vue'

const emit = defineEmits<{ (e: 'back-to-list'): void }>()
const s = useMailShell()
const settings = useSettings()
const previewLabel = computed(() => labelFor(s.selectedConversation.value, s.labels.value))
const threadScroll = ref<HTMLElement | null>(null)
const replyEditor = ref<InstanceType<typeof MarkdownEditor> | null>(null)

function scrollThread(delta: number | 'top' | 'bottom') {
  const target = threadScroll.value
  if (!target) return
  if (delta === 'top') { target.scrollTo({ top: 0 }); return }
  if (delta === 'bottom') { target.scrollTo({ top: target.scrollHeight }); return }
  target.scrollBy({ top: delta * 72, behavior: 'auto' })
}
function focusReply() {
  nextTick(() => replyEditor.value?.focus())
}
function openReply(kind: 'reply' | 'replyAll' | 'forward') {
  s.openReply(kind)
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
        <div class="thread-title"><h1>{{ s.selectedThread.value.subject }}</h1><p>{{ s.selectedThread.value.messageCount }} messages · {{ participantLine(s.selectedThread.value) }}</p></div>
        <div class="thread-actions">
          <button class="ghost-button" type="button" @click="s.toggleStar()"><PhStar :size="14" :weight="s.selectedThread.value?.starred ? 'fill' : 'regular'" /> Star</button>
          <button class="ghost-button" type="button" @click="s.archiveThread()"><PhArchive :size="14" /> Archive <kbd>e</kbd></button>
          <button class="ghost-button" type="button" @click="s.snoozeThread()"><PhClock :size="14" /> Snooze <kbd>s</kbd></button>
          <button class="ghost-button" type="button" @click="s.toggleRead()"><PhEnvelopeOpen :size="14" /> Unread <kbd>u</kbd></button>
        </div>
      </header>
      <div ref="threadScroll" class="thread-messages">
        <article v-for="message in s.threadMessages.value" :key="message.id">
          <span class="avatar" :style="avatarStyle(message.from)">{{ initials(message.from) }}</span>
          <div>
            <header><strong>{{ message.from.name || message.from.addr }}</strong><span>{{ message.from.addr }}</span><time>{{ formatDate(message.date) }}</time></header>
            <div v-if="message.expanded" class="message-body" @click="message.expanded = false">
              <iframe v-if="message.html" class="email-html-frame" sandbox="allow-popups allow-popups-to-escape-sandbox" referrerpolicy="no-referrer" :srcdoc="renderEmailHtml(message.html)" />
              <template v-else><p v-for="paragraph in message.body" :key="paragraph">{{ paragraph }}</p></template>
            </div>
            <p v-else class="snippet" @click="message.expanded = true">{{ message.snippet }}</p>
          </div>
        </article>
      </div>
      <footer class="reply-panel" :class="{ expanded: s.replyExpanded.value }">
        <button v-if="s.replyOpen.value" class="reply-expand-toggle" type="button" @click="s.replyExpanded.value = !s.replyExpanded.value"><PhCaretDown v-if="s.replyExpanded.value" :size="14" /><PhCaretUp v-else :size="14" /></button>
        <div class="reply-tabs" :class="{ compact: !s.replyOpen.value }">
          <button :class="{ active: s.replyMode.value === 'reply' }" type="button" @click="openReply('reply')">Reply</button>
          <button :class="{ active: s.replyMode.value === 'replyAll' }" type="button" @click="openReply('replyAll')">Reply all</button>
          <button :class="{ active: s.replyMode.value === 'forward' }" type="button" @click="openReply('forward')">Forward</button>
          <span>{{ s.replyMode.value === 'forward' ? 'forward draft' : `to ${s.selectedThread.value.from.name || s.selectedThread.value.from.addr}` }}</span>
        </div>
        <MarkdownEditor
          v-if="s.replyOpen.value"
          ref="replyEditor"
          v-model:body="s.draft.value.body"
          variant="reply"
          placeholder="Write your reply...  (⌘↵ to send)"
          :status="s.status.value"
          :reset-key="s.draft.value.id"
          :vim="settings.vimMode"
          :expanded="s.replyExpanded.value"
          @send="s.sendDraft()"
          @attach="s.attachMock()"
        />
      </footer>
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
  </section>
</template>
