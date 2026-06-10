<script setup lang="ts">
import { NText } from 'naive-ui'

defineProps<{
  title: string
  subtitle?: string
}>()
</script>

<template>
  <div>
    <NText v-if="subtitle" depth="3" class="subtitle">
      {{ subtitle }}
    </NText>
    <NText depth="1" tag="h3" class="title">
      {{ title }}
    </NText>
  </div>
</template>

<style scoped>
.subtitle {
  display: block;
  font-size: var(--text-xs);
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.08em;
  margin-bottom: 0.25rem;
}
.title {
  margin: 0;
  font-size: var(--text-base);
  font-weight: 800;
  letter-spacing: -0.02em;
}
</style>
