<script setup lang="ts">
// Middle pane: conversation list with Today/Earlier sections, category tabs,
// and a relative-line-number gutter (gated by settings.relativenumber).
// Search input now lives in the command line; this pane just renders results.
import { computed, nextTick, ref, watch } from 'vue'
import { categoryTabs, useMailShell } from '../../composables/useMailShell'
import { useSettings } from '../../composables/useSettings'
import type { Conversation } from '../../mail/types'
import { PhBookmarkSimple, PhCloud, PhMagnifyingGlass, PhSpinnerGap, PhX } from '@phosphor-icons/vue'
import EmailRow from './EmailRow.vue'

const s = useMailShell()
const settings = useSettings()
const emit = defineEmits<{ (e: 'open-thread'): void }>()
const scrollRegion = ref<HTMLElement | null>(null)

const indexById = computed(() => {
  const map = new Map<string, number>()
  s.activeList.value.forEach((conversation, index) => map.set(conversation.id, index))
  return map
})
function rel(conversation: Conversation) {
  const index = indexById.value.get(conversation.id) ?? 0
  const delta = index - s.selectedIndex.value
  return delta === 0 ? index + 1 : Math.abs(delta)
}
function isCurrent(conversation: Conversation) {
  return indexById.value.get(conversation.id) === s.selectedIndex.value
}
function rowIndex(conversation: Conversation) {
  return indexById.value.get(conversation.id) ?? 0
}
function selectAndOpen(conversation: Conversation) {
  s.selectConversation(conversation)
  void s.openThread(conversation.id)
  emit('open-thread')
}
// In Visual mode a row click toggles selection (anchored on the clicked row)
// rather than opening the thread.
function toggleSelect(conversation: Conversation) {
  s.selectConversation(conversation)
  s.toggleSelect(conversation.id)
}
function keepSelectionVisible() {
  const row = scrollRegion.value?.querySelector<HTMLElement>(`[data-list-index="${s.selectedIndex.value}"]`)
  row?.scrollIntoView({ block: 'nearest' })
}

watch(
  () => [s.selectedIndex.value, s.activeList.value.length, s.searchActive.value, s.snoozedActive.value, s.activeMailbox.value, s.activeCategory.value],
  () => nextTick(keepSelectionVisible),
  { flush: 'post' },
)
</script>

<template>
  <section class="list-pane" :class="{ 'relno-on': settings.relativenumber, focused: s.focusPane.value === 'list', 'thread-dimmed': s.focusPane.value === 'thread' }" @pointerdown="s.focusList()">
    <header class="list-header">
      <p v-if="s.snoozedActive.value"><strong>{{ s.snoozedItems.value.length }}</strong> snoozed</p>
      <p v-else-if="s.searchActive.value"><strong>{{ s.searchResults.value.length }}</strong> results</p>
      <p v-else><strong>{{ s.filteredConversations.value.length }}</strong> · {{ s.unreadCount.value }} unread</p>
      <span class="header-actions">
        <template v-if="s.searchActive.value">
          <button class="searchbtn" type="button" title="Save this search" @click="s.saveSearch()"><PhBookmarkSimple :size="14" /></button>
          <button class="searchbtn server-search" type="button" title="Search all mail on the server" :disabled="s.serverSearching.value" @click="s.searchServer()">
            <PhSpinnerGap v-if="s.serverSearching.value" :size="14" class="spin" />
            <PhCloud v-else :size="14" />
            <span>Server</span>
          </button>
        </template>
        <button v-else-if="s.snoozedActive.value" class="searchbtn back" type="button" title="Back to mailbox" @click="s.closeSnoozed()"><PhX :size="14" /></button>
        <button v-else class="searchbtn" type="button" @click="s.openCommand('search')"><PhMagnifyingGlass :size="14" /></button>
      </span>
    </header>

    <nav class="mobile-mailboxes" aria-label="Mailboxes">
      <button
        v-for="mailbox in s.mailboxes.value"
        :key="mailbox.id"
        type="button"
        :class="{ active: s.activeMailbox.value === mailbox.id }"
        :aria-current="s.activeMailbox.value === mailbox.id ? 'page' : undefined"
        @click="s.openMailbox(mailbox.id)"
      >
        {{ mailbox.name }} <span v-if="mailbox.unread">{{ mailbox.unread }}</span>
      </button>
    </nav>

    <nav v-if="!s.searchActive.value && !s.snoozedActive.value" class="category-tabs" role="tablist" aria-label="Inbox categories">
      <button v-for="tab in categoryTabs" :key="tab.id" role="tab" :aria-selected="s.activeCategory.value === tab.id" :class="{ active: s.activeCategory.value === tab.id }" type="button" @click="s.selectCategory(tab.id)">
        {{ tab.label }} <span>{{ s.categoryCounts.value[tab.id] }}</span>
      </button>
    </nav>

    <div ref="scrollRegion" class="scroll-region" role="list" :aria-label="s.snoozedActive.value ? 'Snoozed conversations' : s.searchActive.value ? 'Search results' : 'Conversations'">
      <template v-if="s.snoozedActive.value">
        <p v-if="!s.snoozedItems.value.length" class="empty-state">Nothing snoozed. Press <kbd>s</kbd> on a conversation to snooze it.</p>
        <EmailRow
          v-for="conversation in s.snoozedItems.value"
          :key="conversation.id"
          :conversation="conversation"
          :selected="isCurrent(conversation)"
          :relative-number="settings.relativenumber ? rel(conversation) : undefined"
          :data-list-index="rowIndex(conversation)"
          :multi-select="s.visualMode.value"
          :in-selection="s.selectedIds.value.has(conversation.id)"
          @open="selectAndOpen"
          @toggle-star="s.toggleStar"
          @toggle-select="toggleSelect"
        />
      </template>

      <template v-else-if="s.searchActive.value">
        <EmailRow
          v-for="conversation in s.searchResults.value"
          :key="conversation.id"
          :conversation="conversation"
          :selected="isCurrent(conversation)"
          :relative-number="settings.relativenumber ? rel(conversation) : undefined"
          :data-list-index="rowIndex(conversation)"
          :multi-select="s.visualMode.value"
          :in-selection="s.selectedIds.value.has(conversation.id)"
          @open="selectAndOpen"
          @toggle-star="s.toggleStar"
          @toggle-select="toggleSelect"
        />
      </template>

      <template v-else>
        <p v-if="!s.filteredConversations.value.length" class="empty-state">No conversations in {{ s.activeCategory.value === 'all' ? 'this mailbox' : s.activeCategory.value }}.</p>
        <template v-for="section in [{ label: 'Today', rows: s.todayConversations.value }, { label: 'Earlier', rows: s.earlierConversations.value }]" :key="section.label">
          <template v-if="section.rows.length">
            <p class="section-label">{{ section.label }}</p>
            <EmailRow
              v-for="conversation in section.rows"
              :key="conversation.id"
              :conversation="conversation"
              :labels="s.labels.value"
              :selected="isCurrent(conversation)"
              :relative-number="settings.relativenumber ? rel(conversation) : undefined"
              :data-list-index="rowIndex(conversation)"
              :multi-select="s.visualMode.value"
              :in-selection="s.selectedIds.value.has(conversation.id)"
              @open="selectAndOpen"
              @toggle-star="s.toggleStar"
              @toggle-select="toggleSelect"
            />
          </template>
        </template>
      </template>
    </div>
  </section>
</template>
