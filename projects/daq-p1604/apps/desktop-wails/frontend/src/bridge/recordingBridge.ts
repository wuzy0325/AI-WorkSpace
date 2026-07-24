// Recording Bridge —— Win7 分支（HTTP + WebSocket）
//
// 与 Wails v3 版差异：
//   - RPC 调用：fetch http://127.0.0.1:18182/api/recording/* 替代 Wails 生成绑定
//   - 事件订阅：WebSocket 单例（wsClient.ts）替代 @wailsio/runtime Events.On
//   - PickDirectory 改由 Electron IPC 处理，bridge 仅保留入口以兼容调用方签名
//
// 与 daq-t1603 Win7 版的差异：
//   - 多了 startRecordingWithConfig 端点（daq-p1604 录制支持 FileRotation + StopConditions）
//   - 没有 recording-fatal / recording-backpressure 事件（daq-p1604 的 CSVRecorder
//     未实现这两个回调，背压通过 droppedCount 在 status 中体现）

import { get, post } from './httpClient'
import { on } from './wsClient'

/** 文件滚动条件（任一满足即滚动到新文件，0/undefined 表示不限制） */
export interface FileRotation {
  maxSizeBytes?: number
  maxDurationMs?: number
  maxRecordCount?: number
}

/** 自动停止条件（任一满足即停止整个录制，0/undefined 表示不限制） */
export interface StopConditions {
  maxDurationMs?: number
  maxFileSizeBytes?: number
  maxRecordCount?: number
}

/** 录制会话状态（与 core/recording.go RecordingSession 对应） */
export interface RecordingSession {
  id: string
  outputDir: string
  filePrefix: string
  startTimeMs: number
  snapshotCount: number   // 已写入快照数
  droppedCount: number    // 队列满时丢弃的快照数
  fileCount: number       // 已创建的文件数（含滚动）
  currentFile?: string    // 当前正在写入的文件完整路径（文件滚动时更新）
  lastError?: string      // 最近一次错误信息
  status: number          // 0=Idle 1=Active 2=Stopping
}

/**
 * 开始录制（不带滚动/停止条件，等价于 start-with-config 传零值）。
 *
 * @param outputDir 输出目录（绝对路径，由前端通过 Electron IPC 选择）
 * @param filePrefix 文件名前缀（如 "DAQ-P1604"）
 */
export function startRecording(outputDir: string, filePrefix: string): Promise<void> {
  return post('/api/recording/start', { outputDir, filePrefix })
}

/**
 * 开始录制并透传 FileRotation + StopConditions 配置。
 *
 * 适用于需要按大小/时长滚动文件、自动停止的复杂场景。
 * 启动后立即广播一次状态（由后端 App.StartRecordingWithConfig 内部完成）。
 */
export function startRecordingWithConfig(
  outputDir: string,
  filePrefix: string,
  rotation: FileRotation,
  stopConditions: StopConditions,
): Promise<void> {
  return post('/api/recording/start-with-config', {
    outputDir,
    filePrefix,
    rotation,
    stopConditions,
  })
}

/** 停止录制，flush 文件后关闭 */
export function stopRecording(): Promise<void> {
  return post('/api/recording/stop')
}

/** 查询当前录制状态 */
export function getRecordingStatus(): Promise<RecordingSession> {
  return get<RecordingSession>('/api/recording/status')
}

/**
 * 选择目录对话框。
 *
 * Win7 分支实现：通过 Electron preload 注入的 window.electronAPI.showOpenDialog()
 * 调用原生对话框。在浏览器开发环境（无 Electron）下回退返回空字符串。
 *
 * 兼容性：保留原 bridge 签名，调用方（MainTopBar.vue / logStore.ts）无需改动。
 */
export async function pickDirectory(): Promise<string> {
  // Electron preload 注入的全局 API（详见 env.d.ts 与 desktop-electron/preload.cjs）
  if (window.electronAPI?.showOpenDialog) {
    return await window.electronAPI.showOpenDialog()
  }
  // 浏览器开发环境（vite dev server）下无 Electron，返回空串让调用方取消
  console.warn('[recordingBridge] electronAPI.showOpenDialog not available, returning empty string')
  return ''
}

/** recording-status 事件订阅句柄 */
let recordingStatusUnsubscribe: (() => void) | null = null

/**
 * 订阅录制状态变更事件（daq:recording-status）。
 * App.emitRecordingStatus 在录制启停、每秒周期、relay 收尾时推送。
 */
export function onRecordingStatus(handler: (session: RecordingSession) => void): void {
  offRecordingStatus()
  recordingStatusUnsubscribe = on<RecordingSession>('daq:recording-status', (data) => {
    handler(data)
  })
}

/** 解除录制状态订阅 */
export function offRecordingStatus(): void {
  if (recordingStatusUnsubscribe) {
    recordingStatusUnsubscribe()
    recordingStatusUnsubscribe = null
  }
}
