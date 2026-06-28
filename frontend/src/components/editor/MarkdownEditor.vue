<script setup lang="ts">
// Self-contained markdown editor: textarea + line gutter + terminal caret +
// INSERT/NORMAL vim modes + format helpers + preview.
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { type InlineImage, inlineCidImages, renderMarkdown } from '../../mail/format'
import { PhPaperPlaneTilt, PhTextB, PhTextItalic, PhCode, PhLink, PhPaperclip } from '@phosphor-icons/vue'

type EditorMode = 'INSERT' | 'NORMAL'

const props = withDefaults(defineProps<{
  body: string
  placeholder?: string
  status?: string
  variant?: 'reply' | 'compose'
  showDiscard?: boolean
  autofocus?: boolean
  expanded?: boolean
  resetKey?: string
  vim?: boolean
  // cid → inline image, so the preview can resolve embedded ![](cid:…) refs.
  inlineImages?: Record<string, InlineImage>
}>(), { variant: 'reply', placeholder: 'Write a message...', status: '', vim: true })

const emit = defineEmits<{
  (e: 'update:body', value: string): void
  (e: 'send'): void
  (e: 'attach'): void
  (e: 'attach-inline', payload: { file: File; cid: string }): void
  (e: 'attach-files', files: File[]): void
  (e: 'discard'): void
}>()

const body = computed({ get: () => props.body, set: (value) => emit('update:body', value) })

const editorMode = ref<EditorMode>('INSERT')
const preview = ref(false)
const editorFocused = ref(false)
const currentLine = ref(1)
const caretStyle = ref({ transform: 'translate(58px, 20px)', height: '22px' })
const textareaRef = ref<HTMLTextAreaElement | null>(null)
let charCanvas: HTMLCanvasElement | undefined

const lineNumbers = computed(() => Array.from({ length: Math.max(1, props.body.split('\n').length) }, (_, index) => index + 1))
const renderedPreview = computed(() => inlineCidImages(renderMarkdown(props.body), props.inlineImages))

// Paste an image from the clipboard → embed it inline: insert ![name](cid:id)
// markdown at the cursor and hand the bytes up to be stored as an inline part.
function handlePaste(event: ClipboardEvent) {
  const items = event.clipboardData?.items
  if (!items) return
  const images = Array.from(items).filter((item) => item.kind === 'file' && item.type.startsWith('image/'))
  if (!images.length) return
  event.preventDefault()
  for (const item of images) {
    const file = item.getAsFile()
    if (!file) continue
    embedInlineImage(file)
  }
}

// Drag-and-drop: images embed inline (like paste); other files attach normally.
const dragging = ref(false)
function handleDragOver(event: DragEvent) {
  if (!event.dataTransfer?.types.includes('Files')) return
  event.preventDefault()
  dragging.value = true
}
function handleDragLeave(event: DragEvent) {
  // Ignore leaves into child nodes; only clear when leaving the drop zone.
  if (event.currentTarget instanceof Node && event.relatedTarget instanceof Node && (event.currentTarget as Node).contains(event.relatedTarget as Node)) return
  dragging.value = false
}
function handleDrop(event: DragEvent) {
  const files = Array.from(event.dataTransfer?.files ?? [])
  if (!files.length) return
  event.preventDefault()
  dragging.value = false
  const others: File[] = []
  for (const file of files) {
    if (file.type.startsWith('image/')) embedInlineImage(file)
    else others.push(file)
  }
  if (others.length) emit('attach-files', others)
}
function embedInlineImage(file: File) {
  const cid = `img-${Date.now()}-${Math.round(Math.random() * 1e6)}`
  insertAtCursor(`\n![${file.name || 'image'}](cid:${cid})\n`)
  emit('attach-inline', { file, cid })
}
function insertAtCursor(text: string) {
  const textarea = textareaRef.value
  const start = textarea?.selectionStart ?? props.body.length
  const end = textarea?.selectionEnd ?? props.body.length
  body.value = props.body.slice(0, start) + text + props.body.slice(end)
  nextTick(() => {
    const pos = start + text.length
    textarea?.setSelectionRange(pos, pos)
    textarea?.focus()
    updateCaret()
  })
}

watch(() => props.body, () => nextTick(updateCaret))
watch(() => props.resetKey, () => { preview.value = false })

onMounted(() => { if (props.autofocus) nextTick(() => textareaRef.value?.focus()) })

function focus() {
  preview.value = false
  nextTick(() => {
    textareaRef.value?.focus()
    updateCaret()
  })
}
defineExpose({ focus })

function handleEditorKeydown(event: KeyboardEvent) {
  if (!props.vim) return
  if (event.key === 'Escape') { editorMode.value = 'NORMAL'; event.preventDefault(); return }
  if (editorMode.value === 'INSERT') return
  const textarea = textareaRef.value
  if (!textarea) return
  const cursor = textarea.selectionStart
  if (event.key === 'i') editorMode.value = 'INSERT'
  else if (event.key === 'a') { textarea.setSelectionRange(cursor + 1, cursor + 1); editorMode.value = 'INSERT' }
  else if (event.key === 'x') replaceBody(textarea.value.slice(0, cursor) + textarea.value.slice(cursor + 1), cursor)
  else if (event.key === 'j' || event.key === 'ArrowDown') moveEditorLine(1)
  else if (event.key === 'k' || event.key === 'ArrowUp') moveEditorLine(-1)
  else return
  event.preventDefault()
  updateCaret()
}
function replaceBody(value: string, cursor: number) {
  body.value = value
  nextTick(() => textareaRef.value?.setSelectionRange(cursor, cursor))
}
function moveEditorLine(delta: number) {
  const textarea = textareaRef.value
  if (!textarea) return
  const lines = textarea.value.split('\n')
  const before = textarea.value.slice(0, textarea.selectionStart).split('\n')
  const line = before.length - 1
  const column = before.at(-1)?.length ?? 0
  const nextLine = Math.max(0, Math.min(lines.length - 1, line + delta))
  const offset = lines.slice(0, nextLine).join('\n').length + (nextLine === 0 ? 0 : 1)
  const next = offset + Math.min(column, lines[nextLine]?.length ?? 0)
  textarea.setSelectionRange(next, next)
}
function applyFormat(kind: 'bold' | 'italic' | 'code' | 'link') {
  const textarea = textareaRef.value
  if (!textarea) return
  const start = textarea.selectionStart
  const end = textarea.selectionEnd
  const selected = props.body.slice(start, end) || 'text'
  const map = {
    bold: [`**${selected}**`, 2],
    italic: [`_${selected}_`, 1],
    code: selected.includes('\n') ? [`\`\`\`\n${selected}\n\`\`\``, 4] : [`\`${selected}\``, 1],
    link: [`[${selected}](https://)`, selected.length + 3],
  } as const
  const [replacement, offset] = map[kind] as [string, number]
  body.value = `${props.body.slice(0, start)}${replacement}${props.body.slice(end)}`
  nextTick(() => { textarea.focus(); textarea.setSelectionRange(start + offset, start + offset + selected.length); updateCaret() })
}
function updateCaret() {
  const textarea = textareaRef.value
  if (!textarea) return
  const style = window.getComputedStyle(textarea)
  const lineHeight = parseFloat(style.lineHeight) || 22
  const paddingTop = parseFloat(style.paddingTop) || 0
  const paddingLeft = parseFloat(style.paddingLeft) || 0
  const context = (charCanvas ??= document.createElement('canvas')).getContext('2d')
  if (context) context.font = style.font
  const charWidth = context?.measureText('M').width || 8.4
  const before = textarea.value.slice(0, textarea.selectionStart)
  const lines = before.split('\n')
  currentLine.value = lines.length
  const column = lines.at(-1)?.length ?? 0
  caretStyle.value = { transform: `translate(${paddingLeft + column * charWidth - textarea.scrollLeft}px, ${paddingTop + (currentLine.value - 1) * lineHeight - textarea.scrollTop}px)`, height: `${lineHeight}px` }
}
function onFocus() { editorFocused.value = true; nextTick(updateCaret) }
function onBlur() { editorFocused.value = false }
</script>

<template>
  <div class="md-editor" :class="[variant, { expanded, 'no-vim': !vim }]">
    <div class="editor-shell" :class="{ dragging }" @dragover="handleDragOver" @dragleave="handleDragLeave" @drop="handleDrop">
      <div v-if="dragging" class="drop-hint">Drop images to embed · files to attach</div>
      <button class="preview-toggle" :class="{ active: preview }" type="button" @click="preview = !preview">{{ preview ? 'Edit' : 'Preview' }}</button>
      <ol class="line-gutter" aria-hidden="true"><li v-for="line in lineNumbers" :key="line" :class="{ current: editorFocused && line === currentLine }">{{ line }}</li></ol>
      <textarea v-if="!preview" ref="textareaRef" v-model="body" class="editor-input" spellcheck="true" :placeholder="placeholder" wrap="off" @keydown="handleEditorKeydown" @paste="handlePaste" @focus="onFocus" @blur="onBlur" @click="updateCaret" @keyup="updateCaret" @select="updateCaret" @scroll="updateCaret" @input="updateCaret" />
      <span v-if="vim && !preview && editorFocused" class="terminal-caret" :style="caretStyle" />
      <div v-if="preview" class="editor-preview" v-html="renderedPreview" />
    </div>
    <footer class="compose-toolbar">
      <button type="button" class="primary-action" @click="emit('send')"><PhPaperPlaneTilt :size="14" /> Send <kbd>⌘↵</kbd></button>
      <button type="button" class="format-button" @mousedown.prevent @click="applyFormat('bold')" aria-label="Bold"><PhTextB :size="15" /></button>
      <button type="button" class="format-button" @mousedown.prevent @click="applyFormat('italic')" aria-label="Italic"><PhTextItalic :size="15" /></button>
      <button type="button" class="format-button" @mousedown.prevent @click="applyFormat('code')" aria-label="Code"><PhCode :size="15" /></button>
      <button type="button" class="format-button" @mousedown.prevent @click="applyFormat('link')" aria-label="Link"><PhLink :size="15" /></button>
      <button type="button" class="ghost-button" @click="emit('attach')"><PhPaperclip :size="14" /> Attach</button>
      <span class="editor-status"><b>{{ editorMode }}</b> · {{ status }}</span>
      <button v-if="showDiscard" type="button" class="ghost-button" @click="emit('discard')">Discard</button>
    </footer>
  </div>
</template>
