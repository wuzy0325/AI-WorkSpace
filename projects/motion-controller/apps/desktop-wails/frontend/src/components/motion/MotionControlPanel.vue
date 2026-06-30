<script setup lang="ts">
import { onMounted, onBeforeUnmount, computed, reactive, ref, watch } from 'vue'
import { useMotionStore } from '@stores/motionStore'
import { useI18nStore } from '@stores/i18nStore'
import { useFeedbackStore } from '@stores/feedbackStore'
import type { AxisName } from '@shared/types/motion'
import { getAxisThemeClass } from './motionConfigEditor'
import MotionControllerConfig from './MotionControllerConfig.vue'

const motion = useMotionStore()
const i18n = useI18nStore()
const feedback = useFeedbackStore()
const selectedId = ref<string | null>(null)
const showConfig = ref(false)

type AxisHistoryMap = Map<AxisName, number[]>
const axisHistory = ref<Map<string, AxisHistoryMap>>(new Map())
const MAX_HISTORY = 50

interface AxisLocalState {
  targetPosition: number
  step: number
  expandedPanel: 'monitor' | 'jog' | 'move' | null
}

type AxisLocalStateMap = Record<AxisName, AxisLocalState>

const axisLocalState = reactive<Record<string, AxisLocalStateMap>>({})

function ensureAxisLocalState(controllerId: string, axisName: AxisName): AxisLocalState {
  if (!axisLocalState[controllerId]) {
    axisLocalState[controllerId] = {
      X: { targetPosition: 0, step: 1, expandedPanel: 'monitor' },
      Y: { targetPosition: 0, step: 1, expandedPanel: 'monitor' },
      Z: { targetPosition: 0, step: 1, expandedPanel: 'monitor' },
      U: { targetPosition: 0, step: 1, expandedPanel: 'monitor' }
    }
  }
  return axisLocalState[controllerId][axisName]
}

function togglePanel(controllerId: string, axisName: AxisName, panel: 'monitor' | 'jog' | 'move'): void {
  const state = ensureAxisLocalState(controllerId, axisName)
  state.expandedPanel = state.expandedPanel === panel ? null : panel
}

type AxisZeroOffsetMap = Record<AxisName, number>
const axisZeroOffset = reactive<Record<string, AxisZeroOffsetMap>>({})

function ensureZeroOffsetMap(controllerId: string): AxisZeroOffsetMap {
  if (!axisZeroOffset[controllerId]) {
    axisZeroOffset[controllerId] = {
      X: 0,
      Y: 0,
      Z: 0,
      U: 0
    }
  }
  return axisZeroOffset[controllerId]
}

function getZeroOffset(controllerId: string, axisName: AxisName): number {
  return ensureZeroOffsetMap(controllerId)[axisName] ?? 0
}

function setZeroOffset(controllerId: string, axisName: AxisName, value: number): void {
  ensureZeroOffsetMap(controllerId)[axisName] = value
}

function getAbsoluteTargetPosition(controllerId: string, axisName: AxisName): number {
  if (!controllerId) return 0
  return getZeroOffset(controllerId, axisName) + ensureAxisLocalState(controllerId, axisName).targetPosition
}

const currentStatus = computed(() =>
  selectedId.value ? motion.statusById(selectedId.value) : undefined
)

const currentProfile = computed(() =>
  selectedId.value ? motion.profiles.find((p) => p.id === selectedId.value) : undefined
)

const controllerConnected = computed(() => Boolean(currentStatus.value?.connected))

const axes = computed(() => currentStatus.value?.axes ?? [])

function getAxisLimits(axisName: AxisName): { min: number | undefined; max: number | undefined } {
  if (!currentProfile.value) return { min: undefined, max: undefined }
  const axisConfig = currentProfile.value.axes.find((a) => a.name === axisName)
  return {
    min: axisConfig?.minLimit,
    max: axisConfig?.maxLimit
  }
}

function validateTargetPosition(axisName: AxisName, targetPosition: number): { valid: boolean; warning?: string } {
  const limits = getAxisLimits(axisName)
  if (limits.min !== undefined && targetPosition < limits.min) {
    return { valid: false, warning: `目标位置 ${targetPosition.toFixed(2)} 超出负限位 ${limits.min.toFixed(2)}` }
  }
  if (limits.max !== undefined && targetPosition > limits.max) {
    return { valid: false, warning: `目标位置 ${targetPosition.toFixed(2)} 超出正限位 ${limits.max.toFixed(2)}` }
  }
  if (limits.min !== undefined && limits.max !== undefined) {
    const range = limits.max - limits.min
    const margin = range * 0.1
    if (targetPosition < limits.min + margin || targetPosition > limits.max - margin) {
      return { valid: true, warning: '接近限位，请谨慎操作' }
    }
  }
  return { valid: true }
}

function getLimitWarningClass(axisName: AxisName, targetPosition: number): string {
  const result = validateTargetPosition(axisName, targetPosition)
  if (!result.valid) return 'limit-exceeded'
  if (result.warning) return 'limit-near'
  return ''
}

function getAxisKind(axisName: AxisName): 'LINEAR' | 'ROTARY' {
  if (!currentProfile.value) return 'LINEAR'
  const axisConfig = currentProfile.value.axes.find((a) => a.name === axisName)
  return axisConfig?.kind ?? 'LINEAR'
}

function getAxisUnit(axisName: AxisName): string {
  return getAxisKind(axisName) === 'ROTARY' ? '°' : 'mm'
}

function selectController(id: string): void {
  selectedId.value = id
}

async function handleProfileSaved(id: string): Promise<void> {
  selectedId.value = id
  await motion.refreshStatus()
}

async function handleConnect(): Promise<void> {
  if (!selectedId.value) return
  try {
    await motion.connect(selectedId.value)
  } catch {
    // 错误已在 store 中处理
  }
}

async function handleDisconnect(): Promise<void> {
  if (!selectedId.value) return
  await motion.disconnect(selectedId.value)
}

async function move(axis: AxisName): Promise<void> {
  if (!selectedId.value || !controllerConnected.value) return
  const state = ensureAxisLocalState(selectedId.value, axis)
  const offset = getZeroOffset(selectedId.value, axis)
  const absoluteTarget = offset + state.targetPosition

  if (!Number.isFinite(absoluteTarget)) {
    feedback.pushToast('无效的目标位置', 'error')
    return
  }

  const validation = validateTargetPosition(axis, absoluteTarget)
  if (!validation.valid) {
    feedback.pushToast(validation.warning || '目标位置超出限位', 'error')
    return
  }
  if (validation.warning) {
    feedback.pushToast(validation.warning, 'warning')
  }

  await motion.moveTo(selectedId.value, axis, absoluteTarget)
}

async function jogAxis(axis: AxisName, direction: 'forward' | 'reverse'): Promise<void> {
  if (!selectedId.value || !controllerConnected.value) return
  const axisConfig = currentProfile.value?.axes.find((a) => a.name === axis)
  const maxSpeed = axisConfig?.maxSpeed ?? 10
  await motion.jog(selectedId.value, axis, direction, maxSpeed)
}

async function stop(axis?: AxisName): Promise<void> {
  if (!selectedId.value || !controllerConnected.value) return
  await motion.stop(selectedId.value, axis)
}

async function emergencyStop(): Promise<void> {
  if (!selectedId.value) return
  await motion.emergencyStop(selectedId.value)
}

async function handleResetEmergencyStop(): Promise<void> {
  if (!selectedId.value) return
  await motion.resetEmergencyStop(selectedId.value)
}

function clearCurrentError(): void {
  if (!selectedId.value) return
  motion.clearError(selectedId.value)
}

async function adjustByStep(axis: AxisName, direction: 'forward' | 'reverse'): Promise<void> {
  if (!selectedId.value || !controllerConnected.value || !currentStatus.value) return
  const axisStatus = currentStatus.value.axes.find((a) => a.name === axis)
  if (!axisStatus) return
  const state = ensureAxisLocalState(selectedId.value, axis)
  if (!Number.isFinite(state.step) || state.step <= 0) {
    feedback.pushToast('步长必须为正数', 'error')
    return
  }
  const delta = direction === 'forward' ? state.step : -state.step
  const validation = validateTargetPosition(axis, axisStatus.position + delta)
  if (!validation.valid) {
    feedback.pushToast(validation.warning || '目标位置超出限位', 'error')
    return
  }
  if ((delta > 0 && axisStatus.posLimit) || (delta < 0 && axisStatus.negLimit)) {
    feedback.pushToast('当前方向限位已触发，禁止继续点动', 'error')
    return
  }
  await motion.moveBy(selectedId.value, axis, delta)
}

async function setZero(axis: AxisName): Promise<void> {
  if (!selectedId.value || !controllerConnected.value) return
  await motion.definePosition(selectedId.value, axis, 0)
  setZeroOffset(selectedId.value, axis, 0)
  const state = ensureAxisLocalState(selectedId.value, axis)
  state.targetPosition = 0
}

function appendHistory(controllerId: string, axisName: AxisName, position: number): void {
  let ctrlMap = axisHistory.value.get(controllerId)
  if (!ctrlMap) {
    ctrlMap = new Map()
    axisHistory.value.set(controllerId, ctrlMap)
  }
  let arr = ctrlMap.get(axisName)
  if (!arr) {
    arr = []
    ctrlMap.set(axisName, arr)
  }
  const offset = getZeroOffset(controllerId, axisName)
  arr.push(position - offset)
  if (arr.length > MAX_HISTORY) {
    arr.splice(0, arr.length - MAX_HISTORY)
  }
}

function getAxisHistory(axisName: AxisName): number[] {
  if (!selectedId.value) return []
  const ctrlMap = axisHistory.value.get(selectedId.value)
  if (!ctrlMap) return []
  return ctrlMap.get(axisName) ?? []
}

function historyBarStyle(axisName: AxisName): Record<string, string> {
  const data = getAxisHistory(axisName)
  if (data.length === 0) return {}
  const min = data.reduce((a, b) => Math.min(a, b), Infinity)
  const max = data.reduce((a, b) => Math.max(a, b), -Infinity)
  if (!Number.isFinite(min) || !Number.isFinite(max) || min === max) {
    return {}
  }
  const segments: string[] = []
  const n = data.length
  for (let i = 0; i < n; i += 1) {
    const t = n === 1 ? 0 : (i / (n - 1)) * 100
    const ratio = (data[i] - min) / (max - min)
    const hue = 210 - ratio * 90
    segments.push(`hsl(${hue} 80% 60%) ${t.toFixed(1)}%`)
  }
  return {
    backgroundImage: `linear-gradient(to right, ${segments.join(', ')})`
  }
}

let unsubscribeStatus: (() => void) | null = null

function handleKeydown(e: KeyboardEvent): void {
  if (e.key === 'Escape') {
    e.preventDefault()
    emergencyStop()
    return
  }
}

onMounted(async () => {
  await motion.refreshProfiles()
  await motion.refreshStatus()
  if (motion.profiles.length > 0) {
    selectedId.value = motion.profiles[0].id
  }
  // 先注册状态监听器，再自动连接，确保自动连接的状态更新被捕获
  unsubscribeStatus = motion.attachStatusListener()
  await motion.autoConnectAll()
  window.addEventListener('keydown', handleKeydown)
})

onBeforeUnmount(() => {
  if (unsubscribeStatus) unsubscribeStatus()
  window.removeEventListener('keydown', handleKeydown)
})

watch(
  () => currentStatus.value,
  (status) => {
    if (!status || !selectedId.value) return
    for (const axis of status.axes) {
      appendHistory(selectedId.value, axis.name, axis.position)
    }
  },
  { deep: true }
)

watch(
  () => motion.profiles.map((p) => p.id),
  (ids) => {
    if (!selectedId.value || ids.includes(selectedId.value)) return
    selectedId.value = ids[0] ?? null
  }
)
</script>

<template>
  <div class="flex gap-4 motion-control-panel h-full min-h-0">
    <!-- 左侧边栏：控制器列表 -->
    <aside data-test="motion-panel-surface" class="motion-sidebar shrink-0 bg-[color:var(--bg-panel)] border border-[color:var(--border-default)] rounded-[var(--radius-lg)] flex flex-col shadow-[var(--shadow-panel)]">
      <!-- 边栏头部 -->
      <div class="sidebar-header">
        <div class="sidebar-title">
          {{ i18n.t.motionController }}
        </div>
        <button
          class="sidebar-config-btn"
          @click="showConfig = true"
        >
          <svg class="w-3 h-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z"/>
            <circle cx="12" cy="12" r="3"/>
          </svg>
          {{ i18n.t.config }}
        </button>
      </div>

      <!-- 空状态 -->
      <div v-if="motion.profiles.length === 0" class="sidebar-empty">
        <p class="sidebar-empty-title">{{ i18n.t.noControllerConfig }}</p>
        <p class="sidebar-empty-desc">{{ i18n.t.clickConfigToAdd }}</p>
      </div>

      <!-- 控制器列表 -->
      <div v-else class="sidebar-list custom-scrollbar">
        <button
          v-for="p in motion.profiles"
          :key="p.id"
          @click="selectController(p.id)"
          class="motion-list-item w-full text-left"
          :class="{ 'motion-list-item--active': selectedId === p.id }"
        >
          <div class="flex items-center justify-between gap-2">
            <div class="flex items-center gap-2 min-w-0">
              <span
                class="motion-status-dot"
                :class="motion.statusById(p.id)?.connected ? 'motion-status-dot--connected' : 'motion-status-dot--disconnected'"
              />
              <span class="truncate motion-item-name" :title="p.name">{{ p.name }}</span>
            </div>
            <span
              class="motion-status-text"
              :class="motion.statusById(p.id)?.connected ? 'motion-status-text--connected' : 'motion-status-text--disconnected'"
            >
              {{ motion.statusById(p.id)?.connected ? i18n.t.connected : i18n.t.disconnected }}
            </span>
          </div>
          <div class="flex items-center justify-between motion-item-meta">
            <span class="truncate">{{ p.address }}:{{ p.port }}</span>
            <span class="tracking-widest motion-item-type">{{ p.type }}</span>
          </div>
        </button>
      </div>
    </aside>

    <!-- 主内容区 -->
    <section data-test="motion-panel-surface" class="flex-1 bg-[color:var(--bg-panel)] border border-[color:var(--border-default)] rounded-[var(--radius-lg)] flex flex-col shadow-[var(--shadow-panel)] overflow-hidden min-h-0">
      <!-- 面板头部 -->
      <header class="panel-header">
        <div class="panel-header-info">
          <div class="panel-header-icon">
            <svg class="w-5 h-5 text-[color:var(--accent-primary)]" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <path d="M12 2v4"/>
              <path d="m16.2 7.8 2.9-2.9"/>
              <path d="M18 12h4"/>
              <path d="m16.2 16.2 2.9 2.9"/>
              <path d="M12 18v4"/>
              <path d="m4.9 19.1 2.9-2.9"/>
              <path d="M2 12h4"/>
              <path d="m4.9 4.9 2.9 2.9"/>
            </svg>
          </div>
          <div class="min-w-0">
            <div class="flex items-center gap-2">
              <h2
                class="panel-header-name truncate"
                :title="selectedId ? motion.profiles.find(p => p.id === selectedId)?.name || '' : ''"
              >
                {{ selectedId ? motion.profiles.find(p => p.id === selectedId)?.name || i18n.t.selectController : i18n.t.selectController }}
              </h2>
              <span
                v-if="selectedId"
                class="panel-header-badge"
              >
                {{ motion.profiles.find(p => p.id === selectedId)?.type }}
              </span>
            </div>
            <div class="panel-header-status">
              <span class="status-dot" :class="currentStatus?.connected ? 'status-dot--online' : 'status-dot--offline'"></span>
              <p class="status-text">
                {{ currentStatus?.connected ? i18n.t.systemOnline : i18n.t.systemOffline }}
                <span v-if="selectedId" class="status-address">· {{ motion.profiles.find(p => p.id === selectedId)?.address }}</span>
              </p>
            </div>
          </div>
        </div>

        <!-- 操作按钮组 -->
        <div class="panel-header-actions">
          <button
            class="btn-action btn-action--primary"
            @click="handleConnect"
            :disabled="!selectedId || currentStatus?.connected"
          >
            {{ i18n.t.connectBtn }}
          </button>
          <button
            class="btn-action btn-action--secondary"
            @click="handleDisconnect"
            :disabled="!selectedId || !currentStatus?.connected"
          >
            {{ i18n.t.disconnectBtn }}
          </button>
          <div class="action-divider"></div>
          <button
            class="btn-action btn-action--warning"
            @click="stop()"
            :disabled="!selectedId || !currentStatus?.connected"
          >
            {{ i18n.t.stopAll }}
          </button>
          <button
            class="btn-estop"
            @click="emergencyStop"
            :disabled="!selectedId"
            :title="i18n.t.eStopShortcut"
          >
            <span class="btn-estop__icon">
              <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
                <circle cx="12" cy="12" r="10"/>
                <line x1="15" y1="9" x2="9" y2="15"/>
                <line x1="9" y1="9" x2="15" y2="15"/>
              </svg>
            </span>
            {{ i18n.t.eStop }}
          </button>
        </div>
      </header>

      <!-- 错误提示 -->
      <div
        v-if="currentStatus?.lastError"
        class="alert-banner alert-banner--error"
      >
        <svg class="w-5 h-5 shrink-0 text-[color:var(--accent-danger)]" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <circle cx="12" cy="12" r="10"/>
          <line x1="12" y1="8" x2="12" y2="12"/>
          <line x1="12" y1="16" x2="12.01" y2="16"/>
        </svg>
        <div class="flex-1 min-w-0">
          <p class="alert-banner-title text-[color:var(--accent-danger)]">{{ i18n.t.controllerAlarm }}</p>
          <p class="alert-banner-msg">{{ currentStatus.lastError }}</p>
        </div>
        <button
          class="alert-close-btn"
          @click="clearCurrentError"
        >
          <svg class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <line x1="18" y1="6" x2="6" y2="18"/>
            <line x1="6" y1="6" x2="18" y2="18"/>
          </svg>
        </button>
      </div>

      <!-- 急停提示 -->
      <div
        v-if="currentStatus?.emergencyStopped"
        class="alert-banner alert-banner--warning"
      >
        <svg class="w-5 h-5 shrink-0 text-[color:var(--accent-warning)]" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <path d="M10.29 3.86 1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/>
          <line x1="12" y1="9" x2="12" y2="13"/>
          <line x1="12" y1="17" x2="12.01" y2="17"/>
        </svg>
        <div class="flex-1 min-w-0">
          <p class="alert-banner-title text-[color:var(--accent-warning)]">{{ i18n.t.eStopActive || '急停已激活' }}</p>
          <p class="alert-banner-msg">{{ i18n.t.eStopResetHint || '解除急停后方可继续操作' }}</p>
        </div>
        <button
          class="alert-action-btn"
          @click="handleResetEmergencyStop"
        >
          {{ i18n.t.eStopReset || '解除急停' }}
        </button>
      </div>

      <!-- 未选择控制器状态 -->
      <div v-if="!selectedId" class="empty-state">
        <div class="empty-state-icon">{{ i18n.t.selectController }}</div>
        <p class="empty-state-hint">{{ i18n.t.selectControllerHint }}</p>
      </div>

      <!-- 轴内容区 -->
      <div v-else class="flex flex-col min-h-0">
        <div class="axis-content custom-scrollbar">
          <!-- 无轴配置状态 -->
          <div v-if="axes.length === 0" class="empty-state">
            <svg class="w-16 h-16 mb-4 opacity-20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
              <path d="M12 2v4"/>
              <path d="m16.2 7.8 2.9-2.9"/>
              <path d="M18 12h4"/>
              <path d="m16.2 16.2 2.9 2.9"/>
              <path d="M12 18v4"/>
              <path d="m4.9 19.1 2.9-2.9"/>
              <path d="M2 12h4"/>
              <path d="m4.9 4.9 2.9 2.9"/>
            </svg>
            <p class="empty-state-title">{{ i18n.t.noAxesConfigured || '未配置运动轴' }}</p>
            <p class="empty-state-desc">{{ i18n.t.checkProfileAxes || '请在配置中启用至少一个轴' }}</p>
            <button
              class="empty-state-btn"
              @click="showConfig = true"
            >
              {{ i18n.t.openConfig || '打开配置' }}
            </button>
          </div>

          <!-- 轴卡片网格 -->
          <div v-else class="axis-grid">
            <div
              v-for="axis in axes"
              :key="axis.name"
              class="axis-card"
              :class="getAxisThemeClass(axis.name)"
            >
              <!-- 轴卡片头部 -->
              <div class="axis-card-header">
                <div class="axis-card-header-left">
                  <div class="axis-badge">
                    {{ axis.name }}
                  </div>
                  <div class="axis-header-info">
                    <span class="axis-header-label">{{ i18n.t.axisNode }}</span>
                    <span class="axis-header-value">{{ i18n.t.axisMotionControl }}</span>
                  </div>
                </div>
                <div class="axis-status-pill">
                  <span
                    class="axis-status-dot"
                    :class="axis.moving ? 'axis-status-dot--moving' : 'axis-status-dot--idle'"
                  />
                  <span class="axis-status-text">{{ axis.moving ? i18n.t.moving : i18n.t.idle }}</span>
                </div>
              </div>

              <!-- 位置读数（核心数据，独立区域） -->
              <div class="axis-readout">
                <div class="axis-readout-value">
                  <div class="axis-readout-number">
                    {{
                      selectedId
                        ? (axis.position - getZeroOffset(selectedId as string, axis.name as AxisName)).toFixed(2)
                        : axis.position.toFixed(2)
                    }}
                  </div>
                  <div class="axis-readout-unit">{{ getAxisUnit(axis.name as AxisName) }}</div>
                </div>
                <div class="axis-readout-label">
                  <span>{{ i18n.t.currentPosition }}</span>
                  <span
                    class="history-hint"
                    title="底部彩色条带显示最近 50 次位置采样历史"
                  >
                    <svg class="w-3 h-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><line x1="12" y1="16" x2="12.01" y2="16"/><path d="M12 12v-4"/></svg>
                  </span>
                </div>
                <div class="axis-readout-history">
                  <div class="axis-readout-history-bar" :style="historyBarStyle(axis.name as AxisName)"></div>
                </div>
              </div>

              <!-- 功能区域：监视 -->
              <div class="axis-section">
                <div class="axis-section-header">
                  <svg class="w-3 h-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M2 12s3-7 10-7 10 7 10 7-3 7-10 7-10-7-10-7Z"/><circle cx="12" cy="12" r="3"/></svg>
                  {{ i18n.t.monitor || '监视' }}
                </div>
                <div class="axis-section-body">
                  <div class="limit-status-row">
                    <div class="limit-status-item">
                      <span class="limit-indicator-sm" :class="axis.negLimit ? 'active' : ''"></span>
                      <span class="limit-status-label">{{ i18n.t.negLimit }}</span>
                    </div>
                    <div class="limit-status-item">
                      <span class="limit-status-label">{{ i18n.t.posLimit }}</span>
                      <span class="limit-indicator-sm" :class="axis.posLimit ? 'active' : ''"></span>
                    </div>
                  </div>
                  <div class="axis-action-row">
                    <button
                      class="btn-zero"
                      @click="setZero(axis.name as AxisName)"
                      :disabled="axis.moving || !controllerConnected"
                    >{{ i18n.t.setZero }}</button>
                    <button
                      class="btn-stop"
                      @click="stop(axis.name as AxisName)"
                      :disabled="!controllerConnected"
                    >{{ i18n.t.stop }}</button>
                  </div>
                </div>
              </div>

              <!-- 功能区域：点动 -->
              <div class="axis-section">
                <div class="axis-section-header">
                  <svg class="w-3 h-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m5 9 7-7 7 7"/><path d="m5 15 7 7 7-7"/></svg>
                  {{ i18n.t.jog }}
                </div>
                <div class="axis-section-body">
                  <div class="jog-control-row">
                    <button
                      class="btn-step-sm"
                      @click="adjustByStep(axis.name as AxisName, 'reverse')"
                      :disabled="axis.moving || !controllerConnected"
                    >
                      <svg class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><path d="m15 18-6-6 6-6"/></svg>
                    </button>
                    <div class="jog-input-wrap">
                      <input
                        v-model.number="ensureAxisLocalState(selectedId as string, axis.name as AxisName).step"
                        type="number"
                        class="input-field jog-input"
                        :disabled="axis.moving || !controllerConnected"
                      />
                      <span class="jog-unit">{{ getAxisUnit(axis.name as AxisName) }}</span>
                    </div>
                    <button
                      class="btn-step-sm"
                      @click="adjustByStep(axis.name as AxisName, 'forward')"
                      :disabled="axis.moving || !controllerConnected"
                    >
                      <svg class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><path d="m9 18 6-6-6-6"/></svg>
                    </button>
                  </div>
                </div>
              </div>

              <!-- 功能区域：定位 -->
              <div class="axis-section">
                <div class="axis-section-header">
                  <svg class="w-3 h-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 5v14"/><path d="m19 12-7 7-7-7"/></svg>
                  {{ i18n.t.move }}
                </div>
                <div class="axis-section-body">
                  <div class="move-control-row">
                    <div class="move-input-wrap">
                      <input
                        v-model.number="ensureAxisLocalState(selectedId as string, axis.name as AxisName).targetPosition"
                        type="number"
                        class="input-field move-input"
                        :class="getLimitWarningClass(axis.name as AxisName, getAbsoluteTargetPosition(selectedId as string, axis.name as AxisName))"
                        :disabled="axis.moving || !controllerConnected"
                        placeholder="0.00"
                      />
                      <span class="move-unit">{{ getAxisUnit(axis.name as AxisName) }}</span>
                    </div>
                    <button
                      class="btn-move"
                      @click="move(axis.name as AxisName)"
                      :disabled="axis.moving || !controllerConnected"
                    >{{ i18n.t.move }}</button>
                  </div>
                  <!-- 限位警告 -->
                  <div
                    v-if="!validateTargetPosition(axis.name as AxisName, getAbsoluteTargetPosition(selectedId as string, axis.name as AxisName)).valid"
                    class="limit-warning limit-warning--error"
                  >
                    <svg class="w-3 h-3 shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/></svg>
                    {{ validateTargetPosition(axis.name as AxisName, getAbsoluteTargetPosition(selectedId as string, axis.name as AxisName)).warning }}
                  </div>
                  <div
                    v-else-if="validateTargetPosition(axis.name as AxisName, getAbsoluteTargetPosition(selectedId as string, axis.name as AxisName)).warning"
                    class="limit-warning limit-warning--warn"
                  >
                    <svg class="w-3 h-3 shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M10.29 3.86 1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg>
                    {{ validateTargetPosition(axis.name as AxisName, getAbsoluteTargetPosition(selectedId as string, axis.name as AxisName)).warning }}
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </section>

    <MotionControllerConfig :open="showConfig" :current-id="selectedId" @saved="handleProfileSaved" @close="showConfig = false" />
  </div>
</template>

<style scoped>
/* ============================================================
   轴主题色变量
   ============================================================ */
.axis-card.axis-x-theme { --axis-hue: var(--axis-x); --axis-hue-soft: var(--axis-x-soft); }
.axis-card.axis-y-theme { --axis-hue: var(--axis-y); --axis-hue-soft: var(--axis-y-soft); }
.axis-card.axis-z-theme { --axis-hue: var(--axis-z); --axis-hue-soft: var(--axis-z-soft); }
.axis-card.axis-u-theme { --axis-hue: var(--axis-u); --axis-hue-soft: var(--axis-u-soft); }

/* ============================================================
   布局容器
   ============================================================ */

/* 主面板间距：使用统一的间距token */
.motion-control-panel {
  gap: var(--space-4);
}

/* 侧边栏 */
.motion-sidebar {
  width: var(--layout-sidebar-width);
  padding: var(--space-4);
  gap: var(--space-4);
}

/* 侧边栏头部 */
.sidebar-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2);
  padding-bottom: var(--space-3);
  border-bottom: 1px solid var(--border-default);
}

.sidebar-title {
  font-size: 0.6875rem;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.08em;
  color: var(--text-secondary);
}

.sidebar-config-btn {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  padding: var(--space-1) var(--space-2-5);
  font-size: 0.625rem;
  font-weight: 700;
  border-radius: var(--radius-md);
  border: 1px solid color-mix(in srgb, var(--accent-primary) 30%, transparent);
  color: var(--accent-primary);
  background: transparent;
  transition: all var(--motion-fast) var(--easing-standard);
  cursor: pointer;
}

.sidebar-config-btn:hover {
  color: white;
  background: var(--accent-primary);
  border-color: var(--accent-primary);
}

.sidebar-config-btn:active {
  transform: scale(0.95);
}

/* 侧边栏空状态 */
.sidebar-empty {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  padding: var(--space-6) var(--space-2);
  text-align: center;
}

.sidebar-empty-title {
  font-size: 0.875rem;
  font-weight: 700;
  color: var(--text-primary);
}

.sidebar-empty-desc {
  font-size: 0.75rem;
  color: var(--text-muted);
  line-height: 1.5;
}

/* 侧边栏列表 */
.sidebar-list {
  flex: 1;
  overflow: auto;
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

/* ============================================================
   主面板头部
   ============================================================ */
.panel-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-4);
  padding: var(--space-4) var(--space-5);
  border-bottom: 1px solid var(--border-default);
  background: var(--bg-panel);
  flex-wrap: wrap;
}

.panel-header-info {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  min-width: 0;
}

.panel-header-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 2.5rem;
  height: 2.5rem;
  border-radius: var(--radius-lg);
  background: linear-gradient(135deg, color-mix(in srgb, var(--accent-primary) 20%, transparent), color-mix(in srgb, var(--accent-primary) 5%, transparent));
  border: 1px solid color-mix(in srgb, var(--accent-primary) 20%, transparent);
  flex-shrink: 0;
}

.panel-header-name {
  font-size: 1rem;
  font-weight: 700;
  letter-spacing: -0.01em;
  color: var(--text-primary);
}

.panel-header-badge {
  padding: var(--space-0-5) var(--space-2);
  font-size: 0.5625rem;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  border-radius: var(--radius-pill);
  border: 1px solid var(--border-default);
  background: var(--bg-panel-strong);
  color: var(--text-secondary);
  flex-shrink: 0;
}

.panel-header-status {
  display: flex;
  align-items: center;
  gap: var(--space-1-5);
  margin-top: var(--space-1);
}

.status-dot {
  width: 0.5rem;
  height: 0.5rem;
  border-radius: 50%;
  flex-shrink: 0;
}

.status-dot--online {
  background: var(--accent-success);
  box-shadow: 0 0 8px var(--accent-success);
}

.status-dot--offline {
  background: var(--text-muted);
}

.status-text {
  font-size: 0.625rem;
  font-weight: 700;
  color: var(--text-muted);
  letter-spacing: 0.02em;
}

.status-address {
  margin-left: var(--space-2);
  opacity: 0.6;
}

/* 头部操作按钮组 */
.panel-header-actions {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  flex-wrap: wrap;
  justify-content: flex-end;
}

.btn-action {
  height: 2.25rem;
  padding: 0 var(--space-3);
  border-radius: var(--radius-md);
  font-size: 0.75rem;
  font-weight: 700;
  white-space: nowrap;
  transition: all var(--motion-fast) var(--easing-standard);
  cursor: pointer;
  border: none;
}

.btn-action:active:not(:disabled) {
  transform: scale(0.95);
}

.btn-action:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.btn-action--primary {
  background: var(--accent-success);
  color: white;
  box-shadow: 0 4px 12px color-mix(in srgb, var(--accent-success) 30%, transparent);
}

.btn-action--primary:hover:not(:disabled) {
  opacity: 0.9;
}

.btn-action--secondary {
  background: transparent;
  color: var(--text-muted);
  border: 1px solid var(--border-default);
}

.btn-action--secondary:hover:not(:disabled) {
  color: var(--text-primary);
  background: var(--bg-panel-strong);
}

.btn-action--warning {
  background: var(--accent-warning);
  color: white;
  box-shadow: 0 4px 12px color-mix(in srgb, var(--accent-warning) 30%, transparent);
}

.btn-action--warning:hover:not(:disabled) {
  opacity: 0.9;
}

.action-divider {
  width: 1px;
  height: 1.5rem;
  background: var(--border-default);
  margin: 0 var(--space-1);
}

/* ============================================================
   警告横幅
   ============================================================ */
.alert-banner {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  margin: var(--space-4) var(--space-5) 0;
  padding: var(--space-3) var(--space-4);
  border-radius: var(--radius-md);
  border: 1px solid transparent;
}

.alert-banner--error {
  background: color-mix(in srgb, var(--accent-danger) 10%, transparent);
  border-color: color-mix(in srgb, var(--accent-danger) 30%, transparent);
}

.alert-banner--warning {
  background: color-mix(in srgb, var(--accent-warning) 10%, transparent);
  border-color: color-mix(in srgb, var(--accent-warning) 30%, transparent);
}

.alert-banner-title {
  font-size: 0.625rem;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.06em;
}

.alert-banner-msg {
  font-size: 0.8125rem;
  font-weight: 600;
  color: var(--text-primary);
  margin-top: var(--space-0-5);
}

.alert-close-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 1.5rem;
  height: 1.5rem;
  border-radius: var(--radius-sm);
  color: color-mix(in srgb, var(--accent-danger) 60%, transparent);
  background: transparent;
  border: none;
  cursor: pointer;
  transition: all var(--motion-fast) var(--easing-standard);
  flex-shrink: 0;
}

.alert-close-btn:hover {
  color: var(--accent-danger);
  background: color-mix(in srgb, var(--accent-danger) 10%, transparent);
}

.alert-action-btn {
  padding: var(--space-1) var(--space-3);
  border-radius: var(--radius-md);
  font-size: 0.625rem;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  border: 1px solid color-mix(in srgb, var(--accent-warning) 30%, transparent);
  color: var(--accent-warning);
  background: transparent;
  cursor: pointer;
  transition: all var(--motion-fast) var(--easing-standard);
  flex-shrink: 0;
}

.alert-action-btn:hover {
  background: var(--accent-warning);
  color: white;
}

/* ============================================================
   空状态
   ============================================================ */
.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  flex: 1;
  padding: var(--space-12);
  color: var(--text-muted);
  text-align: center;
}

.empty-state-icon {
  font-size: 3.5rem;
  font-weight: 700;
  font-style: italic;
  opacity: 0.15;
  margin-bottom: var(--space-4);
}

.empty-state-hint {
  font-size: 0.75rem;
  font-weight: 700;
  letter-spacing: 0.15em;
  text-transform: uppercase;
}

.empty-state-title {
  font-size: 0.875rem;
  font-weight: 600;
  margin-bottom: var(--space-1);
}

.empty-state-desc {
  font-size: 0.75rem;
  opacity: 0.6;
  margin-bottom: var(--space-4);
}

.empty-state-btn {
  padding: var(--space-2) var(--space-4);
  border-radius: var(--radius-md);
  font-size: 0.75rem;
  font-weight: 700;
  border: 1px solid var(--border-default);
  color: var(--text-muted);
  background: transparent;
  cursor: pointer;
  transition: all var(--motion-fast) var(--easing-standard);
}

.empty-state-btn:hover {
  color: var(--text-primary);
  background: var(--bg-panel-strong);
}

/* ============================================================
   轴内容滚动区
   ============================================================ */
.axis-content {
  flex: 1;
  min-height: 0;
  overflow: auto;
  padding: var(--space-3) var(--space-5);
  /* 让轴网格作为 flex 子项填充整个滚动区高度，避免内容不足时下方出现大片空白 */
  display: flex;
  flex-direction: column;
}

/* ============================================================
   轴卡片网格
   ============================================================ */
.axis-grid {
  display: grid;
  grid-template-columns: repeat(1, 1fr);
  gap: var(--space-4);
  /* 占满 axis-content 的可用高度，使卡片能随窗口高度拉伸 */
  flex: 1;
  min-height: 0;
}

@media (min-width: 768px) {
  .axis-grid {
    grid-template-columns: repeat(2, 1fr);
  }
}

@media (min-width: 1280px) {
  .axis-grid {
    grid-template-columns: repeat(4, 1fr);
  }
}

/* ============================================================
   轴卡片
   ============================================================ */
.axis-card {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  padding: var(--space-4);
  background: var(--bg-panel);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-lg);
  min-width: 200px;
  /* 占满网格单元格高度；内部通过 flex column 保持内容顶部对齐 */
  height: 100%;
  transition: border-color var(--motion-base) var(--easing-standard),
              box-shadow var(--motion-base) var(--easing-standard);
}

.axis-card:hover {
  border-color: var(--axis-hue);
  box-shadow: 0 10px 15px -3px color-mix(in srgb, var(--axis-hue) 15%, transparent),
              0 4px 6px -4px color-mix(in srgb, var(--axis-hue) 10%, transparent);
}

/* 轴卡片头部 */
.axis-card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2);
}

.axis-card-header-left {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  min-width: 0;
}

.axis-badge {
  width: 2.25rem;
  height: 2.25rem;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: var(--radius-md);
  font-size: 1.125rem;
  font-weight: 900;
  background: var(--axis-hue-soft);
  color: var(--axis-hue);
  flex-shrink: 0;
}

.axis-header-info {
  display: flex;
  flex-direction: column;
  gap: var(--space-0-5);
  min-width: 0;
}

.axis-header-label {
  font-size: 0.625rem;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: var(--text-muted);
}

.axis-header-value {
  font-size: 0.8125rem;
  font-weight: 600;
  color: var(--text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* 轴状态胶囊 */
.axis-status-pill {
  display: flex;
  align-items: center;
  gap: var(--space-1-5);
  padding: var(--space-1) var(--space-2-5);
  border-radius: var(--radius-pill);
  background: var(--bg-canvas);
  flex-shrink: 0;
}

.axis-status-dot {
  width: 0.5rem;
  height: 0.5rem;
  border-radius: 50%;
}

.axis-status-dot--moving {
  background: var(--accent-success);
  box-shadow: 0 0 8px var(--accent-success);
  animation: pulse-dot 2s ease-in-out infinite;
}

.axis-status-dot--idle {
  background: var(--text-muted);
}

.axis-status-text {
  font-size: 0.625rem;
  font-weight: 700;
  color: var(--text-muted);
}

@keyframes pulse-dot {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.5; }
}

/* ============================================================
   位置读数区（核心视觉焦点）
   ============================================================ */
.axis-readout {
  position: relative;
  padding: var(--space-4);
  background: var(--bg-canvas);
  border-radius: var(--radius-md);
  overflow: hidden;
}

.axis-readout-value {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: var(--space-2);
}

.axis-readout-number {
  font-size: 1.75rem;
  font-family: var(--font-family-mono, monospace);
  font-variant-numeric: tabular-nums;
  font-weight: 700;
  color: var(--text-primary);
  letter-spacing: -0.02em;
  line-height: 1.2;
  truncate: true;
}

.axis-readout-unit {
  font-size: 0.625rem;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.08em;
  color: var(--text-muted);
  flex-shrink: 0;
}

.axis-readout-label {
  display: flex;
  align-items: center;
  gap: var(--space-1-5);
  margin-top: var(--space-2);
}

.axis-readout-label span:first-child {
  font-size: 0.625rem;
  color: var(--text-muted);
}

/* 历史条带 */
.axis-readout-history {
  position: absolute;
  bottom: 0;
  left: 0;
  right: 0;
  height: 3px;
  opacity: 0.7;
}

.axis-readout-history-bar {
  height: 100%;
  width: 100%;
}

/* ============================================================
   轴功能区域（监视/点动/定位）
   ============================================================ */
.axis-section {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.axis-section + .axis-section {
  padding-top: var(--space-3);
  border-top: 1px solid var(--border-default);
}

.axis-section-header {
  display: flex;
  align-items: center;
  gap: var(--space-1-5);
  font-size: 0.625rem;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: var(--text-muted);
}

.axis-section-header svg {
  color: var(--axis-hue, var(--accent-primary));
}

.axis-section-body {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

/* 操作按钮行 */
.axis-action-row {
  display: flex;
  gap: var(--space-2);
}

.axis-action-row .btn-zero,
.axis-action-row .btn-stop {
  flex: 1;
  min-width: 0;
  height: 1.75rem;
  padding: 0 var(--space-1);
  border-radius: var(--radius-md);
  font-size: 0.6875rem;
  font-weight: 700;
  letter-spacing: 0.04em;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  cursor: pointer;
  transition: all var(--motion-fast) var(--easing-standard);
}

/* ============================================================
   按钮样式
   ============================================================ */

/* 急停按钮 */
.btn-estop {
  height: 2.25rem;
  padding: 0 var(--space-4);
  border-radius: var(--radius-md);
  font-size: 0.75rem;
  font-weight: 700;
  letter-spacing: 0.04em;
  white-space: nowrap;
  background: var(--accent-danger);
  color: white;
  border: 2px solid var(--accent-danger);
  display: inline-flex;
  align-items: center;
  gap: var(--space-1-5);
  cursor: pointer;
  animation: estop-pulse 2s ease-in-out infinite;
  transition: all var(--motion-fast) var(--easing-standard);
}

.btn-estop:hover:not(:disabled) {
  background: #dc2626;
  border-color: #dc2626;
  box-shadow: 0 0 20px 4px color-mix(in srgb, var(--accent-danger) 40%, transparent);
  animation: none;
}

.btn-estop:active:not(:disabled) {
  transform: scale(0.92);
  background: #b91c1c;
}

.btn-estop:disabled {
  background: var(--bg-panel-strong);
  border-color: var(--border-default);
  color: var(--text-muted);
  animation: none;
  cursor: not-allowed;
  opacity: 0.4;
}

.btn-estop__icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
}

@keyframes estop-pulse {
  0%, 100% {
    box-shadow: 0 0 0 0 color-mix(in srgb, var(--accent-danger) 50%, transparent);
  }
  50% {
    box-shadow: 0 0 0 6px color-mix(in srgb, var(--accent-danger) 0%, transparent);
  }
}

/* 移动按钮 */
.btn-move {
  height: 2rem;
  padding: 0 var(--space-3);
  border-radius: var(--radius-md);
  font-size: 0.6875rem;
  font-weight: 700;
  letter-spacing: 0.04em;
  white-space: nowrap;
  background: var(--axis-hue);
  color: white;
  border: 1px solid var(--axis-hue);
  box-shadow: 0 2px 6px color-mix(in srgb, var(--axis-hue) 35%, transparent);
  cursor: pointer;
  flex-shrink: 0;
  transition: all var(--motion-fast) var(--easing-standard);
}

.btn-move:hover:not(:disabled) {
  opacity: 0.9;
  box-shadow: 0 4px 12px color-mix(in srgb, var(--axis-hue) 45%, transparent);
}

.btn-move:active:not(:disabled) {
  transform: scale(0.95);
  box-shadow: 0 1px 3px color-mix(in srgb, var(--axis-hue) 25%, transparent);
}

.btn-move:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

/* 归零按钮 */
.btn-zero {
  background: var(--bg-panel-strong);
  border: 1px solid var(--border-strong);
  color: var(--text-secondary);
  box-shadow: 0 1px 2px color-mix(in srgb, #000 6%, transparent);
}

.btn-zero:hover:not(:disabled) {
  background: var(--accent-primary);
  border-color: var(--accent-primary);
  color: white;
  box-shadow: 0 2px 8px color-mix(in srgb, var(--accent-primary) 30%, transparent);
}

.btn-zero:active:not(:disabled) {
  transform: scale(0.95);
}

.btn-zero:disabled {
  opacity: 0.35;
  cursor: not-allowed;
  box-shadow: none;
}

/* 停止按钮 */
.btn-stop {
  background: var(--bg-panel-strong);
  border: 1px solid var(--border-strong);
  color: var(--text-secondary);
  box-shadow: 0 1px 2px color-mix(in srgb, #000 6%, transparent);
}

.btn-stop:hover:not(:disabled) {
  background: var(--accent-danger);
  border-color: var(--accent-danger);
  color: white;
  box-shadow: 0 2px 8px color-mix(in srgb, var(--accent-danger) 30%, transparent);
}

.btn-stop:active:not(:disabled) {
  transform: scale(0.95);
}

.btn-stop:disabled {
  opacity: 0.35;
  cursor: not-allowed;
  box-shadow: none;
}

/* 步进按钮 */
.btn-step-sm {
  width: 2rem;
  height: 2rem;
  flex-shrink: 0;
  border-radius: var(--radius-md);
  border: 1px solid var(--border-strong);
  background: var(--bg-panel-strong);
  color: var(--text-primary);
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  box-shadow: 0 1px 2px color-mix(in srgb, #000 8%, transparent);
  transition: all var(--motion-fast) var(--easing-standard);
}

.btn-step-sm:hover:not(:disabled) {
  background: var(--axis-hue);
  border-color: var(--axis-hue);
  color: white;
  box-shadow: 0 2px 6px color-mix(in srgb, var(--axis-hue) 25%, transparent);
}

.btn-step-sm:active:not(:disabled) {
  transform: scale(0.92);
}

.btn-step-sm:disabled {
  opacity: 0.35;
  cursor: not-allowed;
  box-shadow: none;
}

/* ============================================================
   输入框
   ============================================================ */
.input-field {
  background: var(--bg-panel);
  border: 1px solid transparent;
  border-radius: var(--radius-md);
  color: var(--text-primary);
  transition: border-color var(--motion-fast) var(--easing-standard),
              box-shadow var(--motion-fast) var(--easing-standard);
}

.input-field:focus {
  outline: none;
  border-color: var(--accent-primary);
  box-shadow: 0 0 0 3px var(--accent-primary-muted);
}

.input-field:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.input-field.limit-exceeded,
.move-input.limit-exceeded {
  border-color: var(--accent-danger);
  background: color-mix(in srgb, var(--accent-danger) 8%, var(--bg-panel));
  box-shadow: 0 0 0 2px color-mix(in srgb, var(--accent-danger) 25%, transparent);
}

.input-field.limit-near,
.move-input.limit-near {
  border-color: var(--accent-warning);
  background: color-mix(in srgb, var(--accent-warning) 8%, var(--bg-panel));
}

/* 点动输入 */
.jog-input-wrap {
  flex: 1;
  min-width: 0;
  display: flex;
  align-items: center;
  gap: var(--space-1-5);
}

.jog-input {
  width: 100%;
  height: 2rem;
  padding: 0 var(--space-2);
  text-align: left;
  font-size: 0.875rem;
  font-weight: 600;
}

.jog-unit {
  font-size: 0.625rem;
  font-weight: 700;
  text-transform: uppercase;
  color: var(--text-muted);
  pointer-events: none;
  white-space: nowrap;
  flex-shrink: 0;
}

/* 定位输入 */
.move-input-wrap {
  flex: 1;
  min-width: 0;
  display: flex;
  align-items: center;
  gap: var(--space-1-5);
}

.move-input {
  width: 100%;
  height: 2rem;
  padding: 0 var(--space-2);
  text-align: left;
  font-size: 0.875rem;
  font-weight: 600;
}

.move-unit {
  font-size: 0.625rem;
  font-weight: 700;
  text-transform: uppercase;
  color: var(--text-muted);
  pointer-events: none;
  white-space: nowrap;
  flex-shrink: 0;
}

/* ============================================================
   限位状态
   ============================================================ */
.limit-status-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--space-1) var(--space-1-5);
  background: var(--bg-panel);
  border-radius: calc(var(--radius-md) - 2px);
}

.limit-status-item {
  display: flex;
  align-items: center;
  gap: var(--space-1);
}

.limit-status-label {
  font-size: 0.625rem;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  color: var(--text-muted);
}

.limit-indicator-sm {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  border: 1px solid var(--border-default);
  background: transparent;
  transition: all var(--motion-base) var(--easing-standard);
}

.limit-indicator-sm.active {
  background: var(--accent-danger);
  box-shadow: 0 0 4px var(--accent-danger);
  border-color: transparent;
}

/* ============================================================
   限位警告
   ============================================================ */
.limit-warning {
  display: flex;
  align-items: center;
  gap: var(--space-1);
  font-size: 0.625rem;
  font-weight: 600;
}

.limit-warning--error {
  color: var(--accent-danger);
}

.limit-warning--warn {
  color: var(--accent-warning);
}

/* ============================================================
   控制器列表项
   ============================================================ */
.motion-list-item {
  padding: var(--space-3);
  border-radius: var(--radius-md);
  background: var(--bg-panel-strong);
  border: 1px solid transparent;
  transition: all var(--motion-fast) var(--easing-standard);
  cursor: pointer;
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
}

.motion-list-item:hover {
  background: var(--bg-canvas);
  border-color: var(--border-default);
}

.motion-list-item--active {
  background: color-mix(in srgb, var(--accent-success) 8%, transparent) !important;
  border-color: color-mix(in srgb, var(--accent-success) 30%, transparent) !important;
}

.motion-item-name {
  font-size: 0.875rem;
  font-weight: 700;
  color: var(--text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.motion-item-meta {
  font-size: 0.625rem;
  color: var(--text-muted);
}

.motion-item-type {
  font-size: 0.5625rem;
  color: var(--text-muted);
}

.motion-status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
  background: var(--text-muted);
}

.motion-status-dot--connected {
  background: var(--accent-success);
  box-shadow: 0 0 8px var(--accent-success);
}

.motion-status-dot--disconnected {
  background: var(--text-muted);
}

.motion-status-text {
  display: flex;
  align-items: center;
  font-size: 0.5625rem;
  font-weight: 700;
  letter-spacing: 0.04em;
  color: var(--text-muted);
}

.motion-status-text--connected {
  color: var(--accent-success);
}

/* ============================================================
   历史提示
   ============================================================ */
.history-hint {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: var(--text-muted);
  cursor: help;
  transition: color var(--motion-fast) var(--easing-standard);
}

.history-hint:hover {
  color: var(--accent-primary);
}

/* ============================================================
   控制行布局
   ============================================================ */
.jog-control-row {
  display: flex;
  align-items: center;
  gap: var(--space-1-5);
}

.move-control-row {
  display: flex;
  align-items: center;
  gap: var(--space-1-5);
}
</style>
