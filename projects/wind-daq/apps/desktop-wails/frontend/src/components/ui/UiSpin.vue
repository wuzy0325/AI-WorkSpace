<script setup lang="ts">
import { NSpin } from 'naive-ui'

withDefaults(
  defineProps<{
    show?: boolean
    size?: 'small' | 'medium' | 'large'
    description?: string
  }>(),
  { show: true, size: 'small', description: '' },
)
</script>

<template>
  <NSpin :show="show" :size="size" :description="description">
    <slot />
  </NSpin>
</template>
