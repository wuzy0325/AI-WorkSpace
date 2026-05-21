<script setup lang="ts">
defineProps<{
  title: string
  subtitle?: string
}>()
</script>

<template>
  <div class="ui-section-header">
    <p v-if="subtitle" class="ui-section-header__subtitle">{{ subtitle }}</p>
    <h3 class="ui-section-header__title">{{ title }}</h3>
  </div>
</template>

<style scoped>
.ui-section-header__title {
  margin: 0;
  font-size: 0.95rem;
  font-weight: 800;
  letter-spacing: -0.02em;
}

.ui-section-header__subtitle {
  margin: 0 0 0.25rem;
  color: var(--text-muted);
  font-size: 0.65rem;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}
</style>
