import type { CalibrationConfig, CalibrationType, SphereTankGateConfig } from '@shared/types/calibration'
import { request } from '@api/http-client'

const CALIBRATION_STORAGE_KEY = 'wind-daq.calibration-config'

function getStorageKey(type: CalibrationType): string {
  return `${CALIBRATION_STORAGE_KEY}.${type}`
}

// 兼容旧 API 的类型
export interface CalibrationPointResult {
  pointIndex: number
  targetPressure: number
  timestamp: number
  values: Record<number, number>
}

export interface CalibrationStatus {
  taskId: string
  state: string
  currentPoint: number
  totalPoints: number
  lastError?: string
  results?: CalibrationPointResult[]
}

export interface OldCalibrationConfig {
  taskId: string
  deviceId: string
  type: string
  channels: number[]
  pressurePoints: number[]
  averageSamples: number
}

export const calibrationApi = {
  getConfig: async (type: CalibrationType): Promise<{ success: boolean; data?: CalibrationConfig; error?: string }> => {
    try {
      const raw = window.localStorage.getItem(getStorageKey(type))
      if (raw) {
        return { success: true, data: JSON.parse(raw) }
      }
      return { success: false, error: 'No saved config' }
    } catch (e) {
      return { success: false, error: String(e) }
    }
  },

  saveConfig: async (type: CalibrationType, config: CalibrationConfig): Promise<{ success: boolean; data?: CalibrationConfig; error?: string }> => {
    try {
      window.localStorage.setItem(getStorageKey(type), JSON.stringify(config))
      return { success: true, data: config }
    } catch (e) {
      return { success: false, error: String(e) }
    }
  },

  startCalibration: async (_config: CalibrationConfig): Promise<{ success: boolean; taskId?: string; error?: string }> => {
    try {
      const result = await request<{ success: boolean; taskId?: string; error?: string }>('/api/calibration/start', {
        method: 'POST',
        body: JSON.stringify(_config),
      })
      return result
    } catch (e) {
      return { success: false, error: String(e) }
    }
  },

  pauseCalibration: async (_taskId: string): Promise<{ success: boolean; error?: string }> => {
    return { success: true }
  },

  resumeCalibration: async (_taskId: string): Promise<{ success: boolean; error?: string }> => {
    return { success: true }
  },

  stopCalibration: async (_taskId: string): Promise<{ success: boolean; error?: string }> => {
    return { success: true }
  },

  saveData: async (_taskId: string): Promise<{ success: boolean; filepath?: string; error?: string }> => {
    return { success: true, filepath: '' }
  },

  exportReport: async (_taskId: string): Promise<{ success: boolean; filepath?: string; error?: string }> => {
    return { success: true, filepath: '' }
  },

  // 旧 API 兼容性，用于测试
  start: async (_cfg: OldCalibrationConfig): Promise<{ success: boolean }> => {
    return { success: true }
  },

  status: async (): Promise<CalibrationStatus> => {
    return {
      taskId: 'test',
      state: 'idle',
      currentPoint: 0,
      totalPoints: 0
    }
  },

  collect: async (): Promise<{ success: boolean }> => {
    return { success: true }
  },

  pause: async (): Promise<{ success: boolean }> => {
    return { success: true }
  },

  resume: async (): Promise<{ success: boolean }> => {
    return { success: true }
  },

  stop: async (): Promise<{ success: boolean }> => {
    return { success: true }
  },

  getResult: async (_taskId: string): Promise<CalibrationStatus> => {
    return {
      taskId: _taskId,
      state: 'idle',
      currentPoint: 0,
      totalPoints: 0
    }
  },

  updateSphereTankGate: async (_gate: SphereTankGateConfig): Promise<{ success: boolean; error?: string }> => {
    return { success: true }
  },
}
