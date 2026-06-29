// Per-account signature editor: list selection plus the contenteditable rich
// editor (paste-from-web, inline image embedding).
import { computed, nextTick, ref, watch, type Ref } from 'vue'
import type { EmailSignature, Settings } from './useSettings'
import {
  deleteSignature,
  emptySignature,
  plainSignatureHtml,
  sanitizeSignatureHtml,
  saveSignature,
  signaturesFor,
} from '../mail/signatures'

export function useSignatureEditor(settings: Settings, accountId: Ref<string>) {
  const richEditor = ref<HTMLElement | null>(null)
  const selectedSignatureId = ref('')

  const signatures = computed(() => signaturesFor(settings, accountId.value))
  const selectedSignature = computed(
    () => signatures.value.find((signature) => signature.id === selectedSignatureId.value) ?? null,
  )
  const defaultSignatureId = computed(() => settings.defaultSignatureIds[accountId.value] ?? '')

  watch(
    signatures,
    (list) => {
      if (!list.length) {
        selectedSignatureId.value = ''
        return
      }
      if (!list.some((signature) => signature.id === selectedSignatureId.value)) {
        selectedSignatureId.value = defaultSignatureId.value || list[0]?.id || ''
      }
    },
    { immediate: true },
  )

  watch(selectedSignature, () => nextTick(syncRichEditor), { immediate: true })

  function addSignature() {
    if (!accountId.value) return
    const next = emptySignature()
    next.name = signatures.value.length ? `Signature ${signatures.value.length + 1}` : 'Default'
    saveSignature(settings, accountId.value, next)
    selectedSignatureId.value = next.id
  }

  function updateSignature(patch: Partial<EmailSignature>) {
    if (!accountId.value || !selectedSignature.value) return
    saveSignature(settings, accountId.value, { ...selectedSignature.value, ...patch })
  }

  function syncRichEditor() {
    if (!richEditor.value || !selectedSignature.value) return
    // don't rewrite mid-edit: replacing innerHTML collapses the selection and
    // snaps the caret to the start.
    if (document.activeElement === richEditor.value) return
    const html = selectedSignature.value.html || plainSignatureHtml(selectedSignature.value.body)
    if (richEditor.value.innerHTML !== html) richEditor.value.innerHTML = html
  }

  function saveRichSignature() {
    if (!richEditor.value) return
    const html = sanitizeSignatureHtml(richEditor.value.innerHTML)
    updateSignature({ html, body: richEditor.value.innerText.trim() })
  }

  async function pasteRichSignature(event: ClipboardEvent) {
    event.preventDefault()
    const items = Array.from(event.clipboardData?.items ?? [])
    const imageItems = items.filter((item) => item.type.startsWith('image/'))
    if (imageItems.length) {
      for (const item of imageItems) {
        const file = item.getAsFile()
        if (!file) continue
        const dataUrl = await fileToDataURL(file)
        insertHtmlAtCursor(
          `<img src="${dataUrl}" alt="${file.name.replace(/"/g, '&quot;')}" style="max-width:220px;height:auto">`,
        )
      }
      saveRichSignature()
      return
    }
    const html = event.clipboardData?.getData('text/html')
    const text = event.clipboardData?.getData('text/plain')
    insertHtmlAtCursor(sanitizeSignatureHtml(html || plainSignatureHtml(text || '')))
    saveRichSignature()
  }

  function insertHtmlAtCursor(html: string) {
    richEditor.value?.focus()
    // execCommand is deprecated but the only one-liner that inserts at the caret
    // with native undo; contenteditable has no standard replacement.
    document.execCommand('insertHTML', false, html)
  }

  function fileToDataURL(file: File) {
    return new Promise<string>((resolve, reject) => {
      const reader = new FileReader()
      reader.onerror = () => reject(reader.error ?? new Error('read failed'))
      reader.onload = () => resolve(String(reader.result || ''))
      reader.readAsDataURL(file)
    })
  }

  function removeSignature(id: string) {
    if (!accountId.value) return
    deleteSignature(settings, accountId.value, id)
  }

  function setDefaultSignature(id: string) {
    if (!accountId.value) return
    settings.defaultSignatureIds = { ...settings.defaultSignatureIds, [accountId.value]: id }
  }

  return {
    richEditor,
    selectedSignatureId,
    signatures,
    selectedSignature,
    defaultSignatureId,
    addSignature,
    updateSignature,
    saveRichSignature,
    pasteRichSignature,
    removeSignature,
    setDefaultSignature,
  }
}
