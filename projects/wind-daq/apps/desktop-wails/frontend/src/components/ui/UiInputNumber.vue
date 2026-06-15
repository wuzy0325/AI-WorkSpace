<script setup lang="ts">
import { NInputNumber } from 'naive-ui'

withDefaults(
  defineProps<{
    modelValue?: number | null
    min?: number
    max?: number
    step?: number
    disabled?: boolean
    placeholder?: string
  }>(),
  { modelValue: null, disabled: false, placeholder: '' },
)

const emit = defineEmits<{ (e: 'update:modelValue', v: number | null): void }>()
</script>

<template>
  <NInputNumber
    :value="modelValue"
    :min="min"
    :max="max"
    :step="step"
    :disabled="disabled"
    :placeholder="placeholder"
    size="small"
    @update:value="emit('update:modelValue', $event)"
  />
</template>
