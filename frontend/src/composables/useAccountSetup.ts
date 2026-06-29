// app lifecycle + account management: discover accounts, boot a mailbox client,
// onboarding, add/remove accounts, reconcile the active view with the backend
// changefeed. owns boot phase + onboarding form state; shared refs are injected.
import { ref, type Ref } from 'vue'
import { Events } from '@wailsio/runtime'
import { createMailClient } from '../mail/client'
import { applyNotifyPrefs, applyPollInterval } from '../mail/syncSettings'
import {
  createOnboardingClient,
  type ConfiguredAccount,
  type SetupMethod,
} from '../onboarding/client'
import { errorMessage } from '../mail/format'
import type { Account, Label, MailClient, Mailbox } from '../mail/types'
import type { Settings } from './useSettings'

export type AppPhase = 'starting' | 'onboarding' | 'mail'

type AccountSetupDeps = {
  client: Ref<MailClient | null>
  account: Ref<Account | null>
  mailboxes: Ref<Mailbox[]>
  labels: Ref<Label[]>
  activeMailbox: Ref<string>
  status: Ref<string>
  settings: Settings
  openMailbox: (mailboxId: string) => Promise<void>
  reloadList: () => Promise<void>
}

export function useAccountSetup({
  client,
  account,
  mailboxes,
  labels,
  activeMailbox,
  status,
  settings,
  openMailbox,
  reloadList,
}: AccountSetupDeps) {
  const onboarding = createOnboardingClient()
  const appPhase = ref<AppPhase>('starting')
  const configuredAccounts = ref<ConfiguredAccount[]>([])

  // Onboarding form state.
  const setupStatus = ref('checking accounts')
  const setupError = ref('')
  const setupBusy = ref(false)
  const emptySetup = () => ({
    method: 'appPassword' as SetupMethod,
    email: '',
    displayName: '',
    appPassword: '',
    imapHost: '',
    imapPort: '',
    smtpHost: '',
    smtpPort: '',
  })
  const setup = ref(emptySetup())

  let changefeedOff: (() => void) | null = null
  let changefeedPending: ReturnType<typeof setTimeout> | null = null

  async function initializeApp() {
    setupStatus.value = 'checking accounts'
    try {
      configuredAccounts.value = await onboarding.listAccounts()
    } catch (error) {
      appPhase.value = 'onboarding'
      setupStatus.value = 'account setup required'
      setupError.value = errorMessage(error)
      return
    }
    if (!configuredAccounts.value.length) {
      appPhase.value = 'onboarding'
      setupStatus.value = 'account setup required'
      return
    }
    await bootMailbox(configuredAccounts.value[0])
  }
  async function bootMailbox(configuredAccount?: ConfiguredAccount) {
    client.value = await createMailClient(configuredAccount?.id)
    account.value = configuredAccount
      ? accountFromConfigured(configuredAccount)
      : await client.value.getAccount()
    await refreshShell()
    // push saved poll interval now that sync loops are running; backend default
    // applies until this lands.
    if (client.value.source === 'wails') {
      void applyPollInterval(settings.pollIntervalSeconds)
      void applyNotifyPrefs(settings.notify)
      subscribeChangefeed()
    }
    appPhase.value = 'mail'
  }
  // reconciles the active view when the backend store changes. events are
  // best-effort hints, so we coalesce bursts and refetch rather than apply
  // individual ids. registered once; survives account switches.
  function subscribeChangefeed() {
    if (changefeedOff) return
    changefeedOff = Events.On('store:change', (ev: { data?: { account?: string } }) => {
      // ignore changes for other accounts.
      if (ev.data?.account && account.value && ev.data.account !== account.value.id) return
      if (changefeedPending) return // already scheduled; coalesce the burst
      changefeedPending = setTimeout(() => {
        changefeedPending = null
        void reloadList()
      }, 250)
    })
  }
  // tear down the changefeed subscription + pending timer.
  function teardownChangefeed() {
    changefeedOff?.()
    changefeedOff = null
    if (changefeedPending) {
      clearTimeout(changefeedPending)
      changefeedPending = null
    }
  }
  async function submitOnboarding(): Promise<boolean> {
    setupError.value = ''
    const email = setup.value.email.trim()
    if (!email) {
      setupError.value = 'Email address is required.'
      return false
    }
    if (setup.value.method === 'appPassword' && !setup.value.appPassword.trim()) {
      setupError.value = 'App password is required.'
      return false
    }
    if (setup.value.method === 'imap') {
      if (!setup.value.imapHost.trim()) {
        setupError.value = 'IMAP server is required.'
        return false
      }
      if (!setup.value.appPassword.trim()) {
        setupError.value = 'Password is required.'
        return false
      }
    }
    setupBusy.value = true
    setupStatus.value = 'verifying account'
    try {
      const added = await onboarding.addAccount(setup.value)
      configuredAccounts.value = [
        added,
        ...configuredAccounts.value.filter((item) => item.id !== added.id),
      ]
      setup.value.appPassword = ''
      await bootMailbox(added)
      return true
    } catch (error) {
      setupError.value = errorMessage(error)
      setupStatus.value = 'setup did not finish'
      return false
    } finally {
      setupBusy.value = false
    }
  }
  // clear the setup form to add another account mid-session.
  function resetSetup() {
    setup.value = emptySetup()
    setupError.value = ''
    setupStatus.value = ''
  }
  // forget an account (credentials + local store) and re-home the active view
  // if the current account was removed.
  async function removeAccount(id: string) {
    await onboarding.removeAccount(id)
    configuredAccounts.value = await onboarding.listAccounts()
    if (account.value?.id !== id) return
    if (configuredAccounts.value.length) {
      await bootMailbox(configuredAccounts.value[0])
    } else {
      client.value = null
      account.value = null
      appPhase.value = 'onboarding'
      setupStatus.value = 'account setup required'
    }
  }
  function accountFromConfigured(configuredAccount: ConfiguredAccount): Account {
    return {
      id: configuredAccount.id,
      email: configuredAccount.email,
      name: configuredAccount.name || configuredAccount.email,
    }
  }
  async function refreshShell() {
    if (!client.value) return
    mailboxes.value = await client.value.listMailboxes()
    labels.value = await client.value.listLabels()
    const nextMailbox =
      mailboxes.value.find((mailbox) => mailbox.id === activeMailbox.value)?.id ??
      mailboxes.value.find((mailbox) => mailbox.role === 'inbox')?.id ??
      mailboxes.value[0]?.id ??
      ''
    if (nextMailbox) await openMailbox(nextMailbox)
    status.value = client.value.source === 'wails' ? 'synced from local store' : 'using mock data'
  }

  return {
    appPhase,
    configuredAccounts,
    setupStatus,
    setupError,
    setupBusy,
    setup,
    initializeApp,
    bootMailbox,
    submitOnboarding,
    resetSetup,
    removeAccount,
    refreshShell,
    teardownChangefeed,
  }
}
