import {
  GetProfiles,
  UpsertProfile,
  DeleteProfile,
  Connect,
  Disconnect,
  StartAcquisition,
  StopAcquisition,
  ApplyConfig,
  GetStatus,
  ScanDevices,
} from '../../bindings/daq-p1604/backend/app'
import { Events } from '@wailsio/runtime'

// P1604 设备配置
export interface P1604Config {
  samplingRate: number   // 采样周期（毫秒），由采样频率换算得出
  unit: string           // 压力单位
  autoConnect: boolean   // 启动时自动连接
  precision: number      // 全局默认显示精度（小数位数 0-6），单通道精度未设置时回退到此值
}

// 通道配置
export interface ChannelConfig {
  index: number
  name: string
  enabled: boolean
  unit: string
  color: string
  precision: number
  rangeMin?: number
  rangeMax?: number
}

// 压力采集设备配置档案
export interface PressureProfile {
  id: string
  name: string
  address: string
  port: number
  samplingRate: number
  channels: ChannelConfig[]
  p1604Config: P1604Config
  createdAt: number
}

// 设备状态
export interface DeviceState {
  profile: PressureProfile
  status: number
  statusText: string
  error: string
  connectedAt: number
  acquiringAt: number
  samplingRate: number
}

// 扫描结果
export interface ScanResult {
  id: string
  name: string
  address: string
  port: number
  macAddress?: string
  serialNumber?: string
  firmwareVersion?: string
}

// 压力采集数据快照
export interface PressureSnapshot {
  deviceId: string
  timestamp: number
  hardwareTimestamp: number
  values: number[]  // 18 通道
  unit: string
}

export type LogLevel = 'debug' | 'info' | 'warn' | 'error'
export type LogCategory = 'system' | 'hardware-send' | 'hardware-recv' | 'acquisition'

export interface DeviceLogEvent {
  level: LogLevel
  category: LogCategory
  deviceId?: string
  source: string
  message: string
  detail?: string
  timestamp: number
}

function toPlain<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T
}

export function getProfiles(): Promise<PressureProfile[]> {
  return GetProfiles() as any
}

export function upsertProfile(profile: PressureProfile): Promise<void> {
  return UpsertProfile(toPlain(profile) as any) as Promise<void>
}

export function deleteProfile(id: string): Promise<void> {
  return DeleteProfile(id) as Promise<void>
}

export function connect(id: string): Promise<void> {
  return Connect(id) as Promise<void>
}

export function disconnect(id: string): Promise<void> {
  return Disconnect(id) as Promise<void>
}

export function startAcquisition(id: string): Promise<void> {
  return StartAcquisition(id) as Promise<void>
}

export function stopAcquisition(id: string): Promise<void> {
  return StopAcquisition(id) as Promise<void>
}

export function getStatus(id: string): Promise<DeviceState | boolean> {
  return GetStatus(id) as any
}

export function applyConfig(id: string, cfg: P1604Config): Promise<void> {
  return ApplyConfig(id, toPlain(cfg) as any) as Promise<void>
}

export function scanDevices(): Promise<ScanResult[]> {
  return ScanDevices() as any
}

export function onPayload(handler: (snapshot: PressureSnapshot) => void): void {
  offPayload()
  payloadUnsubscribe = Events.On('daq:payload', (event: { data: PressureSnapshot }) => {
    handler(event.data)
  })
}

export function offPayload(): void {
  if (payloadUnsubscribe) {
    payloadUnsubscribe()
    payloadUnsubscribe = null
  }
}

export function onLog(handler: (entry: DeviceLogEvent) => void): void {
  offLog()
  logUnsubscribe = Events.On('daq:log', (event: { data: DeviceLogEvent }) => {
    handler(event.data)
  })
}

export function offLog(): void {
  if (logUnsubscribe) {
    logUnsubscribe()
    logUnsubscribe = null
  }
}

export function onDeviceState(handler: (id: string, state: DeviceState) => void): void {
  offDeviceState()
  deviceStateUnsubscribe = Events.On('daq:device-state', (event: { data: [string, DeviceState] }) => {
    const [id, state] = event.data
    handler(id, state)
  })
}

export function offDeviceState(): void {
  if (deviceStateUnsubscribe) {
    deviceStateUnsubscribe()
    deviceStateUnsubscribe = null
  }
}

let payloadUnsubscribe: (() => void) | null = null
let logUnsubscribe: (() => void) | null = null
let deviceStateUnsubscribe: (() => void) | null = null
