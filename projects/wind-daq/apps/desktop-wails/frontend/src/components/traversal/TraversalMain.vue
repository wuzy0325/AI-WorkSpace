/**
 * ============================================================================
 * 五孔探针移位测试主画面（FiveHoleTraversalMain）
 * ============================================================================
 *
 * 【功能定位】
 * 用于五孔探针的正式风洞实验，按预设轨迹自动遍历多个点位，
 * 使用已校准的 PRB 系数进行实时插值计算。
 *
 * 【布局重构（2026-07）】
 * 参考 ThreeHoleMain.vue 的布局风格：
 *   - 顶部：Header（标题+配置） + 状态栏（跨全宽，集中展示操作员最频繁看的核心信息）
 *   - 中部：断点恢复横幅 / 插值器恢复横幅（条件显示）
 *   - 主工作区：左侧栏 384px（三段式：控制按钮 + 数据 + 硬件状态条） + 右侧 Tab 工作区
 *
 * 【组件结构】
 * - shell/TraversalTopBar.vue        — Header + 状态栏
 * - shell/TraversalCheckpointBanner.vue — 断点恢复横幅
 * - shell/TraversalErrorBanner.vue   — 错误横幅
 * - panels/TraversalLiveMonitor.vue  — 左侧三段式实时监测面板（含控制按钮）
 * - panels/TraversalWorkspaceArea.vue — 右侧 Tab 工作区
 * - dialogs/TraversalStartConfirm.vue — 启动确认对话框
 *
 * @module FiveHoleTraversalMain
 * @see TraversalSettings.vue - 测试配置
 * @see FiveHoleMain.vue - 探针标定系统
 * ============================================================================
 */
<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { useDeviceStore } from '@stores/deviceStore'
import { useFeedbackStore } from '@stores/feedbackStore'
import { useMotionStore } from '@stores/motionStore'
import { useTraversalStore } from '@stores/traversalStore'
import { useI18nStore } from '@stores/i18nStore'
import type {
  TraversalCheckpoint,
  PreconditionCheckResult
} from '@shared/types/traversal'
import { TRAVERSAL_PROBE_PRESENTATION } from '@shared/types/traversal'
import { joinCalibrationPath } from '@shared/calibrationCsvPath'
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
  /**
   * 打开 TraversalSettings 对话框。
   * @param step 可选,定位到的步骤索引(0=通道, 1=PRB, 2=布点, 3=摘要);
   *             默认 0。"PRB 未加载" 状态条点击时传 1 直达 PRB 步骤。
   */
  openSettings: [step?: number]
  back: []
}>()

const traversalStore = useTraversalStore()
const deviceStore = useDeviceStore()
const motionStore = useMotionStore()
const feedbackStore = useFeedbackStore()
const hasConfig = computed(() => traversalStore.config !== null)
const currentConfig = computed(() => traversalStore.config)

// 实时数据 composable：统一订阅 DAQ 快照并计算压力/插值输入/展示项
const {
  liveInterpolationInput,
  pressureItems,
  subscribeSnapshot,
  ensureSubscribed,
} = useTraversalRealtimeData(currentConfig)

const { t } = storeToRefs(useI18nStore())

// 探针展示元数据（spec §6.5）：五孔 Alpha=攻角/Beta=侧滑角，
// 七孔 Alpha=侧滑角/Beta=迎角；标题与角度标签统一查 TRAVERSAL_PROBE_PRESENTATION。
const probePresentation = computed(
  () => TRAVERSAL_PROBE_PRESENTATION[(traversalStore.config?.probeType ?? 'five-hole') as keyof typeof TRAVERSAL_PROBE_PRESENTATION]
)
const probeTitleText = computed(() => t.value[probePresentation.value.titleKey as unknown as keyof typeof t.value] ?? probePresentation.value.titleKey)
const alphaLabelText = computed(() => t.value[probePresentation.value.alphaLabelKey as unknown as keyof typeof t.value] ?? probePresentation.value.alphaLabelKey)
const betaLabelText = computed(() => t.value[probePresentation.value.betaLabelKey as unknown as keyof typeof t.value] ?? probePresentation.value.betaLabelKey)

const activeWorkspaceTab = ref<WorkspaceTab>('preview')
// 启动请求防重入标志（与 store.isStarting 配合，避免重复触发）
const isStartRequestPending = ref(false)

// 当前探针类型：config 未加载时按五孔默认（与 probePresentation fallback 一致）。
// 用于判断「探针参考」tab 是否适用——ProbeReferenceCard 仅绘制五孔端面 P1-P5 几何，
// 七孔端面几何不同，参考图会误导操作员，故七孔时隐藏该 tab。
const probeType = computed(() => traversalStore.config?.probeType ?? 'five-hole')

// 前置条件检查与启动确认对话框状态
const showStartConfirm = ref(false)
const isCheckingPreconditions = ref(false)
const preconditionResult = ref<PreconditionCheckResult | null>(null)
/** 顶栏引用：用于确认对话框关闭后将焦点回到开始按钮（开始按钮在顶栏 Header 行） */
const topBarRef = ref<InstanceType<typeof TraversalTopBar> | null>(null)

// 本地维护的断点信息（用于 UI 横幅显示，与 store.checkpoint 同步）
const checkpoint = ref<TraversalCheckpoint | null>(null)
let unsubscribeDaqSnapshot: (() => void) | null = null
let unsubscribeDeviceStatus: (() => void) | null = null
let unsubscribeMotionStatus: (() => void) | null = null

const hasCheckpoint = computed(() => checkpoint.value !== null)
// 是否显示真实控制按钮（恢复中不显示）
const showRealControls = computed(() => !props.recovering)

const workspaceTabs = computed(() => {
  const tabs: Array<{ value: WorkspaceTab; label: string }> = [
    { value: 'preview', label: t.value.pointsPreview },
    { value: 'visualization', label: t.value.flowVisualization }
  ]
  // 「探针参考」tab 仅五孔探针适用：ProbeReferenceCard 硬编码 P1-P5 五孔端面几何，
  // 七孔探针端面布局不同，参考图会误导操作员，故七孔时隐藏该 tab。
  if (probeType.value === 'five-hole') {
    tabs.push({ value: 'reference', label: t.value.probeReference })
  }
  return tabs
})

// 当探针类型变化导致当前选中的 tab 不再可用时，切回 preview。
// 典型场景：用户在五孔配置下切到「探针参考」tab，随后修改配置为七孔；
// 不处理会导致 activeWorkspaceTab 指向已隐藏的 tab，工作区显示空白。
watch(probeType, (newType) => {
  if (newType === 'seven-hole' && activeWorkspaceTab.value === 'reference') {
    activeWorkspaceTab.value = 'preview'
  }
})

onMounted(async () => {
  void deviceStore.refreshInstances()
  unsubscribeDeviceStatus = deviceStore.attachStatusListener()

  void motionStore.refreshStatus()
  unsubscribeMotionStatus = motionStore.attachStatusListener()

  unsubscribeDaqSnapshot = subscribeSnapshot()

  // 恢复已保存的配置，确保指示灯和 UI 能正确反映设备/机构状态
  await traversalStore.loadConfig()
  // 配置恢复后确保数据订阅已建立：设备可能已在其他页面开始采集，
  // 也可能后续由遍历测试自动启动采集。走 deviceStore.ensureSubscribed
  // 而非 deviceApi.subscribeToDevice，确保 subscribedDeviceIds 同步更新，
  // 离开页面时 cleanupSnapshotSubscriptions 能正确清理轮询定时器。
  ensureSubscribed()

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
    // 断点恢复后同样确保数据订阅：用户可能从其他页面返回，
    // 之前的订阅可能已取消；幂等调用不会重复建立。
    ensureSubscribed()
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

/**
 * 打开 TraversalSettings 对话框。
 * @param step 可选,定位到的步骤索引(默认 0=通道);"PRB 未加载" 状态条点击时传 1。
 */
function openSettings(step?: number): void {
  emit('openSettings', step)
}

/**
 * 启动测试入口：打开确认对话框并执行前置条件检查。
 * 与 Cursor DAQ 行为一致：先检查再确认，避免误触。
 */
async function startTest(): Promise<void> {
  if (isStartRequestPending.value || traversalStore.isStarting) {
    return
  }

  if (startDisabledReason.value) {
    feedbackStore.pushToast(startDisabledReason.value, 'warning')
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
  if (startDisabledReason.value) {
    showStartConfirm.value = false
    feedbackStore.pushToast(startDisabledReason.value, 'warning')
    return
  }
  showStartConfirm.value = false
  isStartRequestPending.value = true
  try {
    await traversalStore.startTest(currentConfig.value)
    // 后端可能在此自动启动了采集；确保前端数据订阅已建立，
    // 否则 onSnapshot 不会收到持续数据，左侧栏会"编一个点才更新一次"。
    ensureSubscribed()
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
  // 关闭后将焦点返回触发按钮；按钮不在 DOM 中（运行态）时退回顶栏根元素
  nextTick(() => {
    if (topBarRef.value?.focusStart()) return
    const fallback = document.querySelector('[data-test="traversal-top-toolbar"]') as HTMLElement | null
    fallback?.focus()
  })
}

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
  // 停止测试会丢失当前进度且未保存数据无法恢复，需弹出二次确认防止误触。
  // 与校准模块 stopCalibration 行为一致：使用 feedbackStore.confirm 等待用户决定，
  // 仅在用户确认后才调用 store.stop() 触发后端停止流程。
  const accepted = await feedbackStore.confirm(t.value.travStopConfirm, {
    title: t.value.travStopTitle,
    confirmText: t.value.stop,
    cancelText: t.value.cancel,
  })
  if (!accepted) return

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

// 状态色 token：将 statusType 映射到设计 token，替代 Tailwind 调色板硬编码
// 与 ThreeHoleMain.vue 中的 statusColorToken 实现保持一致
// 注意：TraversalTestStatusType 仅含 idle/running/paused/completed/error/stopped，
// moving/stabilizing/acquiring/saving 属于 TraversalPointPhase（点位阶段），不应在此判断
const statusColorToken = computed(() => {
  switch (traversalStore.statusType) {
    case 'running':
      return '--accent-success'
    case 'paused':
    case 'stopped':
      return '--accent-warning'
    case 'completed':
      return '--accent-info'
    case 'error':
      return '--accent-danger'
    default:
      return '--text-muted'
  }
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

// 使用 ref + setInterval(1s) 维护 now 引用，供 elapsedText 和 estimatedRemainingText 共享
const now = ref(Date.now())
let elapsedTimer: ReturnType<typeof setInterval> | null = null
onMounted(() => {
  elapsedTimer = setInterval(() => {
    now.value = Date.now()
  }, 1000)
})
onBeforeUnmount(() => {
  if (elapsedTimer) {
    clearInterval(elapsedTimer)
    elapsedTimer = null
  }
})

// 终态冻结锚点：测试进入终态（completed/stopped/error）那一刻记录时间戳，
// 用于"已用时间"冻结，避免完成后 Elapsed 继续随 now 增长（见 elapsedText）。
// 重新进入运行态时清空，确保下一轮测试能再次正确记录。
const finishedAt = ref<number | null>(null)
function isTerminalStatusValue(s: string | undefined): boolean {
  return s === 'completed' || s === 'error' || s === 'stopped'
}
watch(
  () => traversalStore.status?.status,
  (newStatus) => {
    if (isTerminalStatusValue(newStatus)) {
      if (finishedAt.value === null) {
        finishedAt.value = Date.now()
      }
    } else {
      finishedAt.value = null
    }
  }
)

// 预估剩余时间：后端不返回该字段，前端基于已用时间 + 已完成点数自行估算
// 公式：平均单点耗时 = 已用时间 / 已完成点数；剩余时间 = 平均单点耗时 × 剩余点数
//
// 首点（completed=0）估算策略：
//   后端 CurrentPoint 在每个点采完后才递增（traversal_acquisition.go:331/472），
//   采首点期间 completedPoints=0，旧逻辑直接返回 '--'，UI 长时间无剩余时间显示。
//   现用首点已用时间作为单点耗时估算（会偏小，但比 '--' 更有参考价值）；
//   首点完成后自动切换为 elapsed/completed 的精确平均值。
const estimatedRemainingText = computed(() => {
  const status = traversalStore.status
  const startTime = status?.startTime
  const completed = status?.completedPoints ?? 0
  const total = status?.totalPoints ?? 0
  if (typeof startTime !== 'number' || startTime <= 0) return '--'
  if (total <= 0) return '--'
  // 终态或全部完成：剩余 0
  if (traversalStore.isTerminal || total <= completed) return '0s'
  const elapsedMs = Math.max(0, now.value - startTime)
  if (elapsedMs <= 0) return '--'
  // completed=0 时用首点已用时间作为单点耗时下界估算（偏小，首点完成后自动修正）
  const avgPerPoint = completed > 0 ? elapsedMs / completed : elapsedMs
  const remainingMs = Math.round(avgPerPoint * (total - completed))
  if (remainingMs <= 0) return '0s'
  const seconds = Math.ceil(remainingMs / 1000)
  if (seconds < 60) return `${seconds}s`
  const minutes = Math.floor(seconds / 60)
  const remainSeconds = seconds % 60
  if (minutes < 60) return `${minutes}m ${remainSeconds}s`
  const hours = Math.floor(minutes / 60)
  const remainMinutes = minutes % 60
  return `${hours}h ${remainMinutes}m`
})

const elapsedText = computed(() => {
  const startTime = traversalStore.status?.startTime
  if (typeof startTime !== 'number' || startTime <= 0) return '--'
  let elapsedMs: number
  if (traversalStore.isTerminal) {
    // 终态：冻结已用时间，不再随 now 增长。
    // 优先使用后端返回的真实耗时（TraversalCompleteEvent.duration，毫秒），
    // 缺失时回退到进入终态那一刻 now - startTime 冻结值。
    const dur = traversalStore.completeEvent?.duration
    elapsedMs = dur && dur > 0 ? dur : Math.max(0, (finishedAt.value ?? now.value) - startTime)
  } else {
    elapsedMs = Math.max(0, now.value - startTime)
  }
  const seconds = Math.floor(elapsedMs / 1000)
  if (seconds < 60) return `${seconds}s`
  const minutes = Math.floor(seconds / 60)
  const remainSeconds = seconds % 60
  if (minutes < 60) return `${minutes}m ${remainSeconds}s`
  const hours = Math.floor(minutes / 60)
  const remainMinutes = minutes % 60
  return `${hours}h ${remainMinutes}m`
})

// 顶栏进度摘要可见性（运行/暂停/已完成时显示）
const showProgress = computed(() =>
  traversalStore.isRunning ||
  traversalStore.isPaused ||
  traversalStore.status?.status === 'completed'
)

// 目标点位：来自后端 status.currentPoint。兼容字段名为 alpha/beta，实际语义是逻辑 X/Y 目标。
const targetPoint = computed(() => traversalStore.status?.currentPoint)

// 实时马赫数/速度：来自 traversalStore.realtimeResult
const machNumber = computed(() => traversalStore.realtimeResult?.machNumber)
const velocity = computed(() => traversalStore.realtimeResult?.velocity)

// CSV 保存路径：操作员需知道数据写到哪。
// 优先使用后端 Status.csvPath（实际落盘路径，含撞名 -2/-3 后缀），
// 回退到 config 静态拼接的预期路径（与后端 ResolveOutputPath 语义对齐）：
//   - savePath 为空 → 返回空串，侧边栏不展示该卡片
//   - savePath 已带 .csv 后缀（大小写不敏感）→ 视为完整文件路径，直接展示
//   - 否则 → 拼接 savePath + saveFileName，并保证文件名带 .csv 后缀
// 注意：仅进入 TraversalSettings 会通过 buildCalibrationCsvName 归一化 saveFileName；
// 若用户仅进入主页（loadConfig 直读后端），持久化 saveFileName 可能为旧值不带 .csv，
// 故此处必须显式补齐 .csv，否则侧边栏展示路径与实际落盘文件名不一致。
// saveFileName 缺失时不强行 fallback，避免展示后端 taskID 派生的默认名
// （taskID 在前端启动前未确定，展示假名反而误导）。
const csvSavePath = computed(() => {
  // 测试启动后优先展示后端实际落盘路径（撞名 -2/-3 后缀后的真实文件名）；
  // 启动前或未注入 v2 csvPort 时 status.csvPath 为空，回退到 config 静态拼接的预期路径
  const actualPath = traversalStore.status?.csvPath?.trim()
  if (actualPath) return actualPath

  const cfg = currentConfig.value
  if (!cfg) return ''
  const dir = cfg.savePath?.trim() ?? ''
  if (!dir) return ''
  // savePath 已带 .csv 后缀：直接当完整文件路径展示
  if (/\.csv$/i.test(dir)) return dir
  let fileName = cfg.saveFileName?.trim() ?? ''
  if (!fileName) return dir
  // 与后端 ResolveOutputPath 行为对齐：saveFileName 缺 .csv 后缀时自动补齐，
  // 避免旧配置展示的路径与实际落盘文件名不一致
  if (!/\.csv$/i.test(fileName)) fileName += '.csv'
  return joinCalibrationPath(dir, fileName)
})

// 错误信息：优先 status.lastError，其次 store.error
const lastError = computed(() => traversalStore.status?.lastError ?? traversalStore.error ?? '')

// 轴位置 / 运动器连接 / 采集设备连接：抽到 useHardwareConnectionStatus，
// 模板需要的颜色 / label 等显示态由 composable 统一计算。
const {
  axisPositions,
  positionerConnection,
  acquisitionConnection
} = useHardwareConnectionStatus(currentConfig)

const startDisabledReason = computed(() => {
  if (!hasConfig.value) return t.value.pleaseConfigureFirst
  if (positionerConnection.value.state !== 'connected') return t.value.wf_motionControllerDisconnected
  // state 取自 useHardwareConnectionStatus，按严重度从高到低判定：
  //   'acquiring'    → 正常，不禁用
  //   'connected'    → 已连接但未采集（提示先开始采集）
  //   'unconfigured' → 配置里没有任何已启用的采集通道绑定（与"未连接"是两种状态，文案分开）
  //   其他（含 'disconnected'）→ 未连接
  switch (acquisitionConnection.value.state) {
    case 'acquiring':
      return ''
    case 'connected':
      return t.value.wf_acquisitionDeviceNotAcquiring
    case 'unconfigured':
      return t.value.wf_acquisitionDeviceUnconfigured
    default:
      return t.value.wf_acquisitionDeviceDisconnected
  }
})

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
</script>

<template>
  <div
    class="flex h-full w-full flex-col flex-1 min-w-0"
    :style="{ background: 'var(--bg-canvas)', color: 'var(--text-primary)' }"
  >
    <!-- 顶栏：Header（含控制按钮）+ 状态栏 -->
    <TraversalTopBar
      ref="topBarRef"
      :title="probeTitleText"
      :status-text="statusText"
      :status-color-token="statusColorToken"
      :automated-run-label="t.automatedRun"
      :has-config="hasConfig"
      :is-start-request-pending="isStartRequestPending"
      :is-starting="traversalStore.isStarting"
      :show-real-controls="showRealControls"
      :can-start="traversalStore.canStart"
      :start-disabled="!!startDisabledReason"
      :start-disabled-reason="startDisabledReason"
      :can-pause="traversalStore.canPause"
      :can-resume="traversalStore.canResume"
      :show-progress="showProgress"
      :progress-summary="progressSummary"
      :progress-percent="progressPercent"
      :elapsed-text="elapsedText"
      :estimated-remaining-text="estimatedRemainingText"
      :labels="{
        configBtn: t.configBtn,
        startRun: t.startRun,
        travPause: t.travPause,
        travStop: t.travStop,
        travResume: t.travResume,
        elapsed: t.travElapsed,
        remaining: t.travRemaining,
        progress: t.travProgress,
      }"
      @open-settings="openSettings"
      @start="startTest"
      @pause="pauseTest"
      @resume="resumeTest"
      @stop="stopTest"
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
      >{{ t.travGotIt }}</button>
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

    <!-- 主工作区：左侧栏 384px + 右侧 Tab 工作区，flex 布局占满空间 -->
    <div v-else class="flex flex-1 overflow-hidden">
      <TraversalLiveMonitor
        :target-point="targetPoint"
        :pattern="currentConfig?.layout.pattern"
        :actual-positions="axisPositions"
        :mach-number="machNumber"
        :velocity="velocity"
        :csv-save-path="csvSavePath"
        :last-error="lastError"
        :validation-warnings="traversalStore.status?.validationWarnings"
        :warning="traversalStore.status?.warning"
        :motion-safety-failure="traversalStore.status?.motionSafetyFailure"
        :acquisition-connection="acquisitionConnection"
        :positioner-connection="positionerConnection"
        :pressure-items="pressureItems"
        :realtime-result="traversalStore.realtimeResult"
        :has-loaded-interpolator="traversalStore.hasLoadedInterpolator"
        @navigate-to-prb="openSettings(1)"
        :labels="{
          target: t.travTarget,
          targetXDirection: t.travTargetXDirection,
          targetYDirection: t.travTargetYDirection,
          targetZDirection: t.travTargetZDirection,
          targetUDirection: t.travTargetUDirection,
          actual: t.travActual,
          mach: t.mach,
          velocity: t.velocity,
          realtimeCalculation: t.realtimeCalculation,
          interpolationNotLoaded: t.interpolationNotLoaded,
          interpolationInvalid: t.interpolationInvalid,
          interpolationWaitingData: t.interpolationWaitingData,
          realtimePressureData: t.realtimePressureData,
          alpha: alphaLabelText,
          beta: betaLabelText,
          csvPath: t.travCsvPath,
          validationWarnings: t.travValidationWarnings,
          returnToOriginWarning: t.travReturnToOriginWarning,
          hardwareStatus: t.travHardwareStatus,
          acquisitionDevice: t.travAcquisitionDevice,
          positionerDevice: t.travPositionerDevice,
          moving: t.moving,
          motionSafetyAlert: t.travMotionSafetyAlert,
          motionSafetyAlertEmergency: t.travMotionSafetyAlertEmergency,
          motionSafetyAxis: t.travMotionSafetyAxis,
          motionSafetyTarget: t.travMotionSafetyTarget,
          motionSafetyActual: t.travMotionSafetyActual,
          motionSafetyDeviation: t.travMotionSafetyDeviation,
          motionSafetyPointIndex: t.travMotionSafetyPointIndex,
          motionSafetyController: t.travMotionSafetyController,
          motionSafetyVerdictOk: t.travMotionSafetyVerdictOk,
          motionSafetyVerdictArrived: t.travMotionSafetyVerdictArrived,
          motionSafetyVerdictDeviation: t.travMotionSafetyVerdictDeviation,
          motionSafetyVerdictCriticalDeviation: t.travMotionSafetyVerdictCriticalDeviation,
          motionSafetyVerdictLimitTriggered: t.travMotionSafetyVerdictLimitTriggered,
          motionSafetyVerdictNoProgress: t.travMotionSafetyVerdictNoProgress,
          motionSafetyVerdictOvershoot: t.travMotionSafetyVerdictOvershoot,
          motionSafetyVerdictStatusUnavailable: t.travMotionSafetyVerdictStatusUnavailable,
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
