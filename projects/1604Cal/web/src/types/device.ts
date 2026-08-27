export interface ChannelConfigDTO {
  index: number      // 通道号（1-based）
  name: string
  enabled: boolean
  unit: string
  rangeMin: number   // 工程量下限（对应 4mA）
  rangeMax: number   // 工程量上限（对应 20mA）
  precision: number
}

export interface DeviceDTO {
  id: string
  name: string
  type: 'measure' | 'pressure'
  model: string
  host: string
  port: number
  unit?: string
  status: 'disconnected' | 'connecting' | 'connected' | 'error'
  localAddr?: string
  lastErrorReason?: string
  lastErrorAt?: string
  // 每通道采集配置（P1603 等需要按通道量程换算工程量的设备使用）
  channels?: ChannelConfigDTO[]
}

export interface DeviceStatusChangedEventData {
  id: string
  type?: DeviceDTO['type']
  status?: DeviceDTO['status']
  errorReason?: string
  lastErrorAt?: string
}

export interface UnitConsistencyDTO {
  consistent: boolean
  conflicts: string[]
}

export interface DeviceConnectConfigDTO {
  connectAttemptTimeoutMs: number
  connectMaxAttempts: number
  connectInitialBackoffMs: number
  connectMaxBackoffMs: number
  disconnectAttemptTimeoutMs: number
  disconnectMaxAttempts: number
  disconnectInitialBackoffMs: number
  disconnectMaxBackoffMs: number
}

export interface SetDevicesRequest {
  measureDeviceId?: string
  measureDeviceIds?: string[]
  pressureDeviceId: string
}
