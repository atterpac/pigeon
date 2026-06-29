// local + server search + saved-searches list. `query`/`searchActive` are
// owned by the shell; everything search-specific lives here.
import { nextTick, ref, type Ref } from 'vue'
import type { Conversation, MailClient } from '../mail/types'
import type { Settings } from './useSettings'
import { errorMessage } from '../mail/format'

type MailSearchDeps = {
  client: Ref<MailClient | null>
  query: Ref<string>
  searchResults: Ref<Conversation[]>
  searchActive: Ref<boolean>
  selectedIndex: Ref<number>
  focusPane: Ref<'list' | 'thread'>
  status: Ref<string>
  settings: Settings
}

export function useMailSearch({
  client,
  query,
  searchResults,
  searchActive,
  selectedIndex,
  focusPane,
  status,
  settings,
}: MailSearchDeps) {
  // true during a server-side search round-trip.
  const serverSearching = ref(false)

  async function runSearch() {
    if (!client.value) return
    searchResults.value = await client.value.searchConversations(query.value)
    selectedIndex.value = 0
  }
  // recipient autocomplete: ranked address-book matches for a prefix.
  // [] when the client predates the feature or the query is empty.
  async function searchContacts(prefix: string) {
    const trimmed = prefix.trim()
    if (!trimmed || !client.value?.searchContacts) return []
    try {
      return await client.value.searchContacts(trimmed)
    } catch (error) {
      console.warn('contact search failed', error)
      return []
    }
  }
  async function openSearch() {
    searchActive.value = true
    await runSearch()
    await nextTick()
  }
  function closeSearch() {
    searchActive.value = false
    selectedIndex.value = 0
    focusPane.value = 'list'
  }
  // reach mail not synced locally: run the query on the server and merge new
  // hits into the local results. one network round-trip.
  async function searchServer() {
    if (!client.value?.searchServer) {
      status.value = 'server search not supported'
      return
    }
    if (!searchActive.value) await openSearch()
    if (!query.value.trim()) {
      status.value = 'type a query first'
      return
    }
    serverSearching.value = true
    status.value = 'searching server…'
    try {
      const hits = await client.value.searchServer(query.value)
      const seen = new Set(searchResults.value.map((conversation) => conversation.id))
      const fresh = hits.filter((conversation) => !seen.has(conversation.id))
      searchResults.value = [...searchResults.value, ...fresh].sort(
        (left, right) => Date.parse(right.lastAt) - Date.parse(left.lastAt),
      )
      status.value = fresh.length
        ? `found ${fresh.length} more on server`
        : 'no additional results on server'
    } catch (error) {
      status.value = `server search failed: ${errorMessage(error)}`
    } finally {
      serverSearching.value = false
    }
  }
  // ── Saved searches ─────────────────────────────────────────────────────
  function saveSearch(name?: string) {
    const q = query.value.trim()
    if (!q) {
      status.value = 'nothing to save'
      return
    }
    const label = (name || q).trim()
    const entry = { name: label, query: q }
    const index = settings.savedSearches.findIndex((item) => item.name === label)
    if (index >= 0) settings.savedSearches[index] = entry
    else settings.savedSearches.push(entry)
    status.value = `saved search “${label}”`
  }
  function runSavedSearch(savedQuery: string) {
    query.value = savedQuery
    void openSearch()
  }
  function removeSavedSearch(name: string) {
    settings.savedSearches = settings.savedSearches.filter((item) => item.name !== name)
  }

  return {
    serverSearching,
    runSearch,
    searchContacts,
    openSearch,
    closeSearch,
    searchServer,
    saveSearch,
    runSavedSearch,
    removeSavedSearch,
  }
}
