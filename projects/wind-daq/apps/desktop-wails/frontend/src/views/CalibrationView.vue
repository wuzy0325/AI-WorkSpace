<script setup lang="ts">
import { defineAsyncComponent } from 'vue'

const CalibrationWindow = defineAsyncComponent(() => import('@components/calibration/CalibrationWindow.vue'))
</script>

<template>
  <div class="flex-1 min-h-0 w-full">
    <CalibrationWindow />
  </div>
</template>
