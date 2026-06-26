import { createWailsMailClient } from './wailsClient'
import type { MailClient } from './types'

export async function createMailClient(accountId?: string): Promise<MailClient> {
  return createWailsMailClient(accountId)
}
