<script setup lang="ts">
// Flat sidebar with selectable style + nav-layout presets. Reads shared
// state from the mail shell composable and presentation prefs from settings.
import { computed, nextTick, ref } from 'vue'
import { useMailShell } from '../../composables/useMailShell'
import { useSettings } from '../../composables/useSettings'
import { pigeonLogoForTheme } from '../../theme/logo'
import type { Mailbox } from '../../mail/types'
import { folderIconComponent, type FolderEdit, type FolderIconPref, type FolderIconWeight } from '../../mail/folderIcons'
import type { ConfiguredAccount } from '../../onboarding/client'
import AddAccountModal from '../overlays/AddAccountModal.vue'
import FolderIconPicker from './FolderIconPicker.vue'
import { PhArchiveBox, PhCaretDown, PhCheck, PhClock, PhDotsThree, PhEnvelope, PhEyeSlash, PhMagnifyingGlass, PhNotePencil, PhPaperPlaneTilt, PhPencilSimple, PhPlus, PhSmiley, PhStar, PhTrash, PhTray, PhX } from '@phosphor-icons/vue'

const s = useMailShell()
const settings = useSettings()
const logoSrc = computed(() => pigeonLogoForTheme(settings.theme))

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
function toggleAccountMenu() {
  if (settings.navCollapsed) {
    settings.navCollapsed = false
    accountMenuOpen.value = true
    return
  }
  accountMenuOpen.value = !accountMenuOpen.value
}
async function confirmRemoveAccount(id: string) {
  removingAccountId.value = null
  accountMenuOpen.value = false
  await s.removeAccount(id)
}

// Group headings are noise in the plain/rail layouts.
const showHeads = computed(() => !settings.navCollapsed && settings.navLayout !== 'plain' && settings.navLayout !== 'rail')
const showAccounts = computed(() => settings.navLayout === 'accounts')
const canCrud = computed(() => !!s.client.value?.createMailbox)
const visibleMailboxes = computed(() => s.mailboxes.value.filter((mailbox) => !settings.hiddenMailboxIds.includes(mailbox.id)))

// Folder CRUD UI state (inline inputs — webview-friendly, no native dialogs).
const creating = ref(false)
const newName = ref('')
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
  if (!name) return
  await s.createMailbox(name)
  // Offer to pick an icon right after the folder appears.
  const created = s.mailboxes.value.find((mailbox) => mailbox.name === name)
  if (created) openIconPicker(created)
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
function glyph(mailbox: Mailbox) {
  return (mailbox.name[0] ?? '•').toUpperCase()
}
// User-assigned icon (persisted on the mailbox) wins; otherwise role/name.
function mailboxIcon(mailbox: Mailbox) {
  const custom = folderIconComponent(mailbox.icon)
  if (custom) return custom
  const name = mailbox.name.toLowerCase()
  if (mailbox.role === 'inbox' || name.includes('inbox')) return PhTray
  if (mailbox.role === 'sent' || name.includes('sent')) return PhPaperPlaneTilt
  if (mailbox.role === 'archive' || name.includes('archive')) return PhArchiveBox
  if (mailbox.role === 'trash' || name.includes('trash') || name.includes('bin')) return PhTrash
  if (name.includes('star')) return PhStar
  return PhEnvelope
}
function mailboxIconWeight(mailbox: Mailbox): FolderIconWeight {
  return (mailbox.iconWeight as FolderIconWeight) || 'regular'
}
function mailboxIconColor(mailbox: Mailbox) {
  return mailbox.iconColor || ''
}
function mailboxPref(mailbox: Mailbox): FolderIconPref | null {
  if (!mailbox.icon) return null
  return { icon: mailbox.icon, weight: mailboxIconWeight(mailbox), color: mailbox.iconColor || '' }
}

// Per-folder context menu (Rename / Change icon / Hide / Delete).
const menuFor = ref<string | null>(null)
function toggleMenu(id: string) { menuFor.value = menuFor.value === id ? null : id }

// Folder editor (rename + icon + weight + color in one modal).
const iconPickerFor = ref<Mailbox | null>(null)
function openIconPicker(mailbox: Mailbox) {
  menuFor.value = null
  iconPickerFor.value = mailbox
}
async function assignIcon(result: FolderEdit) {
  const mailbox = iconPickerFor.value
  iconPickerFor.value = null
  if (!mailbox) return
  const { name, icon, weight, color } = result
  // Set the icon first so a rename that changes the mailbox id carries it over.
  await s.setMailboxIcon(mailbox.id, icon, weight, color)
  if (name && name !== mailbox.name && isUserFolder(mailbox)) await s.renameMailbox(mailbox.id, name)
}
function accountSlug(acc: Pick<ConfiguredAccount, 'name' | 'email'> | null | undefined) {
	const base = acc?.email?.split('@')[0] || acc?.name || 'account'
	return base.toLowerCase().replace(/[^a-z0-9._-]+/g, '-').replace(/^-+|-+$/g, '') || 'account'
}
function accountInitial(acc: Pick<ConfiguredAccount, 'name' | 'email'> | null | undefined) {
	const base = acc?.name?.trim() || acc?.email?.trim() || '?'
	return (base[0] ?? '?').toUpperCase()
}
</script>

<template>
  <aside class="sidebar" :class="[`sidebar-${settings.sidebarStyle}`, `nav-${settings.navLayout}`]">
    <div class="sidebar-brand">
      <img class="sidebar-brand-logo" :src="logoSrc" alt="" aria-hidden="true" />
    </div>

    <button
      class="nav-edge-toggle"
      type="button"
      :title="settings.navCollapsed ? 'Expand sidebar' : 'Collapse sidebar'"
      :aria-label="settings.navCollapsed ? 'Expand sidebar' : 'Collapse sidebar'"
      :aria-expanded="!settings.navCollapsed"
      @click="settings.navCollapsed = !settings.navCollapsed"
    >
      <span aria-hidden="true">{{ settings.navCollapsed ? '::' : '::' }}</span>
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

    <nav class="navgroup">
      <button class="navitem" type="button" title="Snoozed" :class="{ active: s.snoozedActive.value }" :aria-current="s.snoozedActive.value ? 'page' : undefined" @click="s.openSnoozed()">
        <span class="navicon"><PhClock :size="16" /></span>
        <span class="navlabel">Snoozed</span>
      </button>
    </nav>

    <div v-if="showHeads" class="grouphead grouphead-row">
      <span>Folders</span>
      <button v-if="canCrud" class="grouphead-action" type="button" title="New folder" aria-label="New folder" @click="startCreate"><PhPlus :size="13" /></button>
    </div>
    <nav class="navgroup">
      <div v-for="mailbox in visibleMailboxes" :key="mailbox.id" class="navrow">
        <button
            class="navitem"
            :class="{ active: !s.searchActive.value && s.activeMailbox.value === mailbox.id }"
            type="button"
            :title="mailbox.name"
            :aria-current="!s.searchActive.value && !s.snoozedActive.value && s.activeMailbox.value === mailbox.id ? 'page' : undefined"
            @click="s.openMailbox(mailbox.id)"
          >
            <span v-if="settings.navLayout === 'rail'" class="navicon">{{ glyph(mailbox) }}</span>
            <span v-else class="navicon" :style="{ color: mailboxIconColor(mailbox) || undefined }"><component :is="mailboxIcon(mailbox)" :size="16" :weight="mailboxIconWeight(mailbox)" /></span>
            <span class="navlabel">{{ mailbox.name }}</span>
            <span v-if="pendingDelete !== mailbox.id && mailbox.unread" class="dot">{{ mailbox.unread }}</span>
          </button>
          <div v-if="pendingDelete === mailbox.id" class="navactions confirm">
            <button class="folder-mini danger" type="button" title="Confirm delete" @click="confirmDelete(mailbox.id)"><PhCheck :size="13" /></button>
            <button class="folder-mini" type="button" title="Cancel" @click="pendingDelete = null"><PhX :size="13" /></button>
          </div>
          <div v-else class="navactions">
            <button class="folder-mini" type="button" title="Folder actions" :aria-label="`Actions for ${mailbox.name}`" aria-haspopup="menu" :aria-expanded="menuFor === mailbox.id" @click="toggleMenu(mailbox.id)"><PhDotsThree :size="14" /></button>
          </div>
          <template v-if="menuFor === mailbox.id">
            <div class="menu-scrim" @click="menuFor = null" />
            <div class="folder-menu" role="menu" :aria-label="`${mailbox.name} actions`">
              <button type="button" @click="openIconPicker(mailbox)"><PhPencilSimple :size="14" /> Rename</button>
              <button type="button" @click="openIconPicker(mailbox)"><PhSmiley :size="14" /> Change icon</button>
              <button type="button" @click="menuFor = null; hideMailbox(mailbox.id)"><PhEyeSlash :size="14" /> Hide</button>
              <template v-if="isUserFolder(mailbox)">
                <hr />
                <button type="button" class="danger" @click="menuFor = null; pendingDelete = mailbox.id"><PhTrash :size="14" /> Delete</button>
              </template>
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
      <button v-for="label in s.labels.value" :key="label.id" class="navitem label" type="button" :title="label.name" :class="{ active: !s.searchActive.value && s.activeMailbox.value === label.id }" :aria-current="!s.searchActive.value && !s.snoozedActive.value && s.activeMailbox.value === label.id ? 'page' : undefined" @click="s.openMailbox(label.id)">
        <span class="navicon"><span class="swatch" :style="{ backgroundColor: label.swatch }" /></span>
        <span class="navlabel">{{ label.name }}</span>
        <span class="dot soft">{{ label.count }}</span>
      </button>
    </nav>

    <template v-if="settings.savedSearches.length">
      <p v-if="showHeads" class="grouphead">Searches</p>
      <nav class="navgroup">
        <div v-for="saved in settings.savedSearches" :key="saved.name" class="navrow">
          <button class="navitem" type="button" :title="saved.query" @click="s.runSavedSearch(saved.query)">
            <span class="navicon"><PhMagnifyingGlass :size="15" /></span>
            <span class="navlabel">{{ saved.name }}</span>
          </button>
          <div class="navactions">
            <button class="folder-mini" type="button" :title="`Remove ${saved.name}`" :aria-label="`Remove saved search ${saved.name}`" @click="s.removeSavedSearch(saved.name)"><PhX :size="13" /></button>
          </div>
        </div>
      </nav>
    </template>

    <div class="account-wrap sidebar-account-bottom">
      <button class="account account-command-trigger" type="button" aria-label="Account menu" aria-haspopup="menu" :aria-expanded="accountMenuOpen" @click="toggleAccountMenu">
        <span class="account-initial">{{ accountInitial(s.account.value) }}</span>
        <span class="cmdpath">
          <b>{{ accountSlug(s.account.value) }}</b>
        </span>
        <PhCaretDown class="chev" :size="12" />
      </button>

      <template v-if="accountMenuOpen">
        <div class="menu-scrim" @click="accountMenuOpen = false; removingAccountId = null" />
        <div class="account-menu account-path-menu">
          <p class="menu-head">~/accounts</p>
          <div v-for="acc in s.configuredAccounts.value" :key="acc.id" class="menu-row">
            <button class="menu-item path-menu-item" :class="{ active: s.account.value?.id === acc.id }" type="button" @click="switchAccount(acc)">
              <span class="path-main">
                <b>{{ accountSlug(acc) }}</b>
              </span>
              <span class="path-meta">{{ acc.email }}</span>
              <PhCheck v-if="s.account.value?.id === acc.id" :size="13" class="menu-check" />
            </button>
            <template v-if="removingAccountId === acc.id">
              <button class="folder-mini danger" type="button" title="Confirm remove" @click="confirmRemoveAccount(acc.id)"><PhCheck :size="13" /></button>
              <button class="folder-mini" type="button" title="Cancel" @click="removingAccountId = null"><PhX :size="13" /></button>
            </template>
            <button v-else class="folder-mini" type="button" title="Remove account" @click="removingAccountId = acc.id"><PhTrash :size="13" /></button>
          </div>
          <button class="menu-add path-menu-add" type="button" @click="openAddAccount"><PhPlus :size="15" /> ~/add-account</button>
        </div>
      </template>
    </div>

    <AddAccountModal v-if="addAccountOpen" @close="addAccountOpen = false" />
    <FolderIconPicker
      v-if="iconPickerFor"
      :folder-name="iconPickerFor.name"
      :initial="mailboxPref(iconPickerFor)"
      @assign="assignIcon"
      @close="iconPickerFor = null"
    />
  </aside>
</template>
