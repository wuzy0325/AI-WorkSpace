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
  InterpolationAlgorithm
} from '@shared/types/traversal'

export type RealtimePressures = TraversalRawPressure

export const useTraversalStore = defineStore('traversal', () => {
  const statusRecoveryFailed = ref(false)
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
  const canStart = computed(() => statusType.value !== 'unknown' && (statusType.value === 'idle' || isTerminal.value))
  const canPause = computed(() => statusType.value === 'running')
  const canResume = computed(() => statusType.value === 'paused')
  const canStop = computed(() => statusType.value === 'running' || statusType.value === 'paused')
  const progress = computed(() => status.value?.progress ?? 0)
  const dataPoints = ref<TraversalDataPoint[]>([])

  const realtimePressures = ref<RealtimePressures | null>(null)
  const realtimeResult = ref<InterpolationResult | null>(null)

  const config = ref<TraversalTestConfig | null>(null)

  const completeEvent = ref<TraversalCompleteEvent | null>(null)

  const error = ref<string | null>(null)

  const { uiRefreshHz, uiRefreshIntervalMs, setUiRefreshHz } = useUiRefreshThrottle('traversal.uiRefreshHz')

  function toSerializableConfig(cfg: TraversalTestConfig): TraversalTestConfig {
    return JSON.parse(JSON.stringify(cfg)) as TraversalTestConfig
  }

  let unsubscribeProgress: (() => void) | null = null
  let unsubscribeComplete: (() => void) | null = null
  let unsubscribeError: (() => void) | null = null
  let recoveryRequestId = 0
  let startupBlockedTaskId: string | null = null
  let startupPendingTaskId: string | null = null
  let realtimeInterpolationRequestId = 0

  function isTerminalStatus(value: TraversalTestStatus['status'] | undefined): value is TraversalTerminalStatus {
    return value === 'completed' || value === 'error' || value === 'stopped'
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

  async function requestRealtimeResult(
    input: TraversalInterpolationInput,
    configOverride?: TraversalTestConfig
  ): Promise<void> {
    const requestId = ++realtimeInterpolationRequestId

    const res = await traversalApi.calculateRealtime(
      input,
      configOverride ? toSerializableConfig(configOverride) : undefined
    )
    if (requestId !== realtimeInterpolationRequestId) {
      return
    }

    realtimeResult.value = res.success ? (res.data ?? null) : null
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

  function syncRecoveredStatus(nextStatus: TraversalTestStatus | null): void {
    if (nextStatus) {
      clearStartupWindow()
    }

    status.value = nextStatus
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
      status: previousStatus?.status === 'paused' ? 'paused' : 'running',
      totalPoints: event.totalPoints,
      completedPoints: event.completedPoints,
      currentPoint: event.currentPoint,
      currentPointPhase: event.currentPointPhase,
      progress: event.totalPoints > 0 ? (event.completedPoints / event.totalPoints) * 100 : 0,
      startTime: previousStatus?.startTime,
      estimatedRemaining: previousStatus?.estimatedRemaining,
      lastError: previousStatus?.lastError
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
      status: 'error',
      totalPoints: previousStatus?.totalPoints ?? 0,
      completedPoints: previousStatus?.completedPoints ?? 0,
      currentPoint: previousStatus?.currentPoint,
      progress: previousStatus?.progress ?? 0,
      startTime: previousStatus?.startTime,
      estimatedRemaining: previousStatus?.estimatedRemaining,
      lastError: event.error
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
      status: completionStatus,
      totalPoints: event.totalPoints,
      completedPoints: completionStatus === 'completed' ? event.totalPoints : (previousStatus?.completedPoints ?? 0),
      currentPoint: previousStatus?.currentPoint,
      currentPointPhase: undefined,
      progress: completionStatus === 'completed' ? 100 : (previousStatus?.progress ?? 0),
      startTime: previousStatus?.startTime,
      estimatedRemaining: previousStatus?.estimatedRemaining,
      lastError: completionStatus === 'completed' ? undefined : (event.error ?? previousStatus?.lastError)
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
    }
  }

  async function recoverRendererState(): Promise<void> {
    const requestId = beginRecoveryRequest()
    error.value = null
    statusRecoveryFailed.value = false

    const [configResult, statusResult] = await Promise.all([
      traversalApi.getConfig(),
      traversalApi.getStatus()
    ])

    if (!isActiveRecoveryRequest(requestId)) {
      return
    }

    const recoveryErrors: string[] = []

    if (configResult.success) {
      config.value = configResult.data ?? null
    } else {
      config.value = null
      recoveryErrors.push(configResult.error || '加载移位测试配置失败')
    }

    if (statusResult.success) {
      statusRecoveryFailed.value = false
      syncRecoveredStatus(statusResult.data ?? null)
    } else {
      statusRecoveryFailed.value = true
      syncRecoveredStatus(null)
      recoveryErrors.push(statusResult.error || 'Failed to get traversal status')
    }

    if (recoveryErrors.length > 0) {
      error.value = recoveryErrors.join('; ')
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

  async function startTest(cfg: TraversalTestConfig): Promise<string> {
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

  function clearError(): void {
    error.value = null
  }

  function reset(): void {
    cancelRecovery()
    statusRecoveryFailed.value = false
    clearStartupWindow()
    realtimeInterpolationRequestId += 1
    status.value = null
    dataPoints.value = []
    completeEvent.value = null
    error.value = null
    realtimePressures.value = null
    realtimeResult.value = null

    teardownEventSubscriptions()
  }

  return {
    status,
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
    config,
    completeEvent,
    error,
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
    clearError,
    reset,
    setUiRefreshHz,
    requestRealtimeResult
  }
})

