<script setup lang="ts">
import { defineAsyncComponent, ref } from 'vue'
import UiErrorState from '@components/ui/UiErrorState.vue'
import UiButton from '@components/ui/UiButton.vue'

const loadError = ref('')
const CalibrationWindow = defineAsyncComponent({
  loader: () => import('@components/calibration/CalibrationWindow.vue'),
  onError: (err) => { loadError.value = err.message },
})
</script>

<template>
  <div class="flex-1 min-h-0 w-full">
    <UiErrorState v-if="loadError" title="加载失败" :message="loadError">
      <template #action>
        <UiButton variant="secondary" size="sm" @click="loadError = ''">重试</UiButton>
      </template>
    </UiErrorState>
    <CalibrationWindow v-else />
  </div>
</template>
