export enum CalibrationStep {
  DEVICE_CONNECT = 0,
  CHANNEL_SELECT = 1,
  START_CALIBRATION = 2,
  DATA_COLLECTION = 3,
  DATA_FITTING = 4,
  COMPLETED = 5
}

export interface PressurePoint {
  id: string
  index: number
  targetPressure: number
  status: 'pending' | 'pressurizing' | 'stabilizing' | 'collecting' | 'completed' | 'error' | 'skipped'
  direction?: 'forward' | 'backward'
  collectedData?: number[]
  collectedByDevice?: Record<string, DevicePointData>
  actualPressure?: number
}

/** 单台计量设备在某个压力点的采集结果与状态 */
export interface DevicePointData {
  deviceId: string
  collected?: number[]
  status: 'completed' | 'error' | 'skipped'
  collectTime?: string
  skipReason?: string
  error?: string
}

import type { PressureMode } from '@/types/calibration'

export interface CalibrationParams {
  minValue: number
  maxValue: number
  points: number
  precision: number
  averageCount: number
  stableTime: number
  precisionLevel: string
  pressureMode: PressureMode
}

/** 主按钮动作定义：随会话状态自动切换文案/图标/色阶 */
export interface PrimaryAction {
  key: string
  label: string
  icon: string
  variant: 'mint' | 'slate' | 'blue' | 'amber'
}

/** 副按钮动作定义：仅在该状态确实可用时才出现 */
export interface SecondaryAction {
  key: string
  label: string
  variant: 'slate' | 'red' | 'blue' | 'amber'
  confirm?: string
}
