import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { traversalApi } from '@api/traversalApi'

import { useUiRefreshThrottle } from '@composables/useUiRefreshThrottle'

function formatApiError(err: unknown): string {
  const msg = err instanceof Error ? err.message : String(err)
  if (msg.includes('Failed to fetch') || msg.includes('NetworkError')) {
    return '网络连接失败，请检查后端服务是否已启动'
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
  TraversalErrorCode
} from '@shared/types/traversal'

export type RealtimePressures = TraversalRawPressure

export const useTraversalStore = defineStore('traversal', () => {
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

  const { uiRefreshHz, uiRefreshIntervalMs, setUiRefreshHz } = useUiRefreshThrottle('traversal.uiRefreshHz')

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
  // 实时插值节流相关状态（与 Cursor DAQ 一致：基于定时器的节流，避免高频刷新压垮 UI）
  let realtimeInterpolationTimer: ReturnType<typeof setTimeout> | null = null
  let pendingRealtimeInterpolationInput: TraversalInterpolationInput | null = null
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
      configOverride ? toSerializableConfig(configOverride) : undefined
    )
    if (requestId !== realtimeInterpolationRequestId) {
      return
    }

    realtimeResult.value = res.success ? (res.data ?? null) : null
  }

  /**
   * 同步实时插值输入（节流入口）
   * 与 Cursor DAQ 一致：基于 uiRefreshIntervalMs 节流，避免高频输入压垮后端
   * 若距离上次计算未达到节流间隔，则暂存输入并设置定时器；否则立即计算
   */
  function syncRealtimeInterpolation(
    input: TraversalInterpolationInput | null,
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
    input: TraversalInterpolationInput,
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
          validationWarnings: nextStatus.validationWarnings ?? previousStatus.validationWarnings
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
      validationWarnings: previousStatus?.validationWarnings
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
      lastErrorCode: event.code as TraversalErrorCode
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
      lastErrorCode: completionStatus === 'completed' ? undefined : previousStatus?.lastErrorCode
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
      config.value = res.data ?? null
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
        recoveryErrors.push(configResult.value.error || '加载移位测试配置失败')
      }
    } else {
      config.value = null
      recoveryErrors.push(toErrorMessage(configResult.reason, '加载移位测试配置失败'))
    }

    if (statusResult.status === 'fulfilled') {
      if (statusResult.value.success) {
        statusRecoveryFailed.value = false
        syncRecoveredStatus(statusResult.value.data ?? null)
      } else {
        statusRecoveryFailed.value = true
        syncRecoveredStatus(null)
        recoveryErrors.push(statusResult.value.error || '获取移位测试状态失败')
      }
    } else {
      statusRecoveryFailed.value = true
      syncRecoveredStatus(null)
      recoveryErrors.push(toErrorMessage(statusResult.reason, '获取移位测试状态失败'))
    }

    if (recoveryErrors.length > 0) {
      error.value = recoveryErrors.join('；')
    }
  }

  async function saveConfig(cfg: TraversalTestConfig): Promise<boolean> {
    const res = await traversalApi.saveConfig(cfg)
    if (res.success) {
      config.value = cfg
      return true
    }
    error.value = res.error || '保存配置失败'
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
    error.value = res.error || '导入 PRB 文件失败'
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
    error.value = res.error || '导入 CSV 标定数据失败'
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

    error.value = res.error || '导入多 PRB 文件失败'
    return null
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
      throw new Error('测试正在启动')
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
        throw new Error(startRes.error || '启动测试失败')
      }

      if (!status.value) {
        setStartupPendingTask(startRes.data.taskId)
      } else {
        clearStartupWindow()
      }

      await refreshStatus()
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
    if (!res.success) throw new Error(res.error || '暂停失败')
    await refreshStatus()
  }

  async function resume(): Promise<void> {
    const res = await traversalApi.resume()
    if (!res.success) throw new Error(res.error || '继续失败')
    // 乐观更新：立即将状态切换为 running，避免轮询间隙的 UI 闪烁
    if (status.value && status.value.status === 'paused') {
      status.value = { ...status.value, status: 'running' }
    }
    // 与 pause() 保持一致：刷新后端状态，确保乐观更新与实际状态同步
    await refreshStatus()
  }

  async function stop(): Promise<void> {
    const res = await traversalApi.stop()
    if (!res.success) throw new Error(res.error || '停止失败')
    await refreshStatus()
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
      throw new Error('测试正在启动')
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
        throw new Error(res.error || '从断点恢复测试失败')
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

  // 根据已加载的配置推断后端插值器是否已就绪
  function inferInterpolatorState(): void {
    const cfg = config.value
    if (!cfg) {
      hasLoadedInterpolator.value = false
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
   */
  async function verifyInterpolatorWithBackend(): Promise<void> {
    if (!hasLoadedInterpolator.value) {
      interpolatorRestoreMessage.value = null
      return
    }
    try {
      const res = await traversalApi.checkPreconditions()
      // API 调用未成功（如网络异常）：保留推断状态，但提示用户校验未完成
      if (!res.success || !res.data) {
        interpolatorRestoreMessage.value =
          '无法校验后端插值器状态（' + (res.error || '响应为空') + '）。如导入后未真实加载，请重新导入 PRB / CSV 文件。'
        return
      }
      const result: PreconditionCheckResult = res.data
      const prbCheck = result.checks.find((c) => c.name === 'PRB')
      if (prbCheck && !prbCheck.passed) {
        hasLoadedInterpolator.value = false
        // 把后端 message 透传给 UI，便于用户看到根本原因（如"PRB 文件不存在"）
        interpolatorRestoreMessage.value = prbCheck.message || '后端未加载插值器，请重新导入 PRB / CSV 文件'
      } else {
        interpolatorRestoreMessage.value = null
      }
    } catch (err) {
      // 抛错时不改变推断状态（避免网络抖动误判），但通过 message 通知 UI
      interpolatorRestoreMessage.value =
        '校验后端插值器状态时出错：' + (err instanceof Error ? err.message : String(err))
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
    uiRefreshHz,
    uiRefreshIntervalMs,
    loadConfig,
    recoverRendererState,
    cancelRecovery,
    saveConfig,
    importPrbFile,
    importCalibrationCsvFile,
    setInterpolationAlgorithm,
    importMultiPrbFiles,
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
    setUiRefreshHz,
    syncRealtimeInterpolation,
    requestRealtimeResult
  }
})

export { formatApiError }
