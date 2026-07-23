// Device Bridge —— Win7 分支（HTTP + WebSocket）
//
// 与 Wails v3 版差异：
//   - RPC 调用：fetch http://127.0.0.1:18181/api/device/* 替代 Wails 生成绑定
//   - 事件订阅：WebSocket 单例（wsClient.ts）替代 @wailsio/runtime Events.On
//   - 类型定义保留原结构，stores/components 无需改动
//
// 与后端 httpserver/device_handler.go 一一对应，详见该文件路由表。

import { get, post, del } from './httpClient'
import { on, off } from './wsClient'

// ---- 类型定义（与 core/types.go 对应，保留原 bridge 接口签名） ----

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

export interface RecordingFatalEvent {
  deviceId: string
  error: string
}

export interface RecordingBackpressureEvent {
  deviceId: string
  queueLen: number
  queueCap: number
  droppedTotal: number
}

// ---- RPC 调用（HTTP fetch） ----

/** 把 Pinia 响应式对象深拷贝为纯结构，避免 JSON.stringify 携带 Proxy 元数据 */
function toPlain<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T
}

/** 获取全部设备配置 */
export function getProfiles(): Promise<TemperatureProfile[]> {
  return get<TemperatureProfile[]>('/api/device/profiles')
}

/** 新增/更新设备配置 */
export function upsertProfile(profile: TemperatureProfile): Promise<void> {
  return post('/api/device/profile', toPlain(profile))
}

/** 删除设备配置 */
export function deleteProfile(id: string): Promise<void> {
  return del(`/api/device/profile/${encodeURIComponent(id)}`)
}

/** 连接设备 */
export function connect(id: string): Promise<void> {
  return post('/api/device/connect', { id })
}

/** 断开设备 */
export function disconnect(id: string): Promise<void> {
  return post('/api/device/disconnect', { id })
}

/** 启动采集 */
export function startAcquisition(id: string): Promise<void> {
  return post('/api/device/start', { id })
}

/** 停止采集 */
export function stopAcquisition(id: string): Promise<void> {
  return post('/api/device/stop', { id })
}

/**
 * 查询设备状态。
 *
 * 后端在设备不存在时返回 404 + ok:false，httpClient 会 reject Error。
 * 调用方（deviceStore）用 `.catch(() => false)` 兼容此签名，
 * 期望返回 Promise<DeviceState | false>，故此处包一层。
 */
export function getStatus(id: string): Promise<DeviceState | false> {
  return get<DeviceState>(`/api/device/status/${encodeURIComponent(id)}`).catch(() => false)
}

/** 下发设备配置 */
export function applyConfig(id: string, cfg: T1603Config): Promise<void> {
  return post('/api/device/apply-config', { id, config: toPlain(cfg) })
}

/** 扫描设备 */
export function scanDevices(): Promise<ScanResult[]> {
  return post<ScanResult[]>('/api/device/scan')
}

// ---- 事件订阅（WebSocket） ----
//
// 后端事件名与原 Wails 版完全一致，前端 wsClient 收到消息后按 event 字段路由。
// 5 个事件订阅保留原 onXxx/offXxx 命名，stores/App.vue 无需改动。

/** payload 事件订阅句柄 */
let payloadUnsubscribe: (() => void) | null = null
/** log 事件订阅句柄 */
let logUnsubscribe: (() => void) | null = null
/** device-state 事件订阅句柄 */
let deviceStateUnsubscribe: (() => void) | null = null
/** recording-fatal 事件订阅句柄 */
let recordingFatalUnsubscribe: (() => void) | null = null
/** recording-backpressure 事件订阅句柄 */
let recordingBackpressureUnsubscribe: (() => void) | null = null

/**
 * 订阅温度快照事件（daq:payload）。
 * DeviceService.relayStream 按 100ms 频率推送最新一帧。
 */
export function onPayload(handler: (snapshot: TemperatureSnapshot) => void): void {
  offPayload()
  payloadUnsubscribe = on<TemperatureSnapshot>('daq:payload', (data) => {
    handler(data)
  })
}

/** 解除温度快照订阅 */
export function offPayload(): void {
  if (payloadUnsubscribe) {
    payloadUnsubscribe()
    payloadUnsubscribe = null
  }
}

/**
 * 订阅日志事件（daq:log）。
 * LogService.EmitLog 推送，前端 logStore 据此实时显示。
 */
export function onLog(handler: (entry: DeviceLogEvent) => void): void {
  offLog()
  logUnsubscribe = on<DeviceLogEvent>('daq:log', (data) => {
    handler(data)
  })
}

/** 解除日志订阅 */
export function offLog(): void {
  if (logUnsubscribe) {
    logUnsubscribe()
    logUnsubscribe = null
  }
}

/**
 * 订阅设备状态变更事件（daq:device-state）。
 *
 * 注意：后端 daq-t1603 当前未发射此事件，订阅为前置基础设施。
 * 数据形态为 [deviceId, DeviceState] 元组，与原 Wails 版约定一致。
 */
export function onDeviceState(handler: (id: string, state: DeviceState) => void): void {
  offDeviceState()
  deviceStateUnsubscribe = on<[string, DeviceState]>('daq:device-state', (data) => {
    if (!Array.isArray(data) || data.length < 2) return
    handler(data[0], data[1])
  })
}

/** 解除设备状态订阅 */
export function offDeviceState(): void {
  if (deviceStateUnsubscribe) {
    deviceStateUnsubscribe()
    deviceStateUnsubscribe = null
  }
}

/**
 * 订阅录制不可恢复错误事件（daq:recording-fatal）。
 * recorder I/O 错误时触发，每设备每秒最多 1 条。
 */
export function onRecordingFatal(handler: (event: RecordingFatalEvent) => void): void {
  offRecordingFatal()
  recordingFatalUnsubscribe = on<RecordingFatalEvent>('daq:recording-fatal', (data) => {
    handler(data)
  })
}

/** 解除录制 fatal 订阅 */
export function offRecordingFatal(): void {
  if (recordingFatalUnsubscribe) {
    recordingFatalUnsubscribe()
    recordingFatalUnsubscribe = null
  }
}

/**
 * 订阅录制背压丢帧事件（daq:recording-backpressure）。
 * recorder 队列满丢帧时触发，全局 1Hz 限频。
 */
export function onRecordingBackpressure(handler: (event: RecordingBackpressureEvent) => void): void {
  offRecordingBackpressure()
  recordingBackpressureUnsubscribe = on<RecordingBackpressureEvent>('daq:recording-backpressure', (data) => {
    handler(data)
  })
}

/** 解除录制背压订阅 */
export function offRecordingBackpressure(): void {
  if (recordingBackpressureUnsubscribe) {
    recordingBackpressureUnsubscribe()
    recordingBackpressureUnsubscribe = null
  }
}

// 注意：不在此处统一调用 closeWebSocket()，避免影响 recordingBridge/logBridge 的事件订阅。
// 应用卸载时由 App.vue onUnmounted 显式调用 closeWebSocket()（如果需要）。
// 当前 App.vue 仅调用 offXxx 解除订阅，不主动关连接，让 wsClient 在页面卸载时自动释放。
// 显式 closeWebSocket 可在测试环境调用以清理资源。
export { closeWebSocket } from './wsClient'
