<script setup lang="ts">
// Middle pane: conversation list with Today/Earlier sections, category tabs,
// and a relative-line-number gutter (gated by settings.relativenumber).
// Search input now lives in the command line; this pane just renders results.
import { computed, nextTick, ref, watch } from 'vue'
import { categoryTabs, useMailShell } from '../../composables/useMailShell'
import { useSettings } from '../../composables/useSettings'
import type { Conversation } from '../../mail/types'
import { PhMagnifyingGlass } from '@phosphor-icons/vue'
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
function keepSelectionVisible() {
  const row = scrollRegion.value?.querySelector<HTMLElement>(`[data-list-index="${s.selectedIndex.value}"]`)
  row?.scrollIntoView({ block: 'nearest' })
}

watch(
  () => [s.selectedIndex.value, s.activeList.value.length, s.searchActive.value, s.activeMailbox.value, s.activeCategory.value],
  () => nextTick(keepSelectionVisible),
  { flush: 'post' },
)
</script>

<template>
  <section class="list-pane" :class="{ 'relno-on': settings.relativenumber, focused: s.focusPane.value === 'list', 'thread-dimmed': s.focusPane.value === 'thread' }" @pointerdown="s.focusList()">
    <header class="list-header">
      <p v-if="s.searchActive.value"><strong>{{ s.searchResults.value.length }}</strong> results</p>
      <p v-else><strong>{{ s.filteredConversations.value.length }}</strong> · {{ s.unreadCount.value }} unread</p>
      <button class="searchbtn" type="button" @click="s.openCommand('search')"><PhMagnifyingGlass :size="14" /></button>
    </header>

    <nav class="mobile-mailboxes" aria-label="Mailboxes">
      <button
        v-for="mailbox in s.mailboxes.value"
        :key="mailbox.id"
        type="button"
        :class="{ active: s.activeMailbox.value === mailbox.id }"
        @click="s.openMailbox(mailbox.id)"
      >
        {{ mailbox.name }} <span v-if="mailbox.unread">{{ mailbox.unread }}</span>
      </button>
    </nav>

    <nav v-if="!s.searchActive.value" class="category-tabs" aria-label="Inbox categories">
      <button v-for="tab in categoryTabs" :key="tab.id" :class="{ active: s.activeCategory.value === tab.id }" type="button" @click="s.selectCategory(tab.id)">
        {{ tab.label }} <span>{{ s.categoryCounts.value[tab.id] }}</span>
      </button>
    </nav>

    <div ref="scrollRegion" class="scroll-region">
      <template v-if="s.searchActive.value">
        <EmailRow
          v-for="conversation in s.searchResults.value"
          :key="conversation.id"
          :conversation="conversation"
          :selected="isCurrent(conversation)"
          :relative-number="settings.relativenumber ? rel(conversation) : undefined"
          :data-list-index="rowIndex(conversation)"
          @open="selectAndOpen"
          @toggle-star="s.toggleStar"
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
              show-label
              :data-list-index="rowIndex(conversation)"
              @open="selectAndOpen"
              @toggle-star="s.toggleStar"
            />
          </template>
        </template>
      </template>
    </div>
  </section>
</template>
