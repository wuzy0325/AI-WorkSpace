/** 多设备打压设备运行状态 */
export interface MultiPressDeviceState {
  deviceId: string
  currentPressure: number
  targetPressure: number
  unit: string
  stable: boolean
  status: 'idle' | 'pressurizing' | 'exhausting' | 'error'
  errorMessage?: string
}
