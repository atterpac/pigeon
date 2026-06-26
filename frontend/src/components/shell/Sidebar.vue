<script setup lang="ts">
// Flat sidebar with selectable style + nav-layout presets (R2). Reads shared
// state from the mail shell composable and presentation prefs from settings.
import { computed, nextTick, ref } from 'vue'
import { useMailShell } from '../../composables/useMailShell'
import { useSettings } from '../../composables/useSettings'
import { initials } from '../../mail/format'
import type { Mailbox } from '../../mail/types'
import { PhArchiveBox, PhCaretDown, PhCheck, PhEnvelope, PhNotePencil, PhPaperPlaneTilt, PhPencilSimple, PhPlus, PhStar, PhTag, PhTrash, PhTray, PhX } from '@phosphor-icons/vue'

const s = useMailShell()
const settings = useSettings()

// Group headings are noise in the plain/rail layouts.
const showHeads = computed(() => settings.navLayout !== 'plain' && settings.navLayout !== 'rail')
const showIcons = computed(() => settings.navLayout === 'icons')
const showAccounts = computed(() => settings.navLayout === 'accounts')
const canCrud = computed(() => !!s.client.value?.createMailbox)

// Folder CRUD UI state (inline inputs — webview-friendly, no native dialogs).
const creating = ref(false)
const newName = ref('')
const renamingId = ref<string | null>(null)
const editName = ref('')
const pendingDelete = ref<string | null>(null)

function focusInput(selector: string) {
  nextTick(() => {
    const input = document.querySelector<HTMLInputElement>(selector)
    input?.focus()
    input?.select()
  })
}

// System folders (inbox/sent/…) carry a role; user folders don't and are editable.
function isUserFolder(mailbox: Mailbox) {
  return canCrud.value && !mailbox.role
}
function startCreate() {
  creating.value = true
  newName.value = ''
  focusInput('.folder-edit.create input')
}
async function submitCreate() {
  const name = newName.value.trim()
  creating.value = false
  if (name) await s.createMailbox(name)
}
function startRename(mailbox: Mailbox) {
  pendingDelete.value = null
  renamingId.value = mailbox.id
  editName.value = mailbox.name
  focusInput('.folder-edit.rename input')
}
async function submitRename() {
  const id = renamingId.value
  const name = editName.value.trim()
  renamingId.value = null
  if (id && name) await s.renameMailbox(id, name)
}
async function confirmDelete(id: string) {
  pendingDelete.value = null
  await s.deleteMailbox(id)
}

function glyph(mailbox: Mailbox) {
  return (mailbox.name[0] ?? '•').toUpperCase()
}
function mailboxIcon(mailbox: Mailbox) {
  const name = mailbox.name.toLowerCase()
  if (mailbox.role === 'inbox' || name.includes('inbox')) return PhTray
  if (mailbox.role === 'sent' || name.includes('sent')) return PhPaperPlaneTilt
  if (mailbox.role === 'archive' || name.includes('archive')) return PhArchiveBox
  if (mailbox.role === 'trash' || name.includes('trash') || name.includes('bin')) return PhTrash
  if (name.includes('star')) return PhStar
  return PhEnvelope
}
function openLabel(name: string) {
  s.query.value = `label:${name}`
  void s.openSearch()
}
</script>

<template>
  <aside class="sidebar" :class="[`sidebar-${settings.sidebarStyle}`, `nav-${settings.navLayout}`]">
    <button class="account" type="button">
      <span class="avatar sm">{{ s.account.value ? initials({ name: s.account.value.name, addr: s.account.value.email }) : 'me' }}</span>
      <span class="acctcol">
        <b>{{ s.account.value?.name || 'Account' }}</b>
        <small>{{ s.account.value?.email }}</small>
      </span>
      <PhCaretDown class="chev" :size="12" />
    </button>

    <template v-if="showAccounts && s.configuredAccounts.value.length > 1">
      <p v-if="showHeads" class="grouphead">Accounts</p>
      <nav class="navgroup">
        <button v-for="acc in s.configuredAccounts.value" :key="acc.id" class="navitem" :class="{ active: s.account.value?.id === acc.id }" type="button" @click="s.bootMailbox(acc)">
          <span class="navicon">{{ (acc.name || acc.email)[0]?.toUpperCase() }}</span>
          <span class="navlabel">{{ acc.name || acc.email }}</span>
        </button>
      </nav>
    </template>

    <button class="composebtn" type="button" @click="s.compose()"><PhNotePencil :size="15" /><span class="navlabel">Compose</span> <kbd>c</kbd></button>

    <div v-if="showHeads" class="grouphead grouphead-row">
      <span>Folders</span>
      <button v-if="canCrud" class="grouphead-action" type="button" title="New folder" @click="startCreate"><PhPlus :size="13" /></button>
    </div>
    <nav class="navgroup">
      <div v-for="mailbox in s.mailboxes.value" :key="mailbox.id" class="navrow">
        <form v-if="renamingId === mailbox.id" class="folder-edit rename" @submit.prevent="submitRename">
          <input v-model="editName" @keydown.esc="renamingId = null" @blur="submitRename" />
          <button type="submit" class="folder-mini" title="Save"><PhCheck :size="13" /></button>
        </form>
        <template v-else>
          <button
            class="navitem"
            :class="{ active: !s.searchActive.value && s.activeMailbox.value === mailbox.id }"
            type="button"
            :title="mailbox.name"
            @click="s.openMailbox(mailbox.id)"
          >
            <span v-if="showIcons" class="navicon"><component :is="mailboxIcon(mailbox)" :size="14" /></span>
            <span v-else-if="settings.navLayout === 'rail'" class="navicon">{{ glyph(mailbox) }}</span>
            <span class="navlabel">{{ mailbox.name }}</span>
            <span v-if="pendingDelete !== mailbox.id && mailbox.unread" class="dot">{{ mailbox.unread }}</span>
          </button>
          <div v-if="pendingDelete === mailbox.id" class="navactions confirm">
            <button class="folder-mini danger" type="button" title="Confirm delete" @click="confirmDelete(mailbox.id)"><PhCheck :size="13" /></button>
            <button class="folder-mini" type="button" title="Cancel" @click="pendingDelete = null"><PhX :size="13" /></button>
          </div>
          <div v-else-if="isUserFolder(mailbox)" class="navactions">
            <button class="folder-mini" type="button" title="Rename folder" @click="startRename(mailbox)"><PhPencilSimple :size="13" /></button>
            <button class="folder-mini" type="button" title="Delete folder" @click="pendingDelete = mailbox.id"><PhTrash :size="13" /></button>
          </div>
        </template>
      </div>
      <form v-if="creating" class="folder-edit create" @submit.prevent="submitCreate">
        <input v-model="newName" placeholder="New folder name" @keydown.esc="creating = false" @blur="submitCreate" />
        <button type="submit" class="folder-mini" title="Create"><PhCheck :size="13" /></button>
      </form>
    </nav>

    <p v-if="showHeads" class="grouphead">Labels</p>
    <nav class="navgroup">
      <button v-for="label in s.labels.value" :key="label.id" class="navitem label" type="button" :title="label.name" @click="openLabel(label.name)">
        <span v-if="showIcons" class="navicon label-icon"><PhTag :size="14" /></span>
        <span v-else class="swatch" :style="{ backgroundColor: label.swatch }" />
        <span class="navlabel">{{ label.name }}</span>
        <span class="dot soft">{{ label.count }}</span>
      </button>
    </nav>
  </aside>
</template>
