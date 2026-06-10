<script setup lang="ts">
import { NSlider } from 'naive-ui'

withDefaults(
  defineProps<{
    modelValue?: number
    min?: number
    max?: number
    step?: number
  }>(),
  { modelValue: 0, min: 0, max: 100, step: 1 },
)

const emit = defineEmits<{ (e: 'update:modelValue', v: number): void }>()
</script>

<template>
  <NSlider
    :value="modelValue"
    :min="min"
    :max="max"
    :step="step"
    @update:value="emit('update:modelValue', $event)"
  />
</template>
