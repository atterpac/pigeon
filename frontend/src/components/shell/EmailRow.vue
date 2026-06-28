<script setup lang="ts">
import { PhCheckCircle, PhCircle, PhClock, PhStar } from '@phosphor-icons/vue'
import { computed } from 'vue'
import { formatDate, formatWakeTime, labelFor } from '../../mail/format'
import type { Conversation, Label } from '../../mail/types'

const props = withDefaults(defineProps<{
  conversation: Conversation
  labels?: Label[]
  selected?: boolean
  relativeNumber?: number
  showLabel?: boolean
  // Visual (multi-select) mode: render a checkbox in place of the star and let
  // a click toggle membership instead of opening the thread.
  multiSelect?: boolean
  inSelection?: boolean
}>(), {
  labels: () => [],
  selected: false,
  showLabel: false,
  multiSelect: false,
  inSelection: false,
})

const emit = defineEmits<{
  (e: 'open', conversation: Conversation): void
  (e: 'toggle-star', conversation: Conversation): void
  (e: 'toggle-select', conversation: Conversation): void
}>()

function onClick() {
  if (props.multiSelect) emit('toggle-select', props.conversation)
  else emit('open', props.conversation)
}

const label = computed(() => props.showLabel ? labelFor(props.conversation, props.labels) : undefined)
const ariaLabel = computed(() => {
  const c = props.conversation
  const parts = [c.unread ? 'Unread.' : null, `${c.from.name || c.from.addr}.`, c.subject, c.starred ? '· starred' : null]
  return parts.filter(Boolean).join(' ')
})
</script>

<template>
  <article
    class="email-row"
    :class="{ unread: conversation.unread, selected, picked: multiSelect && inSelection }"
    role="listitem"
    tabindex="0"
    :aria-label="ariaLabel"
    :aria-selected="multiSelect ? inSelection : undefined"
    @click="onClick"
    @keydown.enter.stop="onClick"
    @keydown.space.stop.prevent="onClick"
  >
    <span v-if="relativeNumber !== undefined" class="relno" :class="{ cur: selected }">{{ relativeNumber }}</span>
    <button
      v-if="multiSelect"
      class="star pick"
      :class="{ active: inSelection }"
      type="button"
      :aria-label="inSelection ? 'Deselect' : 'Select'"
      @click.stop="emit('toggle-select', conversation)"
    >
      <component :is="inSelection ? PhCheckCircle : PhCircle" :size="16" :weight="inSelection ? 'fill' : 'regular'" />
    </button>
    <button
      v-else
      class="star"
      :class="{ active: conversation.starred }"
      type="button"
      aria-label="Star"
      @click.stop="emit('toggle-star', conversation)"
    >
      <PhStar :size="15" :weight="conversation.starred ? 'fill' : 'regular'" />
    </button>
    <div class="row-main">
      <div class="row-top">
        <strong>{{ conversation.from.name || conversation.from.addr }}</strong>
        <span v-if="conversation.snoozedUntil" class="wake" :title="`Wakes ${new Date(conversation.snoozedUntil).toLocaleString()}`">
          <PhClock :size="11" weight="bold" />{{ formatWakeTime(conversation.snoozedUntil) }}
        </span>
        <time v-else>{{ formatDate(conversation.lastAt) }}</time>
      </div>
      <div class="subject">
        <span>{{ conversation.subject }}</span>
        <em
          v-if="label"
          :style="{
            background: label.bg,
            color: label.fg,
          }"
        >
          {{ label.name }}
        </em>
      </div>
      <div class="snippet-line">{{ conversation.snippet }}</div>
    </div>
  </article>
</template>

<style scoped>
.email-row {
  position: relative;
  display: grid;
  grid-template-columns: 20px minmax(0, 1fr);
  align-items: start;
  gap: 9px;
  min-height: 72px;
  margin: 0;
  padding: 11px 12px;
  border-radius: 10px;
  color: var(--text-dim);
  cursor: pointer;
  overflow: hidden;
  transition: background var(--ease-fast), box-shadow var(--ease-fast);
}

.email-row:hover { background: var(--surface-2) }
.email-row:focus-visible { outline: 2px solid var(--accent); outline-offset: -2px }
.email-row.selected { background: var(--accent-soft); box-shadow: inset 0 0 0 1px var(--accent-line) }

.row-main {
  display: grid;
  grid-template-rows: 18px 18px 17px;
  gap: 2px;
  min-width: 0;
  overflow: hidden;
}

.row-top {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 8px;
  min-width: 0;
  line-height: 18px;
}

.row-top strong {
  min-width: 0;
  overflow: hidden;
  color: var(--text-dim);
  font-size: 13.5px;
  font-weight: 400;
  line-height: 18px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.email-row.unread .row-top strong {
  color: var(--head);
  font-weight: 600;
}

.row-top time {
  flex: none;
  color: var(--text-mut);
  font: 11px/18px "JetBrains Mono", ui-monospace, monospace;
}

.row-top .wake {
  display: inline-flex;
  flex: none;
  align-items: center;
  gap: 3px;
  color: var(--accent);
  font: 11px/18px "JetBrains Mono", ui-monospace, monospace;
}

.row-top .wake svg { display: block }

.subject {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  overflow: hidden;
  color: var(--text-dim);
  font-size: 13.5px;
  line-height: 18px;
}

.subject > :first-child {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.email-row.unread .subject {
  color: var(--head);
  font-weight: 600;
}

.subject em {
  flex: none;
  border-radius: 6px;
  padding: 2px 7px;
  font: 10px/1 "JetBrains Mono", ui-monospace, monospace;
  font-style: normal;
}

.snippet-line {
  overflow: hidden;
  color: var(--text-mut);
  font-size: 12.5px;
  line-height: 17px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.star {
  display: grid;
  place-items: center;
  width: 20px;
  height: 20px;
  margin-top: 1px;
  border: 0;
  background: transparent;
  color: var(--text-mut);
}

.star svg { display: block }
.star.active { color: var(--star) }
.star.pick { color: var(--text-mut) }
.star.pick.active { color: var(--accent) }
.email-row.picked { background: var(--accent-soft) }

</style>

<style>
.list-pane.relno-on .email-row {
  grid-template-columns: 26px 20px minmax(0, 1fr);
}

.relno {
  align-self: center;
  color: var(--text-mut);
  font: 11px/1 "JetBrains Mono", ui-monospace, monospace;
  text-align: right;
}

.relno.cur {
  color: var(--accent);
  font-weight: 700;
  text-align: left;
}

[data-density="compact"] .email-row {
  min-height: 50px;
  padding: 7px 16px;
}

[data-density="compact"] .row-main {
  grid-template-rows: 18px 18px;
}

[data-density="compact"] .snippet-line {
  display: none;
}
</style>
