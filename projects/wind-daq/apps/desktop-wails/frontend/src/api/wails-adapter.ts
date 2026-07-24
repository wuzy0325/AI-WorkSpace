// 桌面端 API 适配器 —— Electron 主进程 + Go HTTP 后端
//
// 设计要点：
//   1. Go 后端（wind-daq-backend.exe）启动后监听 127.0.0.1:8900，
//      通过 http.ServeMux 暴露 /api/* 路由，同时通过 //go:embed 提供前端静态资源。
//   2. Electron 主进程创建 BrowserWindow 后加载 http://127.0.0.1:8900，
//      渲染进程通过 fetch 调用同源 HTTP API，避免跨域与 origin 混乱。
//   3. 历史上 wind-daq 走 Wails v3 binding，本文件已完全移除 @wailsio/runtime 依赖，
//      统一改为 HTTP fetch + Electron IPC（仅文件对话框和 motion 独立窗口走 IPC）。
//   4. `isWailsAvailable()` 保留为兼容导出，语义已改为"是否运行在桌面端环境"
//      （Electron 模式下 window.electronAPI 由 preload 注入）。

import { request } from './http-client'
import { subscribeDaqStream } from './sse-client'

// =====================================================================
// 类型定义（与后端 services/api-go 类型对齐）
// =====================================================================

// 通用响应信封：与 Go 端 writeJSON(w, 200, map[string]bool{"success": true}) 对齐
export interface GenericResponse {
  success: boolean
  error?: string
  /** @deprecated Wails 时代返回大写 Success；保留以兼容历史调用点 */
  Success: boolean
  /** @deprecated Wails 时代返回大写 Error；保留以兼容历史调用点 */
  Error?: string
  /**
   * 返回数据载荷（与后端 GenericResponse.Data 对齐，spec Task 13 起新增）。
   *
   * 仅在需要返回数据的接口（如 sevenhole-preview 返回点位预览结果）中填充；
   * 简单成功/失败响应不填此字段，运行时 omitempty 省略。
   *
   * 类型由调用方按需断言（如 `res.Data as SevenHolePreviewResult`），
   * 此处保留 unknown 以保持 GenericResponse 通用性。
   */
  data?: unknown
  /** @deprecated 与 data 同义，保留 Data 仅为兼容大写字段旧调用点 */
  Data?: unknown
}

export interface FileResponse extends GenericResponse {
  filepath?: string
  /** @deprecated Wails 时代返回大写 Filepath；保留以兼容历史调用点 */
  Filepath?: string
}

export interface VersionInfo {
  name: string
  version: string
}

export interface DeviceProfile {
  id: string
  name: string
  type: string
  samplingRate: number
  channels: any[]
  address?: string
}

export interface DeviceStatus {
  id: string
  name: string
  type: string
  connection: string
  acquiring: boolean
  lastError?: string
}

export interface DeviceScanResult {
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

export interface DeviceDataPayload {
  deviceId: string
  timestamp: number
  channels: number[]
  channelIndices: number[]
}

export interface MotionProfile {
  id: string
  name: string
  type: string
  address: string
  port: number
  autoConnect: boolean
  axes: any[]
}

export interface MotionStatus {
  id: string
  name: string
  type: string
  connected: boolean
  emergencyStopped: boolean
  axes: any[]
  lastError?: string
}

export interface CalibrationConfig {
  taskId?: string
  deviceId?: string
  type: string
  channels?: number[]
  pressurePoints?: number[]
  averageSamples?: number
  probeChannels?: any[]
  points?: any[]
  samplesPerPoint?: number
  dwellTimeMs?: number
  stopOnError?: boolean
}

export interface CalibrationStatus {
  taskId: string
  state: string
  currentPoint: number
  totalPoints: number
  results: any[]
  lastError?: string
  /**
   * 暂停累计时长（毫秒，spec Task 14 新增）。
   * 与后端 core/calibration.Status.PausedDurationMs 对齐，
   * 由后端在校准暂停/恢复时累加。
   */
  pausedDurationMs?: number
  /**
   * 实时物理量快照（spec Task 13/14，与后端 core/calibration.LivePhysics 对齐）。
   *
   * 三态语义（与 *float64 指针语义一致）：
   *   - 字段省略（undefined）：缺失（必需通道未配置/读取失败/物理非法如 Pt < Ps）
   *   - 字段值为 0：有效零（Pt == Ps 即零流量，Task 12）
   *   - 字段值为正数：正常计算值
   *
   * 整体 livePhysics 省略（undefined）：类型不支持（总温）或未启动校准。
   * 整体存在但字段省略：类型支持但运行期读取失败。
   *
   * 关键不变量：前端不得用 truthiness 判断 `livePhysics.machNumber`——0 是有效零。
   */
  livePhysics?: {
    machNumber?: number
    velocity?: number
  }
}

// SevenHoleConfig 与 shared/types/calibration.SevenHoleConfig 对齐，
// 用于 sevenhole-preview 调用——透传到后端 GenerateSevenHolePoints。
// 字段命名遵循后端 json tag（SevenHoleConfigDTO 直接是 calibration.SevenHoleConfig 别名）。
export interface SevenHoleConfig {
  mode: string // 'full' | 'dataset'
  innerAlphaMin: number
  innerAlphaMax: number
  innerAlphaStep: number
  innerBetaMin: number
  innerBetaMax: number
  innerBetaStep: number
  outerThetaMin: number
  outerThetaMax: number
  outerThetaStep: number
  outerPhiMin: number
  outerPhiMax: number
  outerPhiStep: number
  serpentine: boolean
}

// FiveHolePointLayout 与 shared/types/calibration.FiveHolePointLayout 对齐，
// 用于 fivehole-preview 调用（spec Task 11）——
// 透传到后端 usecase.PreviewFiveHolePoints（再调 core.GenerateFiveHoleSnakePoints）。
// 字段命名遵循后端 json tag（FiveHolePointLayoutDTO 直接是 calibration.FiveHolePointLayout 别名）。
export interface FiveHolePointLayout {
  alphaMin: number
  alphaMax: number
  alphaStep: number
  betaMin: number
  betaMax: number
  betaStep: number
  /** 蛇形走位：奇数行反向遍历 α；默认 false 为逐行 raster 扫描 */
  serpentine?: boolean
}

// StorageRecordingConfig 与后端 storage.RecordingConfig 对齐，
// 由 UI 在 Start 时传入，贯穿 usecase -> sink。
export interface StorageRecordingConfig {
  outputDir: string
  filePrefix: string
  /** 存储格式："csv"（默认）或 "binary"；为空时由装配层默认值决定 */
  format?: string
  /** 自动停止条件，全部为零值表示不限制 */
  stopConditions?: {
    maxDurationMs?: number
    maxFileSizeBytes?: number
    maxRecordCount?: number
  }
  /** 文件滚动配置 */
  fileRotation?: {
    enabled: boolean
    maxFileSizeBytes: number
    maxDurationMs: number
  }
  /** 是否在采集启动时自动开始录制（仅由编排层消费，sink 不消费） */
  autoStartOnAcquisition?: boolean
}

// StorageRecordingStatus 与后端 storage.RecordingStatus 对齐，
// 包含运行时统计字段（文件、大小、记录数、丢弃数、错误等）。
export interface StorageRecordingStatus {
  recording: boolean
  outputDir?: string
  /** 当前写入的文件名（不含目录） */
  currentFile?: string
  /** 当前文件累计字节数 */
  fileSize?: number
  /** 本会话已滚动的文件数（含当前文件，从 1 开始） */
  fileCount?: number
  /** 本会话累计写入的记录条数 */
  recordCount?: number
  /** 本会话累计录制时长（毫秒） */
  durationMs?: number
  /** 因队列满被丢弃的 payload 数 */
  droppedCount?: number
  /** 最近一次 I/O 错误描述（空表示无错误） */
  lastError?: string
}

export interface ReportStatus {
  generating: boolean
  lastResult?: string
}

export interface TraversalPoint {
  x: number
  y: number
  z: number
}

export interface TraversalStatus {
  state: string
  currentPoint: number
  totalPoints: number
}

// DSA3217 扫描配置响应
export interface DSA3217ScanConfigWailsResponse {
  Success: boolean
  Data?: {
    Avg: number
    Period: number
    Fps: string
    Unit: string
  }
  Error?: string
}

// 配置加载结果
export interface ConfigLoadResult<T = unknown> {
  success: boolean
  data?: T
  error?: string
}

// =====================================================================
// 环境探测
// =====================================================================

// Electron preload 注入的 window.electronAPI 接口签名
// （参见 apps/desktop-electron/preload.cjs）
interface ElectronAPI {
  showOpenDialog: () => Promise<string>
  openMotionWindow: () => Promise<boolean>
  /** 文件存在性检查（spec Task 13 新增，preload 未实现时为 undefined） */
  fileExists?: (path: string) => Promise<boolean>
  /** 删除文件（spec Task 13 新增，preload 未实现时为 undefined） */
  removeFile?: (path: string) => Promise<boolean>
  /** 获取安装程序语言（spec Task 13 新增，preload 未实现时为 undefined） */
  getInstallerLanguage?: () => Promise<string>
  /** 请求退出应用（spec Task 13 新增，preload 未实现时为 undefined） */
  requestExit?: () => Promise<boolean>
  /** 监听主进程推送的退出请求事件（spec Task 13 新增，preload 未实现时为 undefined） */
  onExitRequested?: (callback: () => void) => () => void
}

interface WindowWithElectronAPI extends Window {
  electronAPI?: ElectronAPI
}

/**
 * 检测是否运行在桌面端环境。
 *
 * 历史命名为 isWailsAvailable，保留导出以避免大量调用点改名。
 * 当前实现：检测 Electron preload 注入的 window.electronAPI 是否存在。
 * - 浏览器开发态（npm run dev）：返回 false，所有调用走相对路径 fetch（命中 Vite proxy）
 * - Electron 桌面端：返回 true，调用走绝对 URL fetch 或 IPC
 */
export const isWailsAvailable = (): boolean => {
  if (typeof window === 'undefined') return false
  const w = window as WindowWithElectronAPI
  return Boolean(w.electronAPI)
}

/**
 * 归一化 GenericResponse 的大小写差异。
 *
 * 后端 HTTP 返回的是小写 success/error，但项目内不少调用点写成了
 * `res.Success`/`res.Error`（大写，源自 Wails 时代的字段）。
 * 这里统一在 callBindingGeneric 出口把两套字段同时填充，保持向后兼容。
 *
 * data/Data 字段同样双写：后端 JSON 序列化为小写 data，
 * 旧调用点可能用 res.Data 读取，这里同时填充两套字段。
 */
function normalizeGenericResponse(raw: unknown): GenericResponse {
  const obj = (raw ?? {}) as Record<string, unknown>
  const success = (obj.success ?? obj.Success ?? false) as boolean
  const error = (obj.error ?? obj.Error) as string | undefined
  const data = (obj.data ?? obj.Data) as unknown
  return { success, error, Success: success, Error: error, data, Data: data }
}

// =====================================================================
// HTTP 便捷封装
// =====================================================================

/** GET 请求并归一化为 GenericResponse */
async function getGeneric(path: string): Promise<GenericResponse> {
  return normalizeGenericResponse(await request<unknown>(path))
}

/** POST 请求并归一化为 GenericResponse */
async function postGeneric(path: string, body?: unknown): Promise<GenericResponse> {
  return normalizeGenericResponse(await request<unknown>(path, {
    method: 'POST',
    body: body !== undefined ? JSON.stringify(body) : undefined,
  }))
}

/** PUT 请求并归一化为 GenericResponse */
async function putGeneric(path: string, body?: unknown): Promise<GenericResponse> {
  return normalizeGenericResponse(await request<unknown>(path, {
    method: 'PUT',
    body: body !== undefined ? JSON.stringify(body) : undefined,
  }))
}

/** DELETE 请求并归一化为 GenericResponse */
async function deleteGeneric(path: string): Promise<GenericResponse> {
  return normalizeGenericResponse(await request<unknown>(path, { method: 'DELETE' }))
}

// =====================================================================
// 桌面端 API 适配器
// =====================================================================

export const wailsApi = {
  // -------------------------------------------------------------------
  // 设备管理 —— 全部走 HTTP API（与 services/api-go/api/server.go 路由对齐）
  // -------------------------------------------------------------------
  device: {
    getProfiles: async (): Promise<DeviceProfile[]> => {
      return await request<DeviceProfile[]>('/api/device/profiles')
    },
    upsertProfile: async (profile: DeviceProfile): Promise<GenericResponse> => {
      return await putGeneric('/api/device/profiles', profile)
    },
    deleteProfile: async (profileId: string): Promise<GenericResponse> => {
      return await deleteGeneric(`/api/device/profiles/${profileId}`)
    },
    scanDevices: async (): Promise<DeviceScanResult[]> => {
      return await request<DeviceScanResult[]>('/api/device/scan')
    },
    connect: async (deviceId: string): Promise<GenericResponse> => {
      return await postGeneric(`/api/device/${deviceId}/connect`)
    },
    disconnect: async (deviceId: string): Promise<GenericResponse> => {
      return await postGeneric(`/api/device/${deviceId}/disconnect`)
    },
    getStatus: async (deviceId: string): Promise<DeviceStatus> => {
      return await request<DeviceStatus>(`/api/device/${deviceId}/status`)
    },
    startAcquisition: async (deviceId: string): Promise<GenericResponse> => {
      return await postGeneric(`/api/device/${deviceId}/startAcquisition`)
    },
    stopAcquisition: async (deviceId: string): Promise<GenericResponse> => {
      return await postGeneric(`/api/device/${deviceId}/stopAcquisition`)
    },
    getLatestData: async (deviceId: string): Promise<DeviceDataPayload> => {
      return await request<DeviceDataPayload>(`/api/daq/latest/${deviceId}`)
    },
    setPublishRate: async (rate: number): Promise<GenericResponse> => {
      return await putGeneric('/api/daq/publishRate', { hz: rate })
    },
    getPublishRate: async (): Promise<number> => {
      const result = await request<{ hz: number }>('/api/daq/publishRate')
      return result.hz
    },
    subscribeStream: async (_deviceId: string, _subscribe: boolean): Promise<GenericResponse> => {
      // SSE 由 deviceApi.subscribeToDevice 内部直接调用 subscribeDaqStream 实现，
      // 此方法保留以兼容旧调用点签名，但不再执行任何后端 subscribe 动作
      // （后端 SSE 始终推送，前端按需订阅）。
      return { success: true, error: undefined, Success: true, Error: undefined, data: undefined, Data: undefined }
    },
    /**
     * 订阅设备实时数据流。
     * 通过 SSE（/api/daq/stream/{id}）接收后端推送的 DataPayload。
     * 返回取消订阅函数。
     */
    onPayload: (deviceId: string, callback: (payload: DeviceDataPayload) => void): (() => void) => {
      const subscription = subscribeDaqStream(
        deviceId,
        (payload) => callback(payload as DeviceDataPayload),
        (error) => console.log(`SSE for ${deviceId}:`, error),
      )
      return () => subscription.unsubscribe()
    },
    getDsa3217ScanConfig: async (deviceId: string): Promise<DSA3217ScanConfigWailsResponse> => {
      return await request<DSA3217ScanConfigWailsResponse>(`/api/device/${deviceId}/dsa3217ScanConfig`)
    },
    applyDsa3217ScanConfig: async (deviceId: string, avg: number, period: number): Promise<DSA3217ScanConfigWailsResponse> => {
      return await request<DSA3217ScanConfigWailsResponse>(`/api/device/${deviceId}/dsa3217ScanConfig`, {
        method: 'PUT',
        body: JSON.stringify({ avg, period }),
      })
    },
  },

  // -------------------------------------------------------------------
  // 运动控制 —— 走 HTTP API（/api/motion/*）
  // -------------------------------------------------------------------
  motion: {
    getProfiles: async (): Promise<MotionProfile[]> => {
      const raw = await request<MotionProfile[]>('/api/motion/profiles')
      return Array.isArray(raw) ? raw : []
    },
    upsertProfile: async (profile: MotionProfile): Promise<GenericResponse> => {
      return await putGeneric('/api/motion/profiles', profile)
    },
    deleteProfile: async (profileId: string): Promise<GenericResponse> => {
      return await deleteGeneric(`/api/motion/profiles/${profileId}`)
    },
    connect: async (controllerId: string): Promise<GenericResponse> => {
      return await postGeneric('/api/motion/connect', { id: controllerId })
    },
    disconnect: async (controllerId: string): Promise<GenericResponse> => {
      return await postGeneric('/api/motion/disconnect', { id: controllerId })
    },
    getStatus: async (): Promise<MotionStatus[]> => {
      const raw = await request<MotionStatus[]>('/api/motion/status')
      return Array.isArray(raw) ? raw : []
    },
    home: async (controllerId: string, axisName: string): Promise<GenericResponse> => {
      return await postGeneric('/api/motion/home', { id: controllerId, axis: axisName })
    },
    moveTo: async (controllerId: string, axisName: string, position: number): Promise<GenericResponse> => {
      return await postGeneric('/api/motion/moveTo', { id: controllerId, axis: axisName, position })
    },
    moveBy: async (controllerId: string, axisName: string, distance: number): Promise<GenericResponse> => {
      return await postGeneric('/api/motion/moveBy', { id: controllerId, axis: axisName, delta: distance })
    },
    jog: async (controllerId: string, axisName: string, velocity: number): Promise<GenericResponse> => {
      return await postGeneric('/api/motion/jog', { id: controllerId, axis: axisName, velocity })
    },
    stop: async (controllerId: string, axisName: string): Promise<GenericResponse> => {
      return await postGeneric('/api/motion/stop', { id: controllerId, axis: axisName })
    },
    emergencyStop: async (controllerId: string): Promise<GenericResponse> => {
      return await postGeneric('/api/motion/emergencyStop', { id: controllerId })
    },
    definePosition: async (controllerId: string, axisName: string, position: number): Promise<GenericResponse> => {
      return await postGeneric('/api/motion/definePosition', { id: controllerId, axis: axisName, position })
    },
    resetEmergencyStop: async (controllerId: string): Promise<GenericResponse> => {
      return await postGeneric('/api/motion/resetEmergencyStop', { id: controllerId })
    },
    /**
     * 运动状态事件订阅。
     * 保留与原 Wails 时代相同的回调签名（callback），但内部已改为 HTTP 轮询：
     * - 运动中：250ms 高频
     * - 空闲：2000ms 心跳
     * 超时 2500ms 容纳 B140 四轴 14 次 TCP 往返的正常耗时。
     */
    onStatusEvent: (callback: (data: unknown) => void): (() => void) => {
      const STATUS_API = '/api/motion/status'
      const FAST_INTERVAL_MS = 250
      const SLOW_INTERVAL_MS = 2000
      let active = true
      let currentInterval = FAST_INTERVAL_MS
      let timer: ReturnType<typeof setTimeout> | null = null

      const scheduleNext = (delay: number): void => {
        if (!active) return
        timer = setTimeout(() => { void poll() }, delay)
      }

      const poll = async (): Promise<void> => {
        if (!active) return
        const controller = new AbortController()
        const timeoutId = setTimeout(() => controller.abort(), 2500)
        try {
          const resp = await fetch(STATUS_API, { signal: controller.signal })
          if (resp.ok) {
            const statuses = await resp.json()
            if (active && Array.isArray(statuses)) {
              try {
                callback(statuses)
              } catch (err) {
                console.error('[wails-adapter] motion status callback threw:', err)
              }
              const hasMoving = (statuses as Array<{ axes?: Array<{ moving?: boolean }> }>)
                .some((s) => Array.isArray(s.axes) && s.axes.some((a) => a.moving === true))
              currentInterval = hasMoving ? FAST_INTERVAL_MS : SLOW_INTERVAL_MS
            }
          }
        } catch {
          // 网络错误或超时静默重试，避免日志噪音
        } finally {
          clearTimeout(timeoutId)
        }
        scheduleNext(currentInterval)
      }

      void poll()

      return () => {
        active = false
        if (timer !== null) {
          clearTimeout(timer)
          timer = null
        }
      }
    },
  },

  // -------------------------------------------------------------------
  // 校准管理 —— 走 HTTP API（/api/calibration/*）
  // -------------------------------------------------------------------
  calibration: {
    start: async (config: CalibrationConfig): Promise<GenericResponse> => {
      return await postGeneric('/api/calibration/start', config)
    },
    stop: async (): Promise<GenericResponse> => {
      return await postGeneric('/api/calibration/stop')
    },
    pause: async (): Promise<GenericResponse> => {
      return await postGeneric('/api/calibration/pause')
    },
    resume: async (): Promise<GenericResponse> => {
      return await postGeneric('/api/calibration/resume')
    },
    collect: async (): Promise<GenericResponse> => {
      return await postGeneric('/api/calibration/collect')
    },
    status: async (): Promise<CalibrationStatus> => {
      return await request<CalibrationStatus>('/api/calibration/status')
    },
    getResult: async (taskId: string): Promise<CalibrationStatus> => {
      return await request<CalibrationStatus>(`/api/calibration/result?taskId=${encodeURIComponent(taskId)}`)
    },
    saveCsv: async (taskId: string, savePath: string): Promise<FileResponse> => {
      const raw = await request<{ success: boolean; filepath?: string; error?: string }>(
        '/api/calibration/saveCsv',
        { method: 'POST', body: JSON.stringify({ taskId, savePath }) },
      )
      const normalized = normalizeGenericResponse(raw)
      const obj = (raw ?? {}) as Record<string, unknown>
      const filepath = obj.filepath as string | undefined
      return { ...normalized, filepath, Filepath: filepath }
    },
    /**
     * 七孔点位预览（spec Task 13/18）
     *
     * 调用后端 POST /api/calibration/sevenhole-preview，纯计算不涉及 I/O：
     *   - 接收前端配置向导提交的 SevenHoleConfig（α/β/θ/φ 范围与步长）
     *   - 调用 usecase.PreviewSevenHolePoints 生成完整点位 + 内/外区聚合统计
     *   - 返回 SevenHolePreviewResult（points + totalCount + innerCount + outerCount）
     *
     * 返回 GenericResponse：success=false 时 error 透传 GenerateSevenHolePoints 错误
     * （如步长 ≤ 0、范围 min > max）；success=true 时 data 字段为 SevenHolePreviewResult。
     *
     * 注意：Win7 后端目前未实现该路由（404），UI 调用时会抛错；
     * 调用方应捕获并降级提示"七孔预览需主进程后端支持"。
     */
    previewSevenHole: async (config: SevenHoleConfig): Promise<GenericResponse> => {
      return await postGeneric('/api/calibration/sevenhole-preview', config)
    },
    /**
     * 五孔点位预览（spec Task 11）
     *
     * 调用后端 POST /api/calibration/fivehole，纯计算不涉及 I/O：
     *   - 接收前端配置向导提交的 FiveHolePointLayout（α/β 范围与步长 + serpentine 开关）
     *   - 调用 usecase.PreviewFiveHolePoints（再调 core.GenerateFiveHoleSnakePoints）
     *   - 返回 []FiveHoleSnakePoint（bare array）
     *
     * 返回 GenericResponse：success=false 时 error 透传 GenerateFiveHoleSnakePoints 错误
     * （如步长 ≤ 0）；success=true 时 data 字段为 []FiveHoleSnakePoint。
     *
     * 与 previewSevenHole 的区别：
     *   - 五孔 data 是 bare array（[]FiveHoleSnakePoint），前端直接迭代
     *   - 七孔 data 是包装对象（SevenHolePreviewResult，含 totalCount/innerCount/outerCount）
     *
     * 注意：Win7 后端目前未实现该路由（404），UI 调用时会抛错。
     */
    previewFiveHole: async (layout: FiveHolePointLayout): Promise<GenericResponse> => {
      return await postGeneric('/api/calibration/fivehole', layout)
    },
  },

  // -------------------------------------------------------------------
  // 存储管理 —— 走 HTTP API（/api/storage/*）
  // -------------------------------------------------------------------
  storage: {
    startRecording: async (config: StorageRecordingConfig): Promise<GenericResponse> => {
      return await postGeneric('/api/storage/start', config)
    },
    stopRecording: async (): Promise<GenericResponse> => {
      return await postGeneric('/api/storage/stop')
    },
    getStatus: async (): Promise<StorageRecordingStatus> => {
      return await request<StorageRecordingStatus>('/api/storage/status')
    },
  },

  // -------------------------------------------------------------------
  // 报告管理 —— 走 HTTP API（/api/report/*）
  // -------------------------------------------------------------------
  report: {
    getStatus: async (): Promise<ReportStatus> => {
      return await request<ReportStatus>('/api/report/status')
    },
  },

  // -------------------------------------------------------------------
  // 配置管理 —— 走 HTTP API（/api/config/{key}）
  // 后端 GET 返回 {success, data, error}，PUT 接收任意 JSON 体作为配置值
  // -------------------------------------------------------------------
  config: {
    load: async <T = unknown>(key: string): Promise<ConfigLoadResult<T>> => {
      try {
        return await request<ConfigLoadResult<T>>(`/api/config/${encodeURIComponent(key)}`)
      } catch (e) {
        return { success: false, error: String(e) }
      }
    },
    save: async (key: string, config: unknown): Promise<GenericResponse> => {
      try {
        return await putGeneric(`/api/config/${encodeURIComponent(key)}`, config)
      } catch (e) {
        return normalizeGenericResponse({ success: false, error: String(e) })
      }
    },
  },

  // -------------------------------------------------------------------
  // 应用通用 —— 版本走 HTTP API；目录选择和 motion 独立窗口走 Electron IPC
  // -------------------------------------------------------------------
  app: {
    getVersion: async (): Promise<VersionInfo> => {
      return await request<VersionInfo>('/api/app/version')
    },
    /**
     * 弹出原生目录选择对话框。
     * - Electron 桌面端：通过 IPC 调用 Electron 主进程的 dialog.showOpenDialog
     * - 浏览器开发态：返回空字符串（不支持文件对话框）
     */
    pickDirectory: async (): Promise<string> => {
      const w = window as WindowWithElectronAPI
      if (w.electronAPI) {
        return await w.electronAPI.showOpenDialog()
      }
      return ''
    },
    resolvePath: async (p: string): Promise<string> => {
      const result = await request<{ path: string } | string>('/api/app/resolve-path', {
        method: 'POST',
        body: JSON.stringify({ path: p }),
      })
      // 后端可能返回字符串或对象，统一适配
      if (typeof result === 'string') return result
      return (result as { path: string }).path ?? ''
    },
    pickFile: async (_title: string, _filters: Array<{ displayName: string; pattern: string }>): Promise<string> => {
      // Electron 暂未实现 pickFile IPC，预留接口
      return ''
    },
    pickFiles: async (_title: string, _filters: Array<{ displayName: string; pattern: string }>): Promise<string[]> => {
      // Electron 暂未实现 pickFiles IPC，预留接口
      return []
    },
    pickSaveFile: async (
      _title: string,
      _defaultFilename: string,
      _filters: Array<{ displayName: string; pattern: string }>,
    ): Promise<string> => {
      // Electron 暂未实现 pickSaveFile IPC，预留接口
      return ''
    },
    /**
     * 检查文件是否存在。
     * - Electron 桌面端：通过 IPC 调用主进程 fs.access（preload 未实现时返回 false）
     * - 浏览器开发态：返回 false
     *
     * 用于校准配置加载时校验用户输入的文件路径是否有效。
     */
    fileExists: async (path: string): Promise<boolean> => {
      const w = window as WindowWithElectronAPI
      if (w.electronAPI?.fileExists) {
        try {
          return await w.electronAPI.fileExists(path)
        } catch {
          return false
        }
      }
      return false
    },
    /**
     * 删除文件。
     * - Electron 桌面端：通过 IPC 调用主进程 fs.unlink（preload 未实现时返回 false）
     * - 浏览器开发态：返回 false
     *
     * 用于清理过期临时文件（如旧版本校准快照）。
     */
    removeFile: async (path: string): Promise<boolean> => {
      const w = window as WindowWithElectronAPI
      if (w.electronAPI?.removeFile) {
        try {
          return await w.electronAPI.removeFile(path)
        } catch {
          return false
        }
      }
      return false
    },
    /** 获取启动模式："normal" 或 "motion"（motion-only 子进程模式） */
    getStartupMode: async (): Promise<string> => {
      const text = await request<string>('/api/app/startup-mode', { headers: { Accept: 'text/plain' } })
      return typeof text === 'string' ? text : 'normal'
    },
    /**
     * 获取安装程序写入的语言偏好（NSIS 安装时记录）。
     * - Electron 桌面端：通过 IPC 读取主进程缓存（preload 未实现时返回空字符串）
     * - 浏览器开发态：返回空字符串
     *
     * 返回值："zh"、"en" 或空字符串（未设置时使用前端默认语言）。
     */
    getInstallerLanguage: async (): Promise<string> => {
      const w = window as WindowWithElectronAPI
      if (w.electronAPI?.getInstallerLanguage) {
        try {
          return await w.electronAPI.getInstallerLanguage()
        } catch {
          return ''
        }
      }
      return ''
    },
    /**
     * 启动运动控制器独立窗口。
     * - Electron 桌面端：通过 IPC 调用 Electron 主进程
     *   主进程会 spawn motion-only 子进程（监听 8901）并创建独立 BrowserWindow
     * - 浏览器开发态：返回失败（不支持独立窗口）
     */
    openMotionWindow: async (): Promise<GenericResponse> => {
      const w = window as WindowWithElectronAPI
      if (w.electronAPI) {
        try {
          await w.electronAPI.openMotionWindow()
          return { success: true, error: undefined, Success: true, Error: undefined, data: undefined, Data: undefined }
        } catch (e) {
          const msg = String(e)
          return { success: false, error: msg, Success: false, Error: msg, data: undefined, Data: undefined }
        }
      }
      return {
        success: false,
        error: 'openMotionWindow 仅在 Electron 桌面端可用',
        Success: false,
        Error: 'openMotionWindow 仅在 Electron 桌面端可用',
        data: undefined,
        Data: undefined,
      }
    },
    /**
     * 请求退出应用：用户在确认对话框中点击"退出"后调用。
     *
     * Electron 模式下：
     *   - 若 preload 注入了 requestExit IPC，则通过 IPC 调用主进程触发完整关闭流程
     *   - 否则降级为 window.close()，由 main.cjs 的 'closed' 事件触发清理
     * 浏览器开发态下：调用 window.close() 关闭当前标签页。
     */
    requestExit: async (): Promise<GenericResponse> => {
      const w = window as WindowWithElectronAPI
      if (w.electronAPI?.requestExit) {
        try {
          const ok = await w.electronAPI.requestExit()
          return {
            success: ok,
            error: ok ? undefined : 'requestExit IPC returned false',
            Success: ok,
            Error: ok ? undefined : 'requestExit IPC returned false',
            data: undefined,
            Data: undefined,
          }
        } catch (e) {
          const msg = String(e)
          return { success: false, error: msg, Success: false, Error: msg, data: undefined, Data: undefined }
        }
      }
      // 降级：直接关闭当前窗口，Electron 主进程会感知到 'closed' 事件并清理子进程
      window.close()
      return { success: true, error: undefined, Success: true, Error: undefined, data: undefined, Data: undefined }
    },
    /**
     * 监听主进程推送的退出请求事件（用户点 X 按钮时主进程通过 IPC 推送）。
     * 调用方应在回调中弹出确认对话框，根据用户选择决定是否调用 requestExit()。
     *
     * - Electron 桌面端且 preload 注入了 onExitRequested：监听 IPC 事件
     * - 其他场景：返回 noop（不监听，用户直接通过浏览器/系统关闭按钮退出）
     *
     * 返回取消监听函数。
     */
    onExitRequested: (callback: () => void): (() => void) => {
      const w = window as WindowWithElectronAPI
      if (w.electronAPI?.onExitRequested) {
        try {
          return w.electronAPI.onExitRequested(callback)
        } catch {
          return () => {}
        }
      }
      return () => {}
    },
  },
}
