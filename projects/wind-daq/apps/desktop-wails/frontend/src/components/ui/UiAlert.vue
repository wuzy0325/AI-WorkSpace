<script setup lang="ts">
import { NAlert } from 'naive-ui'
import { computed } from 'vue'

const props = withDefaults(
  defineProps<{
    type?: 'info' | 'success' | 'warning' | 'error'
    title?: string
    closable?: boolean
  }>(),
  { type: 'info', title: '', closable: false },
)

const emit = defineEmits<{ (e: 'close'): void }>()

const naiveType = computed(() => {
  if (props.type === 'error') return 'error'
  if (props.type === 'warning') return 'warning'
  if (props.type === 'success') return 'success'
  return 'info'
})
</script>

<template>
  <NAlert
    :type="naiveType"
    :title="title"
    :closable="closable"
    @close="emit('close')"
  >
    <slot />
  </NAlert>
</template>
