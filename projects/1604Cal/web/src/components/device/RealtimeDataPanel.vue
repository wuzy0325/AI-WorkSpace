<template>
  <section class="realtime-data-panel">
    <header class="panel-header">
      <div class="header-title">
        <el-icon class="panel-icon">
          <DataLine />
        </el-icon>
        <h2>实时数据监控</h2>
      </div>
      <div class="header-actions">
        <div class="device-status-group">
          <span
            class="device-status-badge"
            :class="measureDeviceStatusClass"
          >
            计量: {{ measureDeviceStatusText }}
          </span>
          <span
            class="device-status-badge"
            :class="pressureDeviceStatusClass"
          >
            打压: {{ pressureDeviceStatusText }}
          </span>
        </div>
        <span class="update-time">{{ lastUpdateTime }}</span>
        <button
          type="button"
          class="icon-btn"
          title="重新连接"
          @click="reconnect"
        >
          <el-icon><Refresh /></el-icon>
        </button>
      </div>
    </header>

    <!-- 紧凑指标栏 -->
    <RealtimeMetricsBar
      :current-pressure="currentPressure"
      :target-pressure="effectiveTargetPressure"
      :is-stable="isStable"
      :stable-duration="stableDuration"
      :stability-threshold="stabilityThreshold"
    />

    <!-- 通道数据网格 -->
    <RealtimeChannelGrid
      :channel-data="channelData"
      :active-channels="activeChannels"
      :total-channels="totalChannels"
    />

    <!-- 采集进度 -->
    <div
      v-if="showProgress"
      class="progress-section"
    >
      <div class="progress-header">
        <div class="progress-title">
          <el-icon><Histogram /></el-icon>
          <span>采集进度</span>
        </div>
        <span class="progress-percent">{{ progressPercent }}%</span>
      </div>
      <div
        class="progress-bar"
        role="progressbar"
        :aria-valuenow="completedPoints"
        aria-valuemin="0"
        :aria-valuemax="totalPoints"
      >
        <div
          class="progress-fill"
          :style="{ transform: 'scaleX(' + progressPercent / 100 + ')' }"
        />
      </div>
      <div class="progress-meta">
        <span>已完成: {{ completedPoints }}/{{ totalPoints }} 点</span>
        <span>预计剩余: {{ estimatedTime }}</span>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { DataLine, Refresh, Histogram } from '@element-plus/icons-vue'
import { createEventStream } from "@/api/client"
import { fetchDevices } from "@/api/device"
import {
  readPressure,
  readStability,
  readMeasureData
} from "@/api/session"
import type { StreamEventPayload } from "@/types/api"
import { useDeviceStore } from '@/stores/deviceStore'

import RealtimeMetricsBar from './RealtimeMetricsBar.vue'
import RealtimeChannelGrid from './RealtimeChannelGrid.vue'

// ---- 通道数据类型 ----

interface ChannelInfo {
  value: number | null
  status: 'ok' | 'warning' | 'error' | 'idle'
  isActive: boolean
}

// ---- Props ----

const props = withDefaults(defineProps<{
  targetPressure?: number | null
  completedPoints?: number
  totalPoints?: number
}>(), {
  targetPressure: null,
  completedPoints: 0,
  totalPoints: 0
})

// ---- 响应式数据 ----

const currentPressure = ref(0)
const eventTargetPressure = ref<number | null>(null)
const isStable = ref(false)
const stableDuration = ref(0)
const stabilityThreshold = ref(0.5)
const lastUpdateTime = ref('--:--:--')
const isConnected = ref(false)
const measureDeviceStatus = ref<'connected' | 'disconnected' | 'not_selected'>('not_selected')
const pressureDeviceStatus = ref<'connected' | 'disconnected' | 'not_selected'>('not_selected')

const channelData = ref<ChannelInfo[]>(
  Array.from({ length: 16 }, () => ({
    value: null,
    status: 'idle' as ChannelInfo['status'],
    isActive: false
  }))
)

const deviceStore = useDeviceStore()

// ---- 计算属性 ----

const totalPoints = computed(() => Math.max(0, Math.round(props.totalPoints)))
const totalChannels = computed(() => 16)

const completedPoints = computed(() => {
  const raw = Math.max(0, Math.round(props.completedPoints))
  if (totalPoints.value === 0) return 0
  return Math.min(totalPoints.value, raw)
})

const showProgress = computed(() => totalPoints.value > 0)

const estimatedTime = computed(() => {
  const remaining = totalPoints.value - completedPoints.value
  if (remaining <= 0) return '00:00'
  return '--'
})

const effectiveTargetPressure = computed(() => {
  if (typeof props.targetPressure === 'number' && Number.isFinite(props.targetPressure)) {
    return props.targetPressure
  }
  if (typeof eventTargetPressure.value === 'number' && Number.isFinite(eventTargetPressure.value)) {
    return eventTargetPressure.value
  }
  return null
})

const measureDeviceStatusText = computed(() => {
  switch (measureDeviceStatus.value) {
    case 'connected': return '已连接'
    case 'disconnected': return '未连接'
    default: return '未选择'
  }
})

const pressureDeviceStatusText = computed(() => {
  switch (pressureDeviceStatus.value) {
    case 'connected': return '已连接'
    case 'disconnected': return '未连接'
    default: return '未选择'
  }
})

const measureDeviceStatusClass = computed(() => ({
  'status-connected': measureDeviceStatus.value === 'connected',
  'status-disconnected': measureDeviceStatus.value === 'disconnected',
  'status-not-selected': measureDeviceStatus.value === 'not_selected'
}))

const pressureDeviceStatusClass = computed(() => ({
  'status-connected': pressureDeviceStatus.value === 'connected',
  'status-disconnected': pressureDeviceStatus.value === 'disconnected',
  'status-not-selected': pressureDeviceStatus.value === 'not_selected'
}))

const activeChannels = computed(() =>
  channelData.value.filter(ch => ch.isActive).length
)

const progressPercent = computed(() => {
  if (totalPoints.value <= 0) return 0
  return Math.round((completedPoints.value / totalPoints.value) * 100)
})

// ---- SSE 事件订阅 ----

let eventSource: EventSource | null = null
let pollInterval: ReturnType<typeof setInterval> | null = null

function setupSSE() {
  eventSource = createEventStream({
    onEvent: (payload: StreamEventPayload) => {
      const now = new Date()
      lastUpdateTime.value = now.toLocaleTimeString('zh-CN')

      if (payload.type === 'device.status.changed') {
        void syncConnectionFromDevices()
      }

      if (payload.type === 'pressure.applied') {
        const data = payload.data as { actualPressure?: number; targetPressure?: number }
        if (data?.actualPressure !== undefined) currentPressure.value = data.actualPressure
        if (data?.targetPressure !== undefined) eventTargetPressure.value = data.targetPressure
      }

      if (payload.type === 'data.collected') {
        const data = payload.data as { data?: number[]; channels?: number[] }
        if (data?.data && data?.channels) updateChannelData(data.data, data.channels)
      }
    },
    onError: (error) => {
      console.warn('[RealtimeDataPanel] SSE 连接断开:', error)
    }
  })
}

function updateChannelData(values: number[], channels: number[]) {
  const newChannelData: ChannelInfo[] = Array.from({ length: 16 }, () => ({
    value: null,
    status: 'idle' as ChannelInfo['status'],
    isActive: false
  }))

  channels.forEach((ch, idx) => {
    if (ch >= 1 && ch <= 16 && idx < values.length) {
      const chIdx = ch - 1
      const currentValue = values[idx]
      const compareTarget = effectiveTargetPressure.value ?? currentValue
      newChannelData[chIdx] = {
        value: currentValue,
        status: Math.abs(currentValue - compareTarget) > stabilityThreshold.value * 3 ? 'warning' : 'ok',
        isActive: true
      }
    }
  })

  channelData.value = newChannelData
}

// ---- 轮询 ----

async function pollRealtimeData() {
  try {
    const pressure = await readPressure()
    currentPressure.value = pressure
    isConnected.value = true
  } catch {
    // 设备未连接或不可用
  }

  try {
    const stable = await readStability()
    isStable.value = stable
    if (stable) {
      stableDuration.value += 1
    } else {
      stableDuration.value = 0
    }
  } catch {
    // 忽略
  }

  try {
    const data = await readMeasureData()
    if (data.length > 0) {
      const channels = Array.from({ length: Math.min(data.length, 16) }, (_, i) => i + 1)
      updateChannelData(data, channels)
    }
  } catch {
    // 忽略
  }
}

function reconnect() {
  if (eventSource) eventSource.close()
  setupSSE()
  syncConnectionFromDevices()
}

/** 从后端设备列表同步连接状态 */
async function syncConnectionFromDevices() {
  try {
    const devices = await fetchDevices()
    const selection = deviceStore.selectionByModule('measurement')
    const pressureId = selection.pressureDeviceId
    const measureId = selection.measureDeviceId

    const pressureDevice = devices.find(d => d.id === pressureId)
    const measureDevice = devices.find(d => d.id === measureId)

    measureDeviceStatus.value = measureId
      ? (measureDevice?.status === 'connected' ? 'connected' : 'disconnected')
      : 'not_selected'
    pressureDeviceStatus.value = pressureId
      ? (pressureDevice?.status === 'connected' ? 'connected' : 'disconnected')
      : 'not_selected'

    const pressureConnected = pressureId && pressureDevice?.status === 'connected'
    const measureConnected = measureId && measureDevice?.status === 'connected'
    isConnected.value = !!(pressureConnected && measureConnected)
  } catch {
    // 查询失败时保持当前状态
  }
}

// ---- 生命周期 ----

onMounted(() => {
  setupSSE()
  syncConnectionFromDevices()
  pollInterval = setInterval(pollRealtimeData, 2000)
  pollRealtimeData()
})

onUnmounted(() => {
  if (eventSource) {
    eventSource.close()
    eventSource = null
  }
  if (pollInterval) {
    clearInterval(pollInterval)
    pollInterval = null
  }
})
</script>

<style scoped lang="scss">
.realtime-data-panel {
  background: var(--bg-secondary);
  border: 1px solid var(--border-color);
  border-radius: 4px;
  padding: var(--spacing-sm);
  height: 100%;
  min-height: 0;
  display: grid;
  grid-template-rows: auto auto minmax(0, 1fr) auto;
  gap: var(--spacing-sm);
}

.panel-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 0;
  padding-bottom: var(--spacing-xs);
  border-bottom: 1px solid var(--border-color);
}

.header-title {
  display: flex;
  align-items: center;
  gap: var(--spacing-md);
}

.panel-icon {
  font-size: 20px;
  color: var(--accent-primary);
}

.header-title h2 {
  margin: 0;
  font-size: 14px;
  font-weight: 600;
  color: var(--text-primary);
}

.connection-status {
  display: inline-flex;
  align-items: center;
  gap: var(--spacing-xs);
  padding: 3px 8px;
  border-radius: 3px;
  font-size: 11px;
  font-weight: 600;

  .el-icon {
    font-size: 11px;
  }
}

.status-connected {
  background: var(--status-success-bg);
  color: var(--status-success);
}

.status-disconnected {
  background: var(--status-error-bg);
  color: var(--status-error);
}

.status-not-selected {
  background: var(--bg-tertiary);
  color: var(--text-muted);
}

.device-status-group {
  display: flex;
  gap: var(--spacing-xs);
}

.device-status-badge {
  display: inline-flex;
  align-items: center;
  padding: 3px 8px;
  border-radius: 3px;
  font-size: 11px;
  font-weight: 600;
}

.header-actions {
  display: flex;
  align-items: center;
  gap: var(--spacing-md);
}

.update-time {
  font-size: 12px;
  color: var(--text-muted);
  font-family: Consolas, monospace;
}

.icon-btn {
  width: 28px;
  height: 28px;
  border: 1px solid var(--border-color-strong);
  border-radius: 3px;
  background: var(--bg-tertiary);
  color: var(--text-secondary);
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: all 0.15s ease;

  .el-icon {
    font-size: 14px;
  }

  &:hover {
    background: var(--border-color);
    border-color: var(--accent-primary);
    color: var(--accent-primary);
  }
}

// ---- 采集进度 ----

.progress-section {
  background: var(--bg-tertiary);
  border: 1px solid var(--border-color);
  border-radius: 3px;
  padding: var(--spacing-sm) var(--spacing-md);
}

.progress-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 13px;
  font-weight: 500;
  color: var(--text-primary);
  margin-bottom: var(--spacing-sm);
}

.progress-title {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);

  .el-icon {
    font-size: 14px;
    color: var(--accent-primary);
  }
}

.progress-percent {
  color: var(--accent-primary);
  font-weight: 600;
}

.progress-bar {
  height: 4px;
  background: var(--bg-primary);
  border-radius: 2px;
  overflow: hidden;
  margin-bottom: var(--spacing-sm);
}

.progress-fill {
  height: 100%;
  background: var(--accent-primary);
  border-radius: 2px;
  width: 100%;
  transform-origin: left;
  transition: transform 0.3s ease;
}

.progress-meta {
  display: flex;
  justify-content: space-between;
  font-size: 11px;
  color: var(--text-secondary);
}

// ---- 响应式 ----

@media (max-width: 768px) {
  .panel-header {
    flex-direction: column;
    align-items: flex-start;
    gap: var(--spacing-sm);
  }
}
</style>