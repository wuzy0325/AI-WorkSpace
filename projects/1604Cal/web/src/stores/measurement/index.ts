import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import type { ActionResult } from '@/types/api'
import {
  bindDevices as apiBindDevices,
  bindMeasureDevice as apiBindMeasureDevice,
  unbindMeasureDevices as apiUnbindMeasureDevices,
  readPressure as apiReadPressure,
  readStability as apiReadStability,
  readValveStatus as apiReadValveStatus,
  readValveStatusAll as apiReadValveStatusAll,
  setValveStatus as apiSetValveStatus,
  readMeasureUnit as apiReadMeasureUnit,
  readMeasureUnitAll as apiReadMeasureUnitAll,
  setMeasureUnit as apiSetMeasureUnit,
  setMeasureUnitAll as apiSetMeasureUnitAll,
  readSessionUnitConsistency as apiReadSessionUnitConsistency,
  readDeviceInfo as apiReadDeviceInfo,
  resetDevice as apiResetDevice,
  calibrateZero as apiCalibrateZero
} from '@/api/session'
import {
  fetchMeasurementState,
  startMeasurement,
  pauseMeasurement,
  stopMeasurement,
  fetchMeasurementData,
  generateMeasurementPoints,
  fetchMeasurementPoints,
  getMeasurementAlarmConfig,
  saveMeasurementAlarmConfig,
  checkMeasurementAlarmPending,
  resolveMeasurementAlarm,
  skipMeasurementDevice,
  getMeasurementParamsConfig,
  saveMeasurementParamsConfig,
  autoCollectMeasurement,
  manualPressurizeMeasurement,
  manualCollectMeasurement,
  manualStartMeasurement,
  type MeasurementPoint,
  type MeasurementAlarmConfig,
  type MeasurementParamsPayload,
  type AlarmDecision
} from '@/api/measurement'
import {
  readMeasureDataAllDevices as apiReadMeasureDataAllDevices
} from '@/api/session'
import type { MeasurementState, CollectedRow, StabilityUpdate, AlarmData, PrimaryAction, SecondaryAction } from './types'
import { ControlMode, PressureMode } from '@/types/calibration'
import { useMeasurementDeviceStore } from '@/stores/measurement/deviceStore'
import { useGatesStore } from '@/stores/app/gates'

export type { MeasurementState, CollectedRow, StabilityUpdate }

/**
 * 浏览器只保留最近 200 条实时遥测。完整点位结果和报告数据由后端保存，
 * 无需把后端 2000 条恢复窗口全部放入 Vue 响应式系统。
 */
export const MEASUREMENT_MAX_ROWS = 200

export const useMeasurementStore = defineStore('measurement', () => {
  // ── 状态 ──
  const state = ref<MeasurementState>('idle')
  const rows = ref<CollectedRow[]>([])
  const channels = ref<number[]>(Array.from({ length: 16 }, (_, i) => i + 1))
  // 多设备绑定列表（保持勾选顺序）；measureDeviceId 为首个设备的兼容视图
  const measureDeviceIds = ref<string[]>([])
  const measureDeviceId = computed(() => measureDeviceIds.value[0] ?? '')
  const activeDeviceId = ref('')
  const pressureDeviceId = ref('')

  // 设备实时数据
  const currentPressure = ref(0)
  const isStable = ref(false)
  const channelData = ref<number[]>([])
  // 各绑定设备实时通道数据（deviceID -> 通道数据），多设备展示用
  const channelDataByDevice = ref<Record<string, number[]>>({})
  // 已跳过设备（deviceID -> 跳过原因），剩余压力点不再采集这些设备
  const skippedDevices = ref<Record<string, string>>({})
  const valveStatus = ref('')
  const measureUnit = ref('')
  const valveStatusByDevice = ref<Record<string, string>>({})
  const valveErrorByDevice = ref<Record<string, string>>({})
  const measureUnitByDevice = ref<Record<string, string>>({})
  const measureUnitErrorByDevice = ref<Record<string, string>>({})
  const deviceInfo = ref<Record<string, string>>({})
  // 设备压力单位一致性（计量设备与打压设备需同单位才能开始计量）。
  // 初始视为一致，避免首次进入未加载时误拦；连接/改单位/生成点位时会刷新。
  const unitConsistent = ref(true)

  // 稳定性监控状态
  const stabilityState = ref<StabilityUpdate>({
    pointIndex: 0,
    isStable: false,
    isInRange: false,
    currentValue: 0,
    stableDurationMs: 0,
    requiredDurationMs: 0,
    progress: 0
  })

  // 计量参数（UI 直接绑定，与 backend config 映射）
  const measurementParams = ref({
    minPressure: 0,
    maxPressure: 10,
    pointCount: 5,
    precision: 3,
    averageCount: 3,
    stableWaitS: 3,
    precisionLevel: 0.0002,
    pressureMode: PressureMode.Single as PressureMode,
    controlMode: ControlMode.Auto as ControlMode
  })

  // 计量工作流相关
  const config = ref<MeasurementParamsPayload | null>(null)
  const points = ref<MeasurementPoint[]>([])
  const pointsEdited = ref(false)
  const pointsConfigKey = ref('')
  const currentPointIndex = ref(0)
  const alarmConfig = ref<MeasurementAlarmConfig>({
    enabled: true,
    enabledChannels: [1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16],
    confirmOnAlarm: false,
    soundEnabled: true
  })
  const alarmPending = ref(false)
  const alarmData = ref<AlarmData | null>(null)
  const stabilityTimeoutPending = ref(false)

  // ── 计算属性 ──
  const runningStates: MeasurementState[] = ['pressurizing', 'stabilizing', 'collecting']
  const startableStates: MeasurementState[] = ['idle', 'completed', 'error', 'stopped']

  const isCollecting = computed(() => state.value === 'collecting')
  const isRunning = computed(() => runningStates.includes(state.value))
  const isStartable = computed(() => startableStates.includes(state.value))
  const isPaused = computed(() => state.value === 'paused')
  const isIdle = computed(() => state.value === 'idle')
  const totalRows = computed(() => rows.value.length)
  const deviceBound = computed(() => measureDeviceId.value !== '')
  const hasCompletedPoints = computed(() => points.value.some(p => p.status === 'completed'))

  // 阀门=校准模式是计量启动的必要条件，
  // 与标定模块共用同一规则。
  // 开关由 gate store 从后端 /api/v1/config/gates 拉取，避免前端硬编码。
  const valveReady = computed(() => {
    if (measureDeviceIds.value.length <= 1 || Object.keys(valveStatusByDevice.value).length === 0) {
      return valveStatus.value === 'calibration'
    }
    return measureDeviceIds.value.every(id =>
      valveStatusByDevice.value[id] === 'calibration' && !valveErrorByDevice.value[id]
    )
  })
  const gatesStore = useGatesStore()
  const enforceValveCalibrationGate = computed(() => gatesStore.enforceValveCalibrationGate)
  // 开始计量必须满足：阀门=校准（若启用门禁）且设备压力单位一致。
  const canStart = computed(() => unitConsistent.value && (!enforceValveCalibrationGate.value || valveReady.value))

  // 主按钮：随会话状态自动切换文案、图标、色阶
  const primaryAction = computed<PrimaryAction>(() => {
    switch (state.value) {
      case 'idle':
      case 'ready':
      case 'stopped':
        return { key: 'start', label: '开始采集', icon: 'VideoPlay', variant: 'mint' }
      case 'pressurizing':
      case 'stabilizing':
      case 'collecting':
        return { key: 'pause', label: '暂停', icon: 'VideoPause', variant: 'slate' }
      case 'paused':
        return { key: 'resume', label: '继续采集', icon: 'VideoPlay', variant: 'mint' }
      case 'completed':
        return { key: 'export', label: '导出报告', icon: 'Download', variant: 'blue' }
      case 'error':
        return { key: 'retry', label: '重试', icon: 'Refresh', variant: 'amber' }
      default:
        return { key: 'start', label: '开始采集', icon: 'VideoPlay', variant: 'mint' }
    }
  })

  // 副按钮：仅在该状态确实可用时才出现
  const secondaryActions = computed<SecondaryAction[]>(() => {
    const out: SecondaryAction[] = []
    switch (state.value) {
      case 'pressurizing':
      case 'stabilizing':
      case 'collecting':
      case 'paused':
        out.push({ key: 'stop', label: '停止', variant: 'red', confirm: '确认终止采集？已采集数据将保留。' })
        break
      case 'completed':
        out.push({ key: 'restart', label: '重新采集', variant: 'slate', confirm: '将清空当前结果，重新开始？' })
        break
      case 'stopped':
        if (hasCompletedPoints.value) {
          out.push({ key: 'export', label: '导出报告', variant: 'blue' })
        }
        out.push({ key: 'reset', label: '清空数据', variant: 'slate', confirm: '将永久删除当前采集结果？' })
        break
      case 'error':
        out.push({ key: 'stop', label: '停止', variant: 'red', confirm: '确认终止当前会话？' })
        out.push({ key: 'view-error', label: '查看错误', variant: 'amber' })
        break
    }
    return out
  })

  // ── 设备绑定 ──

  const normalizeMeasureDeviceIds = (ids: string | string[]): string[] => {
    const values = Array.isArray(ids) ? ids : [ids]
    return [...new Set(values.map(id => id.trim()).filter(Boolean))]
  }

  const repairActiveDevice = () => {
    if (!measureDeviceIds.value.includes(activeDeviceId.value)) {
      activeDeviceId.value = measureDeviceIds.value[0] ?? ''
    }
  }

  const setActiveDevice = (deviceId: string) => {
    if (measureDeviceIds.value.includes(deviceId)) {
      activeDeviceId.value = deviceId
    }
  }

  // 绑定多台计量设备 + 打压设备；兼容旧调用方传入单个设备 ID。
  const bindDevices = async (measureDevIds: string | string[], pressureDevId: string) => {
    const normalizedIds = normalizeMeasureDeviceIds(measureDevIds)
    await apiBindDevices(normalizedIds, pressureDevId, 'measurement')
    measureDeviceIds.value = normalizedIds
    pressureDeviceId.value = pressureDevId
    repairActiveDevice()
    await refreshUnitConsistency()
  }

  // 仅绑定计量设备（保留当前打压设备绑定）；兼容旧调用方传入单个设备 ID。
  const bindMeasureDevice = async (measureDevIds: string | string[]) => {
    const normalizedIds = normalizeMeasureDeviceIds(measureDevIds)
    await apiBindMeasureDevice(normalizedIds, 'measurement')
    measureDeviceIds.value = normalizedIds
    repairActiveDevice()
    await refreshUnitConsistency()
  }

  // resetBindingState 仅清空前端绑定相关状态（Pinia 内存态），不调用后端。
  // 需要释放后端绑定/断开设备时走 clearMeasureDeviceBinding。
  const resetBindingState = () => {
    measureDeviceIds.value = []
    activeDeviceId.value = ''
    pressureDeviceId.value = ''
    channelData.value = []
    channelDataByDevice.value = {}
    skippedDevices.value = {}
    valveStatus.value = ''
    measureUnit.value = ''
    valveStatusByDevice.value = {}
    valveErrorByDevice.value = {}
    measureUnitByDevice.value = {}
    measureUnitErrorByDevice.value = {}
    deviceInfo.value = {}
  }

  const unbindPressureDevice = () => {
    pressureDeviceId.value = ''
  }

  // ── 实时数据读取 ──

  const refreshPressure = async () => {
    try { currentPressure.value = await apiReadPressure() }
    catch { /* 设备未绑定时静默 */ }
  }

  const refreshStability = async () => {
    try { isStable.value = await apiReadStability() }
    catch { /* 静默 */ }
  }

  const refreshMeasureData = async () => {
    try {
      // 多设备：读取所有绑定设备实时数据；channelData 保留首个设备数据（兼容旧字段）
      const devices = await apiReadMeasureDataAllDevices()
      channelDataByDevice.value = devices
      channelData.value = devices[measureDeviceId.value] ?? []
    } catch { /* 静默 */ }
  }

  const refreshValveStatus = async () => {
    try { valveStatus.value = await apiReadValveStatus() }
    catch { /* 静默 */ }
  }

  const refreshValveStatusAll = async () => {
    const results = await apiReadValveStatusAll()
    const values: Record<string, string> = {}
    const errors: Record<string, string> = {}
    for (const [deviceId, result] of Object.entries(results)) {
      if (result.error) errors[deviceId] = result.error
      else values[deviceId] = result.value ?? ''
    }
    valveStatusByDevice.value = values
    valveErrorByDevice.value = errors
    valveStatus.value = values[measureDeviceId.value] ?? ''
  }

  // setValveStatus 切换阀门状态：
  // 成功时本地状态先按请求值更新，调用方应再通过 refreshValveStatus 校核硬件实际状态；
  // 失败时返回结构化错误（含 N09 等设备拒绝信息），便于 UI 弹出可读提示。
  const setValveStatus = async (status: string): Promise<ActionResult> => {
    try {
      await apiSetValveStatus(status)
      valveStatus.value = status
      return { ok: true }
    } catch (error) {
      const detail = error instanceof Error ? error.message : '设置阀门状态失败'
      console.error('设置阀门状态失败:', error)
      return { ok: false, error: 'VALVE_SET_FAILED', detail }
    }
  }

  const refreshMeasureUnit = async () => {
    for (let attempt = 1; attempt <= 3; attempt++) {
      try {
        measureUnit.value = await apiReadMeasureUnit()
        return
      } catch {
        if (attempt < 3) {
          await new Promise(r => setTimeout(r, 500))
        }
      }
    }
  }

  const refreshMeasureUnitAll = async () => {
    const results = await apiReadMeasureUnitAll()
    const values: Record<string, string> = {}
    const errors: Record<string, string> = {}
    for (const [deviceId, result] of Object.entries(results)) {
      if (result.error) errors[deviceId] = result.error
      else values[deviceId] = result.value ?? ''
    }
    measureUnitByDevice.value = values
    measureUnitErrorByDevice.value = errors
    measureUnit.value = values[measureDeviceId.value] ?? ''
  }

  const refreshDeviceSettingsAll = async () => {
    await Promise.all([refreshValveStatusAll(), refreshMeasureUnitAll()])
  }

  const setMeasureUnit = async (unit: string) => {
    await apiSetMeasureUnit(unit)
    measureUnit.value = unit
    await refreshUnitConsistency()
  }

  const setMeasureUnitAll = async (unit: string): Promise<ActionResult> => {
    try {
      await apiSetMeasureUnitAll(unit)
      return { ok: true }
    } catch (error) {
      return {
        ok: false,
        error: 'UNIT_SET_FAILED',
        detail: error instanceof Error ? error.message : '统一设备单位失败'
      }
    }
  }

  // 刷新设备压力单位一致性状态，用于开始计量的门禁。
  const refreshUnitConsistency = async () => {
    try {
      const check = await apiReadSessionUnitConsistency()
      unitConsistent.value = check?.consistent !== false
    } catch {
      // 拉取失败时不改变现有状态，避免因一次网络抖动误拦启动。
    }
  }

  const refreshDeviceInfo = async () => {
    try { deviceInfo.value = await apiReadDeviceInfo() }
    catch { /* 静默 */ }
  }

  const resetDevice = async (deviceId = activeDeviceId.value) => {
    await apiResetDevice(deviceId)
  }

  const calibrateZero = async (deviceId: string, selectedChannels: number[]) => {
    return apiCalibrateZero(selectedChannels, deviceId)
  }

  const clearMeasureDeviceBinding = async () => {
    await apiUnbindMeasureDevices()
    resetBindingState()
  }

  // ── 采集工作流 ──

  const start = async (selectedChannels: number[]): Promise<ActionResult> => {
    if (!deviceBound.value) {
      return { ok: false, error: 'DEVICE_NOT_BOUND', detail: '请先绑定计量设备' }
    }
    // 阀门=校准模式是启动的必要条件，前端先做一次轻量门禁，
    // 给用户即时反馈；后端会做权威校验，避免绕过。
    if (enforceValveCalibrationGate.value && !valveReady.value) {
      return { ok: false, error: 'VALVE_NOT_READY', detail: '请先将阀门切换到校准模式' }
    }
    try {
      await ensureDevicesBound()
      await syncPointsBeforeStart()
      channels.value = selectedChannels
      const newState = await startMeasurement(selectedChannels)
      state.value = newState as MeasurementState
      rows.value = []
      return { ok: true }
    } catch (error) {
      const detail = error instanceof Error ? error.message : String(error)
      return { ok: false, error: 'START_FAILED', detail }
    }
  }

  const manualStart = async (selectedChannels: number[]): Promise<ActionResult> => {
    if (!deviceBound.value) {
      return { ok: false, error: 'DEVICE_NOT_BOUND', detail: '请先绑定计量设备' }
    }
    if (enforceValveCalibrationGate.value && !valveReady.value) {
      return { ok: false, error: 'VALVE_NOT_READY', detail: '请先将阀门切换到校准模式' }
    }
    try {
      await ensureDevicesBound()
      await syncPointsBeforeStart()
      channels.value = selectedChannels
      currentPointIndex.value = 0
      const newState = await manualStartMeasurement(selectedChannels)
      state.value = newState as MeasurementState
      return { ok: true }
    } catch (error) {
      const detail = error instanceof Error ? error.message : String(error)
      return { ok: false, error: 'MANUAL_START_FAILED', detail }
    }
  }

  const pause = async (): Promise<ActionResult> => {
    try {
      const newState = await pauseMeasurement()
      state.value = newState as MeasurementState
      return { ok: true }
    } catch (error) {
      return { ok: false, error: 'PAUSE_FAILED', detail: '暂停采集失败' }
    }
  }

  const stop = async (): Promise<ActionResult> => {
    try {
      const newState = await stopMeasurement()
      state.value = newState as MeasurementState
      return { ok: true }
    } catch (error) {
      return { ok: false, error: 'STOP_FAILED', detail: '停止采集失败' }
    }
  }

  const refreshData = async () => {
    try {
      const resp = await fetchMeasurementData()
      if (!Array.isArray(resp?.rows)) {
        rows.value = []
        return
      }
      // 后端已限制 s.rows 上限，此处再兜底截断：保留最近 MEASUREMENT_MAX_ROWS 行，
      // 确保任何异常返回都不会撑爆前端响应式 store 与表格 DOM。
      rows.value = resp.rows.length > MEASUREMENT_MAX_ROWS
        ? resp.rows.slice(-MEASUREMENT_MAX_ROWS)
        : resp.rows
    } catch { /* 静默 */ }
  }

  const fetchCurrentState = async () => {
    try {
      const s = await fetchMeasurementState()
      state.value = s as MeasurementState
    } catch { /* 静默 */ }
  }

  const syncState = (newState: MeasurementState) => {
    state.value = newState
  }

  const updateDevicePressure = (deviceId: string, pressure: number) => {
    const deviceStore = useMeasurementDeviceStore()
    deviceStore.updateDevicePressure(deviceId, pressure)
  }

  // ── 计量工作流 ──

  const loadConfig = async () => {
    try {
      const cfg = await getMeasurementParamsConfig()
      config.value = cfg
      // 同步到 UI 绑定源
      if (cfg) {
        measurementParams.value = {
          minPressure: cfg.minPressure,
          maxPressure: cfg.maxPressure,
          pointCount: cfg.pointCount,
          precision: cfg.precision,
          averageCount: cfg.averageCount,
          stableWaitS: Math.round(cfg.stableDurationMs / 1000),
          precisionLevel: cfg.precisionLevel,
          pressureMode: cfg.pressureMode as PressureMode,
          controlMode: cfg.controlMode as ControlMode
        }
      }
    } catch { /* 静默 */ }
  }

  const saveConfig = async (params: MeasurementParamsPayload) => {
    await saveMeasurementParamsConfig(params)
    config.value = params
  }

  const loadPoints = async () => {
    try {
      const loadedPoints = await fetchMeasurementPoints()
      points.value = Array.isArray(loadedPoints) ? loadedPoints : []
      restoreFlowStateFromPoints()
    } catch { /* 静默 */ }
  }

  // restoreFlowStateFromPoints 从后端点位状态恢复刷新即丢的纯前端流程位置：
  // - currentPointIndex 取首个未完成点的前一个序号（手动模式"打压下一点"以它为基准）。
  //   按"第一个非 completed 点"推导而非"最后一个完成点"，跳过/失败/中断的点不会被
  //   越过——刷新后从断点继续，而不是跳到失败点之后。
  // - skippedDevices 从设备维度采集状态重建，避免刷新后重复向已跳过设备发起采集。
  // 已知边界：分批模式（batchStore）的阶段状态仍为内存态、刷新不恢复，
  // 需要后端批次会话查询接口支撑，见 docs/plans 后续迭代。
  const restoreFlowStateFromPoints = () => {
    let restoredIndex = points.value.length
    for (const p of points.value) {
      if (p.status !== 'completed') {
        restoredIndex = Math.max(0, p.index - 1)
        break
      }
    }
    currentPointIndex.value = restoredIndex

    const skipped: Record<string, string> = {}
    for (const p of points.value) {
      for (const [devID, d] of Object.entries(p.collectedByDevice ?? {})) {
        if (d.status === 'skipped') skipped[devID] = d.skipReason ?? ''
      }
    }
    skippedDevices.value = skipped
  }

  const buildMeasurementPayload = (p: typeof measurementParams.value, customPoints?: number[]): MeasurementParamsPayload => ({
    minPressure: p.minPressure,
    maxPressure: p.maxPressure,
    pointCount: p.pointCount,
    precision: p.precision,
    averageCount: p.averageCount,
    stableDurationMs: p.stableWaitS * 1000,
    precisionLevel: p.precisionLevel,
    pressureMode: p.pressureMode,
    controlMode: p.controlMode,
    ...(customPoints !== undefined ? { customPoints } : {})
  })

  const generatePoints = async (): Promise<ActionResult> => {
    try {
      const p = measurementParams.value
      const payload = buildMeasurementPayload(p)
      await saveMeasurementParamsConfig(payload)
      config.value = payload
      points.value = await generateMeasurementPoints()
      pointsEdited.value = false
      pointsConfigKey.value = measurementParamsKey(p)
      return { ok: true }
    } catch (error) {
      const detail = error instanceof Error ? error.message : String(error)
      return { ok: false, error: 'GENERATE_FAILED', detail }
    }
  }

  const syncPointsBeforeStart = async () => {
    const p = measurementParams.value
    const currentConfigKey = measurementParamsKey(p)
    const customPoints = pointsEdited.value && pointsConfigKey.value === currentConfigKey && points.value.length > 0
      ? points.value.map(point => point.targetPressure)
      : undefined
    const payload = buildMeasurementPayload(p, customPoints)
    await saveMeasurementParamsConfig(payload)
    config.value = payload
    points.value = await generateMeasurementPoints()
    pointsEdited.value = false
    pointsConfigKey.value = currentConfigKey
  }

  // 启动前确保所有已勾选计量设备已绑定到会话（多设备整批绑定）。
  // 先与后端实际连接状态对齐：剔除已被外部断开/掉线的设备，
  // 避免残留绑定在重绑时触发已断开设备的自动重连，或读取时报 not connected。
  const ensureDevicesBound = async () => {
    if (measureDeviceIds.value.length === 0) return
    await syncMeasureDevicesWithStatus()
    if (measureDeviceIds.value.length === 0) {
      throw new Error('计量设备均已断开，请重新选择并连接设备')
    }
    if (pressureDeviceId.value) {
      await apiBindDevices(measureDeviceIds.value, pressureDeviceId.value)
      return
    }
    await apiBindMeasureDevice(measureDeviceIds.value)
  }

  // 对齐 measureDeviceIds 与后端实际连接状态：剔除明确已断开的设备。
  // 仅当设备清单拉取成功且对应设备状态明确为 disconnected 时才剔除；
  // 拉取失败或设备不在清单中时保守保留，避免因一次网络抖动误删设备。
  const syncMeasureDevicesWithStatus = async (): Promise<void> => {
    const deviceStore = useMeasurementDeviceStore()
    const loaded = await deviceStore.loadDevices()
    if (!loaded.ok) return
    const statusById = new Map(
      deviceStore.measureDevices.map(d => [d.id, d.status] as const)
    )
    const valid = measureDeviceIds.value.filter(id => statusById.get(id) !== 'disconnected')
    if (valid.length === measureDeviceIds.value.length) return
    measureDeviceIds.value = valid
    repairActiveDevice()
  }

  // 永久跳过指定计量设备（从本批次剩余压力点移除），并记录原因。
  const skipDevice = async (deviceId: string, reason: string) => {
    await skipMeasurementDevice(deviceId, reason)
    skippedDevices.value[deviceId] = reason
  }

  const measurementParamsKey = (p: typeof measurementParams.value): string => JSON.stringify({
    minPressure: p.minPressure,
    maxPressure: p.maxPressure,
    pointCount: p.pointCount,
    precision: p.precision,
    pressureMode: p.pressureMode
  })

  const loadAlarmConfig = async () => {
    try {
      alarmConfig.value = await getMeasurementAlarmConfig()
    } catch { /* 静默 */ }
  }

  const saveAlarmConfig = async (cfg: MeasurementAlarmConfig) => {
    await saveMeasurementAlarmConfig(cfg)
    alarmConfig.value = cfg
  }

  const refreshAlarmPending = async () => {
    try {
      const resp = await checkMeasurementAlarmPending()
      alarmPending.value = resp.pending
      // 恢复报警详情：SSE 事件只在触发瞬间送达一次，
      // 页面刷新后 alarmData 只能从这里重建，否则弹窗/自动放行无从判断。
      alarmData.value = resp.alarm ?? null
    } catch { /* 静默 */ }
  }

  const resolveAlarm = async (decision: AlarmDecision) => {
    await resolveMeasurementAlarm(decision)
    alarmPending.value = false
  }

  // ── 按点采集 ──

  const startPoint = (index: number) => {
    currentPointIndex.value = index
  }

  const completePoint = () => {
    currentPointIndex.value++
  }

  const resetCollection = () => {
    currentPointIndex.value = 0
    rows.value = []
    points.value = []
    pointsEdited.value = false
    pointsConfigKey.value = ''
    state.value = 'idle'
  }

  const updatePointTarget = (pointId: string, targetPressure: number) => {
    const pt = points.value.find(p => p.id === pointId)
    if (pt) {
      pt.targetPressure = targetPressure
      pointsEdited.value = true
    }
  }

  const autoCollect = async (): Promise<ActionResult> => {
    try {
      const newState = await autoCollectMeasurement()
      state.value = newState as MeasurementState
      return { ok: true }
    } catch (error) {
      const detail = error instanceof Error ? error.message : String(error)
      return { ok: false, error: 'AUTO_COLLECT_FAILED', detail }
    }
  }

  const manualPressurize = async (pointIndex: number): Promise<ActionResult> => {
    try {
      const newState = await manualPressurizeMeasurement(pointIndex)
      state.value = newState as MeasurementState
      return { ok: true }
    } catch (error) {
      const detail = error instanceof Error ? error.message : String(error)
      return { ok: false, error: 'MANUAL_PRESSURIZE_FAILED', detail }
    }
  }

  const manualCollect = async (pointIndex: number): Promise<ActionResult> => {
    try {
      const newState = await manualCollectMeasurement(pointIndex)
      state.value = newState as MeasurementState
      return { ok: true }
    } catch (error) {
      const detail = error instanceof Error ? error.message : String(error)
      return { ok: false, error: 'MANUAL_COLLECT_FAILED', detail }
    }
  }

  return {
    // 状态
    state,
    rows,
    channels,
    measureDeviceIds,
    measureDeviceId,
    activeDeviceId,
    pressureDeviceId,
    measurementParams,
    currentPressure,
    isStable,
    channelData,
    channelDataByDevice,
    skippedDevices,
    valveStatus,
    measureUnit,
    valveStatusByDevice,
    valveErrorByDevice,
    measureUnitByDevice,
    measureUnitErrorByDevice,
    deviceInfo,
    unitConsistent,
    stabilityState,
    stabilityTimeoutPending,
    // 计算属性
    isCollecting,
    isRunning,
    isStartable,
    isPaused,
    isIdle,
    totalRows,
    deviceBound,
    hasCompletedPoints,
    valveReady,
    canStart,
    primaryAction,
    secondaryActions,
// 设备绑定
    bindDevices,
    bindMeasureDevice,
    setActiveDevice,
    syncMeasureDevicesWithStatus,
    resetBindingState,
    clearMeasureDeviceBinding,
    unbindPressureDevice,
    skipDevice,
    // 实时数据
    refreshPressure,
    refreshStability,
    refreshMeasureData,
    refreshValveStatus,
    refreshValveStatusAll,
    setValveStatus,
    refreshMeasureUnit,
    refreshMeasureUnitAll,
    refreshDeviceSettingsAll,
    setMeasureUnit,
    setMeasureUnitAll,
    refreshDeviceInfo,
    refreshUnitConsistency,
    resetDevice,
    calibrateZero,
    // 采集工作流
    start,
    manualStart,
    pause,
    stop,
    refreshData,
    // 计量工作流
    config,
    points,
    currentPointIndex,
    alarmConfig,
    alarmPending,
    alarmData,
    loadConfig,
    saveConfig,
    loadPoints,
    generatePoints,
    loadAlarmConfig,
    saveAlarmConfig,
    refreshAlarmPending,
    resolveAlarm,
    // 按点采集
    startPoint,
    completePoint,
    resetCollection,
    updatePointTarget,
    autoCollect,
    manualPressurize,
    manualCollect,
    fetchCurrentState,
    syncState,
    updateDevicePressure
  }
})
