import type { ProbeId, TraversalTestStatus } from '@shared/types/traversal'

/**
 * 双探针 keyed polling 调度器（spec FR6 / Task 16）。
 *
 * 每个 probe ID 一个独立轮询 channel，供该 probe 的 progress/complete/error
 * 订阅复用（双路共 2 个 channel，500ms 各一次，最多 4 req/s 基线）：
 *   - 首个 subscriber 注册后立即请求一次，此后按 500ms 调度；
 *   - 每个 channel 同时最多一个 in-flight status 请求：上一请求未结束时
 *     新的 tick 直接跳过，不叠加请求；
 *   - 最后一个 subscriber 注销时停止该 probe 的 timer 并中止 in-flight 请求；
 *     停止一路不得影响另一路；
 *   - AbortController + generation 双重防护：取消订阅、模式切换或新任务启动
 *     （invalidate）后到达的旧响应一律丢弃。
 */

const POLL_INTERVAL_MS = 500

export interface ProbePollingEntry<T> {
  buildEvent: (status: TraversalTestStatus) => T | null
  callback: (event: T) => void
  lastKey: string
}

export type ProbeStatusFetcher = (signal: AbortSignal) => Promise<{ success: boolean; data?: TraversalTestStatus | null; error?: string }>

interface ProbePollingChannel {
  subscribers: Set<ProbePollingEntry<unknown>>
  timer: number | null
  inFlight: boolean
  generation: number
  abort: AbortController | null
}

const channels = new Map<ProbeId, ProbePollingChannel>()

function channelFor(probeId: ProbeId): ProbePollingChannel {
  let channel = channels.get(probeId)
  if (!channel) {
    channel = { subscribers: new Set(), timer: null, inFlight: false, generation: 0, abort: null }
    channels.set(probeId, channel)
  }
  return channel
}

async function pollOnce(probeId: ProbeId, fetchStatus: ProbeStatusFetcher): Promise<void> {
  const channel = channelFor(probeId)
  // in-flight 上限：上一请求未结束时跳过本次 tick（spec FR6）。
  if (channel.inFlight || channel.subscribers.size === 0) return
  channel.inFlight = true
  const generation = channel.generation
  const abort = new AbortController()
  channel.abort = abort
  try {
    const res = await fetchStatus(abort.signal)
    // generation 校验：取消订阅/invalidate 后到达的旧响应一律丢弃。
    if (abort.signal.aborted || generation !== channel.generation) return
    if (!res.success || !res.data) return
    dispatchToSubscribers(channel, res.data)
  } finally {
    // 仅当本轮未被中止时才复位 inFlight；中止路径由 stopTimer 统一清理。
    if (!abort.signal.aborted && generation === channel.generation) {
      channel.inFlight = false
      channel.abort = null
    }
  }
}

function dispatchToSubscribers(channel: ProbePollingChannel, status: TraversalTestStatus): void {
  for (const entry of channel.subscribers) {
    const event = entry.buildEvent(status)
    if (!event) continue
    const key = JSON.stringify(event)
    if (key === entry.lastKey) continue
    entry.lastKey = key
    try {
      entry.callback(event)
    } catch (err) {
      console.error('[traversalPolling] subscriber callback failed:', err)
    }
  }
}

function scheduleTimer(probeId: ProbeId, fetchStatus: ProbeStatusFetcher): void {
  const channel = channelFor(probeId)
  if (channel.timer !== null) return
  channel.timer = window.setInterval(() => { void pollOnce(probeId, fetchStatus) }, POLL_INTERVAL_MS)
}

function stopChannel(channel: ProbePollingChannel): void {
  if (channel.timer !== null) {
    window.clearInterval(channel.timer)
    channel.timer = null
  }
  // 中止 in-flight 请求并代际失效，丢弃其迟到响应。
  channel.generation += 1
  channel.inFlight = false
  channel.abort?.abort()
  channel.abort = null
}

/**
 * 订阅指定 probe 的状态轮询。返回注销函数：
 * 首个 subscriber 立即请求一次；最后一个注销停止该 probe 的 timer。
 */
export function subscribeProbeStatus<T>(
  probeId: ProbeId,
  fetchStatus: ProbeStatusFetcher,
  entry: ProbePollingEntry<T>,
): () => void {
  const channel = channelFor(probeId)
  channel.subscribers.add(entry as ProbePollingEntry<unknown>)
  if (channel.subscribers.size === 1) {
    // 首个 subscriber：立即请求一次，再按 500ms 调度。
    void pollOnce(probeId, fetchStatus)
    scheduleTimer(probeId, fetchStatus)
  }
  return () => {
    channel.subscribers.delete(entry as ProbePollingEntry<unknown>)
    if (channel.subscribers.size === 0) {
      stopChannel(channel)
    }
  }
}

/**
 * 使指定 probe 的轮询代际失效（模式切换 / 新任务启动）：
 * 中止 in-flight 请求并丢弃其响应；订阅关系与 timer 保留。
 */
export function invalidateProbePolling(probeId: ProbeId): void {
  const channel = channelFor(probeId)
  channel.generation += 1
  channel.inFlight = false
  channel.abort?.abort()
  channel.abort = null
}

/** 测试专用：查询 channel 内部状态（in-flight 上限断言用）。 */
export function probePollingSnapshot(probeId: ProbeId): { subscribers: number; inFlight: boolean; timerRunning: boolean; generation: number } {
  const channel = channels.get(probeId)
  return {
    subscribers: channel?.subscribers.size ?? 0,
    inFlight: channel?.inFlight ?? false,
    timerRunning: channel?.timer != null,
    generation: channel?.generation ?? 0,
  }
}

/** 测试专用：重置全部 channel（避免用例间状态泄漏）。 */
export function resetProbePolling(): void {
  for (const channel of channels.values()) {
    stopChannel(channel)
    channel.subscribers.clear()
  }
  channels.clear()
}
