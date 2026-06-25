import { createMockMailClient } from './mockClient'
import type { MailClient } from './types'

export async function createMailClient(): Promise<MailClient> {
  return createMockMailClient()
}
