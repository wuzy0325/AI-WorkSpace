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
  <!-- 面板组件：根据 padded 属性控制内边距，默认使用紧凑间距 -->
  <NCard
    size="small"
    :bordered="true"
    :content-style="padded ? { padding: 'var(--space-3) var(--space-4)' } : { padding: 0 }"
  >
    <template v-if="$slots.header" #header><slot name="header" /></template>
    <slot />
  </NCard>
</template>
