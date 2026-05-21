<script setup lang="ts">
import { useThemeStore } from '@stores/themeStore'
import MainDashboardView from '@views/main/MainDashboardView.vue'
import UiToastHost from '@components/feedback/UiToastHost.vue'
import UiConfirmDialog from '@components/feedback/UiConfirmDialog.vue'

const themeStore = useThemeStore()
</script>

<template>
  <div class="app-shell" :data-theme="themeStore.theme">
    <MainDashboardView />
    <UiToastHost />
    <UiConfirmDialog />
  </div>
</template>

<style>
body { margin: 0; overflow: hidden; user-select: none; }
</style>
