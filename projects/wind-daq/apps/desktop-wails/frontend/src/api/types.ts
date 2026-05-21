export interface ChannelConfig {
  index: number
  name: string
  enabled: boolean
  unit: string
  precision: number
  rangeMin?: number
  rangeMax?: number
}

export interface DeviceProfile {
  id: string
  name: string
  type: 'SIMULATED' | 'DAQ_T_1603'
  samplingRate: number
  channels: ChannelConfig[]
}

export interface DeviceStatus {
  id: string
  name: string
  type: string
  connection: string
  acquiring: boolean
  lastError?: string
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
