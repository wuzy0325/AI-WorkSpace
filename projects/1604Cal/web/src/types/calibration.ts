// 会话状态 - 与后端 domain.SessionState 完全对齐
export type SessionState =
  | 'idle'
  | 'ready'
  | 'pressurizing'
  | 'stabilizing'
  | 'collecting'
  | 'point_done'
  | 'fitting'
  | 'completed'
  | 'paused'
  | 'stopped'
  | 'await_manual_collect'
  | 'await_alarm_resolution'
  | 'recovering'
  | 'error'

export interface SessionStateDTO {
  state: SessionState
}

export interface ReportTemplateDTO {
  filename: string
}

// --- 控制模式与压力模式枚举 ---
export const ControlMode = { Auto: 'auto', Manual: 'manual' } as const
export type ControlMode = (typeof ControlMode)[keyof typeof ControlMode]

export const PressureMode = { Single: 'single', RoundTrip: 'roundTrip' } as const
export type PressureMode = (typeof PressureMode)[keyof typeof PressureMode]

// 校准相关 DTO
export interface CalibrationConfigDTO {
  channels: number[]
  pressurePoints: number
  averageCount: number
  minPressure: number
  maxPressure: number
  stableWaitMs: number
  controlMode?: ControlMode
  pressureMode?: PressureMode
  precision?: number
  precisionLevel?: number
}

export interface PressurePointDTO {
  index: number
  targetPressure: number
  status: string
  direction?: 'forward' | 'backward'
  collectedData?: number[]
  collectedByDevice?: Record<string, DevicePointDataDTO>
  actualPressure?: number
}

/** 单台计量设备在某个压力点的采集结果与状态 */
export interface DevicePointDataDTO {
  deviceId: string
  collected?: number[]
  status: 'completed' | 'error' | 'skipped'
  collectTime?: string
  skipReason?: string
  error?: string
}

export interface FittingResultDTO {
  slope: number
  intercept: number
  r2: number
  points: number
}
