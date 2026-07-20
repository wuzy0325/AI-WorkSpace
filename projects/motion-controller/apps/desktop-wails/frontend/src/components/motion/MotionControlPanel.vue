<script setup lang="ts">
// MotionControlPanel — 运动控制主面板（容器编排）
// 子组件：MotionSidebar / MotionToolbar / MotionBanner / AxisCard
// 纯函数工具：composables/useAxisCard
//
// 本组件只负责：
//   1. 管理选中的控制器 ID
//   2. 装配子组件
//   3. 维护每轴的本地状态（targetPosition / step / history）
//   4. 转发子组件事件到 motionStore
//
// 轴卡片视图派生（readout/limits/hint/historyStyle）已下沉到 AxisCard 内部 computed，
// 父组件只传原始数据，避免模板内调用方法（state#§12）。

import { onMounted, onBeforeUnmount, computed, reactive, ref, watch } from 'vue'
import { useMotionStore } from '@stores/motionStore'
import { useI18nStore } from '@stores/i18nStore'
import { useFeedbackStore } from '@stores/feedbackStore'
import type { AxisName } from '@shared/types/motion'
import { DEFAULT_JOG_STEP } from './motionConfigEditor'
import MotionSidebar from './MotionSidebar.vue'
import MotionToolbar from './MotionToolbar.vue'
import MotionBanner from './MotionBanner.vue'
import AxisCard from './AxisCard.vue'
import MotionControllerConfig from './MotionControllerConfig.vue'
import { validateTargetPosition } from '@composables/useAxisCard'

const motion = useMotionStore()
const i18n = useI18nStore()
const feedback = useFeedbackStore()

// 选中状态
const selectedId = ref<string | null>(null)
const showConfig = ref(false)

/* ============================================================
   每轴本地状态（targetPosition / step）
   按 controllerId + axisName 索引，避免跨控制器串扰。
   ============================================================ */
interface AxisLocalState {
  targetPosition: number
  step: number
}

type AxisLocalStateMap = Record<AxisName, AxisLocalState>

const axisLocalState = reactive<Record<string, AxisLocalStateMap>>({})

function ensureAxisLocalState(controllerId: string, axisName: AxisName): AxisLocalState {
  if (!axisLocalState[controllerId]) {
    axisLocalState[controllerId] = {
      X: { targetPosition: 0, step: DEFAULT_JOG_STEP },
      Y: { targetPosition: 0, step: DEFAULT_JOG_STEP },
      Z: { targetPosition: 0, step: DEFAULT_JOG_STEP },
      U: { targetPosition: 0, step: DEFAULT_JOG_STEP },
    }
  }
  return axisLocalState[controllerId][axisName]
}

/* ============================================================
   零点偏移（每控制器 × 每轴）
   ============================================================ */
type AxisZeroOffsetMap = Record<AxisName, number>
const axisZeroOffset = reactive<Record<string, AxisZeroOffsetMap>>({})

function ensureZeroOffsetMap(controllerId: string): AxisZeroOffsetMap {
  if (!axisZeroOffset[controllerId]) {
    axisZeroOffset[controllerId] = { X: 0, Y: 0, Z: 0, U: 0 }
  }
  return axisZeroOffset[controllerId]
}

function getZeroOffset(controllerId: string, axisName: AxisName): number {
  return ensureZeroOffsetMap(controllerId)[axisName] ?? 0
}

function setZeroOffset(controllerId: string, axisName: AxisName, value: number): void {
  ensureZeroOffsetMap(controllerId)[axisName] = value
}

/* ============================================================
   历史位置记录（最多 MAX_HISTORY 条）
   ============================================================ */
type AxisHistoryMap = Map<AxisName, number[]>
const axisHistory = ref<Map<string, AxisHistoryMap>>(new Map())
const MAX_HISTORY = 50

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

function getAxisHistory(controllerId: string, axisName: AxisName): number[] {
  const ctrlMap = axisHistory.value.get(controllerId)
  if (!ctrlMap) return []
  return ctrlMap.get(axisName) ?? []
}

/* ============================================================
   派生状态（一次性 computed，避免模板内重复调用）
   ============================================================ */
const currentStatus = computed(() =>
  selectedId.value ? motion.statusById(selectedId.value) : undefined
)

const currentProfile = computed(() =>
  selectedId.value ? motion.profiles.find((p) => p.id === selectedId.value) : undefined
)

const controllerConnected = computed(() => Boolean(currentStatus.value?.connected))

const axes = computed(() => currentStatus.value?.axes ?? [])

const emergencyStopped = computed(() => Boolean(currentStatus.value?.emergencyStopped))
const lastError = computed(() => currentStatus.value?.lastError ?? '')

/** 当前控制器的本地轴状态 map（只读视图，模板里直接索引访问，不再调用方法） */
const currentAxisLocalState = computed<AxisLocalStateMap | null>(() =>
  selectedId.value ? axisLocalState[selectedId.value] ?? null : null
)

/** 当前控制器的零点偏移 map */
const currentZeroOffsetMap = computed<AxisZeroOffsetMap | null>(() =>
  selectedId.value ? axisZeroOffset[selectedId.value] ?? null : null
)

/* ============================================================
   控制器选择
   ============================================================ */
function selectController(id: string): void {
  selectedId.value = id
}

/* ============================================================
   控制器操作
   ============================================================ */
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

async function handleStopAll(): Promise<void> {
  if (!selectedId.value) return
  await motion.stop(selectedId.value)
}

async function handleEmergencyStop(): Promise<void> {
  if (!selectedId.value) return
  await motion.emergencyStop(selectedId.value)
}

async function handleResetEmergencyStop(): Promise<void> {
  if (!selectedId.value) return
  await motion.resetEmergencyStop(selectedId.value)
}

function handleClearError(): void {
  if (!selectedId.value) return
  motion.clearError(selectedId.value)
}

/* ============================================================
   轴操作
   ============================================================ */
async function handleMove(axis: AxisName): Promise<void> {
  if (!selectedId.value || !controllerConnected.value) return
  const state = ensureAxisLocalState(selectedId.value, axis)
  const offset = getZeroOffset(selectedId.value, axis)
  const absoluteTarget = offset + state.targetPosition

  if (!Number.isFinite(absoluteTarget)) {
    feedback.pushToast(i18n.t.targetPositionInvalid, 'error')
    return
  }

  const validation = validateTargetPosition(axis, absoluteTarget, currentProfile.value)
  if (!validation.valid) {
    feedback.pushToast(validation.warning || i18n.t.targetPositionOutOfRange, 'error')
    return
  }
  if (validation.warning) {
    feedback.pushToast(validation.warning, 'warning')
  }

  await motion.moveTo(selectedId.value, axis, absoluteTarget)
}

async function handleJog(axis: AxisName, direction: 'forward' | 'reverse'): Promise<void> {
  if (!selectedId.value || !controllerConnected.value || !currentStatus.value) return
  const axisStatus = currentStatus.value.axes.find((a) => a.name === axis)
  if (!axisStatus) return
  const state = ensureAxisLocalState(selectedId.value, axis)
  if (!Number.isFinite(state.step) || state.step <= 0) {
    feedback.pushToast(i18n.t.stepMustBePositive, 'error')
    return
  }
  const delta = direction === 'forward' ? state.step : -state.step
  const validation = validateTargetPosition(axis, axisStatus.position + delta, currentProfile.value)
  if (!validation.valid) {
    feedback.pushToast(validation.warning || i18n.t.targetPositionOutOfRange, 'error')
    return
  }
  if ((delta > 0 && axisStatus.posLimit) || (delta < 0 && axisStatus.negLimit)) {
    feedback.pushToast(i18n.t.limitTriggeredDirectionForbidden, 'error')
    return
  }
  await motion.moveBy(selectedId.value, axis, delta)
}

async function handleStopAxis(axis: AxisName): Promise<void> {
  if (!selectedId.value || !controllerConnected.value) return
  await motion.stop(selectedId.value, axis)
}

async function handleSetZero(axis: AxisName): Promise<void> {
  if (!selectedId.value || !controllerConnected.value) return
  await motion.definePosition(selectedId.value, axis, 0)
  setZeroOffset(selectedId.value, axis, 0)
  const state = ensureAxisLocalState(selectedId.value, axis)
  state.targetPosition = 0
}

/* ============================================================
   配置保存回调
   ============================================================ */
async function handleProfileSaved(id: string): Promise<void> {
  selectedId.value = id
  await motion.refreshStatus()
}

/* ============================================================
   生命周期 + 状态订阅
   ============================================================ */
let unsubscribeStatus: (() => void) | null = null

function handleKeydown(e: KeyboardEvent): void {
  if (e.key === 'Escape') {
    e.preventDefault()
    handleEmergencyStop()
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

// 位置变化时追加历史
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

// profiles 列表变化时同步选中
watch(
  () => motion.profiles.map((p) => p.id),
  (ids) => {
    if (!selectedId.value || ids.includes(selectedId.value)) return
    selectedId.value = ids[0] ?? null
  }
)

// selectedId 变化时自动初始化本地状态（targetPosition/step/zeroOffset）
// 替代原本模板内调用 ensureAxisLocalState/ensureZeroOffsetMap 的写法（违反 state#§12）
watch(
  selectedId,
  (id) => {
    if (!id) return
    // 触发初始化（函数内部会创建空对象）
    ensureAxisLocalState(id, 'X')
    ensureZeroOffsetMap(id)
  },
  { immediate: true }
)
</script>

<template>
  <div class="motion-panel">
    <!-- 左侧边栏：控制器列表 -->
    <MotionSidebar
      :profiles="motion.profiles"
      :status-list="motion.statusList"
      :selected-id="selectedId"
      @select="selectController"
      @open-config="showConfig = true"
    />

    <!-- 主内容区 -->
    <section class="motion-panel__main" data-test="motion-panel-surface">
      <!-- 横幅（急停 / 错误） -->
      <MotionBanner
        :emergency-stopped="emergencyStopped"
        :last-error="lastError"
        @reset-emergency-stop="handleResetEmergencyStop"
        @clear-error="handleClearError"
      />

      <!-- 顶部控制栏 -->
      <MotionToolbar
        :profiles="motion.profiles"
        :selected-id="selectedId"
        :connected="controllerConnected"
        @connect="handleConnect"
        @disconnect="handleDisconnect"
        @stop-all="handleStopAll"
        @emergency-stop="handleEmergencyStop"
      />

      <!-- 未选择控制器 -->
      <div v-if="!selectedId" class="motion-panel__empty">
        <div class="motion-panel__empty-icon" aria-hidden="true">{{ i18n.t.selectController }}</div>
        <p class="motion-panel__empty-hint">{{ i18n.t.selectControllerHint }}</p>
      </div>

      <!-- 轴内容区 -->
      <div v-else class="motion-panel__axis-scroll custom-scrollbar">
        <!-- 无轴配置 -->
        <div v-if="axes.length === 0" class="motion-panel__empty motion-panel__empty--centered">
          <svg class="motion-panel__empty-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
            <path d="M12 2v4"/>
            <path d="m16.2 7.8 2.9-2.9"/>
            <path d="M18 12h4"/>
            <path d="m16.2 16.2 2.9 2.9"/>
            <path d="M12 18v4"/>
            <path d="m4.9 19.1 2.9-2.9"/>
            <path d="M2 12h4"/>
            <path d="m4.9 4.9 2.9 2.9"/>
          </svg>
          <p class="motion-panel__empty-title">{{ i18n.t.noAxesConfigured }}</p>
          <p class="motion-panel__empty-desc">{{ i18n.t.checkProfileAxes }}</p>
          <button class="motion-panel__empty-btn" @click="showConfig = true">
            {{ i18n.t.openConfig }}
          </button>
        </div>

        <!-- 轴卡片网格 -->
        <div v-else-if="currentAxisLocalState && currentZeroOffsetMap" class="motion-panel__grid">
          <AxisCard
            v-for="axis in axes"
            :key="axis.name"
            :controller-id="selectedId"
            :axis="axis"
            :profile="currentProfile"
            :connected="controllerConnected"
            :zero-offset="currentZeroOffsetMap[axis.name] ?? 0"
            :target-position="currentAxisLocalState[axis.name].targetPosition"
            :step="currentAxisLocalState[axis.name].step"
            :history="getAxisHistory(selectedId, axis.name)"
            @update:target-position="(v) => (currentAxisLocalState![axis.name].targetPosition = v)"
            @update:step="(v) => (currentAxisLocalState![axis.name].step = v)"
            @move="handleMove"
            @jog="handleJog"
            @stop="handleStopAxis"
            @set-zero="handleSetZero"
          />
        </div>
      </div>
    </section>

    <!-- 配置弹窗 -->
    <MotionControllerConfig
      :open="showConfig"
      :current-id="selectedId"
      @saved="handleProfileSaved"
      @close="showConfig = false"
    />
  </div>
</template>

<style scoped>
/* ============================================================
   主容器
   ============================================================ */
.motion-panel {
  display: flex;
  align-items: stretch;
  width: 100%;
  height: 100%;
  min-height: 0;
  background: var(--bg-canvas);
}

/* ============================================================
   主内容区
   ============================================================ */
.motion-panel__main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  background: var(--bg-canvas);
}

/* ============================================================
   空状态
   ============================================================ */
.motion-panel__empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  flex: 1;
  padding: 48px 24px;
  color: var(--text-muted);
  text-align: center;
  min-height: 200px;
}

.motion-panel__empty--centered {
  min-height: 300px;
}

.motion-panel__empty-icon {
  font-size: 3rem;
  font-weight: 700;
  font-style: italic;
  opacity: 0.15;
  margin-bottom: 16px;
}

.motion-panel__empty-svg {
  width: 48px;
  height: 48px;
  opacity: 0.2;
  margin-bottom: 12px;
}

.motion-panel__empty-hint {
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.12em;
  text-transform: uppercase;
}

.motion-panel__empty-title {
  font-size: 14px;
  font-weight: 600;
  margin-bottom: 4px;
}

.motion-panel__empty-desc {
  font-size: 12px;
  opacity: 0.7;
  margin-bottom: 16px;
}

.motion-panel__empty-btn {
  padding: 8px 16px;
  border-radius: 4px;
  font-size: 12px;
  font-weight: 700;
  border: 1px solid var(--accent-primary);
  color: var(--accent-primary);
  background: transparent;
  cursor: pointer;
  transition: all 0.15s ease;
}

.motion-panel__empty-btn:hover {
  background: var(--accent-primary);
  color: var(--color-on-accent);
}

.motion-panel__empty-btn:active {
  transform: scale(0.97);
}

/* ============================================================
   轴内容滚动区
   ============================================================ */
.motion-panel__axis-scroll {
  flex: 1;
  min-height: 0;
  overflow: auto;
  padding: 16px;
  display: flex;
  flex-direction: column;
  /* 内容较少时在滚动区垂直居中，消除下方孤立空白；
     内容超出时由 overflow 接管，仍然从顶部开始滚动 */
  justify-content: center;
}

/* ============================================================
   轴卡片网格
   ============================================================ */
.motion-panel__grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 14px;
  /* 不强制拉伸网格容器，让其在父容器中由 justify-content:center 自然居中 */
  flex: 0 0 auto;
  min-height: 0;
  max-height: 100%;
  align-content: start;
}

@media (max-width: 1280px) {
  .motion-panel__grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 720px) {
  .motion-panel__grid {
    grid-template-columns: minmax(0, 1fr);
  }
}

/* ============================================================
   prefers-reduced-motion：禁用 empty-btn 按压位移
   ============================================================ */
@media (prefers-reduced-motion: reduce) {
  .motion-panel__empty-btn:active {
    transform: none;
  }
  .motion-panel__empty-btn {
    transition: none;
  }
}
</style>
