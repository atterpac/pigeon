<script setup lang="ts">
// Middle pane: conversation list with Today/Earlier sections, category tabs,
// and a folded-in search input. Selecting a row loads it into the reading pane.
import { nextTick, ref, watch } from 'vue'
import { categoryTabs, useMailShell } from '../../composables/useMailShell'
import { formatDate, labelFor } from '../../mail/format'
import type { Conversation } from '../../mail/types'

const s = useMailShell()
const searchInput = ref<HTMLInputElement | null>(null)

watch(s.searchActive, (active) => {
  if (active) nextTick(() => { searchInput.value?.focus(); searchInput.value?.select() })
})

function selectAndOpen(conversation: Conversation) {
  s.selectedIndex.value = s.activeList.value.findIndex((item) => item.id === conversation.id)
  void s.openThread(conversation.id)
}

function onSearchKeydown(event: KeyboardEvent) {
  if (event.key === 'ArrowDown') { event.preventDefault(); s.moveSelection(1) }
  else if (event.key === 'ArrowUp') { event.preventDefault(); s.moveSelection(-1) }
  else if (event.key === 'Enter') { event.preventDefault(); void s.openThread() }
  else if (event.key === 'Escape') { event.preventDefault(); s.closeSearch() }
}
</script>

<template>
  <section class="list-pane">
    <header v-if="s.searchActive.value" class="search-header">
      <label><span>⌕</span><input ref="searchInput" v-model="s.query.value" placeholder="Search — try from:github is:unread" @keydown="onSearchKeydown" /><kbd>esc</kbd></label>
    </header>
    <header v-else class="list-header">
      <p><strong>{{ s.filteredConversations.value.length }}</strong> · {{ s.unreadCount.value }} unread</p>
      <button class="searchbtn" type="button" @click="s.openSearch()">⌕</button>
    </header>

    <nav v-if="!s.searchActive.value" class="category-tabs" aria-label="Inbox categories">
      <button v-for="tab in categoryTabs" :key="tab.id" :class="{ active: s.activeCategory.value === tab.id }" type="button" @click="s.selectCategory(tab.id)">
        {{ tab.label }} <span>{{ s.categoryCounts.value[tab.id] }}</span>
      </button>
    </nav>

    <div class="scroll-region">
      <template v-if="s.searchActive.value">
        <p class="section-label">{{ s.searchResults.value.length }} results</p>
        <article
          v-for="(conversation, index) in s.searchResults.value"
          :key="conversation.id"
          class="email-row"
          :class="{ unread: conversation.unread, selected: index === s.selectedIndex.value }"
          @click="selectAndOpen(conversation)"
        >
          <button class="star" :class="{ active: conversation.starred }" type="button" @click.stop="s.toggleStar(conversation)" aria-label="Star"><svg viewBox="0 0 256 256"><path d="M128 24l31.5 63.8 70.4 10.2-50.9 49.7 12 70.1L128 184.6 65 217.8l12-70.1-50.9-49.7 70.4-10.2L128 24z" /></svg></button>
          <div class="row-main">
            <div class="row-top"><strong>{{ conversation.from.name || conversation.from.addr }}</strong><time>{{ formatDate(conversation.lastAt) }}</time></div>
            <div class="subject">{{ conversation.subject }}</div>
            <div class="snippet-line">{{ conversation.snippet }}</div>
          </div>
        </article>
      </template>

      <template v-else>
        <p v-if="!s.filteredConversations.value.length" class="empty-state">No conversations in {{ s.activeCategory.value === 'all' ? 'this mailbox' : s.activeCategory.value }}.</p>
        <template v-if="s.todayConversations.value.length">
          <p class="section-label">Today</p>
          <article
            v-for="conversation in s.todayConversations.value"
            :key="conversation.id"
            class="email-row"
            :class="{ unread: conversation.unread, selected: s.selectedConversation.value?.id === conversation.id }"
            @click="selectAndOpen(conversation)"
          >
            <button class="star" :class="{ active: conversation.starred }" type="button" @click.stop="s.toggleStar(conversation)" aria-label="Star"><svg viewBox="0 0 256 256"><path d="M128 24l31.5 63.8 70.4 10.2-50.9 49.7 12 70.1L128 184.6 65 217.8l12-70.1-50.9-49.7 70.4-10.2L128 24z" /></svg></button>
            <div class="row-main">
              <div class="row-top"><strong>{{ conversation.from.name || conversation.from.addr }}</strong><time>{{ formatDate(conversation.lastAt) }}</time></div>
              <div class="subject">{{ conversation.subject }}<em v-if="labelFor(conversation, s.labels.value)" :style="{ background: labelFor(conversation, s.labels.value)?.bg, color: labelFor(conversation, s.labels.value)?.fg }">{{ labelFor(conversation, s.labels.value)?.name }}</em></div>
              <div class="snippet-line">{{ conversation.snippet }}</div>
            </div>
          </article>
        </template>
        <template v-if="s.earlierConversations.value.length">
          <p class="section-label">Earlier</p>
          <article
            v-for="conversation in s.earlierConversations.value"
            :key="conversation.id"
            class="email-row"
            :class="{ unread: conversation.unread, selected: s.selectedConversation.value?.id === conversation.id }"
            @click="selectAndOpen(conversation)"
          >
            <button class="star" :class="{ active: conversation.starred }" type="button" @click.stop="s.toggleStar(conversation)" aria-label="Star"><svg viewBox="0 0 256 256"><path d="M128 24l31.5 63.8 70.4 10.2-50.9 49.7 12 70.1L128 184.6 65 217.8l12-70.1-50.9-49.7 70.4-10.2L128 24z" /></svg></button>
            <div class="row-main">
              <div class="row-top"><strong>{{ conversation.from.name || conversation.from.addr }}</strong><time>{{ formatDate(conversation.lastAt) }}</time></div>
              <div class="subject">{{ conversation.subject }}<em v-if="labelFor(conversation, s.labels.value)" :style="{ background: labelFor(conversation, s.labels.value)?.bg, color: labelFor(conversation, s.labels.value)?.fg }">{{ labelFor(conversation, s.labels.value)?.name }}</em></div>
              <div class="snippet-line">{{ conversation.snippet }}</div>
            </div>
          </article>
        </template>
      </template>
    </div>
  </section>
</template>
