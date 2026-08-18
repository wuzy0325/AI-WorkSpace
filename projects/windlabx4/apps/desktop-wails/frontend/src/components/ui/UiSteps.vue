<script setup lang="ts">
import { NSteps } from 'naive-ui'

withDefaults(
  defineProps<{
    current?: number
    status?: 'process' | 'finish' | 'error' | 'wait'
  }>(),
  { current: 0, status: 'process' },
)
</script>

<template>
  <NSteps :current="current" :status="status" size="small">
    <slot />
  </NSteps>
</template>
