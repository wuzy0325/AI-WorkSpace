<script setup lang="ts">
import { onMounted, onBeforeUnmount, computed, reactive, ref, watch } from 'vue'
import { useMotionStore } from '@stores/motionStore'
import { useI18nStore } from '@stores/i18nStore'
import { useFeedbackStore } from '@stores/feedbackStore'
import type { AxisName } from '@shared/types/motion'
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
  /** 控制面板展开状态: monitor | jog | move */
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
  if (!selectedId.value || !controllerConnected.value) return
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
  const delta = direction === 'forward' ? state.step : -state.step
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
</script>

<template>
  <div class="flex h-full gap-4 motion-control-panel">
    <aside data-test="motion-panel-surface" class="w-64 bg-[color:var(--bg-panel)] border border-[color:var(--border-default)] rounded-[var(--radius-md)] p-3 flex flex-col shadow-[var(--shadow-panel)]">
      <div class="flex items-center justify-between mb-2">
        <div class="text-[11px] font-semibold tracking-wide text-[color:var(--text-secondary)] uppercase">
          {{ i18n.t.motionController }}
        </div>
        <button
          class="px-2 py-0.5 text-[10px] rounded-lg transition-colors border border-[color:var(--border-default)] text-[color:var(--text-muted)] hover:text-[color:var(--text-primary)] hover:bg-[color:var(--bg-panel-strong)]"
          @click="showConfig = true"
        >
          {{ i18n.t.config }}
        </button>
      </div>

      <div v-if="motion.profiles.length === 0" class="mt-4 text-[11px] text-[color:var(--text-muted)] space-y-1 leading-relaxed">
        <p class="font-semibold text-[color:var(--text-primary)]">{{ i18n.t.noControllerConfig }}</p>
        <p>{{ i18n.t.clickConfigToAdd }}</p>
      </div>

      <div v-else class="flex-1 overflow-auto space-y-2 mt-1 custom-scrollbar">
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

    <section data-test="motion-panel-surface" class="flex-1 bg-[color:var(--bg-panel)] border border-[color:var(--border-default)] rounded-[var(--radius-lg)] flex flex-col shadow-[var(--shadow-panel)] overflow-hidden">
      <header class="flex items-center justify-between gap-4 border-b border-[color:var(--border-default)] bg-[color:var(--bg-panel)] px-6 py-4">
        <div class="flex items-center gap-4 min-w-0">
          <div class="flex h-12 w-12 items-center justify-center rounded-xl bg-gradient-to-br from-[color:var(--accent-primary)]/20 to-[color:var(--accent-primary)]/5 border border-[color:var(--accent-primary)]/20">
            <svg class="w-6 h-6 text-[color:var(--accent-primary)]" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
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
              <h2 class="text-base font-bold tracking-tight text-[color:var(--text-primary)] truncate">
                {{ selectedId ? motion.profiles.find(p => p.id === selectedId)?.name || i18n.t.selectController : i18n.t.selectController }}
              </h2>
              <span
                v-if="selectedId"
                class="px-2 py-0.5 text-[9px] font-bold rounded-full border border-[color:var(--border-default)] bg-[color:var(--bg-panel-strong)] text-[color:var(--text-secondary)] uppercase tracking-wider"
              >
                {{ motion.profiles.find(p => p.id === selectedId)?.type }}
              </span>
            </div>
            <div class="flex items-center gap-2 mt-1">
              <span class="flex h-2 w-2 rounded-full" :class="currentStatus?.connected ? 'bg-[color:var(--accent-success)] shadow-[0_0_8px_var(--accent-success)]' : 'bg-[color:var(--text-muted)]'"></span>
              <p class="text-[10px] font-bold text-[color:var(--text-muted)] uppercase tracking-tight">
                {{ currentStatus?.connected ? i18n.t.systemOnline : i18n.t.systemOffline }}
                <span v-if="selectedId" class="ml-2 opacity-60">· {{ motion.profiles.find(p => p.id === selectedId)?.address }}</span>
              </p>
            </div>
          </div>
        </div>

        <div class="flex items-center gap-2">
          <button
            class="h-10 px-4 rounded-lg text-xs font-bold transition-all active:scale-95 text-white hover:opacity-90 disabled:opacity-40 disabled:cursor-not-allowed bg-[color:var(--accent-success)] shadow-[0_4px_12px_color-mix(in_srgb,var(--accent-success)_30%,transparent)]"
            @click="handleConnect"
            :disabled="!selectedId || currentStatus?.connected"
          >
            {{ i18n.t.connectBtn }}
          </button>
          <button
            class="h-10 px-4 rounded-lg text-xs font-bold transition-all active:scale-95 disabled:opacity-40 disabled:cursor-not-allowed border border-[color:var(--border-default)] text-[color:var(--text-muted)] hover:text-[color:var(--text-primary)] hover:bg-[color:var(--bg-panel-strong)]"
            @click="handleDisconnect"
            :disabled="!selectedId || !currentStatus?.connected"
          >
            {{ i18n.t.disconnectBtn }}
          </button>
          <div class="w-px h-6 bg-[color:var(--border-default)] mx-1"></div>
          <button
            class="h-10 px-4 rounded-lg text-xs font-bold transition-all active:scale-95 hover:opacity-90 disabled:opacity-40 disabled:cursor-not-allowed text-white bg-[color:var(--accent-warning)] shadow-[0_4px_12px_color-mix(in_srgb,var(--accent-warning)_30%,transparent)]"
            @click="stop()"
            :disabled="!selectedId || !currentStatus?.connected"
          >
            {{ i18n.t.stopAll }}
          </button>
          <button
            class="btn-estop h-10 px-6 rounded-lg text-xs font-bold uppercase tracking-wider transition-all active:scale-95 disabled:opacity-40 disabled:cursor-not-allowed disabled:animate-none"
            @click="emergencyStop"
            :disabled="!selectedId"
            title="紧急停止 (快捷键: Esc)"
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

      <div
        v-if="currentStatus?.lastError"
        class="mx-6 mt-4 p-3 rounded-lg border border-[color:var(--accent-danger)]/30 bg-[color:var(--accent-danger)]/10 flex items-center gap-3"
      >
        <svg class="w-5 h-5 shrink-0 text-[color:var(--accent-danger)]" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <circle cx="12" cy="12" r="10"/>
          <line x1="12" y1="8" x2="12" y2="12"/>
          <line x1="12" y1="16" x2="12.01" y2="16"/>
        </svg>
        <div class="flex-1 min-w-0">
          <p class="text-[10px] font-bold uppercase tracking-wider text-[color:var(--accent-danger)]">{{ i18n.t.controllerAlarm }}</p>
          <p class="text-xs font-semibold text-[color:var(--text-primary)] truncate">{{ currentStatus.lastError }}</p>
        </div>
        <button
          class="shrink-0 h-6 w-6 rounded-md flex items-center justify-center text-[color:var(--accent-danger)]/60 hover:text-[color:var(--accent-danger)] hover:bg-[color:var(--accent-danger)]/10 transition-all"
          @click="clearCurrentError"
        >
          <svg class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <line x1="18" y1="6" x2="6" y2="18"/>
            <line x1="6" y1="6" x2="18" y2="18"/>
          </svg>
        </button>
      </div>

      <div v-if="!selectedId" class="flex-1 flex flex-col items-center justify-center text-[color:var(--text-muted)] p-12">
        <div class="text-6xl mb-4 opacity-20 italic">{{ i18n.t.selectController }}</div>
        <p class="text-xs font-bold tracking-[0.2em] uppercase">{{ i18n.t.selectControllerHint }}</p>
      </div>
      <div v-else class="flex flex-col flex-1 min-h-0">
        <div class="flex-1 min-h-0 overflow-auto p-6 custom-scrollbar">
          <!-- 已连接但没有配置轴时显示提示 -->
          <div v-if="axes.length === 0" class="flex flex-col items-center justify-center h-full text-[color:var(--text-muted)]">
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
            <button
              class="mt-4 px-4 py-2 rounded-lg text-xs font-bold transition-colors border border-[color:var(--border-default)] text-[color:var(--text-muted)] hover:text-[color:var(--text-primary)] hover:bg-[color:var(--bg-panel-strong)]"
              @click="showConfig = true"
            >
              {{ i18n.t.openConfig || '打开配置' }}
            </button>
          </div>
          <div v-else class="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-4 gap-5">
            <div
              v-for="axis in axes"
              :key="axis.name"
              class="axis-card group relative bg-[color:var(--bg-panel)] border border-[color:var(--border-default)] rounded-[var(--radius-lg)] p-4 flex flex-col gap-3 transition-all min-w-[200px]"
              :class="getAxisThemeClass(axis.name)"
            >
              <div class="flex items-center justify-between">
                <div class="flex items-center gap-3 min-w-0">
                  <div class="axis-badge h-10 w-10 shrink-0 rounded-lg flex items-center justify-center font-black text-xl">
                    {{ axis.name }}
                  </div>
                  <div class="min-w-0">
                    <span class="block text-[10px] font-bold uppercase tracking-wider text-[color:var(--text-muted)]">{{ i18n.t.axisNode }}</span>
                    <span class="block text-xs font-semibold text-[color:var(--text-primary)] truncate">{{ i18n.t.axisMotionControl }}</span>
                  </div>
                </div>
                <div class="flex shrink-0 items-center gap-1.5 rounded-full px-2.5 py-1 bg-[color:var(--bg-canvas)]">
                  <span
                    class="w-2 h-2 rounded-full"
                    :class="axis.moving ? 'bg-[color:var(--accent-success)] shadow-[0_0_8px_var(--accent-success)] animate-pulse' : 'bg-[color:var(--text-muted)]'"
                  />
                  <span class="text-[10px] font-bold uppercase text-[color:var(--text-muted)]">{{ axis.moving ? i18n.t.moving : i18n.t.idle }}</span>
                </div>
              </div>

              <div class="readout-display relative py-4 px-4 rounded-[var(--radius-md)] bg-[color:var(--bg-canvas)] overflow-hidden group-hover:opacity-90 transition-opacity">
                <div class="flex items-baseline justify-between relative z-10">
                  <div class="text-3xl font-mono font-bold text-[color:var(--text-primary)] tracking-tight truncate">
                    {{
                      selectedId
                        ? (axis.position - getZeroOffset(selectedId as string, axis.name as AxisName)).toFixed(2)
                        : axis.position.toFixed(2)
                    }}
                  </div>
                  <div class="text-xs font-bold text-[color:var(--text-muted)] uppercase tracking-wider shrink-0 ml-2">{{ getAxisUnit(axis.name as AxisName) }}</div>
                </div>
                <div class="mt-1 flex items-center gap-1.5">
                  <span class="text-[10px] text-[color:var(--text-muted)]">当前位置</span>
                  <span
                    class="history-hint"
                    title="底部彩色条带显示最近 50 次位置采样历史，颜色从蓝(早)到绿(晚)渐变"
                  >
                    <svg class="w-3 h-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><line x1="12" y1="16" x2="12.01" y2="16"/><path d="M12 12v-4"/></svg>
                  </span>
                </div>
                <div class="absolute bottom-0 left-0 right-0 h-1.5 opacity-60">
                  <div class="h-full w-full" :style="historyBarStyle(axis.name as AxisName)"></div>
                </div>
              </div>

              <!-- 操作分层标签页 -->
              <div class="axis-panel-tabs">
                <button
                  class="axis-panel-tab"
                  :class="{ 'axis-panel-tab--active': ensureAxisLocalState(selectedId as string, axis.name as AxisName).expandedPanel === 'monitor' }"
                  @click="togglePanel(selectedId as string, axis.name as AxisName, 'monitor')"
                >
                  <svg class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M2 12s3-7 10-7 10 7 10 7-3 7-10 7-10-7-10-7Z"/><circle cx="12" cy="12" r="3"/></svg>
                  监视
                </button>
                <button
                  class="axis-panel-tab"
                  :class="{ 'axis-panel-tab--active': ensureAxisLocalState(selectedId as string, axis.name as AxisName).expandedPanel === 'jog' }"
                  @click="togglePanel(selectedId as string, axis.name as AxisName, 'jog')"
                  :disabled="!controllerConnected"
                >
                  <svg class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m5 9 7-7 7 7"/><path d="m5 15 7 7 7-7"/></svg>
                  点动
                </button>
                <button
                  class="axis-panel-tab"
                  :class="{ 'axis-panel-tab--active': ensureAxisLocalState(selectedId as string, axis.name as AxisName).expandedPanel === 'move' }"
                  @click="togglePanel(selectedId as string, axis.name as AxisName, 'move')"
                  :disabled="!controllerConnected"
                >
                  <svg class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 5v14"/><path d="m19 12-7 7-7-7"/></svg>
                  定位
                </button>
              </div>

              <!-- 监视面板 -->
              <div v-if="ensureAxisLocalState(selectedId as string, axis.name as AxisName).expandedPanel === 'monitor'" class="axis-panel-content">
                <div class="space-y-2">
                  <div class="flex items-center justify-between text-[10px]">
                    <span class="font-bold uppercase tracking-wider text-[color:var(--text-muted)]">状态</span>
                    <span class="font-semibold" :class="axis.moving ? 'text-[color:var(--accent-success)]' : 'text-[color:var(--text-muted)]'">
                      {{ axis.moving ? '运动中' : '静止' }}
                    </span>
                  </div>
                  <div class="flex items-center justify-between text-[10px]">
                    <span class="font-bold uppercase tracking-wider text-[color:var(--text-muted)]">归零状态</span>
                    <span class="font-semibold" :class="axis.homed ? 'text-[color:var(--accent-success)]' : 'text-[color:var(--accent-warning)]'">
                      {{ axis.homed ? '已归零' : '未归零' }}
                    </span>
                  </div>
                  <div class="flex items-center justify-between px-1 pt-1">
                    <div class="flex items-center gap-1.5 shrink-0">
                      <span class="limit-indicator" :class="axis.negLimit ? 'active' : ''"></span>
                      <span class="text-[9px] font-bold uppercase tracking-tighter text-[color:var(--text-muted)]">{{ i18n.t.negLimit }}</span>
                    </div>
                    <div class="flex items-center gap-1.5 shrink-0">
                      <span class="text-[9px] font-bold uppercase tracking-tighter text-[color:var(--text-muted)]">{{ i18n.t.posLimit }}</span>
                      <span class="limit-indicator" :class="axis.posLimit ? 'active' : ''"></span>
                    </div>
                  </div>
                  <div class="flex gap-2 pt-2">
                    <button
                      class="flex-1 min-w-0 h-8 rounded-md border border-[color:var(--border-default)] bg-[color:var(--bg-panel-strong)] text-xs font-bold uppercase tracking-wider text-[color:var(--text-secondary)] transition-all hover:bg-[color:var(--bg-canvas)] active:scale-95 disabled:opacity-40 truncate px-1"
                      @click="setZero(axis.name as AxisName)"
                      :disabled="axis.moving || !controllerConnected"
                    >{{ i18n.t.setZero }}</button>
                    <button
                      class="btn-stop flex-1 min-w-0 h-8 rounded-md text-xs font-bold uppercase tracking-wider transition-all active:scale-95 disabled:opacity-40 truncate px-1"
                      @click="stop(axis.name as AxisName)"
                      :disabled="!axis.moving || !controllerConnected"
                    >{{ i18n.t.stop }}</button>
                  </div>
                </div>
              </div>

              <!-- 点动面板 -->
              <div v-if="ensureAxisLocalState(selectedId as string, axis.name as AxisName).expandedPanel === 'jog'" class="axis-panel-content">
                <div class="space-y-3">
                  <div>
                    <div class="flex items-center justify-between mb-1.5">
                      <span class="text-[10px] font-bold uppercase tracking-wider text-[color:var(--text-muted)]">{{ i18n.t.jogStep }}</span>
                      <span class="text-[10px] font-semibold text-[color:var(--text-primary)]">{{ ensureAxisLocalState(selectedId as string, axis.name as AxisName).step }} {{ getAxisUnit(axis.name as AxisName) }}</span>
                    </div>
                    <div class="flex gap-2">
                      <button
                        class="btn-step"
                        @click="adjustByStep(axis.name as AxisName, 'reverse')"
                        :disabled="axis.moving || !controllerConnected"
                      >−</button>
                      <input
                        v-model.number="ensureAxisLocalState(selectedId as string, axis.name as AxisName).step"
                        type="number"
                        class="input-field flex-1 min-w-0 h-10 px-2 text-center text-sm font-semibold"
                        :disabled="axis.moving || !controllerConnected"
                      />
                      <button
                        class="btn-step"
                        @click="adjustByStep(axis.name as AxisName, 'forward')"
                        :disabled="axis.moving || !controllerConnected"
                      >+</button>
                    </div>
                  </div>
                  <p class="text-[9px] text-[color:var(--text-muted)] leading-relaxed">
                    点击 −/+ 按设定步长移动，或修改步长值后点击。
                  </p>
                </div>
              </div>

              <!-- 定位面板 -->
              <div v-if="ensureAxisLocalState(selectedId as string, axis.name as AxisName).expandedPanel === 'move'" class="axis-panel-content">
                <div class="space-y-3">
                  <div>
                  <span class="block text-[10px] font-bold uppercase tracking-wider text-[color:var(--text-muted)] mb-1.5">{{ i18n.t.targetPosition }}</span>
                  <div class="flex gap-2">
                    <input
                      v-model.number="ensureAxisLocalState(selectedId as string, axis.name as AxisName).targetPosition"
                      type="number"
                      class="input-field flex-1 min-w-0 h-10 px-3 text-sm font-semibold"
                      :class="getLimitWarningClass(axis.name as AxisName, getZeroOffset(selectedId as string, axis.name as AxisName) + ensureAxisLocalState(selectedId as string, axis.name as AxisName).targetPosition)"
                      :disabled="axis.moving || !controllerConnected"
                      placeholder="0.00"
                    />
                    <button
                      class="btn-move h-10 px-4 shrink-0 rounded-lg text-xs font-bold uppercase tracking-wider active:scale-95 transition-all hover:opacity-90 disabled:opacity-40"
                      @click="move(axis.name as AxisName)"
                      :disabled="axis.moving || !controllerConnected"
                    >{{ i18n.t.move }}</button>
                  </div>
                  <!-- 限位警告提示 -->
                  <div
                    v-if="!validateTargetPosition(axis.name as AxisName, getZeroOffset(selectedId as string, axis.name as AxisName) + ensureAxisLocalState(selectedId as string, axis.name as AxisName).targetPosition).valid"
                    class="mt-1.5 flex items-center gap-1 text-[10px] text-[color:var(--accent-danger)]"
                  >
                    <svg class="w-3 h-3 shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/></svg>
                    {{ validateTargetPosition(axis.name as AxisName, getZeroOffset(selectedId as string, axis.name as AxisName) + ensureAxisLocalState(selectedId as string, axis.name as AxisName).targetPosition).warning }}
                  </div>
                  <div
                    v-else-if="validateTargetPosition(axis.name as AxisName, getZeroOffset(selectedId as string, axis.name as AxisName) + ensureAxisLocalState(selectedId as string, axis.name as AxisName).targetPosition).warning"
                    class="mt-1.5 flex items-center gap-1 text-[10px] text-[color:var(--accent-warning)]"
                  >
                    <svg class="w-3 h-3 shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M10.29 3.86 1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg>
                    {{ validateTargetPosition(axis.name as AxisName, getZeroOffset(selectedId as string, axis.name as AxisName) + ensureAxisLocalState(selectedId as string, axis.name as AxisName).targetPosition).warning }}
                  </div>
                </div>
                <p class="text-[9px] text-[color:var(--text-muted)] leading-relaxed">
                  输入目标位置（相对归零偏移），点击移动。
                </p>
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
.axis-card.axis-x-theme { --axis-hue: var(--axis-x); --axis-hue-soft: var(--axis-x-soft); }
.axis-card.axis-y-theme { --axis-hue: var(--axis-y); --axis-hue-soft: var(--axis-y-soft); }
.axis-card.axis-z-theme { --axis-hue: var(--axis-z); --axis-hue-soft: var(--axis-z-soft); }
.axis-card.axis-u-theme { --axis-hue: var(--axis-u); --axis-hue-soft: var(--axis-u-soft); }

.axis-card {
  transition: all var(--motion-base) var(--easing-standard);
}

.axis-card:hover {
  border-color: var(--axis-hue);
  box-shadow: 0 10px 15px -3px color-mix(in srgb, var(--axis-hue) 15%, transparent), 0 4px 6px -4px color-mix(in srgb, var(--axis-hue) 10%, transparent);
}

.axis-card .axis-badge {
  background: var(--axis-hue-soft);
  color: var(--axis-hue);
}

.axis-card .btn-step:hover {
  background: var(--axis-hue);
  border-color: var(--axis-hue);
  color: white;
}

.axis-card .btn-move {
  background: var(--axis-hue);
  color: white;
  box-shadow: 0 4px 14px 0 color-mix(in srgb, var(--axis-hue) 30%, transparent);
}

.btn-step {
  height: 40px;
  width: 40px;
  flex-shrink: 0;
  border-radius: var(--radius-md);
  border: 1px solid var(--border-default);
  background: var(--bg-panel);
  color: var(--text-primary);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 1.25rem;
  font-weight: 600;
  transition: background-color var(--motion-fast) var(--easing-standard), border-color var(--motion-fast) var(--easing-standard), color var(--motion-fast) var(--easing-standard), transform var(--motion-fast) var(--easing-standard);
}

.btn-step:hover:not(:disabled) {
  background: var(--accent-primary);
  border-color: var(--accent-primary);
  color: white;
}

.btn-step:focus-visible {
  outline: none;
  box-shadow: 0 0 0 1px var(--focus-ring), 0 0 0 3px var(--focus-ring-soft);
}

.btn-step:active:not(:disabled) {
  transform: scale(0.92);
}

.btn-step:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.btn-stop {
  background: var(--bg-panel-strong);
  border: 1px solid var(--border-default);
  color: var(--text-secondary);
}

.btn-stop:focus-visible {
  outline: none;
  box-shadow: 0 0 0 1px var(--focus-ring), 0 0 0 3px var(--focus-ring-soft);
}

.btn-stop:hover:not(:disabled) {
  background: var(--accent-danger);
  border-color: var(--accent-danger);
  color: white;
}

.btn-stop:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.btn-estop {
  background: var(--accent-danger);
  color: white;
  border: 2px solid var(--accent-danger);
  box-shadow: 0 0 0 0 color-mix(in srgb, var(--accent-danger) 50%, transparent);
  display: inline-flex;
  align-items: center;
  gap: 0.375rem;
  animation: estop-pulse 2s ease-in-out infinite;
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

.btn-stop {
  transition: background-color var(--motion-fast) var(--easing-standard), border-color var(--motion-fast) var(--easing-standard), color var(--motion-fast) var(--easing-standard), transform var(--motion-fast) var(--easing-standard);
}

.input-field {
  background: var(--bg-canvas);
  border: 1px solid transparent;
  border-radius: var(--radius-md);
  color: var(--text-primary);
  transition: border-color var(--motion-fast) var(--easing-standard), box-shadow var(--motion-fast) var(--easing-standard);
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

/* 软限位警告样式 */
.input-field.limit-exceeded {
  border-color: var(--accent-danger);
  background: color-mix(in srgb, var(--accent-danger) 8%, var(--bg-canvas));
  box-shadow: 0 0 0 2px color-mix(in srgb, var(--accent-danger) 25%, transparent);
}

.input-field.limit-near {
  border-color: var(--accent-warning);
  background: color-mix(in srgb, var(--accent-warning) 8%, var(--bg-canvas));
}

.limit-indicator {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  border: 1px solid var(--border-default);
  transition: background-color var(--motion-base) var(--easing-standard), border-color var(--motion-base) var(--easing-standard), box-shadow var(--motion-base) var(--easing-standard);
}

.limit-indicator.active {
  background: var(--accent-danger);
  box-shadow: 0 0 6px var(--accent-danger);
  border-color: transparent;
}

.readout-display {
  position: relative;
  transition: opacity var(--motion-fast) var(--easing-standard);
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
  padding: 0.75rem;
  border-radius: var(--radius-md);
  background: var(--bg-panel-strong);
  border: 1px solid transparent;
  transition: background-color var(--motion-fast) var(--easing-standard), border-color var(--motion-fast) var(--easing-standard);
  cursor: pointer;
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}

.motion-list-item:hover {
  background: var(--bg-canvas);
  border-color: var(--border-default);
}

.motion-list-item--active {
  background: color-mix(in srgb, var(--accent-success) 8%, transparent) !important;
  border-color: color-mix(in srgb, var(--accent-success) 30%, transparent) !important;
}

.motion-list-item:focus-visible {
  outline: none;
  box-shadow: 0 0 0 1px var(--focus-ring), 0 0 0 3px var(--focus-ring-soft);
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
  text-transform: uppercase;
  color: var(--text-muted);
}

.motion-status-text--connected {
  color: var(--accent-success);
}

/* 轴面板标签页 */
.axis-panel-tabs {
  display: flex;
  gap: 0.25rem;
  padding: 0.25rem;
  background: var(--bg-canvas);
  border-radius: var(--radius-md);
  margin-bottom: 0.75rem;
}

.axis-panel-tab {
  flex: 1;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 0.25rem;
  padding: 0.375rem 0.5rem;
  border-radius: calc(var(--radius-md) - 2px);
  font-size: 0.6875rem;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  color: var(--text-muted);
  background: transparent;
  border: none;
  cursor: pointer;
  transition: all var(--motion-fast) var(--easing-standard);
}

.axis-panel-tab:hover:not(:disabled) {
  color: var(--text-primary);
  background: var(--bg-panel);
}

.axis-panel-tab--active {
  color: var(--text-primary);
  background: var(--bg-panel);
  box-shadow: 0 1px 3px color-mix(in srgb, #000 15%, transparent);
}

.axis-panel-tab:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.axis-panel-content {
  animation: panel-fade-in var(--motion-fast) var(--easing-standard);
}

@keyframes panel-fade-in {
  from {
    opacity: 0;
    transform: translateY(-4px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}
</style>
