<script setup lang="ts">
import { NSwitch } from 'naive-ui'

withDefaults(
  defineProps<{
    modelValue?: boolean
    disabled?: boolean
  }>(),
  { modelValue: false, disabled: false },
)

const emit = defineEmits<{ (e: 'update:modelValue', v: boolean): void }>()
</script>

<template>
  <NSwitch
    :value="modelValue"
    :disabled="disabled"
    size="small"
    @update:value="emit('update:modelValue', $event)"
  />
</template>
