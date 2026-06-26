<script setup lang="ts">
// The body of one settings category. Split out of SettingsModal so the `scroll`
// settings layout can stack every category, while the other layouts render one.
import { computed } from 'vue'
import { useSettings } from '../../composables/useSettings'
import { useMailShell } from '../../composables/useMailShell'
import { getTheme, themesByPack } from '../../theme/themes'

defineProps<{ category: string }>()
const settings = useSettings()
const s = useMailShell()
const packs = themesByPack()
const activeTheme = computed(() => getTheme(settings.theme))
const hiddenMailboxes = computed(() => s.mailboxes.value.filter((mailbox) => settings.hiddenMailboxIds.includes(mailbox.id)))
function unhideMailbox(id: string) {
  settings.hiddenMailboxIds = settings.hiddenMailboxIds.filter((mailboxId) => mailboxId !== id)
}

const composeOptions = ['centered', 'docked', 'side', 'fullscreen', 'minimal', 'split'] as const
const sidebarOptions = ['flat', 'cards', 'compact', 'outline', 'header', 'airy'] as const
const navOptions = ['grouped', 'plain', 'icons', 'counts', 'rail', 'accounts'] as const
const settingsLayoutOptions = [
  'sidebar',
  'tabs',
  'scroll',
  'cards',
  'palette',
  'fullscreen',
] as const
const densityOptions = ['comfortable', 'compact'] as const
</script>

<template>
  <!-- Appearance -->
  <template v-if="category === 'appearance'">
    <p class="set-section">Theme</p>
    <label class="set-row theme-select-row">
      <span>
        <b>{{ activeTheme.pack }} · {{ activeTheme.name }}</b>
        <small>{{ activeTheme.dark ? 'dark' : 'light' }}</small>
      </span>
      <select class="select theme-select" v-model="settings.theme">
        <optgroup v-for="group in packs" :key="group.pack" :label="group.pack">
          <option v-for="theme in group.themes" :key="theme.id" :value="theme.id">
            {{ theme.name }} · {{ theme.dark ? 'dark' : 'light' }}
          </option>
        </optgroup>
      </select>
    </label>
    <div
      class="theme-preview"
      :style="{
        background: activeTheme.tokens['--bg'],
        borderColor: activeTheme.tokens['--border-2'],
      }"
    >
      <span class="theme-swatches">
        <i :style="{ background: activeTheme.tokens['--accent'] }" />
        <i :style="{ background: activeTheme.tokens['--green'] }" />
        <i :style="{ background: activeTheme.tokens['--orange'] }" />
        <i :style="{ background: activeTheme.tokens['--purple'] }" />
        <i :style="{ background: activeTheme.tokens['--cyan'] }" />
        <i :style="{ background: activeTheme.tokens['--red'] }" />
      </span>
      <span :style="{ color: activeTheme.tokens['--text'] }">Aa</span>
      <small :style="{ color: activeTheme.tokens['--text-mut'] }">{{ activeTheme.id }}</small>
    </div>

    <p class="set-section">Display</p>
    <label class="set-row">
      <span><b>Relative line numbers</b><small>vim-style gutter in the message list</small></span>
      <button
        type="button"
        class="toggle"
        :class="{ on: settings.relativenumber }"
        @click="settings.relativenumber = !settings.relativenumber"
      >
        <i />
      </button>
    </label>
    <label class="set-row">
      <span><b>Vim mode</b><small>NORMAL/INSERT editing in the composer</small></span>
      <button
        type="button"
        class="toggle"
        :class="{ on: settings.vimMode }"
        @click="settings.vimMode = !settings.vimMode"
      >
        <i />
      </button>
    </label>
    <label class="set-row">
      <span><b>Density</b><small>row + padding scale</small></span>
      <select class="select" v-model="settings.density">
        <option v-for="opt in densityOptions" :key="opt" :value="opt">{{ opt }}</option>
      </select>
    </label>
  </template>

  <!-- Layout & Views -->
  <template v-else-if="category === 'layout'">
    <p class="set-section">View presets</p>
    <p class="set-note">All presets are live — pick one and the UI updates immediately.</p>
    <label class="set-row"
      ><span><b>Compose surface</b><small>where new messages open</small></span
      ><select class="select" v-model="settings.compose">
        <option v-for="opt in composeOptions" :key="opt" :value="opt">{{ opt }}</option>
      </select></label
    >
    <label class="set-row"
      ><span><b>Sidebar style</b></span
      ><select class="select" v-model="settings.sidebarStyle">
        <option v-for="opt in sidebarOptions" :key="opt" :value="opt">{{ opt }}</option>
      </select></label
    >
    <label class="set-row"
      ><span><b>Nav layout</b></span
      ><select class="select" v-model="settings.navLayout">
        <option v-for="opt in navOptions" :key="opt" :value="opt">{{ opt }}</option>
      </select></label
    >
    <label class="set-row"
      ><span><b>Settings layout</b><small>this modal's own shape</small></span
      ><select class="select" v-model="settings.settingsLayout">
        <option v-for="opt in settingsLayoutOptions" :key="opt" :value="opt">{{ opt }}</option>
      </select></label
    >
  </template>

  <!-- Accounts -->
  <template v-else-if="category === 'accounts'">
    <p class="set-section">Connected</p>
    <div class="set-row">
      <span
        ><b>{{ s.account.value?.name || 'Account' }}</b
        ><small>{{ s.account.value?.email }}</small></span
      ><span class="set-tag">active</span>
    </div>
    <p class="set-note">
      {{ s.configuredAccounts.value.length }} account(s) configured · stored in the local SQLite
      store + keyring.
    </p>

    <p class="set-section">Hidden folders</p>
    <p v-if="!hiddenMailboxes.length" class="set-note">No hidden folders. Hide one from the sidebar (eye icon) to declutter the list.</p>
    <div v-for="mailbox in hiddenMailboxes" :key="mailbox.id" class="set-row">
      <span><b>{{ mailbox.name }}</b><small>hidden from the sidebar</small></span>
      <button class="ghost-button" type="button" @click="unhideMailbox(mailbox.id)">Show</button>
    </div>
  </template>

  <!-- Keybindings -->
  <template v-else-if="category === 'keybindings'">
    <p class="set-section">Motions</p>
    <p class="set-note">
      Press <b>?</b> anywhere for the full cheatsheet. Vim mode:
      {{ settings.vimMode ? 'on' : 'off' }}.
    </p>
  </template>

  <!-- Placeholder tabs -->
  <template v-else>
    <p class="set-note">Nothing to configure here yet.</p>
  </template>
</template>
