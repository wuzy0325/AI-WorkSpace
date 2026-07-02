<script setup lang="ts">
import { computed } from 'vue'
import { NSelect } from 'naive-ui'

const props = withDefaults(
  defineProps<{
    modelValue?: string
    options?: Array<{ value: string; label: string }>
    placeholder?: string
    size?: 'sm' | 'md' | 'lg'
    disabled?: boolean
    ariaLabel?: string
    dataTest?: string
    /**
     * value 在 options 中找不到匹配项时的回退行为
     *
     * - false：不显示回退内容，直接展示 placeholder（用于"原设备已删除"场景，避免显示原始 UUID）
     * - 默认 true：显示 value 字符串本身（naive-ui NSelect 默认行为）
     */
    fallback?: boolean
  }>(),
  { modelValue: '', options: () => [], placeholder: '', size: 'sm', disabled: false, ariaLabel: '', dataTest: '', fallback: true },
)

const naiveSize = computed(() => {
  if (props.size === 'sm') return 'small'
  if (props.size === 'lg') return 'large'
  return 'medium'
})

const emit = defineEmits<{ (e: 'update:modelValue', v: string): void }>()
</script>

<template>
  <NSelect
    :value="modelValue || null"
    :options="options"
    :placeholder="placeholder"
    :size="naiveSize"
    :disabled="disabled"
    :aria-label="ariaLabel || undefined"
    :data-test="dataTest || undefined"
    :fallback="fallback"
    @update:value="emit('update:modelValue', $event ?? '')"
  />
</template>
