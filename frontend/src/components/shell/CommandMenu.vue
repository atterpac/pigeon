<script setup lang="ts">
// Which-key command menu (Grid layout): assign the selected/open thread to
// archive · snooze · label · move-to-folder. Leader chords at root
// (e/s/l/m/u), then number/arrow/type-to-filter at level 2, with
// type-to-create for labels and folders. Own keydown listener while mounted.
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import {
  PhArchive, PhBellSlash, PhCalendarBlank, PhClock, PhClockClockwise, PhCoffee, PhFolderSimple, PhPlus, PhProhibit, PhSunHorizon, PhTag, PhTrash,
} from '@phosphor-icons/vue'
import { useMailShell } from '../../composables/useMailShell'

const s = useMailShell()
const emit = defineEmits<{ (e: 'close'): void }>()

type Kind = 'leaf' | 'branch' | 'create'
type Mode = 'root' | 'snooze' | 'label' | 'move'
type Item = {
  id: string; label: string; kind: Kind; key?: string; hint?: string
  icon: any; swatch?: string; mode?: Mode; run?: () => void | Promise<void>; createKind?: 'label' | 'folder'
}

const target = computed(() => s.selectedThread.value ?? s.selectedConversation.value)
// In Visual mode the menu acts over the whole selection instead of one thread.
const batch = computed(() => s.visualMode.value && s.selectedCount.value > 0)

// ---- snooze presets (computed against real clock) ----
function at(base: Date, h: number, m = 0) { const d = new Date(base); d.setHours(h, m, 0, 0); return d }
function fmt(d: Date) { return d.toLocaleString(undefined, { weekday: 'short', hour: 'numeric', minute: '2-digit' }) }
const snoozeItems = computed<Item[]>(() => {
  const now = new Date()
  const later = at(now, 18)
  const tomorrow = at(new Date(now.getTime() + 864e5), 8)
  const sat = new Date(now); sat.setDate(now.getDate() + ((6 - now.getDay() + 7) % 7 || 7)); const weekend = at(sat, 9)
  const mon = new Date(now); mon.setDate(now.getDate() + ((8 - now.getDay()) % 7 || 7)); const nextWeek = at(mon, 8)
  const mk = (id: string, label: string, key: string, icon: any, when: Date): Item => ({
    id, label, key, icon, kind: 'leaf', hint: fmt(when),
    run: () => batch.value ? s.snoozeSelection(when.toISOString()) : s.snoozeThread(when.toISOString()),
  })
  return [
    mk('later', 'Later today', '1', PhCoffee, later),
    mk('tomorrow', 'Tomorrow', '2', PhSunHorizon, tomorrow),
    mk('weekend', 'This weekend', '3', PhClock, weekend),
    mk('nextweek', 'Next week', '4', PhCalendarBlank, nextWeek),
  ]
})

const verbs = computed<Item[]>(() => [
  ...(s.snoozedActive.value && !batch.value ? [{ id: 'unsnooze', label: 'Unsnooze now', kind: 'leaf' as Kind, key: 'w', icon: PhClockClockwise, run: () => s.unsnoozeThread() }] : []),
  { id: 'archive', label: 'Archive', kind: 'leaf', key: 'e', icon: PhArchive, run: () => batch.value ? s.archiveSelection() : s.archiveSelected() },
  { id: 'snooze', label: 'Snooze', kind: 'branch', key: 's', icon: PhClock, mode: 'snooze' },
  { id: 'label', label: 'Add label', kind: 'branch', key: 'l', icon: PhTag, mode: 'label' },
  { id: 'move', label: 'Move to folder', kind: 'branch', key: 'm', icon: PhFolderSimple, mode: 'move' },
  { id: 'mute', label: batch.value ? 'Toggle read' : 'Mute thread', kind: 'leaf', key: 'u', icon: PhBellSlash, run: () => batch.value ? s.toggleSelectionRead() : s.toggleRead() },
  { id: 'delete', label: 'Delete', kind: 'leaf', key: '#', icon: PhTrash, run: () => batch.value ? s.deleteSelection() : s.deleteThread() },
  ...(batch.value ? [] : [{ id: 'spam', label: 'Report spam', kind: 'leaf' as Kind, key: '!', icon: PhProhibit, run: () => s.reportSpam() }]),
])

const labelItems = computed<Item[]>(() =>
  s.labels.value.map((l, i): Item => ({
    id: `label-${l.id}`, label: l.name, kind: 'leaf', key: i < 9 ? String(i + 1) : undefined,
    icon: PhTag, swatch: l.swatch, run: () => batch.value ? s.labelSelection(l.id) : s.applyLabel(l.id),
  })))

// Move targets: every mailbox except the one we're already in and draft/sent.
const folderItems = computed<Item[]>(() =>
  s.mailboxes.value
    .filter((m) => m.id !== s.activeMailbox.value && m.role !== 'drafts' && m.role !== 'sent')
    .map((m, i): Item => ({
      id: `move-${m.id}`, label: m.name, kind: 'leaf', key: i < 9 ? String(i + 1) : undefined,
      icon: PhFolderSimple, run: () => batch.value ? s.moveSelectionTo(m.id) : s.moveThreadTo(m.id),
    })))

// ---- runtime state ----
const mode = ref<Mode>('root')
const query = ref('')
const cursor = ref(0)
const rootSearching = ref(false)

const crumb = computed(() => ({ root: 'Action', snooze: 'Snooze', label: 'Add label', move: 'Move to folder' }[mode.value]))
const searching = computed(() => rootSearching.value || mode.value !== 'root')

function listFor(m: Mode): Item[] {
  if (m === 'snooze') return snoozeItems.value
  if (m === 'label') return labelItems.value
  if (m === 'move') return folderItems.value
  return verbs.value
}
function subseq(text: string, q: string): boolean {
  if (!q) return true
  const t = text.toLowerCase()
  let i = 0
  for (const ch of q.toLowerCase()) { i = t.indexOf(ch, i); if (i < 0) return false; i++ }
  return true
}
function createRows(): Item[] {
  const name = query.value.trim()
  if (mode.value === 'label') return [{ id: 'cl', label: `Create label “${name}”`, kind: 'create', createKind: 'label', icon: PhPlus, swatch: 'var(--accent)', run: () => batch.value ? s.createLabelAndLabelSelection(name) : s.createLabelAndApply(name) }]
  if (mode.value === 'move') return [{ id: 'cf', label: `Create folder “${name}”`, kind: 'create', createKind: 'folder', icon: PhPlus, run: () => batch.value ? s.createFolderAndMoveSelection(name) : s.createFolderAndMove(name) }]
  return []
}
const visible = computed<Item[]>(() => {
  const base = listFor(mode.value).filter((it) => subseq(it.label, query.value))
  if (!base.length && query.value.trim()) return createRows()
  return base
})
watch(visible, () => { if (cursor.value >= visible.value.length) cursor.value = Math.max(0, visible.value.length - 1) })

function close() { emit('close') }
function activate(it: Item) {
  if (it.kind === 'branch' && it.mode) { mode.value = it.mode; query.value = ''; cursor.value = 0; rootSearching.value = false; return }
  void it.run?.()
  close()
}
function moveCursor(d: number) { const n = visible.value.length; if (n) cursor.value = (cursor.value + d + n) % n }

function onKey(e: KeyboardEvent) {
  const k = e.key
  const printable = k.length === 1 && !e.metaKey && !e.ctrlKey && !e.altKey
  const chordRoot = mode.value === 'root' && !rootSearching.value
  if (k === 'ArrowDown' || (e.ctrlKey && k === 'n') || (k === 'Tab' && !e.shiftKey)) { e.preventDefault(); moveCursor(1); return }
  if (k === 'ArrowUp' || (e.ctrlKey && k === 'p') || (k === 'Tab' && e.shiftKey)) { e.preventDefault(); moveCursor(-1); return }
  if (k === 'Enter') { e.preventDefault(); const it = visible.value[cursor.value]; if (it) activate(it); return }
  if (k === 'Escape') {
    e.preventDefault()
    if (mode.value !== 'root' || rootSearching.value || query.value) { mode.value = 'root'; query.value = ''; cursor.value = 0; rootSearching.value = false }
    else close()
    return
  }
  if (k === 'Backspace') {
    e.preventDefault()
    if (query.value) query.value = query.value.slice(0, -1)
    else if (mode.value !== 'root') { mode.value = 'root'; rootSearching.value = false }
    else rootSearching.value = false
    return
  }
  if (chordRoot) {
    if (k === '/') { e.preventDefault(); rootSearching.value = true; return }
    if (printable) {
      const verb = verbs.value.find((v) => v.key === k.toLowerCase())
      if (verb) { e.preventDefault(); activate(verb); return }
      e.preventDefault(); rootSearching.value = true; query.value += k
    }
    return
  }
  if (printable && /[1-9]/.test(k) && mode.value !== 'root' && !query.value) {
    const byKey = listFor(mode.value).find((it) => it.key === k)
    if (byKey) { e.preventDefault(); activate(byKey); return }
  }
  if (printable) { e.preventDefault(); query.value += k }
}
// Capture phase so the menu wins over the global mail keymap while open.
onMounted(() => window.addEventListener('keydown', onKey, true))
onBeforeUnmount(() => window.removeEventListener('keydown', onKey, true))
</script>

<template>
  <div class="cmdmenu-backdrop" @click.self="close()">
    <div class="cmdmenu" role="dialog" aria-modal="true" aria-label="Command menu">
      <div class="cm-head">
        <span class="kbadge">{{ mode === 'root' ? 'leader' : mode }}</span>
        <span class="cm-crumb">{{ crumb }}</span>
        <span v-if="batch" class="cm-target">{{ s.selectedCount.value }} selected</span>
        <span v-else-if="target" class="cm-target">{{ target.from.name }} · {{ target.subject }}</span>
        <span v-if="searching" class="cm-query">{{ query }}<i class="caret" /></span>
      </div>
      <div class="cm-cells">
        <button
          v-for="(it, i) in visible"
          :key="it.id"
          type="button"
          class="cm-cell"
          :class="{ active: cursor === i, create: it.kind === 'create' }"
          @mouseenter="cursor = i"
          @click="activate(it)"
        >
          <kbd>{{ it.key ?? '↵' }}</kbd>
          <span class="ri" :style="it.swatch ? { color: it.swatch } : {}"><component :is="it.icon" :size="15" /></span>
          <span class="rl">{{ it.label }}</span>
          <span v-if="it.hint" class="rh">{{ it.hint }}</span>
          <span v-else-if="it.kind === 'branch'" class="rh">›</span>
        </button>
        <p v-if="!visible.length" class="cm-empty">No matches — type a name to create one</p>
      </div>
      <footer class="cm-foot">
        <span><kbd>{{ mode === 'root' ? 'e s l m u # !' : '1–9' }}</kbd> direct</span>
        <span><kbd>/</kbd> search</span>
        <span><kbd>↑↓</kbd> move</span>
        <span><kbd>⌫</kbd> back</span>
        <span><kbd>esc</kbd> close</span>
      </footer>
    </div>
  </div>
</template>

<style scoped>
.cmdmenu-backdrop{position:fixed;inset:0;z-index:80;display:flex;align-items:flex-end;justify-content:center;padding:0 16px 26px;background:rgba(8,8,14,.5);backdrop-filter:blur(2px)}
.cmdmenu{width:min(680px,100%);border:1px solid var(--border-2);border-radius:14px;background:color-mix(in oklab,var(--surface) 96%,transparent);box-shadow:var(--shadow-2),var(--top-hi);overflow:hidden}
.cm-head{display:flex;align-items:center;gap:11px;padding:11px 15px;border-bottom:1px solid var(--border);background:var(--surface-2)}
.kbadge{flex:none;padding:3px 9px;border-radius:6px;background:var(--accent);color:var(--accent-ink);font:700 11px "JetBrains Mono",ui-monospace,monospace;text-transform:uppercase}
.cm-crumb{flex:none;color:var(--text-dim);font-size:13px}
.cm-target{flex:1;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;color:var(--text-mut);font:11px "JetBrains Mono",ui-monospace,monospace}
.cm-query{flex:none;color:var(--text);font:13px "JetBrains Mono",ui-monospace,monospace}
.caret{display:inline-block;width:1.5px;height:1.05em;margin-left:1px;background:var(--accent);vertical-align:text-bottom;animation:cm-blink 1.1s step-end infinite}
@keyframes cm-blink{50%{opacity:0}}
.cm-cells{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:3px;padding:8px}
.cm-cell{display:flex;align-items:center;gap:10px;border:0;border-radius:9px;background:transparent;color:var(--text-dim);padding:9px 10px;text-align:left;cursor:pointer;font:inherit}
.cm-cell kbd{flex:none;min-width:22px;display:inline-grid;place-items:center;height:18px;padding:0 5px;border:1px solid var(--border-2);border-radius:5px;background:var(--surface-3,var(--surface-2));color:var(--accent);font:10px "JetBrains Mono",ui-monospace,monospace}
.cm-cell .ri{flex:none;display:grid;place-items:center;width:22px;height:22px;color:var(--text-dim)}
.cm-cell .rl{flex:1;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;color:var(--text)}
.cm-cell .rh{flex:none;color:var(--text-mut);font:11px "JetBrains Mono",ui-monospace,monospace}
.cm-cell.active{background:var(--accent-soft);box-shadow:inset 0 0 0 1px var(--accent-line)}
.cm-cell.create .rl,.cm-cell.create kbd,.cm-cell.create .ri{color:var(--accent)}
.cm-empty{grid-column:1/-1;margin:0;padding:14px 10px;color:var(--text-mut);font-size:12.5px;text-align:center}
.cm-foot{display:flex;flex-wrap:wrap;gap:16px;padding:9px 16px;border-top:1px solid var(--border);background:var(--surface-2)}
.cm-foot span{display:flex;align-items:center;gap:6px;color:var(--text-mut);font-size:11px}
.cm-foot kbd{display:inline-grid;place-items:center;min-width:18px;height:18px;padding:0 5px;border:1px solid var(--border-2);border-radius:5px;background:var(--surface-2);color:var(--text-mut);font:10px "JetBrains Mono",ui-monospace,monospace}
@media (max-width:640px){.cm-cells{grid-template-columns:1fr}.cm-target{display:none}}
</style>
