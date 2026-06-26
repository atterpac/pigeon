<script setup lang="ts">
// Right pane (persistent): compose form when composing, otherwise the selected
// thread with its reply panel, otherwise an empty placeholder.
import { useMailShell } from '../../composables/useMailShell'
import { formatDate, initials, participantLine, renderEmailHtml } from '../../mail/format'
import MarkdownEditor from '../editor/MarkdownEditor.vue'

const s = useMailShell()
</script>

<template>
  <section class="reading-pane">
    <!-- Compose -->
    <form v-if="s.composeOpen.value" class="compose-card" @submit.prevent="s.sendDraft()">
      <header><h1>New message</h1><span>esc to discard</span></header>
      <label><span>to</span><em v-for="to in s.draft.value.to" :key="to.addr">{{ to.name || to.addr }}</em><input v-model="s.recipientInput.value" placeholder="Add recipients..." /></label>
      <label><span>subject</span><input v-model="s.draft.value.subject" placeholder="Subject" /></label>
      <MarkdownEditor
        v-model:body="s.draft.value.body"
        variant="compose"
        placeholder="Write a message..."
        :status="s.status.value"
        :reset-key="s.draft.value.id"
        :show-discard="true"
        :autofocus="true"
        @send="s.sendDraft()"
        @attach="s.attachMock()"
        @discard="s.discardDraft()"
      />
      <div v-if="s.draft.value.attachments.length" class="attachment-row">
        <button v-for="attachment in s.draft.value.attachments" :key="attachment.filename" type="button" @click="s.draft.value.attachments = s.draft.value.attachments.filter((item) => item.filename !== attachment.filename)">{{ attachment.filename }} <small>×</small></button>
      </div>
    </form>

    <!-- Thread -->
    <template v-else-if="s.selectedThread.value">
      <header class="thread-header">
        <div class="thread-title"><h1>{{ s.selectedThread.value.subject }}</h1><p>{{ s.selectedThread.value.messageCount }} messages · {{ participantLine(s.selectedThread.value) }}</p></div>
        <div class="thread-actions">
          <button class="ghost-button" type="button" @click="s.toggleStar()">Star</button>
          <button class="ghost-button" type="button" @click="s.archiveThread()">Archive <kbd>e</kbd></button>
          <button class="ghost-button" type="button" @click="s.snoozeThread()">Snooze <kbd>s</kbd></button>
          <button class="ghost-button" type="button" @click="s.toggleRead()">Unread <kbd>u</kbd></button>
        </div>
      </header>
      <div class="thread-messages">
        <article v-for="message in s.threadMessages.value" :key="message.id">
          <span class="avatar">{{ initials(message.from) }}</span>
          <div>
            <header><strong>{{ message.from.name || message.from.addr }}</strong><span>{{ message.from.addr }}</span><time>{{ formatDate(message.date) }}</time></header>
            <div v-if="message.expanded" class="message-body" @click="message.expanded = false">
              <iframe v-if="message.html" class="email-html-frame" sandbox="allow-popups allow-popups-to-escape-sandbox" referrerpolicy="no-referrer" :srcdoc="renderEmailHtml(message.html)" />
              <template v-else><p v-for="paragraph in message.body" :key="paragraph">{{ paragraph }}</p></template>
            </div>
            <p v-else class="snippet" @click="message.expanded = true">{{ message.snippet }}</p>
          </div>
        </article>
      </div>
      <footer class="reply-panel" :class="{ expanded: s.replyExpanded.value }">
        <button class="reply-expand-toggle" type="button" @click="s.replyExpanded.value = !s.replyExpanded.value">{{ s.replyExpanded.value ? '⌄' : '⌃' }}</button>
        <div class="reply-tabs">
          <button :class="{ active: s.replyMode.value === 'reply' }" type="button" @click="s.prepareReply('reply')">Reply</button>
          <button :class="{ active: s.replyMode.value === 'replyAll' }" type="button" @click="s.prepareReply('replyAll')">Reply all</button>
          <button :class="{ active: s.replyMode.value === 'forward' }" type="button" @click="s.prepareReply('forward')">Forward</button>
          <span>{{ s.replyMode.value === 'forward' ? 'forward draft' : `to ${s.selectedThread.value.from.name || s.selectedThread.value.from.addr}` }}</span>
        </div>
        <MarkdownEditor
          v-model:body="s.draft.value.body"
          variant="reply"
          placeholder="Write your reply...  (⌘↵ to send)"
          :status="s.status.value"
          :reset-key="s.draft.value.id"
          :expanded="s.replyExpanded.value"
          @send="s.sendDraft()"
          @attach="s.attachMock()"
        />
      </footer>
    </template>

    <!-- Empty -->
    <div v-else class="reading-empty">
      <p>Select a conversation to read.</p>
    </div>
  </section>
</template>
