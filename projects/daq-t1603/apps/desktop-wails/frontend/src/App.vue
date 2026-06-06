<script setup lang="ts">
import { onMounted, onUnmounted } from 'vue'
import { useDeviceStore } from '@stores/deviceStore'
import { useLogStore } from '@stores/logStore'
import { useRecordingStore } from '@stores/recordingStore'
import { onPayload, offPayload, onLog, offLog } from '@bridge/deviceBridge'
import type { TemperatureSnapshot, DeviceLogEvent } from '@bridge/deviceBridge'
import AppShell from '@components/layout/AppShell.vue'
import MonitorView from '@views/MonitorView.vue'

const deviceStore = useDeviceStore()
const logStore = useLogStore()
const recordingStore = useRecordingStore()

onMounted(() => {
  // 从后端加载已保存的设备配置
  deviceStore.loadProfiles()
  onPayload((snapshot: TemperatureSnapshot) => {
    deviceStore.pushSnapshot(snapshot)
  })
  onLog((entry: DeviceLogEvent) => {
    logStore.pushEvent(entry)
  })
  recordingStore.startListening()
})

onUnmounted(() => {
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
