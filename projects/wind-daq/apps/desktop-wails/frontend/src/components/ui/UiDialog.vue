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
  <!-- 对话框组件：支持自定义宽度，限制最大高度为视口 90% 防止内容过长导致布局异常 -->
  <NModal
    :show="show"
    preset="card"
    :title="title"
    :style="width ? { maxWidth: width, width: '92vw', maxHeight: '90vh' } : { maxWidth: '640px', width: '92vw', maxHeight: '90vh' }"
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
