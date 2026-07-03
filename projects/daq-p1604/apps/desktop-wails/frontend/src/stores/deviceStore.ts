import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import * as bridge from '@bridge/deviceBridge'
import type { PressureProfile, PressureSnapshot, P1604Config, ChannelConfig, ScanResult, DeviceState } from '@bridge/deviceBridge'
import { useLogStore } from '@stores/logStore'
import {
  computeExistingKeys,
  planScannedAdditions,
  type ScannedDeviceInput,
  type PlannedProfile,
  type SkippedScanEntry,
  type AddScannedResult,
} from './deviceStoreHelpers'

const MAX_HISTORY_HARD_CAP = 4000
const ACQUISITION_ACTION_TIMEOUT_MS = 8000
const APPLY_CONFIG_TIMEOUT_MS = 15000
/** UI 渲染刷新率默认值（Hz），仅在 store 初始化时使用，运行时由 displayStore 驱动 */
const RENDER_TICK_FALLBACK_HZ = 10
/** 图表历史时间窗口默认值（秒），运行时由 displayStore 驱动 */
const HISTORY_WINDOW_FALLBACK_SEC = 30
/**
 * 快照轮询固定周期（毫秒）。
 * 由后端采样率决定，与前端 UI 刷新率解耦：
 * - 后端 P1604 默认采样 100ms（10Hz），轮询 100ms 已充分覆盖
 * - 更高频轮询没有新数据可拉，只会浪费 IPC
 * - Wails v3 采用轮询是为规避 Event.Emit 触发 WebView2 同步阻塞
 */
const SNAPSHOT_POLL_INTERVAL_MS = 100
/** 定时器周期下限（约 60Hz），保护 WebView2 GUI 线程 */
const MIN_TIMER_INTERVAL_MS = 16

// 18 通道默认颜色
const CHANNEL_COLORS = [
  '#3b82f6', '#10b981', '#f59e0b', '#a855f7',
  '#f43f5e', '#06b6d4', '#f97316', '#6366f1',
  '#84cc16', '#14b8a6', '#d946ef', '#0ea5e9',
  '#eab308', '#22c55e', '#ef4444', '#8b5cf6',
  '#64748b', '#78716c',
]

function defaultChannels() {
  return Array.from({ length: 18 }, (_, i) => ({
    index: i,
    name: i < 16 ? `通道 ${i + 1}` : (i === 16 ? '大气压力' : '大气温度'),
    enabled: true,
    unit: i < 16 ? 'psi' : (i === 16 ? 'Pa' : '°C'),
    color: CHANNEL_COLORS[i % CHANNEL_COLORS.length],
    precision: i < 16 ? 3 : (i === 16 ? 0 : 2),
    rangeMin: 0,
    rangeMax: i < 16 ? 200 : (i === 16 ? 200000 : 100),
  }))
}

function defaultP1604Config(): P1604Config {
  return {
    samplingRate: 100,
    unit: 'psi',
    autoConnect: false,
    precision: 3,
  }
}

function p1604Defaults(cfg: Partial<P1604Config>): P1604Config {
  return {
    samplingRate: cfg.samplingRate ?? 100,
    unit: cfg.unit ?? 'psi',
    autoConnect: cfg.autoConnect ?? false,
    precision: cfg.precision ?? 3,
    // 透传三态：undefined=默认开启（兼容老 profile），true/false=用户显式设置
    // 必须透传，否则 ApplyConfig 会把后端 driver.profile.UseDeviceTimestamp 覆盖为 nil，
    // 导致下次 StartAcquisition 按默认值（true）启用硬件时间戳，与用户关闭意图相反
    useDeviceTimestamp: cfg.useDeviceTimestamp,
  }
}

export const useDeviceStore = defineStore('device', () => {
  const profiles = ref<PressureProfile[]>([])
  const selectedId = ref<string | null>(null)
  const statusMap = ref<Record<string, string>>({})
  const errorMap = ref<Record<string, string>>({})
  const historyMap = ref<Record<string, PressureSnapshot[]>>({})
  // 后端最新快照缓冲：由固定周期轮询写入（10Hz），供内部计算与录制使用
  const snapshotMap = ref<Record<string, PressureSnapshot>>({})
  // 已渲染快照：由 renderTick 按用户选择的 UI 刷新率发布，UI 组件（数值卡）读取此源
  // 与 snapshotMap 的区别：snapshotMap 高频更新用于内部逻辑，renderedSnapshotMap 被节流用于视觉呈现
  const renderedSnapshotMap = ref<Record<string, PressureSnapshot>>({})
  // 待渲染快照缓冲：轮询命中新时间戳时写入，UI 渲染节拍取出后写入 historyMap + renderedSnapshotMap
  // 语义：每个设备只保留"最新一份"未渲染快照，避免堆积
  const pendingSnapshotMap = ref<Record<string, PressureSnapshot>>({})
  const chartSelections = ref<Record<string, Set<number>>>({})
  const scanResults = ref<ScanResult[]>([])
  const isScanning = ref(false)
  // UI 渲染节拍定时器：按 displayStore.refreshRateHz 触发 pending → history
  const renderTickTimer = ref<ReturnType<typeof setInterval> | null>(null)
  // 快照轮询定时器：以固定周期从后端拉最新快照，与 UI 刷新率无关
  const snapshotPollTimer = ref<ReturnType<typeof setInterval> | null>(null)
  // 轮询并发门闩：上一次 IPC 未完成时跳过本次触发，防止 Promise 堆积
  const isPolling = ref(false)
  // 各设备最近一次写入快照的时间戳，用于跳过未变化的快照
  const lastSnapshotTs = ref<Record<string, number>>({})
  // 运行时可动态更新的 UI 刷新率与历史窗口，由外部（App.vue / MainTopBar）注入
  const renderRateHz = ref(RENDER_TICK_FALLBACK_HZ)
  const historyWindowSec = ref(HISTORY_WINDOW_FALLBACK_SEC)

  const selectedProfile = computed(() =>
    profiles.value.find((p) => p.id === selectedId.value) ?? null
  )
  const selectedSnapshot = computed(() =>
    selectedId.value ? renderedSnapshotMap.value[selectedId.value] ?? null : null
  )

  function syncSelectedDevice(): void {
    if (profiles.value.length === 0) {
      selectedId.value = null
      return
    }

    if (selectedId.value && profiles.value.some((p) => p.id === selectedId.value)) {
      return
    }

    selectedId.value = profiles.value[0]!.id
  }

  function selectDevice(id: string): void {
    selectedId.value = id
  }

  function statusFor(id: string): string {
    return statusMap.value[id] ?? 'Disconnected'
  }

  function errorFor(id: string): string {
    return errorMap.value[id] ?? ''
  }

  function acquiringFor(id: string): boolean {
    return statusMap.value[id] === 'Acquiring'
  }

  async function withTimeout<T>(promise: Promise<T>, timeoutMs: number, message: string): Promise<T> {
    let timer: ReturnType<typeof setTimeout> | null = null
    try {
      return await Promise.race([
        promise,
        new Promise<T>((_, reject) => {
          timer = setTimeout(() => reject(new Error(message)), timeoutMs)
        }),
      ])
    } finally {
      if (timer !== null) clearTimeout(timer)
    }
  }

  function historyFor(id: string): PressureSnapshot[] {
    return historyMap.value[id] ?? []
  }

  function isChartSelected(id: string, channelIndex: number): boolean {
    return chartSelections.value[id]?.has(channelIndex) ?? false
  }

  function toggleChartSelection(id: string, channelIndex: number): void {
    if (!chartSelections.value[id]) {
      chartSelections.value[id] = new Set()
    }
    const set = chartSelections.value[id]!
    if (set.has(channelIndex)) {
      set.delete(channelIndex)
    } else {
      set.add(channelIndex)
    }
  }

  function pushSnapshot(snapshot: PressureSnapshot): void {
    snapshotMap.value[snapshot.deviceId] = snapshot
    pendingSnapshotMap.value[snapshot.deviceId] = snapshot
  }

  /**
   * 轮询后端获取所有设备最新快照（替代 daq:payload 事件订阅）。
   *
   * 设计要点：
   * - 加 isPolling 门闩：上一次未完成前不重复发起 IPC，避免 Promise 堆积
   * - 快照时间戳未变化的设备只更新 snapshotMap（内部缓存），
   *   不入 pendingSnapshotMap（避免图表重复采样同一时间点造成平台化）
   *   同时不发布到 renderedSnapshotMap（UI 由 renderTick 节流发布，与轮询解耦）
   * - 静默失败：IPC 异常时下次定时器继续重试，不打断循环
   */
  async function pollLatestSnapshots(): Promise<void> {
    if (isPolling.value) return
    isPolling.value = true
    try {
      let snapshots: Record<string, PressureSnapshot>
      try {
        snapshots = await bridge.getLatestSnapshots()
      } catch {
        return
      }
      if (!snapshots) return
      for (const [deviceId, snapshot] of Object.entries(snapshots)) {
        if (!snapshot || !Array.isArray(snapshot.values)) continue
        const lastTs = lastSnapshotTs.value[deviceId] ?? 0
        if (snapshot.timestamp <= lastTs) {
          // 时间戳未变化：仅刷新内部 snapshotMap，不动 pending / rendered
          snapshotMap.value[deviceId] = snapshot
          continue
        }
        lastSnapshotTs.value[deviceId] = snapshot.timestamp
        pushSnapshot(snapshot)
      }
    } finally {
      isPolling.value = false
    }
  }

  /**
   * 启动快照轮询定时器。
   * 周期由后端采样率决定（默认 100ms），与用户在 UI 中选择的刷新率无关。
   */
  function startSnapshotPolling(): void {
    if (snapshotPollTimer.value !== null) return
    snapshotPollTimer.value = setInterval(() => {
      void pollLatestSnapshots()
    }, SNAPSHOT_POLL_INTERVAL_MS)
  }

  /** 停止快照轮询定时器 */
  function stopSnapshotPolling(): void {
    if (snapshotPollTimer.value !== null) {
      clearInterval(snapshotPollTimer.value)
      snapshotPollTimer.value = null
    }
  }

  /**
   * 计算当前历史容量上限（点数）。
   * 容量 = 时间窗口(秒) × 渲染率(Hz)，即在当前刷新率下能画满窗口需要多少点。
   * 有硬上限 MAX_HISTORY_HARD_CAP 防御异常配置导致内存爆炸。
   */
  function computeHistoryCapacity(): number {
    const cap = Math.round(historyWindowSec.value * renderRateHz.value)
    return Math.min(Math.max(cap, 10), MAX_HISTORY_HARD_CAP)
  }

  /**
   * UI 渲染节拍：将每台设备最新的 pending 快照追加到 historyMap 并发布到 renderedSnapshotMap，
   * 并按"时间窗口 × 刷新率"的容量裁剪历史。
   *
   * 每台设备每个 tick 最多追加 1 个点 —— 这就是"节流"：
   * 后端产生更快也无所谓，UI 只按用户选择的刷新率消费。
   * renderedSnapshotMap 与 historyMap 同源同频，保证图表与数值卡节奏一致。
   */
  function renderTick(): void {
    const pendingEntries = Object.entries(pendingSnapshotMap.value)
    if (pendingEntries.length === 0) return

    const capacity = computeHistoryCapacity()
    for (const [deviceId, snapshot] of pendingEntries) {
      if (!historyMap.value[deviceId]) {
        historyMap.value[deviceId] = []
      }
      const history = historyMap.value[deviceId]!
      history.push(snapshot)
      if (history.length > capacity) {
        history.splice(0, history.length - capacity)
      }
      // 同步发布"已渲染快照"，让数值卡等 UI 组件与图表同频
      renderedSnapshotMap.value[deviceId] = snapshot
    }
    pendingSnapshotMap.value = {}
  }

  /**
   * 应用 UI 显示偏好（渲染刷新率 + 历史时间窗口）。
   * 唯一入口，负责重启渲染节拍定时器。快照轮询独立运行不受影响。
   */
  function applyDisplayPreferences(rateHz: number, windowSec: number): void {
    if (Number.isFinite(rateHz) && rateHz > 0) {
      renderRateHz.value = rateHz
    }
    if (Number.isFinite(windowSec) && windowSec > 0) {
      historyWindowSec.value = windowSec
    }
    const intervalMs = Math.max(MIN_TIMER_INTERVAL_MS, Math.round(1000 / renderRateHz.value))
    if (renderTickTimer.value !== null) {
      clearInterval(renderTickTimer.value)
      renderTickTimer.value = null
    }
    renderTickTimer.value = setInterval(renderTick, intervalMs)
    // 窗口/刷新率变化会改变容量，立即裁剪一次已有历史避免超限
    const capacity = computeHistoryCapacity()
    for (const id of Object.keys(historyMap.value)) {
      const h = historyMap.value[id]!
      if (h.length > capacity) {
        h.splice(0, h.length - capacity)
      }
    }
  }

  /** 停止 UI 渲染 + 快照轮询，flush 剩余 pending 数据 */
  function stopDisplayFlush(): void {
    if (renderTickTimer.value !== null) {
      clearInterval(renderTickTimer.value)
      renderTickTimer.value = null
    }
    stopSnapshotPolling()
    renderTick()
  }

  // 初始化默认节拍（未连接 displayStore 前的兜底）
  applyDisplayPreferences(RENDER_TICK_FALLBACK_HZ, HISTORY_WINDOW_FALLBACK_SEC)

  /** 初始化指定设备的通道选择为全选 */
  function initChartSelections(id: string, channels: ChannelConfig[]): void {
    const set = new Set<number>()
    for (const ch of channels) {
      if (ch.enabled) set.add(ch.index)
    }
    chartSelections.value[id] = set
  }

  async function loadProfiles(): Promise<void> {
    const list = await bridge.getProfiles()
    if (Array.isArray(list)) {
      profiles.value = list
      // 加载配置后默认全选所有已启用通道
      for (const p of list) {
        if (!chartSelections.value[p.id] || chartSelections.value[p.id]!.size === 0) {
          initChartSelections(p.id, p.channels)
        }
      }
      syncSelectedDevice()
    }
  }

  /** 自动连接所有开启了自动连接的设备 */
  async function autoConnectAll(): Promise<void> {
    const targets = profiles.value.filter(
      (p) => p.p1604Config?.autoConnect && statusFor(p.id) === 'Disconnected'
    )
    const results = await Promise.allSettled(targets.map(async (profile) => {
      await connect(profile.id)
      return profile
    }))

    const failures = results.flatMap((result, index) => {
      if (result.status === 'fulfilled') return []
      const profile = targets[index]
      const reason = result.reason instanceof Error ? result.reason.message : String(result.reason)
      return [{ profile, reason }]
    })

    if (failures.length > 0) {
      throw new Error(
        failures.map(({ profile, reason }) => `${profile.name || profile.id}: ${reason}`).join('; ')
      )
    }
  }

  async function transitionStatus(
    id: string,
    action: () => Promise<void>,
    targetStatus: string,
    fallbackStatus: string,
    preStatus?: string,
  ): Promise<void> {
    const prev = statusMap.value[id]
    if (preStatus) statusMap.value[id] = preStatus
    try {
      await action()
      statusMap.value[id] = targetStatus
      // 操作成功时清除错误信息
      if (targetStatus !== 'Error') {
        delete errorMap.value[id]
      }
    } catch (err) {
      statusMap.value[id] = prev ?? fallbackStatus
      // 操作失败时记录错误信息
      const reason = err instanceof Error ? err.message : String(err)
      errorMap.value[id] = reason
      throw err
    }
  }

  async function connect(id: string): Promise<void> {
    await transitionStatus(id, () => bridge.connect(id), 'Connected', 'Disconnected', 'Connecting')
  }

  async function disconnect(id: string): Promise<void> {
    await transitionStatus(id, () => bridge.disconnect(id), 'Disconnected', 'Disconnected')
  }

  async function startAcquisition(id: string): Promise<void> {
    try {
      await transitionStatus(
        id,
        () => withTimeout(bridge.startAcquisition(id), ACQUISITION_ACTION_TIMEOUT_MS, 'Start acquisition timed out'),
        'Acquiring',
        'Connected',
        'Starting',
      )
    } catch (err) {
      const state = await bridge.getStatus(id).catch(() => false)
      if (state && typeof state === 'object' && 'statusText' in state) {
        statusMap.value[id] = (state as any).statusText
      }
      throw err
    }
  }

  async function stopAcquisition(id: string): Promise<void> {
    try {
      await transitionStatus(
        id,
        () => withTimeout(bridge.stopAcquisition(id), ACQUISITION_ACTION_TIMEOUT_MS, 'Stop acquisition timed out'),
        'Connected',
        'Connected',
        'Stopping',
      )
    } catch (err) {
      const state = await bridge.getStatus(id).catch(() => false)
      if (state && typeof state === 'object' && 'statusText' in state) {
        statusMap.value[id] = (state as any).statusText
      }
      throw err
    }
  }

  async function applyConfig(id: string, cfg: Partial<P1604Config>): Promise<void> {
    await withTimeout(
      bridge.applyConfig(id, p1604Defaults(cfg)),
      APPLY_CONFIG_TIMEOUT_MS,
      '应用配置超时，设备可能无响应',
    )
  }

  async function updateChannel(id: string, index: number, patch: Partial<ChannelConfig>): Promise<void> {
    const profile = profiles.value.find((p) => p.id === id)
    if (!profile || index < 0 || index >= profile.channels.length) return
    // 先更新本地状态，再持久化到后端
    const ch = profile.channels[index]!
    Object.assign(ch, patch)
    try {
      await bridge.upsertProfile(profile)
    } catch {
      // 持久化失败时重新加载配置，恢复本地状态一致性
      await loadProfiles()
      throw new Error('保存配置失败')
    }
  }

  /** 批量设置某设备所有通道的精度（全局精度应用） */
  async function applyGlobalPrecision(id: string, precision: number): Promise<void> {
    const profile = profiles.value.find((p) => p.id === id)
    if (!profile) return
    const p = Math.max(0, Math.min(6, Math.floor(precision)))
    for (const ch of profile.channels) {
      ch.precision = p
    }
    try {
      await bridge.upsertProfile(profile)
    } catch {
      await loadProfiles()
      throw new Error('保存配置失败')
    }
  }

  async function saveProfile(profile: PressureProfile): Promise<void> {
    // 先更新本地状态，再持久化到后端（利用 await 的微任务边界让 Vue 处理 watcher）
    const index = profiles.value.findIndex((p) => p.id === profile.id)
    if (index >= 0) {
      profiles.value[index] = profile
    } else {
      profiles.value.push(profile)
    }
    try {
      await bridge.upsertProfile(profile)
    } catch {
      // 持久化失败时重新加载配置，恢复本地状态一致性
      await loadProfiles()
      throw new Error('保存配置失败')
    }
  }

  function clearScanResults(): void {
    scanResults.value = []
  }

  async function scanDevices(): Promise<void> {
    isScanning.value = true
    try {
      scanResults.value = await bridge.scanDevices()
    } finally {
      isScanning.value = false
    }
  }

  async function addProfile(name: string, address: string, port: number): Promise<void> {
    const id = `p1604_${Date.now()}`
    const profile: PressureProfile = {
      id,
      name,
      address,
      port,
      samplingRate: 100,
      channels: defaultChannels(),
      p1604Config: defaultP1604Config(),
      createdAt: Date.now(),
    }
    // 先更新本地状态，再持久化到后端
    profiles.value.push(profile)
    // 新增设备时默认全选所有通道
    initChartSelections(id, profile.channels)
    syncSelectedDevice()
    try {
      await bridge.upsertProfile(profile)
    } catch {
      // 持久化失败时重新加载配置，恢复本地状态一致性
      await loadProfiles()
      throw new Error('保存配置失败')
    }
  }

  /**
   * 已添加设备的 host key 集合（响应式）。
   * 扫描弹窗用于识别哪些扫描结果已经在 profile 列表中，从而置灰不可重加。
   */
  const existingDeviceKeys = computed(() => computeExistingKeys(profiles.value))

  /**
   * 批量添加扫描到的设备。
   *
   * 交互特性：
   * - 按 host:addr:port 去重（跳过已存在，不阻断其它条目）
   * - 名字冲突自动追加 (2)/(3)/...
   * - Profile ID 可复现（有 MAC 用 MAC；无 MAC 用 host-port），便于重扫幂等
   * - 每条 upsertProfile 独立处理，中途失败不影响已成功条目（Promise.allSettled）
   *
   * @param inputs 从扫描结果转换来的最小输入
   * @param options.defaultAutoConnect 顶部批量默认开关的值
   */
  async function addScannedProfiles(
    inputs: ScannedDeviceInput[],
    options: { defaultAutoConnect: boolean },
  ): Promise<AddScannedResult> {
    // 先规划：去重 + 命名 + ID 生成，纯计算无副作用，便于测试
    const plan = planScannedAdditions({
      inputs,
      existingProfiles: profiles.value,
      defaultAutoConnect: options.defaultAutoConnect,
    })

    const added: PressureProfile[] = []
    const failed: Array<{ input: PlannedProfile; error: string }> = []

    // 逐条构造完整 PressureProfile 并持久化；用 allSettled 保证单条失败不阻断整批
    const settled = await Promise.allSettled(
      plan.toAdd.map(async (planned) => {
        const profile: PressureProfile = {
          id: planned.id,
          name: planned.name,
          address: planned.address,
          port: planned.port,
          samplingRate: 100,
          channels: defaultChannels(),
          p1604Config: {
            ...defaultP1604Config(),
            autoConnect: planned.autoConnect,
          },
          createdAt: Date.now(),
        }
        // 先写本地 store：保证 UI 立即刷新
        profiles.value.push(profile)
        initChartSelections(profile.id, profile.channels)
        try {
          await bridge.upsertProfile(profile)
          return profile
        } catch (err) {
          // 回滚本地状态：从 profiles 移除并清理 chartSelections
          profiles.value = profiles.value.filter((p) => p.id !== profile.id)
          delete chartSelections.value[profile.id]
          throw err
        }
      }),
    )

    settled.forEach((result, idx) => {
      const planned = plan.toAdd[idx]!
      if (result.status === 'fulfilled') {
        added.push(result.value)
      } else {
        const reason = result.reason instanceof Error ? result.reason.message : String(result.reason)
        failed.push({ input: planned, error: reason })
      }
    })

    // 有成功条目时同步选中设备（若尚未选择）
    if (added.length > 0) {
      syncSelectedDevice()
    }

    return { added, skipped: plan.skipped, failed }
  }


  async function removeProfile(id: string): Promise<void> {
    await bridge.deleteProfile(id)
    profiles.value = profiles.value.filter((p) => p.id !== id)
    syncSelectedDevice()
  }

  /**
   * 从后端状态变更事件更新前端状态（连接断开、硬件单位同步等）。
   * 同时把状态转 'Error' 或带 error 字段的变更写入 logStore，
   * 让操作员在日志面板可见后端推送的设备异常（如 readLoop 异常退出）。
   *
   * 连接后若后端从硬件读取的单位与 profile 不一致，state.profile
   * 中已包含以硬件为准的更新值，此处同步到前端 profiles ref，
   * 确保配置面板显示的是硬件实际单位。
   */
  function updateStatusFromBackend(id: string, state: DeviceState): void {
    const prevStatus = statusMap.value[id]
    const statusChanged = prevStatus !== state.statusText
    if (statusChanged) {
      statusMap.value[id] = state.statusText
    }
    // 同步后端推送的错误信息（如连接断开原因）
    let errorChanged = false
    if (state.error) {
      errorChanged = errorMap.value[id] !== state.error
      errorMap.value[id] = state.error
    } else if (state.statusText !== 'Error') {
      // 非 Error 状态时清除错误
      if (errorMap.value[id]) {
        delete errorMap.value[id]
      }
    }

    // 同步后端推送的 profile（如连接时硬件单位与配置不一致，以硬件为准更新）
    if (state.profile) {
      const idx = profiles.value.findIndex((p) => p.id === id)
      if (idx >= 0) {
        // 仅在实际变化时更新，避免触发不必要的响应式依赖
        const prevUnit = profiles.value[idx]!.p1604Config.unit
        const newUnit = state.profile.p1604Config.unit
        if (prevUnit !== newUnit) {
          profiles.value[idx] = state.profile
          const logStore = useLogStore()
          logStore.info('device', `设备 [${id}] 单位已从硬件同步: ${prevUnit} -> ${newUnit}`)
        }
      }
    }

    // 仅在状态或错误实际变化时写日志，避免重复刷屏
    if (statusChanged || errorChanged) {
      const logStore = useLogStore()
      if (state.statusText === 'Error' && state.error) {
        logStore.error('device', `设备 [${id}] 状态异常: ${state.error}`)
      } else if (statusChanged && state.statusText === 'Disconnected' && prevStatus && prevStatus !== 'Disconnected') {
        // 后端推送的断开（区别于前端主动断开）—— 记录 info 便于追溯
        logStore.warn('device', `设备 [${id}] 已断开（后端推送，前一状态: ${prevStatus}）`)
      }
    }
  }

  return {
    profiles, selectedId, statusMap, errorMap, historyMap, snapshotMap, renderedSnapshotMap, chartSelections,
    scanResults, isScanning,
    selectedProfile, selectedSnapshot,
    renderRateHz, historyWindowSec,
    existingDeviceKeys,
    selectDevice, statusFor, errorFor, acquiringFor, historyFor, isChartSelected, toggleChartSelection,
    pushSnapshot, loadProfiles, autoConnectAll, connect, disconnect,
    startAcquisition, stopAcquisition, applyConfig, updateChannel, applyGlobalPrecision, saveProfile,
    clearScanResults, scanDevices, addProfile, addScannedProfiles, removeProfile, updateStatusFromBackend,
    applyDisplayPreferences, stopDisplayFlush,
    startSnapshotPolling, stopSnapshotPolling,
  }
})
