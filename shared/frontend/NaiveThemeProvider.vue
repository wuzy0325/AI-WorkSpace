<script setup lang="ts">
// @ts-nocheck
import {
  NConfigProvider,
  NDialogProvider,
  NMessageProvider,
  NNotificationProvider,
  darkTheme,
} from 'naive-ui'

defineProps<{
  theme: 'dark' | 'light'
  themeOverrides?: Record<string, any>
}>()
</script>

<template>
  <NConfigProvider
    :theme="theme === 'dark' ? darkTheme : null"
    :theme-overrides="themeOverrides"
  >
    <NDialogProvider>
      <NMessageProvider>
        <NNotificationProvider>
          <slot />
        </NNotificationProvider>
      </NMessageProvider>
    </NDialogProvider>
  </NConfigProvider>
</template>
