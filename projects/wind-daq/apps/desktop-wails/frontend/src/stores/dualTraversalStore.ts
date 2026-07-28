import { computed, reactive } from 'vue'
import { defineStore } from 'pinia'

import { deviceApi } from '@api/deviceApi'
import { traversalProbeApi } from '@api/traversalApi'
import { invalidateProbePolling } from '@api/traversalPolling'
import { useStorageStore } from '@stores/storageStore'
import type {
  CalibrationCsvFileInfo,
  InterpolationResult,
  MultiPrbInterpolationMode,
  PreconditionCheckResult,
  PrbFileInfo,
  ProbeId,
  SevenHolePrbFileInfo,
  TraversalCompleteEvent,
  TraversalProbeType,
  TraversalRealtimeInput,
  TraversalSessionState,
  TraversalTestConfig,
  TraversalTestStatus,
} from '@shared/types/traversal'

/**
 * 双探针 keyed session store（spec FR5 / Task 17）。
 * sessions: Record<ProbeId, TraversalSessionState>，两路完全隔离（一路
 * reset/失败/unmount 不修改另一路）；每个 action 显式接收 ProbeId 首参；
 * 每 probe 独立 requestId / startWindow / subscriptionHandle /
 * realtimeCalcTimer / pendingInput；实时 DAQ 订阅按 deviceId 引用计数
 * （两路共享设备时一路卸载不取消另一路）；既有 traversalStore.ts 不受影响。
 */

// device-level 订阅引用计数（模块级：两路共享同一物理设备时引用叠加）
const deviceRefCounts = new Map<string, number>()

function acquireDevice(deviceId: string): void {
  const count = deviceRefCounts.get(deviceId) ?? 0
  if (count === 0) {
    deviceApi.subscribeToDevice(deviceId)
  }
  deviceRefCounts.set(deviceId, count + 1)
}

function releaseDevice(deviceId: string): void {
  const count = (deviceRefCounts.get(deviceId) ?? 0) - 1
  if (count <= 0) {
    deviceRefCounts.delete(deviceId)
    deviceApi.unsubscribeFromDevice(deviceId)
    return
  }
  deviceRefCounts.set(deviceId, count)
}

/** 测试断言用：某设备的当前订阅引用数。 */
export function dualDeviceRefCount(deviceId: string): number {
  return deviceRefCounts.get(deviceId) ?? 0
}

/** 测试专用：清空全部引用计数（避免用例间泄漏）。 */
export function resetDualDeviceRefCounts(): void {
  deviceRefCounts.clear()
}

// 每 probe 运行期内部状态（非响应式；响应式状态在 TraversalSessionState）
interface SessionRuntime {
  requestId: number
  blockedTaskId: string | null
  pendingTaskId: string | null
  unsubscribers: Array<() => void>
  realtimeTimer: ReturnType<typeof setTimeout> | null
  lastRealtimeAt: number
  pendingInput: TraversalRealtimeInput | null
  subscribedDeviceIds: string[]
}

function emptySession(): TraversalSessionState {
  return {
    config: null,
    status: null,
    isStarting: false,
    error: null,
    completeEvent: null,
    checkpoint: null,
    realtimePressures: null,
    realtimeResult: null,
    hasLoadedInterpolator: false,
    interpolatorRestoreMessage: null,
  }
}

function emptyRuntime(): SessionRuntime {
  return {
    requestId: 0,
    blockedTaskId: null,
    pendingTaskId: null,
    unsubscribers: [],
    realtimeTimer: null,
    lastRealtimeAt: 0,
    pendingInput: null,
    subscribedDeviceIds: [],
  }
}

const REALTIME_THROTTLE_FALLBACK_MS = 200

function uniqueDeviceIds(config: TraversalTestConfig | null): string[] {
  if (!config) return []
  const ids = new Set<string>()
  for (const ch of config.channels?.probeChannels ?? []) {
    if (ch.enabled !== false && ch.channel?.deviceId) ids.add(ch.channel.deviceId)
  }
  return Array.from(ids).sort()
}

function nextRequestId(runtime: SessionRuntime): number {
  runtime.requestId += 1
  return runtime.requestId
}

function isActiveStatus(value: string | undefined): boolean {
  return value === 'running' || value === 'moving' || value === 'stabilizing' ||
    value === 'acquiring' || value === 'saving' || value === 'paused'
}

function inferInterpolatorLoaded(config: TraversalTestConfig | null): boolean {
  if (!config) return false
  if ((config.probeType ?? 'five-hole') === 'seven-hole') {
    const prb = config.sevenHolePrb
    const knownKind = prb?.kind === 'seven-hole-prb-set' || prb?.kind === 'seven-hole-calibration-csv'
    return !!(
      knownKind &&
      prb?.innerFile?.filePath &&
      prb.outerFiles?.length === 6 &&
      prb.outerFiles.every((file) => !!file?.filePath)
    )
  }
  return !!(
    config.prbFile?.filePath ||
    config.calibrationCsvFile?.filePath ||
    (config.useMultiPrb && config.multiPrb?.files?.length)
  )
}

export const useDualTraversalStore = defineStore('dualTraversal', () => {
  const sessions = reactive<Record<ProbeId, TraversalSessionState>>({
    probe1: emptySession(),
    probe2: emptySession(),
  })
  const runtimes: Record<ProbeId, SessionRuntime> = {
    probe1: emptyRuntime(),
    probe2: emptyRuntime(),
  }

  const uiRefreshIntervalMs = computed(() => {
    const store = useStorageStore()
    const hz = store?.settings?.refreshRateHz
    return hz && hz > 0 ? Math.round(1000 / hz) : REALTIME_THROTTLE_FALLBACK_MS
  })

  function sessionOf(probeId: ProbeId): TraversalSessionState {
    return sessions[probeId]
  }

  // 订阅管理（每 probe 独立；停止一路不影响另一路）
  function teardownSubscriptions(probeId: ProbeId): void {
    const runtime = runtimes[probeId]
    for (const unsubscribe of runtime.unsubscribers.splice(0)) {
      unsubscribe()
    }
  }

  function setupSubscriptions(probeId: ProbeId): void {
    teardownSubscriptions(probeId)
    const runtime = runtimes[probeId]
    const session = sessionOf(probeId)
    runtime.unsubscribers.push(
      traversalProbeApi.onStatus(probeId, (status) => {
        session.status = status
      }),
      traversalProbeApi.onProgress(probeId, (event) => {
        if (event.latestData) {
          session.realtimePressures = event.latestData.rawPressure
          if (event.latestData.interpolationResult) {
            session.realtimeResult = event.latestData.interpolationResult
          }
        }
      }),
      traversalProbeApi.onComplete(probeId, (event: TraversalCompleteEvent) => {
        session.completeEvent = event
        if (session.status) session.status = { ...session.status, status: event.status }
        releaseDevices(probeId)
      }),
      traversalProbeApi.onError(probeId, (event) => {
        session.error = event.error
      }),
    )
  }

  function ensureSubscriptions(probeId: ProbeId): void {
    if (runtimes[probeId].unsubscribers.length === 0) {
      setupSubscriptions(probeId)
    }
  }

  function acquireDevices(probeId: ProbeId): void {
    const runtime = runtimes[probeId]
    const ids = uniqueDeviceIds(sessionOf(probeId).config)
    for (const deviceId of ids) {
      if (!runtime.subscribedDeviceIds.includes(deviceId)) {
        acquireDevice(deviceId)
        runtime.subscribedDeviceIds.push(deviceId)
      }
    }
  }

  function releaseDevices(probeId: ProbeId): void {
    const runtime = runtimes[probeId]
    for (const deviceId of runtime.subscribedDeviceIds.splice(0)) {
      releaseDevice(deviceId)
    }
  }

  // 实时计算（每 probe 独立节流 timer 与 requestId）
  function clearRealtimeTimer(probeId: ProbeId): void {
    const runtime = runtimes[probeId]
    if (runtime.realtimeTimer) {
      clearTimeout(runtime.realtimeTimer)
      runtime.realtimeTimer = null
    }
  }

  async function runRealtimeInterpolation(probeId: ProbeId): Promise<void> {
    const runtime = runtimes[probeId]
    clearRealtimeTimer(probeId)
    const input = runtime.pendingInput
    runtime.pendingInput = null
    if (!input) {
      runtime.requestId += 1
      sessionOf(probeId).realtimeResult = null
      return
    }
    const requestId = nextRequestId(runtime)
    runtime.lastRealtimeAt = Date.now()
    const session = sessionOf(probeId)
    const res = await traversalProbeApi.calculateRealtime(
      probeId,
      input,
      session.config ?? undefined,
      session.config?.probeType,
    )
    if (requestId !== runtime.requestId) return
    if (res.success) {
      session.realtimeResult = res.data ?? null
    } else {
      // 与 legacy 一致的占位结果：IsValid=false + warning，让 UI 正确三态分类。
      session.realtimeResult = {
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

  function syncRealtimeInput(probeId: ProbeId, input: TraversalRealtimeInput | null): void {
    const runtime = runtimes[probeId]
    const session = sessionOf(probeId)
    if (!input || !session.hasLoadedInterpolator) {
      runtime.pendingInput = null
      clearRealtimeTimer(probeId)
      runtime.requestId += 1
      session.realtimeResult = null
      return
    }
    runtime.pendingInput = input
    const elapsed = Date.now() - runtime.lastRealtimeAt
    const delay = Math.max(0, uiRefreshIntervalMs.value - elapsed)
    if (delay === 0) {
      void runRealtimeInterpolation(probeId)
      return
    }
    if (!runtime.realtimeTimer) {
      runtime.realtimeTimer = setTimeout(() => { void runRealtimeInterpolation(probeId) }, delay)
    }
  }

  // 配置与生命周期 actions（每个 action 显式接收 ProbeId 首参）
  async function loadConfig(probeId: ProbeId): Promise<void> {
    const requestId = nextRequestId(runtimes[probeId])
    const res = await traversalProbeApi.getConfig(probeId)
    if (requestId !== runtimes[probeId].requestId) return
    if (res.success) {
      const session = sessionOf(probeId)
      session.config = res.data ?? null
      session.hasLoadedInterpolator = inferInterpolatorLoaded(session.config)
      session.interpolatorRestoreMessage = null
      if (session.hasLoadedInterpolator) {
        await verifyInterpolatorWithBackend(probeId, requestId)
      }
    }
  }

  async function verifyInterpolatorWithBackend(probeId: ProbeId, requestId: number): Promise<void> {
    const session = sessionOf(probeId)
    try {
      const res = await traversalProbeApi.checkPreconditions(probeId, session.config ?? undefined)
      if (requestId !== runtimes[probeId].requestId) return
      if (!res.success || !res.data) {
        session.interpolatorRestoreMessage = `插值器状态校验失败: ${res.error ?? '响应为空'}`
        return
      }
      const result: PreconditionCheckResult = res.data
      const prbCheck = result.checks.find((check) => check.name === 'PRB')
      if (prbCheck && !prbCheck.passed) {
        session.hasLoadedInterpolator = false
        session.interpolatorRestoreMessage = prbCheck.message ?? '插值器未加载'
      }
    } catch (err) {
      if (requestId !== runtimes[probeId].requestId) return
      session.interpolatorRestoreMessage = `插值器状态校验失败: ${err instanceof Error ? err.message : String(err)}`
    }
  }

  async function recoverRuntime(probeId: ProbeId): Promise<void> {
    const runtime = runtimes[probeId]
    const requestId = nextRequestId(runtime)
    const res = await traversalProbeApi.getStatus(probeId)
    if (requestId !== runtime.requestId) return
    const session = sessionOf(probeId)
    if (!res.success) {
      session.error = res.error ?? '恢复运行状态失败'
      return
    }
    session.status = res.data ?? null
    if (isActiveStatus(session.status?.status)) {
      ensureSubscriptions(probeId)
      acquireDevices(probeId)
      return
    }
    teardownSubscriptions(probeId)
    releaseDevices(probeId)
  }

  async function saveConfig(probeId: ProbeId, config: TraversalTestConfig): Promise<boolean> {
    const res = await traversalProbeApi.saveConfig(probeId, config)
    if (res.success) {
      sessionOf(probeId).config = config
      return true
    }
    sessionOf(probeId).error = res.error ?? '保存配置失败'
    return false
  }

  function applyInterpolatorImport<T>(
    probeId: ProbeId,
    res: { success: boolean; data?: T; error?: string },
    fallback: string,
  ): T | null {
    const session = sessionOf(probeId)
    if (res.success && res.data) {
      session.hasLoadedInterpolator = true
      session.error = null
      return res.data
    }
    session.error = res.error ?? fallback
    return null
  }

  async function importPrbFile(probeId: ProbeId, filePath: string): Promise<PrbFileInfo | null> {
    return applyInterpolatorImport(probeId, await traversalProbeApi.importPrb(probeId, filePath), 'PRB 导入失败')
  }

  async function importCalibrationCsvFile(probeId: ProbeId, filePath: string): Promise<CalibrationCsvFileInfo | null> {
    return applyInterpolatorImport(probeId, await traversalProbeApi.importCalibrationCsv(probeId, filePath), 'CSV 导入失败')
  }

  async function importMultiPrbFiles(
    probeId: ProbeId,
    filePaths: string[],
    machNumbers?: number[],
    interpolationMode: MultiPrbInterpolationMode = 'linear',
  ): Promise<{ files: PrbFileInfo[]; machNumbers: number[]; warnings: string[] } | null> {
    return applyInterpolatorImport(
      probeId,
      await traversalProbeApi.importMultiPrb(probeId, filePaths, machNumbers, interpolationMode),
      '多 PRB 导入失败',
    )
  }

  async function importSevenHolePrbFiles(
    probeId: ProbeId,
    innerFilePath: string,
    outerFilePaths: string[],
  ): Promise<{ files: SevenHolePrbFileInfo[]; validRange: PrbFileInfo['validRange'] } | null> {
    return applyInterpolatorImport(
      probeId,
      await traversalProbeApi.importSevenHolePrb(probeId, innerFilePath, outerFilePaths),
      '七孔 PRB 导入失败',
    )
  }

  async function importSevenHoleCalibrationCsvFiles(
    probeId: ProbeId,
    innerFilePath: string,
    outerFilePaths: string[],
  ): Promise<{ files: SevenHolePrbFileInfo[]; validRange: PrbFileInfo['validRange'] } | null> {
    return applyInterpolatorImport(
      probeId,
      await traversalProbeApi.importSevenHoleCalibrationCsv(probeId, innerFilePath, outerFilePaths),
      '七孔 CSV 导入失败',
    )
  }

  async function clearInterpolator(probeId: ProbeId, probeType: TraversalProbeType): Promise<boolean> {
    const res = await traversalProbeApi.clearInterpolator(probeId, probeType)
    const session = sessionOf(probeId)
    if (!res.success) {
      session.error = res.error ?? '清除插值器失败'
      return false
    }
    session.hasLoadedInterpolator = false
    session.realtimeResult = null
    session.error = null
    return true
  }

  async function loadCheckpoint(probeId: ProbeId): Promise<void> {
    const requestId = nextRequestId(runtimes[probeId])
    const res = await traversalProbeApi.loadCheckpoint(probeId)
    if (requestId !== runtimes[probeId].requestId) return
    if (res.success) {
      sessionOf(probeId).checkpoint = res.data ?? null
    }
  }

  async function start(probeId: ProbeId): Promise<boolean> {
    const session = sessionOf(probeId)
    if (session.isStarting || !session.config) return false
    session.isStarting = true
    session.error = null
    session.completeEvent = null
    // 新任务启动：代际失效丢弃旧轮询响应（spec FR6）。
    invalidateProbePolling(probeId)
    const res = await traversalProbeApi.start(probeId, session.config)
    session.isStarting = false
    if (!res.success || !res.data) {
      session.error = res.error ?? '启动失败'
      return false
    }
    runtimes[probeId].pendingTaskId = res.data.taskId
    session.status = { taskId: res.data.taskId, status: 'running' } as TraversalTestStatus
    setupSubscriptions(probeId)
    acquireDevices(probeId)
    return true
  }

  async function pause(probeId: ProbeId): Promise<boolean> {
    const res = await traversalProbeApi.pause(probeId)
    if (res.success) {
      const session = sessionOf(probeId)
      if (session.status) session.status = { ...session.status, status: 'paused', state: 'paused' }
      return true
    }
    sessionOf(probeId).error = res.error ?? '暂停失败'
    return false
  }

  async function resume(probeId: ProbeId): Promise<boolean> {
    const res = await traversalProbeApi.resume(probeId)
    if (res.success) {
      const session = sessionOf(probeId)
      if (session.status) session.status = { ...session.status, status: 'running', state: 'running' }
      return true
    }
    sessionOf(probeId).error = res.error ?? '恢复失败'
    return false
  }

  async function stop(probeId: ProbeId): Promise<boolean> {
    const res = await traversalProbeApi.stop(probeId)
    if (res.success) return true
    sessionOf(probeId).error = res.error ?? '停止失败'
    return false
  }

  async function close(probeId: ProbeId): Promise<boolean> {
    const res = await traversalProbeApi.close(probeId)
    if (!res.success) {
      sessionOf(probeId).error = res.error ?? '关闭失败'
      return false
    }
    cleanupLocal(probeId)
    return true
  }

  function cleanupLocal(probeId: ProbeId): void {
    releaseDevices(probeId)
    teardownSubscriptions(probeId)
    clearRealtimeTimer(probeId)
    invalidateProbePolling(probeId)
    const runtime = runtimes[probeId]
    runtime.requestId += 1
    runtime.pendingInput = null
  }

  async function resumeFromCheckpoint(probeId: ProbeId, taskId: string): Promise<boolean> {
    invalidateProbePolling(probeId)
    const res = await traversalProbeApi.resumeFromCheckpoint(probeId, taskId)
    if (!res.success || !res.data) {
      sessionOf(probeId).error = res.error ?? '恢复失败'
      return false
    }
    sessionOf(probeId).checkpoint = null
    sessionOf(probeId).status = { taskId: res.data.taskId, status: 'running' } as TraversalTestStatus
    setupSubscriptions(probeId)
    acquireDevices(probeId)
    return true
  }

  async function clearCheckpoint(probeId: ProbeId, taskId: string): Promise<boolean> {
    const res = await traversalProbeApi.clearCheckpoint(probeId, taskId)
    if (!res.success) {
      sessionOf(probeId).error = res.error ?? '清除断点失败'
      return false
    }
    sessionOf(probeId).checkpoint = null
    return true
  }

  function markInterpolatorLoaded(probeId: ProbeId, restoreMessage: string | null = null): void {
    const session = sessionOf(probeId)
    session.hasLoadedInterpolator = true
    session.interpolatorRestoreMessage = restoreMessage
  }

  // reset：仅清理本 probe（timer/订阅/设备/状态），另一路完全不受影响
  function reset(probeId: ProbeId): void {
    teardownSubscriptions(probeId)
    releaseDevices(probeId)
    clearRealtimeTimer(probeId)
    invalidateProbePolling(probeId)
    const runtime = runtimes[probeId]
    runtime.requestId += 1
    runtime.pendingInput = null
    runtime.blockedTaskId = null
    runtime.pendingTaskId = null
    Object.assign(sessions[probeId], emptySession())
  }

  // 派生状态（供模式开关与 UI 门禁）
  function isActive(probeId: ProbeId): boolean {
    return sessions[probeId].isStarting || isActiveStatus(sessions[probeId].status?.status)
  }

  const anyActive = computed(() => isActive('probe1') || isActive('probe2'))

  return {
    sessions,
    sessionOf,
    anyActive,
    isActive,
    loadConfig, saveConfig,
    recoverRuntime,
    importPrbFile, importMultiPrbFiles, importCalibrationCsvFile,
    importSevenHolePrbFiles, importSevenHoleCalibrationCsvFiles, clearInterpolator,
    loadCheckpoint,
    start,
    pause, resume,
    stop, close,
    cleanupLocal,
    resumeFromCheckpoint,
    clearCheckpoint,
    markInterpolatorLoaded,
    syncRealtimeInput,
    reset,
  }
})
