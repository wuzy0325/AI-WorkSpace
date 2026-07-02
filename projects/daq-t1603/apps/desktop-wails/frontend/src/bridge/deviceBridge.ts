// Device Bridge —— Wails v3 版
//
// 与 v2 时期差异：
//   - 调用入口由 `wailsjs/go/backend/App` 改为 `wails3 generate bindings`
//     生成的 `frontend/bindings/daq-t1603/backend` 下的 DeviceService 模块；
//   - 事件总线从 `wailsjs/runtime` 的 EventsOn/EventsOff 改为
//     `@wailsio/runtime` 的 Events.On/Events.Off。
//
// 该文件刻意保留了 v2 时期的类型定义（ChannelConfig / T1603Config / DeviceState 等），
// 避免业务层（stores/views）大面积改动；后续若依赖生成的 models 类型，
// 可逐步替换为 `import type { core } from '../../bindings/daq-t1603/models'`。

import { Events } from '@wailsio/runtime'
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
} from '../../bindings/daq-t1603/backend/deviceservice'

export interface ChannelConfig {
  index: number
  name: string
  enabled: boolean
  unit: string
  color: string
  precision: number
  rangeMin?: number
  rangeMax?: number
  thermocoupleType: string
}

export interface T1603Config {
  thermocoupleTypes: string
  channelMask: string
  samplingRate: number
  averageCount: number
  showTimestamp: boolean
  showSequence: boolean
  autoConnect: boolean
}

export interface TemperatureProfile {
  id: string
  name: string
  address: string
  port: number
  samplingRate: number
  channels: ChannelConfig[]
  t1603Config: T1603Config
  createdAt: number
}

export interface DeviceState {
  profile: TemperatureProfile
  status: number
  statusText: string
  error: string
  connectedAt: number
  acquiringAt: number
  samplingRate: number
}

export interface ScanResult {
  id: string
  name: string
  address: string
  port: number
  macAddress?: string
  serialNumber?: string
  firmwareVersion?: string
}

export interface TemperatureSnapshot {
  deviceId: string
  timestamp: number
  hardwareTimestamp: number
  values: number[]
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

/** 把 Pinia 响应式对象深拷贝为纯结构，避免传给 Wails 时携带 Proxy 元数据 */
function toPlain<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T
}

export function getProfiles(): Promise<TemperatureProfile[]> {
  return GetProfiles() as any
}

export function upsertProfile(profile: TemperatureProfile): Promise<void> {
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

/**
 * v3 后端签名为 `GetStatus(id) (DeviceState, error)`，
 * 设备不存在时会 reject（前端用 `.catch(() => false)` 兼容）。
 */
export function getStatus(id: string): Promise<DeviceState | boolean> {
  return GetStatus(id) as any
}

export function applyConfig(id: string, cfg: T1603Config): Promise<void> {
  return ApplyConfig(id, toPlain(cfg) as any) as Promise<void>
}

export function scanDevices(): Promise<ScanResult[]> {
  return ScanDevices() as any
}

// ---- 事件订阅 ----------------------------------------------------------------
// Wails v3 的 Events.On 回调入参形态为 `{ data: T, ... }`，
// 这里统一在桥层把 e.data 解出来，保持业务侧（stores）对 v2 写法的兼容。

/** payload 事件订阅句柄，用于解除监听 */
let payloadUnsubscribe: (() => void) | null = null
let logUnsubscribe: (() => void) | null = null
let deviceStateUnsubscribe: (() => void) | null = null
let recordingFatalUnsubscribe: (() => void) | null = null
let recordingBackpressureUnsubscribe: (() => void) | null = null

/** 录制不可恢复错误事件载荷 */
export interface RecordingFatalEvent {
  deviceId: string
  error: string
}

/** 录制背压丢帧事件载荷 */
export interface RecordingBackpressureEvent {
  deviceId: string
  queueLen: number
  queueCap: number
  droppedTotal: number
}

export function onPayload(handler: (snapshot: TemperatureSnapshot) => void): void {
  offPayload()
  payloadUnsubscribe = Events.On('daq:payload', (event: { data: TemperatureSnapshot }) => {
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

/**
 * 订阅设备状态变更事件（daq:device-state）。
 * 后端在设备 connect/disconnect/acquiring/error 等状态变化时推送，
 * 前端通过此事件实时更新 statusMap 与错误信息，避免依赖轮询。
 */
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

/** 订阅录制不可恢复错误事件（recorder I/O 错误时触发） */
export function onRecordingFatal(handler: (event: RecordingFatalEvent) => void): void {
  offRecordingFatal()
  recordingFatalUnsubscribe = Events.On('daq:recording-fatal', (event: { data: RecordingFatalEvent }) => {
    handler(event.data)
  })
}

export function offRecordingFatal(): void {
  if (recordingFatalUnsubscribe) {
    recordingFatalUnsubscribe()
    recordingFatalUnsubscribe = null
  }
}

/** 订阅录制背压丢帧事件（recorder 队列满丢帧时触发） */
export function onRecordingBackpressure(handler: (event: RecordingBackpressureEvent) => void): void {
  offRecordingBackpressure()
  recordingBackpressureUnsubscribe = Events.On('daq:recording-backpressure', (event: { data: RecordingBackpressureEvent }) => {
    handler(event.data)
  })
}

export function offRecordingBackpressure(): void {
  if (recordingBackpressureUnsubscribe) {
    recordingBackpressureUnsubscribe()
    recordingBackpressureUnsubscribe = null
  }
}
