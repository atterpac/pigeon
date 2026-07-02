import * as Onboarding from '../bindings/github.com/atterpac/pigeon/internal/desktop/onboard/onboarding'
import type { Account as BindingAccount } from '../bindings/github.com/atterpac/pigeon/internal/email/models'

export type SetupMethod = 'appPassword' | 'imap'

export type ConfiguredAccount = {
  id: string
  kind: 'imap'
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
      if (input.method === 'imap') {
        account = await Onboarding.AddIMAPAccount({
          Email: email,
          DisplayName: displayName,
          Password: input.appPassword,
          IMAPHost: input.imapHost.trim(),
          IMAPPort: Number(input.imapPort) || 0,
          SMTPHost: input.smtpHost.trim(),
          SMTPPort: Number(input.smtpPort) || 0,
        })
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
    kind: 'imap',
    email: account.Email,
    name: account.Name,
  }
}
