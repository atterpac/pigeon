// Compose + reply: draft state, recipients, signatures, autosave, send (with
// undo-send hold), and attachments. Reads open thread for reply prep.
import { computed, ref, type Ref } from 'vue'
import {
  newDraft,
  replaceSignature,
  type ReplyMode,
  replyDraft,
  withSignature,
} from '../mail/drafts'
import { defaultSignatureId, signatureBody, signatureHTML, signaturesFor } from '../mail/signatures'
import { parseAddresses } from '../mail/format'
import { useAttachments } from './useAttachments'
import type { Account, Conversation, MailClient, ThreadMessage } from '../mail/types'
import type { Settings } from './useSettings'
import type { ShellToast } from './useShellToast'

type ComposeDeps = {
  client: Ref<MailClient | null>
  account: Ref<Account | null>
  status: Ref<string>
  settings: Settings
  selectedThread: Ref<Conversation | null>
  threadMessages: Ref<ThreadMessage[]>
  composeOpen: Ref<boolean>
  showToast: (next: Omit<ShellToast, 'id'>, timeout?: number) => void
  recordUndo: (
    verb: string,
    undo: () => Promise<void>,
    kind?: 'triage' | 'send',
    count?: number,
    ttl?: number,
  ) => void
}

export function useCompose({
  client,
  account,
  status,
  settings,
  selectedThread,
  threadMessages,
  composeOpen,
  showToast,
  recordUndo,
}: ComposeDeps) {
  const draft = ref(newDraft())
  const recipientInput = ref('')
  const ccInput = ref('')
  const bccInput = ref('')
  const replyMode = ref<ReplyMode>('reply')
  const replyOpen = ref(false)
  const replyExpanded = ref(false)

  const signatureOptions = computed(() => signaturesFor(settings, account.value?.id))

  function prepareReply(replyKind: ReplyMode) {
    replyMode.value = replyKind
    draft.value = replyDraft(
      replyKind,
      selectedThread.value,
      threadMessages.value,
      account.value?.email ?? '',
    )
    draft.value.signatureId = defaultSignatureId(settings, account.value?.id)
  }
  function openReply(replyKind: ReplyMode) {
    prepareReply(replyKind)
    // add signature only on actual open, not the auto-prep run for every thread.
    draft.value.body = withSignature(draft.value.body, signatureFor(draft.value.signatureId))
    replyOpen.value = true
  }
  function toggleReplyExpanded() {
    replyExpanded.value = !replyExpanded.value
  }
  // active account's signature (empty when none configured).
  function signatureFor(id = defaultSignatureId(settings, account.value?.id)) {
    return signatureBody(settings, account.value?.id, id)
  }
  function signatureHtmlFor(id = defaultSignatureId(settings, account.value?.id)) {
    return signatureHTML(settings, account.value?.id, id)
  }
  function draftWithDefaultSignature() {
    const signatureId = defaultSignatureId(settings, account.value?.id)
    return newDraft({
      signatureId,
      signatureHtml: signatureHtmlFor(signatureId),
      body: withSignature('', signatureFor(signatureId)),
    })
  }
  function setDraftSignature(id: string) {
    draft.value.signatureId = id
    draft.value.signatureHtml = signatureHtmlFor(id)
    draft.value.body = replaceSignature(draft.value.body, signatureFor(id))
  }
  function compose() {
    draft.value = draftWithDefaultSignature()
    recipientInput.value = ''
    ccInput.value = ''
    bccInput.value = ''
    composeOpen.value = true
  }
  async function sendDraft() {
    if (!client.value) return
    const outgoing = materializeRecipients()
    if (!outgoing.to.length && composeOpen.value) {
      status.value = 'add at least one recipient'
      return
    }
    const hold = Math.max(0, settings.sendUndoSeconds | 0)
    // snapshot so "Undo send" can reopen the draft for editing after cancel.
    const restore = { ...draft.value }
    const opId = await client.value.sendDraft(outgoing, hold)
    composeOpen.value = false
    draft.value = draftWithDefaultSignature()
    if (opId && hold > 0 && client.value.cancelSend) {
      status.value = 'sending…'
      recordUndo(
        'Sending…',
        async () => {
          await client.value!.cancelSend!(opId)
          draft.value = restore
          composeOpen.value = true
          status.value = 'send cancelled'
        },
        'send',
        1,
        hold * 1000,
      )
    } else {
      status.value = 'sent'
    }
  }
  async function discardDraft() {
    if (client.value) await client.value.discardDraft(draft.value.id)
    draft.value = draftWithDefaultSignature()
    composeOpen.value = false
  }
  // fold unconfirmed To/Cc/Bcc input text into the draft's address lists, so a
  // recipient typed but not "chipped" isn't dropped on save/send.
  function materializeRecipients() {
    const to = parseAddresses(recipientInput.value)
    if (to.length) draft.value.to = [...draft.value.to, ...to]
    const cc = parseAddresses(ccInput.value)
    if (cc.length) draft.value.cc = [...draft.value.cc, ...cc]
    const bcc = parseAddresses(bccInput.value)
    if (bcc.length) draft.value.bcc = [...draft.value.bcc, ...bcc]
    recipientInput.value = ''
    ccInput.value = ''
    bccInput.value = ''
    return draft.value
  }
  let saveTimer: number | undefined
  // autosave persists the draft as-is; explicit saves (`:w`, send) pass
  // materialize=true to fold in unconfirmed To/Cc/Bcc text. kept apart so a body
  // keystroke doesn't chip — and clear — a half-typed address.
  function queueSave(materialize = false) {
    if (!client.value || (!composeOpen.value && !selectedThread.value)) return
    status.value = 'saving...'
    if (saveTimer) window.clearTimeout(saveTimer)
    saveTimer = window.setTimeout(async () => {
      if (!client.value) return
      draft.value = await client.value.saveDraft(
        materialize ? materializeRecipients() : draft.value,
      )
      status.value = 'draft saved'
    }, 350)
  }
  function cancelAutosave() {
    if (saveTimer) {
      window.clearTimeout(saveTimer)
      saveTimer = undefined
    }
  }

  const attachments = useAttachments({ draft, status, client, queueSave, showToast })

  return {
    draft,
    recipientInput,
    ccInput,
    bccInput,
    replyMode,
    replyOpen,
    replyExpanded,
    signatureOptions,
    prepareReply,
    openReply,
    toggleReplyExpanded,
    compose,
    sendDraft,
    discardDraft,
    materializeRecipients,
    queueSave,
    setDraftSignature,
    cancelAutosave,
    ...attachments,
  }
}
