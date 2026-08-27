/**
 * 设备管理面板的轮询与 SSE 事件订阅逻辑。
 * 封装自动刷新、事件流订阅、生命周期管理，与 UI 组件解耦。
 */
import { onMounted, onUnmounted, ref, watch } from 'vue'
import {
  fetchDeviceConnectConfig,
  fetchDevices,
  fetchUnitConsistency
} from "@/api/device"
import { createEventStream } from "@/api/client"
import { EVENT_DEVICE_CONNECT_PROGRESS } from "@/shared/events"
import type {
  DeviceConnectConfigDTO,
  DeviceDTO,
  DeviceStatusChangedEventData
} from "@/types/device"
import type { StreamEventPayload } from "@/types/api"

export interface UseDevicePollingOptions {
  /** 轮询间隔（毫秒），默认 3000 */
  intervalMs?: number
  /** 收到设备状态变更事件时的回调 */
  onDeviceStatusChanged?: (data: DeviceStatusChangedEventData) => void
  /** 收到连接进度事件时的回调 */
  onConnectProgress?: (deviceId: string, message: string) => void
  /** 设备列表刷新完成后的回调 */
  onRefreshed?: (data: {
    devices: DeviceDTO[]
    config: DeviceConnectConfigDTO | null
    unitConsistent: boolean
    unitStatusText: string
  }) => void
  /** 错误回调 */
  onError?: (error: Error) => void
}

export function useDevicePolling(options: UseDevicePollingOptions = {}) {
  const {
    intervalMs = 3000,
    onDeviceStatusChanged,
    onConnectProgress,
    onRefreshed,
    onError
  } = options

  const autoRefresh = ref(true)
  const lastRefreshAt = ref<Date | null>(null)

  let pollTimer: ReturnType<typeof setInterval> | null = null
  let eventSource: EventSource | null = null

  // ---- 方法 ----

  /** 执行一次完整的设备数据刷新 */
  async function refreshAll() {
    try {
      const [list, consistency, policy] = await Promise.all([
        fetchDevices(),
        fetchUnitConsistency(),
        fetchDeviceConnectConfig()
      ])
      lastRefreshAt.value = new Date()
      onRefreshed?.({
        devices: list,
        config: policy,
        unitConsistent: consistency.consistent,
        unitStatusText: consistency.consistent ? '一致' : '冲突'
      })
    } catch (error) {
      onError?.(error instanceof Error ? error : new Error('刷新设备状态失败'))
    }
  }

  // ---- SSE 事件处理 ----

  function isDeviceStatusChangedEventData(data: unknown): data is DeviceStatusChangedEventData {
    if (!data || typeof data !== 'object') return false
    const candidate = data as Record<string, unknown>
    return typeof candidate.id === 'string'
  }

  function startEventStream() {
    if (eventSource) return

    eventSource = createEventStream({
      onEvent: (payload: StreamEventPayload) => {
        if (payload.type === 'device.status.changed') {
          if (isDeviceStatusChangedEventData(payload.data)) {
            onDeviceStatusChanged?.(payload.data)
          } else {
            void refreshAll()
          }
        }
        if (payload.type === EVENT_DEVICE_CONNECT_PROGRESS) {
          const data = payload.data as { deviceId?: string; message?: string }
          if (data.deviceId && data.message) {
            onConnectProgress?.(data.deviceId, data.message)
          }
        }
      },
      onError: (error) => {
        console.warn('[DeviceManagementPanel] SSE 连接断开:', error)
      }
    })
  }

  function stopEventStream() {
    if (!eventSource) return
    eventSource.close()
    eventSource = null
  }

  function startPolling() {
    if (pollTimer) return
    pollTimer = setInterval(() => {
      void refreshAll()
    }, intervalMs)
  }

  function stopPolling() {
    if (!pollTimer) return
    clearInterval(pollTimer)
    pollTimer = null
  }

  watch(autoRefresh, (enabled) => {
    if (enabled) {
      startPolling()
    } else {
      stopPolling()
    }
  })

  onMounted(() => {
    void refreshAll()
    startPolling()
    startEventStream()
  })

  onUnmounted(() => {
    stopPolling()
    stopEventStream()
  })

  return {
    /** 自动刷新开关 */
    autoRefresh,
    /** 最后刷新时间 */
    lastRefreshAt,
    /** 手动触发一次刷新 */
    refreshAll
  }
}