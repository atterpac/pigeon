// Draft attachments: OS file picker, size-capped queueing, inline image
// embedding, and saving received attachments via the backend.
import { computed, type Ref } from 'vue'
import type { ComposeDraft, MailClient } from '../mail/types'
import { errorMessage } from '../mail/format'
import type { ShellToast } from './useShellToast'

// per-message payload cap; SMTP servers commonly reject more once base64
// expansion (~33%) is counted.
const MAX_ATTACHMENT_BYTES = 25 * 1024 * 1024

type AttachmentDeps = {
  draft: Ref<ComposeDraft>
  status: Ref<string>
  client: Ref<MailClient | null>
  queueSave: () => void
  showToast: (next: Omit<ShellToast, 'id'>, timeout?: number) => void
}

export function useAttachments({ draft, status, client, queueSave, showToast }: AttachmentDeps) {
  // picks files onto the draft. transient input element works from both compose
  // surfaces (modal + reply panel) without a DOM ref.
  function pickAttachments() {
    const input = document.createElement('input')
    input.type = 'file'
    input.multiple = true
    input.addEventListener('change', () => {
      void attachFiles(input.files)
    })
    input.click()
  }
  async function attachFiles(files: FileList | File[] | null) {
    const list = files ? Array.from(files) : []
    if (!list.length) return
    let total = draft.value.attachments.reduce((sum, item) => sum + (item.size ?? 0), 0)
    for (const file of list) {
      if (total + file.size > MAX_ATTACHMENT_BYTES) {
        status.value = `attachment skipped: ${file.name} exceeds 25 MB limit`
        continue
      }
      try {
        const content = await readFileBase64(file)
        draft.value.attachments.push({
          filename: file.name,
          contentType: file.type || 'application/octet-stream',
          content,
          size: file.size,
        })
        total += file.size
      } catch (error) {
        status.value = `attachment failed: ${errorMessage(error)}`
      }
    }
    const count = draft.value.attachments.length
    status.value = `${count} attachment${count === 1 ? '' : 's'} queued`
    queueSave()
  }
  // saves a received attachment. prompt=false → Downloads; prompt=true → native
  // "Save as" dialog.
  async function downloadAttachment(
    messageId: string,
    index: number,
    prompt = false,
    filename = 'Attachment',
  ) {
    if (!client.value?.saveAttachment) {
      status.value = 'download not supported'
      showToast({ kind: 'error', title: 'Download not supported', detail: filename })
      return
    }
    status.value = prompt ? 'choose where to save…' : 'saving attachment…'
    try {
      const path = await client.value.saveAttachment(messageId, index, prompt)
      if (path) {
        status.value = `saved → ${path}`
        showToast({ kind: 'success', title: 'File downloaded', detail: filename })
      } else {
        status.value = 'save cancelled'
        showToast({ kind: 'info', title: 'Download cancelled', detail: filename })
      }
    } catch (error) {
      const message = errorMessage(error)
      status.value = `save failed: ${message}`
      showToast({ kind: 'error', title: 'Download failed', detail: message })
    }
  }
  // embeds an inline image part (referenced from the body via cid:<contentId>),
  // distinct from a file attachment. editor inserts the ![](cid:…) markdown; this
  // stores the bytes.
  async function attachInlineImage(file: File, contentId: string) {
    try {
      const content = await readFileBase64(file)
      draft.value.attachments.push({
        filename: file.name || `${contentId}.png`,
        contentType: file.type || 'image/png',
        content,
        size: file.size,
        contentId,
      })
      queueSave()
    } catch (error) {
      status.value = `inline image failed: ${errorMessage(error)}`
    }
  }
  // cid → inline image (base64), so the compose preview can resolve its own cid:
  // refs to data URLs.
  const inlineImageMap = computed<Record<string, { contentType: string; content: string }>>(() => {
    const map: Record<string, { contentType: string; content: string }> = {}
    for (const attachment of draft.value.attachments) {
      if (attachment.contentId && attachment.content)
        map[attachment.contentId] = {
          contentType: attachment.contentType || 'image/png',
          content: attachment.content,
        }
    }
    return map
  })

  return { pickAttachments, attachFiles, attachInlineImage, downloadAttachment, inlineImageMap }
}

// File → base64 (the encoding the Outfile binding decodes back to []byte),
// stripping the data-URL prefix readAsDataURL adds.
function readFileBase64(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onerror = () => reject(reader.error ?? new Error('read failed'))
    reader.onload = () => resolve(String(reader.result).split(',')[1] ?? '')
    reader.readAsDataURL(file)
  })
}
