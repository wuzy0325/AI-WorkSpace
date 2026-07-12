<script setup lang="ts">
import { NSlider } from 'naive-ui'

withDefaults(
  defineProps<{
    modelValue?: number
    min?: number
    max?: number
    step?: number
    ariaLabel?: string
    /** 是否显示悬浮数值提示，默认开启以兼容现有行为 */
    tooltip?: boolean
  }>(),
  { modelValue: 0, min: 0, max: 100, step: 1, tooltip: true },
)

const emit = defineEmits<{ (e: 'update:modelValue', v: number): void }>()
</script>

<template>
  <NSlider
    :value="modelValue"
    :min="min"
    :max="max"
    :step="step"
    :aria-label="ariaLabel"
    :tooltip="tooltip"
    @update:value="emit('update:modelValue', $event)"
  />
</template>
