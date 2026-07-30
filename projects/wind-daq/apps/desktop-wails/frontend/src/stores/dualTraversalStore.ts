import { computed, reactive } from 'vue'
import { defineStore } from 'pinia'

import { deviceApi } from '@api/deviceApi'
import { traversalProbeApi } from '@api/traversalApi'
import { invalidateProbePolling } from '@api/traversalPolling'
import { useI18nStore } from '@stores/i18nStore'
import { useStorageStore } from '@stores/storageStore'
import {
  acquireDualTraversalDevice,
  createDualTraversalRuntime,
  dualDeviceRefCount,
  releaseDualTraversalDevice,
  resetDualDeviceRefCounts,
  uniqueTraversalDeviceIds,
} from '@stores/dualTraversalRuntime'
import {
  buildRealtimePressuresFromSnapshots,
  toRealtimeInterpolationInput,
} from '@composables/useTraversalRealtimeData'
import { safeInterpolate } from '@shared/i18nInterpolate'
import { isRecoverableTaskExistsError } from '@api/traversalErrorMapper'
import type { DualTraversalRuntimes, DualTraversalSessionRuntime } from '@stores/dualTraversalRuntime'
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

export { dualDeviceRefCount, resetDualDeviceRefCounts } from '@stores/dualTraversalRuntime'

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

const REALTIME_THROTTLE_FALLBACK_MS = 200

function nextRequestId(runtime: DualTraversalSessionRuntime): number {
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
  // C7 修复：在 setup 顶部一次性捕获 i18n store，替代所有硬编码中文错误消息。
  const i18n = useI18nStore()
  const sessions = reactive<Record<ProbeId, TraversalSessionState>>({
    probe1: emptySession(),
    probe2: emptySession(),
  })
  const runtimes: DualTraversalRuntimes = {
    probe1: createDualTraversalRuntime(),
    probe2: createDualTraversalRuntime(),
  }
  const checkpointPending = reactive<Record<ProbeId, boolean>>({ probe1: false, probe2: false })

  const uiRefreshIntervalMs = computed(() => {
    const store = useStorageStore()
    const hz = store?.settings?.refreshRateHz
    return hz && hz > 0 ? Math.round(1000 / hz) : REALTIME_THROTTLE_FALLBACK_MS
  })

  function sessionOf(probeId: ProbeId): TraversalSessionState {
    return sessions[probeId]
  }

  // 订阅管理（每 probe 独立；停止一路不影响另一路）
  //
  // C9 修复：订阅拆分为两类
  // - polling 类（onStatus/onProgress/onComplete/onError）：500ms 轮询，终态后调用
  //   teardownPollingSubscriptions 停止，避免无意义轮询浪费 CPU/网络。
  // - snapshot 类（onSnapshot DAQ 实时快照）：终态后保留，让用户在结果界面仍能看到
  //   当前压力值，直到显式 close/reset。
  function teardownPollingSubscriptions(probeId: ProbeId): void {
    const runtime = runtimes[probeId]
    for (const unsubscribe of runtime.pollingUnsubscribers.splice(0)) {
      unsubscribe()
    }
  }

  function teardownSnapshotSubscriptions(probeId: ProbeId): void {
    const runtime = runtimes[probeId]
    for (const unsubscribe of runtime.snapshotUnsubscribers.splice(0)) {
      unsubscribe()
    }
  }

  function teardownSubscriptions(probeId: ProbeId): void {
    teardownPollingSubscriptions(probeId)
    teardownSnapshotSubscriptions(probeId)
  }

  function setupSubscriptions(probeId: ProbeId): void {
    teardownSubscriptions(probeId)
    const runtime = runtimes[probeId]
    const session = sessionOf(probeId)
    // snapshot 类订阅：DAQ 实时快照，终态后保留以显示当前压力值。
    runtime.snapshotUnsubscribers.push(
      deviceApi.onSnapshot((payload) => {
        const config = session.config
        if (!config || !uniqueTraversalDeviceIds(config).includes(payload.deviceId)) return
        runtime.latestSnapshots.set(payload.deviceId, payload)
        const pressures = buildRealtimePressuresFromSnapshots(
          config,
          Array.from(runtime.latestSnapshots.values()),
        )
        session.realtimePressures = pressures as TraversalSessionState['realtimePressures']
        syncRealtimeInput(
          probeId,
          toRealtimeInterpolationInput(pressures, config.probeType ?? 'five-hole'),
        )
      }),
    )
    // polling 类订阅：500ms 轮询，终态后 teardown。
    runtime.pollingUnsubscribers.push(
      traversalProbeApi.onStatus(probeId, (status) => {
        // I-17 修复：pause/resume 乐观窗口内仅合并非 status.state 字段，
        // 避免陈旧轮询把刚切换的 paused/running 状态回退（500ms 内最多 3 次陈旧回写）。
        const now = Date.now()
        if (runtime.optimisticStatusUntil > now && session.status) {
          session.status = {
            ...status,
            status: session.status.status,
            state: session.status.state,
          }
          return
        }
        // 窗口外正常覆写；同时清除乐观标记（避免下次 pause/resume 前残留）。
        if (runtime.optimisticStatusUntil !== 0 && runtime.optimisticStatusUntil <= now) {
          runtime.optimisticStatusUntil = 0
        }
        session.status = status
      }),
      traversalProbeApi.onProgress(probeId, (event) => {
        // 只更新权威插值结果，不覆盖 DAQ 快照的实时压力
        if (event.latestData?.interpolationResult) {
          session.realtimeResult = event.latestData.interpolationResult
        }
      }),
      traversalProbeApi.onComplete(probeId, (event: TraversalCompleteEvent) => {
        session.completeEvent = event
        // C5 修复：dual 路径同时更新 status 与 state，避免 'running'+'completed' 矛盾组合。
        // 与 traversalStore.applyCompleteEvent 行为对齐。
        if (session.status) {
          session.status = { ...session.status, status: event.status, state: event.status }
        }
        // C9 修复：终态后立即 teardown 轮询类订阅，停止 500ms 无意义轮询。
        // snapshot 类（onSnapshot）保留，让用户在结果界面仍能看到当前压力值。
        teardownPollingSubscriptions(probeId)
        if (event.status === 'error' || event.status === 'stopped') {
          void loadCheckpoint(probeId)
        }
      }),
      traversalProbeApi.onError(probeId, (event) => {
        session.error = event.error
      }),
    )
  }

  function ensureSubscriptions(probeId: ProbeId): void {
    const runtime = runtimes[probeId]
    if (runtime.pollingUnsubscribers.length === 0 || runtime.snapshotUnsubscribers.length === 0) {
      setupSubscriptions(probeId)
    }
  }

  function acquireDevices(probeId: ProbeId): void {
    const runtime = runtimes[probeId]
    const ids = uniqueTraversalDeviceIds(sessionOf(probeId).config)
    for (const deviceId of ids) {
      if (!runtime.subscribedDeviceIds.includes(deviceId)) {
        acquireDualTraversalDevice(deviceId)
        runtime.subscribedDeviceIds.push(deviceId)
      }
    }
  }

  function releaseDevices(probeId: ProbeId): void {
    const runtime = runtimes[probeId]
    for (const deviceId of runtime.subscribedDeviceIds.splice(0)) {
      releaseDualTraversalDevice(deviceId)
    }
    runtime.latestSnapshots.clear()
  }

  function refreshMonitoring(probeId: ProbeId): void {
    releaseDevices(probeId)
    setupSubscriptions(probeId)
    acquireDevices(probeId)
  }

  // 实时计算（每 probe 独立节流 timer 与 requestId）
  function clearRealtimeTimer(probeId: ProbeId): void {
    const runtime = runtimes[probeId]
    if (runtime.realtimeTimer) {
      clearTimeout(runtime.realtimeTimer)
      runtime.realtimeTimer = null
    }
  }

  // I-12/I-20 修复：取消 pending realtime 请求的统一入口。
  // - 递增 requestId 让在飞的 calculateRealtime 返回时通过代际校验丢弃结果；
  // - 清空 pendingInput 避免下个节流周期复用陈旧输入；
  // - 清 timer 阻止已调度的节流回调触发；
  // - 不重置 realtimeInFlight（在飞请求自身返回时会因代际不匹配被丢弃）。
  function cancelPendingRealtime(probeId: ProbeId): void {
    const runtime = runtimes[probeId]
    runtime.requestId += 1
    runtime.pendingInput = null
    clearRealtimeTimer(probeId)
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
    // I-20 修复：in-flight 守卫——上一次请求尚未返回时直接跳过本次触发，
    // 避免高频 snapshot 在节流间隙堆积并发请求导致 UI 闪烁与后端压力。
    // 节流 timer 会在上次请求返回后由 syncRealtimeInput 重新调度。
    if (runtime.realtimeInFlight) {
      // 把当前 input 放回 pendingInput，让在飞请求返回后下一次 snapshot 触发时能立刻使用。
      runtime.pendingInput = input
      return
    }
    const requestId = nextRequestId(runtime)
    runtime.lastRealtimeAt = Date.now()
    runtime.realtimeInFlight = true
    const session = sessionOf(probeId)
    try {
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
    } finally {
      runtime.realtimeInFlight = false
    }
  }

  function syncRealtimeInput(probeId: ProbeId, input: TraversalRealtimeInput | null): void {
    const runtime = runtimes[probeId]
    const session = sessionOf(probeId)
    if (!input || !session.hasLoadedInterpolator) {
      // I-12 修复：通过 cancelPendingRealtime 统一取消在飞请求、清 timer、清 pendingInput。
      cancelPendingRealtime(probeId)
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
      if (requestId === runtimes[probeId].requestId) refreshMonitoring(probeId)
    } else {
      // C7 修复：原 loadConfig 在 res.success===false 时静默返回，UI 无错误提示。
      // 改为写入 session.error，让 UI 能通过 warningText 显示。
      sessionOf(probeId).error = res.error ?? i18n.t.dualErrLoadConfig
    }
  }

  async function verifyInterpolatorWithBackend(probeId: ProbeId, requestId: number): Promise<void> {
    const session = sessionOf(probeId)
    try {
      const res = await traversalProbeApi.checkPreconditions(probeId, session.config ?? undefined)
      if (requestId !== runtimes[probeId].requestId) return
      if (!res.success || !res.data) {
        // C7 + C8 修复：使用 i18n + safeInterpolate，避免硬编码中文与 $ 注入。
        session.interpolatorRestoreMessage = safeInterpolate(
          i18n.t.dualErrVerifyInterpolator,
          '{error}',
          res.error ?? i18n.t.travErrResponseEmpty,
        )
        return
      }
      const result: PreconditionCheckResult = res.data
      const prbCheck = result.checks.find((check) => check.name === 'PRB')
      if (prbCheck && !prbCheck.passed) {
        session.hasLoadedInterpolator = false
        session.interpolatorRestoreMessage = prbCheck.message ?? i18n.t.dualErrInterpolatorNotLoaded
      }
    } catch (err) {
      if (requestId !== runtimes[probeId].requestId) return
      session.interpolatorRestoreMessage = safeInterpolate(
        i18n.t.dualErrVerifyInterpolatorException,
        '{error}',
        err instanceof Error ? err.message : String(err),
      )
    }
  }

  async function recoverRuntime(probeId: ProbeId): Promise<void> {
    const runtime = runtimes[probeId]
    const requestId = nextRequestId(runtime)
    const res = await traversalProbeApi.getStatus(probeId)
    if (requestId !== runtime.requestId) return
    const session = sessionOf(probeId)
    if (!res.success) {
      session.error = res.error ?? i18n.t.dualErrRecoverRuntime
      return
    }
    session.status = res.data ?? null
    if (isActiveStatus(session.status?.status)) {
      ensureSubscriptions(probeId)
      acquireDevices(probeId)
    }
  }

  async function saveConfig(probeId: ProbeId, config: TraversalTestConfig): Promise<boolean> {
    const res = await traversalProbeApi.saveConfig(probeId, config)
    if (res.success) {
      sessionOf(probeId).config = config
      refreshMonitoring(probeId)
      return true
    }
    sessionOf(probeId).error = res.error ?? i18n.t.dualErrSaveConfig
    return false
  }

  function applyInterpolatorImport<T>(
    probeId: ProbeId,
    res: { success: boolean; data?: T; error?: string },
    fallbackKey: keyof typeof i18n.t,
  ): T | null {
    const session = sessionOf(probeId)
    if (res.success && res.data) {
      session.hasLoadedInterpolator = true
      session.error = null
      return res.data
    }
    session.error = res.error ?? i18n.t[fallbackKey]
    return null
  }

  async function importPrbFile(probeId: ProbeId, filePath: string): Promise<PrbFileInfo | null> {
    return applyInterpolatorImport(probeId, await traversalProbeApi.importPrb(probeId, filePath), 'dualErrImportPrb')
  }

  async function importCalibrationCsvFile(probeId: ProbeId, filePath: string): Promise<CalibrationCsvFileInfo | null> {
    return applyInterpolatorImport(probeId, await traversalProbeApi.importCalibrationCsv(probeId, filePath), 'dualErrImportCalibrationCsv')
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
      'dualErrImportMultiPrb',
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
      'dualErrImportSevenHolePrb',
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
      'dualErrImportSevenHoleCalibrationCsv',
    )
  }

  async function clearInterpolator(probeId: ProbeId, probeType: TraversalProbeType): Promise<boolean> {
    const res = await traversalProbeApi.clearInterpolator(probeId, probeType)
    const session = sessionOf(probeId)
    if (!res.success) {
      session.error = res.error ?? i18n.t.dualErrClearInterpolator
      return false
    }
    session.hasLoadedInterpolator = false
    // I-12 修复：清除插值器后必须取消 pending realtime 请求并清空 realtime 字段，
    // 否则在飞请求返回时仍会写入 realtimeResult 导致 UI 闪现一次错误结果。
    cancelPendingRealtime(probeId)
    session.realtimeResult = null
    session.realtimePressures = null
    session.error = null
    return true
  }

  async function loadCheckpoint(probeId: ProbeId): Promise<{ ok: boolean; error?: string }> {
    const runtime = runtimes[probeId]
    const requestId = ++runtime.checkpointRequestId
    runtime.checkpointAbort?.abort()
    const abort = new AbortController()
    runtime.checkpointAbort = abort
    const res = await traversalProbeApi.loadCheckpoint(probeId, abort.signal)
    if (requestId !== runtime.checkpointRequestId) return { ok: false }
    runtime.checkpointAbort = null
    if (res.success) {
      sessionOf(probeId).checkpoint = res.data ?? null
      return { ok: true }
    }
    return { ok: false, error: res.error ?? i18n.t.dualErrLoadCheckpoint }
  }

  async function start(probeId: ProbeId): Promise<boolean> {
    const session = sessionOf(probeId)
    const runtime = runtimes[probeId]
    if (session.isStarting || !session.config) return false
    // I-18 修复：clearCheckpoint/resumeFromCheckpoint 进行中时 checkpointPending=true，
    // 此时若用户点 start 会与正在进行的 loadCheckpoint 产生 race（两边都改 session.checkpoint）。
    // 直接拒绝并提示，让用户等当前操作完成后再点开始。
    if (checkpointPending[probeId]) {
      session.error = i18n.t.dualErrCheckpointPending
      return false
    }
    const lifecycleGeneration = runtime.lifecycleGeneration
    session.isStarting = true
    session.error = null
    // I-14 修复：开帧前清空上一轮的 realtime 字段，避免新一轮采集开始前 UI 闪现
    // 上一轮最后一帧的压力值与插值结果（用户切换 task 后视觉残留）。
    cancelPendingRealtime(probeId)
    session.realtimePressures = null
    session.realtimeResult = null
    const checkpointResult = await loadCheckpoint(probeId)
    if (lifecycleGeneration !== runtime.lifecycleGeneration) return false
    if (!checkpointResult.ok) {
      session.isStarting = false
      session.error = checkpointResult.error ?? i18n.t.dualErrLoadCheckpoint
      return false
    }
    if (session.checkpoint) {
      // C10 修复：原静默返回 false，UI 无任何反馈，用户会反复点击"开始"。
      // 改为写入 session.error，UI 通过 warningText 显示提示。
      session.isStarting = false
      session.error = i18n.t.dualErrCheckpointPending
      return false
    }
    // 新任务启动：代际失效丢弃旧轮询响应（spec FR6）。
    invalidateProbePolling(probeId)
    const res = await traversalProbeApi.start(probeId, session.config)
    if (lifecycleGeneration !== runtime.lifecycleGeneration) {
      if (res.success && res.data) await traversalProbeApi.stop(probeId)
      return false
    }
    session.isStarting = false
    if (!res.success || !res.data) {
      // I-16 修复：用共享常量替代裸字符串 'recoverable_task_exists'，
      // 后端改码或前端多处判定时只需修改 isRecoverableTaskExistsError 一处。
      if (isRecoverableTaskExistsError(res.error)) {
        const recoveryResult = await loadCheckpoint(probeId)
        if (lifecycleGeneration !== runtime.lifecycleGeneration) return false
        if (session.checkpoint) {
          session.error = i18n.t.dualErrCheckpointPending
          return false
        }
        if (!recoveryResult.ok) {
          session.error = recoveryResult.error ?? i18n.t.dualErrLoadCheckpoint
          return false
        }
      }
      session.error = res.error ?? i18n.t.dualErrStart
      return false
    }
    session.completeEvent = null
    session.status = { taskId: res.data.taskId, status: 'running' } as TraversalTestStatus
    setupSubscriptions(probeId)
    acquireDevices(probeId)
    return true
  }

  // I-17 修复：pause/resume 乐观更新窗口长度。
  // 后端 onStatus 轮询 500ms 一次，3 次轮询（1.5s）足以让真实状态变化反映到 status；
  // 窗口内 onStatus 仅合并非 status 字段，避免陈旧 running 把 paused 回退。
  const OPTIMISTIC_STATUS_WINDOW_MS = 1500

  async function pause(probeId: ProbeId): Promise<boolean> {
    const res = await traversalProbeApi.pause(probeId)
    if (res.success) {
      const session = sessionOf(probeId)
      if (session.status) session.status = { ...session.status, status: 'paused', state: 'paused' }
      // I-17 修复：开启乐观窗口，防止 onStatus 轮询把 'paused' 回退为 'running'。
      runtimes[probeId].optimisticStatusUntil = Date.now() + OPTIMISTIC_STATUS_WINDOW_MS
      return true
    }
    sessionOf(probeId).error = res.error ?? i18n.t.dualErrPause
    return false
  }

  async function resume(probeId: ProbeId): Promise<boolean> {
    const res = await traversalProbeApi.resume(probeId)
    if (res.success) {
      const session = sessionOf(probeId)
      if (session.status) session.status = { ...session.status, status: 'running', state: 'running' }
      // I-17 修复：开启乐观窗口，防止 onStatus 轮询把 'running' 回退为 'paused'。
      runtimes[probeId].optimisticStatusUntil = Date.now() + OPTIMISTIC_STATUS_WINDOW_MS
      return true
    }
    sessionOf(probeId).error = res.error ?? i18n.t.dualErrResume
    return false
  }

  async function stop(probeId: ProbeId): Promise<boolean> {
    const res = await traversalProbeApi.stop(probeId)
    if (res.success) return true
    sessionOf(probeId).error = res.error ?? i18n.t.dualErrStop
    return false
  }

  async function close(probeId: ProbeId): Promise<boolean> {
    const res = await traversalProbeApi.close(probeId)
    if (!res.success) {
      sessionOf(probeId).error = res.error ?? i18n.t.dualErrClose
      return false
    }
    cleanupLocal(probeId)
    // I-13 修复：close 成功后必须清空 session 终态字段，否则 UI 仍显示 'running'/'completed'
    // 等陈旧状态（cleanupLocal 仅释放资源，不动 session 状态以支持 close 失败重试场景）。
    // 这里在 cleanupLocal 后显式重置 session 状态字段。
    const session = sessionOf(probeId)
    session.status = null
    session.completeEvent = null
    session.realtimePressures = null
    session.realtimeResult = null
    session.error = null
    session.checkpoint = null
    return true
  }

  function cleanupLocal(probeId: ProbeId): void {
    releaseDevices(probeId)
    teardownSubscriptions(probeId)
    clearRealtimeTimer(probeId)
    invalidateProbePolling(probeId)
    const runtime = runtimes[probeId]
    runtime.lifecycleGeneration += 1
    runtime.requestId += 1
    runtime.checkpointRequestId += 1
    runtime.checkpointOperationId += 1
    runtime.checkpointAbort?.abort()
    runtime.checkpointAbort = null
    runtime.pendingInput = null
    runtime.realtimeInFlight = false
    checkpointPending[probeId] = false
    sessionOf(probeId).isStarting = false
  }

  async function resumeFromCheckpoint(probeId: ProbeId, taskId: string): Promise<boolean> {
    if (checkpointPending[probeId]) return false
    const runtime = runtimes[probeId]
    const lifecycleGeneration = runtime.lifecycleGeneration
    const operationId = ++runtime.checkpointOperationId
    checkpointPending[probeId] = true
    invalidateProbePolling(probeId)
    const res = await traversalProbeApi.resumeFromCheckpoint(probeId, taskId)
    if (operationId !== runtime.checkpointOperationId || lifecycleGeneration !== runtime.lifecycleGeneration) {
      if (res.success && res.data) await traversalProbeApi.stop(probeId)
      return false
    }
    checkpointPending[probeId] = false
    if (!res.success || !res.data) {
      sessionOf(probeId).error = res.error ?? i18n.t.dualErrResumeFromCheckpoint
      return false
    }
    const session = sessionOf(probeId)
    session.checkpoint = null
    session.error = null
    session.completeEvent = null
    session.status = { taskId: res.data.taskId, status: 'running' } as TraversalTestStatus
    setupSubscriptions(probeId)
    acquireDevices(probeId)
    return true
  }

  async function clearCheckpoint(probeId: ProbeId, taskId: string): Promise<boolean> {
    if (checkpointPending[probeId]) return false
    const runtime = runtimes[probeId]
    const lifecycleGeneration = runtime.lifecycleGeneration
    const operationId = ++runtime.checkpointOperationId
    checkpointPending[probeId] = true
    const res = await traversalProbeApi.clearCheckpoint(probeId, taskId)
    if (operationId !== runtime.checkpointOperationId || lifecycleGeneration !== runtime.lifecycleGeneration) return false
    if (!res.success) {
      checkpointPending[probeId] = false
      sessionOf(probeId).error = res.error ?? i18n.t.dualErrClearCheckpoint
      return false
    }
    const checkpointResult = await loadCheckpoint(probeId)
    if (operationId !== runtime.checkpointOperationId || lifecycleGeneration !== runtime.lifecycleGeneration) return false
    checkpointPending[probeId] = false
    const session = sessionOf(probeId)
    if (!checkpointResult.ok) {
      session.error = checkpointResult.error ?? i18n.t.dualErrLoadCheckpoint
      return false
    }
    if (session.checkpoint) {
      session.error = i18n.t.dualErrClearCheckpointRetry
      return false
    }
    session.status = null
    session.error = null
    session.completeEvent = null
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
    runtime.lifecycleGeneration += 1
    runtime.requestId += 1
    runtime.checkpointRequestId += 1
    runtime.checkpointOperationId += 1
    runtime.checkpointAbort?.abort()
    runtime.checkpointAbort = null
    runtime.pendingInput = null
    runtime.realtimeInFlight = false
    checkpointPending[probeId] = false
    Object.assign(sessions[probeId], emptySession())
  }

  // 派生状态（供模式开关与 UI 门禁）
  function isActive(probeId: ProbeId): boolean {
    return sessions[probeId].isStarting || isActiveStatus(sessions[probeId].status?.status)
  }

  const anyActive = computed(() => isActive('probe1') || isActive('probe2'))

  return {
    sessions,
    checkpointPending,
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
