<script setup lang="ts">
import { computed, ref, onMounted, onBeforeUnmount, nextTick } from 'vue'
import { Sun, Moon, Activity, Settings2, Plus, CircleDot, Play, Square, Circle, Gauge } from '@lucide/vue'
import { useDeviceStore } from '@stores/deviceStore'
import { useDisplayStore } from '@stores/displayStore'
import { useRecordingStore } from '@stores/recordingStore'
import { useTheme } from '@composables/useTheme'
import { pickDirectory } from '@bridge/recordingBridge'

const emit = defineEmits<{
  (e: 'add-device'): void
  (e: 'open-config'): void
  (e: 'toggle-acquisition'): void
}>()

const props = defineProps<{
  version: string
  /** 操作进行中标志：用于禁用采集按钮防止重复点击 */
  isToggling?: boolean
}>()

const deviceStore = useDeviceStore()
const displayStore = useDisplayStore()
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

/** 采集按钮是否应该被禁用（操作进行中或无可操作设备） */
const isAcquisitionDisabled = computed(() => !canToggleAcquisition.value || props.isToggling)

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

// --- 刷新率下拉菜单 ---
const refreshRateOptions = [2, 5, 10, 15, 20, 30]
const showRefreshMenu = ref(false)
const refreshTriggerRef = ref<HTMLElement | null>(null)
const refreshDropdownRef = ref<HTMLElement | null>(null)
const refreshDropdownStyle = ref<Record<string, string>>({})

/** 根据触发器位置计算下拉菜单的 fixed 定位 */
function updateRefreshDropdownPosition() {
  if (!showRefreshMenu.value || !refreshTriggerRef.value) {
    refreshDropdownStyle.value = {}
    return
  }
  const rect = refreshTriggerRef.value.getBoundingClientRect()
  refreshDropdownStyle.value = {
    position: 'fixed',
    right: `${window.innerWidth - rect.right}px`,
    top: `${rect.bottom + 4}px`,
    minWidth: `${Math.max(rect.width, 120)}px`,
    zIndex: '9999',
  }
}

function toggleRefreshMenu() {
  showRefreshMenu.value = !showRefreshMenu.value
  if (showRefreshMenu.value) {
    nextTick(updateRefreshDropdownPosition)
  }
}

/** 选择刷新率后即时生效 */
function selectRefreshRate(hz: number) {
  displayStore.setRefreshRateHz(hz)
  deviceStore.setDisplayRefreshRateHz(hz)
  showRefreshMenu.value = false
}

function onRefreshClickOutside(e: MouseEvent) {
  if (!showRefreshMenu.value) return
  const target = e.target as Node
  if (
    refreshTriggerRef.value?.contains(target) ||
    refreshDropdownRef.value?.contains(target)
  ) {
    return
  }
  showRefreshMenu.value = false
}

onMounted(() => {
  document.addEventListener('mousedown', onRefreshClickOutside, true)
})

onBeforeUnmount(() => {
  document.removeEventListener('mousedown', onRefreshClickOutside, true)
})
</script>

<template>
  <header class="topbar">
    <div class="topbar__content">
      <div class="topbar__brand">
        <div class="topbar__logo">
          <Activity class="topbar__logo-icon" />
        </div>
        <div class="topbar__title-wrap">
          <h1 class="topbar__title" data-testid="topbar-title">
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
            :disabled="isAcquisitionDisabled"
            :title="isAcquisitionDisabled ? (isToggling ? '操作中...' : (isAcquiring ? '停止采集' : '没有可用的设备')) : (isAcquiring ? '停止采集' : '开始采集')"
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
          data-testid="btn-config"
          @click="emit('open-config')"
        >
          <Settings2 class="topbar__icon" />
        </button>

        <button
          class="topbar__icon-btn"
          ref="refreshTriggerRef"
          title="界面刷新率"
          @click="toggleRefreshMenu"
        >
          <Gauge class="topbar__icon" />
        </button>

        <Teleport to="body">
          <div
            v-if="showRefreshMenu"
            ref="refreshDropdownRef"
            class="topbar__refresh-dropdown"
            :style="refreshDropdownStyle"
          >
            <div class="topbar__refresh-header">界面刷新率</div>
            <div
              v-for="hz in refreshRateOptions"
              :key="hz"
              class="topbar__refresh-option"
              :class="{ 'topbar__refresh-option--active': displayStore.refreshRateHz === hz }"
              @click="selectRefreshRate(hz)"
            >
              {{ hz }} Hz
            </div>
          </div>
        </Teleport>

        <button
          class="topbar__icon-btn"
          @click="toggleTheme"
          :aria-label="themeToggleLabel()"
          :title="themeToggleLabel()"
          data-testid="btn-theme-toggle"
        >
          <Sun v-if="theme === 'dark'" class="topbar__icon" />
          <Moon v-else class="topbar__icon" />
        </button>

        <span class="topbar__version" data-testid="topbar-version">v{{ version }}</span>
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

/* 刷新率下拉菜单 */
.topbar__refresh-dropdown {
  background: var(--bg-panel);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-lg);
  padding: 0.25rem;
}

.topbar__refresh-header {
  padding: 0.35rem 0.75rem;
  font-size: 0.6rem;
  font-weight: 700;
  color: var(--text-muted);
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.topbar__refresh-option {
  padding: 0.4rem 0.75rem;
  font-size: var(--font-size-sm);
  font-weight: 600;
  color: var(--text-primary);
  border-radius: var(--radius-sm);
  cursor: pointer;
  transition: background var(--motion-fast) var(--easing-standard);
}

.topbar__refresh-option:hover {
  background: var(--btn-bg-hover);
}

.topbar__refresh-option--active {
  color: var(--accent);
  background: var(--accent-muted);
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
