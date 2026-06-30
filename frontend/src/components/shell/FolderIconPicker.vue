<script setup lang="ts">
// Studio-style modal for assigning a Phosphor icon (+ weight + color) to a
// folder. Self-contained: owns its keyboard while open and emits the chosen
// preference back to the caller.
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import {
  FOLDER_ICONS,
  FOLDER_ICON_COLORS,
  FOLDER_ICON_WEIGHTS,
  folderIconComponent,
  type FolderEdit,
  type FolderIconDef,
  type FolderIconPref,
  type FolderIconWeight,
} from '../../mail/folderIcons'
import { PhCheck, PhFolderSimple, PhMagnifyingGlass, PhX } from '@phosphor-icons/vue'

const props = defineProps<{ folderName: string; initial?: FolderIconPref | null; canRename?: boolean }>()
const emit = defineEmits<{ (e: 'assign', result: FolderEdit): void; (e: 'close'): void }>()

const COLS = 7

const name = ref(props.folderName)
const query = ref('')
const cursor = ref(0)
const weight = ref<FolderIconWeight>(props.initial?.weight ?? 'regular')
const color = ref(props.initial?.color ?? '')

const visible = computed<FolderIconDef[]>(() => {
  const q = query.value.trim().toLowerCase()
  if (!q) return FOLDER_ICONS
  return FOLDER_ICONS.filter((it) => it.name.toLowerCase().includes(q) || it.id.includes(q))
})

// Start the cursor on the folder's current icon when re-editing.
const initialIndex = FOLDER_ICONS.findIndex((d) => d.id === props.initial?.icon)
if (initialIndex >= 0) cursor.value = initialIndex

const current = computed(() => visible.value[cursor.value] ?? null)
const CurIcon = computed(() => current.value?.icon ?? PhFolderSimple)
const previewColor = computed(() => color.value || 'var(--accent)')

function clampCursor() { if (cursor.value >= visible.value.length) cursor.value = Math.max(0, visible.value.length - 1) }
function move(d: number) { const n = visible.value.length; if (n) cursor.value = (cursor.value + d + n) % n }

function assign(it: FolderIconDef | null) {
  if (!it) return
  emit('assign', { name: name.value.trim() || props.folderName, icon: it.id, weight: weight.value, color: color.value })
}

function onKey(e: KeyboardEvent) {
  const k = e.key
  const printable = k.length === 1 && !e.metaKey && !e.ctrlKey && !e.altKey
  if (k === 'Escape') { e.preventDefault(); emit('close'); return }
  // When focus is in the name field, only intercept Enter (submit); let the
  // field handle typing/navigation itself.
  if (e.target instanceof HTMLInputElement && e.target.classList.contains('fip-name-input')) {
    if (k === 'Enter') { e.preventDefault(); assign(current.value) }
    return
  }
  if (k === 'Enter') { e.preventDefault(); assign(current.value); return }
  if (k === 'ArrowRight' || (k === 'Tab' && !e.shiftKey)) { e.preventDefault(); move(1); return }
  if (k === 'ArrowLeft' || (k === 'Tab' && e.shiftKey)) { e.preventDefault(); move(-1); return }
  if (k === 'ArrowDown') { e.preventDefault(); move(COLS); return }
  if (k === 'ArrowUp') { e.preventDefault(); move(-COLS); return }
  if (k === 'Backspace') { e.preventDefault(); query.value = query.value.slice(0, -1); clampCursor(); return }
  if (printable) { e.preventDefault(); query.value += k; cursor.value = 0 }
}

// Capture-phase so the global mail keymap stays inert while the picker is open.
onMounted(() => window.addEventListener('keydown', onKey, true))
onBeforeUnmount(() => window.removeEventListener('keydown', onKey, true))
</script>

<template>
  <div class="fip-scrim" @click="emit('close')" />
  <div class="fip" role="dialog" aria-label="Edit folder">
    <button class="fip-x" type="button" title="Close" @click="emit('close')"><PhX :size="14" /></button>
    <div class="fip-left">
      <div class="fip-glyph" :style="{ color: previewColor }"><component :is="CurIcon" :size="64" :weight="weight" /></div>
      <b class="fip-name">{{ name.trim() || current?.name || 'Folder' }}</b>
      <small class="fip-folder">{{ current?.name ?? 'Folder' }} icon</small>

      <div v-if="canRename !== false" class="fip-control">
        <p>Name</p>
        <input v-model="name" class="fip-name-input" placeholder="Folder name" @keydown.enter.prevent="assign(current)" />
      </div>

      <div class="fip-control">
        <p>Weight</p>
        <div class="fip-wt">
          <button v-for="w in FOLDER_ICON_WEIGHTS" :key="w" type="button" :class="{ on: weight === w }" :title="w" @click="weight = w">
            <component :is="CurIcon" :size="14" :weight="w" />
          </button>
        </div>
      </div>

      <div class="fip-control">
        <p>Color</p>
        <div class="fip-sw">
          <button
            v-for="c in FOLDER_ICON_COLORS"
            :key="c.id"
            type="button"
            class="sw"
            :class="{ on: color === c.value, inherit: c.value === '' }"
            :style="c.value ? { background: c.value } : {}"
            :title="c.id"
            @click="color = c.value"
          />
        </div>
      </div>

      <button class="fip-assign" type="button" @click="assign(current)"><PhCheck :size="15" /> Assign</button>
    </div>

    <div class="fip-right">
      <div class="fip-search"><PhMagnifyingGlass :size="14" /><span>{{ query }}<i class="caret" /></span></div>
      <div class="fip-grid">
        <button
          v-for="(it, i) in visible"
          :key="it.id"
          type="button"
          class="cell"
          :class="{ active: cursor === i }"
          :style="cursor === i && color ? { color } : {}"
          @mouseenter="cursor = i"
          @click="assign(it)"
        >
          <component :is="it.icon" :size="19" :weight="weight" />
        </button>
        <p v-if="!visible.length" class="fip-empty">No icons match “{{ query }}”</p>
      </div>
      <footer class="fip-foot"><kbd>↵</kbd> assign<kbd>esc</kbd> cancel<span class="dim">{{ visible.length }} icons</span></footer>
    </div>
  </div>
</template>

<style scoped>
.fip-scrim { position: fixed; inset: 0; z-index: 80; background: rgba(8, 8, 14, .55); backdrop-filter: blur(3px) }
.fip {
  position: fixed; left: 50%; top: 50%; transform: translate(-50%, -50%); z-index: 81;
  width: min(660px, calc(100vw - 32px)); display: grid; grid-template-columns: 230px minmax(0, 1fr);
  border: 1px solid var(--border-2); border-radius: 18px; background: var(--surface);
  box-shadow: 0 28px 80px -24px rgba(0, 0, 0, .85), inset 0 1px 0 rgba(255, 255, 255, .08); overflow: hidden;
}
.fip button { font: inherit; cursor: pointer }
.fip-x { position: absolute; top: 10px; right: 10px; z-index: 2; display: grid; place-items: center; width: 26px; height: 26px; border: 0; border-radius: 8px; background: transparent; color: var(--text-mut) }
.fip-x:hover { background: var(--surface-3); color: var(--text) }
.caret { display: inline-block; width: 1.5px; height: 1.05em; margin-left: 1px; background: var(--accent); vertical-align: text-bottom; animation: fipblink 1.1s step-end infinite }
@keyframes fipblink { 50% { opacity: 0 } }

.fip-left { display: flex; flex-direction: column; align-items: center; gap: 12px; padding: 22px 18px; border-right: 1px solid var(--border); background: var(--surface-2) }
.fip-glyph { display: grid; place-items: center; width: 120px; height: 120px; border: 1px solid var(--border-2); border-radius: 22px; background: var(--surface-3) }
.fip-name { color: var(--head); font-size: 15px }
.fip-folder { margin-top: -8px; color: var(--text-mut); font: 11px "JetBrains Mono", ui-monospace, monospace; max-width: 100%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap }
.fip-control { width: 100% }
.fip-control p { margin: 0 0 6px; color: var(--text-mut); font: 10px "JetBrains Mono", ui-monospace, monospace; text-transform: uppercase; letter-spacing: .05em }
.fip-name-input { width: 100%; border: 1px solid var(--border-2); border-radius: 9px; background: var(--bg); color: var(--text); padding: 8px 10px; font: inherit; outline: none }
.fip-name-input:focus { border-color: var(--accent-line) }
.fip-wt { display: grid; grid-template-columns: repeat(4, 1fr); gap: 5px }
.fip-wt button { display: grid; place-items: center; height: 34px; border: 1px solid var(--border-2); border-radius: 9px; background: var(--surface-3); color: var(--text-dim) }
.fip-wt button.on { border-color: var(--accent-line); background: var(--accent-soft); color: var(--accent) }
.fip-sw { display: flex; gap: 8px; flex-wrap: wrap }
.fip-sw .sw { width: 22px; height: 22px; border-radius: 50%; border: 2px solid transparent }
.fip-sw .sw.on { border-color: var(--text) }
.fip-sw .sw.inherit { background: var(--surface-3); position: relative }
.fip-sw .sw.inherit::after { content: "A"; position: absolute; inset: 0; display: grid; place-items: center; color: var(--text-mut); font: 700 11px "JetBrains Mono", ui-monospace, monospace }
.fip-assign { display: flex; align-items: center; justify-content: center; gap: 7px; width: 100%; margin-top: auto; border: 1px solid var(--accent-line); border-radius: 11px; background: var(--accent-soft); color: var(--accent); padding: 11px; font-size: 13px }

.fip-right { display: flex; flex-direction: column; gap: 10px; padding: 16px }
/* Right margin keeps the bar clear of the absolute close button (.fip-x), so
   clicking the search bar can't accidentally land on (and trigger) Close. */
.fip-search { display: flex; align-items: center; gap: 8px; margin-right: 30px; padding: 8px 12px; border: 1px solid var(--border-2); border-radius: 10px; background: var(--surface-3); color: var(--text-mut); font: 12px "JetBrains Mono", ui-monospace, monospace }
.fip-search span { display: inline-flex; align-items: center; min-height: 1.05em; color: var(--text) }
.fip-grid { display: grid; grid-template-columns: repeat(7, 1fr); gap: 4px; align-content: start; min-height: 160px }
.fip-grid .cell { display: grid; place-items: center; aspect-ratio: 1; border: 1px solid transparent; border-radius: 10px; background: transparent; color: var(--text-dim) }
.fip-grid .cell:hover { color: var(--text) }
.fip-grid .cell.active { background: var(--accent-soft); border-color: var(--accent-line); color: var(--accent) }
.fip-empty { grid-column: 1 / -1; margin: 0; padding: 24px; text-align: center; color: var(--text-mut) }
.fip-foot { display: flex; align-items: center; gap: 7px; color: var(--text-mut); font-size: 11px }
.fip-foot kbd { display: inline-grid; place-items: center; min-width: 18px; height: 18px; padding: 0 5px; margin-right: 2px; border: 1px solid var(--border-2); border-radius: 5px; background: var(--surface-3); color: var(--accent); font: 10px "JetBrains Mono", ui-monospace, monospace }
.fip-foot .dim { margin-left: auto; font-family: "JetBrains Mono", ui-monospace, monospace }

@media (max-width: 640px) { .fip { grid-template-columns: 1fr } }
</style>
