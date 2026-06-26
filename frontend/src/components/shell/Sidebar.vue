<script setup lang="ts">
// Flat sidebar with selectable style + nav-layout presets (R2). Reads shared
// state from the mail shell composable and presentation prefs from settings.
import { computed, nextTick, ref } from 'vue'
import { useMailShell } from '../../composables/useMailShell'
import { useSettings } from '../../composables/useSettings'
import { initials } from '../../mail/format'
import type { Mailbox } from '../../mail/types'
import type { ConfiguredAccount } from '../../onboarding/client'
import AddAccountModal from '../overlays/AddAccountModal.vue'
import { PhArchiveBox, PhCaretDown, PhCheck, PhEnvelope, PhEye, PhEyeSlash, PhNotePencil, PhPaperPlaneTilt, PhPencilSimple, PhPlus, PhStar, PhTag, PhTrash, PhTray, PhUserPlus, PhX } from '@phosphor-icons/vue'

const s = useMailShell()
const settings = useSettings()

// Account switcher / add / remove.
const accountMenuOpen = ref(false)
const addAccountOpen = ref(false)
const removingAccountId = ref<string | null>(null)

function switchAccount(acc: ConfiguredAccount) {
  accountMenuOpen.value = false
  if (s.account.value?.id !== acc.id) void s.bootMailbox(acc)
}
function openAddAccount() {
  accountMenuOpen.value = false
  addAccountOpen.value = true
}
async function confirmRemoveAccount(id: string) {
  removingAccountId.value = null
  accountMenuOpen.value = false
  await s.removeAccount(id)
}

// Group headings are noise in the plain/rail layouts.
const showHeads = computed(() => settings.navLayout !== 'plain' && settings.navLayout !== 'rail')
const showIcons = computed(() => settings.navLayout === 'icons')
const showAccounts = computed(() => settings.navLayout === 'accounts')
const canCrud = computed(() => !!s.client.value?.createMailbox)
const visibleMailboxes = computed(() => s.mailboxes.value.filter((mailbox) => !settings.hiddenMailboxIds.includes(mailbox.id)))
const hiddenMailboxes = computed(() => s.mailboxes.value.filter((mailbox) => settings.hiddenMailboxIds.includes(mailbox.id)))

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
function hideMailbox(id: string) {
  if (!settings.hiddenMailboxIds.includes(id)) settings.hiddenMailboxIds.push(id)
  if (s.activeMailbox.value === id) {
    const fallback = visibleMailboxes.value.find((mailbox) => mailbox.id !== id)
    if (fallback) void s.openMailbox(fallback.id)
  }
}
function showMailbox(id: string) {
  settings.hiddenMailboxIds = settings.hiddenMailboxIds.filter((mailboxId) => mailboxId !== id)
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
    <div class="account-wrap">
      <button class="account" type="button" @click="accountMenuOpen = !accountMenuOpen">
        <span class="avatar sm">{{ s.account.value ? initials({ name: s.account.value.name, addr: s.account.value.email }) : 'me' }}</span>
        <span class="acctcol">
          <b>{{ s.account.value?.name || 'Account' }}</b>
          <small>{{ s.account.value?.email }}</small>
        </span>
        <PhCaretDown class="chev" :size="12" />
      </button>

      <template v-if="accountMenuOpen">
        <div class="menu-scrim" @click="accountMenuOpen = false; removingAccountId = null" />
        <div class="account-menu">
          <p class="menu-head">Accounts</p>
          <div v-for="acc in s.configuredAccounts.value" :key="acc.id" class="menu-row">
            <button class="menu-item" :class="{ active: s.account.value?.id === acc.id }" type="button" @click="switchAccount(acc)">
              <span class="avatar sm">{{ initials({ name: acc.name, addr: acc.email }) }}</span>
              <span class="acctcol"><b>{{ acc.name || acc.email }}</b><small>{{ acc.email }}</small></span>
              <PhCheck v-if="s.account.value?.id === acc.id" :size="13" class="menu-check" />
            </button>
            <template v-if="removingAccountId === acc.id">
              <button class="folder-mini danger" type="button" title="Confirm remove" @click="confirmRemoveAccount(acc.id)"><PhCheck :size="13" /></button>
              <button class="folder-mini" type="button" title="Cancel" @click="removingAccountId = null"><PhX :size="13" /></button>
            </template>
            <button v-else class="folder-mini" type="button" title="Remove account" @click="removingAccountId = acc.id"><PhTrash :size="13" /></button>
          </div>
          <button class="menu-add" type="button" @click="openAddAccount"><PhUserPlus :size="15" /> Add account</button>
        </div>
      </template>
    </div>

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
      <div v-for="mailbox in visibleMailboxes" :key="mailbox.id" class="navrow">
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
          <div v-else class="navactions">
            <button class="folder-mini" type="button" title="Hide folder" @click="hideMailbox(mailbox.id)"><PhEyeSlash :size="13" /></button>
            <template v-if="isUserFolder(mailbox)">
              <button class="folder-mini" type="button" title="Rename folder" @click="startRename(mailbox)"><PhPencilSimple :size="13" /></button>
              <button class="folder-mini" type="button" title="Delete folder" @click="pendingDelete = mailbox.id"><PhTrash :size="13" /></button>
            </template>
          </div>
        </template>
      </div>
      <form v-if="creating" class="folder-edit create" @submit.prevent="submitCreate">
        <input v-model="newName" placeholder="New folder name" @keydown.esc="creating = false" @blur="submitCreate" />
        <button type="submit" class="folder-mini" title="Create"><PhCheck :size="13" /></button>
      </form>
    </nav>

    <template v-if="hiddenMailboxes.length">
      <p v-if="showHeads" class="grouphead">Hidden</p>
      <nav class="navgroup">
        <div v-for="mailbox in hiddenMailboxes" :key="mailbox.id" class="navrow">
          <button class="navitem muted" type="button" :title="`Show ${mailbox.name}`" @click="showMailbox(mailbox.id)">
            <span v-if="showIcons" class="navicon"><component :is="mailboxIcon(mailbox)" :size="14" /></span>
            <span v-else-if="settings.navLayout === 'rail'" class="navicon">{{ glyph(mailbox) }}</span>
            <span class="navlabel">{{ mailbox.name }}</span>
          </button>
          <div class="navactions">
            <button class="folder-mini" type="button" title="Show folder" @click="showMailbox(mailbox.id)"><PhEye :size="13" /></button>
          </div>
        </div>
      </nav>
    </template>

    <p v-if="showHeads" class="grouphead">Labels</p>
    <nav class="navgroup">
      <button v-for="label in s.labels.value" :key="label.id" class="navitem label" type="button" :title="label.name" @click="openLabel(label.name)">
        <span v-if="showIcons" class="navicon label-icon"><PhTag :size="14" /></span>
        <span v-else class="swatch" :style="{ backgroundColor: label.swatch }" />
        <span class="navlabel">{{ label.name }}</span>
        <span class="dot soft">{{ label.count }}</span>
      </button>
    </nav>

    <AddAccountModal v-if="addAccountOpen" @close="addAccountOpen = false" />
  </aside>
</template>
