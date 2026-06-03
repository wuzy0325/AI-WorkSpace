export type DeviceType = 'SIMULATED' | 'DAQ-P-1604' | 'DAQ-T-1603' | 'DAQ-P-1064Pre' | 'WTN_PXI' | 'DSA3217'

export interface ChannelConfig {
  index: number
  name: string
  enabled: boolean
  unit: string
  precision: number
  rangeMin?: number
  rangeMax?: number
}

export interface DaqT1603HardwareConfig {
  thermocoupleTypes: string
  channelMask: string
  samplingRate: number
  binaryFormat: boolean
  averageCount: number
  triggerMode: number
  triggerEdge: number
  triggerCount: number
  showTimestamp: boolean
  showSequence: boolean
  openCircuitCheck: string
}

/** DSA3217 扫描配置（从 LIST S 读取） */
export interface DSA3217ScanConfig {
  /** 平均值 1~240 */
  avg: number
  /** 周期 73~65535 μs */
  period: number
  /** 数据帧率 Hz（根据 avg/period 自动换算） */
  fps: string
  /** 压力单位 */
  unit: string
}

export interface DeviceProfile {
  id: string
  name: string
  type: DeviceType
  transport?: 'tcp' | 'serial'
  address?: string
  port?: number
  serialPort?: string
  baudRate?: number
  autoConnect?: boolean
  macAddress?: string
  samplingRate: number
  channels: ChannelConfig[]
  daqT1603Config?: DaqT1603HardwareConfig
}

export interface DeviceStatus {
  id: string
  name: string
  type: string
  connection: string
  acquiring: boolean
  lastError?: string
}

export interface ScanResult {
  id: string
  name: string
  type: string
  available: boolean
  address?: string
  port?: number
  macAddress?: string
  serialNumber?: string
  firmwareVersion?: string
}

export interface DataPayload {
  deviceId: string
  timestamp: number
  channels: number[]
  channelIndices: number[]
}

export type LogLevel = 'debug' | 'info' | 'warn' | 'error'

export interface LogEntry {
  id: string
  timestamp: string
  level: LogLevel
  source: string
  message: string
  details?: string
}
