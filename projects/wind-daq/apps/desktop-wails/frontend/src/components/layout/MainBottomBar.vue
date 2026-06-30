<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount, computed, watch } from 'vue'
import { Timer, Clock } from '@lucide/vue'

const props = withDefaults(
  defineProps<{
    isAcquiring: boolean
    t?: Record<string, string>
    totalDevices: number
  }>(),
  {
    t: () => ({}),
  }
)

const emit = defineEmits<{
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
    <!-- Left: Status Info -->
    <div class="main-bottom-bar__left">
      <!-- Status Pill (从顶部栏移入) -->
      <div
        data-test="system-status-pill"
        class="main-bottom-bar__status-pill"
        :class="isAcquiring ? 'main-bottom-bar__status-pill--active' : 'main-bottom-bar__status-pill--idle'"
      >
        <span class="main-bottom-bar__status-dot" :class="{ 'status-pulse': isAcquiring }"></span>
        <span data-test="system-status-label" class="main-bottom-bar__status-text">
          {{ isAcquiring ? (t.acquiring || '采集开启') : (t.idle || '就绪') }}
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
          <Timer class="w-4 h-4 text-[var(--state-success)]" />
          <span class="mono-font">{{ elapsedTime }}</span>
        </div>
      </div>

      <!-- System Time -->
      <div class="main-bottom-bar__stat">
        <span class="main-bottom-bar__stat-label">{{ t.systemTime || '系统时间' }}</span>
        <div class="main-bottom-bar__stat-value">
          <Clock class="w-4 h-4 text-[var(--text-tertiary)]" />
          <span class="mono-font text-[var(--text-tertiary)]">{{ currentTime }}</span>
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
  background: var(--bg-panel-strong);
  border-top: 1px solid var(--border-default);
}

:root[data-theme='light'] .main-bottom-bar {
  background: var(--bg-panel-strong);
  border-top: 1px solid var(--border-default);
}

.main-bottom-bar__left {
  display: flex;
  align-items: center;
  gap: 1.5rem;
}

/* 状态 Pill 样式（从顶部栏移入） */
.main-bottom-bar__status-pill {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.25rem 0.75rem;
  border-radius: 9999px;
  font-size: 0.75rem;
  font-weight: 700;
  letter-spacing: 0.05em;
  text-transform: uppercase;
}

.main-bottom-bar__status-pill--active {
  background: color-mix(in srgb, var(--accent-primary) 10%, transparent);
  border: 1px solid color-mix(in srgb, var(--accent-primary) 20%, transparent);
  color: var(--accent-primary);
}

.main-bottom-bar__status-pill--idle {
  background: rgba(148, 163, 184, 0.1);
  border: 1px solid rgba(148, 163, 184, 0.2);
  color: var(--text-muted);
}

.main-bottom-bar__status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: currentColor;
}

.status-pulse {
  animation: status-pulse 2s ease-in-out infinite;
}

@keyframes status-pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.4; }
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
