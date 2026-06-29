<script setup lang="ts" generic="T extends string | number">
// horizontal group of mutually-exclusive `.set-btn` choices (poll interval,
// notify mode, undo-send window)
defineProps<{
  options: ReadonlyArray<{ label: string; value: T }>
  ariaLabel?: string
}>()
const model = defineModel<T>({ required: true })
</script>

<template>
  <div class="notif-actions" role="group" :aria-label="ariaLabel">
    <button
      v-for="opt in options"
      :key="opt.value"
      class="set-btn"
      :class="{ active: model === opt.value }"
      :aria-pressed="model === opt.value"
      type="button"
      @click="model = opt.value"
    >
      {{ opt.label }}
    </button>
  </div>
</template>
