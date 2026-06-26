<script setup lang="ts">
// Middle pane: conversation list with Today/Earlier sections, category tabs,
// and a relative-line-number gutter (gated by settings.relativenumber).
// Search input now lives in the command line; this pane just renders results.
import { computed } from 'vue'
import { categoryTabs, useMailShell } from '../../composables/useMailShell'
import { useSettings } from '../../composables/useSettings'
import { formatDate, labelFor } from '../../mail/format'
import type { Conversation } from '../../mail/types'

const s = useMailShell()
const settings = useSettings()

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
function selectAndOpen(conversation: Conversation) {
  s.selectedIndex.value = indexById.value.get(conversation.id) ?? 0
  void s.openThread(conversation.id)
}
</script>

<template>
  <section class="list-pane" :class="{ 'relno-on': settings.relativenumber }">
    <header class="list-header">
      <p v-if="s.searchActive.value"><strong>{{ s.searchResults.value.length }}</strong> results</p>
      <p v-else><strong>{{ s.filteredConversations.value.length }}</strong> · {{ s.unreadCount.value }} unread</p>
      <button class="searchbtn" type="button" @click="s.openCommand('search')">⌕</button>
    </header>

    <nav v-if="!s.searchActive.value" class="category-tabs" aria-label="Inbox categories">
      <button v-for="tab in categoryTabs" :key="tab.id" :class="{ active: s.activeCategory.value === tab.id }" type="button" @click="s.selectCategory(tab.id)">
        {{ tab.label }} <span>{{ s.categoryCounts.value[tab.id] }}</span>
      </button>
    </nav>

    <div class="scroll-region">
      <template v-if="s.searchActive.value">
        <article
          v-for="conversation in s.searchResults.value"
          :key="conversation.id"
          class="email-row"
          :class="{ unread: conversation.unread, selected: isCurrent(conversation) }"
          @click="selectAndOpen(conversation)"
        >
          <span v-if="settings.relativenumber" class="relno" :class="{ cur: isCurrent(conversation) }">{{ rel(conversation) }}</span>
          <button class="star" :class="{ active: conversation.starred }" type="button" @click.stop="s.toggleStar(conversation)" aria-label="Star"><svg viewBox="0 0 256 256"><path d="M128 24l31.5 63.8 70.4 10.2-50.9 49.7 12 70.1L128 184.6 65 217.8l12-70.1-50.9-49.7 70.4-10.2L128 24z" /></svg></button>
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
              @click="selectAndOpen(conversation)"
            >
              <span v-if="settings.relativenumber" class="relno" :class="{ cur: isCurrent(conversation) }">{{ rel(conversation) }}</span>
              <button class="star" :class="{ active: conversation.starred }" type="button" @click.stop="s.toggleStar(conversation)" aria-label="Star"><svg viewBox="0 0 256 256"><path d="M128 24l31.5 63.8 70.4 10.2-50.9 49.7 12 70.1L128 184.6 65 217.8l12-70.1-50.9-49.7 70.4-10.2L128 24z" /></svg></button>
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
