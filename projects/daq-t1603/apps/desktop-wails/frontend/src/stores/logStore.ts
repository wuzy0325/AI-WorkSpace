import { defineStore } from 'pinia'
import { ref, computed } from 'vue'

/** 日志级别 */
export type LogLevel = 'debug' | 'info' | 'warn' | 'error'

/** 单条日志记录 */
export interface LogEntry {
  id: number
  level: LogLevel
  tag: string
  message: string
  timestamp: number
}

const MAX_LOG_ENTRIES = 500

let nextId = 0

/** 级别权重，用于过滤 */
const LEVEL_WEIGHT: Record<LogLevel, number> = {
  debug: 0,
  info: 1,
  warn: 2,
  error: 3,
}

export const useLogStore = defineStore('log', () => {
  const entries = ref<LogEntry[]>([])
  const minLevel = ref<LogLevel>('info')

  /** 根据最低级别过滤后的日志 */
  const filteredEntries = computed(() =>
    entries.value.filter((e) => LEVEL_WEIGHT[e.level] >= LEVEL_WEIGHT[minLevel.value])
  )

  /** 添加一条日志 */
  function log(level: LogLevel, tag: string, message: string): void {
    const entry: LogEntry = {
      id: nextId++,
      level,
      tag,
      message,
      timestamp: Date.now(),
    }
    entries.value.push(entry)
    // 超出上限时裁剪旧日志
    if (entries.value.length > MAX_LOG_ENTRIES) {
      entries.value.splice(0, entries.value.length - MAX_LOG_ENTRIES)
    }
    // 同时输出到浏览器控制台，方便开发调试
    const prefix = `[${tag}]`
    switch (level) {
      case 'debug': console.debug(prefix, message); break
      case 'info':  console.info(prefix, message);  break
      case 'warn':  console.warn(prefix, message);  break
      case 'error': console.error(prefix, message); break
    }
  }

  function debug(tag: string, message: string): void { log('debug', tag, message) }
  function info(tag: string, message: string): void  { log('info', tag, message) }
  function warn(tag: string, message: string): void  { log('warn', tag, message) }
  function error(tag: string, message: string): void { log('error', tag, message) }

  /** 清空所有日志 */
  function clear(): void {
    entries.value = []
  }

  /** 设置最低日志级别 */
  function setMinLevel(level: LogLevel): void {
    minLevel.value = level
  }

  return {
    entries, minLevel, filteredEntries,
    log, debug, info, warn, error,
    clear, setMinLevel,
  }
})
