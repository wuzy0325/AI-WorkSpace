import { useLogStore } from '@stores/logStore'
import { request } from './http-client'
import { isWailsAvailable } from './wails-adapter'
import type { LogLevel } from './types'

interface LogSseEntry {
  id: number
  timestamp: string
  level: string
  source: string
  message: string
  details?: string
}

export interface LogSseSubscription {
  unsubscribe: () => void
}

// Wails 桌面端必须走主进程本地 API；浏览器开发态保留相对路径以命中 Vite proxy。
// Dev 模式可能用 http://127.0.0.1:9245 加载前端，不能只依赖 location.protocol 判断。
const apiBase = import.meta.env.VITE_API_BASE || (isWailsAvailable() ? 'http://127.0.0.1:8900' : '')

const levelMap: Record<string, LogLevel> = {
  debug: 'debug',
  info: 'info',
  warn: 'warn',
  error: 'error',
}

// 记录已经警告过的未知 level，避免后端持续返回新值时把控制台刷爆。
// 限制集合最大 50 条，超过后静默拒绝，防止长期运行内存缓慢增长。
const warnedUnknownLevels = new Set<string>()
const MAX_WARNED_UNKNOWN = 50

function resolveLevel(raw: string): LogLevel {
  const mapped = levelMap[raw]
  if (mapped) return mapped
  if (!warnedUnknownLevels.has(raw) && warnedUnknownLevels.size < MAX_WARNED_UNKNOWN) {
    warnedUnknownLevels.add(raw)
    // 仅首次提示，便于排查后端新增级别未在前端映射的问题
    console.warn(`[logSseClient] 未知日志级别 "${raw}"，已回退为 info`)
  }
  return 'info'
}

let currentSubscription: LogSseSubscription | null = null

interface LogSseParseState {
  buffer: string
  event: string
  data: string
}

export function parseLogSseChunk(
  state: LogSseParseState,
  chunk: string,
  onEntry: (entry: LogSseEntry) => void,
): void {
  state.buffer += chunk
  const lines = state.buffer.split('\n')
  state.buffer = lines.pop() ?? ''

  for (const rawLine of lines) {
    const line = rawLine.endsWith('\r') ? rawLine.slice(0, -1) : rawLine
    // SSE 注释行（以 ':' 开头）仅用于 keep-alive，跳过不处理
    if (line.startsWith(':')) continue
    if (line.startsWith('event: ')) {
      state.event = line.slice(7).trim()
    } else if (line.startsWith('data: ')) {
      state.data = state.data ? `${state.data}\n${line.slice(6).trim()}` : line.slice(6).trim()
    } else if (line === '') {
      if (state.data && state.event === 'log') {
        onEntry(JSON.parse(state.data) as LogSseEntry)
      }
      state.event = ''
      state.data = ''
    }
  }
}

export function subscribeLogStream(): LogSseSubscription {
  let reader: ReadableStreamDefaultReader<Uint8Array> | null = null
  let aborted = false
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null
  let backoff = 500
  // attemptCount 记录连接尝试次数：0=首次，>0=重试。
  // 用显式计数而非 backoff>500 阈值判断，语义更清晰。
  let attemptCount = 0

  async function connect() {
    if (aborted) return
    const logStore = useLogStore()
    const reconnecting = attemptCount > 0
    logStore.setStreamStatus(reconnecting ? 'reconnecting' : 'connecting', reconnecting ? '日志流重连中' : '正在连接日志流')
    attemptCount++
    try {
      const response = await fetch(`${apiBase}/api/log/stream`)
      if (!response.ok) {
        logStore.setStreamStatus('error', `日志流连接失败: HTTP ${response.status}`)
        scheduleReconnect()
        return
      }
      backoff = 500

      const body = response.body
      if (!body) {
        logStore.setStreamStatus('error', '日志流响应为空')
        scheduleReconnect()
        return
      }

      reader = body.getReader()
      logStore.setStreamStatus('connected', '日志流已连接')
      const decoder = new TextDecoder()
      const parseState: LogSseParseState = { buffer: '', event: '', data: '' }

      while (!aborted) {
        const { done, value } = await reader.read()
        if (done) break

        try {
          parseLogSseChunk(parseState, decoder.decode(value, { stream: true }), (entry) => {
            logStore.pushEntry({
              id: `log-${entry.id}-${Date.now()}`,
              timestamp: entry.timestamp,
              level: resolveLevel(entry.level),
              source: entry.source,
              message: entry.message,
              details: entry.details,
            })
          })
        } catch {
          // 解析失败跳过当前事件，连接继续保持。
          parseState.event = ''
          parseState.data = ''
        }
      }
    } catch {
      // 连接断开，重连
      if (!aborted) logStore.setStreamStatus('error', '日志流连接已断开')
    }

    if (!aborted) {
      reader = null
      scheduleReconnect()
    }
  }

  function scheduleReconnect() {
    if (aborted) return
    // 指数退避，但最大不超过 5s，避免用户看到过长等待。
    backoff = Math.min(backoff * 1.5, 5000)
    const logStore = useLogStore()
    // 显示规则：<1s 用毫秒，≥1s 用秒（一位小数），避免 750ms 被取整显示为"1s"造成误导。
    const waitText = backoff < 1000
      ? `${Math.round(backoff)}ms`
      : `${(backoff / 1000).toFixed(1)}s`
    logStore.setStreamStatus('reconnecting', `将在 ${waitText} 后重连日志流`)
    reconnectTimer = setTimeout(() => { void connect() }, backoff)
  }

  void connect()

  return {
    unsubscribe: () => {
      aborted = true
      const logStore = useLogStore()
      logStore.setStreamStatus('idle', '日志流已停止')
      if (reconnectTimer !== null) clearTimeout(reconnectTimer)
      if (reader !== null) { void reader.cancel().catch(() => {}) }
    },
  }
}

export function startLogSubscription(): void {
  if (currentSubscription) return
  currentSubscription = subscribeLogStream()
}

export function stopLogSubscription(): void {
  if (currentSubscription) {
    currentSubscription.unsubscribe()
    currentSubscription = null
  }
}

export async function fetchRecentLogs(limit = 500): Promise<void> {
  const logStore = useLogStore()
  logStore.setRecentLoadStatus('loading', `正在读取最近 ${limit} 条日志`)
  try {
    const data = await request<{ entries: LogSseEntry[] }>(`/api/log/recent?limit=${limit}`)
    for (const entry of data.entries) {
      logStore.pushEntry({
        id: `recent-${entry.id}`,
        timestamp: entry.timestamp,
        level: resolveLevel(entry.level),
        source: entry.source,
        message: entry.message,
        details: entry.details,
      })
    }
    logStore.setRecentLoadStatus('loaded', `已读取 ${data.entries.length} 条历史日志`)
  } catch {
    // 拉取历史日志失败，静默处理
    logStore.setRecentLoadStatus('error', '历史日志读取失败，请检查后端服务')
  }
}
