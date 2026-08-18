import { request } from '@api/http-client'
import { isWailsAvailable } from '@api/wails-adapter'
import { subscribeProbeStatus } from '@api/traversalPolling'
import type {
  CalibrationCsvFileInfo,
  InterpolationResult,
  MultiPrbInterpolationMode,
  PrbFileInfo,
  PreconditionCheckResult,
  ProbeId,
  SevenHolePrbFileInfo,
  TraversalCompleteEvent,
  TraversalCoordPoint,
  TraversalErrorEvent,
  TraversalErrorCode,
  TraversalProbeType,
  TraversalProgressEvent,
  TraversalRealtimeInput,
  TraversalTestConfig,
  TraversalTestStatus,
} from '@shared/types/traversal'

function apiBase(): string {
  if (import.meta.env.VITE_API_BASE) return import.meta.env.VITE_API_BASE
  if (import.meta.env.DEV) return ''
  return isWailsAvailable() ? 'http://127.0.0.1:8900' : ''
}

function apiRequest<T>(path: string, init?: RequestInit): Promise<T> {
  const fullPath = path.startsWith('http') ? path : `${apiBase()}${path}`
  return request<T>(fullPath, init)
}

function ok<T>(data?: T): { success: boolean; data?: T; error?: string } {
  return data === undefined ? { success: true } : { success: true, data }
}

function formatApiError(err: unknown): string {
  const msg = err instanceof Error ? err.message : String(err)
  if (msg.includes('Failed to fetch') || msg.includes('NetworkError')) {
    return '网络连接失败，请检查后端服务是否已启动'
  }
  return msg
}

async function invoke<T>(path: string, body?: unknown, method?: string, signal?: AbortSignal): Promise<{ success: boolean; data?: T; error?: string }> {
  try {
    const init: RequestInit = { method: method ?? 'POST' }
    if (signal) init.signal = signal
    if (body !== undefined) {
      init.body = JSON.stringify(body)
    }
    const data = await apiRequest<T>(path, init)
    return ok(data)
  } catch (err) {
    return { success: false, error: formatApiError(err) }
  }
}

/**
 * 共享状态轮询调度器：所有 onProgress/onComplete/onError 订阅复用同一个
 * 500ms 定时器，避免每个订阅者各自 setInterval 导致的"3 倍 GET 风暴"。
 *
 * 设计要点：
 * - 单例 timer 仅在有订阅者时启动，最后一个订阅者注销时停止；
 * - 每次轮询只发起一次 getStatus，分发给所有事件构造器；
 * - 通过 lastKey 去重，避免重复回调相同事件。
 */
const POLL_INTERVAL_MS = 500

interface SubscriptionEntry<T> {
  buildEvent: (status: TraversalTestStatus) => T | null
  callback: (event: T) => void
  lastKey: string
}

let pollingTimer: number | null = null
const subscribers: Set<SubscriptionEntry<unknown>> = new Set()

async function pollOnce(): Promise<void> {
  if (subscribers.size === 0) return
  const res = await traversalApi.getStatus()
  if (!res.success || !res.data) return
  const status = res.data
  for (const entry of subscribers) {
    const event = entry.buildEvent(status)
    if (!event) continue
    const key = JSON.stringify(event)
    if (key === entry.lastKey) continue
    entry.lastKey = key
    try {
      entry.callback(event)
    } catch (err) {
      console.error('[traversalApi] subscriber callback failed:', err)
    }
  }
}

function ensurePollingStarted(): void {
  if (pollingTimer !== null) return
  pollingTimer = window.setInterval(() => { void pollOnce() }, POLL_INTERVAL_MS)
}

function ensurePollingStopped(): void {
  if (subscribers.size === 0 && pollingTimer !== null) {
    window.clearInterval(pollingTimer)
    pollingTimer = null
  }
}

function createPollingSubscription<T>(
  buildEvent: (status: TraversalTestStatus) => T | null,
  callback: (event: T) => void,
): () => void {
  const entry: SubscriptionEntry<T> = { buildEvent, callback, lastKey: '' }
  subscribers.add(entry as SubscriptionEntry<unknown>)
  ensurePollingStarted()
  return () => {
    subscribers.delete(entry as SubscriptionEntry<unknown>)
    ensurePollingStopped()
  }
}

/** 后端返回的原始状态响应类型（包含后端额外字段） */
type TraversalStatusRawResponse = Omit<TraversalTestStatus, 'state' | 'lastErrorCode' | 'validationWarnings'> & {
  // 兼容后端既有 JSON 字段名；这里承载的是遍历逻辑目标 X/Y/Z/U（z/u 仅 custom 有值），不是插值结果 α/β。
  currentPointCoordinates?: TraversalCoordPoint
  currentPoint?: number | TraversalCoordPoint
  state?: string
  lastErrorCode?: TraversalErrorCode
  validationWarnings?: string[]
}

export const traversalApi = {
  getConfig: async (): Promise<{ success: boolean; data?: TraversalTestConfig | null; error?: string }> =>
    invoke<TraversalTestConfig | null>('/api/traversal/config', undefined, 'GET'),

  saveConfig: async (config: TraversalTestConfig): Promise<{ success: boolean; error?: string }> =>
    invoke('/api/traversal/config', config),

  importPrb: async (filePath: string): Promise<{ success: boolean; data?: PrbFileInfo; error?: string }> =>
    invoke<PrbFileInfo>('/api/traversal/importPrb', { filePath }),

  importCalibrationCsv: async (filePath: string): Promise<{ success: boolean; data?: CalibrationCsvFileInfo; error?: string }> =>
    invoke<CalibrationCsvFileInfo>('/api/traversal/importCalibrationCsv', { filePath }),

  importMultiPrb: async (
    filePaths: string[],
    machNumbers?: number[],
    interpolationMode?: MultiPrbInterpolationMode,
  ): Promise<{ success: boolean; data?: { files: PrbFileInfo[]; machNumbers: number[]; warnings: string[] }; error?: string }> =>
    invoke('/api/traversal/importMultiPrb', { filePaths, machNumbers, interpolationMode }),

  /** 七孔 PRB 文件集导入（spec §5.6）：1 个内区文件 + 按孔号 1..6 顺序的 6 个扇区文件 */
  importSevenHolePrb: async (
    innerFilePath: string,
    outerFilePaths: string[],
  ): Promise<{ success: boolean; data?: { files: SevenHolePrbFileInfo[]; validRange: PrbFileInfo['validRange'] }; error?: string }> =>
    invoke('/api/traversal/importSevenHolePrb', { innerFilePath, outerFilePaths }),

  /** 七孔校准 CSV 文件集导入（校准导出直接导入，免导出 .prb） */
  importSevenHoleCalibrationCsv: async (
    innerFilePath: string,
    outerFilePaths: string[],
  ): Promise<{ success: boolean; data?: { files: SevenHolePrbFileInfo[]; validRange: PrbFileInfo['validRange'] }; error?: string }> =>
    invoke('/api/traversal/importSevenHoleCalibrationCsv', { innerFilePath, outerFilePaths }),

  /** 显式清理指定探针类型的插值器（spec §5.2.1；探针切换事务的第一步） */
  clearInterpolator: async (probeType: TraversalProbeType): Promise<{ success: boolean; data?: { cleared: boolean }; error?: string }> =>
    invoke('/api/traversal/clearInterpolator', { probeType }),

  calculateRealtime: async (
    pressures: TraversalRealtimeInput,
    config?: TraversalTestConfig,
    probeType?: TraversalProbeType,
  ): Promise<{ success: boolean; data?: InterpolationResult; error?: string }> =>
    // 五孔请求体与旧版逐字节一致（省略 probeType）；七孔必须显式携带（spec §5.6）
    invoke<InterpolationResult>('/api/traversal/calculateRealtime', { pressures, config, ...(probeType ? { probeType } : {}) }),

  checkPreconditions: async (config?: TraversalTestConfig): Promise<{ success: boolean; data?: PreconditionCheckResult; error?: string }> =>
    invoke<PreconditionCheckResult>('/api/traversal/checkPreconditions', { config }),

  start: async (config: TraversalTestConfig): Promise<{ success: boolean; data?: { taskId: string }; error?: string }> =>
    invoke<{ taskId: string }>('/api/traversal/start', config),

  pause: async (): Promise<{ success: boolean; error?: string }> => invoke('/api/traversal/pause'),

  resume: async (): Promise<{ success: boolean; error?: string }> => invoke('/api/traversal/resume'),

  stop: async (): Promise<{ success: boolean; error?: string }> => invoke('/api/traversal/stop'),

  getStatus: async (): Promise<{ success: boolean; data?: TraversalTestStatus | null; error?: string }> => {
    const res = await invoke<TraversalStatusRawResponse | null>('/api/traversal/status', undefined, 'GET')
    if (!res.success || !res.data) return res as { success: boolean; data?: TraversalTestStatus | null; error?: string }
    const raw = res.data
    const point = typeof raw.currentPoint === 'number'
      ? raw.currentPointCoordinates
      : raw.currentPoint
    return {
      success: true,
      data: {
        ...raw,
        currentPoint: point,
        state: raw.state ?? raw.status,
        lastErrorCode: raw.lastErrorCode,
        validationWarnings: raw.validationWarnings,
      } as TraversalTestStatus
    }
  },

  onProgress: (callback: (event: TraversalProgressEvent) => void): (() => void) =>
    createPollingSubscription((status) => {
      if (status.status !== 'running' && status.status !== 'paused') return null
      // 不含 timestamp / duration 等每次轮询都会变化的字段：
      // polling 通过 JSON.stringify(event) 做去重，包含 Date.now() 会让 lastKey 永不相等，
      // 导致每 500ms 必触发回调。需要时间戳的回调方在接收时自行 Date.now()。
      return {
        taskId: status.taskId,
        totalPoints: status.totalPoints,
        completedPoints: status.completedPoints,
        currentPoint: status.currentPoint ?? { alpha: 0, beta: 0 },
        currentPointPhase: status.currentPointPhase,
        latestData: status.latestData,
        // 等待设备恢复采集：进度事件重建状态时必须透传，否则 store 丢失等待信息。
        waitingForAcquisition: status.waitingForAcquisition,
        waitingDevices: status.waitingDevices,
        waitingForAcquisitionSinceMs: status.waitingForAcquisitionSinceMs,
      }
    }, callback),

  onComplete: (callback: (event: TraversalCompleteEvent) => void): (() => void) =>
    createPollingSubscription((status) => {
      if (!['completed', 'stopped', 'error'].includes(status.status)) return null
      // 不含 duration：每次轮询 Date.now() - startTime 都变化，破坏去重。
      // 消费方（如 TraversalMain.vue）按需自行计算 finishedAt - startTime。
      return {
        taskId: status.taskId,
        success: status.status === 'completed',
        status: status.status as TraversalCompleteEvent['status'],
        totalPoints: status.totalPoints,
        filePath: status.csvPath,
        error: status.lastError,
      }
    }, callback),

  onError: (callback: (event: TraversalErrorEvent) => void): (() => void) =>
    createPollingSubscription((status) => {
      if (status.status !== 'error' || !status.lastError) return null
      const rawStatus = status as TraversalStatusRawResponse
      return {
        taskId: status.taskId,
        error: status.lastError,
        code: rawStatus.lastErrorCode ?? 'UNKNOWN',
        recoverable: false
      }
    }, callback),
}

// ---------------------------------------------------------------------------
// 双探针 probe-aware API（spec FR4 / Task 16）
//
// 两段路由 /api/traversal/{probeId}/{action}；legacy 无 probeId 函数保持不变。
// ---------------------------------------------------------------------------

function probePath(probeId: ProbeId, action: string): string {
  return `/api/traversal/${probeId}/${action}`
}

function mapStatusResponse(raw: TraversalStatusRawResponse): TraversalTestStatus {
  const point = typeof raw.currentPoint === 'number'
    ? raw.currentPointCoordinates
    : raw.currentPoint
  return {
    ...raw,
    currentPoint: point,
    state: raw.state ?? raw.status,
    lastErrorCode: raw.lastErrorCode,
    validationWarnings: raw.validationWarnings,
  } as TraversalTestStatus
}

export const traversalProbeApi = {
  getConfig: async (probeId: ProbeId): Promise<{ success: boolean; data?: TraversalTestConfig | null; error?: string }> =>
    invoke<TraversalTestConfig | null>(probePath(probeId, 'config'), undefined, 'GET'),

  saveConfig: async (probeId: ProbeId, config: TraversalTestConfig): Promise<{ success: boolean; error?: string }> =>
    invoke(probePath(probeId, 'config'), config),

  importPrb: async (probeId: ProbeId, filePath: string): Promise<{ success: boolean; data?: PrbFileInfo; error?: string }> =>
    invoke<PrbFileInfo>(probePath(probeId, 'importPrb'), { filePath }),

  importCalibrationCsv: async (probeId: ProbeId, filePath: string): Promise<{ success: boolean; data?: CalibrationCsvFileInfo; error?: string }> =>
    invoke<CalibrationCsvFileInfo>(probePath(probeId, 'importCalibrationCsv'), { filePath }),

  importMultiPrb: async (
    probeId: ProbeId,
    filePaths: string[],
    machNumbers?: number[],
    interpolationMode?: MultiPrbInterpolationMode,
  ): Promise<{ success: boolean; data?: { files: PrbFileInfo[]; machNumbers: number[]; warnings: string[] }; error?: string }> =>
    invoke(probePath(probeId, 'importMultiPrb'), { filePaths, machNumbers, interpolationMode }),

  importSevenHolePrb: async (
    probeId: ProbeId,
    innerFilePath: string,
    outerFilePaths: string[],
  ): Promise<{ success: boolean; data?: { files: SevenHolePrbFileInfo[]; validRange: PrbFileInfo['validRange'] }; error?: string }> =>
    invoke(probePath(probeId, 'importSevenHolePrb'), { innerFilePath, outerFilePaths }),

  importSevenHoleCalibrationCsv: async (
    probeId: ProbeId,
    innerFilePath: string,
    outerFilePaths: string[],
  ): Promise<{ success: boolean; data?: { files: SevenHolePrbFileInfo[]; validRange: PrbFileInfo['validRange'] }; error?: string }> =>
    invoke(probePath(probeId, 'importSevenHoleCalibrationCsv'), { innerFilePath, outerFilePaths }),

  clearInterpolator: async (probeId: ProbeId, probeType: TraversalProbeType): Promise<{ success: boolean; data?: { cleared: boolean }; error?: string }> =>
    invoke(probePath(probeId, 'clearInterpolator'), { probeType }),

  calculateRealtime: async (
    probeId: ProbeId,
    pressures: TraversalRealtimeInput,
    config?: TraversalTestConfig,
    probeType?: TraversalProbeType,
  ): Promise<{ success: boolean; data?: InterpolationResult; error?: string }> =>
    invoke<InterpolationResult>(probePath(probeId, 'calculateRealtime'), { pressures, config, ...(probeType ? { probeType } : {}) }),

  checkPreconditions: async (probeId: ProbeId, config?: TraversalTestConfig): Promise<{ success: boolean; data?: PreconditionCheckResult; error?: string }> =>
    invoke<PreconditionCheckResult>(probePath(probeId, 'checkPreconditions'), { config }),

  start: async (probeId: ProbeId, config: TraversalTestConfig): Promise<{ success: boolean; data?: { taskId: string }; error?: string }> =>
    invoke<{ taskId: string }>(probePath(probeId, 'start'), config),

  pause: async (probeId: ProbeId): Promise<{ success: boolean; error?: string }> => invoke(probePath(probeId, 'pause')),

  resume: async (probeId: ProbeId): Promise<{ success: boolean; error?: string }> => invoke(probePath(probeId, 'resume')),

  stop: async (probeId: ProbeId): Promise<{ success: boolean; error?: string }> => invoke(probePath(probeId, 'stop')),

  runPoint: async (probeId: ProbeId): Promise<{ success: boolean; error?: string }> => invoke(probePath(probeId, 'runPoint')),

  /** 终态关闭（registry CloseProbe；活动状态返回 409 already_running） */
  close: async (probeId: ProbeId): Promise<{ success: boolean; error?: string }> => invoke(probePath(probeId, 'close')),

  getResult: async (probeId: ProbeId, taskId: string): Promise<{ success: boolean; data?: TraversalTestStatus | null; error?: string }> =>
    invoke<TraversalTestStatus | null>(`${probePath(probeId, 'result')}?taskId=${encodeURIComponent(taskId)}`, undefined, 'GET'),

  getStatus: async (probeId: ProbeId, signal?: AbortSignal): Promise<{ success: boolean; data?: TraversalTestStatus | null; error?: string }> => {
    const res = await invoke<TraversalStatusRawResponse | null>(probePath(probeId, 'status'), undefined, 'GET', signal)
    if (!res.success || !res.data) return res as { success: boolean; data?: TraversalTestStatus | null; error?: string }
    return { success: true, data: mapStatusResponse(res.data) }
  },

  onStatus: (probeId: ProbeId, callback: (status: TraversalTestStatus) => void): (() => void) =>
    subscribeProbeStatus(probeId, (signal) => traversalProbeApi.getStatus(probeId, signal), {
      buildEvent: (status) => status,
      callback,
      lastKey: '',
    }),

  onProgress: (probeId: ProbeId, callback: (event: TraversalProgressEvent) => void): (() => void) =>
    subscribeProbeStatus(probeId, (signal) => traversalProbeApi.getStatus(probeId, signal), {
      buildEvent: (status) => {
        if (status.status !== 'running' && status.status !== 'paused') return null
        // 不含 timestamp / duration：每次轮询 Date.now() 都变化，破坏 lastKey 去重。
        return {
          taskId: status.taskId,
          totalPoints: status.totalPoints,
          completedPoints: status.completedPoints,
          currentPoint: status.currentPoint ?? { alpha: 0, beta: 0 },
          currentPointPhase: status.currentPointPhase,
          latestData: status.latestData,
          waitingForAcquisition: status.waitingForAcquisition,
          waitingDevices: status.waitingDevices,
          waitingForAcquisitionSinceMs: status.waitingForAcquisitionSinceMs,
        }
      },
      callback,
      lastKey: '',
    }),

  onComplete: (probeId: ProbeId, callback: (event: TraversalCompleteEvent) => void): (() => void) =>
    subscribeProbeStatus(probeId, (signal) => traversalProbeApi.getStatus(probeId, signal), {
      buildEvent: (status) => {
        if (!['completed', 'stopped', 'error'].includes(status.status)) return null
        // 不含 duration：每次轮询 Date.now() - startTime 都变化，破坏 lastKey 去重。
        return {
          taskId: status.taskId,
          success: status.status === 'completed',
          status: status.status as TraversalCompleteEvent['status'],
          totalPoints: status.totalPoints,
          filePath: status.csvPath,
          error: status.lastError,
        }
      },
      callback,
      lastKey: '',
    }),

  onError: (probeId: ProbeId, callback: (event: TraversalErrorEvent) => void): (() => void) =>
    subscribeProbeStatus(probeId, (signal) => traversalProbeApi.getStatus(probeId, signal), {
      buildEvent: (status) => {
        if (status.status !== 'error' || !status.lastError) return null
        const rawStatus = status as TraversalStatusRawResponse
        return {
          taskId: status.taskId,
          error: status.lastError,
          code: rawStatus.lastErrorCode ?? 'UNKNOWN',
          recoverable: false
        }
      },
      callback,
      lastKey: '',
    }),
}
