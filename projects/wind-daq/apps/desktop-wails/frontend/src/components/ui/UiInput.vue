<script setup lang="ts">
import { computed, useAttrs } from 'vue'
import { NInput } from 'naive-ui'

const props = withDefaults(
  defineProps<{
    modelValue?: string | number
    type?: 'text' | 'number'
    placeholder?: string
    inputId?: string
    ariaLabel?: string
    ariaDescribedby?: string
  }>(),
  { modelValue: '', type: 'text', placeholder: '' },
)

const emit = defineEmits<{ (e: 'update:modelValue', v: string | number): void }>()
const attrs = useAttrs()

// 生成稳定的输入框 ID，用于 label 关联
const computedId = computed(() => props.inputId || `ui-input-${Math.random().toString(36).slice(2, 9)}`)

// 提取 ARIA 和输入属性
const inputAttrs = computed(() => {
  const result: Record<string, string> = {}
  if (props.ariaLabel) result['aria-label'] = props.ariaLabel
  if (props.ariaDescribedby) result['aria-describedby'] = props.ariaDescribedby
  // 透传其他 aria 属性
  for (const key of Object.keys(attrs)) {
    if (key.startsWith('aria-') || key === 'role' || key === 'autocomplete') {
      result[key] = attrs[key] as string
    }
  }
  return result
})
</script>

<template>
  <NInput
    :id="computedId"
    :value="String(modelValue)"
    :placeholder="placeholder"
    size="small"
    v-bind="inputAttrs"
    @update:value="emit('update:modelValue', $event)"
  >
    <template v-if="$slots.prefix" #prefix>
      <slot name="prefix" />
    </template>
  </NInput>
</template>
