<script setup lang="ts">
// Flat sidebar with selectable style + nav-layout presets (R2). Reads shared
// state from the mail shell composable and presentation prefs from settings.
import { computed } from 'vue'
import { useMailShell } from '../../composables/useMailShell'
import { useSettings } from '../../composables/useSettings'
import { initials } from '../../mail/format'
import type { Mailbox } from '../../mail/types'

const s = useMailShell()
const settings = useSettings()

// Group headings are noise in the plain/rail layouts.
const showHeads = computed(() => settings.navLayout !== 'plain' && settings.navLayout !== 'rail')
const showIcons = computed(() => settings.navLayout === 'icons')
const showAccounts = computed(() => settings.navLayout === 'accounts')

function glyph(mailbox: Mailbox) {
  return (mailbox.name[0] ?? '•').toUpperCase()
}
function openLabel(name: string) {
  s.query.value = `label:${name}`
  void s.openSearch()
}
</script>

<template>
  <aside class="sidebar" :class="[`sidebar-${settings.sidebarStyle}`, `nav-${settings.navLayout}`]">
    <button class="account" type="button">
      <span class="avatar sm">{{ s.account.value ? initials({ name: s.account.value.name, addr: s.account.value.email }) : 'me' }}</span>
      <span class="acctcol">
        <b>{{ s.account.value?.name || 'Account' }}</b>
        <small>{{ s.account.value?.email }}</small>
      </span>
      <span class="chev">⌄</span>
    </button>

    <template v-if="showAccounts && s.configuredAccounts.value.length > 1">
      <p v-if="showHeads" class="grouphead">Accounts</p>
      <nav class="navgroup">
        <button v-for="acc in s.configuredAccounts.value" :key="acc.id" class="navitem" :class="{ active: s.account.value?.id === acc.id }" type="button" @click="s.bootMailbox(acc)">
          <span class="navicon">{{ (acc.name || acc.email)[0]?.toUpperCase() }}</span>
          <span class="navlabel">{{ acc.name || acc.email }}</span>
        </button>
      </nav>
    </template>

    <button class="composebtn" type="button" @click="s.compose()"><span class="navlabel">Compose</span> <kbd>c</kbd></button>

    <p v-if="showHeads" class="grouphead">Folders</p>
    <nav class="navgroup">
      <button
        v-for="mailbox in s.mailboxes.value"
        :key="mailbox.id"
        class="navitem"
        :class="{ active: !s.searchActive.value && s.activeMailbox.value === mailbox.id }"
        type="button"
        :title="mailbox.name"
        @click="s.openMailbox(mailbox.id)"
      >
        <span v-if="showIcons" class="navicon">{{ glyph(mailbox) }}</span>
        <span class="navlabel">{{ mailbox.name }}</span>
        <span v-if="mailbox.unread" class="dot">{{ mailbox.unread }}</span>
      </button>
    </nav>

    <p v-if="showHeads" class="grouphead">Labels</p>
    <nav class="navgroup">
      <button v-for="label in s.labels.value" :key="label.id" class="navitem label" type="button" :title="label.name" @click="openLabel(label.name)">
        <span class="swatch" :style="{ backgroundColor: label.swatch }" />
        <span class="navlabel">{{ label.name }}</span>
        <span class="dot soft">{{ label.count }}</span>
      </button>
    </nav>
  </aside>
</template>
