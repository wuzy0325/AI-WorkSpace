/**
 * ============================================================================
 * 五孔探针移位测试主画面（FiveHoleTraversalMain）
 * ============================================================================
 *
 * 【功能定位】
 * 用于五孔探针的正式风洞实验，按预设轨迹自动遍历多个点位，
 * 使用已校准的 PRB 系数进行实时插值计算。
 *
 * 【使用场景】
 * - 风洞流场测量实验
 * - 使用已校准探针进行正式测试
 * - 实时获取流场参数（迎角、侧滑角、马赫数等）
 *
 * 【关键特征】
 * - 需要预先加载 PRB 校准文件
 * - 支持运行/暂停/停止控制
 * - 显示点位预览图和当前进度
 * - 实时插值计算并显示结果（α、β、Mach、P0、Ps）
 * - 显示原始压力通道数据（P1-P5、Patm、Tatm）
 * - 支持从上次中断处恢复测试
 *
 * 【前置条件】
 * 必须先完成探针标定！校准系统位于：
 * @/components/calibration/five-hole/FiveHoleMain.vue
 *
 * @module FiveHoleTraversalMain
 * @see TraversalSettings.vue - 测试配置
 * @see FiveHoleMain.vue - 探针标定系统
 *
 * 【组件结构 Phase A 重构（2026-06）】
 * 本文件仅负责状态/监听/事件编排。UI 切到子组件：
 * - shell/TraversalTopBar.vue        — 顶栏（标题+状态+进度+操作按钮）
 * - shell/TraversalCheckpointBanner.vue — 断点恢复横幅
 * - shell/TraversalErrorBanner.vue   — 错误横幅
 * - panels/TraversalLiveMonitor.vue  — 左侧实时数据面板
 * - panels/TraversalWorkspaceArea.vue — 右侧 Tab 工作区
 * - dialogs/TraversalStartConfirm.vue — 启动确认对话框
 *
 * Phase B（token 化配色）和 Phase C（布局对齐）将在子组件内单独推进。
 * ============================================================================
 */
<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useDeviceStore } from '@stores/deviceStore'
import { useFeedbackStore } from '@stores/feedbackStore'
import { useMotionStore } from '@stores/motionStore'
import { useTraversalStore } from '@stores/traversalStore'
import { useI18nStore } from '@stores/i18nStore'
import type {
  TraversalCheckpoint,
  PreconditionCheckResult
} from '@shared/types/traversal'
import { useTraversalSimulation } from '@composables/useTraversalSimulation'
import { useTraversalRealtimeData } from '@composables/useTraversalRealtimeData'
import { useHardwareConnectionStatus } from '@composables/useHardwareConnectionStatus'
import { useTraversalStatusDisplay } from '@composables/useTraversalStatusDisplay'
import TraversalTopBar from './shell/TraversalTopBar.vue'
import TraversalCheckpointBanner from './shell/TraversalCheckpointBanner.vue'
import TraversalErrorBanner from './shell/TraversalErrorBanner.vue'
import TraversalLiveMonitor from './panels/TraversalLiveMonitor.vue'
import TraversalWorkspaceArea, { type WorkspaceTab } from './panels/TraversalWorkspaceArea.vue'
import TraversalStartConfirm from './dialogs/TraversalStartConfirm.vue'

const props = withDefaults(
  defineProps<{
    recovering?: boolean
  }>(),
  {
    recovering: false
  }
)

const emit = defineEmits<{
  openSettings: []
  back: []
}>()

const traversalStore = useTraversalStore()
const deviceStore = useDeviceStore()
const motionStore = useMotionStore()
const feedbackStore = useFeedbackStore()
const hasConfig = computed(() => traversalStore.config !== null)
const currentConfig = computed(() => traversalStore.config)

// 模拟模式 composable
const {
  runSimulation,
  cancelSimulation,
  isSimulating
} = useTraversalSimulation()

// 实时数据 composable：统一订阅 DAQ 快照并计算压力/插值输入/展示项
const {
  liveInterpolationInput,
  pressureItems,
  subscribeSnapshot,
  hasRealtimeResult
} = useTraversalRealtimeData(currentConfig)

const i18n = useI18nStore()
const t = computed(() => i18n.t)

const activeWorkspaceTab = ref<WorkspaceTab>('preview')
// 启动请求防重入标志（与 store.isStarting 配合，避免重复触发）
const isStartRequestPending = ref(false)

// 前置条件检查与启动确认对话框状态
const showStartConfirm = ref(false)
const isCheckingPreconditions = ref(false)
const preconditionResult = ref<PreconditionCheckResult | null>(null)
/** 触发确认对话框的按钮引用，用于关闭后恢复焦点 */
const topBarRef = ref<InstanceType<typeof TraversalTopBar> | null>(null)

// 本地维护的断点信息（用于 UI 横幅显示，与 store.checkpoint 同步）
const checkpoint = ref<TraversalCheckpoint | null>(null)
let unsubscribeDaqSnapshot: (() => void) | null = null
let unsubscribeDeviceStatus: (() => void) | null = null
let unsubscribeMotionStatus: (() => void) | null = null

const hasCheckpoint = computed(() => checkpoint.value !== null)
// 模拟模式：store 标记为模拟 + composable 正在运行
const isSimulationMode = computed(() => traversalStore.isSimulation && isSimulating.value)
// 是否显示真实控制按钮（模拟中和恢复中均不显示）
const showRealControls = computed(() => !isSimulationMode.value && !isSimulating.value && !props.recovering)

const workspaceTabs = computed(() => [
  { value: 'preview' as WorkspaceTab, label: t.value.pointsPreview },
  { value: 'visualization' as WorkspaceTab, label: t.value.flowVisualization },
  { value: 'reference' as WorkspaceTab, label: t.value.probeReference }
])

// 焦点陷阱由 UiDialog (NModal) 内置处理，不再需要手动 useFocusTrap

onMounted(async () => {
  void deviceStore.refreshInstances()
  unsubscribeDeviceStatus = deviceStore.attachStatusListener()

  void motionStore.refreshStatus()
  unsubscribeMotionStatus = motionStore.attachStatusListener()

  unsubscribeDaqSnapshot = subscribeSnapshot()

  // 恢复已保存的配置，确保指示灯和 UI 能正确反映设备/机构状态
  await traversalStore.loadConfig()

  // 恢复正在进行的移位测试状态
  await traversalStore.refreshStatus()

  // 检测是否有未完成的测试断点
  await checkForCheckpoint()
})

/** 从后端加载断点信息，用于显示恢复横幅 */
async function checkForCheckpoint(): Promise<void> {
  try {
    const savedCheckpoint = await traversalStore.loadCheckpoint()
    if (savedCheckpoint) {
      checkpoint.value = savedCheckpoint
    }
  } catch (err) {
    feedbackStore.pushToast(
      t.value.failedCheckCheckpoint + '：' + (err instanceof Error ? err.message : String(err)),
      'error'
    )
  }
}

/** 从断点恢复测试 */
async function resumeFromCheckpoint(): Promise<void> {
  if (!checkpoint.value) return

  try {
    await traversalStore.resumeFromCheckpoint(checkpoint.value)
    checkpoint.value = null
  } catch (err) {
    feedbackStore.pushToast(
      t.value.failedResume + '：' + (err instanceof Error ? err.message : String(err)),
      'error'
    )
  }
}

/** 放弃断点，删除断点文件 */
async function discardCheckpoint(): Promise<void> {
  const currentCheckpoint = checkpoint.value
  checkpoint.value = null
  try {
    await traversalStore.clearCheckpoint()
  } catch (err) {
    checkpoint.value = currentCheckpoint
    feedbackStore.pushToast(
      t.value.failedDiscardCheckpoint + '：' + (err instanceof Error ? err.message : String(err)),
      'error'
    )
  }
}

onBeforeUnmount(() => {
  // 清理模拟模式
  cancelSimulation()
  if (traversalStore.isSimulation) {
    traversalStore.isSimulation = false
  }
  if (unsubscribeDaqSnapshot) {
    unsubscribeDaqSnapshot()
    unsubscribeDaqSnapshot = null
  }
  if (unsubscribeDeviceStatus) {
    unsubscribeDeviceStatus()
    unsubscribeDeviceStatus = null
  }
  if (unsubscribeMotionStatus) {
    unsubscribeMotionStatus()
    unsubscribeMotionStatus = null
  }
  traversalStore.reset()
})

function openSettings(): void {
  emit('openSettings')
}

/**
 * 启动测试入口：打开确认对话框并执行前置条件检查。
 * 与 Cursor DAQ 行为一致：先检查再确认，避免误触。
 */
async function startTest(): Promise<void> {
  if (isStartRequestPending.value || traversalStore.isStarting) {
    return
  }

  if (!currentConfig.value) {
    feedbackStore.pushToast(t.value.pleaseConfigureFirst, 'warning')
    return
  }

  // 打开确认对话框，并异步执行前置条件检查
  isCheckingPreconditions.value = true
  preconditionResult.value = null
  showStartConfirm.value = true

  try {
    const result = await traversalStore.checkPreconditions(currentConfig.value)
    preconditionResult.value = result
  } catch (err) {
    preconditionResult.value = { allPassed: false, checks: [] }
    feedbackStore.pushToast(
      t.value.failedCheckPreconditions + '：' + (err instanceof Error ? err.message : String(err)),
      'error'
    )
  } finally {
    isCheckingPreconditions.value = false
  }
}

/** 确认启动：用户在确认对话框中点击"确认启动"后执行 */
async function confirmStartTest(): Promise<void> {
  // 异步期间状态可能变化，需再次校验 config 存在
  if (!currentConfig.value) {
    showStartConfirm.value = false
    return
  }
  showStartConfirm.value = false
  isStartRequestPending.value = true
  try {
    await traversalStore.startTest(currentConfig.value)
  } catch (err) {
    feedbackStore.pushToast(
      t.value.failedStartTraversal + '：' + (err instanceof Error ? err.message : String(err)),
      'error'
    )
  } finally {
    isStartRequestPending.value = false
  }
}

/** 取消启动确认对话框 */
/** 取消启动确认对话框 */
function cancelStartConfirm(): void {
  showStartConfirm.value = false
  preconditionResult.value = null
  // 关闭后将焦点返回触发按钮；若按钮不在 DOM 中（运行态），退回工具栏根元素
  nextTick(() => {
    if (topBarRef.value?.focusStart()) return
    const fallback = document.querySelector('[data-test="traversal-top-toolbar"]') as HTMLElement | null
    fallback?.focus()
  })
}

// 对话框打开时的初始焦点由 NModal 内部处理，无需手动 watch

async function pauseTest(): Promise<void> {
  try {
    await traversalStore.pause()
  } catch (err) {
    feedbackStore.pushToast(
      t.value.failedPause + '：' + (err instanceof Error ? err.message : String(err)),
      'error'
    )
  }
}

async function resumeTest(): Promise<void> {
  try {
    await traversalStore.resume()
  } catch (err) {
    feedbackStore.pushToast(
      t.value.failedResume + '：' + (err instanceof Error ? err.message : String(err)),
      'error'
    )
  }
}

async function stopTest(): Promise<void> {
  try {
    await traversalStore.stop()
  } catch (err) {
    feedbackStore.pushToast(
      t.value.failedStop + '：' + (err instanceof Error ? err.message : String(err)),
      'error'
    )
  }
}

// 状态文本与点样式：抽到 useTraversalStatusDisplay
const { statusText, statusDotClass } = useTraversalStatusDisplay(isStartRequestPending)

const workspaceTabMeta = computed(() => {
  const tval = t.value
  const maps: Record<WorkspaceTab, { title: string; subtitle: string }> = {
    preview:       { title: tval.pointsPreview,     subtitle: tval.layoutTopology },
    visualization: { title: tval.flowVisualization, subtitle: tval.realtimeFlow },
    reference:     { title: tval.probeReference,    subtitle: tval.probeReferenceHint },
  }
  return maps[activeWorkspaceTab.value]
})

const progressSummary = computed(() => `${traversalStore.status?.completedPoints || 0} / ${traversalStore.status?.totalPoints || 0}`)

// 进度百分比（0-100）
const progressPercent = computed(() => {
  const total = traversalStore.status?.totalPoints ?? 0
  const completed = traversalStore.status?.completedPoints ?? 0
  return total > 0 ? Math.round((completed / total) * 100) : 0
})

// 预估剩余时间格式化
const estimatedRemainingText = computed(() => {
  const ms = traversalStore.status?.estimatedRemaining
  if (typeof ms !== 'number' || ms <= 0) return '--'
  const seconds = Math.ceil(ms / 1000)
  if (seconds < 60) return `${seconds}s`
  const minutes = Math.floor(seconds / 60)
  const remainSeconds = seconds % 60
  if (minutes < 60) return `${minutes}m ${remainSeconds}s`
  const hours = Math.floor(minutes / 60)
  const remainMinutes = minutes % 60
  return `${hours}h ${remainMinutes}m`
})

const currentPointSummary = computed(() => ({
  alpha: traversalStore.status?.currentPoint?.alpha?.toFixed(2) || '--',
  beta: traversalStore.status?.currentPoint?.beta?.toFixed(2) || '--'
}))

// 顶栏进度摘要可见性（运行/暂停/已完成时显示）
const showProgress = computed(() =>
  traversalStore.isRunning ||
  traversalStore.isPaused ||
  traversalStore.status?.status === 'completed'
)

// PRB 标签：保留计算逻辑（顶栏未消费但下游可能复用）。
// TODO Phase B/C：核实是否仍需要，未消费则删除。
// 轴位置 / 运动器连接 / 采集设备连接：抽到 useHardwareConnectionStatus，
// 模板需要的颜色 / label 等显示态由 composable 统一计算。
const {
  axisPositions,
  positionerConnection,
  acquisitionConnection
} = useHardwareConnectionStatus(currentConfig)

// 实时插值节流：交由 store 统一管理定时器，避免组件内重复实现
// 监听插值输入与插值器加载状态变化，触发 store 的节流插值
watch(
  [
    liveInterpolationInput,
    () => traversalStore.hasLoadedInterpolator,
  ],
  ([input]) => {
    traversalStore.syncRealtimeInterpolation(input, currentConfig.value)
  }
)

watch(
  () => traversalStore.completeEvent,
  (event) => {
    if (!event) return

    if (event.success) {
      const duration = event.duration ? (event.duration / 1000).toFixed(1) : '--'
      feedbackStore.pushToast(
        `${t.value.testCompleted}\n${t.value.filePath}: ${event.filePath}\n${t.value.duration}: ${duration}s\n${t.value.totalPoints}: ${event.totalPoints}`,
        'success',
        8000
      )
    } else {
      feedbackStore.pushToast(
        `${t.value.testFailed}: ${event.error || t.value.unknownError}\n${t.value.filePath}: ${event.filePath || '--'}`,
        'error',
        8000
      )
    }
  },
  { deep: true }
)

// 模拟完成自动切换到 Visualization tab，并提示模拟结果
watch(
  () => traversalStore.status?.status,
  (newStatus, oldStatus) => {
    if (
      traversalStore.isSimulation &&
      oldStatus === 'running' &&
      (newStatus === 'completed' || newStatus === 'stopped')
    ) {
      activeWorkspaceTab.value = 'visualization'
      if (newStatus === 'completed') {
        feedbackStore.pushToast(
          t.value.travSimComplete.replace('{count}', String(traversalStore.status?.completedPoints)),
          'success',
          5000
        )
      }
    }
  }
)
</script>

<template>
  <div
    class="flex h-full flex-col"
    :style="{ background: 'var(--bg-canvas)', color: 'var(--text-primary)' }"
  >
    <!-- 顶栏 -->
    <TraversalTopBar
      ref="topBarRef"
      :title="t.fiveHoleTraversalTest"
      :status-text="statusText"
      :status-dot-class="statusDotClass"
      :automated-run-label="t.automatedRun"
      :show-progress="showProgress"
      :progress-summary="progressSummary"
      :progress-percent="progressPercent"
      :estimated-remaining-text="estimatedRemainingText"
      :validation-warnings="traversalStore.status?.validationWarnings"
      :has-config="hasConfig"
      :is-start-request-pending="isStartRequestPending"
      :is-starting="traversalStore.isStarting"
      :is-simulation-mode="isSimulationMode"
      :show-real-controls="showRealControls"
      :can-start="traversalStore.canStart"
      :can-pause="traversalStore.canPause"
      :can-resume="traversalStore.canResume"
      :simulation-progress="progressSummary"
      :labels="{
        configBtn: t.configBtn,
        startRun: t.startRun,
        travPause: t.travPause,
        travStop: t.travStop,
        travResume: t.travResume,
        travSimRun: t.travSimRun,
        travSimProgressTemplate: t.travSimProgress,
      }"
      @open-settings="openSettings"
      @start="startTest"
      @pause="pauseTest"
      @resume="resumeTest"
      @stop="stopTest"
      @run-simulation="runSimulation()"
      @cancel-simulation="cancelSimulation"
    />

    <!-- 插值器恢复状态横幅：当后端启动恢复失败或运行期校验失败时展示，
         消息由 traversalStore.interpolatorRestoreMessage 提供（含后端 prbCheck.message）。
         交互：点击右侧"知道了"按钮可临时清除提示；下次刷新或重新校验会再次写入。 -->
    <div
      v-if="traversalStore.interpolatorRestoreMessage"
      role="alert"
      class="trav-interp-banner"
    >
      <span class="trav-interp-banner__icon" aria-hidden="true">⚠</span>
      <span class="trav-interp-banner__text">{{ traversalStore.interpolatorRestoreMessage }}</span>
      <button
        type="button"
        class="trav-interp-banner__close"
        @click="traversalStore.interpolatorRestoreMessage = null"
      >知道了</button>
    </div>

    <!-- 断点恢复横幅 -->
    <TraversalCheckpointBanner
      v-if="hasCheckpoint && !traversalStore.isRunning && checkpoint"
      :checkpoint="checkpoint"
      :labels="{
        detected: t.travCheckDetected,
        completed: t.travCheckCompleted,
        config: t.travCheckConfig,
        continueTest: t.travContinueTest,
        abandon: t.travAbandon,
      }"
      @resume="resumeFromCheckpoint"
      @discard="discardCheckpoint"
    />

    <!-- 恢复加载状态 -->
    <div v-if="recovering" class="flex flex-1 items-center justify-center p-6">
      <div class="flex flex-col items-center justify-center gap-4 text-center">
        <div
          class="h-8 w-8 animate-spin rounded-full border-4 border-t-transparent"
          :style="{ borderColor: 'var(--accent-info)', borderTopColor: 'transparent' }"
        ></div>
        <div class="text-sm font-black uppercase tracking-widest text-[var(--text-muted)]">{{ t.loadingWorkspace }}</div>
      </div>
    </div>

    <!-- Main Workspace -->
    <div v-else class="flex-1 overflow-hidden p-4">
      <div class="grid h-full gap-4 lg:grid-cols-[280px_1fr] grid-cols-1 auto-rows-auto lg:auto-rows-fr">
        <TraversalLiveMonitor
          :has-config="hasConfig"
          :current-point-summary="currentPointSummary"
          :axis-positions="axisPositions"
          :acquisition-connection="acquisitionConnection"
          :positioner-connection="positionerConnection"
          :pressure-items="pressureItems"
          :realtime-result="traversalStore.realtimeResult"
          :labels="{
            monitor: t.travMonitor,
            currentPoint: t.currentPoint,
            currentPointX: t.currentPointX,
            currentPointY: t.currentPointY,
            positioner: t.positioner,
            realtimeCalculation: t.realtimeCalculation,
            realtimePressureData: t.realtimePressureData,
            alpha: t.alpha,
            beta: t.beta,
            mach: t.mach,
            velocity: t.velocity,
          }"
        />

        <TraversalWorkspaceArea
          v-model:active-tab="activeWorkspaceTab"
          :tabs="workspaceTabs"
          :tab-meta="workspaceTabMeta"
          :current-config="currentConfig"
          :current-point="traversalStore.status?.currentPoint"
          :completed-points="traversalStore.status?.completedPoints"
          :current-point-phase="traversalStore.status?.currentPointPhase"
          :labels="{
            moving: t.moving,
            stabilizing: t.stabilizing,
            acquiring: t.acquiring,
            completed: t.completed,
            untested: t.untested,
            noLayoutConfigured: t.noLayoutConfigured,
            pleaseConfigureLayout: t.pleaseConfigureLayout,
            configureLayout: t.configureLayout,
          }"
          @open-settings="openSettings"
        />
      </div>
    </div>

    <!-- 启动确认对话框 (UiDialog primitive handles focus trap) -->
    <TraversalStartConfirm
      :show="showStartConfirm"
      :current-config="currentConfig"
      :is-checking-preconditions="isCheckingPreconditions"
      :precondition-result="preconditionResult"
      :labels="{
        title: t.confirmStartTitle,
        message: t.confirmStartMessage,
        checking: t.checkingPreconditions,
        points: t.confirmStartPoints,
        output: t.confirmStartOutput,
        dismiss: t.dismiss,
        start: t.confirmStartStart,
      }"
      @confirm="confirmStartTest"
      @cancel="cancelStartConfirm"
    />

    <!-- 错误横幅 -->
    <TraversalErrorBanner
      :error="traversalStore.error"
      :dismiss-label="t.dismiss"
      @dismiss="traversalStore.clearError"
    />
  </div>
</template>

<style scoped>
/* 插值器恢复横幅：放在 TopBar 下方，使用工作区设计 token 保证主题一致 */
.trav-interp-banner {
  display: flex;
  align-items: center;
  gap: var(--space-2, 0.5rem);
  padding: var(--space-2, 0.5rem) var(--space-4, 1rem);
  background: color-mix(in srgb, var(--accent-warning, #d97706) 12%, transparent);
  border-bottom: 1px solid color-mix(in srgb, var(--accent-warning, #d97706) 35%, transparent);
  color: var(--accent-warning, #d97706);
  font-size: var(--font-size-xs, 12px);
  font-weight: 600;
  line-height: 1.4;
}

.trav-interp-banner__icon {
  flex-shrink: 0;
  font-size: 14px;
}

.trav-interp-banner__text {
  flex: 1 1 auto;
  min-width: 0;
  word-break: break-word;
}

.trav-interp-banner__close {
  flex-shrink: 0;
  margin-left: auto;
  padding: 2px 10px;
  border-radius: var(--radius-sm, 4px);
  border: 1px solid currentColor;
  background: transparent;
  color: inherit;
  font-size: var(--font-size-xs, 12px);
  font-weight: 700;
  cursor: pointer;
  transition: background-color 0.15s ease;
}

.trav-interp-banner__close:hover {
  background: color-mix(in srgb, currentColor 12%, transparent);
}
</style>
