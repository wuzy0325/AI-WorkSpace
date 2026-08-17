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

// 级别权重：仅用于内部判定，不再作为单一 minLevel 阈值过滤。
// 改为三个独立开关（showInfo / showDebug / showHardware）分别控制可见性，
// 这样用户可以独立打开"硬件通信"而不被普通 info 刷屏。
const LEVEL_WEIGHT: Record<LogLevel, number> = {
  debug: 0,
  info: 1,
  warn: 2,
  error: 3,
}

// localStorage 持久化键：用户对开关的偏好跨会话保留，
// 避免每次进入 LOG 画面都要重新切换。
const STORAGE_KEY = 'WindLabX4.log-viewer.prefs.v1'

interface LogViewerPrefs {
  showHardware: boolean
  showInfo: boolean
  showDebug: boolean
  filterGroup: LogGroup | 'all'
}

const DEFAULT_PREFS: LogViewerPrefs = {
  // 硬件通信默认开：用户需要看到发送/接收帧用于排查设备问题
  showHardware: true,
  // 普通 info 默认关：避免系统启动、配置加载等常态事件刷屏
  showInfo: false,
  // 普通 debug 默认关：仅排查问题时手动打开
  showDebug: false,
  filterGroup: 'all',
}

function loadPrefs(): LogViewerPrefs {
  if (typeof localStorage === 'undefined') return { ...DEFAULT_PREFS }
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return { ...DEFAULT_PREFS }
    const parsed = JSON.parse(raw) as Partial<LogViewerPrefs>
    return {
      showHardware: parsed.showHardware ?? DEFAULT_PREFS.showHardware,
      showInfo: parsed.showInfo ?? DEFAULT_PREFS.showInfo,
      showDebug: parsed.showDebug ?? DEFAULT_PREFS.showDebug,
      filterGroup: parsed.filterGroup ?? DEFAULT_PREFS.filterGroup,
    }
  } catch {
    return { ...DEFAULT_PREFS }
  }
}

function savePrefs(prefs: LogViewerPrefs): void {
  if (typeof localStorage === 'undefined') return
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(prefs))
  } catch {
    // localStorage 满或被禁用时静默降级，不影响功能
  }
}

export const useLogStore = defineStore('log', () => {
  const entries = ref<LogEntry[]>([])
  const initialPrefs = loadPrefs()
  // 三个独立可见性开关，默认值由 STORAGE_KEY 控制（首次进入即默认值）。
  // 设计原因：单一 minLevel 无法表达"显示硬件通信但隐藏普通 info"这种组合需求。
  const showHardware = ref(initialPrefs.showHardware)
  const showInfo = ref(initialPrefs.showInfo)
  const showDebug = ref(initialPrefs.showDebug)
  const filterGroup = ref<LogGroup | 'all'>(initialPrefs.filterGroup)
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

  // ── 去重：记录已入库的后端条目 ID，防止 fetchRecentLogs 与 SSE Recent(200) 重复推送 ──
  // 场景：onMounted 并行调用 fetchRecentLogs(500) + startLogSubscription()，
  //       SSE stream 服务端会再发一次 ring.Recent(200)，导致同一条日志被 push 两次，
  //       且 SSE 因网络延迟后到达，造成时间乱序（新日志出现在旧日志前面）。
  const seenBackendIds = new Set<number>()
  let maxSeenId = 0

  // 持久化偏好：开关或分组变化时写回 localStorage
  function persistPrefs(): void {
    savePrefs({
      showHardware: showHardware.value,
      showInfo: showInfo.value,
      showDebug: showDebug.value,
      filterGroup: filterGroup.value,
    })
  }

  // 可见性判定核心规则：
  //   1. warn / error 永远显示（异常必须可见，避免漏排查）。
  //      这条优先于一切，包括硬件分类 —— 即使用户关掉「硬件通信」开关，
  //      硬件层的 Warn/Error（如 SetUnit 写入失败、单位系数不匹配）仍要露出。
  //   2. hardware-send / hardware-recv 的 debug/info：仅由 showHardware 控制可见性，
  //      不受 showInfo / showDebug 影响。这样用户可以在普通 info 关闭的情况下
  //      仍看到硬件收发帧（这对排查设备问题是必需的）。
  //   3. 其他类别的 info：仅当 showInfo=true 显示
  //   4. 其他类别的 debug：仅当 showDebug=true 显示
  function isEntryVisible(entry: LogEntry): boolean {
    const weight = LEVEL_WEIGHT[entry.level]
    if (weight >= LEVEL_WEIGHT.warn) return true
    if (entry.category === 'hardware-send' || entry.category === 'hardware-recv') {
      return showHardware.value
    }
    if (entry.level === 'info') return showInfo.value
    if (entry.level === 'debug') return showDebug.value
    return false
  }

  const filteredEntries = computed(() => {
    let result = entries.value
    // 级别 + 硬件开关过滤
    result = result.filter(isEntryVisible)
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
      source: 'WindLabX4',
      message: 'WindLabX4 UI initialized',
    })
  }

  function destroy(): void {
    initialized = false
  }

  function toggleHardware(): void {
    showHardware.value = !showHardware.value
    persistPrefs()
  }

  function toggleInfo(): void {
    showInfo.value = !showInfo.value
    persistPrefs()
  }

  function toggleDebug(): void {
    showDebug.value = !showDebug.value
    persistPrefs()
  }

  function setFilterGroup(group: LogGroup | 'all'): void {
    filterGroup.value = group
    persistPrefs()
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
    seenBackendIds.clear()
    maxSeenId = 0
  }

  // 从前端 entry.id 中提取后端数字 ID。
  // id 格式：recent-{backendId} 或 log-{backendId}-{Date.now()} 或 init-1（返回 0）
  function extractBackendId(id: string): number {
    const m = id.match(/^(?:recent|log)-(\d+)/)
    return m ? parseInt(m[1], 10) : 0
  }

  function pushEntry(entry: LogEntry): void {
    // ── 后端 ID 去重：防止 fetchRecentLogs 与 SSE Recent(200) 重复推送 ──
    const backendId = extractBackendId(entry.id)
    if (backendId > 0) {
      if (seenBackendIds.has(backendId)) return // 已存在，跳过重复
      seenBackendIds.add(backendId)
      if (backendId > maxSeenId) maxSeenId = backendId
    }

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

  // 更新后端日志分类开关状态（从 API 返回的快照）
  function updateCategoryStates(states: Record<string, boolean>): void {
    categoryEnabled.value = { ...states }
  }

  // 设置单个分类的启用状态（本地乐观更新 + 后端同步）
  function setCategoryEnabledState(category: string, enabled: boolean): void {
    categoryEnabled.value = { ...categoryEnabled.value, [category]: enabled }
  }

  return {
    entries,
    showHardware,
    showInfo,
    showDebug,
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
    toggleHardware,
    toggleInfo,
    toggleDebug,
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
