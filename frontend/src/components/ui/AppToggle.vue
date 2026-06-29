<script setup lang="ts">
// on/off switch styled by the global `.toggle` class. real switch semantics
// (role + aria-checked) so assistive tech announces state, not just the css.
defineProps<{ label: string }>()
const model = defineModel<boolean>({ required: true })
</script>

<template>
  <button
    type="button"
    role="switch"
    :aria-checked="model"
    :aria-label="label"
    class="toggle"
    :class="{ on: model }"
    @click="model = !model"
  >
    <i />
  </button>
</template>
