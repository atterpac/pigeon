<script setup lang="ts">
// Account-setup phase, extracted from App.vue. Re-tokened, otherwise unchanged.
import { useMailShell } from '../composables/useMailShell'
import { PhArrowRight } from '@phosphor-icons/vue'

const emit = defineEmits<{ (e: 'open-sandbox'): void }>()
const s = useMailShell()
</script>

<template>
  <main class="setup-shell">
    <section class="setup-panel">
      <div class="setup-copy">
        <span class="setup-mark">mail</span>
        <h1>Set up your account</h1>
        <p>Connect an inbox to start syncing mail locally.</p>
        <dl>
          <div><dt>Storage</dt><dd>SQLite mail store and system keyring credentials</dd></div>
          <div><dt>Sync</dt><dd>Inbox starts immediately after the account is verified</dd></div>
          <div><dt>Status</dt><dd>{{ s.setupStatus.value }}</dd></div>
        </dl>
      </div>

      <form class="setup-form" @submit.prevent="s.submitOnboarding()">
        <div class="setup-methods" role="radiogroup" aria-label="Account type">
          <button type="button" :class="{ active: s.setup.value.method === 'google' }" @click="s.setup.value.method = 'google'">
            <strong>Google</strong>
            <span>OAuth browser sign-in</span>
          </button>
          <button type="button" :class="{ active: s.setup.value.method === 'appPassword' }" @click="s.setup.value.method = 'appPassword'">
            <strong>IMAP</strong>
            <span>Gmail app password</span>
          </button>
        </div>

        <label>
          <span>Email</span>
          <input v-model="s.setup.value.email" type="email" autocomplete="email" placeholder="you@example.com" required />
        </label>
        <label>
          <span>Display name</span>
          <input v-model="s.setup.value.displayName" autocomplete="name" placeholder="Jane Doe" />
        </label>
        <label v-if="s.setup.value.method === 'appPassword'">
          <span>App password</span>
          <input v-model="s.setup.value.appPassword" type="password" autocomplete="current-password" placeholder="xxxx xxxx xxxx xxxx" required />
        </label>

        <p v-if="s.setupError.value" class="setup-error">{{ s.setupError.value }}</p>

        <footer>
          <button class="primary-action" type="submit" :disabled="s.setupBusy.value">
            {{ s.setupBusy.value ? 'Connecting...' : s.setup.value.method === 'google' ? 'Continue with Google' : 'Add account' }}
          </button>
          <span>{{ s.configuredAccounts.value.length }} configured</span>
        </footer>
        <button type="button" class="sandbox-link" @click="emit('open-sandbox')">Open sandbox <PhArrowRight :size="12" /></button>
      </form>
    </section>
  </main>
</template>
