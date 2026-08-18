<script setup lang="ts">
defineProps<{
  title?: string
  message: string
}>()
</script>

<template>
  <div class="ui-error-state">
    <h3 v-if="title" class="ui-error-state__title">{{ title }}</h3>
    <p class="ui-error-state__message">{{ message }}</p>
    <div v-if="$slots.action" class="ui-error-state__action">
      <slot name="action" />
    </div>
  </div>
</template>

<style scoped>
.ui-error-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: var(--space-6) var(--space-4);
  text-align: center;
  min-height: 120px;
  gap: var(--space-2);
}
.ui-error-state__title {
  margin: 0;
  font-size: var(--font-size-lg);
  font-weight: var(--font-weight-semibold);
  color: var(--accent-danger);
}
.ui-error-state__message {
  margin: 0;
  font-size: var(--font-size-sm);
  color: var(--text-secondary);
  line-height: var(--line-height-base);
  max-width: 400px;
}
.ui-error-state__action {
  margin-top: var(--space-2);
}
</style>
