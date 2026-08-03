import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import * as bridge from '@bridge/deviceBridge'
import type { TemperatureProfile, TemperatureSnapshot, T1603Config, ChannelConfig, ScanResult, DeviceState } from '@bridge/deviceBridge'
import { useI18nStore } from '@stores/i18nStore'

const MAX_HISTORY = 200
const ACQUISITION_ACTION_TIMEOUT_MS = 8000
const APPLY_CONFIG_TIMEOUT_MS = 15000
// 连接超时：覆盖后端最坏耗时（DialTCP 5s + syncHardwareConfigLocked 4s = 9s）+ 1s 余量。
// 后端在故障 Windows 机器上 DialTCP watchdog 触发 5s + 配置同步 watchdog 触发 2-4s，
// 若无前端超时兜底，bridge.connect 永久 pending → UI 卡死在 'Connecting'。
// 与 startAcquisition/applyConfig 一致采用 withTimeout 包装，超时后翻转 UI 为 'Disconnected'。
const CONNECT_TIMEOUT_MS = 10000
const DISPLAY_REFRESH_RATE_FALLBACK_HZ = 10

const CHANNEL_COLORS = [
  '#3b82f6', '#10b981', '#f59e0b', '#a855f7',
  '#f43f5e', '#06b6d4', '#f97316', '#6366f1',
  '#84cc16', '#14b8a6', '#d946ef', '#0ea5e9',
  '#eab308', '#22c55e', '#ef4444', '#8b5cf6',
]

/**
 * 默认通道配置。
 *
 * name 留空：让 UI 通过 i18n 占位符显示本地化的默认名称（如"通道 1" / "Channel 1"），
 * 避免在持久化数据中固化某一种语言。用户手动输入的名称仍按原样保存。
 */
function defaultChannels() {
  return Array.from({ length: 16 }, (_, i) => ({
    index: i,
    name: '',
    enabled: true,
    unit: '°C',
    color: CHANNEL_COLORS[i % CHANNEL_COLORS.length],
    precision: 2,
    rangeMin: 0,
    rangeMax: 200,
    thermocoupleType: 'K',
  }))
}

function defaultT1603Config(): T1603Config {
  return {
    thermocoupleTypes: 'KKKKKKKKKKKKKKKK',
    channelMask: 'FFFF',
    samplingRate: 10,
    averageCount: 4,
    showTimestamp: false,
    showSequence: false,
    autoConnect: false,
  }
}

function t1603Defaults(cfg: Partial<T1603Config>): T1603Config {
  return {
    thermocoupleTypes: cfg.thermocoupleTypes ?? 'KKKKKKKKKKKKKKKK',
    channelMask: cfg.channelMask ?? 'FFFF',
    samplingRate: cfg.samplingRate ?? 10,
    averageCount: cfg.averageCount ?? 4,
    showTimestamp: cfg.showTimestamp ?? false,
    showSequence: cfg.showSequence ?? false,
    autoConnect: cfg.autoConnect ?? false,
  }
}

export const useDeviceStore = defineStore('device', () => {
  const i18n = useI18nStore()
  const profiles = ref<TemperatureProfile[]>([])
  const selectedId = ref<string | null>(null)
  const statusMap = ref<Record<string, string>>({})
  /** 设备级错误详情映射（key 为 deviceId，value 为后端返回的 Error 字段） */
  const errorMap = ref<Record<string, string>>({})
  const historyMap = ref<Record<string, TemperatureSnapshot[]>>({})
  const snapshotMap = ref<Record<string, TemperatureSnapshot>>({})
  const pendingSnapshotMap = ref<Record<string, TemperatureSnapshot>>({})
  const chartSelections = ref<Record<string, Set<number>>>({})
  const scanResults = ref<ScanResult[]>([])
  const isScanning = ref(false)
  const displayFlushTimer = ref<ReturnType<typeof setInterval> | null>(null)

  const selectedProfile = computed(() =>
    profiles.value.find((p) => p.id === selectedId.value) ?? null
  )
  const selectedSnapshot = computed(() =>
    selectedId.value ? snapshotMap.value[selectedId.value] ?? null : null
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

  /** 获取设备错误详情（无错误返回空字符串） */
  function errorFor(id: string): string {
    return errorMap.value[id] ?? ''
  }

  function acquiringFor(id: string): boolean {
    return statusMap.value[id] === 'Acquiring'
  }

  /**
   * 从后端同步设备状态到前端。
   * 同时更新 statusMap 与 errorMap：
   *   - 后端返回 error 非空 → 写入 errorMap
   *   - 后端返回 error 为空且状态非 'Error' → 清除 errorMap
   *   - 状态为 'Error' 时保留旧 error（若无新 error 信息）
   */
  function syncStatusFromBackend(id: string, state: DeviceState | false): void {
    if (!state || typeof state !== 'object' || !('statusText' in state)) return
    statusMap.value[id] = state.statusText
    if (state.error) {
      errorMap.value[id] = state.error
    } else if (state.statusText !== 'Error') {
      delete errorMap.value[id]
    }
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

  function historyFor(id: string): TemperatureSnapshot[] {
    return historyMap.value[id] ?? []
  }

  /** 清空指定设备的实时波形历史（#31：去掉当前波形）
   *  仅清前端缓冲，不影响后端采集与录制 */
  function clearHistory(id: string): void {
    historyMap.value[id] = []
    delete snapshotMap.value[id]
    delete pendingSnapshotMap.value[id]
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

  function pushSnapshot(snapshot: TemperatureSnapshot): void {
    snapshotMap.value[snapshot.deviceId] = snapshot
    pendingSnapshotMap.value[snapshot.deviceId] = snapshot
  }

  function flushPendingSnapshots(): void {
    const pendingEntries = Object.entries(pendingSnapshotMap.value)
    if (pendingEntries.length === 0) return

    for (const [deviceId, snapshot] of pendingEntries) {
      if (!historyMap.value[deviceId]) {
        historyMap.value[deviceId] = []
      }
      const history = historyMap.value[deviceId]!
      history.push(snapshot)
      if (history.length > MAX_HISTORY) {
        history.splice(0, history.length - MAX_HISTORY)
      }
    }
    pendingSnapshotMap.value = {}
  }

  function setDisplayRefreshRateHz(rateHz: number): void {
    if (!Number.isFinite(rateHz) || rateHz <= 0) return
    if (displayFlushTimer.value !== null) {
      clearInterval(displayFlushTimer.value)
      displayFlushTimer.value = null
    }
    const intervalMs = Math.max(16, Math.round(1000 / rateHz))
    displayFlushTimer.value = setInterval(flushPendingSnapshots, intervalMs)
  }

  function stopDisplayFlush(): void {
    if (displayFlushTimer.value !== null) {
      clearInterval(displayFlushTimer.value)
      displayFlushTimer.value = null
    }
    flushPendingSnapshots()
  }

  setDisplayRefreshRateHz(DISPLAY_REFRESH_RATE_FALLBACK_HZ)

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
      (p) => p.t1603Config?.autoConnect && statusFor(p.id) === 'Disconnected'
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
      // 操作成功且目标状态非 Error → 清除错误
      if (targetStatus !== 'Error') delete errorMap.value[id]
    } catch (err) {
      statusMap.value[id] = prev ?? fallbackStatus
      throw err
    }
  }

  /**
   * 操作失败后从后端同步真实状态。
   * 后端在失败时已通过 EmitLog（error 级别）写入日志，前端通过 onLog 订阅即可收到，
   * 此处不再重复写 logStore，避免同一错误产生中英两条日志。仅同步状态确保 UI 准确。
   */
  async function syncAndLogFailure(id: string): Promise<void> {
    const state = await bridge.getStatus(id).catch(() => false)
    syncStatusFromBackend(id, state as DeviceState | false)
  }

  async function connect(id: string): Promise<void> {
    try {
      await transitionStatus(
        id,
        () => withTimeout(bridge.connect(id), CONNECT_TIMEOUT_MS, 'Connect timed out'),
        'Connected',
        'Disconnected',
        'Connecting',
      )
    } catch (err) {
      await syncAndLogFailure(id)
      throw err
    }
  }

  async function disconnect(id: string): Promise<void> {
    try {
      await transitionStatus(id, () => bridge.disconnect(id), 'Disconnected', 'Disconnected')
    } catch (err) {
      await syncAndLogFailure(id)
      throw err
    }
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
      await syncAndLogFailure(id)
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
      await syncAndLogFailure(id)
      throw err
    }
  }

  async function applyConfig(id: string, cfg: Partial<T1603Config>): Promise<void> {
    await withTimeout(
      bridge.applyConfig(id, t1603Defaults(cfg)),
      APPLY_CONFIG_TIMEOUT_MS,
      i18n.t('error.applyConfigTimeout'),
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
      throw new Error(i18n.t('error.saveConfigFailed'))
    }
  }

  async function saveProfile(profile: TemperatureProfile): Promise<void> {
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
      throw new Error(i18n.t('error.saveConfigFailed'))
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

  /**
   * 判断扫描结果对应的设备是否已被添加为 profile。
   *
   * 匹配优先级：
   * 1. IP 地址 + 端口（去除空白后小写比较）——同一 IP:Port 视为同一物理访问点
   *
   * 注：当前 TemperatureProfile 不持久化 MAC / 序列号，因此仅用网络端点匹配。
   * 若后续 profile 增加 macAddress / serialNumber 字段，可在此扩展匹配规则。
   */
  function isScanResultAdded(result: ScanResult): boolean {
    const targetAddress = (result.address ?? '').trim().toLowerCase()
    const targetPort = result.port
    if (!targetAddress) return false
    return profiles.value.some(
      (p) => p.address.trim().toLowerCase() === targetAddress && p.port === targetPort,
    )
  }

  async function addProfile(name: string, address: string, port: number): Promise<void> {
    // 重复添加防御：若同一 IP:Port 已存在，则拒绝添加
    const normalizedAddress = address.trim().toLowerCase()
    const duplicated = profiles.value.some(
      (p) => p.address.trim().toLowerCase() === normalizedAddress && p.port === port,
    )
    if (duplicated) {
      throw new Error(i18n.t('error.duplicateDevice'))
    }

    const id = `t1603_${Date.now()}`
    const profile: TemperatureProfile = {
      id,
      name,
      address,
      port,
      samplingRate: 10,
      channels: defaultChannels(),
      t1603Config: defaultT1603Config(),
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
      throw new Error(i18n.t('error.saveConfigFailed'))
    }
  }

  async function removeProfile(id: string): Promise<void> {
    await bridge.deleteProfile(id)
    profiles.value = profiles.value.filter((p) => p.id !== id)
    syncSelectedDevice()
  }

  return {
    profiles, selectedId, statusMap, errorMap, historyMap, snapshotMap, chartSelections,
    scanResults, isScanning,
    selectedProfile, selectedSnapshot,
    selectDevice, statusFor, errorFor, acquiringFor, historyFor, clearHistory, isChartSelected, toggleChartSelection,
    pushSnapshot, loadProfiles, autoConnectAll, connect, disconnect,
    startAcquisition, stopAcquisition, applyConfig, updateChannel, saveProfile,
    clearScanResults, scanDevices, addProfile, removeProfile, isScanResultAdded,
    setDisplayRefreshRateHz, stopDisplayFlush,
    // 暴露给 App.vue 订阅 daq:device-state 事件后调用，将后端推送的状态同步进 store
    syncStatusFromBackend,
  }
})
