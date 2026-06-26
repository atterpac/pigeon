<script setup lang="ts">
// Centered modal compose overlay (the default compose surface). Wraps the same
// compose form + MarkdownEditor that previously lived in the reading pane.
// ⌘↵ send and draft autosave are handled by the shell; esc/backdrop close here.
import { computed } from 'vue'
import { useMailShell } from '../../composables/useMailShell'
import { useSettings } from '../../composables/useSettings'
import MarkdownEditor from '../editor/MarkdownEditor.vue'
import { PhX } from '@phosphor-icons/vue'

const s = useMailShell()
const settings = useSettings()
const nonDim = computed(() => ['docked', 'side', 'split'].includes(settings.compose))
</script>

<template>
  <div class="modal-backdrop" :class="[`compose-${settings.compose}`, { 'no-dim': nonDim }]" @click.self="s.discardDraft()">
    <form class="compose-card" @submit.prevent="s.sendDraft()">
      <header>
        <h1>New message</h1>
        <button class="modal-close" type="button" @click="s.discardDraft()" aria-label="Close">esc <PhX :size="12" /></button>
      </header>
      <label><span>to</span><em v-for="to in s.draft.value.to" :key="to.addr">{{ to.name || to.addr }}</em><input v-model="s.recipientInput.value" placeholder="Add recipients..." /></label>
      <label><span>subject</span><input v-model="s.draft.value.subject" placeholder="Subject" /></label>
      <MarkdownEditor
        v-model:body="s.draft.value.body"
        variant="compose"
        placeholder="Write a message..."
        :status="s.status.value"
        :reset-key="s.draft.value.id"
        :vim="settings.vimMode"
        :show-discard="true"
        :autofocus="true"
        @send="s.sendDraft()"
        @attach="s.attachMock()"
        @discard="s.discardDraft()"
      />
      <div v-if="s.draft.value.attachments.length" class="attachment-row">
        <button v-for="attachment in s.draft.value.attachments" :key="attachment.filename" type="button" @click="s.draft.value.attachments = s.draft.value.attachments.filter((item) => item.filename !== attachment.filename)">{{ attachment.filename }} <PhX :size="11" /></button>
      </div>
    </form>
  </div>
</template>
