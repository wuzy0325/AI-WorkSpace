import { useLogStore } from '@stores/logStore'
import { request } from './http-client'
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

const apiBase = import.meta.env.VITE_API_BASE ?? ''

const levelMap: Record<string, LogLevel> = {
  debug: 'debug',
  info: 'info',
  warn: 'warn',
  error: 'error',
}

// 记录已经警告过的未知 level，避免后端持续返回新值时把控制台刷爆
const warnedUnknownLevels = new Set<string>()

function resolveLevel(raw: string): LogLevel {
  const mapped = levelMap[raw]
  if (mapped) return mapped
  if (!warnedUnknownLevels.has(raw)) {
    warnedUnknownLevels.add(raw)
    // 仅首次提示，便于排查后端新增级别未在前端映射的问题
    console.warn(`[logSseClient] 未知日志级别 "${raw}"，已回退为 info`)
  }
  return 'info'
}

let currentSubscription: LogSseSubscription | null = null

export function subscribeLogStream(): LogSseSubscription {
  let reader: ReadableStreamDefaultReader<Uint8Array> | null = null
  let aborted = false
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null
  let backoff = 500

  async function connect() {
    if (aborted) return
    try {
      const response = await fetch(`${apiBase}/api/log/stream`)
      if (!response.ok) {
        scheduleReconnect()
        return
      }
      backoff = 500

      const body = response.body
      if (!body) {
        scheduleReconnect()
        return
      }

      reader = body.getReader()
      const decoder = new TextDecoder()
      let buffer = ''

      while (!aborted) {
        const { done, value } = await reader.read()
        if (done) break

        buffer += decoder.decode(value, { stream: true })
        const lines = buffer.split('\n')
        buffer = lines.pop() ?? ''

        let currentEvent = ''
        let currentData = ''

        for (const line of lines) {
          if (line.startsWith('event: ')) {
            currentEvent = line.slice(7).trim()
          } else if (line.startsWith('data: ')) {
            currentData = line.slice(6).trim()
          } else if (line === '' && currentData && currentEvent === 'log') {
            try {
              const entry = JSON.parse(currentData) as LogSseEntry
              const logStore = useLogStore()
              logStore.pushEntry({
                id: `log-${entry.id}-${Date.now()}`,
                timestamp: entry.timestamp,
                level: resolveLevel(entry.level),
                source: entry.source,
                message: entry.message,
                details: entry.details,
              })
            } catch {
              // 解析失败跳过
            }
            currentEvent = ''
            currentData = ''
          }
        }
      }
    } catch {
      // 连接断开，重连
    }

    if (!aborted) {
      reader = null
      scheduleReconnect()
    }
  }

  function scheduleReconnect() {
    if (aborted) return
    backoff = Math.min(backoff * 1.5, 10000)
    reconnectTimer = setTimeout(() => { void connect() }, backoff)
  }

  void connect()

  return {
    unsubscribe: () => {
      aborted = true
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
  try {
    const data = await request<{ entries: LogSseEntry[] }>(`/api/log/recent?limit=${limit}`)
    const logStore = useLogStore()
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
  } catch {
    // 拉取历史日志失败，静默处理
  }
}
