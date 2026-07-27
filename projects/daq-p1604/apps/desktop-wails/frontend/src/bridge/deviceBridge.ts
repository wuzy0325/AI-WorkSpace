import {
  GetProfiles,
  UpsertProfile,
  DeleteProfile,
  Connect,
  Disconnect,
  StartAcquisition,
  StopAcquisition,
  ZeroCalibration,
  ApplyConfig,
  GetStatus,
  ScanDevices,
  GetLatestSnapshot,
  GetLatestSnapshots,
  ExitApplication,
} from '../../bindings/daq-p1604/backend/app'
import { Events } from '@wailsio/runtime'

// P1604 设备配置
export interface P1604Config {
  samplingRate: number   // 采样周期（毫秒），由采样频率换算得出
  unit: string           // 压力单位
  autoConnect: boolean   // 启动时自动连接
  precision: number      // 全局默认显示精度（小数位数 0-6），单通道精度未设置时回退到此值
  useDeviceTimestamp?: boolean // 是否使用设备硬件时间戳，undefined=默认开启（兼容老 profile），false=用系统时间
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
  localAddress?: string
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

export function zeroCalibration(id: string): Promise<void> {
  return ZeroCalibration(id) as Promise<void>
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

// ============================================================================
// 数据快照轮询 API
//
// 替代原 `Events.On('daq:payload')` 事件订阅：
// Wails v3 的 Event.Emit 会触发 WebView2 在 GUI 线程上同步执行
// ExecuteScript，频率与设备数线性相关，导致 GUI 阻塞和 Eval errors。
// 改为前端按固定周期（默认 500ms）轮询后端缓存，避免 GUI 线程压力。
// ============================================================================

/**
 * 获取指定设备的最新快照
 * 返回 [PressureSnapshot, boolean]，第二项为是否存在快照
 */
export function getLatestSnapshot(id: string): Promise<[PressureSnapshot, boolean]> {
  return GetLatestSnapshot(id) as any
}

/**
 * 批量获取所有设备的最新快照（推荐前端轮询使用，减少 IPC 次数）
 */
export function getLatestSnapshots(): Promise<Record<string, PressureSnapshot>> {
  return GetLatestSnapshots() as any
}

/**
 * 主动退出应用。
 *
 * 由 MainTopBar 退出按钮的确认框 onPositiveClick 调用，
 * 后端 application.Quit() 会触发 ServiceShutdown 走与原生关闭等价的清理流程。
 */
export function exitApplication(): Promise<void> {
  return ExitApplication() as Promise<void>
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

let logUnsubscribe: (() => void) | null = null
let deviceStateUnsubscribe: (() => void) | null = null
