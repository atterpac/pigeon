// Single-thread triage (archive/snooze/move/delete/spam/label/star/read): each
// optimistically updates the list, reconciles, and registers an undo inverse.
// Acts on the open thread or highlighted row (targetThreadId).
import type { ComputedRef, Ref } from 'vue'
import type { Conversation, Label, MailClient, Mailbox } from '../mail/types'

type RecordUndo = (
  verb: string,
  undo: () => Promise<void>,
  kind?: 'triage' | 'send',
  count?: number,
  ttl?: number,
) => void

type TriageDeps = {
  client: Ref<MailClient | null>
  status: Ref<string>
  activeMailbox: Ref<string>
  mailboxes: Ref<Mailbox[]>
  labels: Ref<Label[]>
  snoozedActive: Ref<boolean>
  selectedThread: Ref<Conversation | null>
  conversations: Ref<Conversation[]>
  targetThreadId: ComputedRef<string | undefined>
  selectedConversation: ComputedRef<Conversation | null>
  findRow: (id: string) => Conversation | undefined
  patchListConversation: (id: string, patch: Partial<Conversation>) => void
  removeListConversation: (id: string) => void
  bumpMailboxUnread: (mailboxId: string, delta: number) => void
  reloadList: () => Promise<void>
  recordUndo: RecordUndo
}

export function useTriage({
  client,
  status,
  activeMailbox,
  mailboxes,
  labels,
  snoozedActive,
  selectedThread,
  conversations,
  targetThreadId,
  selectedConversation,
  findRow,
  patchListConversation,
  removeListConversation,
  bumpMailboxUnread,
  reloadList,
  recordUndo,
}: TriageDeps) {
  // optimistically drop the thread, run server op, reconcile.
  async function moveOut(id: string | undefined, op: (id: string) => Promise<void>, label: string) {
    if (!client.value || !id) return
    const wasUnread = conversations.value.find((c) => c.id === id)?.unread
    removeListConversation(id)
    if (wasUnread) bumpMailboxUnread(activeMailbox.value, -1)
    status.value = label
    try {
      await op(id)
    } finally {
      await reloadList()
    }
  }
  // inverse: move a thread back into a mailbox, if supported.
  function moveBack(id: string, dst: string) {
    return async () => {
      if (client.value?.moveThread) await client.value.moveThread(id, dst)
    }
  }
  // archive by id with undo; shared by `archiveThread` (open) and `archiveSelected` (row).
  async function archiveById(id: string | undefined, origin: string) {
    await moveOut(id, (i) => client.value!.archiveThread(i), 'archived')
    if (id) recordUndo('Archived', moveBack(id, origin))
  }
  async function archiveThread() {
    await archiveById(selectedThread.value?.id, activeMailbox.value)
  }
  async function archiveSelected() {
    await archiveById(targetThreadId.value, activeMailbox.value)
  }
  async function snoozeThread(until?: string) {
    const id = targetThreadId.value
    await moveOut(id, (i) => client.value!.snoozeThread(i, until), 'snoozed')
    if (id && client.value?.unsnooze) recordUndo('Snoozed', () => client.value!.unsnooze!(id))
  }
  // wake a snoozed thread now (back to inbox); defaults to the selected Snoozed row.
  async function unsnoozeThread(threadId = selectedConversation.value?.id) {
    if (!client.value?.unsnooze || !threadId) return
    removeListConversation(threadId)
    status.value = 'unsnoozed'
    try {
      await client.value.unsnooze(threadId)
    } finally {
      if (snoozedActive.value) await reloadList()
    }
  }
  // move the selected/open thread into a folder (mailbox).
  async function moveThreadTo(mailboxId: string) {
    const id = targetThreadId.value
    const origin = activeMailbox.value
    if (!client.value?.moveThread) {
      status.value = 'move not supported'
      return
    }
    if (!id) {
      status.value = 'move: nothing selected'
      return
    }
    // call moveThread as a method so `this` stays bound (wails client reads
    // this.account); extracting to a variable detaches `this`.
    await moveOut(id, (mid) => client.value!.moveThread!(mid, mailboxId), `moved → ${mailboxId}`)
    recordUndo('Moved', moveBack(id, origin))
  }
  // delete the selected/open thread (to Trash — reversible).
  async function deleteThread() {
    const id = targetThreadId.value
    const origin = activeMailbox.value
    if (!client.value?.deleteThread) {
      status.value = 'delete not supported'
      return
    }
    await moveOut(id, (tid) => client.value!.deleteThread!(tid), 'deleted')
    if (id) recordUndo('Deleted', moveBack(id, origin))
  }
  // report the selected/open thread as spam (move to Spam mailbox).
  async function reportSpam() {
    const id = targetThreadId.value
    const origin = activeMailbox.value
    const spam = mailboxes.value.find((mailbox) => mailbox.role === 'spam')
    if (!client.value?.moveThread) {
      status.value = 'move not supported'
      return
    }
    if (!spam) {
      status.value = 'no spam folder'
      return
    }
    if (!id) {
      status.value = 'spam: nothing selected'
      return
    }
    await moveOut(id, (tid) => client.value!.moveThread!(tid, spam.id), 'reported spam')
    recordUndo('Reported spam', moveBack(id, origin))
  }
  // apply a label without removing the thread from its mailbox.
  async function applyLabel(labelId: string) {
    const id = targetThreadId.value
    if (!client.value?.applyLabel || !id) return
    const row = findRow(id)
    if (row && !row.labelIds.includes(labelId)) row.labelIds = [...row.labelIds, labelId]
    status.value = 'labelled'
    try {
      await client.value.applyLabel(id, labelId)
    } finally {
      await reloadList()
    }
  }
  async function createLabelAndApply(name: string) {
    if (!client.value?.createLabel) {
      status.value = 'labels not supported'
      return
    }
    const label = await client.value.createLabel(name)
    labels.value = await client.value.listLabels()
    await applyLabel(label.id)
  }
  // create a folder and move the selected thread into it.
  async function createFolderAndMove(name: string) {
    if (!client.value?.createMailbox) {
      status.value = 'folders not supported'
      return
    }
    const created = await client.value.createMailbox(name.trim())
    mailboxes.value = await client.value.listMailboxes()
    await moveThreadTo(created.id)
  }
  async function toggleStar(
    conversation: Conversation | null = selectedThread.value ?? selectedConversation.value,
  ) {
    if (!client.value || !conversation) return
    const next = !conversation.starred
    conversation.starred = next
    patchListConversation(conversation.id, { starred: next })
    await client.value.toggleStar(conversation.id, next)
  }
  async function toggleRead() {
    if (!client.value || !selectedThread.value) return
    const id = selectedThread.value.id
    const read = selectedThread.value.unread
    patchListConversation(id, { unread: !read })
    bumpMailboxUnread(activeMailbox.value, read ? -1 : 1)
    await client.value.markThreadRead(id, read)
  }

  return {
    archiveThread,
    archiveSelected,
    snoozeThread,
    unsnoozeThread,
    moveThreadTo,
    deleteThread,
    reportSpam,
    applyLabel,
    createLabelAndApply,
    createFolderAndMove,
    toggleStar,
    toggleRead,
  }
}
