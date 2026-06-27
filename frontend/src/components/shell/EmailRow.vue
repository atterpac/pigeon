<script setup lang="ts">
import { PhStar } from '@phosphor-icons/vue'
import { computed } from 'vue'
import { formatDate, labelFor } from '../../mail/format'
import type { Conversation, Label } from '../../mail/types'

const props = withDefaults(defineProps<{
  conversation: Conversation
  labels?: Label[]
  selected?: boolean
  relativeNumber?: number
  showLabel?: boolean
}>(), {
  labels: () => [],
  selected: false,
  showLabel: false,
})

const emit = defineEmits<{
  (e: 'open', conversation: Conversation): void
  (e: 'toggle-star', conversation: Conversation): void
}>()

const label = computed(() => props.showLabel ? labelFor(props.conversation, props.labels) : undefined)
</script>

<template>
  <article
    class="email-row"
    :class="{ unread: conversation.unread, selected }"
    @click="emit('open', conversation)"
  >
    <span v-if="relativeNumber !== undefined" class="relno" :class="{ cur: selected }">{{ relativeNumber }}</span>
    <button
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
        <time>{{ formatDate(conversation.lastAt) }}</time>
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
