<script setup lang="ts">
import { computed, onMounted, onUnmounted } from 'vue'
import { useDeviceStore } from '@stores/deviceStore'
import { useDisplayStore } from '@stores/displayStore'
import { useLogStore } from '@stores/logStore'
import { useRecordingStore } from '@stores/recordingStore'
import { useTheme } from '@composables/useTheme'
import { onPayload, offPayload, onLog, offLog } from '@bridge/deviceBridge'
import type { PressureSnapshot, DeviceLogEvent } from '@bridge/deviceBridge'
import AppShell from '@components/layout/AppShell.vue'
import MonitorView from '@views/MonitorView.vue'
import { NaiveThemeProvider } from '@shared-frontend/index'
import type { GlobalThemeOverrides } from 'naive-ui'

const { theme } = useTheme()
const deviceStore = useDeviceStore()
const displayStore = useDisplayStore()
const logStore = useLogStore()
const recordingStore = useRecordingStore()

const themeOverrides = computed<GlobalThemeOverrides>(() => {
  const isDark = theme.value === 'dark'

  return {
    common: {
      primaryColor: isDark ? '#10b981' : '#059669',
      primaryColorHover: isDark ? '#34d399' : '#10b981',
      primaryColorPressed: isDark ? '#059669' : '#047857',
      primaryColorSuppl: isDark ? '#10b981' : '#059669',
      successColor: isDark ? '#10b981' : '#059669',
      successColorHover: isDark ? '#34d399' : '#10b981',
      successColorPressed: isDark ? '#059669' : '#047857',
      warningColor: isDark ? '#f59e0b' : '#d97706',
      warningColorHover: isDark ? '#fbbf24' : '#f59e0b',
      warningColorPressed: isDark ? '#d97706' : '#b45309',
      errorColor: isDark ? '#f43f5e' : '#e11d48',
      errorColorHover: isDark ? '#fb7185' : '#f43f5e',
      errorColorPressed: isDark ? '#e11d48' : '#be123c',
      infoColor: isDark ? '#3b82f6' : '#2563eb',
      fontSize: '12px',
      borderRadius: '4px',
      fontFamily: "-apple-system, BlinkMacSystemFont, 'Segoe UI', 'PingFang SC', 'Microsoft YaHei', sans-serif",
      fontFamilyMono: "ui-monospace, SFMono-Regular, 'SF Mono', Menlo, Consolas, monospace",
      textColorBase: isDark ? '#e2e8f0' : '#0f172a',
      bodyColor: isDark ? '#020617' : '#f6f8fb',
      cardColor: isDark ? '#0f172a' : '#ffffff',
      modalColor: isDark ? '#0f172a' : '#ffffff',
      popoverColor: isDark ? '#1e293b' : '#ffffff',
      borderColor: isDark ? 'rgba(255,255,255,0.06)' : 'rgba(15,23,42,0.07)',
      dividerColor: isDark ? 'rgba(255,255,255,0.04)' : 'rgba(15,23,42,0.06)',
    },
    Button: {
      borderRadiusSmall: '4px',
      borderRadiusMedium: '4px',
    },
    Card: {
      borderRadius: '8px',
      color: isDark ? 'rgba(30,41,59,0.45)' : 'rgba(255,255,255,0.78)',
      borderColor: 'transparent',
    },
    DataTable: {
      borderColor: isDark ? 'rgba(255,255,255,0.06)' : 'rgba(15,23,42,0.07)',
      thColor: isDark ? '#0f172a' : '#f1f5f9',
      tdColor: isDark ? 'transparent' : 'transparent',
      tdColorHover: isDark ? 'rgba(148,163,184,0.06)' : 'rgba(15,23,42,0.03)',
      fontSizeSmall: '12px',
      thFontWeight: '600',
    },
  }
})

onMounted(async () => {
  // 从后端加载已保存的设备配置
  await deviceStore.loadProfiles()
  // 同步日志文件保存状态（后端启动时已自动开启）
  await logStore.refreshFileState()
  // 自动连接所有开启了自动连接的设备
  try {
    await deviceStore.autoConnectAll()
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err)
    logStore.warn('auto-connect', `自动连接失败: ${message}`)
  }
  deviceStore.setDisplayRefreshRateHz(displayStore.refreshRateHz)
  onPayload((snapshot: PressureSnapshot) => {
    deviceStore.pushSnapshot(snapshot)
  })
  onLog((entry: DeviceLogEvent) => {
    logStore.pushEvent(entry)
  })
  recordingStore.startListening()
})

onUnmounted(() => {
  deviceStore.stopDisplayFlush()
  offPayload()
  offLog()
  recordingStore.stopListening()
})
</script>

<template>
  <NaiveThemeProvider :theme="theme" :theme-overrides="themeOverrides">
    <AppShell>
      <MonitorView />
    </AppShell>
  </NaiveThemeProvider>
</template>
