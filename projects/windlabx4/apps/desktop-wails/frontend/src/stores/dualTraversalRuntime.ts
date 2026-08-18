import { deviceApi } from '@api/deviceApi'
import type { DataPayload } from '@api/types'
import type { ProbeId, TraversalRealtimeInput, TraversalTestConfig } from '@shared/types/traversal'

const deviceRefCounts = new Map<string, number>()

export interface DualTraversalSessionRuntime {
  requestId: number
  lifecycleGeneration: number
  // 订阅拆分为两类（C9 修复）：
  // - pollingUnsubscribers: onStatus/onProgress/onComplete/onError，500ms 轮询类，
  //   终态后调用 teardownPollingSubscriptions 停止，避免无意义轮询浪费 CPU/网络。
  // - snapshotUnsubscribers: onSnapshot（DAQ 实时快照），终态后保留，
  //   让用户在结果界面仍能看到当前压力值，直到显式 close/reset。
  pollingUnsubscribers: Array<() => void>
  snapshotUnsubscribers: Array<() => void>
  realtimeTimer: ReturnType<typeof setTimeout> | null
  lastRealtimeAt: number
  pendingInput: TraversalRealtimeInput | null
  // I-20 修复：in-flight 守卫，标记 calculateRealtime 是否在飞。
  // 高频 snapshot 在节流间隙到来时，若上一个请求尚未返回，直接跳过本次触发，
  // 避免后端并发请求堆积与返回顺序不确定导致结果闪烁。
  realtimeInFlight: boolean
  // I-17 修复：pause/resume 乐观更新窗口截止时间戳（ms）。
  // 在此窗口内 onStatus 回调保留当前 status.state/status.status，
  // 仅合并其他字段（totalPoints/completedPoints/currentPoint 等），
  // 避免陈旧轮询把刚切换的 paused/running 状态回退造成 UI 闪烁。
  // 0 表示无乐观窗口，onStatus 正常覆写。
  optimisticStatusUntil: number
  subscribedDeviceIds: string[]
  latestSnapshots: Map<string, DataPayload>
}

export function createDualTraversalRuntime(): DualTraversalSessionRuntime {
  return {
    requestId: 0,
    lifecycleGeneration: 0,
    pollingUnsubscribers: [],
    snapshotUnsubscribers: [],
    realtimeTimer: null,
    lastRealtimeAt: 0,
    pendingInput: null,
    realtimeInFlight: false,
    optimisticStatusUntil: 0,
    subscribedDeviceIds: [],
    latestSnapshots: new Map(),
  }
}

export function uniqueTraversalDeviceIds(config: TraversalTestConfig | null): string[] {
  if (!config) return []
  const ids = new Set<string>()
  for (const channel of config.channels?.probeChannels ?? []) {
    if (channel.enabled !== false && channel.channel?.deviceId) ids.add(channel.channel.deviceId)
  }
  return Array.from(ids).sort()
}

export function acquireDualTraversalDevice(deviceId: string): void {
  const count = deviceRefCounts.get(deviceId) ?? 0
  if (count === 0) deviceApi.subscribeToDevice(deviceId)
  deviceRefCounts.set(deviceId, count + 1)
}

export function releaseDualTraversalDevice(deviceId: string): void {
  const count = (deviceRefCounts.get(deviceId) ?? 0) - 1
  if (count <= 0) {
    deviceRefCounts.delete(deviceId)
    deviceApi.unsubscribeFromDevice(deviceId)
    return
  }
  deviceRefCounts.set(deviceId, count)
}

export function dualDeviceRefCount(deviceId: string): number {
  return deviceRefCounts.get(deviceId) ?? 0
}

export function resetDualDeviceRefCounts(): void {
  deviceRefCounts.clear()
}

export type DualTraversalRuntimes = Record<ProbeId, DualTraversalSessionRuntime>
