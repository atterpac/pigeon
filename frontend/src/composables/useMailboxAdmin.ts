// folder (mailbox) CRUD over the client's optional methods. each refreshes the
// sidebar list and re-homes the active view when needed.
import type { Ref } from 'vue'
import type { MailClient, Mailbox } from '../mail/types'

type MailboxAdminDeps = {
  client: Ref<MailClient | null>
  mailboxes: Ref<Mailbox[]>
  activeMailbox: Ref<string>
  status: Ref<string>
  openMailbox: (mailboxId: string) => Promise<void>
}

export function useMailboxAdmin({
  client,
  mailboxes,
  activeMailbox,
  status,
  openMailbox,
}: MailboxAdminDeps) {
  async function createMailbox(name: string) {
    if (!client.value?.createMailbox) return
    const created = await client.value.createMailbox(name.trim())
    mailboxes.value = await client.value.listMailboxes()
    status.value = `created ${created.name}`
    await openMailbox(created.id)
  }
  async function renameMailbox(id: string, newName: string) {
    if (!client.value?.renameMailbox) return
    const renamed = await client.value.renameMailbox(id, newName.trim())
    mailboxes.value = await client.value.listMailboxes()
    status.value = `renamed to ${renamed.name}`
    if (activeMailbox.value === id) await openMailbox(renamed.id)
  }
  async function setMailboxIcon(id: string, icon: string, weight: string, color: string) {
    if (!client.value?.setMailboxIcon) return
    await client.value.setMailboxIcon(id, icon, weight, color)
    mailboxes.value = await client.value.listMailboxes()
  }
  async function deleteMailbox(id: string) {
    if (!client.value?.deleteMailbox) return
    await client.value.deleteMailbox(id)
    mailboxes.value = await client.value.listMailboxes()
    status.value = 'folder deleted'
    if (activeMailbox.value === id) {
      const fallback =
        mailboxes.value.find((mailbox) => mailbox.role === 'inbox')?.id ?? mailboxes.value[0]?.id
      if (fallback) await openMailbox(fallback)
    }
  }
  return { createMailbox, renameMailbox, setMailboxIcon, deleteMailbox }
}
