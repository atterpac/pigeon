import * as Onboarding from '@/bindings/github.com/atterpac/email/cmd/email/onboarding'
import type { Account as BindingAccount } from '@/bindings/github.com/atterpac/email/pkg/email/models'

export type SetupMethod = 'google' | 'appPassword' | 'imap'

export type ConfiguredAccount = {
  id: string
  kind: 'imap' | 'gmail'
  email: string
  name: string
}

export type AddAccountInput = {
  method: SetupMethod
  email: string
  displayName: string
  appPassword: string
  imapHost: string
  imapPort: string
  smtpHost: string
  smtpPort: string
}

export function createOnboardingClient() {
  return {
    async listAccounts() {
      return (await Onboarding.ListAccounts()).map(normalizeAccount)
    },
    async addAccount(input: AddAccountInput) {
      const email = input.email.trim()
      const displayName = input.displayName.trim()
      let account
      if (input.method === 'google') {
        account = await Onboarding.AddGoogleAccount(email, displayName)
      } else if (input.method === 'imap') {
        account = await Onboarding.AddIMAPAccount(
          email, displayName, input.appPassword,
          input.imapHost.trim(), Number(input.imapPort) || 0,
          input.smtpHost.trim(), Number(input.smtpPort) || 0,
        )
      } else {
        account = await Onboarding.AddAppPasswordAccount(email, displayName, input.appPassword)
      }
      return normalizeAccount(account)
    },
    async removeAccount(id: string) {
      await Onboarding.RemoveAccount(id)
    },
  }
}

function normalizeAccount(account: BindingAccount): ConfiguredAccount {
  return {
    id: account.ID,
    kind: account.Kind === 1 ? 'gmail' : 'imap',
    email: account.Email,
    name: account.Name,
  }
}
