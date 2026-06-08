<script setup lang="ts">
import { NInput } from 'naive-ui'

withDefaults(
  defineProps<{
    modelValue?: string | number
    type?: 'text' | 'number'
    placeholder?: string
  }>(),
  { modelValue: '', type: 'text', placeholder: '' },
)

const emit = defineEmits<{ (e: 'update:modelValue', v: string | number): void }>()
</script>

<template>
  <NInput
    :value="String(modelValue)"
    :placeholder="placeholder"
    size="small"
    @update:value="emit('update:modelValue', $event)"
  />
</template>
