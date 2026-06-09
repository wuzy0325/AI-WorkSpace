<script setup lang="ts">
import { computed } from 'vue'
import { NSelect } from 'naive-ui'

const props = withDefaults(
  defineProps<{
    modelValue?: string
    options?: Array<{ value: string; label: string }>
    placeholder?: string
    size?: 'sm' | 'md' | 'lg'
  }>(),
  { modelValue: '', options: () => [], placeholder: '', size: 'sm' },
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
    @update:value="emit('update:modelValue', $event ?? '')"
  />
</template>
