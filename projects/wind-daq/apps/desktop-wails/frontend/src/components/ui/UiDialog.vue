<script setup lang="ts">
import { NModal } from 'naive-ui'

defineProps<{
  show: boolean
  title?: string
  width?: string
  closable?: boolean
}>()

const emit = defineEmits<{
  (e: 'update:show', v: boolean): void
}>()
</script>

<template>
  <NModal
    :show="show"
    preset="card"
    :title="title"
    :style="width ? { maxWidth: width, width: '92vw' } : { maxWidth: '640px', width: '92vw' }"
    :closable="closable ?? true"
    size="small"
    @update:show="emit('update:show', $event)"
  >
    <template v-if="$slots.header" #header>
      <slot name="header" />
    </template>
    <slot />
    <template v-if="$slots.footer" #footer>
      <slot name="footer" />
    </template>
  </NModal>
</template>
