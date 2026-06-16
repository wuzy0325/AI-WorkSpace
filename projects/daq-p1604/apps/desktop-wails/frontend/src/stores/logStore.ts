import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import type { DeviceLogEvent, LogCategory } from '@bridge/deviceBridge'
import * as logBridge from '@bridge/logBridge'

/** 日志级别 */
export type LogLevel = 'debug' | 'info' | 'warn' | 'error'

/**
 * 前端展示用的日志分组
 * 将后端的细粒度分类合并为用户友好的分组
 */
export type LogGroup = 'system' | 'communication' | 'acquisition'

/** 分组与后端原始分类的映射关系 */
const GROUP_CATEGORY_MAP: Record<LogGroup, LogCategory[]> = {
  system: ['system'],
  communication: ['hardware-send', 'hardware-recv'],
  acquisition: ['acquisition'],
}

/** 分组中文标签 */
export const LOG_GROUP_LABELS: Record<LogGroup, string> = {
  system: '系统',
  communication: '通信',
  acquisition: '采集',
}

/** 根据后端原始分类映射到前端分组 */
export function mapCategoryToGroup(category: LogCategory): LogGroup {
  for (const [group, categories] of Object.entries(GROUP_CATEGORY_MAP) as [LogGroup, LogCategory[]][]) {
    if (categories.includes(category)) return group
  }
  return 'system'
}

/** 后端原始分类的中文标签（用于日志条目内显示） */
export const CATEGORY_LABELS: Record<LogCategory, string> = {
  system: '系统',
  'hardware-send': '发送',
  'hardware-recv': '接收',
  acquisition: '采集',
}

/** 单条日志记录 */
export interface LogEntry {
  id: number
  level: LogLevel
  category: LogCategory
  group: LogGroup
  source: string
  deviceId?: string
  tag: string
  message: string
  detail?: string
  timestamp: number
}

/** 日志条目上限 */
const MAX_LOG_ENTRIES = 500

/**
 * 采集/通信类 debug 日志的节流间隔（毫秒）
 * 同一设备同一分类的 debug 日志，在此间隔内只保留第一条，
 * 后续的合并为一条摘要（"… 及 N 条同类日志"）
 */
const DEBUG_THROTTLE_MS = 2000

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
  const group = ref<LogGroup | 'all'>('all')

  /** 日志文件保存状态 */
  const fileSaving = ref(false)
  const fileOutputDir = ref('')

  /** 节流状态：key = "deviceId:category:level"，value = { lastId, suppressed } */
  const throttleMap = new Map<string, { lastId: number; suppressed: number; timer: ReturnType<typeof setTimeout> | null }>()

  /** 根据最低级别和分组过滤后的日志 */
  const filteredEntries = computed(() =>
    entries.value.filter((e) => {
      if (LEVEL_WEIGHT[e.level] < LEVEL_WEIGHT[minLevel.value]) {
        return false
      }
      if (group.value !== 'all' && e.group !== group.value) {
        return false
      }
      return true
    })
  )

  /** 添加一条日志（含节流逻辑） */
  function log(
    level: LogLevel,
    tag: string,
    message: string,
    options?: Partial<Omit<LogEntry, 'id' | 'level' | 'tag' | 'message'>>,
  ): void {
    const category = options?.category ?? 'system'
    const mappedGroup = mapCategoryToGroup(category)

    // 对通信/采集类的 debug 日志进行节流，避免高频日志拖慢前端
    if (level === 'debug' && (mappedGroup === 'communication' || mappedGroup === 'acquisition')) {
      const deviceId = options?.deviceId ?? ''
      const throttleKey = `${deviceId}:${category}:${level}`

      const existing = throttleMap.get(throttleKey)
      if (existing !== undefined) {
        // 已有同类日志在节流窗口内，抑制此条
        existing.suppressed++
        return
      }

      // 首条：正常写入，并设置节流窗口
      const entryId = nextId
      throttleMap.set(throttleKey, { lastId: entryId, suppressed: 0, timer: null })

      // 节流窗口结束后，如果存在被抑制的日志，追加一条摘要
      const timer = setTimeout(() => {
        const state = throttleMap.get(throttleKey)
        if (state && state.suppressed > 0) {
          appendEntry({
            id: nextId++,
            level: 'debug',
            category,
            group: mappedGroup,
            source: options?.source ?? tag,
            deviceId: options?.deviceId,
            tag,
            message: `… 及 ${state.suppressed} 条同类日志`,
            timestamp: Date.now(),
          })
        }
        throttleMap.delete(throttleKey)
      }, DEBUG_THROTTLE_MS)

      // 记录 timer 以便清理
      const state = throttleMap.get(throttleKey)
      if (state) state.timer = timer
    }

    // 正常写入日志条目
    appendEntry({
      id: nextId++,
      level,
      category,
      group: mappedGroup,
      source: options?.source ?? tag,
      deviceId: options?.deviceId,
      tag,
      message,
      detail: options?.detail,
      timestamp: options?.timestamp ?? Date.now(),
    })
  }

  /** 写入条目到数组，超出上限时裁剪 */
  function appendEntry(entry: LogEntry): void {
    entries.value.push(entry)
    // 超出上限时裁剪旧日志
    if (entries.value.length > MAX_LOG_ENTRIES) {
      entries.value.splice(0, entries.value.length - MAX_LOG_ENTRIES)
    }
    // 同时输出到浏览器控制台，方便开发调试
    const prefix = `[${entry.category}] [${entry.tag}]`
    switch (entry.level) {
      case 'debug': console.debug(prefix, entry.message); break
      case 'info':  console.info(prefix, entry.message);  break
      case 'warn':  console.warn(prefix, entry.message);  break
      case 'error': console.error(prefix, entry.message); break
    }
  }

  function pushEvent(entry: DeviceLogEvent): void {
    log(entry.level, entry.source, entry.message, {
      category: entry.category,
      source: entry.source,
      deviceId: entry.deviceId,
      detail: entry.detail,
      timestamp: entry.timestamp,
    })
  }

  function debug(tag: string, message: string): void { log('debug', tag, message) }
  function info(tag: string, message: string): void  { log('info', tag, message) }
  function warn(tag: string, message: string): void  { log('warn', tag, message) }
  function error(tag: string, message: string): void { log('error', tag, message) }

  /** 清空所有日志并清理定时器 */
  function clear(): void {
    entries.value = []
    // 清理所有节流定时器
    for (const state of throttleMap.values()) {
      if (state.timer !== null) clearTimeout(state.timer)
    }
    throttleMap.clear()
  }

  /** 销毁 store 时清理所有资源 */
  function dispose(): void {
    clear()
  }

  /** 设置最低日志级别 */
  function setMinLevel(level: LogLevel): void {
    minLevel.value = level
  }

  function setGroup(nextGroup: LogGroup | 'all'): void {
    group.value = nextGroup
  }

  /** 开启日志文件保存 */
  async function startFileSaving(outputDir: string): Promise<void> {
    await logBridge.startLogFile(outputDir, 'daq-log')
    fileSaving.value = true
    fileOutputDir.value = outputDir
  }

  /** 停止日志文件保存 */
  async function stopFileSaving(): Promise<void> {
    await logBridge.stopLogFile()
    fileSaving.value = false
  }

  /** 选择日志保存目录 */
  async function pickLogDir(): Promise<string | null> {
    try {
      const dir = await logBridge.pickDirectory()
      return dir || null
    } catch {
      return null
    }
  }

  /** 从后端同步日志文件状态 */
  async function refreshFileState(): Promise<void> {
    try {
      const state = await logBridge.getLogFileState()
      fileSaving.value = state.active
      if (state.outputDir) fileOutputDir.value = state.outputDir
    } catch {
      // 后端未就绪时静默忽略
    }
  }

  return {
    entries, minLevel, group, filteredEntries,
    fileSaving, fileOutputDir,
    log, pushEvent, debug, info, warn, error,
    clear, dispose, setMinLevel, setGroup,
    startFileSaving, stopFileSaving, pickLogDir, refreshFileState,
  }
})
