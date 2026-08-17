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
    /** 是否显示右侧加减按钮，默认隐藏以保持界面简洁 */
    showButton?: boolean
  }>(),
  { modelValue: null, disabled: false, placeholder: '', showButton: false },
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
    :show-button="showButton"
    size="small"
    @update:value="emit('update:modelValue', $event)"
  />
</template>
