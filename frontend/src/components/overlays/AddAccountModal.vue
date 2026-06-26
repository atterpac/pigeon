<script setup lang="ts">
// Add-another-account modal — reuses the shell's onboarding form/state so the
// same flow works mid-session as at first boot. Closes on a successful add.
import { onMounted } from 'vue'
import { useMailShell } from '../../composables/useMailShell'
import { PhX } from '@phosphor-icons/vue'

const emit = defineEmits<{ (e: 'close'): void }>()
const s = useMailShell()

onMounted(() => s.resetSetup())

async function submit() {
  if (await s.submitOnboarding()) emit('close')
}
</script>

<template>
  <div class="modal-backdrop" @click.self="emit('close')">
    <section class="account-modal">
      <header class="set-head">
        <h2>Add account</h2>
        <button class="modal-close" type="button" @click="emit('close')">esc <PhX :size="12" /></button>
      </header>
      <form class="setup-form" @submit.prevent="submit">
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

        <label><span>Email</span><input v-model="s.setup.value.email" type="email" autocomplete="email" placeholder="you@example.com" required /></label>
        <label><span>Display name</span><input v-model="s.setup.value.displayName" autocomplete="name" placeholder="Jane Doe" /></label>
        <label v-if="s.setup.value.method === 'appPassword'"><span>App password</span><input v-model="s.setup.value.appPassword" type="password" autocomplete="current-password" placeholder="xxxx xxxx xxxx xxxx" required /></label>

        <p v-if="s.setupError.value" class="setup-error">{{ s.setupError.value }}</p>

        <footer>
          <button class="primary-action" type="submit" :disabled="s.setupBusy.value">
            {{ s.setupBusy.value ? 'Connecting...' : s.setup.value.method === 'google' ? 'Continue with Google' : 'Add account' }}
          </button>
          <span>{{ s.configuredAccounts.value.length }} configured</span>
        </footer>
      </form>
    </section>
  </div>
</template>
