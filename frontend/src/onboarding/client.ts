import * as Onboarding from '@/bindings/github.com/atterpac/email/cmd/email/onboarding'
import type { Account as BindingAccount } from '@/bindings/github.com/atterpac/email/pkg/email/models'

export type SetupMethod = 'google' | 'appPassword'

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
}

export function createOnboardingClient() {
  return {
    async listAccounts() {
      return (await Onboarding.ListAccounts()).map(normalizeAccount)
    },
    async addAccount(input: AddAccountInput) {
      const email = input.email.trim()
      const displayName = input.displayName.trim()
      const account = input.method === 'google'
        ? await Onboarding.AddGoogleAccount(email, displayName)
        : await Onboarding.AddAppPasswordAccount(email, displayName, input.appPassword)

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
