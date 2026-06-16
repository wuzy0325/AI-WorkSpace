<script setup lang="ts">
import { Play, Square, Circle, Activity } from '@lucide/vue'
import UiButton from '@components/ui/UiButton.vue'

type MainShellPage = 'dashboard' | 'motion' | 'calibration' | 'traversal' | 'log'
type MainViewMode = 'chart' | 'table' | 'both' | 'overview'

const props = withDefaults(
  defineProps<{
    version: string
    isAcquiring: boolean
    isRecording?: boolean
    activePage: MainShellPage
    viewMode: MainViewMode
    t: Record<string, string>
  }>(),
  {
    isRecording: false,
  }
)

const emit = defineEmits<{
  (e: 'set-view-mode', mode: MainViewMode): void
  (e: 'start'): void
  (e: 'stop'): void
  (e: 'toggle-recording'): void
}>()

const dashboardModes: MainViewMode[] = ['chart', 'table', 'both', 'overview']

function modeLabel(mode: MainViewMode): string {
  if (mode === 'chart') return props.t.chartMode || '图表'
  if (mode === 'table') return props.t.tableMode || '表格'
  if (mode === 'both') return props.t.bothMode || '混合'
  return props.t.overviewMode || '总览'
}

function activePageLabel(): string {
  if (props.activePage === 'motion') return props.t.motionControl || '运动控制'
  if (props.activePage === 'calibration') return props.t.probeCalibration || '探针校准'
  if (props.activePage === 'traversal') return props.t.traversalTest || '遍历测试'
  return ''
}
</script>

<template>
  <header class="main-topbar">
    <div data-test="topbar-main-row" class="main-topbar__content">
      <!-- Logo -->
      <div class="main-topbar__brand">
        <div class="main-topbar__logo">
          <Activity class="w-5 h-5 text-white" />
        </div>
        <div>
          <h1 data-test="topbar-brand-title" class="main-topbar__title">
            Wind<span class="main-topbar__title-accent">DAQ</span>
          </h1>
          <p data-test="topbar-brand-subtitle" class="main-topbar__subtitle">DATA ACQUISITION</p>
        </div>
      </div>

      <!-- Navigation -->
      <nav v-if="activePage === 'dashboard'" class="main-topbar__nav">
        <button
          v-for="mode in dashboardModes"
          :key="mode"
          class="main-topbar__nav-btn"
          :class="{ 'main-topbar__nav-btn--active': viewMode === mode }"
          @click="emit('set-view-mode', mode)"
        >
          {{ modeLabel(mode) }}
        </button>
      </nav>
      <div v-else class="main-topbar__nav">
        <div class="main-topbar__nav-btn main-topbar__nav-btn--active">
          {{ activePageLabel() }}
        </div>
      </div>

      <!-- Right Section: 核心操作按钮 -->
      <div data-test="topbar-utility-group" class="main-topbar__actions">
        <!-- 开始/停止采集按钮 -->
        <UiButton
          data-test="acquisition-toggle-btn"
          class="main-topbar__btn"
          :class="isAcquiring ? 'btn-stop' : 'btn-start'"
          @click="isAcquiring ? emit('stop') : emit('start')"
          :title="isAcquiring ? (t.stopAcquisition || '停止采集') : (t.startAcquisition || '开始采集')"
        >
          <template #icon>
            <Play v-if="!isAcquiring" class="w-4 h-4 fill-current" />
            <Square v-else class="w-4 h-4 fill-current" />
          </template>
          {{ isAcquiring ? (t.stopAcquisition || '停止采集') : (t.startAcquisition || '开始采集') }}
        </UiButton>

        <!-- 开始/停止记录按钮 -->
        <UiButton
          data-test="recording-toggle-btn"
          class="main-topbar__btn main-topbar__btn--with-text btn-record"
          :class="{ active: isRecording }"
          @click="emit('toggle-recording')"
          :title="isRecording ? (t.stopRecording || '停止记录') : (t.startRecording || '开始记录')"
        >
          <template #icon>
            <Circle class="w-4 h-4 fill-current" />
          </template>
          {{ isRecording ? (t.stopRecording || '停止记录') : (t.startRecording || '开始记录') }}
        </UiButton>

        <!-- Version -->
        <span data-test="topbar-version-text" class="main-topbar__version">v{{ version }}</span>
      </div>
    </div>
  </header>
</template>

<style scoped>
.main-topbar {
  height: 56px;
  flex-shrink: 0;
  background: var(--bg-panel-strong);
  border-bottom: 1px solid var(--border-default);
  position: relative;
  z-index: 50;
}

:root[data-theme='light'] .main-topbar {
  background: var(--bg-panel-strong);
  border-bottom: 1px solid var(--border-default);
}

.main-topbar__content {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 100%;
  padding: 0 1.5rem;
  gap: 1rem;
}

.main-topbar__brand {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  flex-shrink: 0;
}

.main-topbar__logo {
  width: 32px;
  height: 32px;
  background: var(--accent-primary);
  border-radius: 0.5rem;
  display: flex;
  align-items: center;
  justify-content: center;
  box-shadow: 0 4px 12px color-mix(in srgb, var(--accent-primary) 30%, transparent);
}

.main-topbar__title {
  font-size: 1.125rem;
  font-weight: 800;
  letter-spacing: -0.025em;
  color: var(--text-primary);
  line-height: 1;
}

:root[data-theme='light'] .main-topbar__title {
  color: var(--text-primary);
}

.main-topbar__title-accent {
  color: var(--accent-primary);
}

.main-topbar__subtitle {
  font-size: var(--font-size-2xs);
  font-weight: 600;
  color: var(--text-muted);
  letter-spacing: 0.1em;
  text-transform: uppercase;
  margin-top: 0.125rem;
}

/* 副标题在小屏幕上自然隐藏，不使用生硬断点 */
.main-topbar__subtitle {
  display: block;
}

@media (max-width: 1280px) {
  .main-topbar__subtitle {
    display: none;
  }
}

/* 视图模式导航：使用更紧凑的 pill 设计 */
.main-topbar__nav {
  display: flex;
  align-items: center;
  gap: 0.125rem;
  padding: 0.25rem;
  background: color-mix(in srgb, var(--bg-canvas) 80%, transparent);
  border-radius: 0.625rem;
  flex-shrink: 0;
  border: 1px solid var(--border-default);
}

:root[data-theme='light'] .main-topbar__nav {
  background: color-mix(in srgb, var(--bg-canvas) 80%, transparent);
  border: 1px solid var(--border-default);
}

.main-topbar__nav-btn {
  font-size: var(--font-size-xs);
  font-weight: 500;
  white-space: nowrap;
  padding: 0.375rem 0.875rem;
  border-radius: 0.5rem;
  border: none;
  background: transparent;
  color: var(--text-muted);
  cursor: pointer;
  transition: all 0.2s ease;
  letter-spacing: 0.02em;
}

:root[data-theme='light'] .main-topbar__nav-btn {
  color: var(--text-muted);
}

.main-topbar__nav-btn:hover {
  color: var(--text-primary);
  background: color-mix(in srgb, var(--text-primary) 5%, transparent);
}

:root[data-theme='light'] .main-topbar__nav-btn:hover {
  color: var(--text-primary);
  background: color-mix(in srgb, var(--text-primary) 5%, transparent);
}

.main-topbar__nav-btn--active {
  background: var(--bg-panel-strong) !important;
  color: var(--text-primary) !important;
  font-weight: 600;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.1);
}

:root[data-theme='light'] .main-topbar__nav-btn--active {
  background: var(--bg-panel-strong) !important;
  color: var(--text-primary) !important;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.1);
}

.main-topbar__nav-btn--active:hover {
  background: var(--bg-panel-strong) !important;
}

:root[data-theme='light'] .main-topbar__nav-btn--active:hover {
  background: var(--bg-panel-strong) !important;
}

.main-topbar__actions {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  flex-shrink: 0;
}

/* 顶部栏操作按钮样式 */
:deep(.main-topbar__btn) {
  height: 36px;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0.375rem;
  padding: 0 0.75rem;
  font-size: 0.75rem;
  font-weight: 600;
}

:deep(.main-topbar__btn):hover:not(:disabled) {
  transform: translateY(-1px);
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
  color: var(--accent-danger);
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
  background: color-mix(in srgb, var(--accent-danger) 10%, transparent);
  color: var(--accent-danger);
  border-color: color-mix(in srgb, var(--accent-danger) 30%, transparent);
  /* 使用更 subtle 的呼吸动画，降低视觉干扰 */
  animation: pulse-record 3s ease-in-out infinite;
}

@keyframes pulse-record {
  0%, 100% {
    box-shadow: 0 0 0 0 color-mix(in srgb, var(--accent-danger) 30%, transparent);
  }
  50% {
    box-shadow: 0 0 0 4px color-mix(in srgb, var(--accent-danger) 0%, transparent);
  }
}

.main-topbar__version {
  font-size: var(--font-size-micro);
  font-weight: 500;
  color: var(--text-muted);
  font-family: ui-monospace, monospace;
  display: block;
  opacity: 0.6;
  margin-left: 0.5rem;
}

/* 在较小屏幕上隐藏视图模式切换，用户仍可通过其他方式切换 */
@media (max-width: 1024px) {
  .main-topbar__nav {
    display: none;
  }
}
</style>
