/** 计量模块采集状态 */
export type MeasurementState =
  | 'idle'
  | 'ready'
  | 'pressurizing'
  | 'stabilizing'
  | 'collecting'
  | 'completed'
  | 'error'
  | 'paused'
  | 'stopped'

/** 单次采集的数据行 */
export interface CollectedRow {
  timestamp: string
  channels: Record<string, number>
  /** 所属计量设备（多设备场景区分设备；单设备可能为空） */
  deviceId?: string
}

/** 稳定性监控更新载荷 */
export interface StabilityUpdate {
  pointIndex: number
  isStable: boolean
  isInRange: boolean
  currentValue: number
  stableDurationMs: number
  requiredDurationMs: number
  progress: number
}

/** 报警数据载荷 */
export interface AlarmData {
  pointId: string
  /** 触发报警的计量设备（多设备场景定位故障设备；单设备可能为空） */
  deviceId?: string
  targetPressure: number
  actualPressure: number
  threshold: number
  maxDeviation: number
  overLimitChannels: number[]
}

/** 主按钮动作定义 */
export interface PrimaryAction {
  key: string
  label: string
  icon: string
  variant: 'mint' | 'slate' | 'blue' | 'amber'
}

/** 副按钮动作定义 */
export interface SecondaryAction {
  key: string
  label: string
  variant: 'slate' | 'red' | 'blue' | 'amber'
  confirm?: string
}
