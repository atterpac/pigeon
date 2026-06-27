<script setup lang="ts">
// Sandbox: the REAL sidebar markup (styled by global shell.css/tokens.css so it
// matches the live app + theme), with folder edit-view styles layered on top.
import { computed, ref } from 'vue'
import {
  PhArchiveBox,
  PhBriefcase,
  PhCaretDown,
  PhCheck,
  PhDotsThree,
  PhEyeSlash,
  PhFolderSimple,
  PhGearSix,
  PhNotePencil,
  PhPencilSimple,
  PhPlus,
  PhReceipt,
  PhRocket,
  PhSmiley,
  PhStar,
  PhTag,
  PhTarget,
  PhTrash,
  PhTray,
  PhX,
} from '@phosphor-icons/vue'

defineOptions({ name: 'SandboxScratchpad' })

type Variant = 'inline' | 'expand' | 'popover' | 'sheet' | 'menu' | 'swap'
const variants: Array<{ id: Variant; label: string; blurb: string }> = [
  { id: 'inline', label: 'Inline', blurb: 'Row morphs in place: icon button + name field + confirm' },
  { id: 'expand', label: 'Expand', blurb: 'Row stays, an edit card unfolds beneath it' },
  { id: 'popover', label: 'Popover', blurb: 'Floating editor anchored to the row' },
  { id: 'sheet', label: 'Sheet', blurb: 'Panel slides in over the sidebar' },
  { id: 'menu', label: 'Menu', blurb: 'Context menu of actions; rename/icon are entries' },
  { id: 'swap', label: 'Swap', blurb: 'Whole row becomes a compact toolbar' },
]
const variant = ref<Variant>('inline')

// ---- mock nav data (mirrors the real sidebar) ----
type Folder = { id: string; name: string; icon: any; unread?: number; role?: boolean }
const folders = ref<Folder[]>([
  { id: 'inbox', name: 'Inbox', icon: PhTray, unread: 18, role: true },
  { id: 'archive', name: 'Archive', icon: PhArchiveBox, role: true },
  { id: 'q3', name: 'Q3 Launch', icon: PhRocket, unread: 4 },
  { id: 'clients', name: 'Clients', icon: PhBriefcase },
  { id: 'receipts', name: 'Receipts', icon: PhReceipt, unread: 2 },
])
const labels = [
  { id: 'launch', name: 'Launch', swatch: 'var(--accent)' },
  { id: 'review', name: 'Review', swatch: 'var(--green)' },
  { id: 'vendor', name: 'Vendor', swatch: 'var(--red)' },
]

const activeId = ref('q3')
const editingId = ref('')
const editing = computed(() => folders.value.find((f) => f.id === editingId.value) ?? null)
const draftName = ref('')

const iconChoices = [PhFolderSimple, PhRocket, PhTarget, PhStar, PhTag, PhBriefcase, PhReceipt, PhArchiveBox]
const pickedIcon = ref<any>(PhRocket)
const iconOpen = ref(false)

const toast = ref('')
function fireToast(t: string) { toast.value = t; setTimeout(() => (toast.value = ''), 2200) }
function startEdit(f: Folder) { activeId.value = f.id; editingId.value = f.id; draftName.value = f.name; pickedIcon.value = f.icon; iconOpen.value = false }
function save() { if (editing.value) { editing.value.name = draftName.value.trim() || editing.value.name; editing.value.icon = pickedIcon.value } fireToast('Folder updated'); editingId.value = ''; iconOpen.value = false }
function cancel() { editingId.value = ''; iconOpen.value = false }
function choose(ic: any) { pickedIcon.value = ic; iconOpen.value = false }
function isEditing(f: Folder) { return editingId.value === f.id }

const blurb = computed(() => variants.find((v) => v.id === variant.value)?.blurb ?? '')
</script>

<template>
  <div class="sandbox">
    <nav class="floating-switcher" aria-label="Folder edit views">
      <span>Folder edit</span>
      <button v-for="v in variants" :key="v.id" type="button" :class="{ active: variant === v.id }" @click="variant = v.id">{{ v.label }}</button>
    </nav>

    <div class="stage">
      <!-- REAL sidebar markup; styled by global shell.css -->
      <aside class="sidebar sidebar-flat nav-icons">
        <div class="account-wrap">
          <button class="account account-command-trigger" type="button">
            <span class="cmdprompt">~/</span>
            <span class="cmdpath"><b>atterpac</b></span>
            <PhCaretDown class="chev" :size="12" />
          </button>
        </div>

        <button class="composebtn" type="button"><PhNotePencil :size="15" /><span class="navlabel">Compose</span> <kbd>c</kbd></button>

        <div class="grouphead grouphead-row">
          <span>Folders</span>
          <button class="grouphead-action" type="button" title="New folder"><PhPlus :size="13" /></button>
        </div>
        <nav class="navgroup">
          <div v-for="f in folders" :key="f.id" class="navrow" :class="{ anchor: isEditing(f) && (variant === 'popover' || variant === 'menu') }">

            <!-- ===== INLINE ===== -->
            <div v-if="isEditing(f) && variant === 'inline'" class="edit-inline">
              <button class="icon-trigger" type="button" @click="iconOpen = !iconOpen"><component :is="pickedIcon" :size="16" /></button>
              <input v-model="draftName" class="name-input" @keydown.enter="save" @keydown.esc="cancel" />
              <button class="folder-mini ok" type="button" @click="save"><PhCheck :size="13" /></button>
              <button class="folder-mini" type="button" @click="cancel"><PhX :size="13" /></button>
              <div v-if="iconOpen" class="icon-pop">
                <button v-for="(ic, i) in iconChoices" :key="i" type="button" :class="{ on: pickedIcon === ic }" @click="choose(ic)"><component :is="ic" :size="16" /></button>
              </div>
            </div>

            <!-- ===== SWAP ===== -->
            <div v-else-if="isEditing(f) && variant === 'swap'" class="edit-swap">
              <button class="icon-trigger sm" type="button" @click="iconOpen = !iconOpen"><component :is="pickedIcon" :size="15" /></button>
              <input v-model="draftName" class="name-input bare" @keydown.enter="save" @keydown.esc="cancel" />
              <button class="folder-mini" type="button" title="Icon" @click="iconOpen = !iconOpen"><PhSmiley :size="13" /></button>
              <button class="folder-mini" type="button" title="Delete"><PhTrash :size="13" /></button>
              <button class="folder-mini ok" type="button" @click="save"><PhCheck :size="13" /></button>
              <div v-if="iconOpen" class="icon-pop">
                <button v-for="(ic, i) in iconChoices" :key="i" type="button" :class="{ on: pickedIcon === ic }" @click="choose(ic)"><component :is="ic" :size="16" /></button>
              </div>
            </div>

            <!-- ===== default row (+ overlays for expand/popover/menu) ===== -->
            <template v-else>
              <button class="navitem" :class="{ active: activeId === f.id }" type="button" @click="startEdit(f)">
                <span class="navicon"><component :is="isEditing(f) ? pickedIcon : f.icon" :size="16" /></span>
                <span class="navlabel">{{ isEditing(f) ? (draftName || f.name) : f.name }}</span>
                <span v-if="f.unread && !isEditing(f)" class="dot">{{ f.unread }}</span>
              </button>
              <div v-if="!isEditing(f)" class="navactions">
                <button class="folder-mini" type="button" title="Set icon" @click="startEdit(f)"><PhSmiley :size="13" /></button>
                <button class="folder-mini" type="button" title="Hide folder"><PhEyeSlash :size="13" /></button>
                <button v-if="!f.role" class="folder-mini" type="button" title="Rename folder" @click="startEdit(f)"><PhPencilSimple :size="13" /></button>
                <button v-if="!f.role" class="folder-mini" type="button" title="More"><PhDotsThree :size="13" /></button>
              </div>

              <!-- EXPAND card unfolds beneath the (still visible) row -->
              <div v-if="isEditing(f) && variant === 'expand'" class="edit-card">
                <div class="ec-row">
                  <button class="icon-trigger lg" type="button" @click="iconOpen = !iconOpen"><component :is="pickedIcon" :size="20" /></button>
                  <input v-model="draftName" class="name-input" @keydown.enter="save" @keydown.esc="cancel" />
                </div>
                <div v-if="iconOpen" class="icon-grid">
                  <button v-for="(ic, i) in iconChoices" :key="i" type="button" :class="{ on: pickedIcon === ic }" @click="choose(ic)"><component :is="ic" :size="16" /></button>
                </div>
                <div class="ec-actions">
                  <button class="ec-del" type="button"><PhTrash :size="13" /> Delete</button>
                  <div class="spacer" />
                  <button class="ec-cancel" type="button" @click="cancel">Cancel</button>
                  <button class="ec-save" type="button" @click="save">Save</button>
                </div>
              </div>

              <!-- POPOVER anchored to the row -->
              <div v-if="isEditing(f) && variant === 'popover'" class="popover">
                <header class="pop-head">Edit folder</header>
                <div class="ec-row">
                  <button class="icon-trigger" type="button" @click="iconOpen = !iconOpen"><component :is="pickedIcon" :size="16" /></button>
                  <input v-model="draftName" class="name-input" @keydown.enter="save" @keydown.esc="cancel" />
                </div>
                <div class="icon-grid">
                  <button v-for="(ic, i) in iconChoices" :key="i" type="button" :class="{ on: pickedIcon === ic }" @click="choose(ic)"><component :is="ic" :size="15" /></button>
                </div>
                <div class="ec-actions">
                  <button class="ec-del" type="button"><PhTrash :size="13" /></button>
                  <div class="spacer" />
                  <button class="ec-cancel" type="button" @click="cancel">Cancel</button>
                  <button class="ec-save" type="button" @click="save">Save</button>
                </div>
              </div>

              <!-- MENU of actions -->
              <div v-if="isEditing(f) && variant === 'menu'" class="ctx-menu">
                <button type="button" @click="fireToast('Rename…')"><PhPencilSimple :size="14" /> Rename</button>
                <button type="button" @click="fireToast('Pick icon…')"><PhSmiley :size="14" /> Change icon</button>
                <button type="button" @click="fireToast('Hidden')"><PhEyeSlash :size="14" /> Hide</button>
                <hr />
                <button type="button" class="danger" @click="fireToast('Deleted')"><PhTrash :size="14" /> Delete</button>
              </div>
            </template>
          </div>
        </nav>

        <p class="grouphead">Labels</p>
        <nav class="navgroup">
          <button v-for="l in labels" :key="l.id" class="navitem label" type="button">
            <span class="navicon"><span class="swatch" :style="{ backgroundColor: l.swatch }" /></span>
            <span class="navlabel">{{ l.name }}</span>
          </button>
        </nav>

        <div class="sidebar-foot">
          <button class="navitem settings-navitem" type="button">
            <span class="navicon"><PhGearSix :size="16" /></span>
            <span class="navlabel">Settings</span>
          </button>
        </div>

        <!-- SHEET slides over the sidebar -->
        <transition name="sheet">
          <div v-if="variant === 'sheet' && editing" class="sheet">
            <header class="sheet-head"><b>Edit folder</b><button class="folder-mini" type="button" @click="cancel"><PhX :size="14" /></button></header>
            <div class="field"><span>Icon</span>
              <div class="icon-grid">
                <button v-for="(ic, i) in iconChoices" :key="i" type="button" :class="{ on: pickedIcon === ic }" @click="pickedIcon = ic"><component :is="ic" :size="17" /></button>
              </div>
            </div>
            <div class="field"><span>Name</span>
              <input v-model="draftName" class="name-input solid" @keydown.enter="save" @keydown.esc="cancel" />
            </div>
            <div class="sheet-foot">
              <button class="ec-del" type="button"><PhTrash :size="13" /> Delete</button>
              <div class="spacer" />
              <button class="ec-cancel" type="button" @click="cancel">Cancel</button>
              <button class="ec-save" type="button" @click="save">Save</button>
            </div>
          </div>
        </transition>
      </aside>

      <p class="hint">Click any folder row to open its edit view · style: <b>{{ variant }}</b></p>
    </div>

    <transition name="toast"><div v-if="toast" class="toast"><PhCheck :size="14" /> {{ toast }}</div></transition>
    <div class="variant-note"><span>{{ blurb }}</span></div>
  </div>
</template>

<style scoped>
/* Rely on the global theme tokens (tokens.css) so this matches the live app.
   Only a local --surface-3 fallback for the few raised surfaces below. */
.sandbox{ --surface-3: color-mix(in oklab, var(--surface-2) 80%, var(--text-mut) 12%);
  position:relative;height:100%;min-height:0;overflow:hidden;color:var(--text);background:var(--bg);font:14px "Hanken Grotesk",Inter,ui-sans-serif,system-ui,sans-serif }
button{font:inherit;cursor:pointer}
input{font:inherit}

.floating-switcher{position:fixed;left:50%;top:14px;z-index:60;display:flex;align-items:center;gap:5px;max-width:calc(100vw - 28px);padding:6px;border:1px solid var(--border-2);border-radius:12px;background:color-mix(in oklab,var(--surface) 90%,transparent);box-shadow:var(--shadow-2);backdrop-filter:blur(10px);transform:translateX(-50%)}
.floating-switcher span{padding:0 9px;color:var(--text-mut);font:11px "JetBrains Mono",ui-monospace,monospace}
.floating-switcher button{height:30px;border:0;border-radius:8px;background:transparent;color:var(--text-dim);padding:0 12px;font:12px "JetBrains Mono",ui-monospace,monospace}
.floating-switcher button:hover,.floating-switcher button.active{background:var(--accent-soft);color:var(--accent)}
.variant-note{position:absolute;top:58px;left:50%;transform:translateX(-50%);z-index:40;border:1px solid var(--border-2);border-radius:999px;background:var(--surface-2);padding:5px 13px;color:var(--text-mut);font:11px "JetBrains Mono",ui-monospace,monospace}

.stage{position:absolute;inset:0;display:flex;flex-direction:column;align-items:center;gap:20px;padding:104px 16px 16px}
.sidebar{width:264px;height:min(560px,calc(100% - 40px))}
.hint{margin:0;color:var(--text-mut);font:12px "JetBrains Mono",ui-monospace,monospace}.hint b{color:var(--accent)}
.swatch{width:9px;height:9px;border-radius:50%}

/* ---- edit-view pieces (scoped) ---- */
.icon-trigger{flex:none;display:grid;place-items:center;width:30px;height:30px;border:1px solid var(--border-2);border-radius:8px;background:var(--surface-3);color:var(--accent)}
.icon-trigger.sm{width:26px;height:26px}
.icon-trigger.lg{width:38px;height:38px;border-radius:10px}
.icon-trigger:hover{border-color:var(--accent-line)}
.name-input{flex:1;min-width:0;border:1px solid var(--border-2);border-radius:8px;background:var(--bg);color:var(--text);padding:7px 9px;outline:none}
.name-input:focus{border-color:var(--accent-line)}
.name-input.bare{border-color:transparent;background:transparent;padding:6px 4px}
.name-input.solid{background:var(--surface-3)}
.folder-mini.ok{color:var(--green);border-color:color-mix(in oklab,var(--green) 40%,var(--border-2))}

.icon-grid{display:grid;grid-template-columns:repeat(8,1fr);gap:4px}
.icon-grid button{display:grid;place-items:center;aspect-ratio:1;border:1px solid transparent;border-radius:8px;background:var(--surface-3);color:var(--text-dim)}
.icon-grid button:hover{color:var(--text)}
.icon-grid button.on{border-color:var(--accent-line);background:var(--accent-soft);color:var(--accent)}

.icon-pop{position:absolute;top:calc(100% + 5px);left:0;z-index:30;display:grid;grid-template-columns:repeat(4,1fr);gap:4px;padding:6px;border:1px solid var(--border-2);border-radius:11px;background:var(--surface);box-shadow:var(--shadow-2)}
.icon-pop button{display:grid;place-items:center;width:32px;height:32px;border:1px solid transparent;border-radius:8px;background:var(--surface-3);color:var(--text-dim)}
.icon-pop button.on{border-color:var(--accent-line);background:var(--accent-soft);color:var(--accent)}

/* INLINE / SWAP — replace the row content */
.edit-inline{position:relative;display:flex;align-items:center;gap:5px;width:100%;padding:3px;border-radius:9px;background:var(--surface-2);box-shadow:inset 0 0 0 1px var(--accent-line)}
.edit-swap{position:relative;display:flex;align-items:center;gap:3px;width:100%;padding:3px 5px;border-radius:9px;background:var(--surface-2);box-shadow:inset 0 0 0 1px var(--border-2)}

/* EXPAND */
.navrow{flex-wrap:wrap}
.edit-card{flex-basis:100%;margin:4px 0 6px;padding:11px;border:1px solid var(--border-2);border-radius:12px;background:var(--surface-2);display:grid;gap:9px}
.ec-row{display:flex;align-items:center;gap:9px}
.ec-actions{display:flex;align-items:center;gap:7px}.ec-actions .spacer{flex:1}
.ec-del{display:flex;align-items:center;gap:5px;border:1px solid transparent;border-radius:8px;background:transparent;color:var(--red);padding:6px 9px;font-size:12px}.ec-del:hover{background:color-mix(in oklab,var(--red) 14%,transparent)}
.ec-cancel{border:1px solid var(--border-2);border-radius:8px;background:var(--surface-3);color:var(--text-dim);padding:6px 12px;font-size:12px}
.ec-save{border:1px solid var(--accent-line);border-radius:8px;background:var(--accent);color:var(--accent-ink);padding:6px 14px;font-size:12px;font-weight:600}

/* POPOVER / MENU need the row to allow overflow */
.navrow.anchor{overflow:visible}
.popover{position:absolute;top:calc(100% + 6px);left:0;z-index:30;width:248px;padding:12px;border:1px solid var(--border-2);border-radius:13px;background:var(--surface);box-shadow:var(--shadow-2),var(--top-hi);display:grid;gap:10px}
.pop-head{color:var(--text-mut);font:11px "JetBrains Mono",ui-monospace,monospace;text-transform:uppercase}

.ctx-menu{position:absolute;top:calc(100% + 5px);left:8px;z-index:30;width:190px;padding:5px;border:1px solid var(--border-2);border-radius:11px;background:var(--surface);box-shadow:var(--shadow-2);display:grid;gap:1px}
.ctx-menu button{display:flex;align-items:center;gap:10px;border:0;border-radius:7px;background:transparent;color:var(--text-dim);padding:8px 10px;text-align:left;font-size:13px}
.ctx-menu button:hover{background:var(--surface-2);color:var(--text)}
.ctx-menu button.danger{color:var(--red)}.ctx-menu button.danger:hover{background:color-mix(in oklab,var(--red) 14%,transparent)}
.ctx-menu hr{margin:3px 6px;border:0;border-top:1px solid var(--border)}

/* SHEET */
.sheet{position:absolute;left:0;right:0;top:0;bottom:0;z-index:40;display:flex;flex-direction:column;gap:14px;padding:16px;border-radius:16px;background:var(--surface);box-shadow:var(--shadow-2)}
.sheet-head{display:flex;align-items:center;justify-content:space-between}.sheet-head b{color:var(--head);font-size:15px}
.field{display:grid;gap:7px}.field>span{color:var(--text-mut);font:11px "JetBrains Mono",ui-monospace,monospace;text-transform:uppercase}
.sheet-foot{margin-top:auto;display:flex;align-items:center;gap:7px}.sheet-foot .spacer{flex:1}
.sheet-enter-active,.sheet-leave-active{transition:transform .22s ease,opacity .22s}.sheet-enter-from,.sheet-leave-to{transform:translateX(-12px);opacity:0}

.toast{position:absolute;left:50%;bottom:32px;z-index:50;transform:translateX(-50%);display:flex;align-items:center;gap:8px;border:1px solid var(--accent-line);border-radius:10px;background:var(--surface-2);color:var(--text);padding:10px 15px;font:12.5px "JetBrains Mono",ui-monospace,monospace;box-shadow:var(--shadow-2)}
.toast svg{color:var(--green)}
.toast-enter-active,.toast-leave-active{transition:opacity .18s,transform .18s}.toast-enter-from,.toast-leave-to{opacity:0;transform:translate(-50%,8px)}
</style>
