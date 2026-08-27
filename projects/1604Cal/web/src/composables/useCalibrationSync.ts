import { onMounted, onUnmounted, ref, type Ref, type InjectionKey } from 'vue'
import { ElMessage } from 'element-plus'
import { useEventHub } from '@/composables/useEventHub'
import { connectDevice } from '@/api/device'
import { bindMeasureDevice } from '@/api/session'
import type { SessionState } from '@/types/calibration'
import type { DevicePointData } from '@/stores/calibration/types'
import { useCalibrationStore } from '@/stores/calibration'
import { useDeviceInventoryStore } from '@/stores/device/inventoryStore'
import {
  EVENT_SESSION_STATE_CHANGED,
  EVENT_DEVICE_STATUS_CHANGED,
  EVENT_CALIBRATION_STABILITY_PREFIX,
  EVENT_ALARM_TRIGGERED,
  EVENT_CALIBRATION_ALARM_RESOLVED,
  EVENT_CALIBRATION_POINT_STATUS,
  EVENT_MULTIPRESS_PRESSURE_UPDATE,
  EVENT_AUTO_COLLECTION_COMPLETED,
} from '@/shared/events'
import { multipressListDevices } from '@/api/multipress'

// 稳定性 SSE 事件数据结构
export interface StabilityEventData {
  isStable: boolean
  isInRange: boolean
  currentValue: number
  targetValue: number
  deviation: number
  stableDurationMs: number
  requiredDurationMs: number
  progress: number
}

// 报警 SSE 事件数据结构
export interface AlarmEventData {
  pointIndex: number
  targetPressure: number
  overLimitChannels: number[]
  maxDeviation: number
  channelDetails: Record<string, number>
  deviceId?: string
  error?: string
}

// 稳定性状态的 provide/inject key，供 CalibrationControl 获取
export const stabilityStatusKey: InjectionKey<Ref<StabilityEventData | null>> = Symbol('stabilityStatus')

// 报警事件的 provide/inject key，供 CalibrationControl 获取当前报警对应的设备 ID
export const alarmEventKey: InjectionKey<Ref<AlarmEventData | null>> = Symbol('alarmEvent')

/**
 * Composable that manages SSE event stream and polling for calibration view.
 * Automatically starts on mount and cleans up on unmount.
 */
export function useCalibrationSync() {
  const calibrationStore = useCalibrationStore()
  const deviceStore = useDeviceInventoryStore()

  let bindingInProgress = false
  let boundMeasureId = ''
  let lastRepairAttemptAt = 0

  // 稳定性状态
  const stabilityStatus = ref<StabilityEventData | null>(null)
  // 报警事件状态
  const alarmEvent = ref<AlarmEventData | null>(null)

  const { subscribe, subscribeGlobal, registerPoll } = useEventHub()
  const unsubs: (() => void)[] = []
  const unregPolls: (() => void)[] = []

  // 绑定计量设备并刷新阀门/单位信息。
  // 兼容“设备状态显示已连接但会话驱动未就绪”的场景，必要时静默触发一次重连修复。
  async function bindConnectedMeasureDevice() {
    if (bindingInProgress) return

    const connectedMeasure = deviceStore.measureDevices.find(d => d.status === 'connected')
    if (!connectedMeasure) {
      boundMeasureId = ''
      return
    }

    // 若当前设备已完成绑定且阀门/单位信息已可读，跳过重复绑定。
    if (
      boundMeasureId === connectedMeasure.id &&
      calibrationStore.valveStatus !== '' &&
      calibrationStore.measureUnit !== ''
    ) {
      return
    }

    bindingInProgress = true
    try {
      try {
        // 显式以 'calibration' 身份绑定：本 composable 只在标定视图挂载，
        // 若使用默认 moduleName='measurement' 会把会话所有权翻转成计量模块，
        // 与抽屉的整批 'calibration' 绑定互相冲突（断开重连后触发绑定失败）。
        await bindMeasureDevice(connectedMeasure.id, 'calibration')
      } catch {
        // 绑定失败由后续读取结果决定是否进入修复流程
      }

      let loaded = await calibrationStore.refreshDeviceInfo({ retries: 3, retryDelayMs: 500 })
      if (!loaded) {
        const now = Date.now()
        // 避免高频重连，最多每 15 秒尝试一次静默修复。
        if (now - lastRepairAttemptAt >= 15000) {
          lastRepairAttemptAt = now
          try {
            await connectDevice(connectedMeasure.id)
          } catch {
            // 静默修复失败，不弹窗；等待下一轮刷新重试
          }

          await Promise.all([
            deviceStore.loadDevices().then(() => bindMeasureDevice(connectedMeasure.id, 'calibration').catch(() => {})),
            calibrationStore.refreshDeviceInfo({ retries: 3, retryDelayMs: 500 }).then(r => { loaded = r })
          ])
        }
      }

      if (loaded) {
        boundMeasureId = connectedMeasure.id
      }
    } finally {
      bindingInProgress = false
    }
  }

  onMounted(async () => {
    await Promise.all([
      deviceStore.loadDevices(),
      calibrationStore.fetchCurrentSessionState()
    ])

    await bindConnectedMeasureDevice()

    unsubs.push(subscribe(EVENT_SESSION_STATE_CHANGED, (payload) => {
      const data = payload.data as { state: SessionState }
      if (data?.state) {
        calibrationStore.syncSessionState(data.state)
      }
    }))

    unsubs.push(subscribe(EVENT_DEVICE_STATUS_CHANGED, () => {
      void deviceStore.loadDevices().then(bindConnectedMeasureDevice)
    }))

    unsubs.push(subscribe(EVENT_CALIBRATION_POINT_STATUS, (payload) => {
      const data = payload.data as { index?: number; status?: string; collectedData?: number[]; collectedByDevice?: Record<string, DevicePointData>; actualPressure?: number }
      if (typeof data?.index === 'number') {
        const point = calibrationStore.pressurePoints.find(p => p.index === data.index)
        if (point) {
          if (data.status) point.status = data.status as typeof point.status
          if (data.collectedData) point.collectedData = data.collectedData
          if (data.collectedByDevice) point.collectedByDevice = data.collectedByDevice
          if (data.actualPressure !== undefined) point.actualPressure = data.actualPressure
        }
      }
    }))

    // 稳定性事件使用前缀匹配（calibration.stability.*）
    unsubs.push(subscribeGlobal((payload) => {
      if (payload.type?.startsWith(EVENT_CALIBRATION_STABILITY_PREFIX)) {
        stabilityStatus.value = payload.data as StabilityEventData
      }
    }))

    unsubs.push(subscribe(EVENT_ALARM_TRIGGERED, (payload) => {
      alarmEvent.value = payload.data as AlarmEventData
    }))

    unsubs.push(subscribe(EVENT_CALIBRATION_ALARM_RESOLVED, () => {
      alarmEvent.value = null
    }))

    unsubs.push(subscribe(EVENT_AUTO_COLLECTION_COMPLETED, () => {
      ElMessage.success('所有压力点采集完成，系统已自动排空')
    }))

    unsubs.push(subscribe(EVENT_MULTIPRESS_PRESSURE_UPDATE, (payload) => {
      const data = payload.data as { deviceId?: string; currentPressure?: number }
      if (data?.deviceId && typeof data.currentPressure === 'number') {
        deviceStore.updateDevicePressure(data.deviceId, data.currentPressure)
      }
    }))

    unregPolls.push(registerPoll('calibration-running', async () => {
      if (calibrationStore.isRunning) {
        await Promise.all([
          calibrationStore.refreshPressure(),
          calibrationStore.refreshStability(),
          calibrationStore.refreshMeasureData()
        ])
      }
    }, 2000))

    unregPolls.push(registerPoll('calibration-device', async () => {
      await deviceStore.loadDevices()
      await bindConnectedMeasureDevice()
    }, 5000))

    unregPolls.push(registerPoll('calibration-pressure', async () => {
      try {
        const states = await multipressListDevices()
        for (const s of states) {
          deviceStore.updateDevicePressure(s.deviceId, s.currentPressure)
        }
      } catch {
        // 静默失败
      }
    }, 1000))
  })

  onUnmounted(() => {
    for (const unsub of unsubs) unsub()
    for (const unreg of unregPolls) unreg()
  })

  return { stabilityStatus, alarmEvent }
}
