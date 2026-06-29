// transient app-level toast (download results, attachment errors). one at a
// time: a new toast replaces the current one and resets the dismiss timer.
import { ref } from 'vue'

export type ShellToast = {
  id: number
  kind: 'success' | 'info' | 'error'
  title: string
  detail?: string
}

export function useShellToast() {
  const toast = ref<ShellToast | null>(null)
  let toastTimer: number | undefined
  function showToast(next: Omit<ShellToast, 'id'>, timeout = 3200) {
    if (toastTimer) window.clearTimeout(toastTimer)
    toast.value = { ...next, id: Date.now() }
    toastTimer = window.setTimeout(() => {
      toast.value = null
      toastTimer = undefined
    }, timeout)
  }
  function clearToast() {
    if (toastTimer) window.clearTimeout(toastTimer)
    toastTimer = undefined
    toast.value = null
  }
  return { toast, showToast, clearToast }
}
