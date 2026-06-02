<script setup lang="ts">
import { onMounted, onUnmounted } from 'vue'
import { useDeviceStore } from '@stores/deviceStore'
import { useRecordingStore } from '@stores/recordingStore'
import { onPayload, offPayload } from '@bridge/deviceBridge'
import type { TemperatureSnapshot } from '@bridge/deviceBridge'
import AppShell from '@components/layout/AppShell.vue'
import MonitorView from '@views/MonitorView.vue'

const deviceStore = useDeviceStore()
const recordingStore = useRecordingStore()

onMounted(() => {
  onPayload((snapshot: TemperatureSnapshot) => {
    deviceStore.pushSnapshot(snapshot)
  })
  recordingStore.startListening()
})

onUnmounted(() => {
  offPayload()
  recordingStore.stopListening()
})
</script>

<template>
  <AppShell>
    <MonitorView />
  </AppShell>
</template>
