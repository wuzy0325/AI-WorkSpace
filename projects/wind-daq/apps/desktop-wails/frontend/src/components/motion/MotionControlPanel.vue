<script setup lang="ts">
import { onMounted, onBeforeUnmount, computed, reactive, ref, watch, defineAsyncComponent } from 'vue'
import { useMotionStore } from '@stores/motionStore'
import { useI18nStore } from '@stores/i18nStore'
import { useFeedbackStore } from '@stores/feedbackStore'
import type { AxisName } from '@shared/types/motion'
import UiInputNumber from '@components/ui/UiInputNumber.vue'
import UiButton from '@components/ui/UiButton.vue'

const MotionControllerConfig = defineAsyncComponent(() => import('./MotionControllerConfig.vue'))

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
}

type AxisLocalStateMap = Record<AxisName, AxisLocalState>

const axisLocalState = reactive<Record<string, AxisLocalStateMap>>({})

function ensureAxisLocalState(controllerId: string, axisName: AxisName): AxisLocalState {
  if (!axisLocalState[controllerId]) {
    axisLocalState[controllerId] = {
      X: { targetPosition: 0, step: 1 },
      Y: { targetPosition: 0, step: 1 },
      Z: { targetPosition: 0, step: 1 },
      U: { targetPosition: 0, step: 1 }
    }
  }
  return axisLocalState[controllerId][axisName]
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

/**
 * 获取轴的软限位配置
 */
function getAxisLimits(axisName: AxisName): { min: number | undefined; max: number | undefined } {
  if (!currentProfile.value) return { min: undefined, max: undefined }
  const axisConfig = currentProfile.value.axes.find((a) => a.name === axisName)
  return {
    min: axisConfig?.minLimit,
    max: axisConfig?.maxLimit
  }
}

/**
 * 校验目标位置是否在软限位范围内
 */
function validateTargetPosition(axisName: AxisName, targetPosition: number): { valid: boolean; warning?: string } {
  const limits = getAxisLimits(axisName)
  if (limits.min !== undefined && targetPosition < limits.min) {
    return { valid: false, warning: `目标位置 ${targetPosition.toFixed(2)} 超出负限位 ${limits.min.toFixed(2)}` }
  }
  if (limits.max !== undefined && targetPosition > limits.max) {
    return { valid: false, warning: `目标位置 ${targetPosition.toFixed(2)} 超出正限位 ${limits.max.toFixed(2)}` }
  }
  // 接近限位预警 (90% 范围内)
  if (limits.min !== undefined && limits.max !== undefined) {
    const range = limits.max - limits.min
    const margin = range * 0.1
    if (targetPosition < limits.min + margin || targetPosition > limits.max - margin) {
      return { valid: true, warning: '接近限位，请谨慎操作' }
    }
  }
  return { valid: true }
}

/**
 * 获取轴的限位状态样式
 */
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

async function handleConnect(): Promise<void> {
  if (!selectedId.value) return
  await motion.connect(selectedId.value)
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

  // 软限位校验
  const validation = validateTargetPosition(axis, absoluteTarget)
  if (!validation.valid) {
    feedback.pushToast(validation.warning || '目标位置超出限位', 'error')
    return
  }
  if (validation.warning) {
    // 接近限位时显示警告但允许继续
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

function clearCurrentError(): void {
  if (!selectedId.value) return
  const status = motion.statusById(selectedId.value)
  if (status && status.lastError) {
    status.lastError = ''
  }
}

async function adjustByStep(axis: AxisName, direction: 'forward' | 'reverse'): Promise<void> {
  if (!selectedId.value || !controllerConnected.value || !currentStatus.value) return
  const state = ensureAxisLocalState(selectedId.value, axis)
  const axisStatus = currentStatus.value.axes.find((a) => a.name === axis)
  if (!axisStatus) return
  if (!Number.isFinite(state.step) || state.step <= 0) {
    feedback.pushToast('步长必须为正数', 'error')
    return
  }
  const delta = direction === 'forward' ? state.step : -state.step
  if ((delta > 0 && axisStatus.posLimit) || (delta < 0 && axisStatus.negLimit)) {
    feedback.pushToast('当前方向限位已触发，禁止继续点动', 'error')
    return
  }
  const validation = validateTargetPosition(axis, axisStatus.position + delta)
  if (!validation.valid) {
    feedback.pushToast(validation.warning || '目标位置超出限位', 'error')
    return
  }
  if (currentProfile.value?.type === 'B140-MC') {
    await motion.moveBy(selectedId.value, axis, delta)
    return
  }
  await motion.moveTo(selectedId.value, axis, axisStatus.position + delta)
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
  const min = Math.min(...data)
  const max = Math.max(...data)
  if (!Number.isFinite(min) || !Number.isFinite(max) || min === max) {
    return {}
  }
  // Single-axis-color sparkline: opacity encodes recency/position ratio.
  // No hue rotation — DESIGN.md "calm, no decorative motion".
  const segments: string[] = []
  const n = data.length
  for (let i = 0; i < n; i += 1) {
    const t = n === 1 ? 0 : (i / (n - 1)) * 100
    const ratio = (data[i] - min) / (max - min)
    const alpha = (0.25 + ratio * 0.6).toFixed(2)
    segments.push(`color-mix(in srgb, var(--axis-hue, var(--accent-primary)) ${Math.round(Number(alpha) * 100)}%, transparent) ${t.toFixed(1)}%`)
  }
  return {
    backgroundImage: `linear-gradient(to right, ${segments.join(', ')})`
  }
}

function getAxisThemeClass(axisName: AxisName): string {
  const themeMap: Record<AxisName, string> = {
    X: 'axis-x-theme',
    Y: 'axis-y-theme',
    Z: 'axis-z-theme',
    U: 'axis-u-theme'
  }
  return themeMap[axisName] || ''
}

let unsubscribeStatus: (() => void) | null = null

/**
 * 键盘快捷键处理
 * Esc: 紧急停止
 * Arrow keys: 点动控制 (当某个轴被聚焦时)
 */
function handleKeydown(e: KeyboardEvent): void {
  // Esc 键触发紧急停止
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
  <div class="flex gap-4 motion-control-panel">
    <aside data-test="motion-panel-surface" class="motion-sidebar bg-[color:var(--bg-panel)] border border-[color:var(--border-default)] rounded-[var(--radius-md)] p-3 flex flex-col shadow-[var(--shadow-panel)]">
      <div class="flex items-center justify-between mb-2">
        <div class="text-[11px] font-semibold tracking-wide text-[color:var(--text-secondary)] uppercase">
          {{ i18n.t.motionController }}
        </div>
        <UiButton
          secondary size="sm"
          @click="showConfig = true"
        >
          {{ i18n.t.config }}
        </UiButton>
      </div>

      <div v-if="motion.profiles.length === 0" class="mt-4 text-[11px] text-[color:var(--text-muted)] space-y-1 leading-relaxed">
        <p class="font-semibold text-[color:var(--text-primary)]">{{ i18n.t.noControllerConfig }}</p>
        <p>{{ i18n.t.clickConfigToAdd }}</p>
      </div>

      <div v-else class="flex-1 overflow-auto space-y-1.5 mt-1 custom-scrollbar">
        <button
          v-for="p in motion.profiles"
          :key="p.id"
          type="button"
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
              <span class="truncate motion-item-name">{{ p.name }}</span>
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
            <span class="uppercase tracking-widest motion-item-type">{{ p.type }}</span>
          </div>
        </button>
      </div>
    </aside>

    <section data-test="motion-panel-surface" class="bg-[color:var(--bg-panel)] border border-[color:var(--border-default)] rounded-[var(--radius-lg)] flex flex-col shadow-[var(--shadow-panel)] overflow-hidden h-fit">
      <header class="flex items-center justify-between gap-4 border-b border-[color:var(--border-default)] bg-[color:var(--bg-panel)] px-5 py-3">
        <div class="flex items-center gap-3 min-w-0">
          <div class="motion-header-badge">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
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
              <h2 class="text-[15px] font-semibold tracking-tight text-[color:var(--text-primary)] truncate">
                {{ selectedId ? motion.profiles.find(p => p.id === selectedId)?.name || i18n.t.selectController : i18n.t.selectController }}
              </h2>
              <span
                v-if="selectedId"
                class="motion-header-type-tag"
              >
                {{ motion.profiles.find(p => p.id === selectedId)?.type }}
              </span>
            </div>
            <div class="flex items-center gap-1.5 mt-1">
              <span class="motion-status-dot" :class="currentStatus?.connected ? 'motion-status-dot--connected' : 'motion-status-dot--disconnected'"></span>
              <p class="text-[11px] font-semibold text-[color:var(--text-muted)]">
                {{ currentStatus?.connected ? i18n.t.systemOnline : i18n.t.systemOffline }}
                <span v-if="selectedId" class="ml-2 opacity-70">· {{ motion.profiles.find(p => p.id === selectedId)?.address }}</span>
              </p>
            </div>
          </div>
        </div>

        <!-- 右侧操作区：按逻辑分组 -->
        <div class="flex items-center gap-2">
          <!-- 连接/断开按钮组：未选择控制器时 disabled -->
          <UiButton
            secondary size="sm"
            @click="handleConnect"
            :disabled="!selectedId || currentStatus?.connected"
            :title="!selectedId ? '请先选择控制器' : i18n.t.connectBtn"
          >
            {{ i18n.t.connectBtn }}
          </UiButton>
          <UiButton
            secondary size="sm"
            @click="handleDisconnect"
            :disabled="!selectedId || !currentStatus?.connected"
            :title="!selectedId ? '请先选择控制器' : i18n.t.disconnectBtn"
          >
            {{ i18n.t.disconnectBtn }}
          </UiButton>

          <!-- 停止全部按钮 -->
          <UiButton
            secondary size="sm"
            @click="stop()"
            :disabled="!selectedId || !currentStatus?.connected"
          >
            {{ i18n.t.stopAll }}
          </UiButton>

          <!-- 紧急停止按钮：用分隔线与常规操作区分 -->
          <div class="w-px h-7 bg-[color:var(--border-default)] mx-1"></div>
          <UiButton
            variant="danger" size="lg"
            class="estop-btn"
            @click="emergencyStop"
            :disabled="!selectedId"
            title="紧急停止 (快捷键: Esc)"
          >
            <template #icon>
              <svg class="w-5 h-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
                <circle cx="12" cy="12" r="10"/>
                <line x1="15" y1="9" x2="9" y2="15"/>
                <line x1="9" y1="9" x2="15" y2="15"/>
              </svg>
            </template>
            {{ i18n.t.eStop }}
          </UiButton>
        </div>
      </header>

      <div
        v-if="currentStatus?.lastError"
        class="mx-5 mt-3 p-3 rounded-[var(--radius-md)] border border-[color:var(--accent-danger)]/40 bg-[color:var(--accent-danger)]/10 flex items-center gap-3"
      >
        <svg class="w-4 h-4 shrink-0 text-[color:var(--accent-danger)]" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <circle cx="12" cy="12" r="10"/>
          <line x1="12" y1="8" x2="12" y2="12"/>
          <line x1="12" y1="16" x2="12.01" y2="16"/>
        </svg>
        <div class="flex-1 min-w-0">
          <p class="text-[10px] font-bold uppercase tracking-wider text-[color:var(--accent-danger)]">{{ i18n.t.controllerAlarm }}</p>
          <p class="text-xs font-medium text-[color:var(--text-primary)] truncate">{{ currentStatus.lastError }}</p>
        </div>
        <UiButton
          secondary size="sm"
          @click="clearCurrentError"
        >
          <template #icon>
            <svg class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <line x1="18" y1="6" x2="6" y2="18"/>
              <line x1="6" y1="6" x2="18" y2="18"/>
            </svg>
          </template>
        </UiButton>
      </div>

      <div v-if="!selectedId" class="flex-1 flex flex-col items-center justify-center text-[color:var(--text-muted)] p-12 gap-3">
        <svg class="w-12 h-12 opacity-30" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
          <path d="M12 2v4"/>
          <path d="m16.2 7.8 2.9-2.9"/>
          <path d="M18 12h4"/>
          <path d="m16.2 16.2 2.9 2.9"/>
          <path d="M12 18v4"/>
          <path d="m4.9 19.1 2.9-2.9"/>
          <path d="M2 12h4"/>
          <path d="m4.9 4.9 2.9 2.9"/>
        </svg>
        <p class="text-sm font-semibold text-[color:var(--text-secondary)]">{{ i18n.t.selectController }}</p>
        <p class="text-xs">{{ i18n.t.selectControllerHint }}</p>
      </div>
      <div v-else class="flex flex-col min-h-0 overflow-auto custom-scrollbar">
        <div class="p-5">
          <!-- 已连接但没有配置轴时显示提示 -->
          <div v-if="axes.length === 0" class="flex flex-col items-center justify-center text-[color:var(--text-muted)] py-12">
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
            <p class="text-sm font-semibold">{{ i18n.t.noAxesConfigured || '未配置运动轴' }}</p>
            <p class="text-xs mt-1 opacity-60">{{ i18n.t.checkProfileAxes || '请在配置中启用至少一个轴' }}</p>
            <UiButton
              class="mt-3"
              secondary size="sm"
              @click="showConfig = true"
            >
              {{ i18n.t.openConfig || '打开配置' }}
            </UiButton>
          </div>
          <div v-else class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-3">
            <div
              v-for="axis in axes"
              :key="axis.name"
              class="axis-card group relative bg-[color:var(--bg-panel)] border border-[color:var(--border-default)] rounded-[var(--radius-lg)] p-3 flex flex-col gap-2 transition-all min-w-0 h-fit"
              :class="getAxisThemeClass(axis.name)"
            >
              <div class="flex items-center justify-between">
                <div class="flex items-center gap-3 min-w-0">
                  <div class="axis-badge h-9 w-9 shrink-0 rounded-md flex items-center justify-center font-bold text-lg">
                    {{ axis.name }}
                  </div>
                  <div class="min-w-0">
                    <span class="block text-[10px] font-semibold uppercase tracking-wider text-[color:var(--text-muted)]">{{ i18n.t.axisNode }}</span>
                    <span class="block text-xs font-semibold text-[color:var(--text-primary)] truncate">{{ i18n.t.axisMotionControl }}</span>
                  </div>
                </div>
                <div class="axis-moving-pill" :class="{ 'axis-moving-pill--active': axis.moving }">
                  <span class="motion-status-dot" :class="axis.moving ? 'motion-status-dot--moving' : 'motion-status-dot--disconnected'" />
                  <span>{{ axis.moving ? i18n.t.moving : i18n.t.idle }}</span>
                </div>
              </div>

              <div class="readout-display relative py-3 px-3 rounded-[var(--radius-md)] bg-[color:var(--bg-canvas)] overflow-hidden">
                <div class="flex items-baseline justify-between relative z-10">
                  <div class="text-2xl font-mono font-bold text-[color:var(--text-primary)] tracking-tight truncate readout-value">
                    {{
                      selectedId
                        ? (axis.position - getZeroOffset(selectedId as string, axis.name as AxisName)).toFixed(2)
                        : axis.position.toFixed(2)
                    }}
                  </div>
                  <div class="text-[11px] font-semibold text-[color:var(--text-muted)] uppercase tracking-wider shrink-0 ml-2">{{ getAxisUnit(axis.name as AxisName) }}</div>
                </div>
                <div class="mt-0.5">
                  <span class="text-[10px] text-[color:var(--text-muted)]">当前位置</span>
                </div>
                <div class="readout-sparkline" :style="historyBarStyle(axis.name as AxisName)"></div>
              </div>

              <!-- 监视区域 -->
              <div class="axis-section">
                <div class="axis-section-title">
                  <svg class="w-3 h-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M2 12s3-7 10-7 10 7 10 7-3 7-10 7-10-7-10-7Z"/><circle cx="12" cy="12" r="3"/></svg>
                  监视
                </div>
                <div class="space-y-1.5">
                  <!-- 限位指示 -->
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
                  <!-- 操作按钮 -->
                  <div class="flex gap-1.5">
                    <UiButton
                      secondary size="sm"
                      @click="setZero(axis.name as AxisName)"
                      :disabled="axis.moving || !controllerConnected"
                    >{{ i18n.t.setZero }}</UiButton>
                    <UiButton
                      variant="danger" size="sm"
                      @click="stop(axis.name as AxisName)"
                      :disabled="!axis.moving || !controllerConnected"
                    >{{ i18n.t.stop }}</UiButton>
                  </div>
                </div>
              </div>

              <!-- 点动区域 -->
              <div class="axis-section">
                <div class="axis-section-title">
                  <svg class="w-3 h-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m5 9 7-7 7 7"/><path d="m5 15 7 7 7-7"/></svg>
                  点动
                </div>
                <div class="jog-control-row">
                    <UiButton
                      secondary size="sm"
                      @click="adjustByStep(axis.name as AxisName, 'reverse')"
                      :disabled="axis.moving || !controllerConnected"
                    >−</UiButton>
                  <div class="jog-input-wrap">
                    <UiInputNumber
                      v-model="ensureAxisLocalState(selectedId as string, axis.name as AxisName).step"
                      class="input-width-80"
                      :disabled="axis.moving || !controllerConnected"
                    />
                    <span class="jog-unit">{{ getAxisUnit(axis.name as AxisName) }}</span>
                  </div>
                    <UiButton
                      secondary size="sm"
                      @click="adjustByStep(axis.name as AxisName, 'forward')"
                      :disabled="axis.moving || !controllerConnected"
                    >+</UiButton>
                </div>
              </div>

              <!-- 定位区域 -->
              <div class="axis-section">
                <div class="axis-section-title">
                  <svg class="w-3 h-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 5v14"/><path d="m19 12-7 7-7-7"/></svg>
                  定位
                </div>
                <div class="move-control-row">
                  <div class="move-input-wrap">
                    <UiInputNumber
                      v-model="ensureAxisLocalState(selectedId as string, axis.name as AxisName).targetPosition"
                      class="input-width-80"
                      :class="getLimitWarningClass(axis.name as AxisName, getAbsoluteTargetPosition(selectedId as string, axis.name as AxisName))"
                      :disabled="axis.moving || !controllerConnected"
                      placeholder="0.00"
                    />
                    <span class="move-unit">{{ getAxisUnit(axis.name as AxisName) }}</span>
                  </div>
                    <UiButton
                      variant="primary" size="sm"
                      @click="move(axis.name as AxisName)"
                      :disabled="axis.moving || !controllerConnected"
                    >{{ i18n.t.move }}</UiButton>
                </div>
                <!-- 限位警告提示 -->
                <div
                  v-if="!validateTargetPosition(axis.name as AxisName, getAbsoluteTargetPosition(selectedId as string, axis.name as AxisName)).valid"
                  class="mt-1 flex items-center gap-1 text-[10px] text-[color:var(--accent-danger)]"
                >
                  <svg class="w-3 h-3 shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/></svg>
                  {{ validateTargetPosition(axis.name as AxisName, getAbsoluteTargetPosition(selectedId as string, axis.name as AxisName)).warning }}
                </div>
                <div
                  v-else-if="validateTargetPosition(axis.name as AxisName, getAbsoluteTargetPosition(selectedId as string, axis.name as AxisName)).warning"
                  class="mt-1 flex items-center gap-1 text-[10px] text-[color:var(--accent-warning)]"
                >
                  <svg class="w-3 h-3 shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M10.29 3.86 1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg>
                  {{ validateTargetPosition(axis.name as AxisName, getAbsoluteTargetPosition(selectedId as string, axis.name as AxisName)).warning }}
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </section>

    <MotionControllerConfig :open="showConfig" :current-id="selectedId" @close="showConfig = false" />
  </div>
</template>

<style scoped>
.motion-sidebar {
  width: 244px;
  flex-shrink: 0;
}

.motion-header-badge {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border-radius: var(--radius-md);
  background: var(--bg-panel-strong);
  border: 1px solid var(--border-default);
  color: var(--accent-primary);
  flex-shrink: 0;
}
.motion-header-badge svg {
  width: 16px;
  height: 16px;
}

.motion-header-type-tag {
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  padding: 2px 6px;
  border-radius: var(--radius-sm, 4px);
  background: var(--bg-panel-strong);
  border: 1px solid var(--border-default);
  color: var(--text-secondary);
  flex-shrink: 0;
}

.axis-card.axis-x-theme { --axis-hue: var(--axis-x); --axis-hue-soft: var(--axis-x-soft); }
.axis-card.axis-y-theme { --axis-hue: var(--axis-y); --axis-hue-soft: var(--axis-y-soft); }
.axis-card.axis-z-theme { --axis-hue: var(--axis-z); --axis-hue-soft: var(--axis-z-soft); }
.axis-card.axis-u-theme { --axis-hue: var(--axis-u); --axis-hue-soft: var(--axis-u-soft); }

.axis-card {
  transition: border-color var(--motion-base) var(--easing-standard);
}

.axis-card:hover {
  border-color: color-mix(in srgb, var(--axis-hue) 60%, var(--border-default));
}

.axis-card .axis-badge {
  background: var(--axis-hue-soft);
  color: var(--axis-hue);
  font-variant-numeric: tabular-nums;
}

.axis-moving-pill {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 3px 8px;
  border-radius: 999px;
  background: var(--bg-canvas);
  border: 1px solid var(--border-default);
  font-size: 11px;
  font-weight: 600;
  color: var(--text-muted);
  flex-shrink: 0;
  transition: color var(--motion-fast) var(--easing-standard), border-color var(--motion-fast) var(--easing-standard);
}
.axis-moving-pill--active {
  color: var(--accent-success);
  border-color: color-mix(in srgb, var(--accent-success) 40%, var(--border-default));
}

.limit-indicator {
  width: var(--space-2);
  height: var(--space-2);
  border-radius: 50%;
  border: 1px solid var(--border-default);
  transition: background-color var(--motion-base) var(--easing-standard), border-color var(--motion-base) var(--easing-standard);
}

.limit-indicator.active {
  background: var(--accent-danger);
  border-color: var(--accent-danger);
}

/* Data readout: solid surface, no hover fade (operators read these numbers). */
.readout-display {
  position: relative;
  border: 1px solid color-mix(in srgb, var(--axis-hue, var(--border-default)) 14%, var(--border-default));
}
.readout-value {
  font-variant-numeric: tabular-nums;
}
.readout-sparkline {
  position: absolute;
  left: 0;
  right: 0;
  bottom: 0;
  height: 2px;
  pointer-events: none;
}

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

.custom-scrollbar::-webkit-scrollbar {
  width: 6px;
  height: 6px;
}

.custom-scrollbar::-webkit-scrollbar-track {
  background: transparent;
}

.custom-scrollbar::-webkit-scrollbar-thumb {
  background: var(--border-strong);
  border-radius: 3px;
}

.custom-scrollbar::-webkit-scrollbar-thumb:hover {
  background: var(--text-muted);
}

.motion-list-item {
  padding: var(--space-3);
  border-radius: var(--radius-md);
  background: var(--bg-panel-strong);
  border: 1px solid transparent;
  transition: background-color var(--motion-fast) var(--easing-standard), border-color var(--motion-fast) var(--easing-standard);
  cursor: pointer;
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  align-items: stretch;
  justify-content: flex-start;
  height: auto;
}

.motion-list-item:hover {
  background: var(--bg-canvas);
  border-color: var(--border-default);
}

/* Selected state is a neutral selection cue — distinct from the connected/online color. */
.motion-list-item--active {
  background: var(--bg-canvas);
  border-color: var(--accent-primary);
  box-shadow: inset 2px 0 0 0 var(--accent-primary);
}

.motion-list-item:focus-visible {
  outline: none;
  box-shadow: 0 0 0 1px var(--focus-ring), 0 0 0 3px var(--focus-ring-soft);
}

.motion-item-name {
  font-size: var(--font-size-sm);
  font-weight: 600;
  color: var(--text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.motion-item-meta {
  font-size: var(--font-size-xs);
  color: var(--text-muted);
  gap: var(--space-2);
  font-variant-numeric: tabular-nums;
}

.motion-item-type {
  font-size: var(--font-size-xs);
  color: var(--text-muted);
}

.motion-status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
  background: var(--text-muted);
}

/* No neon glow — DESIGN.md: instrument-grade, calm. */
.motion-status-dot--connected {
  background: var(--accent-success);
}

.motion-status-dot--moving {
  background: var(--accent-success);
  animation: motion-dot-pulse 1.4s ease-in-out infinite;
}

@keyframes motion-dot-pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.45; }
}

.motion-status-dot--disconnected {
  background: var(--text-muted);
}

.motion-status-text {
  display: flex;
  align-items: center;
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.02em;
  color: var(--text-muted);
}

.motion-status-text--connected {
  color: var(--accent-success);
}

/* Axis functional sections — tighter nesting, no double background. */
.axis-section {
  padding: 0.5rem 0.625rem;
  background: var(--bg-canvas);
  border-radius: var(--radius-md);
  margin-top: 0.25rem;
}

.axis-section-title {
  display: flex;
  align-items: center;
  gap: 0.375rem;
  margin-bottom: 0.375rem;
  font-size: 11px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  color: var(--text-muted);
}

.axis-section-title svg {
  color: var(--axis-hue, var(--accent-primary));
}

/* Limit status row sits on the canvas background; no inner panel surface. */
.limit-status-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0;
}

.limit-status-item {
  display: flex;
  align-items: center;
  gap: 0.375rem;
}

.limit-status-label {
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.02em;
  color: var(--text-muted);
}

.limit-indicator-sm {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  border: 1px solid var(--border-default);
  background: transparent;
  transition: background-color var(--motion-base) var(--easing-standard), border-color var(--motion-base) var(--easing-standard);
}

.limit-indicator-sm.active {
  background: var(--accent-danger);
  border-color: var(--accent-danger);
}

/* Jog control row */
.jog-control-row {
  display: flex;
  align-items: center;
  gap: 0.375rem;
}

.jog-input-wrap {
  flex: 1;
  min-width: 0;
  display: flex;
  align-items: center;
  gap: 0.375rem;
}

.jog-unit {
  font-size: 11px;
  font-weight: 600;
  color: var(--text-muted);
  pointer-events: none;
  white-space: nowrap;
  flex-shrink: 0;
}

/* Move control row */
.move-control-row {
  display: flex;
  align-items: center;
  gap: 0.375rem;
}

.move-input-wrap {
  flex: 1;
  min-width: 0;
  display: flex;
  align-items: center;
  gap: 0.375rem;
}

.move-unit {
  font-size: 11px;
  font-weight: 600;
  color: var(--text-muted);
  pointer-events: none;
  white-space: nowrap;
  flex-shrink: 0;
}

.input-width-80 {
  width: 80px;
  flex-shrink: 0;
}

/* Emergency stop: visually weighted via the danger variant itself.
   A single subtle ring marks it as the high-consequence control without
   adding the neon halo DESIGN.md forbids. */
.estop-btn {
  min-width: 112px;
  font-weight: 700 !important;
  letter-spacing: 0.04em;
  box-shadow: 0 0 0 1px color-mix(in srgb, var(--accent-danger) 35%, transparent);
  transition: box-shadow 0.15s ease, transform 0.1s ease;
}
.estop-btn:not(:disabled):hover {
  box-shadow: 0 0 0 2px color-mix(in srgb, var(--accent-danger) 50%, transparent);
}
.estop-btn:not(:disabled):active {
  transform: scale(0.98);
}
</style>
