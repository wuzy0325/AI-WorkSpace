<script setup lang="ts">
import { NCard } from 'naive-ui'

withDefaults(
  defineProps<{
    padded?: boolean
  }>(),
  { padded: true },
)
</script>

<template>
  <NCard
    size="small"
    :bordered="true"
    :content-style="padded ? { padding: '1rem 1.25rem' } : { padding: 0 }"
  >
    <template v-if="$slots.header" #header><slot name="header" /></template>
    <slot />
  </NCard>
</template>
