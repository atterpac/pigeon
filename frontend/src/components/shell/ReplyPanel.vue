<script setup lang="ts">
import { computed, nextTick, ref } from 'vue'
import { PhCaretDown, PhCaretUp, PhX } from '@phosphor-icons/vue'
import { useMailShell } from '../../composables/useMailShell'
import { useSettings } from '../../composables/useSettings'
import { formatBytes } from '../../mail/format'
import MarkdownEditor from '../editor/MarkdownEditor.vue'

const s = useMailShell()
const settings = useSettings()
// Inline images live in the body (cid:), so keep them out of the file-chip row.
const fileAttachments = computed(() => s.draft.value.attachments.filter((attachment) => !attachment.contentId))
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
      :aria-label="s.replyExpanded.value ? 'Collapse reply' : 'Expand reply'"
      :aria-expanded="s.replyExpanded.value"
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
    <div v-if="s.replyOpen.value && s.draft.value.cc.length" class="reply-cc">cc {{ s.draft.value.cc.map((addr) => addr.name || addr.addr).join(', ') }}</div>
    <div v-if="s.replyOpen.value && s.signatureOptions.value.length" class="reply-signature-row">
      <span>signature</span>
      <select :value="s.draft.value.signatureId" @change="s.setDraftSignature(($event.target as HTMLSelectElement).value)">
        <option value="">None</option>
        <option v-for="signature in s.signatureOptions.value" :key="signature.id" :value="signature.id">{{ signature.name || 'Untitled' }}</option>
      </select>
    </div>
    <MarkdownEditor
      v-if="s.replyOpen.value"
      ref="replyEditor"
      v-model:body="s.draft.value.body"
      variant="reply"
      placeholder="Write your reply...  (⌘↵ to send · paste an image to embed it)"
      :status="s.status.value"
      :reset-key="s.draft.value.id"
      :vim="settings.vimMode"
      :expanded="s.replyExpanded.value"
      :inline-images="s.inlineImageMap.value"
      @send="s.sendDraft()"
      @attach="s.pickAttachments()"
      @attach-inline="(p) => s.attachInlineImage(p.file, p.cid)"
      @attach-files="(files) => s.attachFiles(files)"
    />
    <div v-if="s.replyOpen.value && fileAttachments.length" class="attachment-row">
      <button v-for="attachment in fileAttachments" :key="attachment.filename" type="button" @click="s.draft.value.attachments = s.draft.value.attachments.filter((item) => item.filename !== attachment.filename)">{{ attachment.filename }}<small v-if="attachment.size"> · {{ formatBytes(attachment.size) }}</small> <PhX :size="11" /></button>
    </div>
  </footer>
</template>

<style scoped>
.reply-cc {
  padding: 2px 14px 0;
  color: color-mix(in oklab, var(--text) 55%, transparent);
  font-size: 11.5px;
}
.reply-signature-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 6px 14px 0;
  color: var(--text-mut);
  font: 11px "JetBrains Mono", ui-monospace, monospace;
}
.reply-signature-row select {
  flex: 1;
  min-width: 0;
  border: 1px solid var(--border-2);
  border-radius: 7px;
  background: var(--surface-2);
  color: var(--text);
  padding: 5px 8px;
  outline: none;
}
.reply-signature-row select:focus { border-color: var(--accent-line); }
</style>
