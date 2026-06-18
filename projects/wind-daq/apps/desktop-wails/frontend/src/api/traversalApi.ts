import { request } from '@api/http-client'
import type {
  CalibrationCsvFileInfo,
  InterpolationResult,
  MultiPrbInterpolationMode,
  PrbFileInfo,
  PreconditionCheckResult,
  TraversalCheckpoint,
  TraversalCompleteEvent,
  TraversalErrorEvent,
  TraversalErrorCode,
  TraversalInterpolationInput,
  TraversalProgressEvent,
  TraversalTestConfig,
  TraversalTestStatus,
} from '@shared/types/traversal'

const API_BASE = import.meta.env.VITE_API_BASE || 'http://127.0.0.1:8900'

function apiRequest<T>(path: string, init?: RequestInit): Promise<T> {
  const fullPath = path.startsWith('http') ? path : `${API_BASE}${path}`
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

async function invoke<T>(path: string, body?: unknown): Promise<{ success: boolean; data?: T; error?: string }> {
  try {
    const data = await apiRequest<T>(path, body === undefined
      ? undefined
      : { method: 'POST', body: JSON.stringify(body) })
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
  currentPointCoordinates?: { alpha: number; beta: number }
  currentPoint?: number | { alpha: number; beta: number }
  state?: string
  lastErrorCode?: TraversalErrorCode
  validationWarnings?: string[]
}

export const traversalApi = {
  getConfig: async (): Promise<{ success: boolean; data?: TraversalTestConfig | null; error?: string }> =>
    invoke<TraversalTestConfig | null>('/api/traversal/config'),

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

  calculateRealtime: async (
    pressures: TraversalInterpolationInput,
    config?: TraversalTestConfig,
  ): Promise<{ success: boolean; data?: InterpolationResult; error?: string }> =>
    invoke<InterpolationResult>('/api/traversal/calculateRealtime', { pressures, config }),

  checkPreconditions: async (config?: TraversalTestConfig): Promise<{ success: boolean; data?: PreconditionCheckResult; error?: string }> =>
    invoke<PreconditionCheckResult>('/api/traversal/checkPreconditions', { config }),

  start: async (config: TraversalTestConfig): Promise<{ success: boolean; data?: { taskId: string }; error?: string }> =>
    invoke<{ taskId: string }>('/api/traversal/start', config),

  pause: async (): Promise<{ success: boolean; error?: string }> => invoke('/api/traversal/pause'),

  resume: async (): Promise<{ success: boolean; error?: string }> => invoke('/api/traversal/resume'),

  stop: async (): Promise<{ success: boolean; error?: string }> => invoke('/api/traversal/stop'),

  /** 加载断点恢复信息（应用启动或进入遍历页面时调用，判断是否需要展示"恢复"横幅） */
  loadCheckpoint: async (): Promise<{ success: boolean; data?: TraversalCheckpoint | null; error?: string }> =>
    invoke<TraversalCheckpoint | null>('/api/traversal/loadCheckpoint'),

  /** 从断点恢复测试（复用原 taskId，从已完成点数继续） */
  resumeFromCheckpoint: async (checkpoint: TraversalCheckpoint): Promise<{ success: boolean; data?: { taskId: string }; error?: string }> =>
    invoke<{ taskId: string }>('/api/traversal/resumeFromCheckpoint', checkpoint),

  /** 清除断点文件（用户主动放弃恢复时调用） */
  clearCheckpoint: async (): Promise<{ success: boolean; error?: string }> =>
    invoke('/api/traversal/clearCheckpoint'),

  getStatus: async (): Promise<{ success: boolean; data?: TraversalTestStatus | null; error?: string }> => {
    const res = await invoke<TraversalStatusRawResponse | null>('/api/traversal/status')
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
      return {
        taskId: status.taskId,
        totalPoints: status.totalPoints,
        completedPoints: status.completedPoints,
        currentPoint: status.currentPoint ?? { alpha: 0, beta: 0 },
        currentPointPhase: status.currentPointPhase,
        latestData: status.latestData,
        timestamp: Date.now(),
      }
    }, callback),

  onComplete: (callback: (event: TraversalCompleteEvent) => void): (() => void) =>
    createPollingSubscription((status) => {
      if (!['completed', 'stopped', 'error'].includes(status.status)) return null
      return {
        taskId: status.taskId,
        success: status.status === 'completed',
        status: status.status as TraversalCompleteEvent['status'],
        totalPoints: status.totalPoints,
        error: status.lastError,
        duration: status.startTime ? Date.now() - status.startTime : 0,
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
