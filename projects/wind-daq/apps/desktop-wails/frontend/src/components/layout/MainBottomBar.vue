<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount, computed, watch } from 'vue'
import { Play, Square, Timer, Clock, Circle } from '@lucide/vue'
import UiButton from '@components/ui/UiButton.vue'

const props = withDefaults(
  defineProps<{
    isAcquiring: boolean
    isRecording?: boolean
    t?: Record<string, string>
    totalDevices: number
  }>(),
  {
    isRecording: false,
    t: () => ({}),
  }
)

const emit = defineEmits<{
  (e: 'start'): void
  (e: 'stop'): void
  (e: 'toggle-recording'): void
}>()

const currentTime = ref('12:00:00')
const elapsedTime = ref('00:00:00')
const startTimestamp = ref<number | null>(null)
let timeTimer: number | null = null
let elapsedTimer: number | null = null

function updateTime() {
  const now = new Date()
  currentTime.value = now.toLocaleTimeString('zh-CN', {
    hour12: false,
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit'
  })
}

function updateElapsedTime() {
  if (!startTimestamp.value) {
    elapsedTime.value = '00:00:00'
    return
  }
  const now = Date.now()
  const elapsed = now - startTimestamp.value
  const hours = Math.floor(elapsed / 3600000)
  const minutes = Math.floor((elapsed % 3600000) / 60000)
  const seconds = Math.floor((elapsed % 60000) / 1000)
  elapsedTime.value = `${String(hours).padStart(2, '0')}:${String(minutes).padStart(2, '0')}:${String(seconds).padStart(2, '0')}`
}

const isRunning = computed(() => props.isAcquiring)

onMounted(() => {
  updateTime()
  timeTimer = window.setInterval(updateTime, 1000)
})

onBeforeUnmount(() => {
  if (timeTimer !== null) {
    window.clearInterval(timeTimer)
    timeTimer = null
  }
  if (elapsedTimer !== null) {
    window.clearInterval(elapsedTimer)
    elapsedTimer = null
  }
})

watch(isRunning, (newVal, oldVal) => {
  if (newVal && !oldVal) {
    startTimestamp.value = Date.now()
    updateElapsedTime()
    elapsedTimer = window.setInterval(updateElapsedTime, 1000)
  } else if (!newVal && oldVal) {
    if (elapsedTimer !== null) {
      window.clearInterval(elapsedTimer)
      elapsedTimer = null
    }
    startTimestamp.value = null
    elapsedTime.value = '00:00:00'
  }
}, { immediate: true })
</script>

<template>
  <footer class="main-bottom-bar">
    <!-- Left: Control Buttons -->
    <div class="main-bottom-bar__left">
      <div class="main-bottom-bar__controls">
        <UiButton
          data-test="acquisition-toggle-btn"
          class="main-bottom-bar__btn"
          :class="isAcquiring ? 'btn-stop' : 'btn-start'"
          @click="isAcquiring ? emit('stop') : emit('start')"
          :title="isAcquiring ? (t.stopAcquisition || '停止采集') : (t.startAcquisition || '开始采集')"
        >
          <template #icon>
            <Play v-if="!isAcquiring" class="w-5 h-5 fill-current" />
            <Square v-else class="w-4 h-4 fill-current" />
          </template>
        </UiButton>
        <UiButton
          data-test="recording-toggle-btn"
          class="main-bottom-bar__btn btn-record"
          :class="{ active: isRecording }"
          @click="emit('toggle-recording')"
          :title="isRecording ? (t.stopRecording || '停止记录') : (t.startRecording || '开始记录')"
        >
          <template #icon>
            <Circle class="w-4 h-4 fill-current" />
          </template>
        </UiButton>
      </div>

      <!-- Status -->
      <div class="main-bottom-bar__status-item">
        <span class="main-bottom-bar__status-label">{{ t.acquisitionStatusLabel || '状态' }}</span>
        <span class="main-bottom-bar__status-value" :class="isAcquiring ? 'text-emerald-500' : 'text-slate-500'">
          {{ isAcquiring ? (t.acquiring || '运行中') : (t.idle || '已停止') }}
        </span>
      </div>

      <!-- Device Count -->
      <div class="main-bottom-bar__status-item">
        <span class="main-bottom-bar__status-label">{{ t.totalDevices || '设备' }}</span>
        <span class="main-bottom-bar__status-value">{{ totalDevices }}</span>
      </div>
    </div>

    <!-- Right: Time Stats -->
    <div class="main-bottom-bar__stats">
      <!-- Elapsed Time -->
      <div class="main-bottom-bar__stat">
        <span class="main-bottom-bar__stat-label">{{ t.elapsedTime || '运行时间' }}</span>
        <div class="main-bottom-bar__stat-value">
          <Timer class="w-4 h-4 text-emerald-500" />
          <span class="mono-font">{{ elapsedTime }}</span>
        </div>
      </div>

      <!-- System Time -->
      <div class="main-bottom-bar__stat">
        <span class="main-bottom-bar__stat-label">{{ t.systemTime || '系统时间' }}</span>
        <div class="main-bottom-bar__stat-value">
          <Clock class="w-4 h-4 text-slate-400" />
          <span class="mono-font text-slate-400">{{ currentTime }}</span>
        </div>
      </div>
    </div>
  </footer>
</template>

<style scoped>
.main-bottom-bar {
  height: 64px;
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 2rem;
  background: rgba(30, 41, 59, 0.75);
  backdrop-filter: blur(12px);
  -webkit-backdrop-filter: blur(12px);
  border-top: 1px solid rgba(255, 255, 255, 0.08);
  box-shadow: 0 -20px 50px rgba(0, 0, 0, 0.1);
}

:root[data-theme='light'] .main-bottom-bar {
  background: rgba(255, 255, 255, 0.75);
  border-top: 1px solid rgba(0, 0, 0, 0.05);
  box-shadow: 0 -20px 50px rgba(0, 0, 0, 0.05);
}

.main-bottom-bar__left {
  display: flex;
  align-items: center;
  gap: 1.5rem;
}

.main-bottom-bar__controls {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

:deep(.main-bottom-bar__btn) {
  width: 44px;
  height: 44px;
  display: flex;
  align-items: center;
  justify-content: center;
}

:deep(.main-bottom-bar__btn):hover:not(:disabled) {
  transform: translateY(-2px);
}

:deep(.btn-start) {
  background: var(--accent-primary);
  color: white;
  box-shadow: 0 4px 12px color-mix(in srgb, var(--accent-primary) 30%, transparent);
}

:deep(.btn-start):hover {
  background: var(--accent-primary-core-strong);
}

:deep(.btn-stop) {
  background: rgba(244, 63, 94, 0.1);
  color: #f43f5e;
  border: 1px solid rgba(244, 63, 94, 0.2);
}

:deep(.btn-stop):hover {
  background: rgba(244, 63, 94, 0.2);
}

:deep(.btn-record) {
  background: rgba(148, 163, 184, 0.1);
  color: var(--text-muted);
  border: 1px solid rgba(148, 163, 184, 0.2);
}

:deep(.btn-record.active) {
  background: rgba(239, 68, 68, 0.1);
  color: #ef4444;
  border-color: rgba(239, 68, 68, 0.3);
  animation: pulse-record 2s infinite;
}

@keyframes pulse-record {
  0%, 100% {
    box-shadow: 0 0 0 0 rgba(239, 68, 68, 0.4);
  }
  50% {
    box-shadow: 0 0 0 8px rgba(239, 68, 68, 0);
  }
}

.main-bottom-bar__status-item {
  display: flex;
  flex-direction: column;
  gap: 0.125rem;
}

.main-bottom-bar__status-label {
  font-size: var(--font-size-micro);
  font-weight: 700;
  color: var(--text-muted);
  letter-spacing: 0.05em;
  text-transform: uppercase;
}

.main-bottom-bar__status-value {
  font-size: 0.875rem;
  font-weight: 700;
}

.main-bottom-bar__stats {
  display: flex;
  align-items: center;
  gap: 3rem;
}

.main-bottom-bar__stat {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.25rem;
}

.main-bottom-bar__stat-label {
  font-size: var(--font-size-micro);
  font-weight: 700;
  color: var(--text-muted);
  letter-spacing: 0.1em;
  text-transform: uppercase;
}

.main-bottom-bar__stat-value {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 1.25rem;
  font-weight: 800;
  color: var(--text-primary);
  letter-spacing: -0.02em;
}

:root[data-theme='light'] .main-bottom-bar__stat-value {
  color: var(--text-primary);
}

.mono-font {
  font-family: ui-monospace, SFMono-Regular, 'SF Mono', Menlo, Consolas, monospace;
  font-variant-numeric: tabular-nums;
}
</style>
