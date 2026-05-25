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
  thermocoupleType: string
  coldJunction: string
  filterHz: number
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
