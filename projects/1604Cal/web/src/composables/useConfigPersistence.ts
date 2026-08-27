import { onMounted, onUnmounted } from 'vue'
import {
  getCalibrationParamsConfig,
  saveCalibrationParamsConfig,
  getAlarmConfig,
  saveAlarmConfig,
  type CalibrationParamsPayload,
  type AlarmConfigPayload
} from '@/api/calibration'
import { useCalibrationStore } from '@/stores/calibration'
import { useCalibrationConfig } from './useCalibrationConfig'
import { ControlMode, PressureMode } from '@/types/calibration'

/** 配置持久化：页面挂载时从后端加载配置，变更时 250ms 防抖保存到后端。 */
export function useConfigPersistence() {
  const store = useCalibrationStore()
  const { calibrationParams } = useCalibrationConfig()

  let calibrationTimer: ReturnType<typeof setTimeout> | null = null
  let alarmTimer: ReturnType<typeof setTimeout> | null = null

  // 将 CalibrationParams 转换为后端 payload
  function toPayload(p: typeof calibrationParams.value): CalibrationParamsPayload {
    return {
      minPressure: p.minValue,
      maxPressure: p.maxValue,
      pointCount: p.points,
      precision: p.precision,
      averageCount: p.averageCount,
      stableDurationMs: Math.round(p.stableTime * 1000),
      precisionLevel: parseFloat(p.precisionLevel) || 0.05,
      pressureMode: p.pressureMode,
      controlMode: ControlMode.Auto
    }
  }

  // 从后端 payload 转换为 CalibrationParams
  function fromPayload(payload: CalibrationParamsPayload): Partial<typeof calibrationParams.value> {
    return {
      minValue: payload.minPressure,
      maxValue: payload.maxPressure,
      points: payload.pointCount,
      precision: payload.precision,
      averageCount: payload.averageCount,
      stableTime: payload.stableDurationMs / 1000,
      precisionLevel: String(payload.precisionLevel),
      pressureMode: payload.pressureMode === PressureMode.RoundTrip ? PressureMode.RoundTrip : PressureMode.Single
    }
  }

  async function loadConfig() {
    try {
      const params = await getCalibrationParamsConfig()
      const mapped = fromPayload(params)
      Object.assign(calibrationParams.value, mapped)
    } catch (e) {
      console.warn('加载校准参数失败，使用本地配置:', e)
    }
  }

  async function saveConfig() {
    try {
      const payload = toPayload(calibrationParams.value)
      await saveCalibrationParamsConfig(payload)
    } catch (e) {
      console.warn('保存校准参数失败:', e)
    }
  }

  async function loadAlarmConfig() {
    try {
      const config = await getAlarmConfig()
      store.alarmConfig = config
    } catch (e) {
      console.warn('加载报警配置失败:', e)
    }
  }

  async function saveAlarm(config: AlarmConfigPayload) {
    try {
      await saveAlarmConfig(config)
    } catch (e) {
      console.warn('保存报警配置失败:', e)
    }
  }

  // 250ms 防抖保存校准参数
  function debouncedSaveCalibration() {
    if (calibrationTimer) clearTimeout(calibrationTimer)
    calibrationTimer = setTimeout(saveConfig, 250)
  }

  // 250ms 防抖保存报警配置
  function debouncedSaveAlarm(config: AlarmConfigPayload) {
    if (alarmTimer) clearTimeout(alarmTimer)
    alarmTimer = setTimeout(() => saveAlarm(config), 250)
  }

  onMounted(async () => {
    await Promise.all([loadConfig(), loadAlarmConfig()])
  })

  onUnmounted(() => {
    if (calibrationTimer) clearTimeout(calibrationTimer)
    if (alarmTimer) clearTimeout(alarmTimer)
  })

  return {
    loadConfig,
    debouncedSaveCalibration,
    loadAlarmConfig,
    debouncedSaveAlarm
  }
}
