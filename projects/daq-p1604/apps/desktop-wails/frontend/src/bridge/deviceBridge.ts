// Device Bridge —— Win7 分支（HTTP + WebSocket）
//
// 与 Wails v3 版差异：
//   - RPC 调用：fetch http://127.0.0.1:18182/api/device/* 替代 Wails 生成绑定
//   - 事件订阅：WebSocket 单例（wsClient.ts）替代 @wailsio/runtime Events.On
//   - 类型定义保留原结构，stores/components 无需改动
//
// 与 daq-t1603 Win7 版的差异：
//   - 没有 daq:payload 事件（daq-p1604 v0.3.0 已移除），改为前端 500ms 轮询
//     getLatestSnapshots() 拉取所有设备最新快照
//   - 多了 daq:recording-warning 事件（多设备录制场景某台断连警告）
//   - daq:device-state 仍是双参数事件 [id, state]，与 daq-t1603 一致
//
// 与后端 httpserver/device_handler.go 一一对应，详见该文件路由表。

import { get, post, del } from './httpClient'
import { on, off } from './wsClient'

// ---- 类型定义（与 core/types.go 对应，保留原 bridge 接口签名） ----

/** P1604 设备配置 */
export interface P1604Config {
  samplingRate: number         // 采样周期（毫秒），由采样频率换算得出
  unit: string                 // 压力单位
  autoConnect: boolean         // 启动时自动连接
  precision: number            // 全局默认显示精度（小数位数 0-6），单通道精度未设置时回退到此值
  useDeviceTimestamp?: boolean // 是否使用设备硬件时间戳，undefined=默认开启（兼容老 profile），false=用系统时间
}

/** 通道配置 */
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

/** 压力采集设备配置档案 */
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

/** 设备状态 */
export interface DeviceState {
  profile: PressureProfile
  status: number
  statusText: string
  error: string
  connectedAt: number
  acquiringAt: number
  samplingRate: number
}

/** 扫描结果 */
export interface ScanResult {
  id: string
  name: string
  address: string
  port: number
  macAddress?: string
  serialNumber?: string
  firmwareVersion?: string
}

/** 压力采集数据快照 */
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

/**
 * 录制期间设备健康度警告事件载荷（与 backend.RecordingWarningEvent 对应）。
 *
 * 多设备录制场景下某台设备断连时由 App.emitRecordingWarning 推送，
 * 与 RecordingSession 状态变更分离，避免前端混淆"录制真的停了"和"只是有设备掉线"。
 */
export interface RecordingWarningEvent {
  deviceId: string
  message: string
  timestamp: number
}

// ---- RPC 调用（HTTP fetch） ----

/** 把 Pinia 响应式对象深拷贝为纯结构，避免 JSON.stringify 携带 Proxy 元数据 */
function toPlain<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T
}

/** 获取全部设备配置 */
export function getProfiles(): Promise<PressureProfile[]> {
  return get<PressureProfile[]>('/api/device/profiles')
}

/** 新增/更新设备配置 */
export function upsertProfile(profile: PressureProfile): Promise<void> {
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
export function applyConfig(id: string, cfg: P1604Config): Promise<void> {
  return post('/api/device/apply-config', { id, config: toPlain(cfg) })
}

/** 扫描设备 */
export function scanDevices(): Promise<ScanResult[]> {
  return post<ScanResult[]>('/api/device/scan')
}

// ---- 数据快照轮询 API ----
//
// 替代原 `Events.On('daq:payload')` 事件订阅：
// Wails v3 的 Event.Emit 会触发 WebView2 在 GUI 线程上同步执行
// ExecuteScript，频率与设备数线性相关，导致 GUI 阻塞和 Eval errors。
// 改为前端按固定周期（默认 500ms）轮询后端缓存，避免 GUI 线程压力。
//
// Win7 分支延续此设计：HTTP 端点替代 Wails 绑定，调用形态对 stores 透明。

/**
 * 获取指定设备的最新快照。
 *
 * @returns 元组 [snapshot, ok]：ok=true 时 snapshot 有效；ok=false 时无快照（HTTP 404）
 *
 * 兼容性：保留原 Wails 版元组签名，调用方（如果有）无需改动。
 * 当前 stores 仅用 getLatestSnapshots 批量端点，此函数保留作为 API 完整性。
 */
export async function getLatestSnapshot(id: string): Promise<[PressureSnapshot, boolean]> {
  try {
    const snapshot = await get<PressureSnapshot>(`/api/device/latest-snapshot/${encodeURIComponent(id)}`)
    return [snapshot, true]
  } catch {
    // HTTP 404 或网络错误：返回空对象 + false，与原 Wails 多返回值语义一致
    return [{} as PressureSnapshot, false]
  }
}

/**
 * 批量获取所有设备的最新快照（推荐前端轮询使用，减少 HTTP 请求次数）。
 *
 * @returns map[deviceId]PressureSnapshot，无活跃设备时返回空对象
 */
export function getLatestSnapshots(): Promise<Record<string, PressureSnapshot>> {
  return get<Record<string, PressureSnapshot>>('/api/device/latest-snapshots')
}

// ---- 事件订阅（WebSocket） ----
//
// 后端事件名与原 Wails 版完全一致，前端 wsClient 收到消息后按 event 字段路由。
// 3 个事件订阅保留原 onXxx/offXxx 命名，stores/App.vue 无需改动。

/** log 事件订阅句柄 */
let logUnsubscribe: (() => void) | null = null
/** device-state 事件订阅句柄 */
let deviceStateUnsubscribe: (() => void) | null = null
/** recording-warning 事件订阅句柄 */
let recordingWarningUnsubscribe: (() => void) | null = null

/**
 * 订阅日志事件（daq:log）。
 * App.EmitLog 推送，前端 logStore 据此实时显示。
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
 * 多参数事件：后端 WSHub.Emit(name, id, state) 会把 [id, state] 打包为数组推送，
 * 前端 onmessage 收到的 data 是数组，需解构后传给 handler。
 *
 * 触发场景：硬件适配器检测到设备断连等异步状态变更，通过 SetStateSink 回调
 * 注入到 App.EmitDeviceState，避免前端轮询 GetStatus。
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
 * 订阅录制期间设备断连警告事件（daq:recording-warning）。
 *
 * 仅在多设备录制场景下触发：某台设备断连但其他设备仍录制中，
 * App.handleRelayExit 检测到剩余 relay 数 > 0 时 emit 此事件，前端可提示用户
 * "设备 X 已断开，录制继续"。
 *
 * 单设备录制场景下设备断连会直接停止录制（通过 daq:recording-status 通知），
 * 不会触发此事件。
 */
export function onRecordingWarning(handler: (event: RecordingWarningEvent) => void): void {
  offRecordingWarning()
  recordingWarningUnsubscribe = on<RecordingWarningEvent>('daq:recording-warning', (data) => {
    handler(data)
  })
}

/** 解除录制警告订阅 */
export function offRecordingWarning(): void {
  if (recordingWarningUnsubscribe) {
    recordingWarningUnsubscribe()
    recordingWarningUnsubscribe = null
  }
}

// 注意：不在此处统一调用 closeWebSocket()，避免影响 recordingBridge/logBridge 的事件订阅。
// 应用卸载时由 App.vue onUnmounted 显式调用 closeWebSocket()（如果需要）。
// 当前 App.vue 仅调用 offXxx 解除订阅，不主动关连接，让 wsClient 在页面卸载时自动释放。
// 显式 closeWebSocket 可在测试环境调用以清理资源。
export { closeWebSocket } from './wsClient'
