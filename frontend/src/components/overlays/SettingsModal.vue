<script setup lang="ts">
// Sidebar-tabs settings modal. Appearance drives the theme registry live; the
// Layout tab exposes the R2 view presets (persisted switch points). Other tabs
// are light placeholders for now.
import { ref } from 'vue'
import { useSettings } from '../../composables/useSettings'
import { useMailShell } from '../../composables/useMailShell'
import { themesByPack } from '../../theme/themes'

const emit = defineEmits<{ (e: 'close'): void }>()
const settings = useSettings()
const s = useMailShell()
const packs = themesByPack()

const categories = [
  { id: 'general', label: 'General', icon: '⚙' },
  { id: 'accounts', label: 'Accounts', icon: '✉' },
  { id: 'appearance', label: 'Appearance', icon: '◐' },
  { id: 'layout', label: 'Layout & Views', icon: '▦' },
  { id: 'keybindings', label: 'Keybindings', icon: '⌘' },
  { id: 'notifications', label: 'Notifications', icon: '🔔' },
  { id: 'privacy', label: 'Privacy', icon: '🔒' },
] as const
const active = ref<typeof categories[number]['id']>('appearance')

const composeOptions = ['centered', 'docked', 'side', 'fullscreen', 'minimal', 'split'] as const
const sidebarOptions = ['flat', 'cards', 'compact', 'outline', 'header', 'airy'] as const
const navOptions = ['grouped', 'plain', 'icons', 'counts', 'rail', 'accounts'] as const
const settingsLayoutOptions = ['sidebar', 'tabs', 'scroll', 'cards', 'palette', 'fullscreen'] as const
const densityOptions = ['comfortable', 'compact'] as const
</script>

<template>
  <div class="modal-backdrop" @click.self="emit('close')">
    <section class="settings-modal">
      <nav class="set-nav">
        <p class="set-title">Settings</p>
        <button
          v-for="category in categories"
          :key="category.id"
          class="set-catitem"
          :class="{ active: active === category.id }"
          type="button"
          @click="active = category.id"
        >
          <span class="set-icon">{{ category.icon }}</span>{{ category.label }}
        </button>
        <span class="set-navfoot">mail · local</span>
      </nav>

      <div class="set-body">
        <header class="set-head">
          <h2>{{ categories.find((c) => c.id === active)?.label }}</h2>
          <button class="modal-close" type="button" @click="emit('close')">esc ✕</button>
        </header>

        <div class="set-content">
          <!-- Appearance -->
          <template v-if="active === 'appearance'">
            <p class="set-section">Theme</p>
            <template v-for="group in packs" :key="group.pack">
              <p class="set-pack">{{ group.pack }}</p>
              <div class="theme-grid">
                <button
                  v-for="theme in group.themes"
                  :key="theme.id"
                  class="theme-tile"
                  :class="{ active: settings.theme === theme.id }"
                  type="button"
                  :style="{ background: theme.tokens['--bg'], borderColor: settings.theme === theme.id ? theme.tokens['--accent'] : theme.tokens['--border-2'] }"
                  @click="settings.theme = theme.id"
                >
                  <span class="theme-swatches">
                    <i :style="{ background: theme.tokens['--accent'] }" />
                    <i :style="{ background: theme.tokens['--green'] }" />
                    <i :style="{ background: theme.tokens['--orange'] }" />
                    <i :style="{ background: theme.tokens['--purple'] }" />
                  </span>
                  <b :style="{ color: theme.tokens['--text'] }">{{ theme.name }}</b>
                  <small :style="{ color: theme.tokens['--text-mut'] }">{{ theme.dark ? 'dark' : 'light' }}</small>
                </button>
              </div>
            </template>

            <p class="set-section">Display</p>
            <label class="set-row">
              <span><b>Relative line numbers</b><small>vim-style gutter in the message list</small></span>
              <button type="button" class="toggle" :class="{ on: settings.relativenumber }" @click="settings.relativenumber = !settings.relativenumber"><i /></button>
            </label>
            <label class="set-row">
              <span><b>Vim mode</b><small>NORMAL/INSERT editing in the composer</small></span>
              <button type="button" class="toggle" :class="{ on: settings.vimMode }" @click="settings.vimMode = !settings.vimMode"><i /></button>
            </label>
            <label class="set-row">
              <span><b>Density</b><small>row + padding scale</small></span>
              <select class="select" v-model="settings.density"><option v-for="opt in densityOptions" :key="opt" :value="opt">{{ opt }}</option></select>
            </label>
          </template>

          <!-- Layout & Views -->
          <template v-else-if="active === 'layout'">
            <p class="set-section">View presets</p>
            <p class="set-note">Defaults match the shipped UI. Non-default presets persist; some render incrementally.</p>
            <label class="set-row"><span><b>Compose surface</b><small>where new messages open</small></span><select class="select" v-model="settings.compose"><option v-for="opt in composeOptions" :key="opt" :value="opt">{{ opt }}</option></select></label>
            <label class="set-row"><span><b>Sidebar style</b></span><select class="select" v-model="settings.sidebarStyle"><option v-for="opt in sidebarOptions" :key="opt" :value="opt">{{ opt }}</option></select></label>
            <label class="set-row"><span><b>Nav layout</b></span><select class="select" v-model="settings.navLayout"><option v-for="opt in navOptions" :key="opt" :value="opt">{{ opt }}</option></select></label>
            <label class="set-row"><span><b>Settings layout</b></span><select class="select" v-model="settings.settingsLayout"><option v-for="opt in settingsLayoutOptions" :key="opt" :value="opt">{{ opt }}</option></select></label>
          </template>

          <!-- Accounts -->
          <template v-else-if="active === 'accounts'">
            <p class="set-section">Connected</p>
            <div class="set-row"><span><b>{{ s.account.value?.name || 'Account' }}</b><small>{{ s.account.value?.email }}</small></span><span class="set-tag">active</span></div>
            <p class="set-note">{{ s.configuredAccounts.value.length }} account(s) configured · stored in the local SQLite store + keyring.</p>
          </template>

          <!-- Keybindings -->
          <template v-else-if="active === 'keybindings'">
            <p class="set-section">Motions</p>
            <p class="set-note">Press <b>?</b> anywhere for the full cheatsheet. Vim mode: {{ settings.vimMode ? 'on' : 'off' }}.</p>
          </template>

          <!-- Placeholder tabs -->
          <template v-else>
            <p class="set-note">Nothing to configure here yet.</p>
          </template>
        </div>
      </div>
    </section>
  </div>
</template>
