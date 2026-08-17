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

const emit = defineEmits<{
  click: []
}>()

function handleClick() {
  emit('click')
}
</script>

<template>
  <NStep
    :title="title"
    :description="description"
    :status="status"
    :disabled="disabled"
    @click="handleClick"
  >
    <slot />
  </NStep>
</template>
