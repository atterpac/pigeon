<script setup lang="ts">
import { computed, ref } from 'vue'
import {
  PhCheck,
  PhCopy,
  PhDotsThree,
  PhFloppyDisk,
  PhPlus,
  PhSignature,
  PhStar,
  PhTrash,
} from '@phosphor-icons/vue'

defineOptions({ name: 'SandboxScratchpad' })

type Variant = 'split' | 'cards' | 'inline' | 'preview' | 'compact' | 'command'
type Signature = { id: string; name: string; body: string; html: string; default?: boolean }

const variants: Array<{ id: Variant; label: string; blurb: string }> = [
  { id: 'split', label: 'Split', blurb: 'Saved signatures on the left, focused editor on the right' },
  { id: 'cards', label: 'Cards', blurb: 'Each signature is its own editable card with inline controls' },
  { id: 'inline', label: 'Inline', blurb: 'Dense table-like rows for quick renaming and defaulting' },
  { id: 'preview', label: 'Preview', blurb: 'Editor paired with a compose-style rendered preview' },
  { id: 'compact', label: 'Compact', blurb: 'One-select workflow for a tighter settings panel' },
  { id: 'command', label: 'Command', blurb: 'Keyboard-forward picker with actions beside the editor' },
]

const variant = ref<Variant>('split')
const selectedId = ref('personal')
const toast = ref('')
const signatures = ref<Signature[]>([
  { id: 'personal', name: 'Personal', body: 'Michael Capretta\natterpac\nmichael@example.com', html: '<strong>Michael Capretta</strong><br>atterpac<br><a href="mailto:michael@example.com">michael@example.com</a>', default: true },
  { id: 'work', name: 'Work', body: 'Michael Capretta\nProduct Engineering\nBells & Steel', html: '<strong>Michael Capretta</strong><br><span style="color:#667085">Product Engineering</span><br>Bells & Steel' },
  { id: 'short', name: 'Short', body: 'Michael', html: 'Michael' },
])

const selected = computed(() => signatures.value.find((signature) => signature.id === selectedId.value) ?? signatures.value[0])
const blurb = computed(() => variants.find((item) => item.id === variant.value)?.blurb ?? '')

function showToast(message: string) {
  toast.value = message
  setTimeout(() => (toast.value = ''), 1800)
}

function addSignature() {
  const id = `sig-${Date.now()}`
  signatures.value.push({ id, name: `Signature ${signatures.value.length + 1}`, body: '', html: '' })
  selectedId.value = id
  showToast('Signature added')
}

function duplicateSignature(signature = selected.value) {
  if (!signature) return
  const id = `sig-${Date.now()}`
  signatures.value.push({ id, name: `${signature.name} copy`, body: signature.body, html: signature.html })
  selectedId.value = id
  showToast('Signature duplicated')
}

function removeSignature(id: string) {
  const index = signatures.value.findIndex((signature) => signature.id === id)
  signatures.value = signatures.value.filter((signature) => signature.id !== id)
  if (!signatures.value.some((signature) => signature.default) && signatures.value[0]) signatures.value[0].default = true
  if (selectedId.value === id) selectedId.value = signatures.value[Math.max(0, index - 1)]?.id ?? ''
  showToast('Signature deleted')
}

function setDefault(id: string) {
  for (const signature of signatures.value) signature.default = signature.id === id
  showToast('Default updated')
}

function addLogo(signature = selected.value) {
  if (!signature) return
  signature.html += '<br><img src="data:image/svg+xml,%3Csvg xmlns=%22http://www.w3.org/2000/svg%22 width=%22160%22 height=%2242%22 viewBox=%220 0 160 42%22%3E%3Crect width=%22160%22 height=%2242%22 rx=%228%22 fill=%22%2386a5ff%22/%3E%3Ctext x=%2218%22 y=%2227%22 font-family=%22Arial%22 font-size=%2218%22 font-weight=%22700%22 fill=%22%23151622%22%3EPigeon%3C/text%3E%3C/svg%3E" alt="Pigeon" style="max-width:160px;height:auto;display:block;margin-top:8px">'
  showToast('Image inserted')
}
</script>

<template>
  <div class="sandbox siglab">
    <nav class="floating-switcher" aria-label="Signature editor views">
      <span>Signature editor</span>
      <button v-for="v in variants" :key="v.id" type="button" :class="{ active: variant === v.id }" @click="variant = v.id">{{ v.label }}</button>
    </nav>

    <div class="variant-note"><span>{{ blurb }}</span></div>

    <main class="sig-stage">
      <section class="sig-panel" :class="`sig-${variant}`">
        <header class="sig-head">
          <div>
            <PhSignature :size="18" />
            <b>Signatures</b>
          </div>
          <button type="button" @click="addSignature"><PhPlus :size="14" /> New</button>
        </header>

        <template v-if="variant === 'split'">
          <nav class="sig-list">
            <button v-for="signature in signatures" :key="signature.id" type="button" :class="{ active: selectedId === signature.id }" @click="selectedId = signature.id">
              <b>{{ signature.name }}</b>
              <small>{{ signature.default ? 'default' : `${signature.body.length} chars` }}</small>
            </button>
          </nav>
          <article v-if="selected" class="sig-editor">
            <input v-model="selected.name" />
            <textarea v-model="selected.body" />
            <footer><button type="button" @click="addLogo(selected)">Image</button><button type="button" @click="setDefault(selected.id)"><PhStar :size="13" /> Default</button><button type="button" @click="showToast('Saved')"><PhFloppyDisk :size="13" /> Save</button></footer>
          </article>
        </template>

        <template v-else-if="variant === 'cards'">
          <div class="sig-card-grid">
            <article v-for="signature in signatures" :key="signature.id" class="sig-card" :class="{ active: selectedId === signature.id }" @click="selectedId = signature.id">
              <header><input v-model="signature.name" @click.stop /><button type="button" @click.stop="setDefault(signature.id)"><PhStar :weight="signature.default ? 'fill' : 'regular'" :size="14" /></button></header>
              <textarea v-model="signature.body" @click.stop />
              <footer><button type="button" @click.stop="addLogo(signature)">Image</button><button type="button" @click.stop="duplicateSignature(signature)"><PhCopy :size="13" /></button><button type="button" @click.stop="removeSignature(signature.id)"><PhTrash :size="13" /></button></footer>
            </article>
          </div>
        </template>

        <template v-else-if="variant === 'inline'">
          <div class="sig-table">
            <div v-for="signature in signatures" :key="signature.id" class="sig-row" :class="{ active: selectedId === signature.id }">
              <button type="button" @click="selectedId = signature.id"><PhSignature :size="14" /></button>
              <input v-model="signature.name" />
              <textarea v-model="signature.body" />
              <button type="button" @click="setDefault(signature.id)"><PhStar :weight="signature.default ? 'fill' : 'regular'" :size="14" /></button>
              <button type="button" @click="removeSignature(signature.id)"><PhTrash :size="14" /></button>
            </div>
          </div>
        </template>

        <template v-else-if="variant === 'preview'">
          <article v-if="selected" class="sig-editor">
            <div class="sig-editor-row"><input v-model="selected.name" /><button type="button" @click="setDefault(selected.id)"><PhStar :weight="selected.default ? 'fill' : 'regular'" :size="14" /></button></div>
            <textarea v-model="selected.body" />
          </article>
          <aside class="compose-preview">
            <p>Thanks, I can take a look this afternoon.</p>
            <div class="preview-signature" v-html="selected?.html" />
          </aside>
        </template>

        <template v-else-if="variant === 'compact'">
          <article v-if="selected" class="sig-compact-editor">
            <select v-model="selectedId">
              <option v-for="signature in signatures" :key="signature.id" :value="signature.id">{{ signature.name }}{{ signature.default ? ' · default' : '' }}</option>
            </select>
            <input v-model="selected.name" />
            <textarea v-model="selected.body" />
            <footer><button type="button" @click="addLogo()">Image</button><button type="button" @click="setDefault(selected.id)">Make default</button><button type="button" @click="duplicateSignature()">Duplicate</button></footer>
          </article>
        </template>

        <template v-else>
          <nav class="sig-command-list">
            <button v-for="(signature, index) in signatures" :key="signature.id" type="button" :class="{ active: selectedId === signature.id }" @click="selectedId = signature.id">
              <kbd>{{ index + 1 }}</kbd><span>{{ signature.name }}</span><small>{{ signature.default ? 'default' : 'saved' }}</small>
            </button>
          </nav>
          <article v-if="selected" class="sig-editor command-editor">
            <input v-model="selected.name" />
            <textarea v-model="selected.body" />
            <footer>
              <button type="button" @click="setDefault(selected.id)"><PhStar :size="13" /> Default</button>
              <button type="button" @click="addLogo()"><PhSignature :size="13" /> Image</button>
              <button type="button" @click="duplicateSignature()"><PhCopy :size="13" /> Duplicate</button>
              <button type="button" @click="removeSignature(selected.id)"><PhTrash :size="13" /> Delete</button>
              <button type="button" @click="showToast('More actions')"><PhDotsThree :size="13" /></button>
            </footer>
          </article>
        </template>
      </section>
    </main>

    <transition name="toast"><div v-if="toast" class="toast"><PhCheck :size="14" /> {{ toast }}</div></transition>
  </div>
</template>

<style scoped>
.siglab{position:relative;height:100%;min-height:0;overflow:hidden;color:var(--text);background:var(--bg);font:14px "Hanken Grotesk",Inter,ui-sans-serif,system-ui,sans-serif}
button,input,textarea,select{font:inherit}
button{cursor:pointer}
.floating-switcher{position:fixed;left:50%;top:14px;z-index:60;display:flex;align-items:center;gap:5px;max-width:calc(100vw - 28px);padding:6px;border:1px solid var(--border-2);border-radius:12px;background:color-mix(in oklab,var(--surface) 90%,transparent);box-shadow:var(--shadow-2);backdrop-filter:blur(10px);transform:translateX(-50%)}
.floating-switcher span{padding:0 9px;color:var(--text-mut);font:11px "JetBrains Mono",ui-monospace,monospace}
.floating-switcher button{height:30px;border:0;border-radius:8px;background:transparent;color:var(--text-dim);padding:0 12px;font:12px "JetBrains Mono",ui-monospace,monospace}
.floating-switcher button:hover,.floating-switcher button.active{background:var(--accent-soft);color:var(--accent)}
.variant-note{position:absolute;top:58px;left:50%;transform:translateX(-50%);z-index:40;border:1px solid var(--border-2);border-radius:999px;background:var(--surface-2);padding:5px 13px;color:var(--text-mut);font:11px "JetBrains Mono",ui-monospace,monospace}
.sig-stage{position:absolute;inset:0;display:grid;place-items:center;padding:110px 24px 28px}
.sig-panel{display:grid;grid-template-columns:220px minmax(0,1fr);grid-template-rows:auto minmax(0,1fr);gap:14px;width:min(980px,100%);height:min(620px,100%);border:1px solid var(--border-2);border-radius:16px;background:var(--surface);box-shadow:var(--shadow-2);padding:16px;overflow:hidden}
.sig-head{grid-column:1/-1;display:flex;align-items:center;justify-content:space-between;border-bottom:1px solid var(--border);padding-bottom:12px}
.sig-head div,.sig-head button,.sig-editor footer button,.sig-card footer button,.sig-compact-editor footer button{display:flex;align-items:center;gap:7px}
.sig-head b{font-size:15px;color:var(--head)}
.sig-head svg{color:var(--accent)}
.sig-head button,.sig-editor footer button,.sig-card footer button,.sig-compact-editor footer button{border:1px solid var(--border-2);border-radius:8px;background:var(--surface-2);color:var(--text-dim);padding:7px 10px;font-size:12px}
.sig-list{display:flex;flex-direction:column;gap:6px;min-width:0}
.sig-list button{display:grid;gap:3px;border:1px solid var(--border-2);border-radius:10px;background:transparent;color:var(--text-dim);text-align:left;padding:10px}
.sig-list button.active{border-color:var(--accent-line);background:var(--accent-soft)}
.sig-list b{color:var(--text)}
.sig-list small{color:var(--text-mut)}
.sig-editor,.sig-compact-editor{min-width:0;display:grid;grid-template-rows:auto minmax(0,1fr) auto;gap:10px}
.sig-editor input,.sig-editor textarea,.sig-compact-editor input,.sig-compact-editor textarea,.sig-compact-editor select,.sig-card input,.sig-card textarea,.sig-row input,.sig-row textarea{border:1px solid var(--border-2);border-radius:9px;background:var(--bg);color:var(--text);outline:none;padding:9px 10px}
.sig-editor textarea,.sig-compact-editor textarea,.sig-card textarea,.sig-row textarea{resize:none;line-height:1.45;font:12.5px "JetBrains Mono",ui-monospace,monospace}
.sig-editor input:focus,.sig-editor textarea:focus,.sig-compact-editor input:focus,.sig-compact-editor textarea:focus,.sig-compact-editor select:focus,.sig-card input:focus,.sig-card textarea:focus,.sig-row input:focus,.sig-row textarea:focus{border-color:var(--accent-line)}
.sig-editor footer,.sig-card footer,.sig-compact-editor footer{display:flex;justify-content:flex-end;gap:8px}
.sig-card-grid{grid-column:1/-1;display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:12px;overflow:auto}
.sig-card{display:grid;grid-template-rows:auto minmax(0,1fr) auto;gap:8px;min-height:220px;border:1px solid var(--border-2);border-radius:12px;background:var(--surface-2);padding:11px}
.sig-card.active{border-color:var(--accent-line)}
.sig-card header{display:flex;gap:8px}.sig-card header input{flex:1}.sig-card header button,.sig-row button{border:1px solid var(--border-2);border-radius:8px;background:var(--surface);color:var(--text-dim);padding:0 8px}
.sig-card header button svg,.sig-row button svg{color:var(--accent)}
.sig-table{grid-column:1/-1;display:grid;align-content:start;gap:7px;overflow:auto}
.sig-row{display:grid;grid-template-columns:34px 180px minmax(0,1fr) 34px 34px;gap:7px;min-height:58px}
.sig-row.active input,.sig-row.active textarea{border-color:var(--accent-line);background:var(--accent-soft)}
.sig-preview{grid-template-columns:minmax(0,1fr) minmax(0,1fr)}
.sig-editor-row{display:flex;gap:8px}.sig-editor-row input{flex:1}
.compose-preview{border:1px solid var(--border-2);border-radius:12px;background:#fff;color:#24262d;padding:28px;font:15px Georgia,serif;overflow:auto}
.preview-signature{margin-top:28px;color:#464a55;font:14px/1.45 ui-monospace,SFMono-Regular,Menlo,monospace}
.preview-signature:before{content:"--";display:block;margin-bottom:4px}
.preview-signature img{max-width:100%;height:auto}
.sig-compact{grid-template-columns:minmax(0,560px);justify-content:center}
.sig-compact .sig-head,.sig-compact-editor{grid-column:1}
.sig-compact-editor footer{justify-content:flex-start}
.sig-command{grid-template-columns:280px minmax(0,1fr)}
.sig-command-list{display:grid;align-content:start;gap:6px}
.sig-command-list button{display:grid;grid-template-columns:28px minmax(0,1fr) auto;align-items:center;gap:8px;border:1px solid var(--border-2);border-radius:10px;background:transparent;color:var(--text-dim);padding:8px;text-align:left}
.sig-command-list button.active{border-color:var(--accent-line);background:var(--accent-soft)}
.sig-command-list kbd{display:grid;place-items:center;width:22px;height:22px;border:1px solid var(--border-2);border-radius:6px;color:var(--accent);font:11px "JetBrains Mono",ui-monospace,monospace}
.sig-command-list span{overflow:hidden;text-overflow:ellipsis;white-space:nowrap;color:var(--text)}
.sig-command-list small{color:var(--text-mut)}
.command-editor footer{justify-content:flex-start;flex-wrap:wrap}
.toast{position:absolute;left:50%;bottom:32px;z-index:50;transform:translateX(-50%);display:flex;align-items:center;gap:8px;border:1px solid var(--accent-line);border-radius:10px;background:var(--surface-2);color:var(--text);padding:10px 15px;font:12.5px "JetBrains Mono",ui-monospace,monospace;box-shadow:var(--shadow-2)}
.toast svg{color:var(--green)}
.toast-enter-active,.toast-leave-active{transition:opacity .18s,transform .18s}.toast-enter-from,.toast-leave-to{opacity:0;transform:translate(-50%,8px)}
@media (max-width: 760px){.sig-panel,.sig-command{grid-template-columns:1fr}.sig-list,.sig-command-list{max-height:170px;overflow:auto}.sig-card-grid{grid-template-columns:1fr}.sig-row{grid-template-columns:34px minmax(0,1fr) 34px 34px}.sig-row textarea{grid-column:2/-1}.compose-preview{display:none}}
</style>
