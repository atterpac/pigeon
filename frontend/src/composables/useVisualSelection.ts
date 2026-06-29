// Visual (multi-select) mode + batch triage. `v` enters, j/k navigate, space
// cherry-picks rows into selectedIds, then a verb acts on the set.
// `visualMode`/`selectedIds` owned by the shell (reset on mailbox switch); batch
// ops and undo wiring live here.
import type { ComputedRef, Ref } from 'vue'
import type { Conversation, Label, MailClient, Mailbox } from '../mail/types'

type RecordUndo = (
  verb: string,
  undo: () => Promise<void>,
  kind?: 'triage' | 'send',
  count?: number,
  ttl?: number,
) => void

type VisualSelectionDeps = {
  client: Ref<MailClient | null>
  status: Ref<string>
  activeMailbox: Ref<string>
  mailboxes: Ref<Mailbox[]>
  labels: Ref<Label[]>
  focusPane: Ref<'list' | 'thread'>
  visualMode: Ref<boolean>
  selectedIds: Ref<Set<string>>
  activeList: ComputedRef<Conversation[]>
  selectedConversation: ComputedRef<Conversation | null>
  mode: ComputedRef<string>
  findRow: (id: string) => Conversation | undefined
  patchListConversation: (id: string, patch: Partial<Conversation>) => void
  removeListConversation: (id: string) => void
  bumpMailboxUnread: (mailboxId: string, delta: number) => void
  reloadList: () => Promise<void>
  recordUndo: RecordUndo
}

export function useVisualSelection({
  client,
  status,
  activeMailbox,
  mailboxes,
  labels,
  focusPane,
  visualMode,
  selectedIds,
  activeList,
  selectedConversation,
  mode,
  findRow,
  patchListConversation,
  removeListConversation,
  bumpMailboxUnread,
  reloadList,
  recordUndo,
}: VisualSelectionDeps) {
  // entering Visual selects nothing — j/k navigate, space opts rows in.
  function enterVisual() {
    if (!activeList.value.length) return
    visualMode.value = true
    focusPane.value = 'list'
    selectedIds.value = new Set()
    status.value = 'VISUAL'
  }
  function exitVisual() {
    if (!visualMode.value) return
    visualMode.value = false
    selectedIds.value = new Set()
    status.value = mode.value.toLowerCase()
  }
  // toggle the focused row's membership (spacebar).
  function toggleSelect(id = selectedConversation.value?.id) {
    if (!id) return
    const next = new Set(selectedIds.value)
    if (next.has(id)) next.delete(id)
    else next.add(id)
    selectedIds.value = next
  }
  function toggleSelectAll() {
    const ids = activeList.value.map((conversation) => conversation.id)
    selectedIds.value =
      selectedIds.value.size >= ids.length && ids.length > 0 ? new Set() : new Set(ids)
  }
  function selectionConversations() {
    return activeList.value.filter((conversation) => selectedIds.value.has(conversation.id))
  }
  // run the batch client method if present, else one call per id. (invoked on
  // client.value so `this` stays bound — see moveThreadTo.)
  async function applyBatch(
    ids: string[],
    batch: ((ids: string[]) => Promise<void>) | undefined,
    single: (id: string) => Promise<void>,
  ) {
    if (batch) await batch(ids)
    else for (const id of ids) await single(id)
  }
  // triage op over the selection: optimistically drop rows, then batch (or
  // per-thread) client call.
  async function runSelectionMoveOut(
    label: string,
    batch: ((ids: string[]) => Promise<void>) | undefined,
    single: (id: string) => Promise<void>,
    undoLabel?: string,
    undo?: (ids: string[]) => () => Promise<void>,
  ) {
    const ids = [...selectedIds.value]
    if (!client.value || !ids.length) return
    for (const id of ids) removeListConversation(id)
    status.value = `${label} ${ids.length}`
    exitVisual()
    try {
      await applyBatch(ids, batch, single)
    } finally {
      await reloadList()
    }
    if (undoLabel && undo) recordUndo(undoLabel, undo(ids), 'triage', ids.length)
  }
  // inverse of a batch move-out: send every thread back to `dst` (batch if present).
  function moveBackAll(dst: string) {
    return (ids: string[]) => async () => {
      if (client.value?.moveThreads) await client.value.moveThreads(ids, dst)
      else if (client.value?.moveThread)
        for (const id of ids) await client.value.moveThread(id, dst)
    }
  }
  async function archiveSelection() {
    const origin = activeMailbox.value
    await runSelectionMoveOut(
      'archived',
      client.value?.archiveThreads && ((ids) => client.value!.archiveThreads!(ids)),
      (id) => client.value!.archiveThread(id),
      'Archived',
      moveBackAll(origin),
    )
  }
  async function deleteSelection() {
    if (!client.value?.deleteThread && !client.value?.deleteThreads) {
      status.value = 'delete not supported'
      return
    }
    const origin = activeMailbox.value
    await runSelectionMoveOut(
      'deleted',
      client.value?.deleteThreads && ((ids) => client.value!.deleteThreads!(ids)),
      (id) => client.value!.deleteThread!(id),
      'Deleted',
      moveBackAll(origin),
    )
  }
  async function snoozeSelection(until?: string) {
    const undo = client.value?.unsnooze
      ? (ids: string[]) => async () => {
          for (const id of ids) await client.value!.unsnooze!(id)
        }
      : undefined
    await runSelectionMoveOut(
      'snoozed',
      client.value?.snoozeThreads && ((ids) => client.value!.snoozeThreads!(ids, until)),
      (id) => client.value!.snoozeThread(id, until),
      undo ? 'Snoozed' : undefined,
      undo,
    )
  }
  async function moveSelectionTo(mailboxId: string) {
    if (!client.value?.moveThread && !client.value?.moveThreads) {
      status.value = 'move not supported'
      return
    }
    const origin = activeMailbox.value
    await runSelectionMoveOut(
      `moved → ${mailboxId}`,
      client.value?.moveThreads && ((ids) => client.value!.moveThreads!(ids, mailboxId)),
      (id) => client.value!.moveThread!(id, mailboxId),
      'Moved',
      moveBackAll(origin),
    )
  }
  // star/read don't remove rows — patch in place, then call the batch method.
  async function starSelection() {
    const rows = selectionConversations()
    if (!client.value || !rows.length) return
    const on = !rows.every((conversation) => conversation.starred)
    const ids = rows.map((conversation) => conversation.id)
    for (const id of ids) patchListConversation(id, { starred: on })
    status.value = `${on ? 'starred' : 'unstarred'} ${ids.length}`
    try {
      await applyBatch(
        ids,
        client.value.starThreads && ((bids) => client.value!.starThreads!(bids, on)),
        (id) => client.value!.toggleStar(id, on),
      )
    } finally {
      exitVisual()
    }
  }
  async function toggleSelectionRead() {
    const rows = selectionConversations()
    if (!client.value || !rows.length) return
    // all selected rows read → mark unread; else mark read.
    const read = rows.some((conversation) => conversation.unread)
    const ids = rows.map((conversation) => conversation.id)
    for (const id of ids) {
      const wasUnread = findRow(id)?.unread
      patchListConversation(id, { unread: !read })
      if (wasUnread !== !read) bumpMailboxUnread(activeMailbox.value, read ? -1 : 1)
    }
    status.value = `marked ${ids.length} ${read ? 'read' : 'unread'}`
    try {
      await applyBatch(
        ids,
        client.value.markThreadsRead && ((bids) => client.value!.markThreadsRead!(bids, read)),
        (id) => client.value!.markThreadRead(id, read),
      )
    } finally {
      exitVisual()
    }
  }
  async function createLabelAndLabelSelection(name: string) {
    if (!client.value?.createLabel) {
      status.value = 'labels not supported'
      return
    }
    const label = await client.value.createLabel(name)
    labels.value = await client.value.listLabels()
    await labelSelection(label.id)
  }
  async function createFolderAndMoveSelection(name: string) {
    if (!client.value?.createMailbox) {
      status.value = 'folders not supported'
      return
    }
    const created = await client.value.createMailbox(name.trim())
    mailboxes.value = await client.value.listMailboxes()
    await moveSelectionTo(created.id)
  }
  async function labelSelection(labelId: string) {
    const ids = [...selectedIds.value]
    if (!client.value || !ids.length) return
    for (const id of ids) {
      const row = findRow(id)
      if (row && !row.labelIds.includes(labelId)) row.labelIds = [...row.labelIds, labelId]
    }
    status.value = `labelled ${ids.length}`
    try {
      // applyLabel may be unsupported even without a batch method — no-op then.
      await applyBatch(
        ids,
        client.value.labelThreads && ((bids) => client.value!.labelThreads!(bids, labelId)),
        client.value.applyLabel ? (id) => client.value!.applyLabel!(id, labelId) : async () => {},
      )
    } finally {
      exitVisual()
      await reloadList()
    }
  }

  return {
    enterVisual,
    exitVisual,
    toggleSelect,
    toggleSelectAll,
    archiveSelection,
    deleteSelection,
    snoozeSelection,
    moveSelectionTo,
    starSelection,
    toggleSelectionRead,
    labelSelection,
    createLabelAndLabelSelection,
    createFolderAndMoveSelection,
  }
}
