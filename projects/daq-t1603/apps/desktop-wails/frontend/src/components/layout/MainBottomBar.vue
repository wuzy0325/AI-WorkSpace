<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount, computed, watch } from 'vue'
import { Play, Square, Timer, Clock, Circle, FolderOpen } from '@lucide/vue'
import { useDeviceStore } from '@stores/deviceStore'
import { useRecordingStore } from '@stores/recordingStore'
import { pickDirectory } from '@bridge/recordingBridge'

const deviceStore = useDeviceStore()
const recordingStore = useRecordingStore()

const emit = defineEmits<{
  (e: 'toggle-acquisition'): void
}>()

const currentTime = ref('00:00:00')
const elapsedTime = ref('00:00:00')
const startTimestamp = ref<number | null>(null)
let timeTimer: ReturnType<typeof setInterval> | null = null
let elapsedTimer: ReturnType<typeof setInterval> | null = null

function updateTime() {
  const now = new Date()
  currentTime.value = now.toLocaleTimeString('zh-CN', {
    hour12: false,
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
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

const isAcquiring = computed(() =>
  deviceStore.profiles.some((p) => deviceStore.acquiringFor(p.id))
)
const hasAcquisitionCapable = computed(() => deviceStore.profiles.length > 0)
const totalDevices = computed(() => deviceStore.profiles.length)
const recordingCount = computed(() => recordingStore.snapshotCount)

onMounted(() => {
  updateTime()
  timeTimer = setInterval(updateTime, 1000)
  void recordingStore.refreshStatus()
})

onBeforeUnmount(() => {
  if (timeTimer !== null) clearInterval(timeTimer)
  if (elapsedTimer !== null) clearInterval(elapsedTimer)
})

watch(isAcquiring, (newVal, oldVal) => {
  if (newVal && !oldVal) {
    startTimestamp.value = Date.now()
    updateElapsedTime()
    elapsedTimer = setInterval(updateElapsedTime, 1000)
  } else if (!newVal && oldVal) {
    if (elapsedTimer !== null) {
      clearInterval(elapsedTimer)
      elapsedTimer = null
    }
    startTimestamp.value = null
    elapsedTime.value = '00:00:00'
  }
}, { immediate: true })

async function startSave() {
  const dir = await pickDirectory()
  if (!dir) return
  await recordingStore.startRecording(dir, 'DAQ-T1603')
}

function stopSave() {
  void recordingStore.stopRecording()
}
</script>

<template>
  <footer class="bottombar">
    <div class="bottombar__left">
      <div class="bottombar__controls">
        <button
          class="bottombar__btn"
          :class="isAcquiring ? 'bottombar__btn--stop' : 'bottombar__btn--start'"
          :disabled="!hasAcquisitionCapable"
          @click="emit('toggle-acquisition')"
          :title="isAcquiring ? '停止采集' : '开始采集'"
        >
          <Play v-if="!isAcquiring" class="bottombar__btn-icon" />
          <Square v-else class="bottombar__btn-icon" />
        </button>
        <button
          class="bottombar__btn bottombar__btn--record"
          :class="{ 'bottombar__btn--recording': recordingStore.isRecording }"
          :title="recordingStore.isRecording ? '停止记录' : '开始记录'"
          @click="recordingStore.isRecording ? stopSave() : startSave()"
        >
          <Circle class="bottombar__btn-icon" />
        </button>
      </div>

      <div class="bottombar__status">
        <div class="bottombar__status-item">
          <span class="bottombar__status-label">采集状态</span>
          <span class="bottombar__status-value" :class="{ 'bottombar__status-value--active': isAcquiring }">
            {{ isAcquiring ? '运行中' : '已停止' }}
          </span>
        </div>
        <div class="bottombar__status-item">
          <span class="bottombar__status-label">设备</span>
          <span class="bottombar__status-value mono">{{ totalDevices }}</span>
        </div>
        <div v-if="recordingStore.isRecording" class="bottombar__status-item">
          <span class="bottombar__status-label">已记录</span>
          <span class="bottombar__status-value mono bottombar__status-value--rec">{{ recordingCount }}</span>
        </div>
        <div v-if="recordingStore.isRecording" class="bottombar__rec-folder">
          <FolderOpen class="bottombar__rec-icon" />
          <span class="bottombar__rec-path">{{ recordingStore.outputDir }}</span>
        </div>
      </div>
    </div>

    <div class="bottombar__stats">
      <div class="bottombar__stat">
        <span class="bottombar__stat-label">运行时间</span>
        <div class="bottombar__stat-value">
          <Timer class="bottombar__stat-icon bottombar__stat-icon--accent" />
          <span class="mono">{{ elapsedTime }}</span>
        </div>
      </div>
      <div class="bottombar__stat">
        <span class="bottombar__stat-label">系统时间</span>
        <div class="bottombar__stat-value">
          <Clock class="bottombar__stat-icon" />
          <span class="mono bottombar__stat-value--muted">{{ currentTime }}</span>
        </div>
      </div>
    </div>
  </footer>
</template>

<style scoped>
.bottombar {
  height: var(--layout-bottombar-height);
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 2rem;
  background: var(--bottombar-bg);
  backdrop-filter: blur(14px) saturate(140%);
  -webkit-backdrop-filter: blur(14px) saturate(140%);
  border-top: 1px solid var(--border-default);
  box-shadow: var(--shadow-lg);
}

[data-theme='light'] .bottombar {
  box-shadow: 0 -20px 40px rgba(15, 23, 42, 0.05);
}

.bottombar__left {
  display: flex;
  align-items: center;
  gap: 1.75rem;
  min-width: 0;
  flex: 1;
}

.bottombar__controls {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.bottombar__btn {
  width: 44px;
  height: 44px;
  border-radius: var(--radius-lg);
  display: flex;
  align-items: center;
  justify-content: center;
  transition: all 0.2s ease;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.18);
}

.bottombar__btn:hover:not(:disabled) {
  transform: translateY(-1px);
}

.bottombar__btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
  box-shadow: none;
}

.bottombar__btn--start {
  background: linear-gradient(135deg, var(--accent) 0%, var(--accent-active) 100%);
  color: #ffffff;
  box-shadow: 0 4px 14px var(--accent-glow);
}

.bottombar__btn--start:hover:not(:disabled) {
  box-shadow: 0 6px 18px var(--accent-glow);
}

.bottombar__btn--stop {
  background: var(--danger-muted);
  color: var(--danger);
  border: 1px solid var(--danger-border);
}

.bottombar__btn--stop:hover:not(:disabled) {
  background: color-mix(in srgb, var(--danger-muted) 160%, transparent);
}

.bottombar__btn--record {
  background: var(--btn-bg);
  color: var(--text-secondary);
  border: 1px solid var(--border-default);
}

.bottombar__btn--recording {
  background: rgba(244, 63, 94, 0.12);
  color: var(--danger);
  border-color: rgba(244, 63, 94, 0.35);
  animation: record-pulse 1.8s ease-in-out infinite;
}

.bottombar__btn-icon {
  width: 18px;
  height: 18px;
  fill: currentColor;
}

.bottombar__status {
  display: flex;
  align-items: center;
  gap: 1.5rem;
  min-width: 0;
}

.bottombar__status-item {
  display: flex;
  flex-direction: column;
  gap: 0.15rem;
}

.bottombar__status-label {
  font-size: 0.55rem;
  font-weight: 600;
  color: var(--text-muted);
}

.bottombar__status-value {
  font-size: var(--font-size-base);
  font-weight: 700;
  color: var(--text-primary);
}

.bottombar__status-value--active {
  color: var(--accent);
}

.bottombar__status-value--rec {
  color: var(--danger);
}

.bottombar__rec-folder {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.3rem 0.65rem;
  background: var(--btn-bg);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  min-width: 0;
  max-width: 22rem;
}

.bottombar__rec-icon {
  width: 12px;
  height: 12px;
  color: var(--text-muted);
  flex-shrink: 0;
}

.bottombar__rec-path {
  font-size: 0.7rem;
  color: var(--text-secondary);
  font-family: var(--font-family-mono);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.bottombar__stats {
  display: flex;
  align-items: center;
  gap: 2.5rem;
  flex-shrink: 0;
}

.bottombar__stat {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 0.2rem;
}

.bottombar__stat-label {
  font-size: 0.55rem;
  font-weight: 600;
  color: var(--text-muted);
}

.bottombar__stat-value {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  font-size: var(--font-size-xl);
  font-weight: 800;
  color: var(--text-primary);
  letter-spacing: -0.02em;
}

.bottombar__stat-value--muted {
  color: var(--text-secondary);
  font-weight: 700;
}

.bottombar__stat-icon {
  width: 16px;
  height: 16px;
  color: var(--text-muted);
}

.bottombar__stat-icon--accent {
  color: var(--accent);
}

@media (max-width: 1024px) {
  .bottombar__rec-folder,
  .bottombar__stats {
    display: none;
  }
}
</style>
