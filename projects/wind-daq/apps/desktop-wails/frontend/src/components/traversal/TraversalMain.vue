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
 * ============================================================================
 */
<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import UiButton from '@components/ui/UiButton.vue'
import { useDeviceStore } from '@stores/deviceStore'
import { useFeedbackStore } from '@stores/feedbackStore'
import { useMotionStore } from '@stores/motionStore'
import { useTraversalStore } from '@stores/traversalStore'
import { useI18nStore } from '@stores/i18nStore'
import type {
  TraversalTestConfig,
  TraversalCheckpoint,
  PreconditionCheckResult
} from '@shared/types/traversal'
import { getTraversalLayoutPointCount } from '@shared/types/traversal'
import PointsPreview from './PointsPreview.vue'
import ProbeReferenceCard from './ProbeReferenceCard.vue'
import TraversalVisualization from './visualization/TraversalVisualization.vue'
import { AlertTriangle, Activity, ClipboardList, Pause, Play, Settings, Square, FlaskConical, CheckCircle, XCircle, Clock } from '@lucide/vue'
import IconTraversal from '@components/icons/IconTraversal.vue'
import { useTraversalSimulation } from '@composables/useTraversalSimulation'
import { useTraversalRealtimeData } from '@composables/useTraversalRealtimeData'

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
  livePressures,
  liveInterpolationInput,
  pressureItems,
  subscribeSnapshot,
  hasRealtimeResult
} = useTraversalRealtimeData(currentConfig)

const i18n = useI18nStore()
const t = computed(() => i18n.t)

type WorkspaceTab = 'preview' | 'visualization' | 'reference'

const activeWorkspaceTab = ref<WorkspaceTab>('preview')
// 启动请求防重入标志（与 store.isStarting 配合，避免重复触发）
const isStartRequestPending = ref(false)

// 前置条件检查与启动确认对话框状态
const showStartConfirm = ref(false)
const isCheckingPreconditions = ref(false)
const preconditionResult = ref<PreconditionCheckResult | null>(null)
const confirmDialogRef = ref<HTMLElement | null>(null)
/** 触发确认对话框的按钮引用，用于关闭后恢复焦点 */
const startButtonRef = ref<HTMLButtonElement | null>(null)

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

const workspaceTabs = computed<Array<{ value: WorkspaceTab; label: string }>>(() => [
  { value: 'preview', label: t.value.pointsPreview },
  { value: 'visualization', label: t.value.flowVisualization },
  { value: 'reference', label: t.value.probeReference }
])

/** 焦点陷阱：将 Tab 键限制在对话框内循环 */
function trapFocus(event: KeyboardEvent): void {
  const container = confirmDialogRef.value
  if (!container) return

  const focusableSelectors = 'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])'
  const focusableElements = Array.from(container.querySelectorAll<HTMLElement>(focusableSelectors))
  if (focusableElements.length === 0) return

  const firstFocusable = focusableElements[0]
  const lastFocusable = focusableElements[focusableElements.length - 1]

  if (event.shiftKey) {
    // Shift+Tab：如果当前在第一个元素，跳到最后一个
    if (document.activeElement === firstFocusable) {
      event.preventDefault()
      lastFocusable.focus()
    }
  } else {
    // Tab：如果当前在最后一个元素，跳到第一个
    if (document.activeElement === lastFocusable) {
      event.preventDefault()
      firstFocusable.focus()
    }
  }
}

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
function cancelStartConfirm(): void {
  showStartConfirm.value = false
  preconditionResult.value = null
  // 关闭后将焦点返回触发按钮；若按钮不在DOM中（运行态），退回到对话框本身所在的聚焦目标
  nextTick(() => {
    const target = startButtonRef.value ?? document.querySelector('[data-test="traversal-top-toolbar"]') as HTMLElement | null
    target?.focus()
  })
}

// 对话框打开时自动聚焦第一个按钮
watch(showStartConfirm, (isOpen) => {
  if (isOpen) {
    nextTick(() => {
      const container = confirmDialogRef.value
      if (container) {
        const firstButton = container.querySelector<HTMLElement>('button')
        firstButton?.focus()
      }
    })
  }
})

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

const STATUS_CONFIG: Record<string, { dotClass: string }> = {
  running:     { dotClass: 'bg-emerald-500 shadow-[0_0_6px_#10b981]' },
  moving:      { dotClass: 'bg-emerald-500 shadow-[0_0_6px_#10b981]' },
  stabilizing: { dotClass: 'bg-emerald-500 shadow-[0_0_6px_#10b981]' },
  acquiring:   { dotClass: 'bg-emerald-500 shadow-[0_0_6px_#10b981]' },
  saving:      { dotClass: 'bg-emerald-500 shadow-[0_0_6px_#10b981]' },
  paused:      { dotClass: 'bg-amber-500 animate-pulse' },
  completed:   { dotClass: 'bg-blue-500 shadow-[0_0_6px_#3b82f6]' },
  error:       { dotClass: 'bg-rose-500 shadow-[0_0_6px_#f43f5e]' },
  stopped:     { dotClass: 'bg-amber-500' },
  unknown:     { dotClass: 'bg-rose-500 shadow-[0_0_6px_#f43f5e]' },
  idle:        { dotClass: 'bg-slate-400' },
}

// 状态文本：优先显示子状态（moving/stabilizing/acquiring/saving），其次显示主状态
const statusText = computed(() => {
  const phase = traversalStore.status?.currentPointPhase
  // 启动中或运行中：根据子阶段细化文案
  if (isStartRequestPending.value || traversalStore.isStarting) {
    return t.value.statusStarting
  }
  if (traversalStore.canPause) {
    switch (phase) {
      case 'moving':
        return t.value.statusMoving
      case 'stabilizing':
        return t.value.statusStabilizing
      case 'acquiring':
        return t.value.statusAcquiring
      case 'saving':
        return t.value.statusSaving
      default:
        return t.value.statusRunning
    }
  }
  switch (traversalStore.statusType) {
    case 'paused':
      return t.value.statusPaused
    case 'completed':
      return t.value.statusDone
    case 'error':
      return t.value.statusError
    case 'stopped':
      return t.value.statusStopped
    case 'unknown':
      return t.value.statusUnknown
    default:
      return t.value.statusIdle
  }
})

// 状态点样式：根据当前状态类型选择颜色
const statusDotClass = computed(() => {
  const cfg = STATUS_CONFIG[traversalStore.statusType] ?? STATUS_CONFIG.idle
  return cfg.dotClass
})

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

// PRB 标签：多 PRB 优先显示数量，新算法显示 CSV 文件名，旧算法显示 PRB 文件名
const prbLabel = computed(() => {
  const cfg = traversalStore.config
  if (!cfg) return t.value.noPrbSelected
  if (cfg.useMultiPrb && cfg.multiPrb?.files.length) {
    return `${cfg.multiPrb.files.length} PRBs`
  }
  if (cfg.interpolationAlgorithm === 'new') {
    return cfg.calibrationCsvFile?.fileName || t.value.noPrbSelected
  }
  return cfg.prbFile?.fileName || t.value.noPrbSelected
})

// 轴位置数据：监听 motionStore 状态变化，更新轴位置显示
interface AxisPositionDatum { label: string; position: number | undefined; moving: boolean }
const axisPositions = ref<AxisPositionDatum[]>([])
watch(
  () => {
    const axes = currentConfig.value?.channels.motionAxes ?? []
    return axes.map((cfg) => {
      const status = motionStore.statusById(cfg.controllerId)
      const axisStatus = status?.axes.find((a) => a.name === cfg.axis)
      return `${cfg.axis}:${axisStatus?.position?.toFixed(3) ?? ''}:${axisStatus?.moving ?? false}`
    }).join('|')
  },
  () => {
    const axes = currentConfig.value?.channels.motionAxes ?? []
    axisPositions.value = axes.map((cfg) => {
      const status = motionStore.statusById(cfg.controllerId)
      const axisStatus = status?.axes.find((a) => a.name === cfg.axis)
      return { label: cfg.axis, position: axisStatus?.position, moving: axisStatus?.moving ?? false }
    })
  },
  { immediate: true }
)

type PositionerConnectionState = 'connected' | 'disconnected' | 'unconfigured'
type AcquisitionState = 'acquiring' | 'connected' | 'disconnected' | 'unconfigured'

function buildConnectionDisplay(state: PositionerConnectionState, unconfiguredLabel: string) {
  const dotClassMap: Record<PositionerConnectionState, string> = {
    connected: 'bg-emerald-500 shadow-[0_0_8px_#10b981]',
    disconnected: 'bg-rose-500 shadow-[0_0_8px_#f43f5e]',
    unconfigured: 'bg-slate-400'
  }

  const textClassMap: Record<PositionerConnectionState, string> = {
    connected: 'text-emerald-600 dark:text-emerald-400',
    disconnected: 'text-rose-600 dark:text-rose-400',
    unconfigured: 'text-slate-500 dark:text-slate-400'
  }

  const labelMap: Record<PositionerConnectionState, string> = {
    connected: t.value.connected,
    disconnected: t.value.disconnected,
    unconfigured: unconfiguredLabel
  }

  return {
    state,
    label: labelMap[state],
    dotClass: dotClassMap[state],
    textClass: textClassMap[state]
  }
}

// 采集设备状态显示：区分 acquiring / connected / disconnected / unconfigured
function buildAcquisitionDisplay(state: AcquisitionState, unconfiguredLabel: string) {
  const dotClassMap: Record<AcquisitionState, string> = {
    acquiring: 'bg-emerald-500 shadow-[0_0_8px_#10b981]',
    connected: 'bg-amber-400',
    disconnected: 'bg-rose-500 shadow-[0_0_8px_#f43f5e]',
    unconfigured: 'bg-slate-400'
  }

  const textClassMap: Record<AcquisitionState, string> = {
    acquiring: 'text-emerald-600 dark:text-emerald-400',
    connected: 'text-amber-600 dark:text-amber-400',
    disconnected: 'text-rose-600 dark:text-rose-400',
    unconfigured: 'text-slate-500 dark:text-slate-400'
  }

  const labelMap: Record<AcquisitionState, string> = {
    acquiring: t.value.acquiring,
    connected: t.value.connected,
    disconnected: t.value.disconnected,
    unconfigured: unconfiguredLabel
  }

  return {
    state,
    label: labelMap[state],
    dotClass: dotClassMap[state],
    textClass: textClassMap[state]
  }
}

const positionerConnection = computed(() => {
  const controllerIds = Array.from(
    new Set(
      (currentConfig.value?.channels.motionAxes ?? [])
        .map((axis) => axis.controllerId?.trim())
        .filter((controllerId): controllerId is string => Boolean(controllerId))
    )
  )

  let state: PositionerConnectionState = 'unconfigured'
  if (controllerIds.length > 0) {
    state = controllerIds.every((controllerId) => motionStore.statusById(controllerId)?.connected)
      ? 'connected'
      : 'disconnected'
  }

  return buildConnectionDisplay(state, t.value.unconfigured)
})

const acquisitionConnection = computed(() => {
  const deviceIds = Array.from(
    new Set(
      (currentConfig.value?.channels.probeChannels ?? [])
        .filter((channel) => channel.enabled)
        .map((channel) => channel.channel.deviceId?.trim())
        .filter((deviceId): deviceId is string => Boolean(deviceId))
    )
  )

  let state: AcquisitionState = 'unconfigured'
  if (deviceIds.length > 0) {
    const allConnected = deviceIds.every((deviceId) => deviceStore.statusFor(deviceId) === 'Connected')
    if (!allConnected) {
      state = 'disconnected'
    } else {
      // 全部连接且全部采集中 → acquiring；仅连接 → connected
      const allAcquiring = deviceIds.every((deviceId) => deviceStore.acquiringFor(deviceId))
      state = allAcquiring ? 'acquiring' : 'connected'
    }
  }

  return buildAcquisitionDisplay(state, t.value.unconfigured)
})



// 实时插值节流：交由 store 统一管理定时器，避免组件内重复实现
// 监听插值输入与数据集路径变化，触发 store 的节流插值
watch(
  [
    liveInterpolationInput,
    () => currentConfig.value?.prbFile?.filePath ?? null,
    () => currentConfig.value?.calibrationCsvFile?.filePath ?? null,
    () => currentConfig.value?.useMultiPrb ? currentConfig.value?.multiPrb?.files.map(f => f.filePath).join(',') : ''
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
  <div class="flex h-full flex-col bg-slate-50/50 dark:bg-transparent text-[color:var(--text-primary)]">
    <!-- Top Toolbar -->
    <div data-test="traversal-top-toolbar" class="flex shrink-0 items-center justify-between border-b border-slate-200 bg-white px-4 py-2.5 dark:border-slate-700 dark:bg-slate-900">
      <!-- 左侧：标题区 -->
      <div class="flex items-center gap-3">
        <div class="flex h-8 w-8 items-center justify-center rounded-lg bg-blue-500 text-white shadow-sm">
          <IconTraversal :size="16" />
        </div>
        <div>
          <h1 class="text-sm font-bold text-slate-900 dark:text-slate-100">{{ t.fiveHoleTraversalTest }}</h1>
          <div class="flex items-center gap-2 mt-0.5">
            <span class="flex h-1.5 w-1.5 rounded-full" :class="statusDotClass"></span>
            <p class="text-[11px] text-slate-400">{{ statusText }} / {{ t.automatedRun }}</p>
          </div>
        </div>
      </div>

      <!-- 右侧：控制区 -->
      <div class="flex items-center gap-2">
        <!-- 进度摘要：合并进度数字+百分比+预估时间为一行 -->
        <div v-if="traversalStore.isRunning || traversalStore.isPaused || traversalStore.status?.status === 'completed'" class="flex items-center gap-2 rounded-lg border border-slate-200 bg-slate-50 px-3 py-1.5 dark:border-slate-700 dark:bg-slate-800/50">
          <span class="font-mono text-xs font-semibold text-blue-500">{{ progressSummary }}</span>
          <span class="text-[11px] text-slate-400">({{ progressPercent }}%)</span>
          <div class="h-1.5 w-12 overflow-hidden rounded-full bg-slate-200 dark:bg-slate-700">
            <div class="h-full rounded-full bg-blue-500 transition-all duration-300" :style="{ width: `${progressPercent}%` }"></div>
          </div>
          <template v-if="estimatedRemainingText !== '--'">
            <div class="h-3 w-px bg-slate-200 dark:bg-slate-600"></div>
            <Clock class="h-3 w-3 text-slate-400" />
            <span class="font-mono text-[11px] text-slate-500 dark:text-slate-400">{{ estimatedRemainingText }}</span>
          </template>
        </div>

        <!-- 验证警告：仅显示图标+数量，点击展开 -->
        <div v-if="traversalStore.status?.validationWarnings?.length" class="flex items-center gap-1.5 rounded-md border border-amber-200 bg-amber-50 px-2 py-1 dark:border-amber-800/50 dark:bg-amber-900/20" :title="traversalStore.status.validationWarnings.join('\n')">
          <AlertTriangle class="h-3 w-3 text-amber-500" />
          <span class="text-[11px] text-amber-700 dark:text-amber-400">{{ traversalStore.status.validationWarnings.length }}</span>
        </div>

        <!-- 操作按钮组 -->
        <UiButton
          @click="openSettings"
          variant="secondary" size="sm"
        >
          <template #icon>
            <Settings class="h-3.5 w-3.5" />
          </template>
          {{ t.configBtn }}
        </UiButton>

        <div class="h-4 w-px bg-slate-200 dark:bg-slate-700"></div>

        <div class="flex items-center gap-1.5">
          <!-- 模拟模式控制 -->
          <template v-if="isSimulationMode">
            <div class="flex items-center gap-1.5 rounded-md border border-blue-200 bg-blue-50 px-2.5 py-1 dark:border-blue-800/50 dark:bg-blue-900/20">
              <FlaskConical class="h-3.5 w-3.5 text-blue-500" />
              <span class="text-[10px] font-semibold text-blue-600 dark:text-blue-400">{{ t.travSimProgress.replace('{progress}', progressSummary) }}</span>
            </div>
            <UiButton variant="danger" size="md" @click="cancelSimulation">
              <template #icon>
                <Square class="h-4 w-4 fill-current" />
              </template>
              {{ t.travStop }}
            </UiButton>
          </template>

          <!-- 实际控制 -->
          <template v-else-if="showRealControls">
            <UiButton
              v-if="traversalStore.canStart && !isStartRequestPending"
              ref="startButtonRef"
              variant="primary" size="md"
              :disabled="!hasConfig"
              @click="startTest"
            >
              <template #icon>
                <Play class="h-4 w-4 fill-current" />
              </template>
              {{ t.startRun }}
            </UiButton>
            <UiButton
              v-else-if="isStartRequestPending || traversalStore.isStarting"
              variant="primary" size="md"
              disabled
            >
              <template #icon>
                <Play class="h-4 w-4 fill-current" />
              </template>
              {{ t.startRun }}
            </UiButton>
            <template v-else-if="traversalStore.canPause">
              <UiButton variant="warning" size="md" @click="pauseTest">
                <template #icon>
                  <Pause class="h-4 w-4 fill-current" />
                </template>
                {{ t.travPause }}
              </UiButton>
              <UiButton variant="danger" size="md" @click="stopTest">
                <template #icon>
                  <Square class="h-4 w-4 fill-current" />
                </template>
                {{ t.travStop }}
              </UiButton>
            </template>
            <template v-else-if="traversalStore.canResume">
              <UiButton variant="primary" size="md" @click="resumeTest">
                <template #icon>
                  <Play class="h-4 w-4 fill-current" />
                </template>
                {{ t.travResume }}
              </UiButton>
              <UiButton variant="danger" size="md" @click="stopTest">
                <template #icon>
                  <Square class="h-4 w-4 fill-current" />
                </template>
                {{ t.travStop }}
              </UiButton>
            </template>

            <!-- 模拟运行按钮 -->
            <template v-if="traversalStore.canStart && !isStartRequestPending">
              <div class="h-4 w-px bg-slate-200 dark:bg-slate-700"></div>
              <UiButton
                quaternary size="md"
                @click="runSimulation()"
              >
                <template #icon>
                  <FlaskConical class="h-4 w-4" />
                </template>
                {{ t.travSimRun }}
              </UiButton>
            </template>
          </template>
        </div>
      </div>
    </div>

    <!-- Checkpoint Recovery Banner：检测到未完成的测试时显示 -->
    <div v-if="hasCheckpoint && !traversalStore.isRunning" class="flex items-center justify-between border-b border-amber-200 bg-amber-50 px-4 py-2 dark:border-amber-800/50 dark:bg-amber-900/20">
      <div class="flex items-center gap-2.5">
        <div class="flex h-7 w-7 items-center justify-center rounded-full bg-amber-100 text-amber-600 dark:bg-amber-900/40 dark:text-amber-400">
          <Activity class="h-3.5 w-3.5" />
        </div>
        <div>
          <div class="text-xs font-medium text-slate-800 dark:text-slate-100">{{ t.travCheckDetected }}</div>
          <div class="text-[10px] text-slate-500 dark:text-slate-400">
            {{ t.travCheckCompleted }} {{ checkpoint?.completedPoints }} / {{ checkpoint?.totalPoints }} ·
            {{ t.travCheckConfig }} {{ checkpoint?.config?.name || 'Unknown' }}
          </div>
        </div>
      </div>
      <div class="flex items-center gap-1.5">
        <UiButton variant="warning" size="sm" @click="resumeFromCheckpoint">
          <template #icon>
            <Play class="h-3 w-3 fill-current" />
          </template>
          {{ t.travContinueTest }}
        </UiButton>
        <UiButton quaternary size="sm" @click="discardCheckpoint">
          {{ t.travAbandon }}
        </UiButton>
      </div>
    </div>

    <!-- Loading State -->
    <div v-if="recovering" class="flex flex-1 items-center justify-center p-6">
      <div class="flex flex-col items-center justify-center gap-4 text-center">
        <div class="h-8 w-8 animate-spin rounded-full border-4 border-blue-500 border-t-transparent"></div>
        <div class="text-sm font-black uppercase tracking-widest text-slate-500">{{ t.loadingWorkspace }}</div>
      </div>
    </div>

    <!-- Main Workspace -->
    <div class="flex-1 overflow-hidden p-4">
      <div class="grid h-full gap-4 lg:grid-cols-[280px_1fr] grid-cols-1 auto-rows-auto lg:auto-rows-fr">
        <!-- Sidebar：合并为单一实时数据面板 -->
        <aside class="flex min-h-0 flex-col overflow-y-auto pr-1">
          <section data-test="traversal-sidebar-monitor" class="rounded-xl border border-slate-200 bg-white p-3 shadow-sm dark:border-slate-700 dark:bg-slate-900 flex-shrink-0">
            <!-- 面板头部 -->
            <div class="mb-2 flex items-center justify-between">
              <div class="flex items-center gap-1.5">
                <Activity class="h-4 w-4 text-blue-500" />
                <div>
                  <h2 class="text-sm font-semibold text-slate-800 dark:text-slate-100">{{ t.travMonitor }}</h2>
                </div>
              </div>
              <div class="flex shrink-0 items-center gap-1.5">
                <div data-test="traversal-acquisition-indicator" class="flex items-center gap-1">
                  <span class="h-2 w-2 rounded-full" :class="acquisitionConnection.dotClass"></span>
                  <span class="text-xs" :class="acquisitionConnection.textClass">
                    {{ acquisitionConnection.label }}
                  </span>
                </div>
                <div class="h-3 w-px bg-slate-200 dark:bg-slate-700"></div>
                <div data-test="traversal-positioner-indicator" class="flex items-center gap-1">
                  <span class="h-2 w-2 rounded-full" :class="positionerConnection.dotClass"></span>
                  <span class="text-xs" :class="positionerConnection.textClass">
                    {{ positionerConnection.label }}
                  </span>
                </div>
              </div>
            </div>

            <!-- 当前点位：关键数值使用更大字号 -->
            <div class="mb-2 rounded-lg border border-slate-100 bg-slate-50/80 p-2 dark:border-slate-700/50 dark:bg-slate-800/50">
              <div class="mb-1 flex items-center gap-1.5">
                <div class="h-1.5 w-1.5 rounded-full bg-blue-400"></div>
                <span class="text-xs font-medium uppercase tracking-wider text-slate-400">{{ t.currentPoint }}</span>
              </div>
              <div class="grid grid-cols-2 gap-3">
                <div class="flex flex-col">
                  <span class="text-xs text-slate-400">{{ t.alpha }}</span>
                  <span class="font-mono text-base font-bold text-slate-800 dark:text-slate-100">{{ currentPointSummary.alpha }}°</span>
                </div>
                <div class="flex flex-col">
                  <span class="text-xs text-slate-400">{{ t.beta }}</span>
                  <span class="font-mono text-base font-bold text-slate-800 dark:text-slate-100">{{ currentPointSummary.beta }}°</span>
                </div>
              </div>
            </div>

            <!-- 轴位置：提升字号 -->
            <div v-if="axisPositions.length && hasConfig" class="mb-2 rounded-lg border border-slate-100 bg-slate-50/80 p-2 dark:border-slate-700/50 dark:bg-slate-800/50">
              <div class="mb-1 flex items-center gap-1.5">
                <div class="h-1.5 w-1.5 rounded-full bg-blue-400"></div>
                <span class="text-xs font-medium uppercase tracking-wider text-slate-400">{{ t.positioner }}</span>
              </div>
              <div class="flex flex-wrap gap-x-4 gap-y-1 font-mono text-sm">
                <div v-for="axis in axisPositions" :key="axis.label" class="flex items-center gap-1">
                  <span class="text-xs text-slate-400">{{ axis.label }}:</span>
                  <span class="text-sm" :class="axis.moving ? 'text-emerald-600 dark:text-emerald-400 font-semibold' : 'text-slate-700 dark:text-slate-200'">
                    {{ axis.position !== undefined ? axis.position.toFixed(2) : '--' }}
                  </span>
                </div>
              </div>
            </div>

            <!-- 气动参数网格：提升标签和数值字号 -->
            <div class="mb-2 space-y-1.5">
              <div class="flex items-center gap-1.5">
                <div class="h-1.5 w-1.5 rounded-full bg-blue-400"></div>
                <span class="text-xs font-medium uppercase tracking-wider text-slate-400">{{ t.realtimeCalculation }}</span>
              </div>
              <div class="grid grid-cols-2 gap-1.5">
                <div class="flex flex-col rounded-md border border-slate-100 bg-slate-50/60 p-1.5 dark:border-slate-700/50 dark:bg-slate-800/60">
                  <span class="text-xs text-slate-400">{{ t.alpha }}</span>
                  <span class="font-mono text-sm font-bold text-blue-600">{{ traversalStore.realtimeResult?.alpha?.toFixed(2) ?? '--' }}°</span>
                </div>
                <div class="flex flex-col rounded-md border border-slate-100 bg-slate-50/60 p-1.5 dark:border-slate-700/50 dark:bg-slate-800/60">
                  <span class="text-xs text-slate-400">{{ t.beta }}</span>
                  <span class="font-mono text-sm font-bold text-slate-700 dark:text-slate-200">{{ traversalStore.realtimeResult?.beta?.toFixed(2) ?? '--' }}°</span>
                </div>
                <div class="flex flex-col rounded-md border border-slate-100 bg-slate-50/60 p-1.5 dark:border-slate-700/50 dark:bg-slate-800/60">
                  <span class="text-xs text-slate-400">{{ t.mach }}</span>
                  <span class="font-mono text-sm font-bold text-blue-600">{{ traversalStore.realtimeResult?.machNumber?.toFixed(3) ?? '--' }}</span>
                </div>
                <div class="flex flex-col rounded-md border border-slate-100 bg-slate-50/60 p-1.5 dark:border-slate-700/50 dark:bg-slate-800/60">
                  <span class="text-xs text-slate-400">{{ t.velocity }}</span>
                  <span class="font-mono text-sm font-bold text-slate-700 dark:text-slate-200">{{ traversalStore.realtimeResult?.velocity?.toFixed(1) ?? '--' }}</span>
                </div>
                <div class="flex flex-col rounded-md border border-slate-100 bg-slate-50/60 p-1.5 dark:border-slate-700/50 dark:bg-slate-800/60">
                  <span class="text-xs text-slate-400">P0</span>
                  <span class="font-mono text-sm font-bold text-blue-600">{{ traversalStore.realtimeResult?.P0?.toFixed(2) ?? '--' }}</span>
                </div>
                <div class="flex flex-col rounded-md border border-slate-100 bg-slate-50/60 p-1.5 dark:border-slate-700/50 dark:bg-slate-800/60">
                  <span class="text-xs text-slate-400">Ps</span>
                  <span class="font-mono text-sm font-bold text-blue-600">{{ traversalStore.realtimeResult?.Ps?.toFixed(2) ?? '--' }}</span>
                </div>
              </div>
              <!-- 有效性状态 -->
              <div class="flex items-center justify-between rounded-md border px-2 py-1.5" :class="hasRealtimeResult && traversalStore.realtimeResult?.isValid ? 'border-emerald-200 bg-emerald-50/60 dark:border-emerald-800/50 dark:bg-emerald-900/15' : 'border-slate-100 bg-slate-50/60 dark:border-slate-700/50 dark:bg-slate-800/60'">
                <div class="flex items-center gap-1.5">
                  <span class="h-2 w-2 rounded-full" :class="hasRealtimeResult && traversalStore.realtimeResult?.isValid ? 'bg-emerald-500' : hasRealtimeResult ? 'bg-rose-500' : 'bg-slate-300'"></span>
                  <span class="text-xs text-slate-500">{{ t.validity }}</span>
                </div>
                <span class="font-mono text-xs font-semibold" :class="hasRealtimeResult && traversalStore.realtimeResult?.isValid ? 'text-emerald-600 dark:text-emerald-400' : hasRealtimeResult ? 'text-rose-600 dark:text-rose-400' : 'text-slate-400'">
                  {{ hasRealtimeResult ? (traversalStore.realtimeResult?.isValid ? t.valid : t.limit) : '--' }}
                </span>
              </div>
            </div>

            <!-- 原始压力数据：合并到同一面板，用分隔线区分 -->
            <div>
              <div class="flex items-center gap-1.5 mb-1.5">
                <ClipboardList class="h-3.5 w-3.5 text-slate-400" />
                <span class="text-xs font-medium uppercase tracking-wider text-slate-400">{{ t.realtimePressureData }}</span>
              </div>

              <div class="grid grid-cols-2 gap-1">
                <div
                  v-for="item in pressureItems"
                  :key="item.key"
                  class="flex flex-col rounded-md border px-1.5 py-1 transition-colors"
                  :class="item.disabled
                    ? 'border-slate-100 bg-slate-50/40 dark:border-slate-700/50 dark:bg-slate-800/40'
                    : 'border-blue-100 bg-blue-50/20 dark:border-blue-800/30 dark:bg-blue-900/10'"
                >
                  <div class="flex items-center justify-between">
                    <span class="text-xs font-medium" :class="item.disabled ? 'text-slate-400' : 'text-slate-500 dark:text-slate-400'">{{ item.label }}</span>
                    <span v-if="!item.disabled" class="text-xs text-slate-300 dark:text-slate-600">{{ item.unit }}</span>
                  </div>
                  <div class="font-mono text-sm font-semibold" :class="item.disabled ? 'text-slate-300 dark:text-slate-600' : 'text-slate-700 dark:text-slate-200'">{{ item.value }}</div>
                </div>
              </div>
            </div>
          </section>
        </aside>

        <!-- Main Content Area -->
        <div class="flex min-h-0 flex-1 flex-col overflow-hidden">
          <div data-test="traversal-workspace-primary" class="min-h-0 flex-1">
            <section class="h-full flex flex-col rounded-xl border border-slate-200 bg-white shadow-sm dark:border-slate-700 dark:bg-slate-900">
              <!-- 工作区头部 -->
              <div class="flex flex-wrap items-center justify-between gap-3 border-b border-slate-100 px-4 py-3 dark:border-slate-800">
                <div class="flex items-center gap-2">
                  <div class="h-6 w-1 rounded-full bg-blue-500"></div>
                  <div>
                  <h3 class="text-sm font-semibold text-slate-800 dark:text-slate-100">
                      {{ workspaceTabMeta.title }}
                    </h3>
                    <p class="text-[10px] text-slate-400">
                      {{ workspaceTabMeta.subtitle }}
                    </p>
                  </div>
                </div>
                <!-- 标签页切换 -->
                <div class="flex rounded-lg border border-slate-200 bg-slate-50 p-0.5 dark:border-slate-700 dark:bg-slate-800">
                  <UiButton
                    v-for="tab in workspaceTabs"
                    :key="tab.value"
                    quaternary
                    :class="[
                      activeWorkspaceTab === tab.value ? 'bg-blue-500 text-white shadow-sm' : 'text-slate-500 hover:text-slate-700 dark:hover:text-slate-200',
                      'h-9 px-3 text-sm'
                    ]"
                    @click="activeWorkspaceTab = tab.value"
                  >
                    {{ tab.label }}
                  </UiButton>
                </div>
              </div>

              <!-- 内容区 -->
              <div class="flex-1 overflow-hidden relative">
                <!-- 点位预览图例 -->
                <div v-if="activeWorkspaceTab === 'preview'" class="absolute right-4 top-3 z-10 flex items-center gap-3 rounded-full border border-slate-200 bg-white/90 px-3 py-1.5 text-[10px] shadow-sm backdrop-blur dark:border-slate-700 dark:bg-slate-900/90">
                  <div class="flex items-center gap-1">
                    <span class="h-2 w-2 rounded-full bg-blue-500"></span>
                    <span class="text-slate-500">{{ t.moving }}</span>
                  </div>
                  <div class="flex items-center gap-1">
                    <span class="h-2 w-2 rounded-full bg-amber-400"></span>
                    <span class="text-slate-500">{{ t.stabilizing }}</span>
                  </div>
                  <div class="flex items-center gap-1">
                    <span class="h-2 w-2 rounded-full bg-emerald-500"></span>
                    <span class="text-slate-500">{{ t.acquiring }}</span>
                  </div>
                  <div class="flex items-center gap-1">
                    <span class="h-2 w-2 rounded-full bg-gradient-to-r from-purple-500 to-pink-500"></span>
                    <span class="text-slate-500">{{ t.completed }}</span>
                  </div>
                  <div class="flex items-center gap-1">
                    <span class="h-2 w-2 rounded-full bg-slate-300"></span>
                    <span class="text-slate-500">{{ t.untested }}</span>
                  </div>
                </div>

                <template v-if="activeWorkspaceTab === 'preview'">
                  <PointsPreview
                    v-if="currentConfig?.layout"
                    :layout="currentConfig.layout"
                    :current-point="traversalStore.status?.currentPoint"
                    :completed-points="traversalStore.status?.completedPoints"
                    :current-point-phase="traversalStore.status?.currentPointPhase"
                    :visible="activeWorkspaceTab === 'preview'"
                  />
                  <!-- 空状态 -->
                  <div v-else class="flex h-full w-full flex-col items-center justify-center gap-4 text-center">
                    <div class="flex h-16 w-16 items-center justify-center rounded-2xl bg-slate-100 dark:bg-slate-800">
                      <ClipboardList class="h-8 w-8 text-slate-300 dark:text-slate-600" />
                    </div>
                    <div>
                      <div class="text-sm font-medium text-slate-500 dark:text-slate-400">{{ t.noLayoutConfigured }}</div>
                      <div class="mt-1 text-xs text-slate-400">{{ t.pleaseConfigureLayout }}</div>
                    </div>
                    <UiButton
                      variant="primary" size="sm"
                      @click="openSettings"
                    >
                      <template #icon>
                        <Settings class="h-3.5 w-3.5" />
                      </template>
                      {{ t.configureLayout }}
                    </UiButton>
                  </div>
                </template>
                <div v-else-if="activeWorkspaceTab === 'visualization'" class="h-full p-4">
                  <TraversalVisualization />
                </div>
                <div v-else class="h-full overflow-auto p-4">
                  <ProbeReferenceCard />
                </div>
              </div>
            </section>
          </div>
        </div>
      </div>
    </div>

    <!-- 启动确认对话框：前置条件检查 + 测试摘要 + 确认/取消 -->
    <div v-if="showStartConfirm" class="fixed inset-0 z-50 flex items-center justify-center bg-slate-900/60" role="dialog" aria-modal="true" :aria-label="t.confirmStartTitle" aria-describedby="confirm-start-desc" @keydown.escape="cancelStartConfirm" @click.self="cancelStartConfirm">
      <div ref="confirmDialogRef" class="w-[calc(100%-2rem)] max-w-[440px] rounded-lg border border-slate-200 bg-white p-5 shadow-xl dark:border-slate-700 dark:bg-slate-900" @keydown.tab="trapFocus">
        <h3 id="confirm-start-title" class="mb-1 text-base font-bold text-slate-900 dark:text-slate-100">{{ t.confirmStartTitle }}</h3>
        <p id="confirm-start-desc" class="mb-4 text-xs text-slate-500 dark:text-slate-400">{{ t.confirmStartMessage }}</p>

        <!-- 前置条件检查结果 -->
        <div v-if="isCheckingPreconditions" class="mb-4 flex items-center gap-2 text-xs text-slate-400">
          <div class="h-4 w-4 animate-spin rounded-full border-2 border-blue-500 border-t-transparent"></div>
          {{ t.checkingPreconditions }}
        </div>
        <div v-else-if="preconditionResult" class="mb-4 space-y-1.5">
          <div
            v-for="check in preconditionResult.checks"
            :key="check.name"
            class="flex items-center gap-2 rounded-lg px-2.5 py-1.5 text-xs"
            :class="check.passed ? 'bg-emerald-50 dark:bg-emerald-900/20' : 'bg-rose-50 dark:bg-rose-900/20'"
          >
            <CheckCircle v-if="check.passed" class="h-3.5 w-3.5 shrink-0 text-emerald-500" />
            <XCircle v-else class="h-3.5 w-3.5 shrink-0 text-rose-500" />
            <span :class="check.passed ? 'text-slate-600 dark:text-slate-300' : 'text-rose-600 dark:text-rose-400 font-medium'">
              {{ check.message || check.name }}
            </span>
          </div>
        </div>

        <!-- 测试摘要 -->
        <div v-if="currentConfig" class="mb-4 rounded-lg bg-slate-50 p-3 dark:bg-slate-800/60">
          <div class="grid grid-cols-2 gap-2 text-xs">
            <div>
              <span class="text-slate-400">{{ t.confirmStartPoints }}</span>
              <span class="ml-1.5 font-mono font-semibold text-blue-500">{{ currentConfig.layout ? getTraversalLayoutPointCount(currentConfig.layout) : '--' }}</span>
            </div>
            <div>
              <span class="text-slate-400">{{ t.confirmStartOutput }}</span>
              <span class="ml-1.5 truncate text-slate-600 dark:text-slate-300">{{ currentConfig.savePath || '--' }}</span>
            </div>
          </div>
        </div>

        <!-- 操作按钮 -->
        <div class="flex items-center justify-end gap-2">
          <UiButton quaternary size="sm" @click="cancelStartConfirm">
            {{ t.dismiss }}
          </UiButton>
          <UiButton
            :variant="preconditionResult?.allPassed ? 'primary' : 'danger'"
            size="sm"
            :disabled="!preconditionResult || isCheckingPreconditions || !preconditionResult.allPassed"
            @click="confirmStartTest"
          >
            <template #icon>
              <Play class="h-3.5 w-3.5 fill-current" />
            </template>
            {{ t.confirmStartStart }}
          </UiButton>
        </div>
      </div>
    </div>

    <!-- Error Banner -->
    <div v-if="traversalStore.error" class="shrink-0 border-t border-rose-200 bg-rose-50 px-6 py-3 dark:border-rose-900/30 dark:bg-rose-900/10">
      <div class="flex items-center justify-between">
        <div class="flex items-center gap-3 text-sm font-medium text-rose-600 dark:text-rose-400">
          <AlertTriangle class="h-4 w-4" />
          <span>{{ traversalStore.error }}</span>
        </div>
        <UiButton variant="danger" size="sm" @click="traversalStore.clearError">{{ t.dismiss }}</UiButton>
      </div>
    </div>
  </div>
</template>

