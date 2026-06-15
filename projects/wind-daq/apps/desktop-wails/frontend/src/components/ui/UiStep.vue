<script setup lang="ts">
import { NStep } from 'naive-ui'

withDefaults(
  defineProps<{
    title?: string
    description?: string
    status?: 'process' | 'finish' | 'error' | 'wait'
    disabled?: boolean
  }>(),
  { title: '', description: '', status: 'process', disabled: false },
)
</script>

<template>
  <NStep :title="title" :description="description" :status="status" :disabled="disabled">
    <slot />
  </NStep>
</template>
