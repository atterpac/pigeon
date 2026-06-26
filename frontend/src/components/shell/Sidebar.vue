<script setup lang="ts">
// Flat sidebar: account block, Compose button, grouped Folders + Labels with
// dot-number unread counts. Reads shared state from the mail shell composable.
import { useMailShell } from '../../composables/useMailShell'
import { initials } from '../../mail/format'

const s = useMailShell()

function openLabel(name: string) {
  s.query.value = `label:${name}`
  void s.openSearch()
}
</script>

<template>
  <aside class="sidebar">
    <button class="account" type="button">
      <span class="avatar sm">{{ s.account.value ? initials({ name: s.account.value.name, addr: s.account.value.email }) : 'me' }}</span>
      <span class="acctcol">
        <b>{{ s.account.value?.name || 'Account' }}</b>
        <small>{{ s.account.value?.email }}</small>
      </span>
      <span class="chev">⌄</span>
    </button>

    <button class="composebtn" type="button" @click="s.compose()">Compose <kbd>c</kbd></button>

    <p class="grouphead">Folders</p>
    <nav class="navgroup">
      <button
        v-for="mailbox in s.mailboxes.value"
        :key="mailbox.id"
        class="navitem"
        :class="{ active: !s.searchActive.value && s.activeMailbox.value === mailbox.id }"
        type="button"
        @click="s.openMailbox(mailbox.id)"
      >
        <span class="navlabel">{{ mailbox.name }}</span>
        <span v-if="mailbox.unread" class="dot">{{ mailbox.unread }}</span>
      </button>
    </nav>

    <p class="grouphead">Labels</p>
    <nav class="navgroup">
      <button v-for="label in s.labels.value" :key="label.id" class="navitem label" type="button" @click="openLabel(label.name)">
        <span class="swatch" :style="{ backgroundColor: label.swatch }" />
        <span class="navlabel">{{ label.name }}</span>
        <span class="dot soft">{{ label.count }}</span>
      </button>
    </nav>
  </aside>
</template>
