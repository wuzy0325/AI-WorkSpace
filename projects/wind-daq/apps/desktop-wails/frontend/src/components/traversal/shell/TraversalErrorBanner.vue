<script setup lang="ts">
import UiAlert from '@components/ui/UiAlert.vue'

defineProps<{
  error: string | null
  dismissLabel: string
}>()

defineEmits<{
  dismiss: []
}>()
</script>

<template>
  <UiAlert
    v-if="error"
    type="error"
    :closable="true"
    @close="$emit('dismiss')"
  >
    {{ error }}
  </UiAlert>
</template>