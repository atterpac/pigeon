// In-thread find (`/` while the thread pane is focused). A conversation mixes
// two kinds of message body:
//   - HTML bodies render in sandboxed iframes the parent can't read, so the
//     match/highlight happens *inside* the iframe (see renderEmailHtml) and we
//     coordinate over postMessage.
//   - Plain-text bodies render as ordinary app DOM (.email-text-body), which the
//     parent can search and highlight directly.
// This composable unifies both into one ordered list of "units" with a single
// global match cursor, and scrolls the outer thread region to each active match.
//
// Singleton (like useMailShell) so the command line, reading pane, and keyboard
// layer all share one find session.
import { ref } from 'vue'

interface FrameUnit {
  kind: 'frame'
  win: Window | null
  el: HTMLIFrameElement
  count: number
  tops: number[]
}
interface DomUnit {
  kind: 'dom'
  el: HTMLElement
  marks: HTMLElement[]
}
type Unit = FrameUnit | DomUnit

const UNIT_SELECTOR = 'iframe.email-html-frame, .email-text-body'

function createThreadFind() {
  const active = ref(false)
  const query = ref('')
  const total = ref(0)
  // 1-based index of the active match for display; 0 when there are none.
  const current = ref(0)

  let scrollEl: HTMLElement | null = null
  let units: Unit[] = []
  let pending = 0
  let listening = false
  let knownFrames = 0

  function register(el: HTMLElement | null) {
    scrollEl = el
  }

  function collectFrames(): HTMLIFrameElement[] {
    if (!scrollEl) return []
    return Array.from(scrollEl.querySelectorAll<HTMLIFrameElement>('iframe.email-html-frame'))
  }

  function ensureListener() {
    if (listening) return
    listening = true
    window.addEventListener('message', onMessage)
  }

  function onMessage(event: MessageEvent) {
    const type = event.data?.type
    // An iframe finished (re)rendering. While find is active, a newly-expanded
    // message means there's more to search — re-run, keeping the cursor put.
    if (type === 'email-frame-height') {
      if (active.value && query.value.trim() && collectFrames().length !== knownFrames) {
        run(query.value, false)
      }
      return
    }
    if (type !== 'email-find-result') return
    const unit = units.find((u): u is FrameUnit => u.kind === 'frame' && u.win === event.source)
    if (!unit) return
    unit.count = Number(event.data.count) || 0
    unit.tops = Array.isArray(event.data.tops) ? event.data.tops.map(Number) : []
    pending = Math.max(0, pending - 1)
    if (pending === 0) finalize()
  }

  function unitCount(unit: Unit): number {
    return unit.kind === 'frame' ? unit.count : unit.marks.length
  }

  function finalize() {
    total.value = units.reduce((sum, u) => sum + unitCount(u), 0)
    if (total.value === 0) { current.value = 0; return }
    current.value = Math.min(Math.max(current.value, 1), total.value)
    activate(current.value)
  }

  // ── Direct DOM highlighting (plain-text message bodies) ──────────────────
  function highlightDom(root: HTMLElement, q: string): HTMLElement[] {
    const marks: HTMLElement[] = []
    const needle = q.toLowerCase()
    if (!needle) return marks
    const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT, {
      acceptNode(node) {
        if (!node.nodeValue || node.nodeValue.toLowerCase().indexOf(needle) === -1) return NodeFilter.FILTER_SKIP
        const tag = node.parentNode?.nodeName
        if (tag === 'SCRIPT' || tag === 'STYLE' || tag === 'MARK') return NodeFilter.FILTER_REJECT
        return NodeFilter.FILTER_ACCEPT
      },
    })
    const targets: Text[] = []
    let n: Node | null
    while ((n = walker.nextNode())) targets.push(n as Text)
    for (const node of targets) {
      const text = node.nodeValue ?? ''
      const lower = text.toLowerCase()
      const frag = document.createDocumentFragment()
      let last = 0
      let pos = lower.indexOf(needle, last)
      while (pos !== -1) {
        if (pos > last) frag.appendChild(document.createTextNode(text.slice(last, pos)))
        const mark = document.createElement('mark')
        mark.className = 'ef-find'
        mark.textContent = text.slice(pos, pos + needle.length)
        frag.appendChild(mark)
        marks.push(mark)
        last = pos + needle.length
        pos = lower.indexOf(needle, last)
      }
      if (last < text.length) frag.appendChild(document.createTextNode(text.slice(last)))
      node.parentNode?.replaceChild(frag, node)
    }
    return marks
  }
  function clearDom(unit: DomUnit) {
    for (const mark of unit.marks) {
      if (mark.parentNode) mark.parentNode.replaceChild(document.createTextNode(mark.textContent ?? ''), mark)
    }
    unit.el.normalize()
    unit.marks = []
  }

  function clearAll() {
    for (const unit of units) {
      if (unit.kind === 'frame') unit.win?.postMessage({ type: 'email-find-clear' }, '*')
      else clearDom(unit)
    }
  }

  function open() {
    active.value = true
    query.value = ''
    total.value = 0
    current.value = 0
    knownFrames = 0
    ensureListener()
  }

  function close() {
    active.value = false
    query.value = ''
    total.value = 0
    current.value = 0
    clearAll()
    units = []
    knownFrames = 0
  }

  // resetCursor=false re-runs the same query (e.g. after a new iframe loads)
  // without snapping the active match back to the first result.
  function run(q: string, resetCursor = true) {
    query.value = q
    ensureListener()
    clearAll()
    const els = scrollEl ? Array.from(scrollEl.querySelectorAll<HTMLElement>(UNIT_SELECTOR)) : []
    units = els.map((el): Unit => el.tagName === 'IFRAME'
      ? { kind: 'frame', win: (el as HTMLIFrameElement).contentWindow, el: el as HTMLIFrameElement, count: 0, tops: [] }
      : { kind: 'dom', el, marks: [] })
    knownFrames = collectFrames().length

    if (!q.trim()) {
      total.value = 0
      current.value = 0
      return
    }
    if (resetCursor || current.value === 0) current.value = 1

    // DOM units highlight synchronously; iframe units reply asynchronously.
    pending = 0
    for (const unit of units) {
      if (unit.kind === 'frame') {
        pending += 1
        unit.win?.postMessage({ type: 'email-find', query: q }, '*')
      } else {
        unit.marks = highlightDom(unit.el, q)
      }
    }
    if (pending === 0) finalize()
  }

  // Tell each unit which (if any) of its matches is the globally-active one and
  // scroll the outer region so it's in view.
  function activate(globalIndex: number) {
    let remaining = globalIndex - 1
    for (const unit of units) {
      const count = unitCount(unit)
      const localActive = remaining >= 0 && remaining < count ? remaining : -1
      if (unit.kind === 'frame') {
        unit.win?.postMessage({ type: 'email-find-activate', index: localActive }, '*')
        if (localActive >= 0) scrollFrameMatch(unit, localActive)
      } else {
        unit.marks.forEach((m, i) => m.classList.toggle('ef-active', i === localActive))
        const activeMark = localActive >= 0 ? unit.marks[localActive] : undefined
        if (activeMark) scrollDomMatch(activeMark)
      }
      remaining -= count
    }
  }

  function scrollFrameMatch(unit: FrameUnit, localIndex: number) {
    if (!scrollEl) return
    const matchTop = unit.tops[localIndex] ?? 0
    const frameRect = unit.el.getBoundingClientRect()
    const scrollRect = scrollEl.getBoundingClientRect()
    const target = scrollEl.scrollTop + (frameRect.top - scrollRect.top) + matchTop - 100
    scrollEl.scrollTo({ top: Math.max(0, target), behavior: 'auto' })
  }
  function scrollDomMatch(mark: HTMLElement) {
    if (!scrollEl) return
    const markRect = mark.getBoundingClientRect()
    const scrollRect = scrollEl.getBoundingClientRect()
    const target = scrollEl.scrollTop + (markRect.top - scrollRect.top) - 100
    scrollEl.scrollTo({ top: Math.max(0, target), behavior: 'auto' })
  }

  function next() {
    if (total.value === 0) return
    current.value = current.value >= total.value ? 1 : current.value + 1
    activate(current.value)
  }

  function prev() {
    if (total.value === 0) return
    current.value = current.value <= 1 ? total.value : current.value - 1
    activate(current.value)
  }

  return { active, query, total, current, register, open, close, run, next, prev }
}

export type ThreadFindApi = ReturnType<typeof createThreadFind>

let instance: ThreadFindApi | null = null
export function useThreadFind(): ThreadFindApi {
  if (!instance) instance = createThreadFind()
  return instance
}
