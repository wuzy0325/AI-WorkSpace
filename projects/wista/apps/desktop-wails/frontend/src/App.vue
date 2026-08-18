<script setup lang="ts">
import { computed, onMounted, onUnmounted } from 'vue'
import { useDeviceStore } from '@stores/deviceStore'
import { useDisplayStore } from '@stores/displayStore'
import { useI18nStore } from '@stores/i18nStore'
import { useLogStore } from '@stores/logStore'
import { useRecordingStore } from '@stores/recordingStore'
import { useTheme } from '@composables/useTheme'
import { onPayload, offPayload, onLog, offLog, onDeviceState, offDeviceState, onRecordingFatal, offRecordingFatal, onRecordingBackpressure, offRecordingBackpressure, setUIRefreshRateHz } from '@bridge/deviceBridge'
import type { TemperatureSnapshot, DeviceLogEvent, DeviceState } from '@bridge/deviceBridge'
import AppShell from '@components/layout/AppShell.vue'
import MonitorView from '@views/MonitorView.vue'
import { NaiveThemeProvider } from '@shared-frontend/index'
import type { GlobalThemeOverrides } from 'naive-ui'

const { theme } = useTheme()
const deviceStore = useDeviceStore()
const displayStore = useDisplayStore()
const i18n = useI18nStore()
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
  // 语言偏好已在 i18nStore 创建时同步从 localStorage 读取，无需在此显式初始化
  // 从后端加载已保存的设备配置
  await deviceStore.loadProfiles()
  // 同步日志文件保存状态（后端启动时已自动开启）
  await logStore.refreshFileState()
  // 自动连接所有开启了自动连接的设备
  // 注意：单个设备连接失败时后端已通过 daq:log（error 级别）推送详细错误，
  // 前端 onLog 订阅会写入 logStore，此处仅记录汇总信息，避免重复日志。
  try {
    await deviceStore.autoConnectAll()
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err)
    logStore.info('auto-connect', i18n.t('logMessage.autoConnectFailed', { message }))
  }
  deviceStore.setDisplayRefreshRateHz(displayStore.refreshRateHz)
  onPayload((snapshot: TemperatureSnapshot) => {
    deviceStore.pushSnapshot(snapshot)
  })
  // 同步到后端：让 relayStream 按用户保存的刷新率推送 daq:payload，
  // 而非后端默认 10Hz。放在 onPayload 订阅之后，避免 await 阻塞事件订阅。
  // 失败不阻塞启动，后端会沿用默认值。
  void setUIRefreshRateHz(displayStore.refreshRateHz).catch((err) => {
    logStore.error('system', i18n.t('logMessage.syncRefreshRateFailed', {
      message: err instanceof Error ? err.message : String(err),
    }))
  })
  onLog((entry: DeviceLogEvent) => {
    logStore.pushEvent(entry)
  })
  // 订阅设备状态变更：后端在 connect/disconnect/acquiring/error 等状态变化时推送，
  // 前端实时同步 statusMap 与 errorMap，避免依赖轮询。
  // 注意：wista 后端当前未主动发射此事件，订阅为前置基础设施，
  // 待后端补齐 EmitDeviceState 后自动生效。
  onDeviceState((id: string, state: DeviceState) => {
    deviceStore.syncStatusFromBackend(id, state)
  })
  // 订阅录制 fatal/backpressure 事件：写入 logStore + 更新 recordingStore 状态
  onRecordingFatal((event) => {
    logStore.error('acquisition', i18n.t('logMessage.recordingFatal', {
      deviceId: event.deviceId,
      error: event.error,
    }))
    recordingStore.handleFatalError(event.error)
  })
  onRecordingBackpressure((event) => {
    logStore.warn('acquisition', i18n.t('logMessage.recordingBackpressure', {
      deviceId: event.deviceId,
      queueLen: event.queueLen,
      queueCap: event.queueCap,
    }))
    recordingStore.handleBackpressure(event.droppedTotal)
  })
  recordingStore.startListening()

})

onUnmounted(() => {
  deviceStore.stopDisplayFlush()
  offPayload()
  offLog()
  offDeviceState()
  offRecordingFatal()
  offRecordingBackpressure()
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
