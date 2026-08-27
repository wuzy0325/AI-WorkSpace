import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import type { ActionResult } from '@/types/api'
import {
  triggerSessionAction,
  fetchSessionState,
  setCalibrationConfig,
  fitData as apiFitData,
  resolveAlarm as apiResolveAlarm,
  skipCalibrationDevice,
  type AlarmConfigPayload
} from "@/api/calibration"
import { type SessionState, ControlMode } from "@/types/calibration"
import { useDeviceInventoryStore } from '@/stores/device/inventoryStore'
import { usePressurePointStore } from './pressurePoints'
import { useDeviceControlStore } from './deviceControl'
import { useGatesStore } from '@/stores/app/gates'
import { sessionStateToStep, isSessionRunning } from '@/composables/useCalibrationFlow'
import { useCalibrationConfig } from '@/composables/useCalibrationConfig'
import { CalibrationStep } from './types'
import type { PrimaryAction, SecondaryAction } from './types'
import { fetchUnitConsistency } from '@/api/device'

export { CalibrationStep } from './types'
export type { PressurePoint, CalibrationParams, PrimaryAction, SecondaryAction } from './types'

export const useCalibrationStore = defineStore('calibration', () => {
  const deviceStore = useDeviceInventoryStore()
  const pressurePointStore = usePressurePointStore()
  const deviceControlStore = useDeviceControlStore()
  const calibrationConfig = useCalibrationConfig()

  // State (own)
  const currentStep = ref(CalibrationStep.DEVICE_CONNECT)
  const isCollecting = ref(false)
  const currentCollectingPoint = ref(0)
  const sessionState = ref<SessionState>('idle')
  const controlMode = ref<ControlMode>(ControlMode.Auto)
  const alarmConfig = ref<AlarmConfigPayload>({
    enabled: true,
    precisionThreshold: 5.0,
    soundEnabled: true,
    confirmOnAlarm: true,
    enabledChannels: []
  })

  // State (delegated from composable - these are Refs)
  const selectedChannels = calibrationConfig.selectedChannels
  const calibrationParams = calibrationConfig.calibrationParams

  // Getters
  const device1604Connected = computed(() => deviceControlStore.device1604Connected)
  const pressDeviceConnected = computed(() => deviceControlStore.pressDeviceConnected)
  const channelsSelected = computed(() => selectedChannels.value.length > 0)
  const hasCollectedData = computed(() => pressurePointStore.hasCollectedData)
  // 所有压力点是否已采集完成（completed 或 skipped）。用于在 point_done 会话态下
  // 区分"单点完成、流程仍在进行"与"全部点采集完毕、等待拟合"。
  const allPointsCollected = computed(() => {
    const pts = pressurePointStore.pressurePoints
    return pts.length > 0 && pts.every(p => p.status === 'completed' || p.status === 'skipped')
  })
  const valveReady = computed(() => deviceControlStore.valveStatus === 'calibration')
  // 阀门=校准模式是标定与计量启动的必要条件。
  // 开关由 gate store 从后端 /api/v1/config/gates 拉取，避免前端硬编码短路后端配置。
  const gatesStore = useGatesStore()
  const enforceValveCalibrationGate = computed(() => gatesStore.enforceValveCalibrationGate)

  // 前端阀门门禁统一开关：true 表示严格门禁。
  const canStartCalibration = computed(() =>
    device1604Connected.value && channelsSelected.value && (!enforceValveCalibrationGate.value || valveReady.value)
  )
  const isRunning = computed(() => isSessionRunning(sessionState.value))

  /* ── 动态状态机：主按钮随会话状态切换文案/图标/色阶 ──
     设计意图：让用户视线焦点稳定在主按钮位置，避免静态罗列 6 个按钮
     造成"哪个能用"的认知负担。副按钮仅在状态确实需要时才出现。 */
  const primaryAction = computed<PrimaryAction>(() => {
    switch (sessionState.value) {
      case 'idle':
      case 'ready':
      case 'stopped':
        return { key: 'start', label: '开始标定', icon: 'VideoPlay', variant: 'mint' }
      case 'pressurizing':
      case 'stabilizing':
      case 'collecting':
      case 'await_manual_collect':
      case 'recovering':
        return { key: 'pause', label: '暂停', icon: 'VideoPause', variant: 'slate' }
      case 'point_done':
        // 全部压力点采集完成后主按钮切换为"拟合"，引导用户进入拟合流程；
        // 采集中途的单点完成（下一点尚未打压）仍保留"暂停"。
        if (allPointsCollected.value && hasCollectedData.value) {
          return { key: 'fit', label: '拟合', icon: 'DataAnalysis', variant: 'mint' }
        }
        if (allPointsCollected.value) {
          // 全部点位被跳过、无数据可拟合：引导重新开始
          return { key: 'reset', label: '重新开始', icon: 'RefreshRight', variant: 'slate' }
        }
        return { key: 'pause', label: '暂停', icon: 'VideoPause', variant: 'slate' }
      case 'paused':
        return { key: 'resume', label: '继续标定', icon: 'RefreshRight', variant: 'mint' }
      case 'await_alarm_resolution':
        // 报警态主按钮：最常见的"确认继续"，副按钮提供跳过/重采/停止
        return { key: 'alarm-continue', label: '确认继续', icon: 'CircleCheck', variant: 'amber' }
      case 'fitting':
        // 拟合进行中：主按钮显示状态，禁用点击
        return { key: 'fitting', label: '拟合中...', icon: 'DataAnalysis', variant: 'slate' }
      case 'completed':
        // 已完成：主按钮是结束标定（清理会话），重新拟合作为副按钮
        return { key: 'end', label: '结束标定', icon: 'CircleClose', variant: 'mint' }
      case 'error':
        return { key: 'stop', label: '停止', icon: 'CloseBold', variant: 'amber' }
      default:
        return { key: 'start', label: '开始标定', icon: 'VideoPlay', variant: 'mint' }
    }
  })

  const secondaryActions = computed<SecondaryAction[]>(() => {
    const out: SecondaryAction[] = []
    switch (sessionState.value) {
      case 'pressurizing':
      case 'stabilizing':
      case 'collecting':
      case 'await_manual_collect':
      case 'recovering':
      case 'paused':
        out.push({ key: 'stop', label: '停止', variant: 'red', confirm: '确认终止标定？已采集数据将保留。' })
        break
      case 'point_done':
        // 全部点位采集完成：提供"重新开始"；否则仍提供"停止"以便中断流程。
        if (allPointsCollected.value) {
          if (hasCollectedData.value) {
            out.push({ key: 'reset', label: '重新开始', variant: 'slate', confirm: '将清空当前结果，重新开始标定？' })
          }
        } else {
          out.push({ key: 'stop', label: '停止', variant: 'red', confirm: '确认终止标定？已采集数据将保留。' })
        }
        break
      case 'await_alarm_resolution':
        // 报警态副按钮：跳过此点 / 重新采集 / 停止标定
        out.push({ key: 'alarm-skip', label: '跳过此点', variant: 'blue' })
        out.push({ key: 'alarm-recollect', label: '重新采集', variant: 'slate' })
        out.push({ key: 'alarm-stop', label: '停止标定', variant: 'red', confirm: '确认终止标定？已采集数据将保留。' })
        break
      case 'completed':
        // 已完成态副按钮：重新拟合 / 重新开始
        if (hasCollectedData.value) {
          out.push({ key: 'fit', label: '重新拟合', variant: 'blue' })
        }
        out.push({ key: 'reset', label: '重新开始', variant: 'slate', confirm: '将清空当前结果，重新开始标定？' })
        break
      case 'stopped':
        out.push({ key: 'reset', label: '清空数据', variant: 'slate', confirm: '将永久删除当前标定结果？' })
        break
      case 'error':
        out.push({ key: 'stop', label: '停止', variant: 'red', confirm: '确认终止当前会话？' })
        break
    }
    return out
  })

  // Actions
  const setStep = (step: CalibrationStep) => { currentStep.value = step }

  const syncSessionState = (state: SessionState) => {
    sessionState.value = state
    currentStep.value = sessionStateToStep(state)
    isCollecting.value = isSessionRunning(state)
  }

  const fetchCurrentSessionState = async () => {
    try {
      const data = await fetchSessionState()
      syncSessionState(data.state)
    } catch (error) { console.error('获取会话状态失败:', error) }
  }

  // 连接1604计量设备：单台走原逻辑；多台整批连接并绑定。
  const connectDevice1604 = async (deviceId: string | string[]): Promise<ActionResult> => {
    const ids = Array.isArray(deviceId) ? deviceId : [deviceId]
    const result = ids.length > 1
      ? await deviceControlStore.connectMeasureDevices(ids)
      : await deviceControlStore.connectDevice1604(ids[0])
    if (result.ok && deviceControlStore.device1604Connected) {
      setStep(CalibrationStep.CHANNEL_SELECT)
    }
    return result
  }

  // 断开1604计量设备：单台走原逻辑；多台逐台断开。
  const disconnectDevice1604 = async (deviceId: string | string[]): Promise<ActionResult> => {
    const ids = Array.isArray(deviceId) ? deviceId : [deviceId]
    const result = ids.length > 1
      ? await deviceControlStore.disconnectMeasureDevices(ids)
      : await deviceControlStore.disconnectDevice1604(ids[0])
    if (!deviceControlStore.device1604Connected) {
      setStep(CalibrationStep.DEVICE_CONNECT)
    }
    return result
  }

  const connectPressDevice = async (deviceId: string): Promise<ActionResult> => {
    const result = await deviceControlStore.connectPressDevice(deviceId)
    if (deviceControlStore.device1604Connected && deviceControlStore.pressDeviceConnected) {
      setStep(CalibrationStep.CHANNEL_SELECT)
    }
    return result
  }

  const disconnectPressDevice = async (deviceId: string): Promise<ActionResult> => {
    const result = await deviceControlStore.disconnectPressDevice(deviceId)
    if (!deviceControlStore.pressDeviceConnected) {
      setStep(CalibrationStep.DEVICE_CONNECT)
    }
    return result
  }

  const setSelectedChannels = calibrationConfig.setSelectedChannels

  const generatePressurePoints = async (opts?: { controlMode?: string; pressureMode?: string; silent?: boolean }): Promise<ActionResult> => {
    const activeControlMode: ControlMode = opts?.controlMode === ControlMode.Manual ? ControlMode.Manual : controlMode.value
    return pressurePointStore.generatePressurePoints({
      ...opts,
      controlMode: activeControlMode,
      channels: selectedChannels.value,
      silent: opts?.silent,
      params: {
        points: calibrationParams.value.points,
        averageCount: calibrationParams.value.averageCount,
        minValue: calibrationParams.value.minValue,
        maxValue: calibrationParams.value.maxValue,
        stableTime: calibrationParams.value.stableTime,
        precision: calibrationParams.value.precision,
        precisionLevel: calibrationParams.value.precisionLevel
      }
    })
  }

  const startCalibration = async (opts?: { controlMode?: string }): Promise<ActionResult> => {
    const activeControlMode: ControlMode = opts?.controlMode === ControlMode.Manual ? ControlMode.Manual : controlMode.value
    controlMode.value = activeControlMode

    if (!canStartCalibration.value) {
      const missing: string[] = []
      if (!device1604Connected.value) missing.push('连接计量设备')
      if (!channelsSelected.value) missing.push('选择通道')
      if (enforceValveCalibrationGate.value && !valveReady.value) missing.push('将阀门切换到校准状态')
      return { ok: false, error: 'MISSING_REQUIREMENTS', detail: `请先${missing.join('并')}` }
    }
    // 自动模式额外校验打压设备
    if (activeControlMode === ControlMode.Auto && !pressDeviceConnected.value) {
      return { ok: false, error: 'MISSING_PRESS_DEVICE', detail: '自动模式需要连接打压设备' }
    }

    // 自动模式校验采集设备和打压设备单位一致
    if (activeControlMode === ControlMode.Auto) {
      try {
        const unitCheck = await fetchUnitConsistency()
        if (!unitCheck.consistent) {
          return { ok: false, error: 'UNIT_MISMATCH', detail: '采集设备与打压设备压力单位不一致，请统一单位后再开始标定' }
        }
      } catch {
        return { ok: false, error: 'UNIT_CHECK_FAILED', detail: '无法检查设备单位一致性，请确认设备连接正常' }
      }
    }

    // 保存用户手动编辑的目标压力值，避免重新生成时被覆盖
    const savedTargets = new Map(
      pressurePointStore.pressurePoints.map(p => [p.index, p.targetPressure])
    )

    // 重新生成压力点以初始化后端内存中的点位
    const pointsResult = await pressurePointStore.generatePressurePoints({
      controlMode: activeControlMode,
      channels: selectedChannels.value,
      params: {
        points: calibrationParams.value.points,
        averageCount: calibrationParams.value.averageCount,
        minValue: calibrationParams.value.minValue,
        maxValue: calibrationParams.value.maxValue,
        stableTime: calibrationParams.value.stableTime,
        precision: calibrationParams.value.precision,
        precisionLevel: calibrationParams.value.precisionLevel
      }
    })
    if (!pointsResult.ok || pressurePointStore.pressurePoints.length === 0) {
      return { ok: false, error: 'POINTS_NOT_READY', detail: '开始标定失败：压力点未就绪' }
    }

    // 恢复用户手动编辑的目标压力值
    const restoreResults = await Promise.all(
      pressurePointStore.pressurePoints.map(async (point) => {
        const edited = savedTargets.get(point.index)
        if (edited === undefined || Math.abs(edited - point.targetPressure) <= 0.001) return null
        return pressurePointStore.updateTargetPressure(point.id, edited)
      })
    )
    const failures = restoreResults.filter(r => r !== null && !r.ok)
    if (failures.length > 0) {
      console.warn(`${failures.length} 个压力点的编辑值恢复失败`, failures)
    }

    return pushCalibrationConfigAndStart(activeControlMode)
  }

  const pushCalibrationConfigAndStart = async (controlMode: ControlMode): Promise<ActionResult> => {
    try {
      const measureDevs = deviceStore.measureDevices.filter(d => d.status === 'connected')
      const pressureDev = deviceStore.pressureDevices.find(d => d.status === 'connected')
      if (measureDevs.length > 0) {
        if (pressureDev) {
          if (measureDevs.length === 1) {
            await deviceControlStore.setDevices(measureDevs[0].id, pressureDev.id)
          } else {
            await deviceControlStore.setMeasureDevices(measureDevs.map(d => d.id), pressureDev.id)
          }
        } else if (measureDevs.length === 1) {
          await deviceControlStore.setDevices(measureDevs[0].id, '')
        } else {
          await deviceControlStore.setMeasureDevices(measureDevs.map(d => d.id), '')
        }
      }
      await setCalibrationConfig({
        channels: selectedChannels.value,
        pressurePoints: calibrationParams.value.points,
        averageCount: calibrationParams.value.averageCount,
        minPressure: calibrationParams.value.minValue,
        maxPressure: calibrationParams.value.maxValue,
        stableWaitMs: calibrationParams.value.stableTime * 1000,
        controlMode,
        precision: calibrationParams.value.precision,
        precisionLevel: Number(calibrationParams.value.precisionLevel) || 0.05,
        pressureMode: calibrationParams.value.pressureMode
      })
      const data = await triggerSessionAction('start')
      syncSessionState(data.state)
      isCollecting.value = true
      setStep(CalibrationStep.DATA_COLLECTION)
      return { ok: true }
    } catch (error) {
      console.error('开始标定失败:', error)
      const detail = error instanceof Error ? error.message : String(error)
      return { ok: false, error: 'START_FAILED', detail: `开始标定失败: ${detail}` }
    }
  }

  const withSessionAction = async (action: 'start' | 'pause' | 'resume' | 'stop', onSuccess?: () => void): Promise<ActionResult> => {
    try {
      const data = await triggerSessionAction(action)
      syncSessionState(data.state)
      onSuccess?.()
      return { ok: true }
    } catch (error) {
      console.error(`${action} 失败:`, error)
      return { ok: false, error: `${action.toUpperCase()}_FAILED`, detail: String(error) }
    }
  }

  const pauseCalibration = () => withSessionAction('pause', () => { isCollecting.value = false })

  const resumeCalibration = () => withSessionAction('resume', () => { isCollecting.value = true })

  const stopCalibration = () => withSessionAction('stop', () => { isCollecting.value = false; setStep(CalibrationStep.START_CALIBRATION) })

  const resolveAlarm = async (decision: 'continue' | 'skip' | 'recollect' | 'stop'): Promise<ActionResult> => {
    try {
      await apiResolveAlarm(decision)
      return { ok: true }
    } catch (error) {
      console.error('报警处理失败:', error)
      return { ok: false, error: 'ALARM_RESOLVE_FAILED', detail: '报警处理失败' }
    }
  }

  // 永久跳过指定计量设备（从本批次剩余压力点移除）。
  const skipDevice = async (deviceId: string, reason: string): Promise<ActionResult> => {
    try {
      await skipCalibrationDevice(deviceId, reason)
      return { ok: true }
    } catch (error) {
      console.error('跳过设备失败:', error)
      return { ok: false, error: 'SKIP_DEVICE_FAILED', detail: '跳过设备失败' }
    }
  }

  const canOperateCurrentPoint = () => {
    return sessionState.value !== 'idle' && sessionState.value !== 'stopped' && sessionState.value !== 'completed'
  }

  const pressurize = async (pointId: string): Promise<ActionResult> => {
    if (!canOperateCurrentPoint()) {
      return { ok: false, error: 'NOT_RUNNING', detail: '请先开始标定流程' }
    }

    if (controlMode.value === ControlMode.Manual && !pressDeviceConnected.value) {
      return { ok: false, error: 'NOT_CONNECTED', detail: '手动模式且未连接打压设备，请先确认压力到位' }
    }
    return pressurePointStore.pressurize(pointId)
  }

  const fitData = async (): Promise<ActionResult> => {
    if (!hasCollectedData.value) {
      return { ok: false, error: 'NO_DATA', detail: '没有可拟合的数据' }
    }
    try {
      setStep(CalibrationStep.DATA_FITTING)
      const result = await apiFitData()
      pressurePointStore.fittingResult = result
      setStep(CalibrationStep.COMPLETED)
      sessionState.value = 'completed'
      return { ok: true }
    } catch (error) {
      console.error('拟合失败:', error)
      return { ok: false, error: 'FIT_FAILED', detail: '数据拟合失败' }
    }
  }

  const endCalibration = async (): Promise<ActionResult> => {
    if (isSessionRunning(sessionState.value)) {
      try { await triggerSessionAction('stop') }
      catch (error) { console.error('停止后端会话失败:', error) }
    } else if (sessionState.value !== 'idle') {
      try { await triggerSessionAction('stop') }
      catch (error) { console.error('结束后端校准失败:', error) }
    }
    isCollecting.value = false
    currentCollectingPoint.value = 0
    setStep(CalibrationStep.DEVICE_CONNECT)
    sessionState.value = 'idle'
    pressurePointStore.resetCollection()
    return { ok: true }
  }

  const resetCollection = (): ActionResult => {
    pressurePointStore.resetCollection()
    isCollecting.value = false
    currentCollectingPoint.value = 0
    sessionState.value = 'idle'
    setStep(CalibrationStep.CHANNEL_SELECT)
    return { ok: true }
  }

  return {
    // State
    currentStep,
    selectedChannels,
    pressurePoints: computed(() => pressurePointStore.pressurePoints),
    calibrationParams,
    isCollecting,
    currentCollectingPoint,
    sessionState,
    controlMode,
    alarmConfig,
    currentPressure: computed(() => deviceControlStore.currentPressure),
    isStable: computed(() => deviceControlStore.isStable),
    channelData: computed(() => deviceControlStore.channelData),
    valveStatus: computed(() => deviceControlStore.valveStatus),
    measureUnit: computed(() => deviceControlStore.measureUnit),
    deviceInfo: computed(() => deviceControlStore.deviceInfo),
    fittingResult: computed(() => pressurePointStore.fittingResult),
    // Getters
    device1604Connected,
    pressDeviceConnected,
    channelsSelected,
    hasCollectedData,
    allPointsCollected,
    valveReady,
    canStartCalibration,
    isRunning,
    primaryAction,
    secondaryActions,
    // Actions
    setStep,
    syncSessionState,
    fetchCurrentSessionState,
    connectDevice1604,
    disconnectDevice1604,
    connectPressDevice,
    disconnectPressDevice,
    setSelectedChannels,
    generatePressurePoints,
    addPressurePoint: pressurePointStore.addPressurePoint,
    updateTargetPressure: pressurePointStore.updateTargetPressure,
    removePressurePoint: pressurePointStore.removePressurePoint,
    updatePointStatus: pressurePointStore.updatePointStatus,
    startCalibration,
    pauseCalibration,
    resumeCalibration,
    stopCalibration,
    resolveAlarm,
    skipDevice,
    pressurize,
    collectData: pressurePointStore.collectData,
    fitData,
    endCalibration,
    resetCollection,
    refreshPressure: deviceControlStore.refreshPressure,
    refreshStability: deviceControlStore.refreshStability,
    refreshMeasureData: deviceControlStore.refreshMeasureData,
    refreshDeviceInfo: deviceControlStore.refreshDeviceInfo,
    refreshValveStatus: deviceControlStore.refreshValveStatus,
    setValveStatus: deviceControlStore.setValveStatus,
    refreshMeasureUnit: deviceControlStore.refreshMeasureUnit,
    setMeasureUnit: deviceControlStore.setMeasureUnit,
    resetDevice: deviceControlStore.resetDevice,
    setDevices: deviceControlStore.setDevices
  }
})
