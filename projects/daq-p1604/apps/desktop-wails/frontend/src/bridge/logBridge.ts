// Log Bridge —— Win7 分支（HTTP）
//
// 与 Wails v3 版差异：
//   - RPC 调用：fetch http://127.0.0.1:18182/api/log/* 替代 Wails 生成绑定
//   - 无事件订阅：日志事件通过 deviceBridge.onLog 统一订阅（daq:log），
//     logBridge 仅负责 RPC 调用
//   - PickDirectory 改由 Electron IPC 处理

import { get, post } from './httpClient'

/** 日志文件写入状态（与 backend.LogFileState 对应） */
export interface LogFileState {
  active: boolean
  outputDir: string
}

/**
 * 开启日志文件写入。
 *
 * @param outputDir 输出目录（绝对路径，由前端通过 Electron IPC 选择）
 * @param prefix 文件名前缀（如 "daq-log"）
 */
export function startLogFile(outputDir: string, prefix: string): Promise<void> {
  return post('/api/log/start', { outputDir, prefix })
}

/** 停止日志文件写入，flush 已写入内容后关闭 */
export function stopLogFile(): Promise<void> {
  return post('/api/log/stop')
}

/** 查询日志文件当前状态 */
export function getLogFileState(): Promise<LogFileState> {
  return get<LogFileState>('/api/log/state')
}

/**
 * 选择目录对话框。
 *
 * Win7 分支实现：通过 Electron preload 注入的 window.electronAPI.showOpenDialog()
 * 调用原生对话框。在浏览器开发环境下回退返回空字符串。
 *
 * 兼容性：保留原 bridge 签名，调用方（logStore.pickLogDir）无需改动。
 */
export async function pickDirectory(): Promise<string> {
  if (window.electronAPI?.showOpenDialog) {
    return await window.electronAPI.showOpenDialog()
  }
  console.warn('[logBridge] electronAPI.showOpenDialog not available, returning empty string')
  return ''
}
