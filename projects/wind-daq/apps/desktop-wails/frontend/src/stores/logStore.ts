import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import type { LogEntry, LogLevel } from '@api/types'

const MAX_ENTRIES = 2000

export const useLogStore = defineStore('log', () => {
  const entries = ref<LogEntry[]>([])
  const filterLevel = ref<LogLevel | null>(null)
  const filterSearch = ref('')
  const isPaused = ref(false)
  const buffer = ref<LogEntry[]>([])

  const filteredEntries = computed(() => {
    let result = entries.value
    if (filterLevel.value) {
      result = result.filter((e) => e.level === filterLevel.value)
    }
    if (filterSearch.value) {
      const q = filterSearch.value.toLowerCase()
      result = result.filter(
        (e) => e.message.toLowerCase().includes(q) || e.source.toLowerCase().includes(q)
      )
    }
    return result
  })

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
    filteredEntries,
    init,
    destroy,
    setFilterLevel,
    setFilterSearch,
    togglePause,
    clear,
    pushEntry,
  }
})
