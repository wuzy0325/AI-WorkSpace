<script setup lang="ts">
import { NSelect } from 'naive-ui'

withDefaults(
  defineProps<{
    modelValue?: string
    options?: Array<{ value: string; label: string }>
    placeholder?: string
  }>(),
  { modelValue: '', options: () => [], placeholder: '' },
)

const emit = defineEmits<{ (e: 'update:modelValue', v: string): void }>()
</script>

<template>
  <NSelect
    :value="modelValue || null"
    :options="options"
    :placeholder="placeholder"
    size="small"
    @update:value="emit('update:modelValue', $event ?? '')"
  />
</template>
