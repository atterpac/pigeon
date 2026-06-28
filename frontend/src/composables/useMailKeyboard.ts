import type { Ref } from 'vue'
import { onMounted, onUnmounted } from 'vue'
import type ReadingPane from '../components/shell/ReadingPane.vue'
import type { MailShellApi } from './useMailShell'

type MobilePane = 'list' | 'thread'

interface KeyboardOptions {
  shell: MailShellApi
  readingPane: Ref<InstanceType<typeof ReadingPane> | null>
  mobilePane: Ref<MobilePane>
  cheatsheetOpen: Ref<boolean>
  settingsOpen: Ref<boolean>
}

export function useMailKeyboard({ shell: s, readingPane, mobilePane, cheatsheetOpen, settingsOpen }: KeyboardOptions) {
  let countBuffer = ''
  let gPending = false
  let dPending = false

  function resetPending() {
    countBuffer = ''
    gPending = false
    dPending = false
  }

  // WebKitGTK (Wails/Linux) frequently swallows auto-repeat keydown events, so
  // holding j/k does nothing past the first press. Drive our own repeat: the
  // first press acts immediately, then we tick on a timer until keyup.
  const REPEAT_DELAY = 240
  const REPEAT_INTERVAL = 45
  let heldNav: 'j' | 'k' | null = null
  let repeatDelayTimer: ReturnType<typeof setTimeout> | null = null
  let repeatTickTimer: ReturnType<typeof setInterval> | null = null

  function navScroll(dir: 'j' | 'k', count: number) {
    if (s.focusPane.value === 'thread') readingPane.value?.scrollThread(dir === 'j' ? count : -count)
    else s.moveSelection(dir === 'j' ? count : -count)
  }

  function stopNavRepeat() {
    heldNav = null
    if (repeatDelayTimer) { clearTimeout(repeatDelayTimer); repeatDelayTimer = null }
    if (repeatTickTimer) { clearInterval(repeatTickTimer); repeatTickTimer = null }
  }

  function startNavRepeat(dir: 'j' | 'k') {
    if (heldNav === dir) return
    stopNavRepeat()
    heldNav = dir
    repeatDelayTimer = setTimeout(() => {
      repeatTickTimer = setInterval(() => navScroll(dir, 1), REPEAT_INTERVAL)
    }, REPEAT_DELAY)
  }

  function handleGlobalKeyup(event: KeyboardEvent) {
    const k = event.key
    if (heldNav && (k === heldNav || k === 'ArrowDown' || k === 'ArrowUp')) stopNavRepeat()
  }

  function handleGlobalKeydown(event: KeyboardEvent) {
    if (event.defaultPrevented) return
    const target = event.target as HTMLElement | null
    const inField = !!target && ['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName)

    if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') {
      event.preventDefault()
      s.openCommand('search')
      return
    }
    if ((event.metaKey || event.ctrlKey) && event.key === 'Enter') {
      event.preventDefault()
      void s.sendDraft()
      return
    }
    if (inField) return

    // While the which-key command menu is open it owns the keyboard.
    if (s.commandMenuOpen.value) return

    const key = event.key

    // Visual (multi-select) mode owns a handful of keys; movement (j/k/g/G and
    // counts) falls through to the shared navigation below, which sweep-extends
    // the selection via the visual-aware navScroll.
    if (s.visualMode.value) {
      if (key === 'v') { s.exitVisual(); resetPending(); return }
      if (key === ' ') { event.preventDefault(); s.toggleSelect(); resetPending(); return }
      if (key === 'V') { s.toggleSelectAll(); resetPending(); return }
      if (key === 'Enter') { event.preventDefault(); s.openCommandMenu(); resetPending(); return }
      if (key === 'e') { void s.archiveSelection(); resetPending(); return }
      if (key === '#') { void s.deleteSelection(); resetPending(); return }
      if (key === 's') { void s.snoozeSelection(); resetPending(); return }
      if (key === '*') { void s.starSelection(); resetPending(); return }
      if (key === 'u') { void s.toggleSelectionRead(); resetPending(); return }
    }

    // Space = leader: open the command menu on the selected/open thread.
    if (key === ' ' && (s.selectedThread.value || s.selectedConversation.value)) {
      event.preventDefault()
      s.openCommandMenu()
      resetPending()
      return
    }
    if (key === '?') {
      event.preventDefault()
      cheatsheetOpen.value = !cheatsheetOpen.value
      return
    }
    if (key === 'Escape') {
      if (settingsOpen.value) settingsOpen.value = false
      else if (cheatsheetOpen.value) cheatsheetOpen.value = false
      else onEscape()
      resetPending()
      return
    }
    if ((key === 'h' || key === 'Backspace') && (s.focusPane.value === 'thread' || s.selectedThread.value)) {
      event.preventDefault()
      onEscape()
      resetPending()
      return
    }

    if (/[0-9]/.test(key) && !(key === '0' && !countBuffer)) {
      countBuffer += key
      return
    }
    const count = parseInt(countBuffer || '1', 10)

    if (key === 'g') {
      if (gPending) {
        if (s.focusPane.value === 'thread') readingPane.value?.scrollThread('top')
        else s.selectFirst()
        resetPending()
      } else {
        gPending = true
      }
      return
    }
    gPending = false

    if (key === 'd') {
      if (dPending) {
        void s.archiveSelected()
        resetPending()
      } else {
        dPending = true
      }
      return
    }
    dPending = false

    if (key === 'J' && s.focusPane.value === 'thread') {
      s.focusAdjacentMessage(count)
    } else if (key === 'K' && s.focusPane.value === 'thread') {
      s.focusAdjacentMessage(-count)
    } else if (key === 'G') {
      if (s.focusPane.value === 'thread') readingPane.value?.scrollThread('bottom')
      else s.selectLast()
    } else if (key === '/') {
      event.preventDefault()
      // In the reading pane, `/` finds within the open thread; elsewhere it runs
      // a mailbox-wide search.
      if (s.focusPane.value === 'thread' && s.selectedThread.value) s.openFind()
      else s.openCommand('search')
    } else if (key === ':') {
      event.preventDefault()
      s.openCommand('ex')
    } else if (key === 'c') {
      s.compose()
    } else if (key === 'v' && !s.selectedThread.value) {
      s.enterVisual()
    } else if (key === 'j' || key === 'ArrowDown') {
      if (event.repeat) return
      navScroll('j', count)
      if (countBuffer === '') startNavRepeat('j')
    } else if (key === 'k' || key === 'ArrowUp') {
      if (event.repeat) return
      navScroll('k', count)
      if (countBuffer === '') startNavRepeat('k')
    } else if (key === 'Tab' && s.selectedThread.value) {
      event.preventDefault()
      if (s.focusPane.value === 'thread') s.focusList()
      else s.focusThread()
    } else if (key === 'Enter') {
      void s.openThread()
      mobilePane.value = 'thread'
    } else if (key === 'e') {
      void s.archiveSelected()
    } else if (key === 's') {
      void s.snoozeThread()
    } else if (key === '*') {
      void s.toggleStar()
    } else if (key === 'u') {
      if (s.snoozedActive.value) void s.unsnoozeThread()
      else void s.toggleRead()
    } else if (key === 'U') {
      void s.performUndo()
    } else if (key === '#') {
      void s.deleteThread()
    } else if (key === '!') {
      void s.reportSpam()
    } else if (s.selectedThread.value && key === 'r') {
      s.openReply('reply')
      readingPane.value?.focusReply()
    } else if (s.selectedThread.value && key === 'a') {
      s.openReply('replyAll')
      readingPane.value?.focusReply()
    } else if (s.selectedThread.value && key === 'f') {
      s.openReply('forward')
      readingPane.value?.focusReply()
    }
    resetPending()
  }

  function onEscape() {
    if (s.composeOpen.value) {
      void s.discardDraft()
      return
    }
    if (s.visualMode.value) {
      s.exitVisual()
      return
    }
    if (s.command.value) {
      s.cancelCommand()
      return
    }
    if (s.searchActive.value) {
      s.closeSearch()
      return
    }
    if (s.snoozedActive.value && !s.selectedThread.value) {
      s.closeSnoozed()
      return
    }
    if (s.focusPane.value === 'thread' || s.selectedThread.value) {
      s.closeThread()
      mobilePane.value = 'list'
    }
  }

  onMounted(() => {
    window.addEventListener('keydown', handleGlobalKeydown)
    window.addEventListener('keyup', handleGlobalKeyup)
    window.addEventListener('blur', stopNavRepeat)
  })
  onUnmounted(() => {
    window.removeEventListener('keydown', handleGlobalKeydown)
    window.removeEventListener('keyup', handleGlobalKeyup)
    window.removeEventListener('blur', stopNavRepeat)
    stopNavRepeat()
  })
}
