import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { traversalApi } from '@api/traversalApi'

import { useI18nStore } from '@stores/i18nStore'
import { useDeviceStore } from '@stores/deviceStore'
import { useStorageStore } from '@stores/storageStore'
import { normalizeTraversalLayoutRanges } from '@shared/types/traversal'
import {
  createDefaultTraversalProbeChannels,
  createSevenHoleTraversalProbeChannels
} from '@shared/types/traversal'

function formatApiError(err: unknown): string {
  const msg = err instanceof Error ? err.message : String(err)
  if (msg.includes('Failed to fetch') || msg.includes('NetworkError')) {
    return useI18nStore().t.travErrNetwork
  }
  return msg
}

import type {
  TraversalTestConfig,
  TraversalTestStatus,
  TraversalDataPoint,
  TraversalProgressEvent,
  TraversalCompleteEvent,
  TraversalErrorEvent,
  InterpolationResult,
  MultiPrbInterpolationMode,
  PreconditionCheckResult,
  PrbFileInfo,
  TraversalTerminalStatus,
  TraversalInterpolationInput,
  TraversalRawPressure,
  CalibrationCsvFileInfo,
  InterpolationAlgorithm,
  TraversalCheckpoint,
  TraversalErrorCode,
  TraversalProbeType,
  TraversalRealtimeInput,
  SevenHolePrbFileInfo,
  SevenHoleTraversalInterpolationConfig
} from '@shared/types/traversal'

export type RealtimePressures = TraversalRawPressure

export const useTraversalStore = defineStore('traversal', () => {
  // 国际化：store 内部错误消息需随语言切换，故引入 i18n store
  const i18n = useI18nStore()
  // 设备 store：用于同步采集状态指示。遍历测试不控制设备采集生命周期，
  // 开始前要求操作员已在设备管理中启动采集。
  const deviceStore = useDeviceStore()
  const statusRecoveryFailed = ref(false)
  // 启动防重入标志：避免用户连续点击"开始"导致并发启动
  const isStarting = ref(false)
  const status = ref<TraversalTestStatus | null>(null)
  const statusType = computed(() => {
    if (statusRecoveryFailed.value && !status.value) {
      return 'unknown'
    }

    return status.value?.status ?? 'idle'
  })
  const isRunning = computed(() => status.value?.status === 'running')
  const isPaused = computed(() => status.value?.status === 'paused')
  const isTerminal = computed(() => isTerminalStatus(status.value?.status))
  // canStart 需要考虑 isStarting，避免启动过程中重复触发
  const canStart = computed(() => !isStarting.value && statusType.value !== 'unknown' && (statusType.value === 'idle' || isTerminal.value))
  // canPause/canStop 使用 isActiveStatus，覆盖 moving/stabilizing/acquiring/saving 子状态
  const canPause = computed(() => isActiveStatus(status.value?.status))
  const canResume = computed(() => statusType.value === 'paused')
  const canStop = computed(() => isActiveStatus(status.value?.status) || statusType.value === 'paused')
  const progress = computed(() => status.value?.progress ?? 0)
  const dataPoints = ref<TraversalDataPoint[]>([])

  const realtimePressures = ref<RealtimePressures | null>(null)
  const realtimeResult = ref<InterpolationResult | null>(null)

  // 后端插值器是否已加载（PRB/CSV 导入成功后置 true，无需等待配置保存）
  const hasLoadedInterpolator = ref(false)

  // 插值器后端状态消息：
  //   - 正常加载时为 null
  //   - 启动校验发现后端未加载（如 PRB 文件被删除/移动）时填入后端 message
  //   - 网络抖动等导致校验失败时填入提示文本，但不强制清空 hasLoadedInterpolator
  // 该消息由 UI 层订阅展示（例如 TraversalPrbStep 顶部 Banner），
  // 避免静默失败导致用户在"已加载"假象下点击开始遍历后才报错。
  const interpolatorRestoreMessage = ref<string | null>(null)

  const config = ref<TraversalTestConfig | null>(null)

  const completeEvent = ref<TraversalCompleteEvent | null>(null)

  const error = ref<string | null>(null)
  // 模拟模式标志：纯前端模拟时为 true，不调用后端硬件接口
  const isSimulation = ref(false)

  // 实时插值节流间隔：复用全局 storageStore.settings.refreshRateHz（默认 5Hz），
  // 与实时压力数据同频，避免出现"压力卡片与插值卡片刷新节奏不一致"的视觉错位。
  // refreshRateHz 已在 storageStore 内 clamp 到 1–10Hz（系统边界），此处直接消费不再二次保护。
  const uiRefreshIntervalMs = computed(() => Math.round(1000 / useStorageStore().settings.refreshRateHz))

  // 断点恢复信息（应用启动时加载，用于判断是否展示"恢复"横幅）
  const checkpoint = ref<TraversalCheckpoint | null>(null)
  const hasCheckpoint = computed(() => checkpoint.value !== null)

  function toSerializableConfig(cfg: TraversalTestConfig): TraversalTestConfig {
    return JSON.parse(JSON.stringify(cfg)) as TraversalTestConfig
  }

  let unsubscribeProgress: (() => void) | null = null
  let unsubscribeComplete: (() => void) | null = null
  let unsubscribeError: (() => void) | null = null
  let recoveryRequestId = 0
  let startupBlockedTaskId: string | null = null
  let startupPendingTaskId: string | null = null
  // 实时插值节流相关状态：基于定时器的节流，间隔由 storageStore.settings.refreshRateHz 派生
  let realtimeInterpolationTimer: ReturnType<typeof setTimeout> | null = null
  let pendingRealtimeInterpolationInput: TraversalRealtimeInput | null = null
  let pendingRealtimeInterpolationConfig: TraversalTestConfig | null = null
  let lastRealtimeInterpolationAt = 0
  let realtimeInterpolationRequestId = 0

  function isTerminalStatus(value: TraversalTestStatus['status'] | undefined): value is TraversalTerminalStatus {
    return value === 'completed' || value === 'error' || value === 'stopped'
  }

  // 判断是否为活跃状态（含 running 及其子状态 moving/stabilizing/acquiring/saving）
  // 后端状态机会返回子状态字符串，故参数类型放宽为 string
  function isActiveStatus(value: string | undefined): boolean {
    return value === 'running' || value === 'moving' || value === 'stabilizing' || value === 'acquiring' || value === 'saving'
  }

  function beginStartupWindow(previousTaskId: string | null): void {
    startupBlockedTaskId = previousTaskId
    startupPendingTaskId = null
  }

  function setStartupPendingTask(taskId: string): void {
    startupPendingTaskId = taskId
  }

  function clearStartupWindow(): void {
    startupBlockedTaskId = null
    startupPendingTaskId = null
  }

  function beginRecoveryRequest(): number {
    recoveryRequestId += 1
    return recoveryRequestId
  }

  function isActiveRecoveryRequest(requestId: number): boolean {
    return recoveryRequestId === requestId
  }

  function cancelRecovery(): void {
    recoveryRequestId += 1
  }

  function clearRealtimeInterpolationTimer(): void {
    if (realtimeInterpolationTimer) {
      clearTimeout(realtimeInterpolationTimer)
      realtimeInterpolationTimer = null
    }
  }

  // 实际执行实时插值计算（清空 pending 并发起请求）
  async function runRealtimeInterpolation(): Promise<void> {
    clearRealtimeInterpolationTimer()

    const input = pendingRealtimeInterpolationInput
    const configOverride = pendingRealtimeInterpolationConfig
    pendingRealtimeInterpolationInput = null
    pendingRealtimeInterpolationConfig = null

    if (!input) {
      realtimeInterpolationRequestId += 1
      realtimeResult.value = null
      return
    }

    const requestId = ++realtimeInterpolationRequestId
    lastRealtimeInterpolationAt = Date.now()

    const res = await traversalApi.calculateRealtime(
      input,
      configOverride ? toSerializableConfig(configOverride) : undefined,
      // 七孔必须显式携带 probeType（spec §5.6）；五孔省略保持旧请求体
      (configOverride?.probeType ?? 'five-hole') === 'seven-hole' ? 'seven-hole' : undefined
    )
    if (requestId !== realtimeInterpolationRequestId) {
      return
    }

    if (res.success) {
      // HTTP 200:后端已返回 InterpolationResult(IsValid 区分成功/数据层失败)
      realtimeResult.value = res.data ?? null
    } else {
      // HTTP 400(如 PRB 未加载、探针类型不一致):后端返回 error 不带 body,
      // 旧实现直接置 null,导致 interpStatus 把 null 当作"未采到数据"判 ok,
      // 状态条被吞掉,用户看不到任何提示。
      // 这里构造 IsValid=false + warning 的占位结果,让 UI 三态分类能正确识别:
      //   - hasLoadedInterpolator=false → prb-missing(橙色,可点击跳配置)
      //   - hasLoadedInterpolator=true  → invalid(红色,tooltip 显示后端 error)
      // 数值字段全部为 0,与后端 PrbMissing/Invalid 路径零值契约一致,
      // 前端模板在 isValid=false 时已强制显示 '--' 不会读这些零值。
      realtimeResult.value = {
        isValid: false,
        warning: res.error ?? '',
        alpha: 0,
        beta: 0,
        machNumber: 0,
        velocity: 0,
        dynamicPressure: 0,
      } as InterpolationResult
    }
  }

  /**
   * 同步实时插值输入（节流入口）
   * 基于 uiRefreshIntervalMs 节流（派生自全局 storageStore.settings.refreshRateHz），
   * 避免高频输入压垮后端；同时与实时压力数据同频，避免视觉错位
   * 若距离上次计算未达到节流间隔，则暂存输入并设置定时器；否则立即计算
   */
  function syncRealtimeInterpolation(
    input: TraversalRealtimeInput | null,
    configOverride: TraversalTestConfig | null = null
  ): void {
    // 后端插值器已加载即可进行实时插值，无需等待配置保存
    // （importPrb/importCalibrationCsv/importMultiPrb 成功后后端已 SetInterpolator）
    const hasInterpolationDataset = hasLoadedInterpolator.value
    const effectiveConfig = configOverride ?? config.value

    if (!input || !hasInterpolationDataset) {
      pendingRealtimeInterpolationInput = null
      pendingRealtimeInterpolationConfig = null
      clearRealtimeInterpolationTimer()
      realtimeInterpolationRequestId += 1
      realtimeResult.value = null
      return
    }

    pendingRealtimeInterpolationInput = input
    pendingRealtimeInterpolationConfig = effectiveConfig

    const now = Date.now()
    const elapsed = now - lastRealtimeInterpolationAt
    const delay = Math.max(0, uiRefreshIntervalMs.value - elapsed)

    if (delay === 0) {
      void runRealtimeInterpolation()
      return
    }

    if (!realtimeInterpolationTimer) {
      realtimeInterpolationTimer = setTimeout(() => {
        void runRealtimeInterpolation()
      }, delay)
    }
  }

  // 兼容旧接口：直接请求实时插值（内部走节流）
  async function requestRealtimeResult(
    input: TraversalRealtimeInput,
    configOverride?: TraversalTestConfig
  ): Promise<void> {
    syncRealtimeInterpolation(input, configOverride ?? null)
  }

  function shouldIgnoreTaskEvent(taskId: string): boolean {
    const previousStatus = status.value
    if (previousStatus) {
      return previousStatus.taskId !== taskId
    }

    if (startupPendingTaskId) {
      return startupPendingTaskId !== taskId
    }

    if (startupBlockedTaskId) {
      return startupBlockedTaskId === taskId
    }

    return false
  }

  function teardownEventSubscriptions(): void {
    if (unsubscribeProgress) {
      unsubscribeProgress()
      unsubscribeProgress = null
    }

    if (unsubscribeComplete) {
      unsubscribeComplete()
      unsubscribeComplete = null
    }

    if (unsubscribeError) {
      unsubscribeError()
      unsubscribeError = null
    }
  }

  function setupEventSubscriptions(): void {
    teardownEventSubscriptions()

    unsubscribeProgress = traversalApi.onProgress((event: TraversalProgressEvent) => {
      applyProgressEvent(event)
    })

    unsubscribeComplete = traversalApi.onComplete((event: TraversalCompleteEvent) => {
      applyCompleteEvent(event)
    })

    unsubscribeError = traversalApi.onError((event: TraversalErrorEvent) => {
      applyErrorEvent(event)
    })
  }

  function hasEventSubscriptions(): boolean {
    return unsubscribeProgress !== null && unsubscribeComplete !== null && unsubscribeError !== null
  }

  // 将异常原因转换为可读错误消息
  function toErrorMessage(reason: unknown, fallback: string): string {
    if (reason instanceof Error && reason.message) {
      return reason.message
    }

    if (typeof reason === 'string' && reason) {
      return reason
    }

    return fallback
  }

  /**
   * 同步恢复的状态
   * 与 Cursor DAQ 一致：若新状态是活跃状态且 taskId 与前序一致，则保留前序的
   * currentPoint/currentPointPhase/validationWarnings，避免轮询间隙的状态闪烁
   */
  function syncRecoveredStatus(nextStatus: TraversalTestStatus | null): void {
    if (nextStatus) {
      clearStartupWindow()
    }

    const previousStatus = status.value
    status.value = nextStatus
      && previousStatus?.taskId === nextStatus.taskId
      && isActiveStatus(nextStatus.status)
      ? {
          ...nextStatus,
          currentPoint: nextStatus.currentPoint ?? previousStatus.currentPoint,
          currentPointPhase: nextStatus.currentPointPhase ?? previousStatus.currentPointPhase,
          validationWarnings: nextStatus.validationWarnings ?? previousStatus.validationWarnings,
          warning: nextStatus.warning ?? previousStatus.warning
        }
      : nextStatus

    if (nextStatus?.dataPoints) {
      dataPoints.value = nextStatus.dataPoints
      const latest = nextStatus.dataPoints[nextStatus.dataPoints.length - 1]
      if (latest) {
        realtimePressures.value = latest.rawPressure
        realtimeResult.value = latest.interpolationResult
      }
    }

    if (!nextStatus) {
      teardownEventSubscriptions()
      completeEvent.value = null
      error.value = null
      return
    }

    if (nextStatus.status === 'running' || nextStatus.status === 'paused') {
      if (!hasEventSubscriptions()) {
        setupEventSubscriptions()
      }
      completeEvent.value = null
      error.value = null
      return
    }

    if (!isTerminalStatus(nextStatus.status)) {
      completeEvent.value = null
    }

    error.value = nextStatus.status === 'error'
      ? (nextStatus.lastError ?? error.value ?? null)
      : null
  }

  function shouldIgnoreProgressEvent(event: TraversalProgressEvent): boolean {
    if (shouldIgnoreTaskEvent(event.taskId)) {
      return true
    }

    const previousStatus = status.value
    if (!previousStatus) {
      return false
    }

    return isTerminalStatus(previousStatus.status)
  }

  function shouldIgnoreCompleteEvent(event: TraversalCompleteEvent): boolean {
    return shouldIgnoreTaskEvent(event.taskId)
  }

  function shouldIgnoreErrorEvent(event: TraversalErrorEvent): boolean {
    if (shouldIgnoreTaskEvent(event.taskId)) {
      return true
    }

    const previousStatus = status.value
    if (!previousStatus) {
      return false
    }

    return isTerminalStatus(previousStatus.status)
  }

  function applyProgressEvent(event: TraversalProgressEvent): void {
    if (shouldIgnoreProgressEvent(event)) {
      return
    }

    const previousStatus = status.value
    status.value = {
      taskId: previousStatus?.taskId ?? event.taskId,
      state: previousStatus?.state ?? 'running',
      status: previousStatus?.status === 'paused' ? 'paused' : 'running',
      totalPoints: event.totalPoints,
      completedPoints: event.completedPoints,
      currentPoint: event.currentPoint,
      currentPointPhase: event.currentPointPhase,
      progress: event.totalPoints > 0 ? (event.completedPoints / event.totalPoints) * 100 : 0,
      startTime: previousStatus?.startTime,
      estimatedRemaining: previousStatus?.estimatedRemaining,
      lastError: previousStatus?.lastError,
      lastErrorCode: previousStatus?.lastErrorCode,
      validationWarnings: previousStatus?.validationWarnings,
      warning: previousStatus?.warning,
      // 保留后端写入的实际 CSV 路径：progress 事件不携带此字段，
      // 必须从 previousStatus 透传，避免轮询刷新后丢失真实文件名（撞名 -2/-3 后缀）
      csvPath: previousStatus?.csvPath
    }

    if (!previousStatus) {
      clearStartupWindow()
    }

    if (event.latestData) {
      dataPoints.value.push(event.latestData)
      realtimePressures.value = event.latestData.rawPressure
      realtimeResult.value = event.latestData.interpolationResult
    }
  }

  function applyErrorEvent(event: TraversalErrorEvent): void {
    if (shouldIgnoreErrorEvent(event)) {
      return
    }

    const previousStatus = status.value
    status.value = {
      taskId: previousStatus?.taskId ?? event.taskId,
      state: 'error',
      status: 'error',
      totalPoints: previousStatus?.totalPoints ?? 0,
      completedPoints: previousStatus?.completedPoints ?? 0,
      // 出错后清除当前点，避免残留的 currentPoint 导致颜色显示异常
      currentPoint: undefined,
      currentPointPhase: undefined,
      progress: previousStatus?.progress ?? 0,
      startTime: previousStatus?.startTime,
      estimatedRemaining: previousStatus?.estimatedRemaining,
      lastError: event.error,
      lastErrorCode: event.code as TraversalErrorCode,
      // 保留实际 CSV 路径：错误事件不携带此字段，必须从 previousStatus 透传，
      // 否则侧边栏会回退到 config 静态拼接的预期路径（撞名 -2/-3 时为错误路径）
      csvPath: previousStatus?.csvPath
    }

    if (!previousStatus) {
      clearStartupWindow()
    }

    error.value = event.error
  }

  function applyCompleteEvent(event: TraversalCompleteEvent): void {
    if (shouldIgnoreCompleteEvent(event)) {
      return
    }

    const previousStatus = status.value
    const completionStatus = event.status
    completeEvent.value = event
    status.value = {
      taskId: previousStatus?.taskId ?? event.taskId,
      state: completionStatus,
      status: completionStatus,
      totalPoints: event.totalPoints,
      completedPoints: completionStatus === 'completed' ? event.totalPoints : (previousStatus?.completedPoints ?? 0),
      // 测试结束后清除当前点，避免残留的 currentPoint 导致颜色显示异常
      currentPoint: undefined,
      currentPointPhase: undefined,
      progress: completionStatus === 'completed' ? 100 : (previousStatus?.progress ?? 0),
      startTime: previousStatus?.startTime,
      estimatedRemaining: previousStatus?.estimatedRemaining,
      lastError: completionStatus === 'completed' ? undefined : (event.error ?? previousStatus?.lastError),
      lastErrorCode: completionStatus === 'completed' ? undefined : previousStatus?.lastErrorCode,
      // 保留实际 CSV 路径：完成事件 filePath 在轮询路径下未填充，必须从 previousStatus 透传，
      // 否则侧边栏会回退到 config 静态拼接的预期路径（撞名 -2/-3 时为错误路径）
      csvPath: previousStatus?.csvPath
    }

    if (!previousStatus) {
      clearStartupWindow()
    }

    error.value = completionStatus === 'error'
      ? (event.error ?? previousStatus?.lastError ?? null)
      : null

    teardownEventSubscriptions()
  }

  async function loadConfig(): Promise<void> {
    const res = await traversalApi.getConfig()
    if (res.success) {
      const loadedConfig = res.data ?? null
      config.value = loadedConfig
        ? { ...loadedConfig, layout: normalizeTraversalLayoutRanges(loadedConfig.layout) }
        : null
      inferInterpolatorState()
      // 启动恢复时通过后端 API 校验插值器实际加载状态，
      // 避免 PRB 文件被删除/移动后前端误判为已加载（导致实时插值静默失败）
      await verifyInterpolatorWithBackend()
    }
  }

  /**
   * 恢复渲染进程状态
   * 与 Cursor DAQ 一致：使用 Promise.allSettled 容错，单个请求失败不影响另一个
   */
  async function recoverRendererState(): Promise<void> {
    const requestId = beginRecoveryRequest()
    error.value = null
    statusRecoveryFailed.value = false

    const [configResult, statusResult] = await Promise.allSettled([
      traversalApi.getConfig(),
      traversalApi.getStatus()
    ])

    if (!isActiveRecoveryRequest(requestId)) {
      return
    }

    const recoveryErrors: string[] = []

    if (configResult.status === 'fulfilled') {
      if (configResult.value.success) {
        config.value = configResult.value.data ?? null
        inferInterpolatorState()
        // 启动恢复时通过后端 API 校验插值器实际加载状态
        await verifyInterpolatorWithBackend()
      } else {
        config.value = null
        recoveryErrors.push(configResult.value.error || i18n.t.travErrLoadConfig)
      }
    } else {
      config.value = null
      recoveryErrors.push(toErrorMessage(configResult.reason, i18n.t.travErrLoadConfig))
    }

    if (statusResult.status === 'fulfilled') {
      if (statusResult.value.success) {
        statusRecoveryFailed.value = false
        syncRecoveredStatus(statusResult.value.data ?? null)
      } else {
        statusRecoveryFailed.value = true
        syncRecoveredStatus(null)
        recoveryErrors.push(statusResult.value.error || i18n.t.travErrGetStatus)
      }
    } else {
      statusRecoveryFailed.value = true
      syncRecoveredStatus(null)
      recoveryErrors.push(toErrorMessage(statusResult.reason, i18n.t.travErrGetStatus))
    }

    if (recoveryErrors.length > 0) {
      error.value = recoveryErrors.join('；')
    }
  }

  async function saveConfig(cfg: TraversalTestConfig): Promise<boolean> {
    const res = await traversalApi.saveConfig(cfg)
    if (res.success) {
      config.value = cfg
      // 修复 Bug 1: 保存配置（通常意味着用户重新布局/调整了点位）后，
      // 若当前非运行/暂停态，清空上一轮测试残留的已完成点状态、完成事件与数据点，
      // 让布点预览画面回到"未测试"状态（紫色完成点恢复为灰色未完成点）。
      // 运行/暂停态下不清空，避免抹掉正在进行的测试进度。
      // 同步清空 statusRecoveryFailed：status=null + 残留 statusRecoveryFailed=true
      // 会让 statusType 退化为 'unknown'，canStart 误判为 false 阻塞下一次启动。
      // 同步清空 realtimePressures/realtimeResult：与 start/reset 路径一致，
      // 避免新一轮测试实时监控画面短暂闪现上一轮最后一帧的值，直到新帧到达才覆盖。
      if (!isRunning.value && !isPaused.value) {
        status.value = null
        completeEvent.value = null
        dataPoints.value = []
        statusRecoveryFailed.value = false
        realtimePressures.value = null
        realtimeResult.value = null
      }
      return true
    }
    error.value = res.error || i18n.t.travErrSaveConfig
    return false
  }

  async function importPrbFile(filePath: string): Promise<PrbFileInfo | null> {
    const res = await traversalApi.importPrb(filePath)
    if (res.success && res.data) {
      hasLoadedInterpolator.value = true
      if (config.value) {
        config.value.prbFile = res.data
      }
      return res.data
    }
    error.value = res.error || i18n.t.travErrImportPrb
    return null
  }

  async function importCalibrationCsvFile(filePath: string): Promise<CalibrationCsvFileInfo | null> {
    const res = await traversalApi.importCalibrationCsv(filePath)
    if (res.success && res.data) {
      hasLoadedInterpolator.value = true
      if (config.value) {
        config.value.calibrationCsvFile = res.data
      }
      return res.data
    }
    error.value = res.error || i18n.t.travErrImportCsv
    return null
  }

  function setInterpolationAlgorithm(algorithm: InterpolationAlgorithm): void {
    if (config.value) {
      config.value.interpolationAlgorithm = algorithm
    }
  }

  async function importMultiPrbFiles(
    filePaths: string[],
    machNumbers?: number[],
    interpolationMode: MultiPrbInterpolationMode = 'linear'
  ): Promise<{ files: PrbFileInfo[]; machNumbers: number[]; warnings: string[] } | null> {
    const res = await traversalApi.importMultiPrb(filePaths, machNumbers, interpolationMode)
    if (res.success && res.data) {
      hasLoadedInterpolator.value = true
      if (config.value) {
        config.value.prbFile = null
        config.value.useMultiPrb = true
        config.value.multiPrb = {
          files: res.data.files,
          machNumbers: res.data.machNumbers,
          interpolationMode
        }
      }
      return res.data
    }

    error.value = res.error || i18n.t.travErrImportMultiPrb
    return null
  }

  /**
   * 七孔文件集导入的公共处理：成功后置已加载、按响应逐文件信息回填 sevenHolePrb 配置。
   */
  function applySevenHoleImport(
    kind: SevenHoleTraversalInterpolationConfig['kind'],
    data: { files: SevenHolePrbFileInfo[]; validRange: PrbFileInfo['validRange'] }
  ): void {
    hasLoadedInterpolator.value = true
    if (config.value) {
      const files = data.files
      const inner = files.find((f) => f.sector === 7)
      const outer = [1, 2, 3, 4, 5, 6].map((n) => files.find((f) => f.sector === n))
      if (inner && outer.every((f): f is SevenHolePrbFileInfo => f != null)) {
        config.value.sevenHolePrb = {
          kind,
          innerFile: inner,
          outerFiles: outer as SevenHoleTraversalInterpolationConfig['outerFiles']
        }
      }
    }
  }

  /**
   * 导入七孔 PRB 文件集（1 内区 + 6 扇区，spec §5.6）。
   * 成功后：插值器已加载、sevenHolePrb 配置按响应逐文件信息回填；
   * 返回完整响应（files + validRange）供向导展示。
   */
  async function importSevenHolePrbFiles(
    innerFilePath: string,
    outerFilePaths: string[]
  ): Promise<{ files: SevenHolePrbFileInfo[]; validRange: PrbFileInfo['validRange'] } | null> {
    const res = await traversalApi.importSevenHolePrb(innerFilePath, outerFilePaths)
    if (res.success && res.data) {
      applySevenHoleImport('seven-hole-prb-set', res.data)
      return res.data
    }
    error.value = res.error || i18n.t.travErrImportSevenHolePrb
    return null
  }

  /**
   * 导入七孔校准 CSV 文件集（校准导出直接导入，免导出 .prb；spec §10 Q2 落地）。
   * 与 PRB 文件集同槽位语义（1 内区 + 6 扇区），kind 记录为 calibration-csv 供恢复。
   */
  async function importSevenHoleCalibrationCsvFiles(
    innerFilePath: string,
    outerFilePaths: string[]
  ): Promise<{ files: SevenHolePrbFileInfo[]; validRange: PrbFileInfo['validRange'] } | null> {
    const res = await traversalApi.importSevenHoleCalibrationCsv(innerFilePath, outerFilePaths)
    if (res.success && res.data) {
      applySevenHoleImport('seven-hole-calibration-csv', res.data)
      return res.data
    }
    error.value = res.error || i18n.t.travErrImportSevenHoleCsv
    return null
  }

  /**
   * 异步清理指定探针类型的插值器（先清后端再改本地；后端失败不动本地状态）。
   * 返回是否成功；失败时 error 已填充。探针切换不清插值器（双变体语义），
   * 本动作保留给"显式移除校准文件"类场景使用。
   */
  async function clearProbeInterpolator(probeType: TraversalProbeType): Promise<boolean> {
    const res = await traversalApi.clearInterpolator(probeType)
    if (!res.success) {
      error.value = res.error || i18n.t.travErrClearInterpolator
      return false
    }
    return true
  }

  /**
   * 切换激活探针类型（双变体语义）：
   * 仅更新 config.probeType 并按激活变体重新推导 hasLoadedInterpolator，
   * 不清理后端任何插值器、不重置任何变体字段——五孔字段与 sevenHolePrb
   * 在配置中并存，后端按激活 probeType 经策略表取用对应插值器，
   * 未激活插值器保持挂载但对计算/前置检查不可达（traversal_probe.go）。
   * 切换后立即经 checkPreconditions 复核后端真实加载状态（应用重启只恢复
   * 激活变体，切到未恢复侧时由 verify 纠正推断并展示根因）。
   */
  function activateProbeType(next: TraversalProbeType): void {
    if (config.value) {
      config.value.probeType = next
    }
    inferInterpolatorState()
    void verifyInterpolatorWithBackend()
  }

  async function checkPreconditions(cfg?: TraversalTestConfig): Promise<PreconditionCheckResult> {
    const res = await traversalApi.checkPreconditions(cfg ? toSerializableConfig(cfg) : undefined)
    return res.success ? (res.data ?? { allPassed: false, checks: [] }) : { allPassed: false, checks: [] }
  }

  /**
   * 启动测试
   * 与 Cursor DAQ 一致：使用 isStarting 防重入，避免用户连续点击导致并发启动
   */
  async function startTest(cfg: TraversalTestConfig): Promise<string> {
    if (isStarting.value) {
      throw new Error(i18n.t.travErrTestStarting)
    }

    isStarting.value = true
    try {
      beginStartupWindow(status.value?.taskId ?? completeEvent.value?.taskId ?? null)
      error.value = null
      statusRecoveryFailed.value = false
      status.value = null
      dataPoints.value = []
      completeEvent.value = null
      realtimePressures.value = null
      realtimeResult.value = null

      setupEventSubscriptions()

      const startRes = await traversalApi.start(toSerializableConfig(cfg))
      if (!startRes.success || !startRes.data?.taskId) {
        throw new Error(startRes.error || i18n.t.travErrStartTest)
      }

      if (!status.value) {
        setStartupPendingTask(startRes.data.taskId)
      } else {
        clearStartupWindow()
      }

      await refreshStatus()
      // 后端 ParseAndStartTraversal 已隐式启动设备采集，前端 deviceStatuses 需同步刷新，
      // 否则侧边栏 acquiringFor 返回 false，采集状态指示灯停在"已连接"。
      // 不阻塞启动主流程：refreshAllStatuses 内部 catch 所有错误，最坏情况是指示灯延迟更新，
      // 不影响测试启动成功语义。
      deviceStore.refreshAllStatuses().catch((err) => {
        console.warn('[traversalStore] startTest: refreshAllStatuses failed:', err)
      })
      return startRes.data.taskId
    } catch (err) {
      clearStartupWindow()
      teardownEventSubscriptions()
      error.value = err instanceof Error ? err.message : String(err)
      throw err
    } finally {
      isStarting.value = false
    }
  }

  async function pause(): Promise<void> {
    const res = await traversalApi.pause()
    if (!res.success) throw new Error(res.error || i18n.t.travErrPause)
    await refreshStatus()
  }

  async function resume(): Promise<void> {
    const res = await traversalApi.resume()
    if (!res.success) throw new Error(res.error || i18n.t.travErrResume)
    // 乐观更新：立即将状态切换为 running，避免轮询间隙的 UI 闪烁
    if (status.value && status.value.status === 'paused') {
      status.value = { ...status.value, status: 'running' }
    }
    // 与 pause() 保持一致：刷新后端状态，确保乐观更新与实际状态同步
    await refreshStatus()
  }

  async function stop(): Promise<void> {
    const res = await traversalApi.stop()
    if (!res.success) throw new Error(res.error || i18n.t.travErrStop)
    await refreshStatus()
    // 后端 stop 会隐式停止设备采集（若由遍历启动），前端需同步刷新，
    // 否则侧边栏 acquiringFor 仍返回 true，指示灯停在"采集中"
    deviceStore.refreshAllStatuses().catch((err) => {
      console.warn('[traversalStore] stop: refreshAllStatuses failed:', err)
    })
  }

  async function refreshStatus(): Promise<void> {
    const res = await traversalApi.getStatus()
    if (res.success) {
      statusRecoveryFailed.value = false
      syncRecoveredStatus(res.data ?? null)
    }
  }

  /** 加载断点恢复信息（应用启动或进入遍历页面时调用） */
  async function loadCheckpoint(): Promise<TraversalCheckpoint | null> {
    const res = await traversalApi.loadCheckpoint()
    if (res.success) {
      checkpoint.value = res.data ?? null
      return checkpoint.value
    }
    checkpoint.value = null
    return null
  }

  /** 从断点恢复测试（复用原 taskId，从已完成点数继续） */
  async function resumeFromCheckpoint(cp: TraversalCheckpoint): Promise<string> {
    if (isStarting.value) {
      throw new Error(i18n.t.travErrTestStarting)
    }

    isStarting.value = true
    try {
      error.value = null
      statusRecoveryFailed.value = false
      status.value = null
      dataPoints.value = []
      completeEvent.value = null
      realtimePressures.value = null
      realtimeResult.value = null

      setupEventSubscriptions()

      const res = await traversalApi.resumeFromCheckpoint(cp)
      if (!res.success || !res.data?.taskId) {
        teardownEventSubscriptions()
        throw new Error(res.error || i18n.t.travErrResumeCheckpoint)
      }

      // 恢复后清空 checkpoint 缓存（后端会在测试完成时自动清理断点文件）
      checkpoint.value = null
      await refreshStatus()
      return res.data.taskId
    } catch (err) {
      teardownEventSubscriptions()
      error.value = err instanceof Error ? err.message : String(err)
      throw err
    } finally {
      isStarting.value = false
    }
  }

  /** 清除断点文件（用户主动放弃恢复时调用） */
  async function clearCheckpoint(): Promise<void> {
    await traversalApi.clearCheckpoint()
    checkpoint.value = null
  }

  function clearError(): void {
    error.value = null
  }

  // 清除插值器状态（移除文件/切换算法时调用）
  function clearInterpolator(): void {
    hasLoadedInterpolator.value = false
    realtimeResult.value = null
  }

  // 根据已加载的配置推断后端插值器是否已就绪（按探针类型判别）
  function inferInterpolatorState(): void {
    const cfg = config.value
    if (!cfg) {
      hasLoadedInterpolator.value = false
      return
    }
    if ((cfg.probeType ?? 'five-hole') === 'seven-hole') {
      const prb = cfg.sevenHolePrb
      const knownKind = prb?.kind === 'seven-hole-prb-set' || prb?.kind === 'seven-hole-calibration-csv'
      hasLoadedInterpolator.value = !!(
        knownKind &&
        prb?.innerFile?.filePath &&
        prb.outerFiles?.length === 6 &&
        prb.outerFiles.every((f) => !!f?.filePath)
      )
      return
    }
    hasLoadedInterpolator.value = !!(
      cfg.prbFile?.filePath ||
      cfg.calibrationCsvFile?.filePath ||
      (cfg.useMultiPrb && cfg.multiPrb?.files?.length)
    )
  }

  /**
   * 通过后端 checkPreconditions API 校验插值器是否真正加载
   * 仅在推断为已加载时调用，防止 PRB 文件被删除/移动后前端误判
   *
   * 实现要点：
   * - 显式区分 API 成功/失败：API 调用失败（success=false）或抛错时保留推断状态，
   *   避免网络抖动导致误判；同时把"无法校验"提示写入 interpolatorRestoreMessage 让 UI 可见。
   * - 复用 PreconditionCheckResult 类型推断，不写匿名内联类型，
   *   后端字段变更时可在编译期感知。
   * - 后端 message 字段（CheckPreconditions 在 PRB 失败时已经把根因写入）会被回填到
   *   interpolatorRestoreMessage，由 UI 层展示给用户。
   * - 显式传入当前 config：双变体恢复下 activateProbeType 仅修改前端 config.probeType，
   *   后端 m.config.ProbeType 在保存前是旧值；后端 CheckPreconditions 已支持按请求
   *   probeType 判定，必须显式传入否则会按旧类型查 PRB 误报"未加载"。
   */
  async function verifyInterpolatorWithBackend(): Promise<void> {
    if (!hasLoadedInterpolator.value) {
      interpolatorRestoreMessage.value = null
      return
    }
    // 传入当前 config 副本，让后端按激活类型（而非陈旧的 m.config.ProbeType）判定 PRB 加载状态
    const payload = config.value ? toSerializableConfig(config.value) : undefined
    try {
      const res = await traversalApi.checkPreconditions(payload)
      // API 调用未成功（如网络异常）：保留推断状态，但提示用户校验未完成
      if (!res.success || !res.data) {
        interpolatorRestoreMessage.value =
          i18n.t.travErrVerifyInterpolator.replace('{error}', res.error || i18n.t.travErrResponseEmpty)
        return
      }
      const result: PreconditionCheckResult = res.data
      const prbCheck = result.checks.find((c) => c.name === 'PRB')
      if (prbCheck && !prbCheck.passed) {
        hasLoadedInterpolator.value = false
        // 把后端 message 透传给 UI，便于用户看到根本原因（如"PRB 文件不存在"）
        interpolatorRestoreMessage.value = prbCheck.message || i18n.t.travErrInterpolatorNotLoaded
      } else {
        interpolatorRestoreMessage.value = null
      }
    } catch (err) {
      // 抛错时不改变推断状态（避免网络抖动误判），但通过 message 通知 UI
      interpolatorRestoreMessage.value =
        i18n.t.travErrVerifyInterpolatorException.replace('{error}', err instanceof Error ? err.message : String(err))
    }
  }

  function reset(): void {
    cancelRecovery()
    statusRecoveryFailed.value = false
    isStarting.value = false
    isSimulation.value = false
    clearStartupWindow()
    clearRealtimeInterpolationTimer()
    pendingRealtimeInterpolationInput = null
    pendingRealtimeInterpolationConfig = null
    lastRealtimeInterpolationAt = 0
    realtimeInterpolationRequestId += 1
    status.value = null
    dataPoints.value = []
    completeEvent.value = null
    error.value = null
    realtimePressures.value = null
    realtimeResult.value = null
    hasLoadedInterpolator.value = false
    checkpoint.value = null

    teardownEventSubscriptions()
  }

  return {
    status,
    isStarting,
    statusType,
    isRunning,
    isPaused,
    isTerminal,
    canStart,
    canPause,
    canResume,
    canStop,
    progress,
    dataPoints,
    realtimePressures,
    realtimeResult,
    hasLoadedInterpolator,
    interpolatorRestoreMessage,
    clearInterpolator,
    config,
    completeEvent,
    error,
    isSimulation,
    checkpoint,
    hasCheckpoint,
    uiRefreshIntervalMs,
    loadConfig,
    recoverRendererState,
    cancelRecovery,
    saveConfig,
    importPrbFile,
    importCalibrationCsvFile,
    setInterpolationAlgorithm,
    importMultiPrbFiles,
    importSevenHolePrbFiles,
    importSevenHoleCalibrationCsvFiles,
    clearProbeInterpolator,
    activateProbeType,
    checkPreconditions,
    startTest,
    pause,
    resume,
    stop,
    refreshStatus,
    loadCheckpoint,
    resumeFromCheckpoint,
    clearCheckpoint,
    clearError,
    reset,
    syncRealtimeInterpolation,
    requestRealtimeResult
  }
})

export { formatApiError }
