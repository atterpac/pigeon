<script setup lang="ts">
import { nextTick, ref } from 'vue'
import { PhCaretDown, PhCaretUp } from '@phosphor-icons/vue'
import { useMailShell } from '../../composables/useMailShell'
import { useSettings } from '../../composables/useSettings'
import MarkdownEditor from '../editor/MarkdownEditor.vue'

const s = useMailShell()
const settings = useSettings()
const replyEditor = ref<InstanceType<typeof MarkdownEditor> | null>(null)

function focusReply() {
  nextTick(() => replyEditor.value?.focus())
}

function openReply(kind: 'reply' | 'replyAll' | 'forward') {
  s.openReply(kind)
  focusReply()
}

defineExpose({ focusReply })
</script>

<template>
  <footer class="reply-panel" :class="{ expanded: s.replyExpanded.value }">
    <button
      v-if="s.replyOpen.value"
      class="reply-expand-toggle"
      type="button"
      @click="s.toggleReplyExpanded()"
    >
      <PhCaretDown v-if="s.replyExpanded.value" :size="14" />
      <PhCaretUp v-else :size="14" />
    </button>
    <div class="reply-tabs" :class="{ compact: !s.replyOpen.value }">
      <button :class="{ active: s.replyMode.value === 'reply' }" type="button" @click="openReply('reply')">Reply</button>
      <button :class="{ active: s.replyMode.value === 'replyAll' }" type="button" @click="openReply('replyAll')">Reply all</button>
      <button :class="{ active: s.replyMode.value === 'forward' }" type="button" @click="openReply('forward')">Forward</button>
      <span>{{ s.replyMode.value === 'forward' ? 'forward draft' : `to ${s.selectedThread.value?.from.name || s.selectedThread.value?.from.addr || ''}` }}</span>
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
