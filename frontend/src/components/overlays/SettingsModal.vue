<script setup lang="ts">
// Settings modal whose own shape is a live R2 preset (settings.settingsLayout):
//   sidebar  — left nav + body (default)
//   tabs     — horizontal tab bar on top
//   fullscreen — same as sidebar but fills the viewport
//   cards    — category tiles on top, body below
//   palette  — searchable command-palette nav
//   scroll   — no nav; every category stacked on one page
import { computed, ref } from 'vue'
import { useSettings } from '../../composables/useSettings'
import SettingsContent from './SettingsContent.vue'
import { PhSlidersHorizontal, PhEnvelope, PhPalette, PhSquaresFour, PhKeyboard, PhBell, PhLock, PhX } from '@phosphor-icons/vue'

const emit = defineEmits<{ (e: 'close'): void }>()
const settings = useSettings()

const categories = [
  { id: 'general', label: 'General', icon: PhSlidersHorizontal },
  { id: 'accounts', label: 'Accounts', icon: PhEnvelope },
  { id: 'appearance', label: 'Appearance', icon: PhPalette },
  { id: 'layout', label: 'Layout & Views', icon: PhSquaresFour },
  { id: 'keybindings', label: 'Keybindings', icon: PhKeyboard },
  { id: 'notifications', label: 'Notifications', icon: PhBell },
  { id: 'privacy', label: 'Privacy', icon: PhLock },
]
const active = ref('appearance')
const filter = ref('')

const isScroll = computed(() => settings.settingsLayout === 'scroll')
const visibleCategories = computed(() => {
  const q = filter.value.trim().toLowerCase()
  return q ? categories.filter((c) => c.label.toLowerCase().includes(q)) : categories
})
const activeLabel = computed(() => categories.find((c) => c.id === active.value)?.label)
</script>

<template>
  <div class="modal-backdrop" @click.self="emit('close')">
    <section class="settings-modal" :class="`settings-${settings.settingsLayout}`">
      <!-- Nav (hidden in scroll layout) -->
      <nav v-if="!isScroll" class="set-nav">
        <p class="set-title">Settings</p>
        <input v-if="settings.settingsLayout === 'palette'" v-model="filter" class="set-search" placeholder="Search settings…" spellcheck="false" />
        <button
          v-for="category in visibleCategories"
          :key="category.id"
          class="set-catitem"
          :class="{ active: active === category.id }"
          type="button"
          @click="active = category.id"
        >
          <component :is="category.icon" class="set-icon" :size="16" /><span class="set-catlabel">{{ category.label }}</span>
        </button>
        <span class="set-navfoot">mail · local</span>
      </nav>

      <div class="set-body">
        <header class="set-head">
          <h2>{{ isScroll ? 'Settings' : activeLabel }}</h2>
          <button class="modal-close" type="button" @click="emit('close')">esc <PhX :size="12" /></button>
        </header>

        <div class="set-content">
          <template v-if="isScroll">
            <section v-for="category in categories" :key="category.id" class="set-scroll-cat">
              <p class="set-scroll-head"><component :is="category.icon" :size="16" /> {{ category.label }}</p>
              <SettingsContent :category="category.id" />
            </section>
          </template>
          <SettingsContent v-else :category="active" />
        </div>
      </div>
    </section>
  </div>
</template>
