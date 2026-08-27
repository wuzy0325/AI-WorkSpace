import { defineStore } from 'pinia'
import { ref, computed } from 'vue'

export type HwLogKind = 'hw-cmd' | 'hw-res' | 'sys-error'

export interface HwLogEntry {
  id: number
  kind: HwLogKind
  timestamp: number
  model: string
  proto: string
  detail: string
  poll?: boolean
}

const MAX_LOG_ENTRIES = 200

/** 攒批刷新间隔：硬件命令/响应事件在轮询场景可达每秒多条，
 * 先落非响应式缓冲，定时一次性替换 entries，避免逐条触发订阅组件重渲染。 */
const FLUSH_INTERVAL_MS = 500

export const useHardwareLogStore = defineStore('hardwareLog', () => {
  const entries = ref<HwLogEntry[]>([])
  let nextId = 1
  let buffer: HwLogEntry[] = []
  let flushTimer: ReturnType<typeof setTimeout> | null = null

  function flush() {
    flushTimer = null
    if (buffer.length === 0) return
    const incoming = buffer
    buffer = []
    const merged = entries.value.concat(incoming)
    entries.value = merged.length > MAX_LOG_ENTRIES
      ? merged.slice(merged.length - MAX_LOG_ENTRIES)
      : merged
  }

  function addEntry(kind: HwLogKind, model: string, proto: string, detail: string, poll?: boolean) {
    buffer.push({
      id: nextId++,
      kind,
      timestamp: Date.now(),
      model,
      proto,
      detail,
      poll
    })
    if (!flushTimer) {
      flushTimer = setTimeout(flush, FLUSH_INTERVAL_MS)
    }
  }

  function clear() {
    buffer = []
    if (flushTimer) {
      clearTimeout(flushTimer)
      flushTimer = null
    }
    entries.value = []
  }

  const count = computed(() => entries.value.length)

  return { entries, addEntry, clear, count }
})
