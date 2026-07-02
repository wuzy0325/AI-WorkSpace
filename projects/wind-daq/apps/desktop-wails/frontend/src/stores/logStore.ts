import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import type { LogCategory, LogEntry, LogLevel } from '@api/types'

const MAX_ENTRIES = 2000
type LogStreamStatus = 'idle' | 'connecting' | 'connected' | 'reconnecting' | 'error'
type RecentLoadStatus = 'idle' | 'loading' | 'loaded' | 'error'
export type LogGroup = 'system' | 'communication' | 'acquisition' | 'business'

export const LOG_GROUP_LABELS: Record<LogGroup, string> = {
  system: '系统',
  communication: '通信',
  acquisition: '采集',
  business: '业务',
}

export const CATEGORY_LABELS: Record<LogCategory, string> = {
  system: '系统',
  'hardware-send': '发送',
  'hardware-recv': '接收',
  acquisition: '采集',
  business: '业务',
}

export function mapCategoryToGroup(category?: LogCategory): LogGroup {
  if (category === 'hardware-send' || category === 'hardware-recv') return 'communication'
  if (category === 'acquisition') return 'acquisition'
  if (category === 'business') return 'business'
  return 'system'
}

// inferCategory 仅做最小兜底：后端 RingHandler 已对每条日志显式推断 category，
// 前端只在后端漏掉时按 message 中的显式关键字回退。
// 不再使用 'tx'/'rx' 子串匹配，避免误命中 "Next"/"context" 等含子串的单词。
function inferCategory(entry: LogEntry): LogCategory {
  if (entry.category) return entry.category
  const text = entry.message ?? ''
  if (text.includes('send') || text.includes('发送')) return 'hardware-send'
  if (text.includes('response') || text.includes('recv') || text.includes('接收') || text.includes('TCP connected') || text.includes('TCP disconnected')) return 'hardware-recv'
  if (text.includes('acquisition') || text.includes('采集')) return 'acquisition'
  if (text.includes('calibration') || text.includes('traversal') || text.includes('storage') || text.includes('motion')) return 'business'
  return 'system'
}

// 级别权重：用于 minLevel 过滤（显示该级别及更高严重度的日志）。
// 默认 minLevel='info'，隐藏 Debug 级别的高频命令收发日志，避免采集期间刷屏。
// 需要排查通信细节时手动切到 Debug。
const LEVEL_WEIGHT: Record<LogLevel, number> = {
  debug: 0,
  info: 1,
  warn: 2,
  error: 3,
}

export const useLogStore = defineStore('log', () => {
  const entries = ref<LogEntry[]>([])
  // minLevel 语义：显示该级别及更高严重度的日志（对齐 daq-t1603）。
  // 默认 'info' 隐藏 Debug，避免高频 hardware-send/hardware-recv 命令日志刷屏。
  const minLevel = ref<LogLevel>('info')
  const filterGroup = ref<LogGroup | 'all'>('all')
  const filterSearch = ref('')
  const isPaused = ref(false)
  const buffer = ref<LogEntry[]>([])
  const streamStatus = ref<LogStreamStatus>('idle')
  const streamMessage = ref('日志流尚未连接')
  const recentLoadStatus = ref<RecentLoadStatus>('idle')
  const recentLoadMessage = ref('')
  const lastReceivedAt = ref<string | null>(null)
  // 后端日志分类开关状态：key=category, value=是否启用（默认全部 true）
  const categoryEnabled = ref<Record<string, boolean>>({})

  const filteredEntries = computed(() => {
    let result = entries.value
    // minLevel 过滤：只保留级别权重 >= 当前 minLevel 的日志
    const minWeight = LEVEL_WEIGHT[minLevel.value]
    result = result.filter((e) => LEVEL_WEIGHT[e.level] >= minWeight)
    if (filterGroup.value !== 'all') {
      // entry.category 在 pushEntry 入库时已统一填充，这里直接 mapCategoryToGroup 即可，
      // 避免 2000 条日志 × 每次过滤触发 inferCategory 字符串拼接。
      result = result.filter((e) => mapCategoryToGroup(e.category) === filterGroup.value)
    }
    if (filterSearch.value) {
      const q = filterSearch.value.toLowerCase()
      result = result.filter(
        (e) =>
          e.message.toLowerCase().includes(q) ||
          e.source.toLowerCase().includes(q) ||
          (e.deviceId?.toLowerCase().includes(q) ?? false) ||
          (e.category?.toLowerCase().includes(q) ?? false) ||
          (e.details?.toLowerCase().includes(q) ?? false)
      )
    }
    return result
  })

  const bufferCount = computed(() => buffer.value.length)

  let initialized = false

  function init(): void {
    if (initialized) return
    initialized = true
    entries.value.push({
      id: 'init-1',
      timestamp: new Date().toISOString(),
      level: 'info',
      category: 'system',
      source: 'wind-daq',
      message: 'Wind-DAQ UI initialized',
    })
  }

  function destroy(): void {
    initialized = false
  }

  function setMinLevel(level: LogLevel): void {
    minLevel.value = level
  }

  function setFilterGroup(group: LogGroup | 'all'): void {
    filterGroup.value = group
  }

  function setFilterSearch(text: string): void {
    filterSearch.value = text
  }

  function setStreamStatus(status: LogStreamStatus, message = ''): void {
    streamStatus.value = status
    streamMessage.value = message
  }

  function setRecentLoadStatus(status: RecentLoadStatus, message = ''): void {
    recentLoadStatus.value = status
    recentLoadMessage.value = message
  }

  function togglePause(): void {
    isPaused.value = !isPaused.value
    if (!isPaused.value && buffer.value.length > 0) {
      entries.value.push(...buffer.value)
      if (entries.value.length > MAX_ENTRIES) {
        entries.value = entries.value.slice(-MAX_ENTRIES)
      }
      buffer.value = []
    }
  }

  function clear(): void {
    entries.value = []
    buffer.value = []
  }

  // 更新后端日志分类开关状态（从 API 返回的快照）
  function updateCategoryStates(states: Record<string, boolean>): void {
    categoryEnabled.value = { ...states }
  }

  // 设置单个分类的启用状态（本地乐观更新 + 后端同步）
  function setCategoryEnabledState(category: string, enabled: boolean): void {
    categoryEnabled.value = { ...categoryEnabled.value, [category]: enabled }
  }

  function pushEntry(entry: LogEntry): void {
    // 入库前确保 category 已填充，避免 filteredEntries 在每次重算时重复执行 inferCategory。
    // 复制 entry 而非原地修改，避免污染调用方持有的引用。
    const normalized: LogEntry = entry.category ? entry : { ...entry, category: inferCategory(entry) }
    lastReceivedAt.value = normalized.timestamp
    if (isPaused.value) {
      buffer.value.push(normalized)
      if (buffer.value.length > MAX_ENTRIES) {
        buffer.value.shift()
      }
    } else {
      entries.value.push(normalized)
      if (entries.value.length > MAX_ENTRIES) {
        entries.value.shift()
      }
    }
  }

  return {
    entries,
    minLevel,
    filterGroup,
    filterSearch,
    isPaused,
    streamStatus,
    streamMessage,
    recentLoadStatus,
    recentLoadMessage,
    lastReceivedAt,
    categoryEnabled,
    filteredEntries,
    bufferCount,
    init,
    destroy,
    setMinLevel,
    setFilterGroup,
    setFilterSearch,
    setStreamStatus,
    setRecentLoadStatus,
    updateCategoryStates,
    setCategoryEnabledState,
    togglePause,
    clear,
    pushEntry,
  }
})
