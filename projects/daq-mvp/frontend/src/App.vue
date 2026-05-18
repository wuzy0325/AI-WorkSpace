<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import ControlBar from './components/ControlBar.vue'
import MetricsPanel from './components/MetricsPanel.vue'
import WaveformCanvas from './components/WaveformCanvas.vue'
import type { Status, UiSampleFrame } from './api/wails'

const status = ref<Status>({
  state: 'idle',
  sampleRateHz: 1000,
  batchCount: 0,
  sampleCount: 0,
  latestValues: [0, 0, 0, 0],
})

const frame = ref<UiSampleFrame>({
  deviceId: 'mock-001',
  sequenceStart: 0,
  sampleCount: 0,
  channels: [0, 1, 2, 3],
  latestValues: [0, 0, 0, 0],
  samplesPerChannel: 0,
  hostTimestampMs: 0,
})

const runtime = useRuntime()

async function start() {
  if (!runtime.ready) return
  try {
    await window.go.main.App.StartAcquisition()
    const s = await window.go.main.App.GetStatus()
    status.value = s
  } catch (e) {
    console.error('start failed', e)
  }
}

async function stop() {
  if (!runtime.ready) return
  try {
    await window.go.main.App.StopAcquisition()
    const s = await window.go.main.App.GetStatus()
    status.value = s
  } catch (e) {
    console.error('stop failed', e)
  }
}

function useRuntime() {
  const ready = ref(false)
  onMounted(() => {
    window.runtime.EventsOn('waveform', (f: UiSampleFrame) => {
      frame.value = f
    })
    window.runtime.EventsOn('status', (s: Status) => {
      status.value = s
    })
    // Poll status initially and every second as fallback
    pollStatus()
  })
  onUnmounted(() => {
    window.runtime.EventsOff('waveform')
    window.runtime.EventsOff('status')
  })
  return { ready }
}

let pollTimer: ReturnType<typeof setInterval> | null = null

async function pollStatus() {
  try {
    if (window.go?.main?.App?.GetStatus) {
      const s = await window.go.main.App.GetStatus()
      status.value = s
      if (s.state === 'running') {
        if (!pollTimer) {
          pollTimer = setInterval(pollStatus, 1000)
        }
      } else {
        if (pollTimer) {
          clearInterval(pollTimer)
          pollTimer = null
        }
      }
    }
  } catch (_) {
    // Wails not ready yet; retry
  }
  setTimeout(pollStatus, 2000)
}
</script>

<template>
  <div class="app-shell">
    <ControlBar :status="status" @start="start" @stop="stop" />
    <div class="app-body">
      <MetricsPanel :status="status" :frame="frame" />
      <WaveformCanvas :frame="frame" :channel-count="4" />
    </div>
  </div>
</template>

<style>
:root {
  --bg-app: #0f172a;
  --bg-panel: #1e293b;
  --bg-canvas: #111c31;
  --text-primary: #e2e8f0;
  --text-secondary: #94a3b8;
  --text-muted: #64748b;
  --accent-primary: #56ccf2;
  --accent-success: #22c55e;
  --accent-danger: #ef5b47;
  --accent-warning: #f59e0b;
  --border-default: #334155;
  --font-sans: 'Microsoft YaHei UI', 'Segoe UI', sans-serif;
  --font-mono: 'Cascadia Code', 'JetBrains Mono', 'Consolas', monospace;
  --ch0: #3B82F6;
  --ch1: #22C55E;
  --ch2: #F59E0B;
  --ch3: #A855F7;
}

* { box-sizing: border-box; margin: 0; padding: 0; }

body {
  background: var(--bg-app);
  color: var(--text-primary);
  font-family: var(--font-sans);
  font-size: 14px;
  user-select: none;
}

.app-shell {
  display: flex;
  flex-direction: column;
  height: 100vh;
  overflow: hidden;
}

.app-body {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 8px;
  overflow: hidden;
}
</style>
