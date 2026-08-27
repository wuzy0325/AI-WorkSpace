import { onMounted, onUnmounted } from 'vue'
import { useEventHub } from '@/composables/useEventHub'
import { useMeasurementStore, MEASUREMENT_MAX_ROWS } from '@/stores/measurement'
import { useDeviceInventoryStore } from '@/stores/device/inventoryStore'
import type { MeasurementState, StabilityUpdate, AlarmData, CollectedRow } from '@/stores/measurement/types'
import type { MeasurementPoint } from '@/api/measurement'
import {
  fetchStabilityTimeoutPending,
} from '@/api/measurement'
import { multipressListDevices } from '@/api/multipress'
import {
  EVENT_MEASUREMENT_STATE_CHANGED,
  EVENT_MEASUREMENT_DATA_UPDATED,
  EVENT_MEASUREMENT_STABILITY_UPDATE,
  EVENT_MEASUREMENT_STABILITY_TIMEOUT,
  EVENT_MEASUREMENT_ALARM_TRIGGERED,
  EVENT_MEASUREMENT_ALARM_RESOLVED,
  EVENT_MEASUREMENT_POINT_STATUS,
  EVENT_MEASUREMENT_DATA_COLLECTED,
  EVENT_POINT_ERROR,
  EVENT_MULTIPRESS_PRESSURE_UPDATE,
} from '@/shared/events'
import { ElMessage } from 'element-plus'

/** 实时采样行攒批刷新间隔：高频 data_updated 事件先落缓冲，
 * 定时一次性写入 store，避免每个事件都触发表格全量重算。 */
const ROW_FLUSH_INTERVAL_MS = 500

/**
 * Composable that manages SSE event stream for measurement view.
 * Mirrors the calibration module's useCalibrationSync pattern:
 * SSE lifecycle is owned by the composable, not the store.
 */
export function useMeasurementSync() {
  const store = useMeasurementStore()
  const deviceStore = useDeviceInventoryStore()

  const { subscribe, registerPoll } = useEventHub()
  const unsubs: (() => void)[] = []
  const unregPolls: (() => void)[] = []
  let disposed = false

  // 实时行缓冲：非响应式普通数组，flush 时才合并进 store.rows。
  let rowBuffer: CollectedRow[] = []
  let rowFlushTimer: ReturnType<typeof setTimeout> | null = null

  function flushRowBuffer() {
    rowFlushTimer = null
    if (rowBuffer.length === 0) return
    const incoming = rowBuffer
    rowBuffer = []
    const merged = store.rows.concat(incoming)
    store.rows = merged.length > MEASUREMENT_MAX_ROWS
      ? merged.slice(merged.length - MEASUREMENT_MAX_ROWS)
      : merged
  }

  // 页面刷新/崩溃恢复：后端可能正阻塞等待用户对稳定超时的决定，
  // 而 stabilityTimeoutPending 只能由 SSE 事件置位——错过事件就必须主动查询。
  async function restorePendingDecisions() {
    try {
      const resp = await fetchStabilityTimeoutPending()
      if (resp.pending) store.stabilityTimeoutPending = true
    } catch { /* 静默 */ }
  }

  // 打压设备压力一次性快照：后端 pressure.update 已去重（值无变化不推送），
  // 迟到的订阅者拿不到"最后一次"值，挂载时主动拉一次补齐，后续靠 SSE 增量更新。
  async function loadMultipressSnapshot() {
    try {
      const states = await multipressListDevices()
      for (const s of states) {
        deviceStore.updateDevicePressure(s.deviceId, s.currentPressure)
      }
    } catch { /* 静默 */ }
  }

  onMounted(async () => {
    await Promise.all([
      store.loadAlarmConfig(),
      store.fetchCurrentState(),
      store.loadPoints(),
      store.refreshData(),
      store.refreshAlarmPending(),
      restorePendingDecisions(),
      loadMultipressSnapshot()
    ])
    if (disposed) return

    unsubs.push(subscribe(EVENT_MEASUREMENT_STATE_CHANGED, (payload) => {
      const newState = (payload.data as { state: MeasurementState }).state
      store.syncState(newState)
      // 进入打压状态时清除旧报警标记，避免上个点的报警残留到当前点
      if (newState === 'pressurizing') {
        store.alarmData = null
      }
    }))

    unsubs.push(subscribe(EVENT_MEASUREMENT_DATA_UPDATED, (payload) => {
      const data = payload.data as { timestamp: string; deviceId?: string; channels: Record<string, number> }
      rowBuffer.push({ timestamp: data.timestamp, deviceId: data.deviceId, channels: data.channels })
      if (!rowFlushTimer) {
        rowFlushTimer = setTimeout(flushRowBuffer, ROW_FLUSH_INTERVAL_MS)
      }
    }))

    unsubs.push(subscribe(EVENT_MEASUREMENT_STABILITY_UPDATE, (payload) => {
      store.stabilityState = payload.data as StabilityUpdate
    }))

    unsubs.push(subscribe(EVENT_MEASUREMENT_ALARM_TRIGGERED, (payload) => {
      store.alarmPending = true
      store.alarmData = payload.data as AlarmData
    }))

    unsubs.push(subscribe(EVENT_MEASUREMENT_STABILITY_TIMEOUT, () => {
      store.stabilityTimeoutPending = true
    }))

    unsubs.push(subscribe(EVENT_MEASUREMENT_ALARM_RESOLVED, () => {
      store.alarmPending = false
      store.alarmData = null
    }))

    unsubs.push(subscribe(EVENT_MEASUREMENT_POINT_STATUS, (payload) => {
      const updated = payload.data as MeasurementPoint
      const idx = store.points.findIndex(p => p.id === updated.id)
      if (idx >= 0) store.points[idx] = updated
    }))

    unsubs.push(subscribe(EVENT_MEASUREMENT_DATA_COLLECTED, (payload) => {
      const collected = payload.data as {
        pointIndex: number
        channels: number[]
        deviceId?: string
        data: number[]
      }
      const ptIdx = store.points.findIndex(p => p.index === collected.pointIndex)
      if (ptIdx < 0) return
      const pt = store.points[ptIdx]
      // 多设备：按设备写入 collectedByDevice；单设备回退 collectedData（兼容旧字段）。
      if (collected.deviceId) {
        const byDevice = { ...(pt.collectedByDevice ?? {}) }
        byDevice[collected.deviceId] = {
          deviceId: collected.deviceId,
          collected: collected.data,
          status: 'completed'
        }
        store.points[ptIdx] = { ...pt, collectedByDevice: byDevice, status: 'completed' }
      } else {
        store.points[ptIdx] = { ...pt, collectedData: collected.data, status: 'completed' }
      }
    }))

    unsubs.push(subscribe(EVENT_POINT_ERROR, (payload) => {
      // 设备级实时采样异常（多设备场景单台故障不拖垮整批）：
      // 提示用户该设备异常，可考虑在报警弹窗中跳过该设备。
      const data = payload.data as { deviceId?: string; error?: string } | undefined
      if (data?.deviceId && data?.error) {
        ElMessage.warning(`设备 ${data.deviceId} 实时采样异常：${data.error}`)
      }
    }))

    unsubs.push(subscribe(EVENT_MULTIPRESS_PRESSURE_UPDATE, (payload) => {
      const data = payload.data as Record<string, unknown>
      const deviceId = data?.deviceId as string | undefined
      const pressure = data?.currentPressure as number | undefined
      if (deviceId && typeof pressure === 'number') {
        store.updateDevicePressure(deviceId, pressure)
        deviceStore.updateDevicePressure(deviceId, pressure)
      }
    }))

    unregPolls.push(registerPoll('measurement-running', async () => {
      if (store.isRunning) {
        await Promise.all([
          store.refreshPressure(),
          store.refreshStability(),
          store.refreshMeasureData()
        ])
      }
    }, 2000))

    // 注：不再注册 1s 的 multipressListDevices 轮询。
    // 打压设备压力已由后端 SSE（multipress.pressure.update）推送，
    // 挂载时另有一次性快照补齐，重复轮询只会放大事件量。
  })

  onUnmounted(() => {
    disposed = true
    for (const unsub of unsubs) unsub()
    for (const unreg of unregPolls) unreg()
    if (rowFlushTimer) {
      clearTimeout(rowFlushTimer)
      rowFlushTimer = null
    }
    rowBuffer = []
  })
}
