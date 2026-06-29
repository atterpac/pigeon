// Undo ledger behind the toast + `U`. Same-verb triage actions collapse into one
// entry (one undo reverses the whole run). `kind:'send'` holds a pending outbox
// send (undo cancels it); else an inverse mutation. Auto-expires after `ttl`.
import { ref, type Ref } from 'vue'
import { errorMessage } from '../mail/format'

export type LastAction = { label: string; kind: 'triage' | 'send'; undo: () => Promise<void> }

// status drives the modeline; reloadList reconciles the list after a triage undo
// lands server-side (send undos don't touch the list).
export function useUndo(status: Ref<string>, reloadList: () => Promise<void>) {
  const lastAction = ref<LastAction | null>(null)
  let undoTimer: number | undefined
  let undoChain: Array<() => Promise<void>> = []
  let undoVerb = ''
  let undoCount = 0

  // register the inverse of the action just taken; toast + `U` invoke it.
  // `count` = conversations the action covered (batch ops pass >1).
  function recordUndo(
    verb: string,
    undo: () => Promise<void>,
    kind: 'triage' | 'send' = 'triage',
    count = 1,
    ttl = 8000,
  ) {
    if (kind === 'triage' && undoVerb === verb && lastAction.value?.kind === 'triage') {
      undoChain.push(undo)
      undoCount += count
    } else {
      undoChain = [undo]
      undoVerb = verb
      undoCount = count
    }
    const chain = undoChain.slice()
    const label = kind === 'triage' && undoCount > 1 ? `${verb} ${undoCount}` : verb
    // reverse so the most recent action undoes first.
    lastAction.value = {
      label,
      kind,
      undo: async () => {
        for (const fn of chain.slice().reverse()) await fn()
      },
    }
    if (undoTimer) window.clearTimeout(undoTimer)
    undoTimer = window.setTimeout(clearUndo, ttl)
  }
  function clearUndo() {
    lastAction.value = null
    undoChain = []
    undoVerb = ''
    undoCount = 0
    if (undoTimer) window.clearTimeout(undoTimer)
  }
  async function performUndo() {
    const action = lastAction.value
    if (!action) return
    clearUndo()
    status.value = 'undoing…'
    try {
      await action.undo()
      status.value = 'undone'
    } catch (error) {
      status.value = `undo failed: ${errorMessage(error)}`
    } finally {
      if (action.kind === 'triage') await reloadList()
    }
  }
  return { lastAction, recordUndo, clearUndo, performUndo }
}
