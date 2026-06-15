<script setup lang="ts">
import { NCheckbox } from 'naive-ui'

withDefaults(
  defineProps<{
    checked?: boolean
    disabled?: boolean
  }>(),
  { checked: false, disabled: false },
)

const emit = defineEmits<{ (e: 'update:checked', v: boolean): void }>()
</script>

<template>
  <NCheckbox
    :checked="checked"
    :disabled="disabled"
    @update:checked="emit('update:checked', $event)"
  >
    <slot />
  </NCheckbox>
</template>
