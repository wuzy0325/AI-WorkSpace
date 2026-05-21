<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { useThemeStore } from '@stores/themeStore'
import MainLayout from '@views/main/MainLayout.vue'

const themeStore = useThemeStore()
const { theme } = storeToRefs(themeStore)
</script>

<template>
  <div class="app-shell" :data-theme="theme">
    <MainLayout />
  </div>
</template>
