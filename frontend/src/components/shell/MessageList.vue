<script setup lang="ts">
// Middle pane: conversation list with Today/Earlier sections, category tabs,
// and a relative-line-number gutter (gated by settings.relativenumber).
// Search input now lives in the command line; this pane just renders results.
import { computed, nextTick, ref, watch } from 'vue'
import { categoryTabs, useMailShell } from '../../composables/useMailShell'
import { useSettings } from '../../composables/useSettings'
import { formatDate, labelFor } from '../../mail/format'
import type { Conversation } from '../../mail/types'
import { PhMagnifyingGlass, PhStar } from '@phosphor-icons/vue'

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
  s.selectedIndex.value = rowIndex(conversation)
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
  <section class="list-pane" :class="{ 'relno-on': settings.relativenumber, focused: s.focusPane.value === 'list' }" @pointerdown="s.focusList()">
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
        <article
          v-for="conversation in s.searchResults.value"
          :key="conversation.id"
          class="email-row"
          :class="{ unread: conversation.unread, selected: isCurrent(conversation) }"
          :data-list-index="rowIndex(conversation)"
          @click="selectAndOpen(conversation)"
        >
          <span v-if="settings.relativenumber" class="relno" :class="{ cur: isCurrent(conversation) }">{{ rel(conversation) }}</span>
          <button class="star" :class="{ active: conversation.starred }" type="button" @click.stop="s.toggleStar(conversation)" aria-label="Star"><PhStar :size="15" :weight="conversation.starred ? 'fill' : 'regular'" /></button>
          <div class="row-main">
            <div class="row-top"><strong>{{ conversation.from.name || conversation.from.addr }}</strong><time>{{ formatDate(conversation.lastAt) }}</time></div>
            <div class="subject"><span>{{ conversation.subject }}</span></div>
            <div class="snippet-line">{{ conversation.snippet }}</div>
          </div>
        </article>
      </template>

      <template v-else>
        <p v-if="!s.filteredConversations.value.length" class="empty-state">No conversations in {{ s.activeCategory.value === 'all' ? 'this mailbox' : s.activeCategory.value }}.</p>
        <template v-for="section in [{ label: 'Today', rows: s.todayConversations.value }, { label: 'Earlier', rows: s.earlierConversations.value }]" :key="section.label">
          <template v-if="section.rows.length">
            <p class="section-label">{{ section.label }}</p>
            <article
              v-for="conversation in section.rows"
              :key="conversation.id"
              class="email-row"
              :class="{ unread: conversation.unread, selected: isCurrent(conversation) }"
              :data-list-index="rowIndex(conversation)"
              @click="selectAndOpen(conversation)"
            >
              <span v-if="settings.relativenumber" class="relno" :class="{ cur: isCurrent(conversation) }">{{ rel(conversation) }}</span>
              <button class="star" :class="{ active: conversation.starred }" type="button" @click.stop="s.toggleStar(conversation)" aria-label="Star"><PhStar :size="15" :weight="conversation.starred ? 'fill' : 'regular'" /></button>
              <div class="row-main">
                <div class="row-top"><strong>{{ conversation.from.name || conversation.from.addr }}</strong><time>{{ formatDate(conversation.lastAt) }}</time></div>
                <div class="subject"><span>{{ conversation.subject }}</span><em v-if="labelFor(conversation, s.labels.value)" :style="{ background: labelFor(conversation, s.labels.value)?.bg, color: labelFor(conversation, s.labels.value)?.fg }">{{ labelFor(conversation, s.labels.value)?.name }}</em></div>
                <div class="snippet-line">{{ conversation.snippet }}</div>
              </div>
            </article>
          </template>
        </template>
      </template>
    </div>
  </section>
</template>
