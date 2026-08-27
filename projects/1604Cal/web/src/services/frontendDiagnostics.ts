import type { Router } from 'vue-router'
import { useMeasurementStore } from '@/stores/measurement'

interface ChromiumMemoryInfo {
  usedJSHeapSize: number
  totalJSHeapSize: number
  jsHeapSizeLimit: number
}

const HEARTBEAT_INTERVAL_MS = 60_000
let logWriteInFlight = false
let pendingLogMessage = ''

function formatError(value: unknown): string {
  if (value instanceof Error) return `${value.name}: ${value.message}\n${value.stack ?? ''}`
  return String(value)
}

function appendFrontendLog(message: string): void {
  const append = window.go?.main?.App?.AppendFrontendLog
  if (typeof append !== 'function') return
  if (logWriteInFlight) {
    pendingLogMessage = message
    return
  }

  logWriteInFlight = true
  Promise.resolve()
    .then(() => append(message))
    .catch(() => undefined)
    .finally(() => {
      logWriteInFlight = false
      if (pendingLogMessage) {
        const pending = pendingLogMessage
        pendingLogMessage = ''
        appendFrontendLog(pending)
      }
    })
}

function memorySnapshot(): string {
  const memory = (performance as Performance & { memory?: ChromiumMemoryInfo }).memory
  if (!memory) return 'heap=unavailable'
  const mb = (bytes: number) => Math.round(bytes / 1024 / 1024)
  return `heapUsedMB=${mb(memory.usedJSHeapSize)} heapTotalMB=${mb(memory.totalJSHeapSize)} heapLimitMB=${mb(memory.jsHeapSizeLimit)}`
}

/**
 * 安装版前端诊断：错误立即落盘，运行状态每分钟写入后端日志。
 * 诊断随应用进程存活，无卸载时机，因此不返回清理函数。
 */
export function installFrontendDiagnostics(router: Router): void {
  const writeHeartbeat = () => {
    const measurement = useMeasurementStore()
    appendFrontendLog(
      `heartbeat route=${router.currentRoute.value.fullPath} measurementState=${measurement.state} ` +
      `rows=${measurement.rows.length} points=${measurement.points.length} ${memorySnapshot()}`
    )
  }

  appendFrontendLog(`diagnostics started userAgent=${navigator.userAgent}`)
  writeHeartbeat()
  window.setInterval(writeHeartbeat, HEARTBEAT_INTERVAL_MS)

  const onError = (event: ErrorEvent) => {
    appendFrontendLog(`window.error ${event.message} ${event.filename}:${event.lineno}:${event.colno} ${formatError(event.error)}`)
  }
  const onRejection = (event: PromiseRejectionEvent) => {
    appendFrontendLog(`unhandledrejection ${formatError(event.reason)}`)
  }
  window.addEventListener('error', onError)
  window.addEventListener('unhandledrejection', onRejection)
}

/** 记录 Vue 全局错误（app.config.errorHandler 回调入口），渲染/观察者异常落盘便于排查。 */
export function logVueError(error: unknown, info: string): void {
  appendFrontendLog(`vue.error info=${info} ${formatError(error)}`)
}
