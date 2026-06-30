import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import type { LogEntry, LogLevel } from '@api/types'

const MAX_ENTRIES = 2000
type LogStreamStatus = 'idle' | 'connecting' | 'connected' | 'reconnecting' | 'error'
type RecentLoadStatus = 'idle' | 'loading' | 'loaded' | 'error'

export const useLogStore = defineStore('log', () => {
  const entries = ref<LogEntry[]>([])
  const filterLevel = ref<LogLevel | null>(null)
  const filterSearch = ref('')
  const isPaused = ref(false)
  const buffer = ref<LogEntry[]>([])
  const streamStatus = ref<LogStreamStatus>('idle')
  const streamMessage = ref('日志流尚未连接')
  const recentLoadStatus = ref<RecentLoadStatus>('idle')
  const recentLoadMessage = ref('')
  const lastReceivedAt = ref<string | null>(null)

  const filteredEntries = computed(() => {
    let result = entries.value
    if (filterLevel.value) {
      result = result.filter((e) => e.level === filterLevel.value)
    }
    if (filterSearch.value) {
      const q = filterSearch.value.toLowerCase()
      result = result.filter(
        (e) =>
          e.message.toLowerCase().includes(q) ||
          e.source.toLowerCase().includes(q) ||
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
      source: 'wind-daq',
      message: 'Wind-DAQ UI initialized',
    })
  }

  function destroy(): void {
    initialized = false
  }

  function setFilterLevel(level: LogLevel | null): void {
    filterLevel.value = level
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

  function pushEntry(entry: LogEntry): void {
    lastReceivedAt.value = entry.timestamp
    if (isPaused.value) {
      buffer.value.push(entry)
      if (buffer.value.length > MAX_ENTRIES) {
        buffer.value.shift()
      }
    } else {
      entries.value.push(entry)
      if (entries.value.length > MAX_ENTRIES) {
        entries.value.shift()
      }
    }
  }

  return {
    entries,
    filterLevel,
    filterSearch,
    isPaused,
    streamStatus,
    streamMessage,
    recentLoadStatus,
    recentLoadMessage,
    lastReceivedAt,
    filteredEntries,
    bufferCount,
    init,
    destroy,
    setFilterLevel,
    setFilterSearch,
    setStreamStatus,
    setRecentLoadStatus,
    togglePause,
    clear,
    pushEntry,
  }
})
