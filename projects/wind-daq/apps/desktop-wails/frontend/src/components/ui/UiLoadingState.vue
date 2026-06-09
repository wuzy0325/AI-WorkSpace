<script setup lang="ts">
defineProps<{
  loading: boolean
  text?: string
}>()
</script>

<template>
  <div v-if="loading" class="ui-loading-state">
    <span class="ui-loading-state__spinner" />
    <p v-if="text" class="ui-loading-state__text">{{ text }}</p>
  </div>
  <slot v-else />
</template>

<style scoped>
.ui-loading-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: var(--space-8) var(--space-4);
  min-height: 120px;
  gap: var(--space-3);
}
.ui-loading-state__spinner {
  width: 24px;
  height: 24px;
  border: 2px solid var(--border-default);
  border-top-color: var(--accent-primary);
  border-radius: 50%;
  animation: ui-spin var(--motion-slow) var(--easing-standard) infinite;
}
.ui-loading-state__text {
  margin: 0;
  font-size: var(--font-size-sm);
  color: var(--text-tertiary);
}
@keyframes ui-spin {
  to { transform: rotate(360deg); }
}
</style>
