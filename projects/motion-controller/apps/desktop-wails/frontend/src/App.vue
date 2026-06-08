<script setup lang="ts">
import MotionView from './views/MotionView.vue'
import ToastOverlay from './components/ToastOverlay.vue'
import UiConfirmDialog from './components/ui/UiConfirmDialog.vue'
</script>

<template>
  <div class="app">
    <MotionView />
    <ToastOverlay />
    <UiConfirmDialog />
  </div>
</template>

<style>
.app {
  font-family: Inter, system-ui, Avenir, Helvetica, Arial, sans-serif;
  line-height: 1.5;
  font-weight: 400;
  color: var(--text-primary);
  background-color: var(--bg-app);
  margin: 0;
  padding: 0;
}
</style>
