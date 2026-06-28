<script setup lang="ts">
// Centered modal compose overlay (the default compose surface). Wraps the same
// compose form + MarkdownEditor that previously lived in the reading pane.
// ⌘↵ send and draft autosave are handled by the shell; esc/backdrop close here.
import { computed, ref } from 'vue'
import { useMailShell } from '../../composables/useMailShell'
import { useSettings } from '../../composables/useSettings'
import MarkdownEditor from '../editor/MarkdownEditor.vue'
import { formatBytes } from '../../mail/format'
import { PhX } from '@phosphor-icons/vue'
import type { Address, Contact } from '../../mail/types'

const s = useMailShell()
const settings = useSettings()
const nonDim = computed(() => ['docked', 'side', 'split'].includes(settings.compose))
// Cc/Bcc start hidden to keep the default compose minimal; auto-reveal if a
// draft already carries either (e.g. a reply-all that pre-filled Cc).
const showCcBcc = ref(s.draft.value.cc.length > 0 || s.draft.value.bcc.length > 0)
// Inline images live in the body (cid:), so keep them out of the file-chip row.
const fileAttachments = computed(() => s.draft.value.attachments.filter((attachment) => !attachment.contentId))

// ── recipient autocomplete ──────────────────────────────────────
// One dropdown is shared across To/Cc/Bcc; `activeField` says which input it
// belongs to. Suggestions come from the harvested address book, debounced and
// filtered against addresses already chipped into that field.
type Field = 'to' | 'cc' | 'bcc'
const inputs: Record<Field, { value: string }> = { to: s.recipientInput, cc: s.ccInput, bcc: s.bccInput }
const chips: Record<Field, () => Address[]> = {
  to: () => s.draft.value.to,
  cc: () => s.draft.value.cc,
  bcc: () => s.draft.value.bcc,
}
const activeField = ref<Field | null>(null)
const suggestions = ref<Contact[]>([])
const highlight = ref(0)
let debounce: number | undefined

function onInput(field: Field) {
  activeField.value = field
  // Autocomplete the recipient currently being typed (the last comma segment).
  const token = inputs[field].value.split(',').pop()?.trim() ?? ''
  if (debounce) window.clearTimeout(debounce)
  if (!token) { suggestions.value = []; return }
  debounce = window.setTimeout(async () => {
    if (activeField.value !== field) return
    const results = await s.searchContacts(token)
    const chosen = new Set(chips[field]().map((address) => address.addr.toLowerCase()))
    suggestions.value = results.filter((contact) => !chosen.has(contact.addr.toLowerCase()))
    highlight.value = 0
  }, 120)
}

function onKeydown(field: Field, event: KeyboardEvent) {
  if (activeField.value !== field || !suggestions.value.length) return
  if (event.key === 'ArrowDown') {
    event.preventDefault()
    highlight.value = (highlight.value + 1) % suggestions.value.length
  } else if (event.key === 'ArrowUp') {
    event.preventDefault()
    highlight.value = (highlight.value - 1 + suggestions.value.length) % suggestions.value.length
  } else if (event.key === 'Enter' || event.key === 'Tab') {
    const choice = suggestions.value[highlight.value]
    if (choice) {
      // Stop Enter from submitting the form / Tab from leaving the field.
      event.preventDefault()
      event.stopPropagation()
      pick(field, choice)
    }
  } else if (event.key === 'Escape') {
    event.preventDefault()
    event.stopPropagation()
    closeSuggestions()
  }
}

function pick(field: Field, contact: Contact) {
  // Preserve any earlier comma-separated recipients; only the trailing token is
  // the one being completed, so swap it for the chosen chip.
  const head = inputs[field].value.split(',').slice(0, -1).map((part) => part.trim()).filter(Boolean)
  for (const address of head) chips[field]().push({ name: '', addr: address })
  chips[field]().push({ name: contact.name, addr: contact.addr })
  inputs[field].value = ''
  closeSuggestions()
}

function closeSuggestions() {
  suggestions.value = []
  activeField.value = null
  highlight.value = 0
}

// Delay so a mousedown on a suggestion registers before the list is torn down.
function onBlur() {
  window.setTimeout(closeSuggestions, 120)
}
</script>

<template>
  <div class="modal-backdrop" :class="[`compose-${settings.compose}`, { 'no-dim': nonDim }]" @click.self="s.discardDraft()">
    <form class="compose-card" @submit.prevent="s.sendDraft()">
      <header>
        <h1>New message</h1>
        <button class="modal-close" type="button" @click="s.discardDraft()" aria-label="Close">esc <PhX :size="12" /></button>
      </header>
      <label class="recipient-field"><span>to</span><em v-for="to in s.draft.value.to" :key="to.addr">{{ to.name || to.addr }}</em><input v-model="s.recipientInput.value" placeholder="Add recipients..." autocomplete="off" @input="onInput('to')" @keydown="onKeydown('to', $event)" @blur="onBlur" /><button v-if="!showCcBcc" type="button" class="ccbcc-toggle" @click="showCcBcc = true">Cc/Bcc</button>
        <ul v-if="activeField === 'to' && suggestions.length" class="contact-suggest" role="listbox">
          <li v-for="(contact, index) in suggestions" :key="contact.addr" class="contact-option" :class="{ active: index === highlight }" role="option" :aria-selected="index === highlight" @mousedown.prevent="pick('to', contact)" @mouseenter="highlight = index">
            <strong v-if="contact.name">{{ contact.name }}</strong><span>{{ contact.addr }}</span>
          </li>
        </ul>
      </label>
      <label v-if="showCcBcc" class="recipient-field"><span>cc</span><em v-for="cc in s.draft.value.cc" :key="cc.addr">{{ cc.name || cc.addr }}</em><input v-model="s.ccInput.value" placeholder="Cc recipients..." autocomplete="off" @input="onInput('cc')" @keydown="onKeydown('cc', $event)" @blur="onBlur" />
        <ul v-if="activeField === 'cc' && suggestions.length" class="contact-suggest" role="listbox">
          <li v-for="(contact, index) in suggestions" :key="contact.addr" class="contact-option" :class="{ active: index === highlight }" role="option" :aria-selected="index === highlight" @mousedown.prevent="pick('cc', contact)" @mouseenter="highlight = index">
            <strong v-if="contact.name">{{ contact.name }}</strong><span>{{ contact.addr }}</span>
          </li>
        </ul>
      </label>
      <label v-if="showCcBcc" class="recipient-field"><span>bcc</span><em v-for="bcc in s.draft.value.bcc" :key="bcc.addr">{{ bcc.name || bcc.addr }}</em><input v-model="s.bccInput.value" placeholder="Bcc recipients..." autocomplete="off" @input="onInput('bcc')" @keydown="onKeydown('bcc', $event)" @blur="onBlur" />
        <ul v-if="activeField === 'bcc' && suggestions.length" class="contact-suggest" role="listbox">
          <li v-for="(contact, index) in suggestions" :key="contact.addr" class="contact-option" :class="{ active: index === highlight }" role="option" :aria-selected="index === highlight" @mousedown.prevent="pick('bcc', contact)" @mouseenter="highlight = index">
            <strong v-if="contact.name">{{ contact.name }}</strong><span>{{ contact.addr }}</span>
          </li>
        </ul>
      </label>
      <label><span>subject</span><input v-model="s.draft.value.subject" placeholder="Subject" /></label>
      <label v-if="s.signatureOptions.value.length" class="signature-select-row">
        <span>signature</span>
        <select class="compose-signature-select" :value="s.draft.value.signatureId" @change="s.setDraftSignature(($event.target as HTMLSelectElement).value)">
          <option value="">None</option>
          <option v-for="signature in s.signatureOptions.value" :key="signature.id" :value="signature.id">{{ signature.name || 'Untitled' }}</option>
        </select>
      </label>
      <MarkdownEditor
        v-model:body="s.draft.value.body"
        variant="compose"
        placeholder="Write a message... (paste an image to embed it inline)"
        :status="s.status.value"
        :reset-key="s.draft.value.id"
        :vim="settings.vimMode"
        :show-discard="true"
        :autofocus="true"
        :inline-images="s.inlineImageMap.value"
        @send="s.sendDraft()"
        @attach="s.pickAttachments()"
        @attach-inline="(p) => s.attachInlineImage(p.file, p.cid)"
        @attach-files="(files) => s.attachFiles(files)"
        @discard="s.discardDraft()"
      />
      <div v-if="fileAttachments.length" class="attachment-row">
        <button v-for="attachment in fileAttachments" :key="attachment.filename" type="button" @click="s.draft.value.attachments = s.draft.value.attachments.filter((item) => item.filename !== attachment.filename)">{{ attachment.filename }}<small v-if="attachment.size"> · {{ formatBytes(attachment.size) }}</small> <PhX :size="11" /></button>
      </div>
    </form>
  </div>
</template>

<style scoped>
.recipient-field { position: relative; }
.contact-suggest {
  position: absolute;
  top: 100%;
  left: 0;
  right: 0;
  z-index: 30;
  margin: 4px 0 0;
  padding: 4px;
  list-style: none;
  max-height: 220px;
  overflow-y: auto;
  background: var(--surface-2);
  border: 1px solid var(--border-2);
  border-radius: 8px;
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.35);
}
.contact-option {
  display: flex;
  align-items: baseline;
  gap: 8px;
  padding: 6px 8px;
  border-radius: 6px;
  cursor: pointer;
  font: 12px "JetBrains Mono", ui-monospace, monospace;
  color: var(--text-mut);
}
.contact-option.active { background: var(--accent-soft, rgba(94, 84, 192, 0.18)); color: var(--text); }
.contact-option strong { color: var(--text); font-weight: 600; }
.contact-option span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.ccbcc-toggle {
  margin-left: auto;
  padding: 2px 8px;
  border: 1px solid var(--border-2);
  border-radius: 6px;
  background: transparent;
  color: var(--text-mut);
  font: 11px "JetBrains Mono", ui-monospace, monospace;
  cursor: pointer;
}
.ccbcc-toggle:hover { color: var(--text); border-color: var(--accent-line); }
.signature-select-row { min-height: 44px; }
.compose-signature-select {
  flex: 1;
  min-width: 0;
  border: 1px solid var(--border-2);
  border-radius: 8px;
  background: var(--surface-2);
  color: var(--text);
  padding: 7px 10px;
  outline: none;
  font: 12px "JetBrains Mono", ui-monospace, monospace;
}
.compose-signature-select:focus { border-color: var(--accent-line); }
</style>
