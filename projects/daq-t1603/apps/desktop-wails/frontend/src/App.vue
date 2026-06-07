<script setup lang="ts">
import { onMounted, onUnmounted } from 'vue'
import { useDeviceStore } from '@stores/deviceStore'
import { useDisplayStore } from '@stores/displayStore'
import { useLogStore } from '@stores/logStore'
import { useRecordingStore } from '@stores/recordingStore'
import { onPayload, offPayload, onLog, offLog } from '@bridge/deviceBridge'
import type { TemperatureSnapshot, DeviceLogEvent } from '@bridge/deviceBridge'
import AppShell from '@components/layout/AppShell.vue'
import MonitorView from '@views/MonitorView.vue'

const deviceStore = useDeviceStore()
const displayStore = useDisplayStore()
const logStore = useLogStore()
const recordingStore = useRecordingStore()

onMounted(async () => {
  // 从后端加载已保存的设备配置
  await deviceStore.loadProfiles()
  // 自动连接所有开启了自动连接的设备
  try {
    await deviceStore.autoConnectAll()
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err)
    logStore.warn('auto-connect', `自动连接失败: ${message}`)
  }
  deviceStore.setDisplayRefreshRateHz(displayStore.refreshRateHz)
  onPayload((snapshot: TemperatureSnapshot) => {
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
  <AppShell>
    <MonitorView />
  </AppShell>
</template>
