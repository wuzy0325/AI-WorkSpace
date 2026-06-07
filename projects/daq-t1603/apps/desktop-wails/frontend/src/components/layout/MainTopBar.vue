<script setup lang="ts">
import { computed } from 'vue'
import { Sun, Moon, Activity, Settings2, Plus, CircleDot, Play, Square, Circle } from '@lucide/vue'
import { useDeviceStore } from '@stores/deviceStore'
import { useRecordingStore } from '@stores/recordingStore'
import { useTheme } from '@composables/useTheme'
import { pickDirectory } from '@bridge/recordingBridge'

defineProps<{
  version: string
}>()

const emit = defineEmits<{
  (e: 'add-device'): void
  (e: 'open-config'): void
  (e: 'toggle-acquisition'): void
}>()

const deviceStore = useDeviceStore()
const recordingStore = useRecordingStore()
const { theme, toggle: toggleTheme } = useTheme()

const acquiringDevices = computed(
  () => deviceStore.profiles.filter((p) => deviceStore.acquiringFor(p.id)).length
)

const isAcquiring = computed(() => acquiringDevices.value > 0)
const hasConnectedDevice = computed(
  () => deviceStore.profiles.some((p) => {
    const status = deviceStore.statusFor(p.id)
    return status === 'Connected' || status === 'Acquiring' || status === 'Starting'
  })
)
const canToggleAcquisition = computed(() => isAcquiring.value || hasConnectedDevice.value)

function themeToggleLabel(): string {
  return theme.value === 'dark' ? '切换为浅色模式' : '切换为深色模式'
}

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
  <header class="topbar">
    <div class="topbar__content">
      <div class="topbar__brand">
        <div class="topbar__logo">
          <Activity class="topbar__logo-icon" />
        </div>
        <div class="topbar__title-wrap">
          <h1 class="topbar__title">
            DAQ-T<span class="topbar__title-accent">1603</span>
          </h1>
          <p class="topbar__subtitle">Temperature Acquisition</p>
        </div>
      </div>

      <div class="topbar__nav">
        <div class="topbar__nav-btn topbar__nav-btn--active">
          <CircleDot class="topbar__nav-icon" />
          实时监控
        </div>
      </div>

      <div class="topbar__actions">
        <div class="topbar__primary-actions">
          <button
            class="topbar__action-btn"
            :class="isAcquiring ? 'topbar__action-btn--stop' : 'topbar__action-btn--start'"
            :disabled="!canToggleAcquisition"
            :title="isAcquiring ? '停止采集' : '开始采集'"
            @click="emit('toggle-acquisition')"
          >
            <Play v-if="!isAcquiring" class="topbar__action-icon" />
            <Square v-else class="topbar__action-icon" />
            <span>{{ isAcquiring ? '停止采集' : '开始采集' }}</span>
          </button>

          <button
            class="topbar__action-btn topbar__action-btn--record"
            :class="{ 'topbar__action-btn--recording': recordingStore.isRecording }"
            :title="recordingStore.isRecording ? '停止保存' : '开始保存'"
            @click="recordingStore.isRecording ? stopSave() : startSave()"
          >
            <Circle class="topbar__action-icon" />
            <span>{{ recordingStore.isRecording ? '停止保存' : '开始保存' }}</span>
          </button>
        </div>

        <button
          class="topbar__icon-btn"
          :title="'添加设备'"
          @click="emit('add-device')"
        >
          <Plus class="topbar__icon" />
        </button>

        <button
          class="topbar__icon-btn"
          :title="'打开配置'"
          @click="emit('open-config')"
        >
          <Settings2 class="topbar__icon" />
        </button>

        <button
          class="topbar__icon-btn"
          @click="toggleTheme"
          :aria-label="themeToggleLabel()"
          :title="themeToggleLabel()"
        >
          <Sun v-if="theme === 'dark'" class="topbar__icon" />
          <Moon v-else class="topbar__icon" />
        </button>

        <span class="topbar__version">v{{ version }}</span>
      </div>
    </div>
  </header>
</template>

<style scoped>
.topbar {
  height: var(--layout-header-height);
  flex-shrink: 0;
  background: var(--topbar-bg);
  backdrop-filter: blur(14px) saturate(140%);
  -webkit-backdrop-filter: blur(14px) saturate(140%);
  border-bottom: 1px solid var(--border-default);
  position: relative;
  z-index: 50;
}

.topbar__content {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 100%;
  padding: 0 1.5rem;
  gap: 1.5rem;
}

.topbar__brand {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  flex-shrink: 0;
}

.topbar__logo {
  width: 36px;
  height: 36px;
  background: linear-gradient(135deg, var(--accent) 0%, var(--accent-active) 100%);
  border-radius: var(--radius-md);
  display: flex;
  align-items: center;
  justify-content: center;
  box-shadow: 0 6px 16px var(--accent-glow);
}

.topbar__logo-icon {
  width: 20px;
  height: 20px;
  color: #ffffff;
}

.topbar__title-wrap {
  display: flex;
  flex-direction: column;
  line-height: 1;
}

.topbar__title {
  font-size: var(--font-size-lg);
  font-weight: 800;
  letter-spacing: -0.02em;
  color: var(--text-primary);
}

.topbar__title-accent {
  color: var(--accent);
}

.topbar__subtitle {
  font-size: 0.5rem;
  font-weight: 600;
  color: var(--text-muted);
  letter-spacing: 0.18em;
  text-transform: uppercase;
  margin-top: 0.25rem;
}

@media (max-width: 1280px) {
  .topbar__subtitle {
    display: none;
  }
}

.topbar__nav {
  display: flex;
  align-items: center;
  gap: 0.25rem;
  padding: 0.25rem;
  background: var(--btn-bg);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  flex-shrink: 0;
}

.topbar__nav-btn {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.4rem 0.875rem;
  font-size: var(--font-size-sm);
  font-weight: 600;
  color: var(--text-secondary);
  border-radius: var(--radius-sm);
  transition: all 0.2s ease;
  white-space: nowrap;
}

.topbar__nav-btn:hover {
  color: var(--text-primary);
}

.topbar__nav-btn--active {
  background: var(--accent-muted);
  color: var(--accent);
  box-shadow: inset 0 -2px 0 var(--accent);
}

.topbar__nav-icon {
  width: 14px;
  height: 14px;
}

.topbar__actions {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  flex-shrink: 0;
}

.topbar__primary-actions {
  display: flex;
  align-items: center;
  gap: 0.6rem;
}

.topbar__action-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0.45rem;
  min-width: 7rem;
  height: 2.5rem;
  padding: 0 1rem;
  border-radius: var(--radius-lg);
  font-size: var(--font-size-sm);
  font-weight: 700;
  transition: all 0.2s ease;
}

.topbar__action-btn:hover:not(:disabled) {
  transform: translateY(-1px);
}

.topbar__action-btn:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}

.topbar__action-btn--start {
  color: #ffffff;
  background: linear-gradient(135deg, var(--accent) 0%, var(--accent-active) 100%);
  box-shadow: 0 6px 18px var(--accent-glow);
}

.topbar__action-btn--stop {
  color: var(--danger);
  background: var(--danger-muted);
  border: 1px solid var(--danger-border);
}

.topbar__action-btn--record {
  color: var(--text-secondary);
  background: var(--btn-bg);
  border: 1px solid var(--border-default);
}

.topbar__action-btn--recording {
  color: var(--danger);
  border-color: rgba(244, 63, 94, 0.35);
  background: rgba(244, 63, 94, 0.12);
}

.topbar__action-icon {
  width: 16px;
  height: 16px;
  fill: currentColor;
}

.topbar__icon-btn {
  width: 36px;
  height: 36px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: var(--radius-md);
  color: var(--text-secondary);
  background: var(--btn-bg);
  border: 1px solid var(--border-default);
  transition: all 0.2s ease;
}

.topbar__icon-btn:hover {
  color: var(--accent);
  background: var(--accent-soft);
  border-color: var(--accent-border);
}

.topbar__icon {
  width: 16px;
  height: 16px;
}

.topbar__version {
  font-size: 0.6rem;
  font-weight: 600;
  color: var(--text-muted);
  font-family: var(--font-family-mono);
  padding-left: 0.25rem;
}

@media (max-width: 1536px) {
  .topbar__version {
    display: none;
  }
}

@media (max-width: 1024px) {
  .topbar__content {
    gap: 0.75rem;
    padding: 0 0.9rem;
  }

  .topbar__nav {
    display: none;
  }

  .topbar__primary-actions {
    gap: 0.45rem;
  }

  .topbar__action-btn {
    min-width: auto;
    height: 2.15rem;
    padding: 0 0.7rem;
    font-size: var(--font-size-xs);
  }

  .topbar__action-btn span {
    white-space: nowrap;
  }

  .topbar__actions {
    gap: 0.45rem;
  }
}

@media (max-width: 767px) {
  .topbar__content {
    gap: 0.5rem;
    padding: 0 0.7rem;
  }

  .topbar__title {
    font-size: var(--font-size-base);
  }

  .topbar__primary-actions {
    gap: 0.35rem;
  }

  .topbar__action-btn {
    padding: 0 0.55rem;
  }

  .topbar__action-btn span {
    display: none;
  }

  .topbar__icon-btn {
    width: 32px;
    height: 32px;
  }
}
</style>
