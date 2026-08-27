import { ref, watch } from 'vue'
import { setCalibrationChannels } from "@/api/calibration"
import type { CalibrationParams } from '@/stores/calibration/types'
import { PressureMode } from '@/types/calibration'

export type { CalibrationParams }

const PARAMS_KEY = 'cal1604_calibration_params'

const defaultParams: CalibrationParams = {
  minValue: 0,
  maxValue: 100,
  points: 2,
  precision: 2,
  averageCount: 5,
  stableTime: 3,
  precisionLevel: '0.05',
  pressureMode: PressureMode.Single
}

function loadSavedParams(): CalibrationParams {
  try {
    const raw = localStorage.getItem(PARAMS_KEY)
    if (raw) return { ...defaultParams, ...JSON.parse(raw) }
  } catch { /* ignore parse errors */ }
  return { ...defaultParams }
}

function saveParams(params: CalibrationParams) {
  try {
    localStorage.setItem(PARAMS_KEY, JSON.stringify(params))
  } catch { /* ignore quota errors */ }
}

export function useCalibrationConfig() {
  const selectedChannels = ref<number[]>(Array.from({ length: 16 }, (_, i) => i + 1))
  const calibrationParams = ref<CalibrationParams>(loadSavedParams())

  let saveTimer: ReturnType<typeof setTimeout> | null = null
  watch(calibrationParams, (val) => {
    if (saveTimer) clearTimeout(saveTimer)
    saveTimer = setTimeout(() => saveParams(val), 300)
  }, { deep: true })

  // 设置采集通道
  const setSelectedChannels = async (channels: number[]) => {
    selectedChannels.value = channels
    try {
      await setCalibrationChannels(channels)
    } catch (error) {
      console.error('设置通道失败:', error)
    }
  }

  // 重置通道选择
  const resetChannels = () => {
    selectedChannels.value = []
  }

  return {
    selectedChannels,
    calibrationParams,
    setSelectedChannels,
    resetChannels
  }
}
