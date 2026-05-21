<script setup lang="ts">
withDefaults(
  defineProps<{
    padded?: boolean
  }>(),
  { padded: true },
)
</script>

<template>
  <div class="ui-panel" :class="{ 'ui-panel--unpadded': !padded }">
    <div v-if="$slots.header" class="ui-panel__header">
      <slot name="header" />
    </div>
    <div class="ui-panel__body">
      <slot />
    </div>
  </div>
</template>

<style scoped>
.ui-panel {
  border-radius: 0.75rem;
  border: 1px solid var(--border-default);
  background: var(--bg-panel);
  box-shadow: 0 10px 30px rgba(2, 6, 23, 0.28);
}

.ui-panel__header {
  padding: 1rem 1.25rem;
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
}

.ui-panel__body {
  padding: 1rem 1.25rem;
}

.ui-panel--unpadded .ui-panel__body {
  padding: 0;
}
</style>
