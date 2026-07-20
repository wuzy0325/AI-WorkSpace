<script setup lang="ts">
import { useThemeStore } from '@stores/themeStore'
import { useFeedbackStore } from '@stores/feedbackStore'
import { useI18nStore } from '@stores/i18nStore'
import { wailsApi, isWailsAvailable } from '@api/wails-adapter'
import UiToastHost from '@components/feedback/UiToastHost.vue'
import UiConfirmDialog from '@components/feedback/UiConfirmDialog.vue'
import { NaiveThemeProvider } from '@shared-frontend/index'
import { computed, onMounted, onBeforeUnmount } from 'vue'
import type { GlobalThemeOverrides } from 'naive-ui'

const themeStore = useThemeStore()
const feedbackStore = useFeedbackStore()
const i18n = useI18nStore()

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
    // 浅色主题下 Input/Select 文字颜色强制使用深色，确保对比度
    Input: {
      textColor: isDark ? '#e2e8f0' : '#0f172a',
      placeholderColor: isDark ? '#64748b' : '#94a3b8',
      border: isDark ? '1px solid #334155' : '1px solid #e2e8f0',
      borderHover: isDark ? '1px solid #38bdf8' : '1px solid #22c55e',
      borderFocus: isDark ? '1px solid #38bdf8' : '1px solid #22c55e',
      color: isDark ? 'rgba(255, 255, 255, 0.03)' : '#ffffff',
      colorFocus: isDark ? 'rgba(255, 255, 255, 0.05)' : '#ffffff',
    },
    Select: {
      textColor: isDark ? '#e2e8f0' : '#0f172a',
      placeholderColor: isDark ? '#64748b' : '#94a3b8',
      border: isDark ? '1px solid #334155' : '1px solid #e2e8f0',
      borderHover: isDark ? '1px solid #38bdf8' : '1px solid #22c55e',
      borderFocus: isDark ? '1px solid #38bdf8' : '1px solid #22c55e',
      color: isDark ? 'rgba(255, 255, 255, 0.03)' : '#ffffff',
    },
  }
})

// 退出确认对话框状态：
//   - exitDialogShowing 防止用户连续点 X 触发多次 confirm 弹窗
//   - 用户在 confirm 弹窗中点"退出" → 调用后端 RequestExit 触发完整关闭流程
//   - 用户点"取消" → 关闭弹窗，应用保持运行
let exitDialogShowing = false
let cleanupExitListener: (() => void) | null = null

async function handleExitRequest(): Promise<void> {
  // 防止重复弹窗（用户连续点 X 时后端会推多次 app:exit-requested 事件）
  if (exitDialogShowing) return
  exitDialogShowing = true
  try {
    const confirmed = await feedbackStore.confirm(i18n.t.exitConfirmMessage, {
      title: i18n.t.exitConfirmTitle,
      confirmText: i18n.t.exitConfirmOk,
      cancelText: i18n.t.exitConfirmCancel,
    })
    if (confirmed) {
      // 用户确认退出，调用后端 RequestExit binding
      // 后端置 userConfirmedExit=true 后调用 application.Quit()，
      // cleanup → window.Close() → hook 再次触发但 userConfirmedExit=true 放行 → 真正关闭
      await wailsApi.app.requestExit()
    }
  } finally {
    exitDialogShowing = false
  }
}

onMounted(() => {
  // 仅在 Wails 环境注册退出请求监听（浏览器预览/Spike 模式无后端事件源）
  if (isWailsAvailable()) {
    cleanupExitListener = wailsApi.app.onExitRequested(() => {
      void handleExitRequest()
    })
  }
})

onBeforeUnmount(() => {
  cleanupExitListener?.()
  cleanupExitListener = null
})
</script>

<template>
  <div class="app-shell" :data-theme="themeStore.theme">
    <NaiveThemeProvider :theme="themeStore.theme" :theme-overrides="themeOverrides">
      <router-view />
      <UiToastHost />
      <UiConfirmDialog />
    </NaiveThemeProvider>
  </div>
</template>

<style>
body { margin: 0; overflow: hidden; user-select: none; }
</style>
