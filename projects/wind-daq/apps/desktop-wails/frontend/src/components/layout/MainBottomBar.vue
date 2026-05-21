<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount, computed, watch } from 'vue'
import { Play, Square, Circle, Timer, Clock } from '@lucide/vue'

const props = withDefaults(
  defineProps<{
    isAcquiring?: boolean
    isRecording?: boolean
    totalDevices?: number
  }>(),
  {
    isAcquiring: false,
    isRecording: false,
    totalDevices: 0,
  },
)

const emit = defineEmits<{
  (e: 'start'): void
  (e: 'stop'): void
  (e: 'toggleRecording'): void
}>()

const currentTime = ref('')
const elapsedTime = ref('00:00:00')
const startTimestamp = ref<number | null>(null)
let timeTimer: number | null = null
let elapsedTimer: number | null = null

function updateTime() {
  currentTime.value = new Date().toLocaleTimeString('zh-CN', {
    hour12: false,
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  })
}

function updateElapsed() {
  if (!startTimestamp.value) {
    elapsedTime.value = '00:00:00'
    return
  }
  const elapsed = Date.now() - startTimestamp.value
  const h = Math.floor(elapsed / 3600000)
  const m = Math.floor((elapsed % 3600000) / 60000)
  const s = Math.floor((elapsed % 60000) / 1000)
  elapsedTime.value = `${String(h).padStart(2, '0')}:${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`
}

const running = computed(() => props.isAcquiring)

watch(running, (val, prev) => {
  if (val && !prev) {
    startTimestamp.value = Date.now()
    updateElapsed()
    elapsedTimer = window.setInterval(updateElapsed, 1000)
  } else if (!val && prev) {
    if (elapsedTimer !== null) {
      clearInterval(elapsedTimer)
      elapsedTimer = null
    }
    startTimestamp.value = null
    elapsedTime.value = '00:00:00'
  }
})

onMounted(() => {
  updateTime()
  timeTimer = window.setInterval(updateTime, 1000)
})

onBeforeUnmount(() => {
  if (timeTimer !== null) { clearInterval(timeTimer); timeTimer = null }
  if (elapsedTimer !== null) { clearInterval(elapsedTimer); elapsedTimer = null }
})
</script>

<template>
  <footer class="main-bottom-bar">
    <div class="main-bottom-bar__left">
      <div class="main-bottom-bar__controls">
        <button
          class="main-bottom-bar__btn start"
          :disabled="isAcquiring"
          @click="emit('start')"
          title="Start acquisition"
        >
          <Play class="w-5 h-5 fill-current" />
        </button>
        <button
          class="main-bottom-bar__btn stop"
          :disabled="!isAcquiring"
          @click="emit('stop')"
          title="Stop acquisition"
        >
          <Square class="w-4 h-4 fill-current" />
        </button>
        <button
          class="main-bottom-bar__btn record"
          :class="{ active: isRecording }"
          @click="emit('toggleRecording')"
          title="Toggle recording"
        >
          <Circle class="w-4 h-4 fill-current" />
        </button>
      </div>

      <div class="main-bottom-bar__status">
        <span class="main-bottom-bar__label">Status</span>
        <strong :class="isAcquiring ? 'text--acquiring' : 'text--idle'">
          {{ isAcquiring ? 'Running' : 'Idle' }}
        </strong>
      </div>

      <div class="main-bottom-bar__stat-item">
        <span class="main-bottom-bar__label">Devices</span>
        <strong>{{ totalDevices }}</strong>
      </div>
    </div>

      <div class="main-bottom-bar__right">
      <div class="main-bottom-bar__stat-item">
        <span class="main-bottom-bar__label">Elapsed</span>
        <div class="main-bottom-bar__stat-row">
          <Timer class="w-4 h-4 text-emerald-500" />
          <strong class="mono">{{ elapsedTime }}</strong>
        </div>
      </div>
      <div class="main-bottom-bar__stat-item">
        <span class="main-bottom-bar__label">Clock</span>
        <div class="main-bottom-bar__stat-row">
          <Clock class="w-4 h-4 text-slate-400" />
          <strong class="mono text--muted">{{ currentTime }}</strong>
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

.main-bottom-bar__left,
.main-bottom-bar__right {
  display: flex;
  align-items: center;
  gap: 1.5rem;
}

.main-bottom-bar__controls {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

.main-bottom-bar__btn {
  width: 44px;
  height: 44px;
  border-radius: 0.75rem;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 0.875rem;
  font-weight: 900;
  transition: all 0.2s ease;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
}

.main-bottom-bar__btn:hover:not(:disabled) {
  transform: translateY(-2px);
}

.main-bottom-bar__btn.start {
  background: #10b981;
  color: white;
  box-shadow: 0 4px 12px rgba(16, 185, 129, 0.3);
}

.main-bottom-bar__btn.start:hover {
  background: #059669;
  box-shadow: 0 6px 16px rgba(16, 185, 129, 0.4);
}

.main-bottom-bar__btn.stop {
  background: rgba(244, 63, 94, 0.1);
  color: #f43f5e;
  border: 1px solid rgba(244, 63, 94, 0.2);
}

.main-bottom-bar__btn.stop:hover {
  background: rgba(244, 63, 94, 0.2);
}

.main-bottom-bar__btn.record {
  background: rgba(148, 163, 184, 0.1);
  color: var(--text-muted);
  border: 1px solid rgba(148, 163, 184, 0.2);
}

.main-bottom-bar__btn.record.active {
  color: #ef4444;
  border-color: rgba(239, 68, 68, 0.3);
  background: rgba(239, 68, 68, 0.1);
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

.main-bottom-bar__status,
.main-bottom-bar__stat-item {
  display: flex;
  flex-direction: column;
  gap: 0.125rem;
}

.main-bottom-bar__stat-row {
  display: flex;
  align-items: center;
  gap: 0.4rem;
}

.main-bottom-bar__label {
  font-size: 0.625rem;
  font-weight: 700;
  color: var(--text-muted);
  letter-spacing: 0.05em;
  text-transform: uppercase;
}

.main-bottom-bar__status strong,
.main-bottom-bar__stat-item strong {
  font-size: 1.25rem;
  font-weight: 800;
  color: var(--text-primary);
}

.text--acquiring {
  color: var(--accent-success);
}

.text--idle {
  color: var(--text-muted);
}

.text--muted {
  color: var(--text-muted);
}

.mono {
  font-family: var(--font-family-mono, monospace);
  font-variant-numeric: tabular-nums;
}
</style>
