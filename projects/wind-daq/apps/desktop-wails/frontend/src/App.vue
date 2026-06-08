<script setup lang="ts">
import { useThemeStore } from '@stores/themeStore'
import MainDashboardView from '@views/main/MainDashboardView.vue'
import UiToastHost from '@components/feedback/UiToastHost.vue'
import UiConfirmDialog from '@components/feedback/UiConfirmDialog.vue'
import { NaiveThemeProvider } from '@shared-frontend/index'
import { computed } from 'vue'
import type { GlobalThemeOverrides } from 'naive-ui'

const themeStore = useThemeStore()

const themeOverrides = computed<GlobalThemeOverrides>(() => {
  const isDark = themeStore.theme === 'dark'

  return {
    common: {
      primaryColor: isDark ? '#38bdf8' : '#22c55e',
      primaryColorHover: isDark ? '#7dd3fc' : '#16a34a',
      primaryColorPressed: isDark ? '#0284c7' : '#15803d',
      primaryColorSuppl: isDark ? '#0ea5e9' : '#22c55e',
      successColor: '#22c55e',
      successColorHover: '#4ade80',
      successColorPressed: '#16a34a',
      warningColor: isDark ? '#f59e0b' : '#f97316',
      warningColorHover: isDark ? '#fbbf24' : '#fb923c',
      warningColorPressed: isDark ? '#d97706' : '#ea580c',
      errorColor: isDark ? '#ef5b47' : '#f97316',
      errorColorHover: isDark ? '#f87171' : '#fb923c',
      errorColorPressed: isDark ? '#dc2626' : '#ea580c',
      infoColor: isDark ? '#38bdf8' : '#22c55e',
      borderRadius: '4px',
      fontFamily: "'Microsoft YaHei UI', 'Microsoft YaHei', 'PingFang SC', 'Segoe UI', sans-serif",
      fontFamilyMono: "'JetBrains Mono', 'Cascadia Code', 'SFMono-Regular', monospace",
      fontSize: '12px',
      textColorBase: isDark ? '#e2e8f0' : '#0f172a',
      bodyColor: isDark ? '#0f172a' : '#f8fafc',
      cardColor: isDark ? '#172338' : '#ffffff',
      modalColor: isDark ? '#172338' : '#ffffff',
      popoverColor: isDark ? '#1e293b' : '#ffffff',
      borderColor: isDark ? '#334155' : '#e2e8f0',
      dividerColor: isDark ? '#334155' : '#e2e8f0',
    },
    Button: {
      borderRadiusSmall: '4px',
      borderRadiusMedium: '4px',
    },
    Card: {
      borderRadius: '4px',
      color: isDark ? '#172338' : '#ffffff',
      borderColor: isDark ? '#334155' : '#e2e8f0',
    },
    DataTable: {
      borderColor: isDark ? '#334155' : '#e2e8f0',
      thColor: isDark ? '#1e293b' : '#f8fafc',
      tdColor: isDark ? '#172338' : '#ffffff',
      tdColorHover: isDark ? '#1e293b' : '#f8fafc',
      fontSizeSmall: '12px',
      thFontWeight: '600',
    },
    Form: {
      labelFontSizeLeftSmall: '12px',
      labelTextColor: isDark ? '#cbd5e1' : '#334155',
    },
  }
})
</script>

<template>
  <div class="app-shell" :data-theme="themeStore.theme">
    <NaiveThemeProvider :theme="themeStore.theme" :theme-overrides="themeOverrides">
      <MainDashboardView />
      <UiToastHost />
      <UiConfirmDialog />
    </NaiveThemeProvider>
  </div>
</template>

<style>
body { margin: 0; overflow: hidden; user-select: none; }
</style>
