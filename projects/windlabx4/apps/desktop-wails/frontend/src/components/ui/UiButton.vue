<script setup lang="ts">
import { computed, useAttrs } from 'vue'
import { NButton } from 'naive-ui'

const props = withDefaults(
  defineProps<{
    variant?: 'primary' | 'secondary' | 'danger' | 'ghost' | 'warning'
    size?: 'sm' | 'md' | 'lg'
    disabled?: boolean
    ariaLabel?: string
  }>(),
  { variant: 'primary', size: 'md', disabled: false },
)

const attrs = useAttrs()

const naiveType = computed(() => {
  if (props.variant === 'danger') return 'error'
  if (props.variant === 'warning') return 'warning'
  if (props.variant === 'primary') return 'primary'
  return 'default'
})

const naiveSecondary = computed(() => props.variant === 'secondary' || props.variant === 'warning' || props.variant === 'danger')
const naiveQuaternary = computed(() => props.variant === 'ghost')

const naiveSize = computed(() => {
  if (props.size === 'sm') return 'small'
  if (props.size === 'lg') return 'large'
  return 'medium'
})

// 提取 ARIA 相关属性，确保无障碍支持
const ariaAttrs = computed(() => {
  const result: Record<string, string> = {}
  if (props.ariaLabel) result['aria-label'] = props.ariaLabel
  // 透传其他 aria 属性
  for (const key of Object.keys(attrs)) {
    if (key.startsWith('aria-') || key === 'role') {
      result[key] = attrs[key] as string
    }
  }
  return result
})
</script>

<template>
  <NButton
    :type="naiveType"
    :secondary="naiveSecondary"
    :quaternary="naiveQuaternary"
    :size="naiveSize"
    :disabled="disabled"
    v-bind="ariaAttrs"
  >
    <template v-if="$slots.icon" #icon>
      <slot name="icon" />
    </template>
    <slot />
  </NButton>
</template>
