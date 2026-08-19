export type DeviceType = 'SIMULATED' | 'DAQ-P-1604' | 'DAQ-P-1603' | 'DAQ-T-1603' | 'DAQ-T-1602' | 'PACE1000' | 'DAQ-P-1604Pre' | 'WTN_PXI' | 'DSA3217'

/**
 * 通道传感器类型（仅 DAQ-P-1603 使用）。
 * 反序列化时由后端 ChannelConfig.UnmarshalJSON 兜底为 'pressure'，
 * 前端读取时可假定非空；写入时显式传入 'pressure' | 'temperature'。
 */
export type ChannelSensorType = 'pressure' | 'temperature'

export interface ChannelConfig {
  index: number
  name: string
  enabled: boolean
  unit: string
  precision: number
  rangeMin?: number
  rangeMax?: number
  thermocoupleType?: string
  /** 通道传感器类型，仅 DAQ-P-1603 使用 */
  sensorType?: ChannelSensorType
  tareOffset?: number
  calibrationOffset?: number
  calibrationUnit?: string
  calibrationAt?: number
  calibrationEnabled?: boolean
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

/**
 * DAQ-T-1602 专属硬件配置（Modbus TCP 温度扫描阀，2 卡 × 8 通道）。
 * 与后端 shared SDK 的 TypeCodes [16]uint8 对应：
 * 16 个通道的热电偶类型码，取值 0~7（0=J 1=K 2=T 3=E 4=R 5=S 6=B 7=N）。
 */
export interface DaqT1602HardwareConfig {
  typeCodes: number[]
  /** 采集/保存频率（Hz），范围 1~5；设备固件采集周期固定 ~100ms，此处控制软件轮询节奏 */
  sampleRateHz?: number
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
  version?: number
  id: string
  name: string
  type: DeviceType
  transport?: 'tcp' | 'serial'
  address?: string
  localAddress?: string
  port?: number
  serialPort?: string
  baudRate?: number
  autoConnect?: boolean
  macAddress?: string
  samplingRate: number
  channels: ChannelConfig[]
  daqT1603Config?: DaqT1603HardwareConfig
  daqT1602Config?: DaqT1602HardwareConfig
  /** DAQ-P-1604 专属：是否使用设备帧内硬件时间戳（关闭时使用主机接收时间） */
  daqP1604UseDeviceTimestamp?: boolean
}

export interface CalibrationResult {
  channelIndex: number
  offset: number
  unit: string
  at: number
  sampleCount: number
}

export interface CalibrationRecord {
  channelIndex: number
  offset: number
  unit: string
  at: number
  enabled: boolean
}

export interface CalibrationProgress {
  running: boolean
  channelIndex?: number
  elapsedMs: number
  sampleCount: number
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
  model?: string
  subnetMask?: string
  gateway?: string
  ipMode?: string
  tcpConnected?: boolean
  ipAssigned?: boolean
}

export interface DataPayload {
  deviceId: string
  deviceType?: DeviceType
  deviceName?: string
  timestamp: number
  deviceTimestamp?: number
  // null 表示该通道无有效测量值（如 DAQ-T-1602 未接入热电偶的通道，
  // 后端把 NaN 序列化为 null）。展示层应渲染 "--"，波形图留空点。
  channels: (number | null)[]
  channelIndices: number[]
}

export type LogLevel = 'debug' | 'info' | 'warn' | 'error'

export type LogCategory = 'system' | 'hardware-send' | 'hardware-recv' | 'acquisition' | 'business'

export interface LogEntry {
  id: string
  timestamp: string
  level: LogLevel
  category?: LogCategory
  deviceId?: string
  source: string
  message: string
  details?: string
}
