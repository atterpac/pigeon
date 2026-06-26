<script setup lang="ts">
import { defineAsyncComponent, onMounted, ref } from 'vue'
import { useMailShell } from './composables/useMailShell'
import OnboardingView from './onboarding/OnboardingView.vue'
import MailShell from './components/shell/MailShell.vue'
import { PhArrowLeft } from '@phosphor-icons/vue'

const s = useMailShell()
const sandboxOpen = ref(false)
const devToolsEnabled = import.meta.env.DEV || import.meta.env.VITE_ENABLE_SANDBOX === 'true'
const Sandbox = defineAsyncComponent(() => import('./sandbox/Sandbox.vue'))

onMounted(() => { void s.initializeApp() })
</script>

<template>
  <main v-if="s.appPhase.value === 'starting'" class="setup-shell">
    <section class="setup-panel compact">
      <span class="setup-mark">mail</span>
      <h1>Starting email</h1>
      <p>{{ s.setupStatus.value }}</p>
    </section>
  </main>

  <main v-else-if="devToolsEnabled && sandboxOpen" class="sandbox-shell">
    <button class="sandbox-exit" type="button" @click="sandboxOpen = false"><PhArrowLeft :size="12" /> exit sandbox</button>
    <Sandbox />
  </main>

  <OnboardingView v-else-if="s.appPhase.value === 'onboarding'" :dev-tools="devToolsEnabled" @open-sandbox="sandboxOpen = true" />

  <MailShell v-else :dev-tools="devToolsEnabled" @open-sandbox="sandboxOpen = true" />
</template>

<style scoped>
.sandbox-shell { position: relative; width: 100vw; height: 100vh; overflow: hidden; background: var(--bg) }
.sandbox-exit { position: fixed; top: 14px; left: 14px; z-index: 10; color: var(--text-dim); background: var(--surface-2); border: 1px solid var(--border-2); border-radius: 8px; padding: 7px 12px; font: 11px "JetBrains Mono", ui-monospace, monospace; backdrop-filter: blur(4px) }
.sandbox-exit:hover { color: var(--accent); border-color: var(--accent-line) }
</style>
