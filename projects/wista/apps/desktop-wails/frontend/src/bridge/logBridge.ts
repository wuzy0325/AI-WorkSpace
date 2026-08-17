// Log Bridge —— Wails v3 版
//
// 通过生成的 LogService 绑定调用 Go 侧方法。
import {
  StartLogFile,
  StopLogFile,
  GetLogFileState,
  PickDirectory,
} from '../../bindings/wista/backend/logservice'

/** 日志文件写入状态 */
export interface LogFileState {
  active: boolean
  outputDir?: string
}

/** 开始将日志写入文件 */
export function startLogFile(outputDir: string, prefix: string): Promise<void> {
  return StartLogFile(outputDir, prefix) as Promise<void>
}

/** 停止日志文件写入 */
export function stopLogFile(): Promise<void> {
  return StopLogFile() as Promise<void>
}

/** 获取日志文件写入状态 */
export function getLogFileState(): Promise<LogFileState> {
  return GetLogFileState() as any
}

/** 选择目录对话框 */
export function pickDirectory(): Promise<string> {
  return PickDirectory() as Promise<string>
}
